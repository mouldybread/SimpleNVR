package main

import (
 "bytes"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func testNVR() *NVR {
 return &NVR{cameras: map[string]*Camera{"legacy_1234": {ID: "legacy_1234", Name: "Front", RecordDir: "/recordings/legacy_1234", RecordingMode: ModeContinuous}}, cameraOrder: []string{"legacy_1234"}, events: []Event{}}
}

func TestConcurrentAtomicSaves(t *testing.T) {
 path := filepath.Join(t.TempDir(), "cameras.json")
 n := testNVR()
 if err := n.saveAt(path); err != nil { t.Fatal(err) }
 var wg sync.WaitGroup
 for i := 0; i < 16; i++ {
  wg.Add(1)
  go func() { defer wg.Done(); if err := n.saveAt(path); err != nil { t.Error(err) } }()
 }
 wg.Wait()
 for _, p := range []string{path, path+".bak"} {
  raw, err := os.ReadFile(p)
  if err != nil { t.Fatal(err) }
  if _, err := decodeState(raw); err != nil { t.Fatal(err) }
  info, err := os.Stat(p)
  if err != nil { t.Fatal(err) }
  if info.Mode().Perm() != 0600 { t.Fatalf("insecure config permissions: %v", info.Mode()) }
 }
}

func TestInvalidConfigNotOverwritten(t *testing.T) {
 path := filepath.Join(t.TempDir(), "cameras.json")
 raw := []byte("{broken")
 if err := os.WriteFile(path, raw, 0600); err != nil { t.Fatal(err) }
 n := testNVR()
 if err := n.loadAt(path); err == nil { t.Fatal("accepted corrupt configuration") }
 if len(n.cameras) != 1 { t.Fatal("failed load changed live state") }
 if err := n.saveAt(path); err == nil { t.Fatal("overwrote corrupt configuration") }
 got, err := os.ReadFile(path)
 if err != nil { t.Fatal(err) }
 if !bytes.Equal(got, raw) { t.Fatal("corrupt configuration modified") }
}

func TestMissingConfigWithBackupFailsClosed(t *testing.T) {
 path := filepath.Join(t.TempDir(), "cameras.json")
 if err := os.WriteFile(path+".bak", []byte("{}"), 0600); err != nil { t.Fatal(err) }
 if err := testNVR().loadAt(path); err == nil { t.Fatal("ignored existing backup") }
}

func TestRejectUnsafeCameraPaths(t *testing.T) {
 for _, pair := range [][2]string{{"../escape", "/escape"}, {"safe", "/etc"}, {"safe", "/recordings/safe/../other"}, {"", "/recordings"}} {
  if err := validateCameraPath(pair[0], pair[1]); err == nil { t.Errorf("accepted unsafe path: %v", pair) }
 }
}

func TestOpaqueIDs(t *testing.T) {
 seen := map[string]bool{}
 for i := 0; i < 100; i++ {
  id := generateID("../../private/name", time.Now())
  if !strings.HasPrefix(id, "id_") || len(id) != 35 || strings.ContainsAny(id, "/\\") || seen[id] { t.Fatalf("unsafe or duplicate ID: %q", id) }
  seen[id] = true
 }
}

func TestRejectNullAndTrailingJSON(t *testing.T) {
 for _, raw := range []string{"null", "{}", `{"cameras":{}} {}`, `{"cameras":{"bad":null}}`} {
  if _, err := decodeState([]byte(raw)); err == nil { t.Errorf("accepted invalid configuration: %s", raw) }
 }
}
