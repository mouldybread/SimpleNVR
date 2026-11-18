package main

import "testing"

func TestFrontendTimelineCanvas(t *testing.T) {
 for _, zone := range []string{"UTC", "Asia/Hong_Kong", "America/Chicago"} {
  t.Run(zone, func(t *testing.T) {
   runFrontendRegression(t, "tests/timeline-canvas.cjs", zone)
  })
 }
}
