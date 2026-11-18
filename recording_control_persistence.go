package main

import "errors"

var errRecordingPersistence=errors.New("recording state changed but could not be persisted")

// A failed mode replacement can still change mode and stopped intent. Persist
// those changes before returning the admission error. Keep controls serialized
// through the save, but never hold NVR.mu during persistence or filesystem I/O.
func (n *NVR) transitionRecordingAndSave(id,action string,mode RecordingMode,worker recordingWorker,check func(string) error,persist func() error) error {
 if persist==nil {return errRecordingPersistence}
 recordingControlMu.Lock();defer recordingControlMu.Unlock()
 n.mu.RLock()
 before:=n.cameras[id]
 var oldMode RecordingMode;var oldIntent bool
 if before!=nil {oldMode=before.RecordingMode;oldIntent=before.IsRecording}
 n.mu.RUnlock()
 transitionErr:=n.transitionRecordingLocked(id,action,mode,worker,check)
 n.mu.RLock()
 after:=n.cameras[id]
 changed:=before!=after || (after!=nil && (after.RecordingMode!=oldMode || after.IsRecording!=oldIntent))
 n.mu.RUnlock()
 if transitionErr==nil || changed {
  if err:=persist();err!=nil {return errors.Join(errRecordingPersistence,transitionErr,err)}
 }
 return transitionErr
}
