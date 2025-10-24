package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Camera struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	RTSPUrl     string    `json:"rtsp_url"`
	RecordDir   string    `json:"record_dir"`
	IsRecording bool      `json:"is_recording"`
	AddedAt     time.Time `json:"added_at"`
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

func generateID(name string, t time.Time) string {
	b := make([]byte, 4)
	rand.Read(b)
	hash := hex.EncodeToString(b)
	return fmt.Sprintf("%s_%s", name, hash)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/cameras", camerasHandler)
	http.HandleFunc("/api/cameras/add", addCameraHandler)
	http.HandleFunc("/api/cameras/delete", deleteCameraHandler)
	http.HandleFunc("/api/go2rtc/scan", scanGo2rtcHandler)
	http.HandleFunc("/api/record/start", startRecordingHandler)
	http.HandleFunc("/api/record/stop", stopRecordingHandler)
	http.Handle("/recordings/", http.StripPrefix("/recordings/", http.FileServer(http.Dir("/recordings"))))

	// Start retention cleanup goroutine
	go cleanupRoutine()

	log.Println("SimpleNVR starting on :80")
	log.Fatal(http.ListenAndServe(":80", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>SimpleNVR</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
    <style>
        body { font-family: sans-serif; margin: 20px; background: #1a1a1a; color: #fff; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #4CAF50; }
        .section { background: #2a2a2a; padding: 20px; margin: 20px 0; border-radius: 8px; }
        input, button { padding: 10px; margin: 5px; border-radius: 4px; border: 1px solid #444; }
        input { background: #333; color: #fff; width: 300px; }
        button { background: #4CAF50; color: white; cursor: pointer; border: none; }
        button:hover { background: #45a049; }
        .camera-card { background: #333; padding: 15px; margin: 10px 0; border-radius: 6px; }
        .camera-name { font-size: 18px; font-weight: bold; color: #4CAF50; }
        .recording { color: #f44336; }
        .btn-danger { background: #f44336; }
        .btn-danger:hover { background: #da190b; }
    </style>
</head>
<body>
<div class="container">
    <h1>SimpleNVR</h1>
    
    <div class="section">
        <h2>Scan go2rtc Server</h2>
        <input type="text" id="go2rtc-url" placeholder="http://go2rtc:1984" value="http://go2rtc:1984">
        <button hx-post="/api/go2rtc/scan" 
                hx-include="#go2rtc-url" 
                hx-target="#camera-list" 
                hx-swap="innerHTML">
            Scan go2rtc
        </button>
    </div>

    <div class="section">
        <h2>Add Camera Manually</h2>
        <input type="text" id="camera-name" placeholder="Camera Name" required>
        <input type="text" id="rtsp-url" placeholder="rtsp://camera:554/stream" required>
        <button hx-post="/api/cameras/add" 
                hx-include="#camera-name,#rtsp-url" 
                hx-target="#camera-list" 
                hx-swap="innerHTML">
            Add Camera
        </button>
    </div>

    <div class="section">
        <h2>Cameras</h2>
        <div id="camera-list" hx-get="/api/cameras" hx-trigger="load, every 5s" hx-swap="innerHTML">
            Loading...
        </div>
    </div>
</div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, tmpl)
}

func camerasHandler(w http.ResponseWriter, r *http.Request) {
	nvr.mu.RLock()
	defer nvr.mu.RUnlock()

	html := ""
	for _, id := range nvr.cameraOrder {
		cam := nvr.cameras[id]
		status := "Idle"
		if cam.IsRecording {
			status = `<span class="recording">● Recording</span>`
		}
		html += fmt.Sprintf(`
		<div class="camera-card">
			<div class="camera-name">%s</div>
			<div>RTSP: %s</div>
			<div>Folder: %s</div>
			<div>Status: %s</div>
			<button hx-post="/api/record/start" hx-vals='{"id":"%s"}' hx-target="#camera-list">Start Recording</button>
			<button hx-post="/api/record/stop" hx-vals='{"id":"%s"}' hx-target="#camera-list">Stop Recording</button>
			<button class="btn-danger" hx-post="/api/cameras/delete" hx-vals='{"id":"%s"}' hx-target="#camera-list" hx-confirm="Delete this camera?">Delete</button>
		</div>`, cam.Name, cam.RTSPUrl, cam.RecordDir, status, cam.ID, cam.ID, cam.ID)
	}
	if html == "" {
		html = "<p>No cameras configured</p>"
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
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
		ID:        id,
		Name:      name,
		RTSPUrl:   rtspUrl,
		RecordDir: recordDir,
		AddedAt:   now,
	}

	nvr.mu.Lock()
	nvr.cameras[id] = cam
	nvr.cameraOrder = append(nvr.cameraOrder, id)
	nvr.mu.Unlock()

	log.Printf("Added camera: %s (%s)", name, id)
	camerasHandler(w, r)
}

func deleteCameraHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	json.NewDecoder(r.Body).Decode(&data)
	id := data["id"]

	nvr.mu.Lock()
	if cam, exists := nvr.cameras[id]; exists {
		if cam.IsRecording {
			cam.IsRecording = false
		}
		delete(nvr.cameras, id)
		
		// Remove from order slice
		for i, cid := range nvr.cameraOrder {
			if cid == id {
				nvr.cameraOrder = append(nvr.cameraOrder[:i], nvr.cameraOrder[i+1:]...)
				break
			}
		}
	}
	nvr.mu.Unlock()

	log.Printf("Deleted camera: %s", id)
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
			ID:        id,
			Name:      streamName,
			RTSPUrl:   rtspUrl,
			RecordDir: recordDir,
			AddedAt:   now,
		}

		nvr.mu.Lock()
		nvr.cameras[id] = cam
		nvr.cameraOrder = append(nvr.cameraOrder, id)
		nvr.mu.Unlock()

		log.Printf("Auto-added camera from go2rtc: %s", streamName)
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
		go recordCamera(cam)
	}
	nvr.mu.Unlock()

	camerasHandler(w, r)
}

func stopRecordingHandler(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	json.NewDecoder(r.Body).Decode(&data)
	id := data["id"]

	nvr.mu.Lock()
	if cam, exists := nvr.cameras[id]; exists {
		cam.IsRecording = false
	}
	nvr.mu.Unlock()

	camerasHandler(w, r)
}

func recordCamera(cam *Camera) {
	log.Printf("Starting recording for %s", cam.Name)

	// TODO: Implement native Go recording using gortsplib
	// For now, this is a placeholder that will be implemented
	// using native Go libraries to consume RTSP and write MP4

	for cam.IsRecording {
		log.Printf("Recording %s (native Go implementation pending)", cam.Name)
		time.Sleep(10 * time.Second)
	}

	log.Printf("Stopped recording for %s", cam.Name)
}

func cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		nvr.mu.RLock()
		for _, cam := range nvr.cameras {
			cleanupOldRecordings(cam.RecordDir, 60*time.Minute)
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
			os.Remove(file)
			log.Printf("Cleaned up old recording: %s", file)
		}
	}
}
