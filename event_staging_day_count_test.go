package main

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "testing"
)

func TestEventStagingCountsDayRootsNotPublishedContents(t *testing.T) {
 root:=t.TempDir();base:=scanTime("20260101","000000")
 const days=365
 for d:=0;d<days;d++ {
  day:=filepath.Join(root,stampedDayName(base.AddDate(0,0,d)))
  if err:=os.Mkdir(day,0700);err!=nil {t.Fatal(err)}
  for i:=0;i<24;i++ {if err:=os.WriteFile(filepath.Join(day,fmt.Sprintf("published-%02d.mp4",i)),nil,0600);err!=nil {t.Fatal(err)}}
 }
 // More than 8,192 published physical files exist, but only their 365 day
 // entries count. Actual root .tmp and temp/event/file entries are charged too.
 putStagingAdmissionFile(t,filepath.Join(root,"capture.mp4.tmp"),"12")
 evidence:=filepath.Join(root,"temp","event-one","pre.mp4")
 putStagingAdmissionFile(t,evidence,"345")
 usage,err:=scanEventStaging(context.Background(),root,days+4,eventStagingScanDepth)
 if err!=nil || usage.entries!=days+4 || usage.bytes!=5 || usage.dirs!=1 {t.Fatal(usage,err)}
 if _,err:=scanEventStaging(context.Background(),root,days+3,eventStagingScanDepth);err==nil {t.Fatal("staging entry bound was bypassed")}
 assertPublicationBytes(t,evidence,"345")
}
