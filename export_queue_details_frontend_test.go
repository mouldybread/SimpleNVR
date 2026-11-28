package main

import "testing"

func TestPreparedExportQueueDetailsUI(t *testing.T) {
 for _,zone:=range []string{"UTC","America/Chicago"} {
  t.Run(zone,func(t *testing.T){runFrontendRegression(t,"tests/frontend/export-queue-details.cjs",zone)})
 }
}
