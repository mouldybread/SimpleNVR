package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPersistenceSaveReplacementRenameFailures(t *testing.T) {
	for _, stage := range []string{"backup", "primary"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "cameras.json")
			old := []byte("{\"cameras\":{},\"camera_order\":[],\"events\":[]}\n")
			older := []byte("older retained backup\n")
			if err := os.WriteFile(path, old, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".bak", older, 0600); err != nil {
				t.Fatal(err)
			}
			n := &NVR{cameras: map[string]*Camera{}, cameraOrder: []string{}, events: []Event{}}
			before := deletionSnapshot(n)
			injected := errors.New("injected " + stage + " replacement rename failure")
			var order []string
			err := n.saveAtWithWriter(path, func(target string, raw []byte) error {
				order = append(order, "write:"+target)
				ops := realPersistenceWriteOps()
				realRename := ops.rename
				ops.rename = func(from, to string) error {
					order = append(order, "rename:"+to)
					if filepath.Dir(from) != dir || to != target {
						t.Fatalf("unexpected replacement paths: %q -> %q (target %q)", from, to, target)
					}
					failTarget := path + ".bak"
					if stage == "primary" {
						failTarget = path
					}
					if to == failTarget {
						return injected
					}
					return realRename(from, to)
				}
				return atomicWriteWithOps(target, raw, ops)
			})
			if !errors.Is(err, injected) {
				t.Fatalf("rename failure lost: %v", err)
			}
			wantOrder := []string{"write:" + path + ".bak", "rename:" + path + ".bak"}
			if stage == "primary" {
				wantOrder = append(wantOrder, "write:"+path, "rename:"+path)
			}
			if !reflect.DeepEqual(order, wantOrder) {
				t.Fatalf("replacement order = %v, want %v", order, wantOrder)
			}
			if !reflect.DeepEqual(before, deletionSnapshot(n)) {
				t.Fatal("save changed live state")
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, old) {
				t.Fatalf("old primary not preserved: %q, %v", got, err)
			}
			wantBackup := older
			if stage == "primary" {
				wantBackup = old
			}
			got, err = os.ReadFile(path + ".bak")
			if err != nil || !bytes.Equal(got, wantBackup) {
				t.Fatalf("backup = %q, want %q: %v", got, wantBackup, err)
			}
			leftovers, err := filepath.Glob(filepath.Join(dir, ".simplenvr-config-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary files remain: %v, %v", leftovers, err)
			}
		})
	}
}
