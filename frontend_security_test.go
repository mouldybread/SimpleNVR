package main

import (
 "context"
 "os"
 "os/exec"
 "testing"
 "time"
)

func runFrontendRegression(t *testing.T,script,zone string) {
 t.Helper()
 node,err:=exec.LookPath("node")
 if err!=nil {
  if os.Getenv("SIMPLENVR_REQUIRE_FFMPEG")=="1" {t.Fatal("CI requires Node.js for frontend regressions")}
  t.Skip("Node.js unavailable; frontend regressions not run")
 }
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,node,script)
 cmd.Env=append(os.Environ(),"TZ="+zone)
 output,err:=cmd.CombinedOutput()
 if err!=nil {t.Fatalf("frontend regressions failed (%s, %s): %v\n%s",script,zone,err,output)}
}

func TestFrontendEventRendering(t *testing.T) {runFrontendRegression(t,"tests/event-rendering.cjs","UTC")}

func TestFrontendEventNavigation(t *testing.T) {
 for _,zone:=range []string{"UTC","Asia/Hong_Kong","America/Chicago"} {
  t.Run(zone,func(t *testing.T){runFrontendRegression(t,"tests/event-navigation.cjs",zone)})
 }
}
