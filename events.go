package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"time"
)

// triggerEventHandler - External systems can POST events here
func triggerEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CameraName      string `json:"camera_name"`
		Label           string `json:"label"`
		StartTime       string `json:"start_time"` // RFC3339 format
		DurationSeconds int    `json:"duration_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.CameraName == "" || req.Label == "" || req.StartTime == "" || req.DurationSeconds <= 0 {
		http.Error(w, "Missing required fields: camera_name, label, start_time, duration_seconds", http.StatusBadRequest)
		return
	}

	// Parse start time in RFC3339 format (handles timezone properly)
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		http.Error(w, "Invalid start_time format. Use RFC3339 (e.g., 2025-10-26T14:30:00Z or 2025-10-26T14:30:00-05:00)", http.StatusBadRequest)
		return
	}

	// Find camera by go2rtc name (Camera.Name)
	nvr.mu.Lock()
	var targetCamera *Camera
	for _, cam := range nvr.cameras {
		if cam.Name == req.CameraName {
			targetCamera = cam
			break
		}
	}
	nvr.mu.Unlock()

	if targetCamera == nil {
		http.Error(w, "Camera not found: "+req.CameraName, http.StatusNotFound)
		return
	}

	// REMOVED: Recording mode check - events can be logged for any camera
	// Events are now universal logging, independent of recording mode

	// Create event
	event := Event{
		ID:         generateID("event", time.Now()),
		CameraName: req.CameraName,
		Label:      req.Label,
		StartTime:  startTime,
		Duration:   req.DurationSeconds,
		CreatedAt:  time.Now(),
	}

	nvr.mu.Lock()
	nvr.events = append(nvr.events, event)
	nvr.mu.Unlock()

	// Save configuration
	if err := nvr.save(); err != nil {
		log.Printf("Failed to save events: %v", err)
	}

	// Trigger recording ONLY if camera is in event-based mode AND has buffer manager
	if targetCamera.RecordingMode == ModeEventBased && targetCamera.bufferManager != nil {
		targetCamera.bufferManager.TriggerEvent(event)
		log.Printf("Event triggered recording for camera %s", targetCamera.Name)
	}

	log.Printf("Event logged: %s on camera %s at %s (duration: %ds)",
		event.Label, event.CameraName, startTime.Format(time.RFC3339), event.Duration)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"event_id": event.ID,
		"message":  "Event recorded successfully",
	})
}

// listEventsHandler - List all recorded events
func listEventsHandler(w http.ResponseWriter, r *http.Request) {
	cameraName := r.URL.Query().Get("camera_name")

	nvr.mu.RLock()
	events := make([]Event, 0)
	for _, e := range nvr.events {
		if cameraName == "" || e.CameraName == cameraName {
			events = append(events, e)
		}
	}
	nvr.mu.RUnlock()

	// Sort by start time descending (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.After(events[j].StartTime)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// clearAllEventsHandler - Clear all events
func clearAllEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nvr.mu.Lock()
	count := len(nvr.events)
	nvr.events = []Event{}
	nvr.mu.Unlock()

	// Save configuration
	if err := nvr.save(); err != nil {
		log.Printf("Failed to save events after clear: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to save",
		})
		return
	}

	log.Printf("Cleared all events (%d events deleted)", count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   count,
	})
}

// clearCameraEventsHandler - Clear events for a specific camera
func clearCameraEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CameraName string `json:"camera_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.CameraName == "" {
		http.Error(w, "camera_name required", http.StatusBadRequest)
		return
	}

	nvr.mu.Lock()
	var filteredEvents []Event
	deletedCount := 0
	for _, event := range nvr.events {
		if event.CameraName != req.CameraName {
			filteredEvents = append(filteredEvents, event)
		} else {
			deletedCount++
		}
	}
	nvr.events = filteredEvents
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save events after clearing camera events: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to save",
		})
		return
	}

	log.Printf("Cleared %d events for camera: %s", deletedCount, req.CameraName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   deletedCount,
	})
}
