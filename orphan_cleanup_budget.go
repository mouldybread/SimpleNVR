package main

import (
 "context"
 "errors"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"

 "golang.org/x/sys/unix"
)

const orphanDirectoryEntryLimit=8192
const orphanPassEntryLimit=262144
const orphanPassReadLimit=4096
const orphanReadChunk=64
var errOrphanInspectionLimit=errors.New("orphan cleanup inspection limit reached")

type orphanDirectoryReader interface {ReadDir(int)([]os.DirEntry,error)}
type orphanInspectionBudget struct {
 ctx context.Context
 entries,reads int
 perDirectory,maxEntries,maxReads int
}
func newOrphanInspectionBudget(ctx context.Context) *orphanInspectionBudget {
 return &orphanInspectionBudget{ctx:ctx,perDirectory:orphanDirectoryEntryLimit,maxEntries:orphanPassEntryLimit,maxReads:orphanPassReadLimit}
}

// All limits count observations, including repeated evidence inspections. A
// partial list or failed close never supplies deletion/empty-directory evidence.
func (budget *orphanInspectionBudget) collect(reader orphanDirectoryReader,one bool) ([]os.DirEntry,error) {
 if budget==nil || budget.ctx==nil || reader==nil || budget.perDirectory<1 || budget.maxEntries<1 {return nil,errOrphanInspectionLimit}
 var entries []os.DirEntry
 for {
  if err:=budget.ctx.Err();err!=nil {return nil,err}
  count:=orphanReadChunk
  if one {count=1} else {
   if remaining:=budget.perDirectory-len(entries)+1;remaining<count {count=remaining}
   if remaining:=budget.maxEntries-budget.entries+1;remaining<count {count=remaining}
  }
  if count<1 {return nil,errOrphanInspectionLimit}
  batch,err:=reader.ReadDir(count)
  budget.entries+=len(batch)
  if len(batch)>count || budget.entries>budget.maxEntries || len(entries)+len(batch)>budget.perDirectory {return nil,errOrphanInspectionLimit}
  if cancelErr:=budget.ctx.Err();cancelErr!=nil {return nil,cancelErr}
  if err!=nil && err!=io.EOF {return nil,err}
  entries=append(entries,batch...)
  if one || err==io.EOF {
   if len(batch)==0 && err==nil {return nil,io.ErrNoProgress}
   sort.Slice(entries,func(i,j int)bool{return entries[i].Name()<entries[j].Name()})
   return entries,nil
  }
  if len(batch)==0 {return nil,io.ErrNoProgress}
 }
}

func (budget *orphanInspectionBudget) readDirectory(path string,one bool) (entries []os.DirEntry,resultErr error) {
 if budget==nil || budget.ctx==nil || budget.maxReads<1 || budget.reads>=budget.maxReads {return nil,errOrphanInspectionLimit}
 if err:=budget.ctx.Err();err!=nil {return nil,err}
 budget.reads++
 file,err:=os.OpenFile(path,os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,0)
 if err!=nil {return nil,err}
 defer func(){resultErr=errors.Join(resultErr,file.Close());if resultErr!=nil {entries=nil}}()
 return budget.collect(file,one)
}
func (budget *orphanInspectionBudget) readDir(path string) ([]os.DirEntry,error) {return budget.readDirectory(path,false)}

func (budget *orphanInspectionBudget) stagingPresent(path string) (bool,error) {
 if err:=budget.ctx.Err();err!=nil {return true,err}
 info,err:=os.Lstat(path)
 if os.IsNotExist(err) {return false,nil}
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 entries,err:=budget.readDirectory(path,true)
 return len(entries)>0 || err!=nil,err
}
func (budget *orphanInspectionBudget) inspect(dir string) (bool,error) {
 if err:=budget.ctx.Err();err!=nil {return true,err}
 info,err:=os.Lstat(dir)
 if err!=nil {return true,err}
 if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
 entries,err:=budget.readDir(dir)
 if err!=nil {return true,err}
 for _,entry:=range entries {if strings.HasSuffix(entry.Name(),".tmp") {return true,nil}}
 for _,name:=range []string{"temp","buffer"} {
  retained,err:=budget.stagingPresent(filepath.Join(dir,name))
  if retained || err!=nil {return true,err}
 }
 return false,nil
}
