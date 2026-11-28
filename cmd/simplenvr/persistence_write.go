package main

import (
 "io"
 "os"
 "path/filepath"
)

type persistenceWriteOps struct {
 create func(string,string)(*os.File,error)
 write func(*os.File,[]byte)(int,error)
 fileSync func(*os.File)error
 fileClose func(*os.File)error
 rename func(string,string)error
 openDir func(string)(*os.File,error)
 dirSync func(*os.File)error
}

func realPersistenceWriteOps() persistenceWriteOps {
 return persistenceWriteOps{
  create:os.CreateTemp,
  write:func(f *os.File,b []byte)(int,error){return f.Write(b)},
  fileSync:func(f *os.File)error{return f.Sync()},
  fileClose:func(f *os.File)error{return f.Close()},
  rename:os.Rename,
  openDir:os.Open,
  dirSync:func(f *os.File)error{return f.Sync()},
 }
}

// Per-call operations permit deterministic failure tests without global hooks.
// A rename may already have taken effect when a subsequent operation fails.
func atomicWriteWithOps(path string,raw []byte,ops persistenceWriteOps) error {
 f,err:=ops.create(filepath.Dir(path),".simplenvr-config-*")
 if err!=nil {return err}
 defer os.Remove(f.Name())
 count,err:=ops.write(f,raw)
 if err==nil && count!=len(raw) {err=io.ErrShortWrite}
 if err!=nil {ops.fileClose(f);return err}
 if err:=ops.fileSync(f);err!=nil {ops.fileClose(f);return err}
 if err:=ops.fileClose(f);err!=nil {return err}
 if err:=ops.rename(f.Name(),path);err!=nil {return err}
 dir,err:=ops.openDir(filepath.Dir(path));if err!=nil {return err}
 defer dir.Close()
 return ops.dirSync(dir)
}
