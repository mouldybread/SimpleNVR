package main

import (
 "bytes"
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "strconv"
 "strings"
 "testing"
 "time"
)

func TestPresentationProbeBufferDoesNotExposeBypassMethods(t *testing.T) {
 b:=&eventPresentationProbeBuffer{limit:3}
 if _,ok:=interface{}(b).(io.ReaderFrom);ok {t.Fatal("ReadFrom bypasses the limiting Write")}
 if _,ok:=interface{}(b).(io.StringWriter);ok {t.Fatal("WriteString bypasses the limiting Write")}
}
func TestPresentationProbeBufferCopyRoutesRemainBounded(t *testing.T) {
 for _,fastSource:=range []bool{false,true} {t.Run(strconv.FormatBool(fastSource),func(t *testing.T){
  b:=&eventPresentationProbeBuffer{limit:3}
  var source io.Reader=strings.NewReader("1234")
  if !fastSource {source=struct{io.Reader}{source}}
  _,err:=io.Copy(b,source)
  if !errors.Is(err,errEventVideoPresentation) || !b.refused || b.Len()>3 {t.Fatal("copy bypassed limit",b.Len(),b.refused,err)}
  if n,err:=b.Write(nil);n!=0 || !errors.Is(err,errEventVideoPresentation) {t.Fatal("refusal was not sticky",n,err)}
 })}
 b:=&eventPresentationProbeBuffer{limit:3}
 if n,err:=io.WriteString(b,"123");n!=3 || err!=nil || b.String()!="123" {t.Fatal(n,err)}
 if n,err:=io.WriteString(b,"4");n!=0 || !errors.Is(err,errEventVideoPresentation) || b.Len()!=3 {t.Fatal(n,err)}
}

// Re-exec only this helper to exercise os/exec's real stdout/stderr pipe copying.
// No media executable or production probe is started by these regressions.
func TestPresentationProbeLimitChild(t *testing.T) {
 stream:=os.Getenv("SIMPLENVR_PRESENTATION_LIMIT_CHILD")
 if stream=="" {return}
 size,err:=strconv.Atoi(os.Getenv("SIMPLENVR_PRESENTATION_LIMIT_SIZE"))
 if err!=nil || size<0 || size>1024 {os.Exit(2)}
 target:=os.Stdout
 if stream=="stderr" {target=os.Stderr} else if stream!="stdout" {os.Exit(2)}
 _,_=target.Write(bytes.Repeat([]byte("x"),size))
 os.Exit(0)
}
func TestPresentationProbeBufferActualExecPipes(t *testing.T) {
 for _,stream:=range []string{"stdout","stderr"} {t.Run(stream,func(t *testing.T){
  for _,size:=range []int{31,32} {t.Run(strconv.Itoa(size),func(t *testing.T){
   ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
   cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestPresentationProbeLimitChild$")
   cmd.Env=append(os.Environ(),"SIMPLENVR_PRESENTATION_LIMIT_CHILD="+stream,"SIMPLENVR_PRESENTATION_LIMIT_SIZE="+strconv.Itoa(size))
   output:=&eventPresentationProbeBuffer{limit:31}
   diagnostic:=&eventPresentationProbeBuffer{limit:31}
   cmd.Stdout=output;cmd.Stderr=diagnostic;cmd.WaitDelay=time.Second
   err:=cmd.Run()
   if ctx.Err()!=nil {t.Fatal("child did not finish within test budget",ctx.Err())}
   selected,other:=output,diagnostic;if stream=="stderr" {selected,other=diagnostic,output}
   if other.Len()!=0 || other.refused {t.Fatal("wrong output stream changed")}
   if size==31 {
    if err!=nil || selected.refused || selected.Len()!=31 {t.Fatal("exact cap refused",selected.Len(),err)}
   } else if !errors.Is(err,errEventVideoPresentation) || !selected.refused || selected.Len()>31 {t.Fatal("pipe copy bypassed cap",selected.Len(),selected.refused,err)}
  })}
 })}
}
