package main

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "time"

 "golang.org/x/sys/unix"
)

const maxStampedNameLen=236
var errStampedDayDir=errors.New("recording day directory unavailable or not a directory")

func stampedLabelValid(label string) bool {
 if label=="" || len(label)>160 {return false}
 for _,r:=range label {if !(r>='a' && r<='z' || r>='A' && r<='Z' || r>='0' && r<='9' || r=='_') {return false}}
 return true
}
func stampedCeilSecond(t time.Time) time.Time {
 t=t.UTC()
 if t.Nanosecond()==0 {return t}
 return t.Truncate(time.Second).Add(time.Second)
}

// The start is truncated and the end rounded up to whole seconds, so the name never
// understates a recording. The day folder is the UTC day of the (truncated) start.
func stampedFinalPath(cameraDir string,start,end time.Time,label string,partial bool) (string,error) {
 if !filepath.IsAbs(cameraDir) || filepath.Clean(cameraDir)!=cameraDir || cameraDir==string(filepath.Separator) || !stampedLabelValid(label) {return "",errStampedName}
 start=start.UTC().Truncate(time.Second);end=stampedCeilSecond(end)
 if !end.After(start) || end.Sub(start)>maxStampedSpan {return "",errStampedName}
 name:=stampedFileName(start,end,label)
 if partial {name=partialRecordingName(name)}
 if len(name)>maxStampedNameLen {return "",errStampedName}
 return filepath.Join(cameraDir,stampedDayName(start),name),nil
}

// Creates <cameraDir>/<day> under the held media lease, so retention (which needs the
// same lease) cannot remove it between creation and publication. An existing
// symlink or non-directory is refused. The camera directory is synced after a create.
func ensureStampedDayDirLeased(ctx context.Context,lease *mediaMutationLease,cameraDir,day string) error {
 if !lease.held() || lease.gate!=recordingMediaOwnership {return errPublicationOwnership}
 if err:=ctx.Err();err!=nil {return err}
 if _,err:=time.Parse(stampDayLayout,day);err!=nil || len(day)!=8 {return errStampedDayDir}
 if !filepath.IsAbs(cameraDir) || filepath.Clean(cameraDir)!=cameraDir || cameraDir==string(filepath.Separator) {return errStampedDayDir}
 fd,err:=unix.Open(cameraDir,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return err}
 camera:=os.NewFile(uintptr(fd),"camera-directory")
 defer camera.Close()
 err=unix.Mkdirat(fd,day,0755)
 created:=err==nil
 if err!=nil && !errors.Is(err,unix.EEXIST) {return err}
 var stat unix.Stat_t
 if err:=unix.Fstatat(fd,day,&stat,unix.AT_SYMLINK_NOFOLLOW);err!=nil {return err}
 if stat.Mode&unix.S_IFMT!=unix.S_IFDIR {return errStampedDayDir}
 if created {if err:=camera.Sync();err!=nil {return err}}
 return ctx.Err()
}

// Returns the intended final path even on error; the caller must not assume the file
// exists unless the error is nil. A failed validation can leave an empty day folder.
func publishStampedLeasedWith(ctx context.Context,lease *mediaMutationLease,temporary,cameraDir string,start,end time.Time,label string,partial bool,validate func(context.Context,string) error) (string,error) {
 final,err:=stampedFinalPath(cameraDir,start,end,label,partial);if err!=nil {return "",err}
 if err:=ensureStampedDayDirLeased(ctx,lease,cameraDir,filepath.Base(filepath.Dir(final)));err!=nil {return final,err}
 ops:=publicationDurabilityOps{renameRecordingNoReplace,func(f *os.File) error {return f.Sync()},func(f *os.File) error {return f.Sync()}}
 return final,publishRecordingDurablyLeased(ctx,lease,temporary,final,validate,ops)
}
func publishStampedRecordingLeased(ctx context.Context,lease *mediaMutationLease,temporary,cameraDir string,start,end time.Time,label string,partial bool) (string,error) {
 return publishStampedLeasedWith(ctx,lease,temporary,cameraDir,start,end,label,partial,validateRecording)
}
