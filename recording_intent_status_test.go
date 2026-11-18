package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func TestCameraTemplateDescribesRecordingIntentNotHealth(t *testing.T) {
	tmpl, err := template.ParseFiles("templates/cameras.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		enabled bool
		wantStatus string
		wantAction string
		absentAction string
	}{
		{"enabled", true, "Recording enabled", "Stop Recording", "Start Recording"},
		{"stopped", false, "Recording stopped", "Start Recording", "Stop Recording"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := map[string]interface{}{
				"ID": "test", "Name": "Test camera", "RTSPUrl": "rtsp://example.invalid/live",
				"RecordDir": "/recordings/test", "DiskUsage": "0 B", "IsRecording": tc.enabled,
				"RecordingMode": ModeEventBased, "ModeDisplay": "Event Based", "RetentionDisplay": 7,
			}
			var out bytes.Buffer
			if err := tmpl.ExecuteTemplate(&out, "cameras.html", []map[string]interface{}{view}); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			for _, want := range []string{
				">" + tc.wantStatus + "</span>",
				">" + tc.wantAction + "</button>",
				"Recording state reflects intent, not camera connectivity or successful footage capture.",
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("rendered template missing %q", want)
				}
			}
			for _, absent := range []string{">Recording</span>", ">Idle</span>", ">" + tc.absentAction + "</button>"} {
				if strings.Contains(body, absent) {
					t.Fatalf("rendered template retained unsupported status or action %q", absent)
				}
			}
		})
	}
}
