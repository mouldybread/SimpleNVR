package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// RecordingMode defines how a camera should record
type RecordingMode string

const (
	ModeContinuous  RecordingMode = "continuous"
	ModeEventBased  RecordingMode = "event_based"
	ModeNoRecording RecordingMode = "no_recording"
)

type Camera struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	RTSPUrl       string        `json:"rtsp_url"`
	RecordDir     string        `json:"record_dir"`
	IsRecording   bool          `json:"is_recording"`
	RetentionDays int           `json:"retention_days"`
	RecordingMode RecordingMode `json:"recording_mode"`
	AddedAt       time.Time     `json:"added_at"`
	recordCancel  context.CancelFunc
	bufferManager *BufferManager
}

type Event struct {
	ID         string    `json:"id"`
	CameraName string    `json:"camera_name"`
	Label      string    `json:"label"`
	StartTime  time.Time `json:"start_time"`
	Duration   int       `json:"duration_seconds"`
	CreatedAt  time.Time `json:"created_at"`
}

type NVR struct {
	cameras     map[string]*Camera
	cameraOrder []string
	events      []Event
	mu          sync.RWMutex
}

var nvr = &NVR{
	cameras:     make(map[string]*Camera),
	cameraOrder: []string{},
	events:      []Event{},
}

const configFile = "/recordings/cameras.json"

var templates *template.Template

func (n *NVR) save() error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	data := struct {
		Cameras     map[string]*Camera `json:"cameras"`
		CameraOrder []string           `json:"camera_order"`
		Events      []Event            `json:"events"`
	}{
		Cameras:     n.cameras,
		CameraOrder: n.cameraOrder,
		Events:      n.events,
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
		Events      []Event            `json:"events"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return err
	}

	n.mu.Lock()
	n.cameras = data.Cameras
	n.cameraOrder = data.CameraOrder
	if data.Events != nil {
		n.events = data.Events
	}
	n.mu.Unlock()
	return nil
}

func generateID(name string, t time.Time) string {
	b := make([]byte, 4)
	rand.Read(b)
	hash := hex.EncodeToString(b)
	return fmt.Sprintf("%s_%s", name, hash)
}

// Auto-start recording for cameras that were recording before server restart
func autoStartRecordings() {
	nvr.mu.Lock()
	defer nvr.mu.Unlock()

	log.Println("Checking for cameras to auto-restart...")
	count := 0

	for _, cam := range nvr.cameras {
		if cam.IsRecording {
			log.Printf("Restarting recording for camera: %s (%s) in %s mode", cam.Name, cam.ID, cam.RecordingMode)
			ctx, cancel := context.WithCancel(context.Background())
			cam.recordCancel = cancel

			if cam.RecordingMode == ModeEventBased {
				go recordCameraEventBased(ctx, cam)
			} else if cam.RecordingMode == ModeContinuous {
				go recordCamera(ctx, cam)
			}
			count++
		}
	}

	log.Printf("Auto-started %d camera(s)", count)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if err := templates.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	recoverPartialRecordings() // <-- ADDED THIS LINE

	// Camera management routes
	http.HandleFunc("/api/cameras", camerasHandler)
	http.HandleFunc("/api/cameras/add", addCameraHandler)
	http.HandleFunc("/api/cameras/delete", deleteCameraHandler)
	http.HandleFunc("/api/cameras/delete-all", deleteAllCamerasHandler)
	http.HandleFunc("/api/cameras/update-retention", updateRetentionHandler)
	http.HandleFunc("/api/cameras/update-retention-all", updateRetentionAllHandler)
	http.HandleFunc("/api/cameras/update-mode", updateRecordingModeHandler)
	http.HandleFunc("/api/go2rtc/scan", scanGo2rtcHandler)
	http.HandleFunc("/api/record/start", startRecordingHandler)
	http.HandleFunc("/api/record/stop", stopRecordingHandler)

	// Playback routes
	http.HandleFunc("/api/stream/", streamProxyHandler)
	http.HandleFunc("/api/recordings/list", listRecordingsHandler)
	http.HandleFunc("/api/timeline/recordings", timelineRecordingsHandler)
	http.HandleFunc("/api/recordings/delete-all", deleteAllRecordingsHandler)
	http.HandleFunc("/api/download", handleDownload)
	http.HandleFunc("/recordings/", serveRecordingHandler)
	http.HandleFunc("/api/cameras/reorder", reorderCamerasHandler)
	
	// Event routes
	http.HandleFunc("/api/events/trigger", triggerEventHandler)
	http.HandleFunc("/api/events/list", listEventsHandler)

	// Cleanup routes
	http.HandleFunc("/api/recordings/cleanup-orphaned", cleanupOrphanedRecordingsHandler)

	// Static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Page handler
	http.HandleFunc("/", indexHandler)

	// Start background cleanup routine
	go cleanupRoutine()

	log.Println("SimpleNVR starting on :80")
	log.Fatal(http.ListenAndServe(":80", nil))
}
