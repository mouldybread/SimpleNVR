package main

import (
	"strings"
	"net/url"
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


// addCameraHandler - Add a new camera manually
func addCameraHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("camera-name")
	rtspUrl := r.FormValue("rtsp-url")

	if name == "" || rtspUrl == "" {
		http.Error(w, "Name and RTSP URL required", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
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
	var needsWait bool

	nvr.mu.Lock()
	cam, exists := nvr.cameras[id]
	if exists {
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
			needsWait = true
		}
	}
	nvr.mu.Unlock()

	// Wait outside the lock so we don't freeze the API
	if needsWait {
		cam.recordWG.Wait()
	}

	if exists {
		nvr.mu.Lock()
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
		nvr.mu.Unlock()
	}

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// deleteAllCamerasHandler - Delete all cameras
func deleteAllCamerasHandler(w http.ResponseWriter, r *http.Request) {
	var waitingCameras []*Camera

	nvr.mu.Lock()
	count := len(nvr.cameras)
	for _, cam := range nvr.cameras {
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
			waitingCameras = append(waitingCameras, cam)
		}
	}
	nvr.mu.Unlock()

	// Wait for all active FFmpeg processes to spin down gracefully
	for _, cam := range waitingCameras {
		cam.recordWG.Wait()
	}

	nvr.mu.Lock()
	for _, cam := range nvr.cameras {
		cam.IsRecording = false
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

	if data.Mode != ModeContinuous && data.Mode != ModeEventBased && data.Mode != ModeNoRecording {
		http.Error(w, "Invalid mode. Must be: continuous, event_based, or no_recording", http.StatusBadRequest)
		return
	}

	var needsWait bool
	var oldMode RecordingMode

	nvr.mu.Lock()
	cam, exists := nvr.cameras[data.ID]
	if !exists {
		nvr.mu.Unlock()
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	oldMode = cam.RecordingMode
	if oldMode != data.Mode {
		cam.RecordingMode = data.Mode
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
			needsWait = true
		}
	}
	nvr.mu.Unlock()

	// Wait outside the lock
	if needsWait {
		cam.recordWG.Wait()
	}

	if oldMode != data.Mode {
		nvr.mu.Lock()
		cam.IsRecording = false

		if data.Mode == ModeContinuous {
			cam.IsRecording = true
			ctx, cancel := context.WithCancel(context.Background())
			cam.recordCancel = cancel
			cam.recordWG.Add(1)
			go recordCamera(ctx, cam)
		} else if data.Mode == ModeEventBased {
			cam.IsRecording = true
			ctx, cancel := context.WithCancel(context.Background())
			cam.recordCancel = cancel
			cam.recordWG.Add(1)
			go recordCameraEventBased(ctx, cam)
		}
		nvr.mu.Unlock()

		log.Printf("Updated recording mode for camera %s to %s", cam.Name, data.Mode)
	}

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

// scanGo2rtcHandler - Scan go2rtc for available streams and add them
func scanGo2rtcHandler(w http.ResponseWriter, r *http.Request) {
	var go2rtcUrl string

	// Explicitly parse multipart forms if the content type matches
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			log.Printf("Failed to parse multipart form: %v", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			log.Printf("Failed to parse form: %v", err)
		}
	}

	// r.Form covers both query parameters and parsed form body parameters
	for _, key := range []string{"go2rtc-url", "go2rtc_url", "url", "go2rtcURL"} {
		if val := r.Form.Get(key); val != "" {
			go2rtcUrl = val
			break
		}
	}

	// Fallback to JSON body if form values were empty
	if go2rtcUrl == "" && r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			var data map[string]interface{}
			if json.Unmarshal(bodyBytes, &data) == nil {
				for _, key := range []string{"go2rtc-url", "go2rtc_url", "url", "go2rtcURL"} {
					if val, ok := data[key].(string); ok && val != "" {
						go2rtcUrl = val
						break
					}
				}
			}
		}
	}

	if go2rtcUrl == "" {
		go2rtcUrl = "http://go2rtc:1984"
	}

	// Clean up any trailing slashes to prevent malformed path endpoints
	go2rtcUrl = strings.TrimSuffix(go2rtcUrl, "/")

	// Strict Sanity Check: Ensure format is http(s)://address:port
	parsedUrl, err := url.Parse(go2rtcUrl)
	if err != nil || (parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https") || parsedUrl.Host == "" || parsedUrl.Port() == "" {
		log.Printf("Invalid go2rtc URL format rejected: %s", go2rtcUrl)
		http.Error(w, "Invalid format. Must be in the form http://websiteaddress:port", http.StatusBadRequest)
		return
	}

	log.Printf("Attempting to connect to go2rtc at: %s", go2rtcUrl)

	host := parsedUrl.Hostname()

	resp, err := http.Get(go2rtcUrl + "/api/streams")
	if err != nil {
		log.Printf("Failed to connect to go2rtc (%s): %v", go2rtcUrl, err)
		http.Error(w, "Failed to connect to go2rtc: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var streams map[string]interface{}
	json.Unmarshal(body, &streams)

	now := time.Now().UTC()
	added := 0
	skipped := 0

	for streamName := range streams {
		// Use the dynamically parsed host instead of hardcoding "go2rtc"
		rtspUrl := fmt.Sprintf("rtsp://%s:8554/%s", host, streamName)
		
		// Check if camera with this RTSP URL already exists
		nvr.mu.RLock()
		exists := false
		for _, cam := range nvr.cameras {
			if cam.RTSPUrl == rtspUrl {
				exists = true
				break
			}
		}
		nvr.mu.RUnlock()

		if exists {
			log.Printf("Skipping camera %s - URL already exists: %s", streamName, rtspUrl)
			skipped++
			continue
		}

		id := generateID(streamName, now)
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
		added++
	}

	log.Printf("Go2RTC scan complete: %d added, %d skipped", added, skipped)

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
		cam.recordWG.Add(1)

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
	var needsWait bool

	nvr.mu.Lock()
	cam, exists := nvr.cameras[id]
	if exists && cam.IsRecording && cam.recordCancel != nil {
		cam.recordCancel()
		needsWait = true
	}
	nvr.mu.Unlock()

	// Wait outside the lock
	if needsWait {
		cam.recordWG.Wait()
	}

	if exists {
		nvr.mu.Lock()
		cam.IsRecording = false
		nvr.mu.Unlock()
	}

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
// camerasHandler - Render camera list template
func camerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	type CameraView struct {
		Camera
		RetentionDisplay int
		ModeDisplay      string
		DiskUsage        string  // NEW: Disk space used
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

		// Calculate disk usage for this camera
		diskUsage := calculateDiskUsage(cam.RecordDir)

		cameras = append(cameras, CameraView{
			Camera:           *cam,
			RetentionDisplay: retentionDays,
			ModeDisplay:      modeDisplay,
			DiskUsage:        diskUsage,
		})
	}

	if err := templates.ExecuteTemplate(w, "cameras.html", cameras); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// calculateDiskUsage - Calculate total disk space used by a camera's recordings
func calculateDiskUsage(recordDir string) string {
	var totalSize int64

	// Walk through all files in the camera's recording directory
	err := filepath.Walk(recordDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() && filepath.Ext(path) == ".mp4" {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		return "0 B"
	}

	// Format the size in human-readable format
	return formatBytes(totalSize)
}

// formatBytes - Convert bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
