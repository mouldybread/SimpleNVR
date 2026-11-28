package main

import (
 "errors"
 "fmt"
 "testing"
)

func TestCopyDeclaredNALLength(t *testing.T) {
 for _,width:=range []int{1,2,4} {
  raw:=[]byte(fmt.Sprintf(`{"streams":[{"codec_name":"h264","start_time":"0.125","nal_length_size":"%d"}]}`,width))
  origin,got,err:=decodeCopyVideoConfig(raw)
  if err!=nil || got!=width || origin!=0.125 { t.Fatalf("width %d: %v %d %v",width,origin,got,err) }
 }
 for _,value:=range []string{"","0","3","5","-1","garbage"} {
  raw:=[]byte(fmt.Sprintf(`{"streams":[{"codec_name":"h264","start_time":"0","nal_length_size":"%s"}]}`,value))
  if _,_,err:=decodeCopyVideoConfig(raw);!errors.Is(err,errCopyUnsafeCut) { t.Fatalf("accepted length %q: %v",value,err) }
 }
 for _,raw:=range []string{`{}`,`{"streams":[]}`,`{"streams":[{"codec_name":"h264","start_time":"0"}]}`,`{"streams":[{"codec_name":"h264","start_time":"N/A","nal_length_size":"4"}]}`} {
  if _,_,err:=decodeCopyVideoConfig([]byte(raw));err==nil { t.Fatal("invalid configuration accepted",raw) }
 }
}

func TestIDRWithDeclaredNALLengths(t *testing.T) {
 for _,width:=range []int{1,2,4} {
  sample:=func(headers ...byte)[]byte {
   var out []byte
   for _,h:=range headers {
    prefix:=make([]byte,width);prefix[width-1]=2
    out=append(out,prefix...);out=append(out,h,0x80)
   }
   return out
  }
  for _,tc:=range []struct{name string; data []byte; want bool}{
   {"IDR",sample(0x65),true},
   {"SEI and IDR",sample(0x06,0x65),true},
   {"non-IDR",sample(0x41),false},
   {"SEI and non-IDR",sample(0x06,0x41),false},
   {"forbidden bit",sample(0xe5),false},
   {"empty",nil,false},
   {"zero length",make([]byte,width),false},
   {"truncated payload",sample(0x65)[:width+1],false},
   {"truncated trailing length",append(sample(0x65),0),false},
  } {
   if got:=nalHasIDRWithLength(tc.data,width);got!=tc.want { t.Fatalf("width %d %s: got %v want %v",width,tc.name,got,tc.want) }
  }
 }
 for _,width:=range []int{0,3,5,-1} { if nalHasIDRWithLength([]byte{0,0,0,1,0x65},width) { t.Fatal("invalid width accepted",width) } }
}
