package main

import (
 "bytes"
 "io/fs"
 "os"
 "path/filepath"
 "regexp"
 "testing"
)

func TestNoLegacyDownloadCallsInAssets(t *testing.T) {
 legacy:=regexp.MustCompile(`\bfunction\s+startDownload\b|\bstartDownload\s*\(|=\s*startDownload\b|\b(?:window|globalThis)\s*(?:\.\s*startDownload\b|\[\s*['"]startDownload['"]\s*\])`)
 for _,root:=range []string{"static","templates"} {
  err:=filepath.WalkDir(root,func(path string,entry fs.DirEntry,walkErr error) error {
   if walkErr!=nil {return walkErr}
   if entry.IsDir() {return nil}
   extension:=filepath.Ext(path)
   if extension!=".js" && extension!=".html" {return nil}
   raw,err:=os.ReadFile(path);if err!=nil {return err}
   if legacy.Match(raw) {t.Errorf("legacy download definition/call/reference remains in %s",path)}
   return nil
  })
  if err!=nil {t.Fatal(err)}
 }
 playback,err:=os.ReadFile("static/js/playback.js");if err!=nil {t.Fatal(err)}
 if bytes.Contains(playback,[]byte("/api/download")) {t.Fatal("playback module must not retain a direct export-request path")}
 page,err:=os.ReadFile("templates/index.html");if err!=nil {t.Fatal(err)}
 if !bytes.Contains(page,[]byte(`onclick="startExportDownloads()"`)) {t.Fatal("monitored export entry point is not wired to the page")}
}

func TestFrontendDownloadEntryPoints(t *testing.T) {runFrontendRegression(t,"tests/download-entrypoints.cjs","UTC")}
