package main

import (
 "bytes"
 "context"
 "errors"
 "os"
 "strings"
 "testing"
)

func TestCorrPrototypeExactBytesAndLimits(t *testing.T) {
 for _,kind:=range []string{"equal","reorder","late difference","empty","cancelled","over limit"} {t.Run(kind,func(t *testing.T){
  a,err:=os.CreateTemp(t.TempDir(),"a");if err!=nil {t.Fatal(err)};defer a.Close()
  b,err:=os.CreateTemp(t.TempDir(),"b");if err!=nil {t.Fatal(err)};defer b.Close()
  data:=bytes.Repeat([]byte("ab"),70000);other:=append([]byte(nil),data...)
  switch kind {
  case "reorder":other[0],other[1]=other[1],other[0]
  case "late difference":other[len(other)-1]='x'
  case "empty":data=nil;other=nil
  }
  if _,err:=a.Write(data);err!=nil {t.Fatal(err)}
  if _,err:=b.Write(other);err!=nil {t.Fatal(err)}
  if kind=="over limit" {if err:=a.Truncate(corrPrototypeStreamLimit+1);err!=nil {t.Fatal(err)}}
  ctx,cancel:=context.WithCancel(context.Background());defer cancel()
  if kind=="cancelled" {cancel()}
  equal,_,err:=corrPrototypeEqual(ctx,a,b)
  if kind=="equal" {if err!=nil || !equal {t.Fatal(equal,err)}} else if equal {t.Fatal("invalid comparison accepted")}
  if (kind=="empty" || kind=="over limit" || kind=="cancelled") && err==nil {t.Fatal("missing refusal")}
  offset,err:=a.Seek(0,1);if err!=nil || offset!=int64(len(data)) {t.Fatal("comparison changed donor offset",offset,err)}
 })}
}

func TestCorrPrototypeSinkRefusesBeforeExcessWrite(t *testing.T) {
 var dst bytes.Buffer
 sink:=&corrPrototypeSink{ctx:context.Background(),writer:&dst,written:corrPrototypeStreamLimit-1}
 if _,err:=sink.Write([]byte("xx"));!errors.Is(err,errCorrPrototype) || dst.Len()!=0 {t.Fatal(err,dst.Len())}
 ctx,cancel:=context.WithCancel(context.Background());cancel();sink.ctx=ctx
 if _,err:=sink.Write([]byte("x"));!errors.Is(err,context.Canceled) || dst.Len()!=0 {t.Fatal(err,dst.Len())}
}

func TestCorrPrototypeDescriptorOnlyCommandPlans(t *testing.T) {
 for _,audio:=range []bool{false,true} {
  args,plan,err:=corrPrototypeArguments(true,3,audio);if err!=nil {t.Fatal(err)}
  want:="file 'file:/proc/self/fd/3'\nfile 'file:/proc/self/fd/4'\nfile 'file:/proc/self/fd/5'\n"
  if plan!=want {t.Fatal(plan)}
  joined:=strings.Join(args," ")
  if !strings.Contains(joined,"-protocol_whitelist file,pipe") || !strings.Contains(joined,"-i pipe:0") {t.Fatal(joined)}
  if strings.Contains(joined,"setts=")!=audio {t.Fatal("ordinal timestamps escaped audio sink",joined)}
  args,plan,err=corrPrototypeArguments(false,1,audio)
  if err!=nil || plan!="" || !strings.Contains(strings.Join(args," "),"-f mov -i /proc/self/fd/3") {t.Fatal(args,plan,err)}
 }
 for _,count:=range []int{0,maxRecoveryInputs+2} {if _,_,err:=corrPrototypeArguments(true,count,false);err==nil {t.Fatal(count)}}
 if _,_,err:=corrPrototypeArguments(false,2,false);err==nil {t.Fatal("multiple actual outputs accepted")}
}

func TestCorrPrototypeRejectsUnboundEvidenceWithoutSpawning(t *testing.T) {
 var absent *eventDurationEvidence
 report,err:=corrPrototypeInspect(context.Background(),nil,absent,t.TempDir())
 if err==nil || report.ProcessesStarted!=0 {t.Fatal(report,err)}
}
