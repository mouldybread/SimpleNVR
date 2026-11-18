package main

import (
 "context"
 "testing"
 "time"
)

func TestDuplicateCameraNamesUseLongestEventRetention(t *testing.T) {
 for iteration:=0;iteration<32;iteration++ {
  for _,days:=range [][]int{{1,30,7},{30,7,1},{7,1,30},{0,1,3}} {
   cameras:=make(map[string]*Camera)
   for index,value:=range days {id:=string(rune('a'+index));cameras[id]=&Camera{ID:id,Name:"Shared",RetentionDays:value}}
   age:=10*24*time.Hour;if days[0]==0 {age=4*24*time.Hour}
   n:=&NVR{cameras:cameras,events:[]Event{{ID:"keep",CameraName:"Shared",StartTime:time.Now().Add(-age)}}}
   saved:=0;cleanupEventsForWithSave(context.Background(),n,func() error {saved++;return nil})
   if len(n.events)!=1 || n.events[0].ID!="keep" || saved!=0 {t.Fatalf("shorter duplicate-name policy expired metadata: %v",days)}
  }
 }
}

func TestEventExpiryUsesMatchingNameAndStillPrunesExpiredAndOrphanedEvents(t *testing.T) {
 now:=time.Now()
 n:=&NVR{cameras:map[string]*Camera{
  "a":{ID:"a",Name:"Shared",RetentionDays:1},
  "b":{ID:"b",Name:"Shared",RetentionDays:30},
  "c":{ID:"c",Name:"Other",RetentionDays:365},
 },events:[]Event{
  {ID:"keep",CameraName:"Shared",StartTime:now.Add(-10*24*time.Hour)},
  {ID:"expired",CameraName:"Shared",StartTime:now.Add(-31*24*time.Hour)},
  {ID:"orphan",CameraName:"Missing",StartTime:now},
 }}
 saved:=0;cleanupEventsForWithSave(context.Background(),n,func() error {saved++;return nil})
 if len(n.events)!=1 || n.events[0].ID!="keep" || saved!=1 {t.Fatalf("unexpected expiry result: %+v, saves=%d",n.events,saved)}
}

func TestAnyInvalidDuplicatePolicyPreservesSharedNameEvents(t *testing.T) {
 for _,invalid:=range []int{-1,366,106752} {
  for _,reverse:=range []bool{false,true} {
   first,second:=1,invalid;if reverse {first,second=second,first}
   n:=&NVR{cameras:map[string]*Camera{"a":{ID:"a",Name:"Shared",RetentionDays:first},"b":{ID:"b",Name:"Shared",RetentionDays:second}},events:[]Event{{ID:"keep",CameraName:"Shared",StartTime:time.Date(2000,1,1,0,0,0,0,time.UTC)}}}
   saved:=0;cleanupEventsForWithSave(context.Background(),n,func() error {saved++;return nil})
   if len(n.events)!=1 || saved!=0 {t.Fatal("invalid duplicate policy did not fail closed")}
  }
 }
}

func TestCanceledEventExpiryDoesNotMutateOrPersist(t *testing.T) {
 n:=&NVR{cameras:map[string]*Camera{},events:[]Event{{ID:"keep",CameraName:"Missing",StartTime:time.Now()}}}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 saved:=0;cleanupEventsForWithSave(ctx,n,func() error {saved++;return nil})
 if len(n.events)!=1 || n.events[0].ID!="keep" || saved!=0 {t.Fatal("canceled event expiry changed metadata")}
}
