# SimpleNVR

A lightweight, self-hosted Network Video Recorder (NVR) built in Go that uses FFmpeg for RTSP recording and MP4 output. SimpleNVR integrates with go2rtc for camera stream management and provides a web interface for monitoring, recording, and playback.

Built with go2rtc and Home Assistant in mind, this NVR assumes you already have adequate live viewing arrangements from your go2rtc headend to endpoints such as Home Assistant or TVs. It provides basic live stream preview for troubleshooting but focuses on recording and playback.

It ingests RTSP streams via FFmpeg, usually from a go2rtc server, and saves them to disk in 10-minute segments with configurable retention periods per camera (1-365 days).

It provides an intuitive timeline-based interface to replay recordings with multi-camera synchronized playback and download merged clips as needed. It runs from a Docker container and tries to be as unobtrusive as possible. It pairs well with TrueNAS Scale or similar storage solutions.

> **⚠️ Security Notice**  
> This app makes no attempt at access control or authentication. You are expected to use reverse proxies, VLANs, and firewall rules to achieve access control.

### Why SimpleNVR?

I really didn't want to build another app, but proprietary NVRs are maddening to use, and existing alternatives are often bloated and quirky. They try to do everything: intercoms, entry systems, alerting, AI object recognition, stream viewing, event orchestration—much of which duplicates tools you likely already run elsewhere.

SimpleNVR was built with a simple philosophy: an NVR should strictly ingest video, catalog and store it, and make it painless to review and export.

---

## 🎯 Features

### Core Functionality

* **MP4 Output** - Browser-compatible video files with H.264 codec
* **10-Minute Segments** - Automatic file splitting for reliability and management
* **Configurable Retention** - Per-camera retention periods (1-365 days)
* **Timeline Playback** - Interactive timeline with multi-camera synchronized playback
* **Event Timeline Integration** - Visual event markers with click-to-seek
* **Recording Downloads** - Export merged recordings with customizable time ranges
* **Live Stream Preview** - Real-time camera feeds through web interface
* **go2rtc Integration** - Auto-discover and import cameras (prevents duplicate imports by URL)
* **Drag-and-Drop Reordering** - Organize camera order with visual feedback
* **Toast Notifications** - Non-intrusive feedback for all operations
* **Per-Camera Storage Tracking** - View disk space consumed by each camera
* **Native HTTPS/TLS Support** - Auto-generates self-signed certificates in `/certs` on first boot, or loads custom certificates

### Event System

SimpleNVR includes a comprehensive event tracking and visualization system:

#### Event Types Supported
* Motion Detected (👁️)
* Person Detected (👤)
* Vehicle Detected (🚗)
* Intrusion (⚠️)
* Loitering (⏱️)
* Object Removed (📦)
* Object Left Behind (🎒)
* Sound Alert (🔔)
* Line Crossing (➡️)
* Tamper Alert (🚨)

#### Event Features
* **Timeline Visualization** - Events appear as interactive markers on playback timeline
* **Event Markers** - Distinct emoji icons for each event type with color-coded indicators
* **Interactive Markers** - Click any event marker to jump directly to recording at that moment
* **Event Tooltips** - Hover over markers to see event details (type, camera, time, duration)
* **Event Log Page** - Dedicated page with filterable, paginated event history
* **Event Filtering** - Filter events by camera, event type, and date range
* **Pagination Control** - Adjustable display (10-100 events per page)
* **Per-Camera Event Management** - Clear events for individual cameras
* **Bulk Event Operations** - Delete all events with confirmation code
* **Event Storage** - JSON-based storage in `/recordings/events.json`

#### Event Management
* Add events manually or import from external systems
* Events tied to specific cameras and timestamps
* Duration tracking for each event
* Automatic synchronization with timeline playback
* Event count display in camera management

### Web Interface

* **Unified Control Header** - All playback controls accessible from single header bar
* **Timeline View** - Visual representation of recording availability with 24-hour timeline
* **Timeline Zoom** - Mouse wheel zoom with adaptive time markers (hourly to 1-minute granularity)
* **Event Markers** - Visual indicators for camera events on timeline
* **Multi-Camera Playback** - View up to 16 cameras simultaneously in grid layout
* **Dropdown Camera Selector** - Multi-select dropdown with color-coded badges
* **Fullscreen Mode** - Immersive viewing with timeline controls
* **Download Manager** - Select cameras and time ranges for merged video downloads
* **Collapsible Sections** - Organized layout with state persistence
* **Responsive Design** - Desktop and mobile compatible with consistent spacing
* **Dark Mode Support** - System preference detection
* **Color-coded Identification** - 16 distinct colors for camera differentiation

### Playback Features

* **Interactive Timeline** - Click timeline to jump directly to specific times or event markers
* **Timeline Zoom** - Scroll to zoom in/out centered on cursor position
  * **Full day view:** Hourly markers
  * **4-12 hours:** 30-minute intervals
  * **2-4 hours:** 10-minute intervals with HH:MM labels
  * **<2 hours:** 1-minute precision with detailed markers
* **Event-based Navigation** - Click event markers to seek directly to events
* **Event Tooltips** - Rich event information on hover
* **Time Range Selection** - Easily navigate through recordings
* **Playback Controls** - Header controls for Speed (0.25x–4x), Mute All, Fullscreen, and Download
* **Keyboard Shortcuts** - Space for play/pause, Arrow keys (Left/Right) for 10-minute segment navigation
* **Multi-Camera Sync** - Synchronized playback across selected cameras
* **Fullscreen with Timeline** - Access timeline and controls in fullscreen mode
* **Persistent Camera Order** - Order set in configuration maintains in playback view
* **Clean Video Display** - Individual video controls hidden for streamlined interface

### Recording Features

* **Continuous Recording** - Start/stop recording per camera
* **Automatic Cleanup** - Old recordings deleted based on retention settings
* **Organized Storage** - Each camera gets unique directory with ID
* **Auto-reconnect** - Automatic retry on stream errors (5-second delay)
* **Recording Downloads** - Export concatenated recordings as single MP4 files
* **Storage Metrics** - Per-camera disk usage displayed in camera management

### Camera Management

* **Drag-and-Drop Ordering** - Reorder cameras with visual feedback and hover highlighting
* **Real-time Updates** - Camera list updates immediately across all views
* **Persistent Order** - Camera arrangement saved and synchronized
* **Color Assignment** - Automatic color assignment from 16-color palette
* **Bulk Operations** - Set retention, remove cameras, cleanup recordings, clear events
* **Duplicate Prevention** - go2rtc scan prevents importing duplicate RTSP URLs
* **Storage Display** - View disk space used by each camera's recordings

---

## 📋 Requirements

* Docker and Docker Compose
* go2rtc server (for camera stream management)
* RTSP-compatible cameras (H.264 video codec)
* FFmpeg (for video concatenation during downloads)

---

## 🚀 Installation

### 1. Directory Structure

```text
/path/to/SimpleNVR/
├── build/
│   ├── main.go
│   ├── recorder.go
│   ├── handlers.go
│   ├── playback.go
│   ├── events.go
│   ├── cleanup.go
│   ├── go.mod
│   ├── .air.toml
│   ├── templates/
│   │   ├── index.html
│   │   └── cameras.html
│   └── static/
│       ├── style.css
│       └── js/
│           ├── core.js
│           ├── cameras.js
│           ├── modals.js
│           ├── playback.js
│           ├── events.js
│           ├── toast.js
│           └── recordings.js
├── certs/
│   ├── cert.pem
│   └── key.pem
└── recordings/
    ├── cameras.json
    └── events.json
