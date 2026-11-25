package main

import (
 "os"
 "path/filepath"
)

const orphanDayFolderLimit=400

// Only one calendar-day level is inspected. Every read, including repeated
// evidence checks, uses the SAME pass budget. Flat trees add no directory reads.
func(budget *orphanInspectionBudget)inspectDays(dir string,entries []os.DirEntry)(bool,error) {
 days:=0
 for _,entry:=range entries {
  if isStampedDayName(entry.Name()) {days++;if days>orphanDayFolderLimit {return true,errOrphanInspectionLimit}}
 }
 for _,entry:=range entries {
  if err:=budget.ctx.Err();err!=nil {return true,err}
  if !isStampedDayName(entry.Name()) {continue}
  path:=filepath.Join(dir,entry.Name())
  info,err:=os.Lstat(path);if err!=nil {return true,err}
  if !info.IsDir() || info.Mode()&os.ModeSymlink!=0 {return true,nil}
  children,err:=budget.readDir(path);if err!=nil {return true,err}
  for _,child:=range children {
   if err:=budget.ctx.Err();err!=nil {return true,err}
   stamp,err:=parseStampedName(child.Name())
   if err!=nil || !stamp.inDay(entry.Name()) {return true,nil}
   info,err:=os.Lstat(filepath.Join(path,child.Name()));if err!=nil {return true,err}
   if !info.Mode().IsRegular() {return true,nil}
  }
 }
 return false,nil
}

// true means abandon the rest of this camera. Root .mp4 handling and its exact
// flat-tree read counts remain in the original loop. Day members require valid
// names and matching start days. Rechecks narrow races but are not atomic: like
// the existing orphan path, this requires trusted, stable pathname ancestors.
func cleanupOrphanDays(dir string,entries []os.DirEntry,ops orphanCleanupIO,readDir func(string)([]os.DirEntry,error),preserve,stop func()bool,result *orphanCleanupResult) bool {
 days:=0
 for _,entry:=range entries {if isStampedDayName(entry.Name()) {days++}}
 if days==0 {return false}
 if days>orphanDayFolderLimit {result.LimitReached=true;result.Failures++;return true}
 if stop() {return true}
 rootID,err:=os.Lstat(dir)
 if err!=nil || !rootID.IsDir() || rootID.Mode()&os.ModeSymlink!=0 {result.Failures++;return true}
 for _,entry:=range entries {
  if !isStampedDayName(entry.Name()) {continue}
  if stop() {return true}
  day:=filepath.Join(dir,entry.Name())
  dayID,err:=os.Lstat(day)
  if err!=nil || !dayID.IsDir() || dayID.Mode()&os.ModeSymlink!=0 {result.Failures++;return true}
  unchanged:=func()bool {
   for path,want:=range map[string]os.FileInfo{dir:rootID,day:dayID} {
    got,err:=os.Lstat(path)
    if err!=nil || !got.IsDir() || got.Mode()&os.ModeSymlink!=0 || !os.SameFile(want,got) {result.Failures++;return false}
   }
   return true
  }
  if !unchanged() {return true}
  children,err:=readDir(day);if err!=nil {result.Failures++;return true}
  // Inspect the entire observed day before removing any member, even when an
  // injected inspect callback is weaker than the production evidence inspector.
  for _,child:=range children {
   stamp,err:=parseStampedName(child.Name())
   if err!=nil || !stamp.inDay(entry.Name()) {result.SkippedRecoveryDirs++;return true}
   info,err:=os.Lstat(filepath.Join(day,child.Name()))
   if err!=nil {result.Failures++;return true}
   if !info.Mode().IsRegular() {result.SkippedRecoveryDirs++;return true}
  }
  for _,child:=range children {
   if stop() {return true}
   file:=filepath.Join(day,child.Name())
   info,err:=os.Lstat(file);if err!=nil {result.Failures++;return true}
   if !info.Mode().IsRegular() {result.SkippedRecoveryDirs++;return true}
   if preserve() || stop() {return true}
   if !unchanged() {return true}
   fresh,err:=os.Lstat(file)
   if err!=nil || !sameMediaFile(info,fresh) {result.Failures++;return true}
   if err:=ops.remove(file);err!=nil {result.Failures++;return true}
   result.Count++;result.Bytes+=info.Size()
  }
  if stop() || preserve() {return true}
  if !unchanged() {return true}
  remaining,err:=readDir(day);if err!=nil {result.Failures++;return true}
  if len(remaining)!=0 {return true}
  if stop() || preserve() {return true}
  if !unchanged() {return true}
  // os.Remove on an observed directory is empty-only; never use RemoveAll.
  if err:=ops.remove(day);err!=nil {result.Failures++;return true}
 }
 return false
}
