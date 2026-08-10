package main

import (
	"net/url"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// listRecordingsHandler - List all recordings for a specific camera
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

// timelineRecordingsHandler - Returns recording availability data for timeline visualization
func timelineRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Timeline API called with date: %s", r.URL.Query().Get("date"))

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Parse the requested date string as a local date in UTC+8 (Hong Kong)
	loc := time.FixedZone("HKT", 8*60*60)
	parsedDate, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		log.Printf("Timeline API: Invalid date format: %s", dateStr)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	// Define the local 24-hour window for that date (00:00:00 to 23:59:59 HKT)
	localStart := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, loc)
	localEnd := localStart.Add(24 * time.Hour)

	// Convert local boundaries to UTC for filename timestamp comparisons
	utcStart := localStart.UTC()
	utcEnd := localEnd.UTC()

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
			fileTime, err := time.ParseInLocation("20060102_150405", timeStr, time.UTC)
			if err != nil {
				continue
			}

			// Check if the file's UTC timestamp falls within the local day's UTC window
			if (fileTime.Equal(utcStart) || fileTime.After(utcStart)) && fileTime.Before(utcEnd) {
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
	}

	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})

	log.Printf("Timeline API returning %d segments for date %s", len(segments), dateStr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(segments)
}

// serveRecordingHandler - Serve individual recording files
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

// deleteAllRecordingsHandler - Delete all recordings from all cameras
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

// handleDownload - Concatenates recordings and streams to client
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
		fileTime, err := time.ParseInLocation("20060102_150405", timestampStr, time.UTC)
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

// streamProxyHandler - Proxy live streams from go2rtc
func streamProxyHandler(w http.ResponseWriter, r *http.Request) {
	cameraName := r.URL.Path[len("/api/stream/"):]
	if cameraName == "" {
		http.Error(w, "Camera name required", http.StatusBadRequest)
		return
	}

	// Find the camera to extract the correct hostname from its saved RTSP URL
	nvr.mu.RLock()
	var targetCam *Camera
	for _, cam := range nvr.cameras {
		if cam.Name == cameraName {
			targetCam = cam
			break
		}
	}
	nvr.mu.RUnlock()

	if targetCam == nil {
		http.Error(w, "Camera not found", http.StatusNotFound)
		return
	}

	// Dynamically reconstruct the proxy URL using the host from the RTSP string
	parsedUrl, err := url.Parse(targetCam.RTSPUrl)
	host := "go2rtc"
	if err == nil && parsedUrl.Hostname() != "" {
		host = parsedUrl.Hostname()
	}

	go2rtcURL := fmt.Sprintf("http://%s:1984/api/stream.mp4?src=%s", host, cameraName)
	log.Printf("Proxying stream for camera: %s from %s", cameraName, go2rtcURL)

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
