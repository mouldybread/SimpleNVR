package main

import (
 "context"
 "crypto/rand"
 "crypto/rsa"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/hex"
 "encoding/pem"
 "html/template"
 "log"
 "math/big"
 "net"
 "net/http"
 "os"
 "os/signal"
 "path/filepath"
 "sync"
 "sync/atomic"
 "syscall"
 "time"
)

func ensureTLSCertificates(certPath,keyPath string) error {
 _,certErr:=os.Stat(certPath);_,keyErr:=os.Stat(keyPath)
 if certErr==nil && keyErr==nil {return nil}
 priv,err:=rsa.GenerateKey(rand.Reader,2048);if err!=nil {return err}
 now:=time.Now()
 serial,err:=rand.Int(rand.Reader,new(big.Int).Lsh(big.NewInt(1),128));if err!=nil {return err}
 cert:=x509.Certificate{SerialNumber:serial,Subject:pkix.Name{Organization:[]string{"SimpleNVR Self-Signed"},CommonName:"SimpleNVR"},NotBefore:now,NotAfter:now.Add(365*24*time.Hour),KeyUsage:x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature,ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},BasicConstraintsValid:true,IPAddresses:[]net.IP{net.ParseIP("127.0.0.1"),net.ParseIP("0.0.0.0")}}
 der,err:=x509.CreateCertificate(rand.Reader,&cert,&cert,&priv.PublicKey,priv);if err!=nil {return err}
 certOut,err:=os.Create(certPath);if err!=nil {return err}
 if err=pem.Encode(certOut,&pem.Block{Type:"CERTIFICATE",Bytes:der});err!=nil {certOut.Close();return err}
 if err=certOut.Close();err!=nil {return err}
 keyBytes,err:=x509.MarshalPKCS8PrivateKey(priv);if err!=nil {return err}
 keyOut,err:=os.OpenFile(keyPath,os.O_WRONLY|os.O_CREATE|os.O_TRUNC,0600);if err!=nil {return err}
 if err=pem.Encode(keyOut,&pem.Block{Type:"PRIVATE KEY",Bytes:keyBytes});err!=nil {keyOut.Close();return err}
 return keyOut.Close()
}

type RecordingMode string
const (ModeContinuous RecordingMode="continuous";ModeEventBased RecordingMode="event_based";ModeNoRecording RecordingMode="no_recording")

type Camera struct {
 ID string `json:"id"`
 Name string `json:"name"`
 RTSPUrl string `json:"rtsp_url"`
 RecordDir string `json:"record_dir"`
 IsRecording bool `json:"is_recording"`
 RetentionDays int `json:"retention_days"`
 RecordingMode RecordingMode `json:"recording_mode"`
 AddedAt time.Time `json:"added_at"`
 recordCancel context.CancelFunc
 recordWG sync.WaitGroup
 bufferManager *BufferManager
}

type Event struct {
 ID string `json:"id"`
 CameraName string `json:"camera_name"`
 Label string `json:"label"`
 StartTime time.Time `json:"start_time"`
 Duration int `json:"duration_seconds"`
 CreatedAt time.Time `json:"created_at"`
}

type NVR struct {
 cameras map[string]*Camera
 cameraOrder []string
 events []Event
 mu sync.RWMutex
 shuttingDown atomic.Bool
}

var nvr=&NVR{cameras:make(map[string]*Camera),cameraOrder:[]string{},events:[]Event{}}
const configFile="/recordings/cameras.json"
var templates *template.Template
func (n *NVR) save() error {return n.saveAt(configFile)}
func (n *NVR) load() error {return n.loadAt(configFile)}

func generateID(_ string,_ time.Time) string {
 b:=make([]byte,16)
 if _,err:=rand.Read(b);err!=nil {log.Panicf("cannot generate secure ID: %v",err)}
 return "id_"+hex.EncodeToString(b)
}

func autoStartRecordings() {
 if err:=nvr.resumeRecordings(runRecordingWorker);err!=nil {log.Printf("Recording restart: %v",err)}
}

func indexHandler(w http.ResponseWriter,r *http.Request) {
 if err:=templates.ExecuteTemplate(w,"index.html",nil);err!=nil {http.Error(w,err.Error(),500)}
}

func main() {
 if len(os.Args)>1 && os.Args[1]=="auth" {if err:=authCommand(os.Args[2:]);err!=nil {log.Fatal(err)};return}
 appCtx,stopSignals:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer stopSignals()
 auth,err:=newAuth(authFile());if err!=nil {log.Fatal(err)}
 templates,err=template.ParseGlob("templates/*.html");if err!=nil {log.Fatal("Failed to load templates: ",err)}
 if err=nvr.load();err!=nil {log.Fatal("Refusing startup with invalid camera configuration: ",err)}
 log.Printf("Loaded %d cameras from disk",len(nvr.cameras))
 recoverBeforeStartup(appCtx,nvr)
 if appCtx.Err()!=nil {return}
 if err=os.MkdirAll("/certs",0755);err!=nil {log.Fatal(err)}
 certPath,keyPath:=filepath.Join("/certs","cert.pem"),filepath.Join("/certs","key.pem")
 if err=ensureTLSCertificates(certPath,keyPath);err!=nil {log.Fatal("Failed to initialize TLS certificates: ",err)}
 if appCtx.Err()!=nil {return}
 http.HandleFunc("/api/cameras",camerasHandler)
 http.HandleFunc("/api/cameras/add",addCameraHandler)
 http.HandleFunc("/api/cameras/delete",deleteCameraHandler)
 http.HandleFunc("/api/cameras/delete-all",deleteAllCamerasHandler)
 http.HandleFunc("/api/cameras/update-retention",updateRetentionHandler)
 http.HandleFunc("/api/cameras/update-retention-all",updateRetentionAllHandler)
 http.HandleFunc("/api/cameras/update-mode",updateRecordingModeHandler)
 http.HandleFunc("/api/go2rtc/scan",scanGo2rtcHandler)
 http.HandleFunc("/api/record/start",startRecordingHandler)
 http.HandleFunc("/api/record/stop",stopRecordingHandler)
 http.HandleFunc("/api/stream/",streamProxyHandler)
 http.HandleFunc("/api/recordings/list",listRecordingsHandler)
 http.HandleFunc("/api/timeline/recordings",timelineRecordingsHandler)
 http.HandleFunc("/api/recordings/delete-all",deleteAllRecordingsHandler)
 http.HandleFunc("/api/download",handleDownload)
 http.HandleFunc("/recordings/",serveRecordingHandler)
 http.HandleFunc("/api/cameras/reorder",reorderCamerasHandler)
 http.HandleFunc("/api/events/trigger",triggerEventHandler)
 http.HandleFunc("/api/events/list",listEventsHandler)
 http.HandleFunc("/api/events/clear-all",clearAllEventsHandler)
 http.HandleFunc("/api/events/clear-camera",clearCameraEventsHandler)
 http.HandleFunc("/api/recordings/cleanup-orphaned",cleanupOrphanedRecordingsHandler)
 http.Handle("/static/",http.StripPrefix("/static/",http.FileServer(http.Dir("static"))))
 http.HandleFunc("/",indexHandler)
 gate:=newShutdownGate(auth.Handler(http.DefaultServeMux))
 server:=&http.Server{Addr:":443",Handler:gate,ReadHeaderTimeout:10*time.Second,ReadTimeout:30*time.Second,IdleTimeout:60*time.Second,MaxHeaderBytes:32768}
 autoStartRecordings()
 gate.startBackgroundJob(func(ctx context.Context){cleanupRoutine(ctx,nvr)})
 log.Println("SimpleNVR starting authenticated HTTPS server on :443...")
 if err=runAuthenticatedServer(appCtx,nvr,gate,server,certPath,keyPath);err!=nil {log.Fatal("Shutdown/server failure: ",err)}
 log.Println("Shutdown complete; recording intent persisted")
}
