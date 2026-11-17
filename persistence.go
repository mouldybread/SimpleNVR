package main

import (
 "bytes"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
)

var persistenceMu sync.Mutex

type persistedState struct {
 Cameras map[string]*Camera `json:"cameras"`
 CameraOrder []string `json:"camera_order"`
 Events []Event `json:"events"`
}

func validateCameraPath(id, dir string) error {
 if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") { return fmt.Errorf("unsafe camera ID %q", id) }
 expected := filepath.Join("/recordings", id)
 if dir != expected { return fmt.Errorf("camera %q has unsafe recording directory %q; expected %q", id, dir, expected) }
 info, err := os.Lstat(dir)
 if err != nil && !os.IsNotExist(err) { return err }
 if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) { return fmt.Errorf("camera directory %q is not a real directory", dir) }
 return nil
}

func decodeState(raw []byte) (persistedState, error) {
 var s persistedState
 dec := json.NewDecoder(bytes.NewReader(raw))
 if err := dec.Decode(&s); err != nil { return s, err }
 var extra interface{}
 if err := dec.Decode(&extra); err != io.EOF { return s, fmt.Errorf("configuration contains trailing data") }
 if s.Cameras == nil { return s, fmt.Errorf("configuration must contain a cameras object") }
 for id, cam := range s.Cameras {
  if cam == nil || cam.ID != id { return s, fmt.Errorf("invalid camera entry %q", id) }
  if err := validateCameraPath(id, cam.RecordDir); err != nil { return s, err }
  if _, err := cameraRetentionDuration(cam.RetentionDays); err != nil { return s, fmt.Errorf("invalid retention for camera %q: %w", id, err) }
  switch cam.RecordingMode {
  case ModeContinuous, ModeEventBased, ModeNoRecording:
  case "": cam.RecordingMode = ModeContinuous
  default: return s, fmt.Errorf("invalid recording mode for camera %q", id)
  }
  if cam.RecordingMode == ModeNoRecording { cam.IsRecording = false }
 }
 seen := make(map[string]bool)
 order := make([]string, 0, len(s.Cameras))
 for _, id := range s.CameraOrder {
  if _, ok := s.Cameras[id]; ok && !seen[id] { order = append(order, id); seen[id] = true }
 }
 missing := make([]string, 0)
 for id := range s.Cameras { if !seen[id] { missing = append(missing, id) } }
 sort.Strings(missing)
 s.CameraOrder = append(order, missing...)
 if s.Events == nil { s.Events = []Event{} }
 return s, nil
}

func atomicWrite(path string, raw []byte) error {
 return atomicWriteWithOps(path,raw,realPersistenceWriteOps())
}

func (n *NVR) saveAt(path string) error {
 return n.saveAtWithWriter(path,atomicWrite)
}

func (n *NVR) saveAtWithWriter(path string,write func(string,[]byte)error) error {
 persistenceMu.Lock()
 defer persistenceMu.Unlock()
 n.mu.RLock()
 raw, err := json.MarshalIndent(persistedState{n.cameras, n.cameraOrder, n.events}, "", "  ")
 n.mu.RUnlock()
 if err != nil { return err }
 raw = append(raw, '\n')
 if _, err = decodeState(raw); err != nil { return fmt.Errorf("refusing invalid configuration: %w", err) }
 old, err := readPersistencePrimary(path)
 if err == nil {
  if _, err = decodeState(old); err != nil { return fmt.Errorf("refusing to overwrite invalid existing configuration: %w", err) }
  if err = write(path+".bak", old); err != nil { return fmt.Errorf("backup configuration: %w", err) }
 } else {
  if !os.IsNotExist(err) { return err }
  // A failed read does not prove absence: preserve dangling primary entries.
  if _, entryErr := os.Lstat(path); entryErr == nil { return fmt.Errorf("refusing to replace unreadable configuration entry at %s; restore explicitly", path) } else if !os.IsNotExist(entryErr) { return entryErr }
  // Do not create a fresh primary over a missing-config recovery situation.
  if _, backupErr := os.Lstat(path+".bak"); backupErr == nil { return fmt.Errorf("refusing to create configuration while backup exists at %s.bak; restore explicitly", path) } else if !os.IsNotExist(backupErr) { return backupErr }
 }
 return write(path, raw)
}

func (n *NVR) loadAt(path string) error {
 persistenceMu.Lock()
 defer persistenceMu.Unlock()
 raw, err := readPersistencePrimary(path)
 if os.IsNotExist(err) {
  // ReadFile follows symlinks: ENOENT does not prove the config entry is absent.
  if _, entryErr := os.Lstat(path); entryErr == nil { return fmt.Errorf("configuration entry exists but cannot be read at %s; restore explicitly", path) } else if !os.IsNotExist(entryErr) { return entryErr }
  // Even a dangling backup symlink is existing recovery evidence, not absence.
  if _, backupErr := os.Lstat(path+".bak"); backupErr == nil { return fmt.Errorf("configuration missing but backup exists at %s.bak; restore explicitly", path) } else if !os.IsNotExist(backupErr) { return backupErr }
  return nil
 }
 if err != nil { return err }
 s, err := decodeState(raw)
 if err != nil { return fmt.Errorf("invalid configuration %s; backup if present: %s.bak: %w", path, path, err) }
 n.mu.Lock()
 n.cameras, n.cameraOrder, n.events = s.Cameras, s.CameraOrder, s.Events
 n.mu.Unlock()
 return nil
}
