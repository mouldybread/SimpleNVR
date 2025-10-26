package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Camera struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	RTSPUrl       string                `json:"rtsp_url"`
	RecordDir     string                `json:"record_dir"`
	IsRecording   bool                  `json:"is_recording"`
	RetentionDays int                   `json:"retention_days"`
	AddedAt       time.Time             `json:"added_at"`
	recordCancel  context.CancelFunc
}

type NVR struct {
	cameras     map[string]*Camera
	cameraOrder []string
	mu          sync.RWMutex
}

var nvr = &NVR{
	cameras:     make(map[string]*Camera),
	cameraOrder: []string{},
}

const configFile = "/recordings/cameras.json"

var templates *template.Template

func (n *NVR) save() error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data := struct {
		Cameras     map[string]*Camera `json:"cameras"`
		CameraOrder []string           `json:"camera_order"`
	}{
		Cameras:     n.cameras,
		CameraOrder: n.cameraOrder,
	}

	file, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(data)
}

func (n *NVR) load() error {
	file, err := os.Open(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var data struct {
		Cameras     map[string]*Camera `json:"cameras"`
		CameraOrder []string           `json:"camera_order"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return err
	}

	n.mu.Lock()
	n.cameras = data.Cameras
	n.cameraOrder = data.CameraOrder
	n.mu.Unlock()
	return nil
}

func generateID(name string, t time.Time) string {
	b := make([]byte, 4)
	rand.Read(b)
	hash := hex.EncodeToString(b)
	return fmt.Sprintf("%s_%s", name, hash)
}

func main() {
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	if err := nvr.load(); err != nil {
		log.Printf("Warning: Failed to load cameras: %v", err)
	} else {
		log.Printf("Loaded %d cameras from disk", len(nvr.cameras))
	}

	autoStartRecordings()

	// API routes
	http.HandleFunc("/api/cameras", camerasHandler)
	http.HandleFunc("/api/cameras/add", addCameraHandler)
	http.HandleFunc("/api/cameras/delete", deleteCameraHandler)
	http.HandleFunc("/api/cameras/delete-all", deleteAllCamerasHandler)
	http.HandleFunc("/api/cameras/update-retention", updateRetentionHandler)
	http.HandleFunc("/api/cameras/update-retention-all", updateRetentionAllHandler)
	http.HandleFunc("/api/go2rtc/scan", scanGo2rtcHandler)
	http.HandleFunc("/api/record/start", startRecordingHandler)
	http.HandleFunc("/api/record/stop", stopRecordingHandler)
	http.HandleFunc("/api/stream/", streamProxyHandler)
	http.HandleFunc("/api/recordings/list", listRecordingsHandler)
	http.HandleFunc("/api/timeline/recordings", timelineRecordingsHandler)
	http.HandleFunc("/api/recordings/delete-all", deleteAllRecordingsHandler)
	http.HandleFunc("/api/download", handleDownload) // NEW: Download handler

	// Static files and recordings
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/recordings/", serveRecordingHandler)

	// Page handler
	http.HandleFunc("/", indexHandler)

	go cleanupRoutine()

	log.Println("SimpleNVR starting on :80")
	log.Fatal(http.ListenAndServe(":80", nil))
}

// Download handler - concatenates recordings and streams to client
func handleDownload(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")

	if cameraID == "" || startTimeStr == "" || endTimeStr == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		http.Error(w, "Invalid start_time", http.StatusBadRequest)
		return
	}

	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		http.Error(w, "Invalid end_time", http.StatusBadRequest)
		return
	}

	nvr.mu.RLock()
	camera, exists := nvr.cameras[cameraID]
	nvr.mu.RUnlock()

	if !exists {
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	// Find all recordings for this camera in the time range
	var recordingFiles []string
	pattern := filepath.Join(camera.RecordDir, "*.mp4")
	files, err := filepath.Glob(pattern)
	if err != nil {
		http.Error(w, "Failed to find recordings", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		filename := filepath.Base(file)
		if len(filename) < 15 {
			continue
		}

		// Parse timestamp from filename (format: YYYYMMDD_HHMMSS_CAMERANAME.mp4)
		timestampStr := filename[0:15]
		fileTime, err := time.Parse("20060102_150405", timestampStr)
		if err != nil {
			continue
		}

		// Check if file is within time range (with some buffer for segment boundaries)
		if fileTime.Before(endTime) && fileTime.Add(10*time.Minute).After(startTime) {
			recordingFiles = append(recordingFiles, file)
		}
	}

	if len(recordingFiles) == 0 {
		http.Error(w, "No recordings found in time range", http.StatusNotFound)
		return
	}

	// Sort files by timestamp
	sort.Strings(recordingFiles)

	log.Printf("Download request for camera %s: %d files in range %s to %s",
		camera.Name, len(recordingFiles), startTime.Format("15:04:05"), endTime.Format("15:04:05"))

	// Create concat file list for FFmpeg
	tmpFile, err := os.CreateTemp("", "concat-*.txt")
	if err != nil {
		http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	for _, file := range recordingFiles {
		absPath, _ := filepath.Abs(file)
		fmt.Fprintf(tmpFile, "file '%s'\n", absPath)
	}
	tmpFile.Close()

	// Output filename
	outputFilename := fmt.Sprintf("%s_%s.mp4",
		strings.ReplaceAll(camera.Name, " ", "_"),
		startTime.Format("20060102_150405"))

	// Set headers for download
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", outputFilename))

	// Run FFmpeg to concatenate and stream directly to response
	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", tmpFile.Name(),
		"-c", "copy",
		"-f", "mp4",
		"-movflags", "frag_keyframe+empty_moov",
		"pipe:1",
	)

	cmd.Stdout = w
	cmd.Stderr = os.Stderr

	log.Printf("Starting FFmpeg concatenation for %s (%d files)", camera.Name, len(recordingFiles))

	if err := cmd.Run(); err != nil {
		log.Printf("FFmpeg error for camera %s: %v", camera.Name, err)
	} else {
		log.Printf("Download completed successfully for camera %s", camera.Name)
	}
}

// Auto-start recording for cameras that were recording before server restart
func autoStartRecordings() {
	nvr.mu.Lock()
	defer nvr.mu.Unlock()

	log.Println("Checking for cameras to auto-restart...")
	count := 0
	for _, cam := range nvr.cameras {
		if cam.IsRecording {
			log.Printf("Restarting recording for camera: %s (%s)", cam.Name, cam.ID)
			ctx, cancel := context.WithCancel(context.Background())
			cam.recordCancel = cancel
			go recordCamera(ctx, cam)
			count++
		}
	}
	log.Printf("Auto-started %d camera(s)", count)
}

// Delete all recordings from all cameras
func deleteAllRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	totalDeleted := 0
	totalSize := int64(0)

	for _, cam := range nvr.cameras {
		pattern := filepath.Join(cam.RecordDir, "*.mp4")
		files, err := filepath.Glob(pattern)
		if err != nil {
			log.Printf("Failed to list recordings for camera %s: %v", cam.Name, err)
			continue
		}

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			totalSize += info.Size()

			if err := os.Remove(file); err != nil {
				log.Printf("Failed to delete recording %s: %v", file, err)
			} else {
				totalDeleted++
			}
		}
	}

	sizeMB := float64(totalSize) / (1024 * 1024)
	log.Printf("Deleted all recordings: %d files (%.2f MB)", totalDeleted, sizeMB)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"deleted": totalDeleted,
		"size_mb": sizeMB,
	})
}

// Timeline API - returns recording availability data for timeline visualization
func timelineRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Timeline API called with date: %s", r.URL.Query().Get("date"))
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Printf("Timeline API: Invalid date format: %s", dateStr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	type TimelineSegment struct {
		CameraID   string    `json:"camera_id"`
		CameraName string    `json:"camera_name"`
		StartTime  time.Time `json:"start_time"`
		EndTime    time.Time `json:"end_time"`
		FilePath   string    `json:"file_path"`
		Size       int64     `json:"size"`
	}

	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	segments := []TimelineSegment{}
	for _, cam := range nvr.cameras {
		pattern := filepath.Join(cam.RecordDir, "*.mp4")
		files, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				continue
			}

			filename := filepath.Base(file)
			if len(filename) < 15 {
				continue
			}

			timeStr := filename[0:15]
			fileTime, err := time.Parse("20060102_150405", timeStr)
			if err != nil {
				continue
			}

			if fileTime.Year() != date.Year() || fileTime.YearDay() != date.YearDay() {
				continue
			}

			endTime := fileTime.Add(10 * time.Minute)
			relPath := filepath.Join(filepath.Base(cam.RecordDir), filename)

			segments = append(segments, TimelineSegment{
				CameraID:   cam.ID,
				CameraName: cam.Name,
				StartTime:  fileTime,
				EndTime:    endTime,
				FilePath:   "/recordings/" + relPath,
				Size:       info.Size(),
			})
		}
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})

	log.Printf("Timeline API returning %d segments", len(segments))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(segments)
}

func serveRecordingHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/recordings/"):]
	fullPath := filepath.Join("/recordings", path)
	log.Printf("Serving recording: %s", fullPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		log.Printf("Recording not found: %s - %v", fullPath, err)
		http.Error(w, "Recording not found", http.StatusNotFound)
		return
	}

	if info.IsDir() {
		log.Printf("Invalid recording path (directory): %s", fullPath)
		http.Error(w, "Invalid recording path", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache")
	log.Printf("Serving recording file: %s (size: %d bytes)", fullPath, info.Size())
	http.ServeFile(w, r, fullPath)
}

func streamProxyHandler(w http.ResponseWriter, r *http.Request) {
	cameraName := r.URL.Path[len("/api/stream/"):]
	if cameraName == "" {
		http.Error(w, "Camera name required", http.StatusBadRequest)
		return
	}

	go2rtcURL := fmt.Sprintf("http://go2rtc:1984/api/stream.mp4?src=%s", cameraName)
	log.Printf("Proxying stream for camera: %s", cameraName)

	proxyReq, err := http.NewRequest("GET", go2rtcURL, nil)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	proxyReq.Header = r.Header.Clone()
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("Failed to connect to go2rtc for camera %s: %v", cameraName, err)
		http.Error(w, "Failed to connect to stream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func listRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	cameraID := r.URL.Query().Get("camera_id")
	if cameraID == "" {
		http.Error(w, "Camera ID required", http.StatusBadRequest)
		return
	}

	nvr.mu.RLock()
	cam, exists := nvr.cameras[cameraID]
	nvr.mu.RUnlock()

	if !exists {
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	pattern := filepath.Join(cam.RecordDir, "*.mp4")
	files, err := filepath.Glob(pattern)
	if err != nil {
		http.Error(w, "Failed to list recordings", http.StatusInternalServerError)
		return
	}

	type Recording struct {
		Filename  string    `json:"filename"`
		Path      string    `json:"path"`
		Timestamp time.Time `json:"timestamp"`
		Size      int64     `json:"size"`
	}

	var recordings []Recording
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		filename := filepath.Base(file)
		relPath := filepath.Join(filepath.Base(cam.RecordDir), filename)

		recordings = append(recordings, Recording{
			Filename:  filename,
			Path:      "/recordings/" + relPath,
			Timestamp: info.ModTime(),
			Size:      info.Size(),
		})
	}

	sort.Slice(recordings, func(i, j int) bool {
		return recordings[i].Timestamp.After(recordings[j].Timestamp)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordings)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func camerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	type CameraView struct {
		Camera
		RetentionDisplay int
	}

	var cameras []CameraView
	for _, id := range nvr.cameraOrder {
		cam := nvr.cameras[id]
		retentionDays := cam.RetentionDays
		if retentionDays == 0 {
			retentionDays = 7
		}
		cameras = append(cameras, CameraView{
			Camera:           *cam,
			RetentionDisplay: retentionDays,
		})
	}

	if err := templates.ExecuteTemplate(w, "cameras.html", cameras); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

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
		AddedAt:       now,
	}

	nvr.mu.Lock()
	nvr.cameras[id] = cam
	nvr.cameraOrder = append(nvr.cameraOrder, id)
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	log.Printf("Added camera: %s (%s) with %d day retention", name, id, cam.RetentionDays)
	camerasHandler(w, r)
}

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

func deleteAllCamerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.Lock()
	count := len(nvr.cameras)
	for _, cam := range nvr.cameras {
		if cam.IsRecording && cam.recordCancel != nil {
			cam.recordCancel()
		}
		cam.IsRecording = false
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
		go recordCamera(ctx, cam)
	}
	nvr.mu.Unlock()

	if err := nvr.save(); err != nil {
		log.Printf("Failed to save cameras: %v", err)
	}

	camerasHandler(w, r)
}

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

func cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	log.Println("Cleanup routine started")
	for range ticker.C {
		nvr.mu.RLock()
		for _, cam := range nvr.cameras {
			retentionDays := cam.RetentionDays
			if retentionDays == 0 {
				retentionDays = 7
			}
			retention := time.Duration(retentionDays) * 24 * time.Hour
			cleanupOldRecordings(cam.RecordDir, retention)
		}
		nvr.mu.RUnlock()
	}
}

func cleanupOldRecordings(dir string, retention time.Duration) {
	files, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-retention)
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			size := float64(info.Size()) / (1024 * 1024)
			err := os.Remove(file)
			if err != nil {
				log.Printf("Failed to delete old recording %s: %v", file, err)
			} else {
				log.Printf("Cleaned up old recording: %s (%.2f MB, age: %.1f days)",
					filepath.Base(file), size, time.Since(info.ModTime()).Hours()/24)
			}
		}
	}
}
