package main

import (
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// After the last camera is removed, deleteCameraHandler renders camerasHandler
// with an empty registry. The single-camera frontend guard accepts only a
// nonredirected HTTP 200 text/html fragment with no document-level tags, so the
// served empty state must satisfy that contract rather than look ambiguous.
func TestCameraListEmptyStateSatisfiesRemovalResponseContract(t *testing.T) {
	parsed, err := template.ParseGlob("templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	oldTemplates, oldNVR := templates, nvr
	templates = parsed
	nvr = &NVR{cameras: make(map[string]*Camera), cameraOrder: []string{}, events: []Event{}}
	t.Cleanup(func() { templates, nvr = oldTemplates, oldNVR })

	server := httptest.NewServer(http.HandlerFunc(camerasHandler))
	defer server.Close()
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/html" {
		t.Fatalf("Content-Type = %q (%v), want text/html", resp.Header.Get("Content-Type"), err)
	}
	html := string(body)
	if strings.TrimSpace(html) == "" {
		t.Fatal("empty camera-list fragment would be rejected by the frontend guard")
	}
	if regexp.MustCompile(`(?i)<(?:!doctype|html|head|body)\b`).MatchString(html) {
		t.Fatalf("empty state contains document-level markup: %q", html)
	}
	if !strings.Contains(html, "No cameras configured yet") || strings.Contains(html, "camera-box") {
		t.Fatalf("unexpected empty-state fragment: %q", html)
	}
}
