package main

import (
 "os"
 "os/exec"
 "testing"
)

func TestFrontendEventRendering(t *testing.T) {
 node,err:=exec.LookPath("node")
 if err!=nil {
  if os.Getenv("SIMPLENVR_REQUIRE_FFMPEG")=="1" {t.Fatal("CI requires Node.js for frontend security regressions")}
  t.Skip("Node.js unavailable; frontend security regressions not run")
 }
 output,err:=exec.Command(node,"tests/event-rendering.cjs").CombinedOutput()
 if err!=nil {t.Fatalf("frontend security regressions failed: %v\n%s",err,output)}
}
