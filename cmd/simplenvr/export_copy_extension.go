package main

import (
 "fmt"
 "time"
)

const maxCopyRangeExtension = 30 * time.Second

func validateCopyExtension(requested, actual exportPlan) error {
 if !requested.End.After(requested.Start) || !actual.End.After(actual.Start) || actual.Start.After(requested.Start) || actual.End.Before(requested.End) {
  return errExportPlanInvalid
 }
 for _,edge := range []struct{name string; extra time.Duration}{
  {"start", requested.Start.Sub(actual.Start)},
  {"end", actual.End.Sub(requested.End)},
 } {
  if edge.extra > maxCopyRangeExtension {
   return &exportPlanHTTPError{422, fmt.Sprintf("Nearest safe %s requires %s of extra footage, exceeding the 30-second per-end limit. Select a wider time range.", edge.name, edge.extra)}
  }
 }
 return nil
}
