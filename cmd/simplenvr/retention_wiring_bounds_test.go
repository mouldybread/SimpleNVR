package main

import (
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// Guards the production wiring only: the time-based entry point must use the
// retention policy that skips newer-day contents. Generic selection would delete
// the misplaced expired file below, so this fails if the entry reverts to it.
func TestProductionRetentionEntryAppliesDaySkipPolicy(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir()
 now:=time.Now().UTC();old:=now.Add(-72*time.Hour)
 expired:=scanFixture(t,dir,old,old.Add(time.Minute),"expired")
 today:=filepath.Join(dir,stampedDayName(now))
 if err:=os.Mkdir(today,0700);err!=nil {t.Fatal(err)}
 misplaced:=filepath.Join(today,stampedFileName(old,old.Add(time.Hour),"misplaced"))
 if err:=os.WriteFile(misplaced,[]byte("x"),0600);err!=nil {t.Fatal(err)}
 if err:=cleanupExpiredPublishedLeased(context.Background(),lease,dir,24*time.Hour,publishedEngineGuard);err!=nil {t.Fatal(err)}
 if _,err:=os.Lstat(expired);!os.IsNotExist(err) {t.Fatal("control file was not deleted",err)}
 if _,err:=os.Lstat(misplaced);err!=nil {t.Fatal("production retention entry did not apply the newer-day skip",err)}
}

// The root bound counts every name, including skipped day folders. Exactly 8,192
// root entries are accepted; the 8,193rd is refused before any deletion.
func TestRetentionRootEntryBoundExact(t *testing.T) {
 lease:=publishedEngineLease(t);dir:=t.TempDir();cutoff:=scanTime("20261110","120000")
 if err:=os.Mkdir(filepath.Join(dir,"20261111"),0700);err!=nil {t.Fatal(err)}
 for i:=0;i<publishedDeletionEntryLimit-1;i++ {
  if err:=os.WriteFile(filepath.Join(dir,fmt.Sprintf("n%05d.txt",i)),nil,0600);err!=nil {t.Fatal(err)}
 }
 work:=newPublishedDeletionWork()
 result,err:=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,work)
 if err!=nil || result.Count!=0 || work.Stats.EntriesProcessed!=publishedDeletionEntryLimit || work.Stats.DayFolders!=1 || work.Stats.DaysSkipped!=1 {t.Fatal("8192 root entries refused",result,work.Stats,err)}
 if err:=os.WriteFile(filepath.Join(dir,"overflow.txt"),nil,0600);err!=nil {t.Fatal(err)}
 over:=newPublishedDeletionWork()
 result,err=deleteExpiredPublishedLeasedAt(context.Background(),lease,dir,cutoff,publishedEngineGuard,over)
 if !errors.Is(err,errPublishedDeletionRefused) || result.Count!=0 || result.Failures!=1 || over.Stats.EntriesProcessed!=publishedDeletionEntryLimit+1 || over.Stats.Unlinks!=0 {t.Fatal("8193 root entries were not refused",result,over.Stats,err)}
}
