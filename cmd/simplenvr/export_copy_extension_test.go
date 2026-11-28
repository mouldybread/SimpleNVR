package main

import (
 "errors"
 "strings"
 "testing"
 "time"
)

func TestCopyRangeExtensionLimit(t *testing.T) {
 base := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
 requested := exportPlan{Start:base, End:base.Add(time.Minute)}
 for _,tc := range []struct{name string; before, after time.Duration; edge string}{
  {"whole recording unchanged",0,0,""},
  {"within both limits",5*time.Second,10*time.Second,""},
  {"exactly both limits",30*time.Second,30*time.Second,""},
  {"start exceeds by nanosecond",30*time.Second+time.Nanosecond,0,"start"},
  {"end exceeds by nanosecond",0,30*time.Second+time.Nanosecond,"end"},
  {"rare IDR extends whole file",4*time.Minute,5*time.Minute,"start"},
 } {
  t.Run(tc.name,func(t *testing.T){
   actual := exportPlan{Start:requested.Start.Add(-tc.before),End:requested.End.Add(tc.after)}
   err := validateCopyExtension(requested,actual)
   if tc.edge == "" { if err != nil { t.Fatal(err) }; return }
   var response *exportPlanHTTPError
   if !errors.As(err,&response) || response.status != 422 || !strings.Contains(response.message,"safe "+tc.edge) { t.Fatalf("unexpected error: %v",err) }
  })
 }
}

func TestCopyRangeExtensionRejectsShortening(t *testing.T) {
 base := time.Unix(0,0)
 requested := exportPlan{Start:base,End:base.Add(time.Minute)}
 for _,actual := range []exportPlan{
  {Start:base.Add(time.Second),End:requested.End},
  {Start:base,End:requested.End.Add(-time.Second)},
 } {
  if !errors.Is(validateCopyExtension(requested,actual),errExportPlanInvalid) { t.Fatal("shortened selection accepted") }
 }
}
