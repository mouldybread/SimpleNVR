package main

import "testing"

func TestFrontendTimelineGeometry(t *testing.T) {
 for _, zone := range []string{"UTC", "Asia/Hong_Kong", "America/Chicago"} {
  t.Run(zone, func(t *testing.T) {
   runFrontendRegression(t, "tests/frontend/timeline-geometry.cjs", zone)
  })
 }
}
