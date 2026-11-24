package main

import "context"

// Export inspection admission is independent of recording selection and caches.
// Keep the existing two-inspection bound shared by trim and keyframe inspection.
var exportProbeSlots=make(chan struct{},2)

func acquireExportProbe(ctx context.Context) (func(),error) {
 if err:=ctx.Err();err!=nil {return nil,err}
 select {
 case exportProbeSlots<-struct{}{}:
  if err:=ctx.Err();err!=nil {<-exportProbeSlots;return nil,err}
  return func(){<-exportProbeSlots},nil
 case <-ctx.Done():return nil,ctx.Err()
 }
}
