package main

import (
 "encoding/json"
 "strconv"
)

func decodeCopyVideoConfig(raw []byte)(float64,int,error) {
 var data struct { Streams []struct {
  Codec string `json:"codec_name"`
  Start string `json:"start_time"`
  Length string `json:"nal_length_size"`
 } `json:"streams"` }
 if json.Unmarshal(raw,&data)!=nil || len(data.Streams)!=1 { return 0,0,errCopyUnsafeCut }
 stream:=data.Streams[0]
 if stream.Codec!="h264" { return 0,0,errPreparedMediaProfile }
 width,err:=strconv.Atoi(stream.Length)
 if err!=nil || (width!=1 && width!=2 && width!=4) { return 0,0,errCopyUnsafeCut }
 origin,err:=preparedMediaSeconds(stream.Start)
 if err!=nil { return 0,0,errCopyUnsafeCut }
 return origin,width,nil
}

func nalHasIDRWithLength(sample []byte,width int) bool {
 if width!=1 && width!=2 && width!=4 { return false }
 idr:=false
 for i:=0;i<len(sample); {
  if len(sample)-i<width { return false }
  var size uint64
  for _,b:=range sample[i:i+width] { size=(size<<8)|uint64(b) }
  i+=width
  if size==0 || size>uint64(len(sample)-i) { return false }
  header:=sample[i]
  if header&0x80!=0 { return false }
  if header&0x1f==5 { idr=true }
  i+=int(size)
 }
 return idr
}
