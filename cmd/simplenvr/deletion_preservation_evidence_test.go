package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"

 "golang.org/x/sys/unix"
)

func TestRetentionInspectedDayFallbackAndPartialPreservation(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();cutoff:=scanTime("20261110","000000")
 expired:=scanFixture(t,dir,cutoff.Add(-24*time.Hour),cutoff.Add(-time.Second),"old_partial")
 equal:=scanFixture(t,dir,cutoff.Add(-24*time.Hour),cutoff,"equal_partial")
 crossing:=scanFixture(t,dir,cutoff.Add(-time.Hour),cutoff.Add(time.Hour),"cross")
 day:=filepath.Dir(expired)
 prefix:=filepath.Join(day,"20261101_000000_legacy.mp4")
 fallback:=filepath.Join(day,"unparseable_long_name.mp4")
 short:=filepath.Join(day,"short.mp4")
 for _,path:=range []string{prefix,fallback,short} {
  putCleanupFixture(t,path,"keep-or-expire")
  if err:=os.Chtimes(path,cutoff.Add(-48*time.Hour),cutoff.Add(-48*time.Hour));err!=nil {t.Fatal(err)}
 }
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,nil)
 if err!=nil || result.Count!=3 {t.Fatal(result,err)}
 for _,path:=range []string{expired,prefix,fallback} {if _,err:=os.Lstat(path);!os.IsNotExist(err) {t.Fatal(path,err)}}
 for _,path:=range []string{equal,crossing,short} {if _,err:=os.Lstat(path);err!=nil {t.Fatal(path,err)}}
}

func TestPublishedDayPreflightRejectsReplacementAndChangedEntry(t *testing.T) {
 for _,kind:=range []string{"parent","root","day","entry"} {
  t.Run(kind,func(t *testing.T) {
   lease:=publishedEngineLease(t);parent:=filepath.Join(t.TempDir(),"parent");dir:=filepath.Join(parent,"camera")
   start:=scanTime("20261101","100000")
   a:=scanFixture(t,dir,start,start.Add(time.Minute),"a")
   b:=scanFixture(t,dir,start,start.Add(time.Minute),"b")
   selected,calls:=0,0
   selectFile:=func(string,time.Time)bool {
    selected++
    if selected==2 {
     switch kind {
     case "parent":if err:=os.Rename(parent,parent+"-moved");err!=nil {t.Fatal(err)};putCleanupFixture(t,filepath.Join(dir,"replacement.mp4"),"replacement")
     case "root":if err:=os.Rename(dir,dir+"-moved");err!=nil {t.Fatal(err)};putCleanupFixture(t,filepath.Join(dir,"replacement.mp4"),"replacement")
     case "day":day:=filepath.Dir(a);if err:=os.Rename(day,day+"-moved");err!=nil {t.Fatal(err)};putCleanupFixture(t,filepath.Join(day,"replacement.mp4"),"replacement")
     case "entry":putCleanupFixture(t,a,"changed evidence is longer")
     }
    }
    return true
   }
   result,err:=deletePublishedLeasedWithUnlink(context.Background(),lease,dir,selectFile,publishedEngineGuard,func(int,string,int)error{calls++;return nil})
   if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || calls!=0 || result.Failures!=1 {t.Fatal(result,err,calls)}
   if kind=="entry" {assertPublicationBytes(t,a,"changed evidence is longer");assertPublicationBytes(t,b,"fixture")}
  })
 }
}

func TestPublishedDayPartialFailureAccounting(t *testing.T) {
 for _,kind:=range []string{"cancel","policy","shutdown","unlink","sync","metadata"} {
  t.Run(kind,func(t *testing.T) {
   lease:=publishedEngineLease(t);dir:=t.TempDir();start:=scanTime("20261101","100000")
   a:=scanFixture(t,dir,start,start.Add(time.Minute),"a")
   b:=scanFixture(t,dir,start,start.Add(time.Minute),"b")
   ctx,cancel:=context.WithCancel(context.Background());defer cancel()
   changed:=false;n:=&NVR{}
   guard:=func()error {
    if kind=="policy" && changed {return errRecordingStateChanged}
    if n.shuttingDown.Load() {return errRecorderShuttingDown}
    return nil
   }
   work:=newPublishedDeletionWork()
   if kind=="sync" {work.syncHook=func(*os.File)error{return os.ErrPermission}}
   if kind=="metadata" {work.statatHook=func(int,string,*unix.Stat_t,int)error{return os.ErrPermission}}
   calls:=0
   result,err:=deletePublishedLeasedWithWork(ctx,lease,dir,publishedEngineAll,guard,func(fd int,name string,flags int)error {
    if flags==0 {
     calls++
     if kind=="unlink" && calls==2 {return os.ErrPermission}
    }
    err:=unix.Unlinkat(fd,name,flags)
    if err==nil && flags==0 {
     changed=true
     if kind=="cancel" {cancel()}
     if kind=="shutdown" {n.shuttingDown.Store(true)}
    }
    return err
   },work)
   want:=1;if kind=="sync" {want=2};if kind=="metadata" {want=0}
   if err==nil || result.Count!=want || result.Bytes!=int64(want*7) || result.Failures!=1 || work.Stats.Unlinks!=want {t.Fatal(result,work.Stats,err)}
   if kind=="cancel" && !errors.Is(err,context.Canceled) {t.Fatal(err)}
   if kind=="policy" && !errors.Is(err,errRecordingStateChanged) {t.Fatal(err)}
   if kind=="shutdown" && !errors.Is(err,errRecorderShuttingDown) {t.Fatal(err)}
   if want<2 {assertPublicationBytes(t,b,"fixture")};if want==0 {assertPublicationBytes(t,a,"fixture")}
   if want>0 && work.Stats.SyncAttempts==0 {t.Fatal("partial mutation skipped sync attempts",work.Stats)}
  })
 }
}

func TestCameraDayReplacementAfterFirstUnlinkStops(t *testing.T) {
 dir,a,b:=cameraDayFixture(t);day:=filepath.Dir(a);moved:=day+"-moved";calls:=0;first:=""
 err:=removeCameraDirectoryWithUnlink(context.Background(),dir,deletionGuard,func(fd int,name string,flags int)error {
  calls++;first=name
  if err:=os.Rename(day,moved);err!=nil {return err}
  putCleanupFixture(t,filepath.Join(day,filepath.Base(a)),"replacement")
  putCleanupFixture(t,filepath.Join(day,filepath.Base(b)),"replacement")
  return unix.Unlinkat(fd,name,flags)
 })
 if !errors.Is(err,errCameraDeleteBlocked) || calls!=1 {t.Fatal(err,calls)}
 assertPublicationBytes(t,a,"replacement");assertPublicationBytes(t,b,"replacement")
 other:=filepath.Base(a);if first==other {other=filepath.Base(b)}
 assertPublicationBytes(t,filepath.Join(moved,other),"fixture")
}

func TestOrphanDayReplacementCannotRedirectRemoval(t *testing.T) {
 for _,kind:=range []string{"camera","day"} {
  t.Run(kind,func(t *testing.T) {
   root,dir,a,b:=orphanDayPair(t);day:=filepath.Dir(a)
   ops:=orphanCleanupIOFor(context.Background());read:=ops.readDir;replaced:=false;removes:=0
   ops.readDir=func(path string)([]os.DirEntry,error) {
    entries,err:=read(path)
    if path==day && err==nil && !replaced {
     replaced=true;pathToMove:=dir;if kind=="day" {pathToMove=day}
     if err:=os.Rename(pathToMove,pathToMove+"-moved");err!=nil {t.Fatal(err)}
     putCleanupFixture(t,a,"replacement");putCleanupFixture(t,b,"replacement")
    }
    return entries,err
   }
   ops.remove=func(string)error{removes++;return nil}
   result,err:=cleanupOrphanedWithIO(root,nil,ops)
   if err!=nil || !replaced || removes!=0 || result.Count!=0 || result.Failures!=1 {t.Fatal(result,err,replaced,removes)}
   assertPublicationBytes(t,a,"replacement");assertPublicationBytes(t,b,"replacement")
  })
 }
}
