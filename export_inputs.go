package main

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "sync"
 "syscall"
)

const maxPinnedExportInputs=128
var errExportInputLimit=errors.New("export input limit exceeded")

type exportInputPool struct {
 slots chan struct{}
 limit int
 tempDir string
 open func(string) (*os.File,error)
}
type pinnedExport struct {
 files []*os.File
 identities []os.FileInfo
 manifest *os.File
 manifestName string
 release func()
 once sync.Once
 closeErr error
}
func openExportInput(path string) (*os.File,error) {return os.OpenFile(path,os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK,0)}
func newExportInputPool(parallel,limit int) *exportInputPool {
 if parallel<1 || limit<1 {panic("invalid export input limits")}
 return &exportInputPool{slots:make(chan struct{},parallel),limit:limit,open:openExportInput}
}
var exportInputPins=newExportInputPool(2,maxPinnedExportInputs)

// The preparation context bounds admission and pinning only. The successful
// caller owns Close and must retain the pins until Cmd.Run has returned.
func (pool *exportInputPool) prepare(ctx context.Context,clips []recordingInterval) (*pinnedExport,error) {
 if err:=ctx.Err();err!=nil {return nil,err}
 if len(clips)==0 || len(clips)>pool.limit {return nil,errExportInputLimit}
 select {case pool.slots<-struct{}{}:case <-ctx.Done():return nil,ctx.Err()}
 pins:=&pinnedExport{release:func(){<-pool.slots}}
 success:=false
 defer func(){if !success {pins.Close()}}()
 for _,clip:=range clips {
  if err:=ctx.Err();err!=nil {return nil,err}
  if clip.identity==nil {return nil,errRecordingChanged}
  file,err:=pool.open(clip.Path)
  if err!=nil {return nil,errRecordingChanged}
  pins.files=append(pins.files,file)
  info,err:=file.Stat()
  if err!=nil || !sameMediaFile(clip.identity,info) || info.Size()!=clip.Size || info.Size()==0 {return nil,errRecordingChanged}
  pins.identities=append(pins.identities,clip.identity)
 }
 if err:=ctx.Err();err!=nil {return nil,err}
 manifest,err:=os.CreateTemp(pool.tempDir,"simplenvr-export-*.ffconcat")
 if err!=nil {return nil,err}
 pins.manifest=manifest;pins.manifestName=manifest.Name()
 // The child receives this descriptor as fd 3; no named temp file survives.
 if err:=os.Remove(pins.manifestName);err!=nil {return nil,err}
 pins.manifestName=""
 if _,err:=io.WriteString(manifest,"ffconcat version 1.0\n");err!=nil {return nil,err}
 for index:=range pins.files {
  if err:=ctx.Err();err!=nil {return nil,err}
  if _,err:=fmt.Fprintf(manifest,"file '/proc/self/fd/%d'\n",index+4);err!=nil {return nil,err}
 }
 if _,err:=manifest.Seek(0,io.SeekStart);err!=nil {return nil,err}
 if err:=pins.validate(ctx);err!=nil {return nil,err}
 success=true
 return pins,nil
}
func (pins *pinnedExport) validate(ctx context.Context) error {
 for index,file:=range pins.files {
  if err:=ctx.Err();err!=nil {return err}
  info,err:=file.Stat()
  if err!=nil || !sameMediaFile(pins.identities[index],info) {return errRecordingChanged}
 }
 return ctx.Err()
}
func (pins *pinnedExport) command(ctx context.Context) (*exec.Cmd,error) {
 if err:=pins.validate(ctx);err!=nil {return nil,err}
 cmd:=ffmpegCommand(ctx,"-protocol_whitelist","file,pipe","-format_whitelist","concat,mov","-f","concat","-safe","0","-i","/proc/self/fd/3","-c","copy","-f","mp4","-movflags","frag_keyframe+empty_moov","pipe:1")
 cmd.ExtraFiles=append([]*os.File{pins.manifest},pins.files...)
 return cmd,nil
}
func (pins *pinnedExport) Close() error {
 pins.once.Do(func(){
  for _,file:=range pins.files {pins.closeErr=errors.Join(pins.closeErr,file.Close())}
  if pins.manifest!=nil {pins.closeErr=errors.Join(pins.closeErr,pins.manifest.Close())}
  if pins.manifestName!="" {if err:=os.Remove(pins.manifestName);err!=nil && !os.IsNotExist(err) {pins.closeErr=errors.Join(pins.closeErr,err)}}
  if pins.release!=nil {pins.release()}
 })
 return pins.closeErr
}
