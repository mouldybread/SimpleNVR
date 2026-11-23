package main

import "math"

const (
 exportCopyEstimateFactor = 1.15
 exportCopyEstimateSlack = 8 << 20
 exportAccurateEstimateFactor = 4.0
)

// estimateExportBytes estimates the size of a prepared export from the source
// recordings it reads: each part contributes the share of its file that the export
// uses. Copy output is close to that share (remux overhead, audio overhang), so it
// gets a small margin. Re-encoded output can be several times larger than a low-bitrate
// camera stream, so Accurate gets a large one. A result of zero means the sizes are
// unknown and the caller reserves the per-job maximum. Callers clamp the result to the
// per-job maximum, and the job fails if its output exceeds what it reserved.
func estimateExportBytes(plan exportPlan,mode string) int64 {
 total:=0.0
 for _,part:=range plan.Parts {
  span:=part.Clip.End.Sub(part.Clip.Start)
  if span<=0 || part.Clip.Size<=0 || part.Out<=part.In {return 0}
  total+=float64(part.Clip.Size)*float64(part.Out-part.In)/float64(span)
 }
 if mode=="accurate" {total*=exportAccurateEstimateFactor} else {total=total*exportCopyEstimateFactor+exportCopyEstimateSlack}
 if math.IsNaN(total) || total<1 || total>math.MaxInt64/2 {return 0}
 return int64(math.Ceil(total))
}
