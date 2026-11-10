package main

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "strings"
 "time"
)

const ffmpegStopGrace=8*time.Second
const recordingFinalizeGrace=15*time.Second

func interruptibleCommand(ctx context.Context,name string,grace time.Duration,args ...string) *exec.Cmd {
 cmd:=exec.CommandContext(ctx,name,args...)
 cmd.Cancel=func() error {
  if cmd.Process==nil {return os.ErrProcessDone}
  return cmd.Process.Signal(os.Interrupt)
 }
 cmd.WaitDelay=grace
 return cmd
}

func ffmpegCommand(ctx context.Context,args ...string) *exec.Cmd {
 return interruptibleCommand(ctx,"ffmpeg",ffmpegStopGrace,append([]string{"-nostdin"},args...)...)
}

func partialRecordingName(path string) string {
 if strings.HasSuffix(path,"_partial.mp4") {return path}
 return strings.TrimSuffix(path,".mp4")+"_partial.mp4"
}

// This confirms a video stream and at least one readable video packet, not
// completeness, duration accuracy, or the decodability of every frame.
func validateRecording(ctx context.Context,path string) error {
 if err:=probeRecording(ctx,path);err!=nil {return err}
 cmd:=exec.CommandContext(ctx,"ffprobe","-v","error","-read_intervals","%+#1","-select_streams","v:0","-show_entries","packet=size","-of","csv=p=0",path)
 cmd.WaitDelay=2*time.Second
 output,err:=cmd.Output()
 if err!=nil {return err}
 if strings.TrimSpace(string(output))=="" {return errors.New("recording has no readable video packet")}
 return nil
}

func publishRecording(ctx context.Context,temporary,final string) error {
 if _,err:=os.Lstat(final);err==nil {return os.ErrExist} else if !os.IsNotExist(err) {return err}
 info,err:=os.Lstat(temporary)
 if err!=nil {return err}
 if !info.Mode().IsRegular() || info.Size()==0 {return errors.New("recording is not a nonempty regular file")}
 if err:=validateRecording(ctx,temporary);err!=nil {return err}
 // Destinations use unique recording/event IDs. Never intentionally replace a file.
 if _,err:=os.Lstat(final);err==nil {return os.ErrExist} else if !os.IsNotExist(err) {return err}
 return os.Rename(temporary,final)
}

func removeEmptyRecording(path string) {
 if info,err:=os.Lstat(path);err==nil && info.Mode().IsRegular() && info.Size()==0 {os.Remove(path)}
}
