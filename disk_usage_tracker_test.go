package main

import (
 "context"
 "path/filepath"
 "sync/atomic"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func blockingUsageCompute(release chan struct{},calls *int32,total int64) func(context.Context,*diskUsageCache,string)(int64,error) {
 return func(context.Context,*diskUsageCache,string)(int64,error) {
  atomic.AddInt32(calls,1)
  <-release
  return total,nil
 }
}

func TestDiskUsageTrackerSharesOneInspectionPerCamera(t *testing.T) {
 release:=make(chan struct{});var calls int32
 tracker:=newDiskUsageTracker(newDiskUsageCache(),4,time.Minute,blockingUsageCompute(release,&calls,42))
 first:=tracker.start("camera");second:=tracker.start("camera")
 if first!=second {t.Fatal("concurrent requests started separate inspections")}
 close(release)
 if got:=first.wait(context.Background());got!="42 B" {t.Fatal(got)}
 if got:=second.wait(context.Background());got!="42 B" {t.Fatal(got)}
 third:=tracker.start("camera")
 if third==first {t.Fatal("a finished inspection was reused")}
 third.wait(context.Background())
 if n:=atomic.LoadInt32(&calls);n!=2 {t.Fatal("inspections run",n)}
}

func TestDiskUsageWaitersHaveIndependentDeadlines(t *testing.T) {
 release:=make(chan struct{});var calls int32
 tracker:=newDiskUsageTracker(newDiskUsageCache(),4,time.Minute,blockingUsageCompute(release,&calls,7))
 call:=tracker.start("camera")
 short,cancel:=context.WithTimeout(context.Background(),20*time.Millisecond);defer cancel()
 if got:=call.wait(short);got!=diskUsageUnavailable {t.Fatal(got)}
 gone,stop:=context.WithCancel(context.Background());stop()
 if got:=call.wait(gone);got!=diskUsageUnavailable {t.Fatal(got)}
 renderCtx,renderCancel:=context.WithTimeout(context.Background(),20*time.Millisecond);defer renderCancel()
 if out:=tracker.render(renderCtx,[]string{"camera"});len(out)!=1 || out[0]!=diskUsageUnavailable {t.Fatal(out)}
 close(release)
 if got:=call.wait(context.Background());got!="7 B" {t.Fatal("waiters giving up cancelled the shared inspection",got)}
 if n:=atomic.LoadInt32(&calls);n!=1 {t.Fatal("extra inspections started",n)}
}

func TestDiskUsageTrackerBoundsConcurrentInspections(t *testing.T) {
 release:=make(chan struct{});entered:=make(chan string,8);var running,peak int32
 compute:=func(_ context.Context,_ *diskUsageCache,dir string)(int64,error) {
  now:=atomic.AddInt32(&running,1)
  for {
   old:=atomic.LoadInt32(&peak)
   if now<=old || atomic.CompareAndSwapInt32(&peak,old,now) {break}
  }
  entered<-dir
  <-release
  atomic.AddInt32(&running,-1)
  return 1,nil
 }
 tracker:=newDiskUsageTracker(newDiskUsageCache(),2,time.Minute,compute)
 var calls []*diskUsageCall
 for _,dir:=range []string{"a","b","c","d","e"} {calls=append(calls,tracker.start(dir))}
 for i:=0;i<2;i++ {
  select {
  case <-entered:
  case <-time.After(5*time.Second):t.Fatal("inspection did not start")
  }
 }
 select {
 case dir:=<-entered:t.Fatal("third inspection started while two were running",dir)
 case <-time.After(100*time.Millisecond):
 }
 close(release)
 for _,call:=range calls {call.wait(context.Background())}
 if p:=atomic.LoadInt32(&peak);p!=2 {t.Fatal("peak concurrent inspections",p)}
}

func TestDiskUsageEvictionPruneAndStaleStores(t *testing.T) {
 cache:=newDiskUsageCache()
 compute:=func(ctx context.Context,c *diskUsageCache,dir string)(int64,error) {
  return computeDiskUsage(ctx,c,dir,farFutureDiskUsageWork())
 }
 tracker:=newDiskUsageTracker(cache,2,time.Minute,compute)
 a:=filepath.Join(t.TempDir(),"a");b:=filepath.Join(t.TempDir(),"b")
 for _,dir:=range []string{a,b} {
  for _,date:=range []string{"20200101","20200102"} {
   scanFixture(t,dir,scanTime(date,"100000"),scanTime(date,"101000"),"x")
  }
 }
 got:=tracker.render(context.Background(),[]string{a,b})
 if len(got)!=2 || got[0]!="14 B" || got[1]!="14 B" || cache.size(a)!=2 || cache.size(b)!=2 {t.Fatal(got,cache.size(a),cache.size(b))}
 got=tracker.render(context.Background(),[]string{a})
 if len(got)!=1 || got[0]!="14 B" || cache.size(a)!=2 || cache.size(b)!=0 {t.Fatal("directory no longer registered was not pruned",got,cache.size(a),cache.size(b))}
 stale:=cache.generation(a);cache.evict(a)
 var stat unix.Stat_t
 cache.store(a,stale,"20200101",&stat,5)
 cache.retain(a,stale,map[string]bool{})
 if cache.size(a)!=0 {t.Fatal("an inspection that began before eviction stored into the new state")}
 got=tracker.render(context.Background(),[]string{a})
 if len(got)!=1 || got[0]!="14 B" || cache.size(a)!=2 {t.Fatal("re-added directory did not start fresh",got,cache.size(a))}
}

func TestCameraDeletionEvictsDiskUsageCache(t *testing.T) {
 n,cam:=lifecycleFixture(ModeContinuous)
 dir:=filepath.Join(t.TempDir(),"camera");cam.RecordDir=dir
 scanFixture(t,dir,scanTime("20200101","100000"),scanTime("20200101","101000"),"x")
 if _,err:=computeDiskUsage(context.Background(),recordingDiskUsageCache,dir,farFutureDiskUsageWork());err!=nil || recordingDiskUsageCache.size(dir)!=1 {t.Fatal("cache not populated",err)}
 deps:=deletionDeps(n,func()error{return nil},func(context.Context,string,func()error)error{return nil})
 if err:=deleteManagedCameras(context.Background(),n,[]string{cam.ID},deps);err!=nil {t.Fatal(err)}
 n.mu.RLock();_,registered:=n.cameras[cam.ID];n.mu.RUnlock()
 if registered {t.Fatal("camera still registered")}
 if recordingDiskUsageCache.size(dir)!=0 {t.Fatal("a deleted camera's cached days were kept")}
}
