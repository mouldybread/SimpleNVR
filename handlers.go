package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// camerasHandler - Render camera list template
func camerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	type CameraView struct {
		Camera
		RetentionDisplay int
		ModeDisplay      string
	}

	var cameras []CameraView
	for _, id := range nvr.cameraOrder {
		cam := nvr.cameras[id]
		retentionDays := cam.RetentionDays
		if retentionDays == 0 {
			retentionDays = 7
		}

		modeDisplay := "Continuous"
		if cam.RecordingMode == ModeEventBased {
			modeDisplay = "Event Based"
		} else if cam.RecordingMode == ModeNoRecording {
			modeDisplay = "No Recording"
		}

		cameras = append(cameras, CameraView{
			Camera:           *cam,
			RetentionDisplay: retentionDays,
			ModeDisplay:      modeDisplay,
		})
	}

	if err := templates.ExecuteTemplate(w, "cameras.html", cameras); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// addCameraHandler - Add a new camera manually
func addCameraHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("camera-name")
	rtspUrl := r.FormValue("rtsp-url")

	if name == "" || rtspUrl == "" {
		http.Error(w, "Name and RTSP URL required", http.StatusBadRequest)
		return
	}

	now := time.Now()
	id := generateID(name, now)
	recordDir := filepath.Join("/recordings", id)
	os.MkdirAll(recordDir, 0755)

	cam := &Camera{
		ID:            id,
		Name:          name,
		RTSPUrl:       rtspUrl,
		RecordDir:     recordDir,
		RetentionDays: 7,
		RecordingMode: ModeContinuous,
		AddedAt:       now,
	}

	nvr.mu.Lock()
	nvr.cameras[id] = cam
	nvr.cameraOrder = append(nvr.cameraOrder, id)
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	log.Printf("Added camera: %s (%s) with %d day retention in %s mode", name, id, cam.RetentionDays, cam.RecordingMode)
	camerasHandler(w, r)
}

// deleteCameraHandler - Delete a single camera
func deleteCameraHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	id := data["id"]

	nvr.mu.Lock()
	if cam, exists := nvr.cameras[id]; exists {
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
		}
		cam.IsRecording = false

		// Delete the camera's recording directory
		if err := os.RemoveAll(cam.RecordDir); err != nil {
			log.Printf("Failed to delete recording directory for camera %s: %v", cam.Name, err)
		} else {
			log.Printf("Deleted recording directory: %s", cam.RecordDir)
		}

		delete(nvr.cameras, id)
		for i, cid := range nvr.cameraOrder {
			if cid == id {
				nvr.cameraOrder = append(nvr.cameraOrder[:i], nvr.cameraOrder[i+1:]...)
				break
			}
		}
		log.Printf("Removed camera: %s (%s)", cam.Name, id)
	}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// deleteAllCamerasHandler - Delete all cameras
func deleteAllCamerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.Lock()
	count := len(nvr.cameras)
	for _, cam := range nvr.cameras {
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
		}
		cam.IsRecording = false

		// Delete the camera's recording directory
		if err := os.RemoveAll(cam.RecordDir); err != nil {
			log.Printf("Failed to delete recording directory for camera %s: %v", cam.Name, err)
		} else {
			log.Printf("Deleted recording directory: %s", cam.RecordDir)
		}
	}
	nvr.cameras = make(map[string]*Camera)
	nvr.cameraOrder = []string{}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	log.Printf("Removed all cameras (count: %d)", count)
	camerasHandler(w, r)
}

// updateRetentionHandler - Update retention for a single camera
func updateRetentionHandler(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ID            string `json:"id"`
		RetentionDays int    `json:"retention_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if data.RetentionDays < 1 {
		http.Error(w, "Retention days must be at least 1", http.StatusBadRequest)
		return
	}

	nvr.mu.Lock()
	cam, exists := nvr.cameras[data.ID]
	if exists {
		cam.RetentionDays = data.RetentionDays
		log.Printf("Updated retention for camera %s to %d days", cam.Name, cam.RetentionDays)
	}
	nvr.mu.Unlock()

	if !exists {
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// updateRetentionAllHandler - Update retention for all cameras
func updateRetentionAllHandler(w http.ResponseWriter, r *http.Request) {
	var data struct {
		RetentionDays int `json:"retention_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if data.RetentionDays < 1 {
		http.Error(w, "Retention days must be at least 1", http.StatusBadRequest)
		return
	}

	nvr.mu.Lock()
	count := 0
	for _, cam := range nvr.cameras {
		cam.RetentionDays = data.RetentionDays
		count++
	}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	log.Printf("Updated retention for all cameras (%d cameras) to %d days", count, data.RetentionDays)
	camerasHandler(w, r)
}

// updateRecordingModeHandler - Update camera recording mode
func updateRecordingModeHandler(w http.ResponseWriter, r *http.Request) {
	var data struct {
		ID   string        `json:"id"`
		Mode RecordingMode `json:"mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate mode
	if data.Mode != ModeContinuous && data.Mode != ModeEventBased && data.Mode != ModeNoRecording {
		http.Error(w, "Invalid mode. Must be: continuous, event_based, or no_recording", http.StatusBadRequest)
		return
	}

	nvr.mu.Lock()
	cam, exists := nvr.cameras[data.ID]
	if exists {
		oldMode := cam.RecordingMode
		cam.RecordingMode = data.Mode

		// Handle mode transitions
		if oldMode != data.Mode {
			// Stop existing recording
			if cam.IsRecording && cam.recordCancel != nil {
				cam.recordCancel()
				cam.IsRecording = false
			}

			// Start appropriate recording mode
			if data.Mode == ModeContinuous {
				cam.IsRecording = true
				ctx, cancel := context.WithCancel(context.Background())
				cam.recordCancel = cancel
				go recordCamera(ctx, cam)
			} else if data.Mode == ModeEventBased {
				cam.IsRecording = true
				ctx, cancel := context.WithCancel(context.Background())
				cam.recordCancel = cancel
				go recordCameraEventBased(ctx, cam)
			}
		}

		log.Printf("Updated recording mode for camera %s to %s", cam.Name, data.Mode)
	}
	nvr.mu.Unlock()

	if !exists {
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// scanGo2rtcHandler - Scan go2rtc for available streams and add them
func scanGo2rtcHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	go2rtcUrl := r.FormValue("go2rtc-url")
	if go2rtcUrl == "" {
		go2rtcUrl = "http://go2rtc:1984"
	}

	resp, err := http.Get(go2rtcUrl + "/api/streams")
	if err != nil {
		http.Error(w, "Failed to connect to go2rtc: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var streams map[string]interface{}
	json.Unmarshal(body, &streams)

	now := time.Now()
	for streamName := range streams {
		id := generateID(streamName, now)
		rtspUrl := fmt.Sprintf("rtsp://go2rtc:8554/%s", streamName)
		recordDir := filepath.Join("/recordings", id)
		os.MkdirAll(recordDir, 0755)

		cam := &Camera{
			ID:            id,
			Name:          streamName,
			RTSPUrl:       rtspUrl,
			RecordDir:     recordDir,
			RetentionDays: 7,
			RecordingMode: ModeContinuous,
			AddedAt:       now,
		}

		nvr.mu.Lock()
		nvr.cameras[id] = cam
		nvr.cameraOrder = append(nvr.cameraOrder, id)
		nvr.mu.Unlock()

		log.Printf("Auto-added camera from go2rtc: %s with %d day retention", streamName, cam.RetentionDays)
	}

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// startRecordingHandler - Start recording for a camera
func startRecordingHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	json.NewDecoder(r.Body).Decode(&data)
	id := data["id"]

	nvr.mu.Lock()
	cam, exists := nvr.cameras[id]
	if exists && !cam.IsRecording {
		cam.IsRecording = true
		ctx, cancel := context.WithCancel(context.Background())
		cam.recordCancel = cancel

		if cam.RecordingMode == ModeEventBased {
			go recordCameraEventBased(ctx, cam)
		} else if cam.RecordingMode == ModeContinuous {
			go recordCamera(ctx, cam)
		}
	}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// stopRecordingHandler - Stop recording for a camera
func stopRecordingHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	json.NewDecoder(r.Body).Decode(&data)
	id := data["id"]

	nvr.mu.Lock()
	if cam, exists := nvr.cameras[id]; exists {
		if cam.recordCancel != nil {
			cam.recordCancel()
		}
		cam.IsRecording = false
	}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// reorderCamerasHandler - Reorder cameras
func reorderCamerasHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		Order []string `json:"order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	nvr.mu.Lock()
	
	// Validate that all IDs exist
	for _, id := range data.Order {
		if _, exists := nvr.cameras[id]; !exists {
			nvr.mu.Unlock()
			http.Error(w, fmt.Sprintf("Camera ID %s not found", id), http.StatusBadRequest)
			return
		}
	}
	
	// Update camera order
	nvr.cameraOrder = data.Order
	nvr.mu.Unlock()

	// Save to disk
	if err := nvr.save(); err != nil {
		log.Printf("Failed to save camera order: %v", err)
		http.Error(w, "Failed to save order", http.StatusInternalServerError)
		return
	}

	log.Printf("Updated camera order: %v", data.Order)
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
