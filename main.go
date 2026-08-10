package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ensureTLSCertificates checks if cert.pem and key.pem exist in certPath/keyPath.
// If not, it automatically generates a self-signed certificate pair.
func ensureTLSCertificates(certPath, keyPath string) error {
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)

	// If both files already exist, we're good to go
	if certErr == nil && keyErr == nil {
		log.Println("Found existing TLS certificates in /certs.")
		return nil
	}

	log.Println("No TLS certificates found. Auto-generating self-signed TLS certificate...")

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year validity

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"SimpleNVR Self-Signed"},
			CommonName:   "SimpleNVR",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	// Write cert.pem
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	// Write key.pem
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}

	log.Printf("Self-signed TLS certificate generated successfully in %s", filepath.Dir(certPath))
	return nil
}

// RecordingMode defines how a camera should record
type RecordingMode string

const (
	ModeContinuous  RecordingMode = "continuous"
	ModeEventBased  RecordingMode = "event_based"
	ModeNoRecording RecordingMode = "no_recording"
)

type Camera struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	RTSPUrl       string             `json:"rtsp_url"`
	RecordDir     string             `json:"record_dir"`
	IsRecording   bool               `json:"is_recording"`
	RetentionDays int                `json:"retention_days"`
	RecordingMode RecordingMode      `json:"recording_mode"`
	AddedAt       time.Time          `json:"added_at"`
	recordCancel  context.CancelFunc
	recordWG      sync.WaitGroup
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

			cam.recordWG.Add(1)

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
	recoverPartialRecordings()

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
	http.HandleFunc("/api/events/clear-all", clearAllEventsHandler)
	http.HandleFunc("/api/events/clear-camera", clearCameraEventsHandler)

	// Cleanup routes
	http.HandleFunc("/api/recordings/cleanup-orphaned", cleanupOrphanedRecordingsHandler)

	// Static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Page handler
	http.HandleFunc("/", indexHandler)

	// Start background cleanup routine
	go cleanupRoutine()

	// Ensure /certs directory exists
	certDir := "/certs"
	if err := os.MkdirAll(certDir, 0755); err != nil {
		log.Fatalf("Failed to create certs directory: %v", err)
	}

	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")

	// Ensure certificates exist (auto-generates into /certs if missing)
	if err := ensureTLSCertificates(certPath, keyPath); err != nil {
		log.Fatalf("Failed to initialize TLS certificates: %v", err)
	}

	log.Println("SimpleNVR starting HTTPS server on :443...")
	if err := http.ListenAndServeTLS(":443", certPath, keyPath, nil); err != nil {
		log.Fatalf("Server failed to start over TLS: %v", err)
	}
}
