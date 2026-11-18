package main

import (
 "fmt"
 "io"
 "os"

 "golang.org/x/sys/unix"
)

// O_PATH checks type without opening a FIFO or device for data. Following a
// symlink here preserves the existing regular-file symlink read policy.
func readPersistencePrimary(path string) ([]byte,error) {
 identityFD,err:=unix.Open(path,unix.O_PATH|unix.O_CLOEXEC,0)
 if err!=nil {return nil,&os.PathError{Op:"open",Path:path,Err:err}}
 defer unix.Close(identityFD)
 var before unix.Stat_t
 if err:=unix.Fstat(identityFD,&before);err!=nil {return nil,err}
 if before.Mode&unix.S_IFMT!=unix.S_IFREG {return nil,fmt.Errorf("configuration %s is not a regular file",path)}
 fd,err:=unix.Open(fmt.Sprintf("/proc/self/fd/%d",identityFD),unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC,0)
 if err!=nil {return nil,err}
 file:=os.NewFile(uintptr(fd),path)
 defer file.Close()
 var after unix.Stat_t
 if err:=unix.Fstat(fd,&after);err!=nil {return nil,err}
 if after.Mode&unix.S_IFMT!=unix.S_IFREG || before.Dev!=after.Dev || before.Ino!=after.Ino {return nil,fmt.Errorf("configuration %s identity changed",path)}
 return io.ReadAll(file)
}
