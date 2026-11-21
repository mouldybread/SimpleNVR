package main

import "testing"

func TestPreparedExportModeUI(t *testing.T) {
 for _,zone:=range []string{"UTC","America/Chicago"} {
  t.Run(zone,func(t *testing.T){runFrontendRegression(t,"tests/export-mode-ui.cjs",zone)})
 }
}
