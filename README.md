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
```

### 2. Docker Compose Configuration

```yaml
networks:
  nvr_net:
    external: true

services:
  simplenvr:
    container_name: simplenvr
    image: golang:1.26-alpine
    command: |
      sh -c '
        apk add --no-cache ffmpeg &&
        go install [github.com/air-verse/air@latest](https://github.com/air-verse/air@latest) &&
        exec /go/bin/air
      '
    working_dir: /app
    restart: unless-stopped
    networks:
      nvr_net:
        ipv4_address: 192.168.1.100
    volumes:
      - /path/to/simplenvr/build:/app
      - /path/to/simplenvr/recordings:/recordings
      - /path/to/simplenvr/certs:/certs
```

### 3. Starting the Container

```bash
docker-compose up -d
```

The application will be available securely at `https://<YOUR_SERVER_IP>` (or `https://your-server-ip:8443` if port mapping).

---

## 🎮 Usage

### Unified Header Interface

The main interface features a unified header bar containing:

* **Simple NVR** title (left)
* **Camera Selector** dropdown with color badges
* **Date Picker** for selecting playback date
* **Speed Control** for playback speed adjustment (0.25x to 4x)
* **Inline Action Buttons** - Mute All, Fullscreen, and Download
* **Events Icon** (📋) for viewing the event log modal
* **Settings Icon** (⚙️) for camera management

### Adding Cameras

#### Method 1: Auto-discover from go2rtc

1. Click the ⚙️ **Settings** icon in the top right.
2. Enter your go2rtc URL (default: `http://go2rtc:1984`).
3. Click **Scan & Import Streams**.
4. All streams will be automatically added with assigned colors (duplicate URLs are automatically skipped).

#### Method 2: Manual Entry

1. Open Camera Management via ⚙️ **Settings**.
2. Under **Add Camera Manually**, enter the camera name and RTSP URL.
3. Click **Add Camera**.

### Organizing Cameras

#### Drag-and-Drop Reordering

1. Open Camera Management modal.
2. Hover over a camera card to locate the drag handle (6 dots icon on the left).
3. Click and drag the card into the desired order.
4. The arrangement automatically saves and applies across playback and grid views.

#### Color Identification

* Each camera automatically receives one of 16 distinct colors.
* Colors appear in dropdown selector badges, timeline recording segments, video labels, and event markers.

### Recording Management

* **Start/Stop Recording:** Toggle recording directly on individual camera cards in Camera Management.
* **Set Retention Period:** Set retention days per camera, or use **Set Retention** under Bulk Operations.
* **View Storage Usage:** Real-time disk space usage is calculated and displayed on each camera card.

### Event Management

#### Viewing & Filtering Events
1. Click the 📋 **Events** icon in the header.
2. Filter events by Camera, Event Type, or Date Range.
3. Use page navigation controls or change pagination density (10–100 events per page).

#### Event Timeline Visualization
* Events appear as circular emoji markers on the 24-hour timeline.
* Hover over any marker for an instant detail tooltip.
* Click any event marker to seek the video grid directly to that timestamp.

### Timeline Playback

#### Camera Selection & Navigation
1. Open the **Cameras** dropdown in the header and check the cameras you wish to view.
2. Select a date using the **Date** picker.
3. Click anywhere on the timeline canvas to seek all synchronized camera streams to that exact moment.

#### Timeline Zoom
* Hover over the timeline canvas and **scroll with your mouse wheel** to zoom in/out centered on your cursor.
* Zoom adjusts dynamically from full day (24h) down to 1-minute precision intervals.

#### Keyboard Shortcuts
* **Space:** Toggle Play/Pause across all active videos.
* **Left Arrow (←):** Jump 10 minutes backward.
* **Right Arrow (→):** Jump 10 minutes forward.

### Multi-Camera Viewing

Grid layout automatically adjusts for the number of selected cameras:

* **1 camera:** Full screen
* **2 cameras:** 2x1 grid
* **3-4 cameras:** 2x2 grid
* **5-6 cameras:** 3x2 grid
* **7-9 cameras:** 3x3 grid
* **10+ cameras:** 4x? grid

All cameras sync to the same timeline position. Video stream labels show the camera name with its assigned color. Individual video controls are hidden for a clean interface, and arrangement matches the configured order.

### Download Recordings

1. Click on the timeline near your desired recording section.
2. Click **Download** in the header.
3. Adjust the download window using **-10 min** or **+10 min** adjustments, or click **Reset**.
4. Select the included cameras and click **Download Merged Videos**.

### Viewing Live Streams

1. Open Camera Management (⚙️ icon).
2. Click **👁️ View Live** on any camera.
3. Stream opens in a modal window.
4. Click **✖ Close** or press **Esc** to exit.

### Removing Cameras

* Click **Remove** once to arm the button.
* Click **Remove** again within 3 seconds to confirm deletion.
* Use **Remove All Cameras** for bulk deletion (requires confirmation code).

### Bulk Operations

* **Set All Retention:** Update retention for all cameras at once.
* **Delete All Recordings:** Remove all recordings from all cameras (requires confirmation code).
* **Delete All Cameras:** Remove all cameras (requires confirmation code).
* **Clear All Events:** Remove all events from all cameras (requires confirmation code).
* **Cleanup Orphaned Recordings:** Remove leftover recordings from deleted cameras.

---

## 🏡 Home Assistant Integration

SimpleNVR ingests external events (AI detections, line crossings, motion sensors) via Home Assistant's native `rest_command`.

### 1. Configure the REST Command

Add the following to your Home Assistant `configuration.yaml` file *(adjust IP address and port to match your SimpleNVR instance)*:

```yaml
rest_command:
  simplenvr_trigger_event:
    url: "https://<YOUR_SERVER_IP>/api/events/trigger"
    method: POST
    verify_ssl: false  # Required if using auto-generated self-signed certificates
    content_type: "application/json"
    payload: >
      {
        "camera_name": "{{ camera_name }}",
        "label": "{{ label }}",
        "start_time": "{{ start_time }}",
        "duration_seconds": {{ duration_seconds }}
      }
```

### 2. Create an Event Toggle (Helper)

1. Go to **Settings** > **Devices & Services** > **Helpers**.
2. Click **+ Create Helper** and select **Toggle**.
3. Name it `Cam1 Event Toggle` (`input_boolean.cam1_event_toggle`).

### 3. Example Automation

This automation calculates event duration when the toggle turns off and posts the event details to SimpleNVR:

```yaml
alias: "NVR: Log Cam1 Event to SimpleNVR"
description: "Calculates event duration from the toggle and sends it to the NVR"
mode: single

trigger:
  - platform: state
    entity_id: input_boolean.cam1_event_toggle
    from: "on"
    to: "off"

action:
  - service: rest_command.simplenvr_trigger_event
    data:
      # MUST match the exact camera name configured inside SimpleNVR
      camera_name: "Cam1" 
      
      # Event label (e.g., Line Crossing, Motion Detected, Person Detected)
      label: "Line Crossing" 
      
      # ISO8601 UTC start time from state change
      start_time: "{{ trigger.from_state.last_updated.isoformat() }}"
      
      # Duration calculation in seconds
      duration_seconds: >
        {{ (as_timestamp(trigger.to_state.last_updated) - as_timestamp(trigger.from_state.last_updated)) | int(default=5) }}
```

---

## 🔒 SSL / TLS Certificates

SimpleNVR runs over HTTPS on port `443` inside the container.

* **Auto-Generated Self-Signed Certs:** If no certificates exist in `/certs`, SimpleNVR automatically generates self-signed `cert.pem` and `key.pem` files on first boot.
* **Custom Certificates:** To use custom or Let's Encrypt certificates, mount your own `cert.pem` and `key.pem` files inside `/certs/`.

---

## 📁 File Structure

### Recording Files
* **Location:** `/recordings/{camera_id}/`
* **Format:** `YYYYMMDD_HHMMSS_CameraName.mp4`
* **Segment Length:** 10 minutes per file
* **Files per day:** 144 per camera (24 hours × 6 segments/hour)

### Configuration & Certificates
* `cameras.json`: Camera definitions, retention, colors, and order (`/recordings/cameras.json`)
* `events.json`: Logged event markers (`/recordings/events.json`)
* `cert.pem` / `key.pem`: TLS certificates (`/certs/`)

### Application Files
* `main.go`: HTTP/HTTPS server, API handlers, camera management
* `recorder.go`: RTSP recording, MP4 muxing, cleanup routines
* `handlers.go`: HTTP request handlers, camera operations
* `playback.go`: Timeline data, recording queries
* `events.go`: Event management, storage, and API endpoints
* `cleanup.go`: Retention and orphan cleanup
* `templates/`: HTML templates (index, camera cards)
* `static/js/`: Modular JavaScript (playback, cameras, modals, core, recordings, events, toast)
* `static/style.css`: Application styling with responsive design

---

## 🔧 Technical Details

### Recording Pipeline

1. **RTSP Connection:** `gortsplib` client connects to camera RTSP stream.
2. **H.264 Decoding:** RTP packets depacketized to NAL units.
3. **SPS/PPS Extraction:** Codec parameters parsed from stream.
4. **MP4 Muxing:** `deepch/vdk` writes MP4 container to disk.
5. **Segmentation:** Rotates file every 10 minutes.
6. **Cleanup:** Old files deleted based on per-camera retention settings.

### Event System Architecture

* **Storage:** Local JSON file with event array (`/recordings/events.json`).
* **Structure:** Timestamp, camera name, label, duration in seconds.
* **Timeline Integration:** Events loaded via date-based API queries.
* **Canvas Rendering:** Drawn as circular markers with emoji icons.
* **Click Detection:** Euclidean distance calculation from cursor to event marker.
* **Tooltip System:** Dynamic DOM elements created/destroyed on hover.
* **Seek Integration:** Event click triggers video seek to exact event timestamp.

### Timeline System

* **Date-based Query:** API returns all recordings and events for selected date.
* **Visual Timeline:** Canvas-based 24-hour timeline showing recording availability.
* **Event Overlay:** Events rendered on top of recording segments.
* **Zoom Implementation:**
  * Mouse wheel event with `preventDefault`.
  * Dynamic zoom level (1.0 = full day, up to 24.0 = 1-hour view).
  * Cursor-centered zooming preserves context.
  * Adaptive time markers based on visible range.
  * Efficient rendering of only visible segments and events.
* **Color-coded Cameras:** 16 distinct colors automatically assigned.
* **Real-time Sync:** Multiple cameras play synchronized to selected time.
* **Segment Matching:** Finds appropriate recording segment based on timestamp.
* **Persistent Selection:** Camera selection stored in `localStorage`.

### Camera Order System

* **Drag-and-Drop:** HTML5 drag and drop with visual feedback.
* **Hover Highlighting:** Drag-over class shows valid drop zones.
* **Drag Leave Detection:** Highlight clears when dragging away from target.
* **Configuration Sync:** Order set in modal applies to playback grid.
* **Stable Colors:** Color assignments tied to camera ID, not position.
* **Label Synchronization:** Video labels update when order changes.
* **Persistent Storage:** Order saved to `cameras.json`.
* **Real-time Updates:** Changes immediately reflected in all views.

### Download System

* **FFmpeg Concatenation:** Uses concat demuxer to merge 10-minute segments.
* **Streaming Output:** Progressive download as FFmpeg processes.
* **Time Range Filtering:** Selects only recordings within specified time range.
* **Per-Camera Output:** Creates separate merged file for each selected camera.
* **Browser-friendly:** MP4 output with fragmented headers for streaming.

### API Endpoints

#### Camera Management
* `GET /api/cameras` - List all cameras (HTML partial with drag-drop markup)
* `POST /api/cameras/add` - Add new camera with color assignment
* `POST /api/cameras/delete` - Remove camera
* `POST /api/cameras/delete-all` - Remove all cameras
* `POST /api/cameras/update-retention` - Update single camera retention
* `POST /api/cameras/update-retention-all` - Update all cameras retention
* `POST /api/cameras/reorder` - Update camera display order

#### Recording Control
* `POST /api/record/start` - Start recording camera
* `POST /api/record/stop` - Stop recording camera

#### Playback & Timeline
* `GET /api/recordings/list?camera_id={id}` - List recordings (JSON)
* `GET /api/timeline/recordings?date=YYYY-MM-DD` - Get timeline data with color info
* `GET /recordings/{camera_id}/{filename}` - Serve recording file

#### Event Management
* `GET /api/events/list` - List all events (JSON)
* `POST /api/events/add` - Add new event
* `POST /api/events/clear-camera` - Clear events for specific camera
* `POST /api/events/clear-all` - Clear all events (requires confirmation)

#### Downloads
* `GET /api/download?camera_id={id}&start_time={iso8601}&end_time={iso8601}` - Download merged recording

#### go2rtc Integration
* `POST /api/go2rtc/scan` - Scan and import streams (skips duplicate URLs)

#### Live Streaming
* `GET /api/stream/{camera_name}` - Proxy live stream from go2rtc

#### Bulk Operations
* `POST /api/recordings/delete-all` - Delete all recordings from all cameras
* `POST /api/recordings/cleanup-orphaned` - Remove orphaned recordings

### Dependencies

#### Go Modules
* `github.com/bluenviron/gortsplib/v4`: RTSP client
* `github.com/bluenviron/mediacommon`: H.264 handling
* `github.com/deepch/vdk`: MP4 container muxing
* `github.com/pion/rtp`: RTP processing

#### System Dependencies
* **FFmpeg**: Video concatenation for downloads

#### Frontend
* **Vanilla JavaScript**: No framework dependencies
* **HTML5 Canvas**: Timeline rendering with zoom and events
* **HTML5 Drag and Drop**: Camera reordering
* **CSS Variables**: Theming and customization
* **localStorage**: Persistent camera selection

### Browser Compatibility

* **Chrome/Edge:** Full support with hardware acceleration
* **Firefox:** Full support with compatibility optimizations
* **Safari:** Full support (H.264 native)
* **Mobile:** Responsive design, touch-friendly

---

## 🎨 Customization

### Segment Duration
Edit `recorder.go`:
```go
segmentDuration := 10 * time.Minute  // Change to desired duration
```

### Default Retention
Edit `main.go` in `addCameraHandler`:
```go
RetentionDays: 7,  // Change default days
```

### Port / TLS Port
Edit `main.go` (and `docker-compose.yml` if mapping ports):
```go
http.ListenAndServeTLS(":443", certPath, keyPath, nil)
```

### Timeline Chunk Size
Edit `playback.js`:
```javascript
const CHUNK_SIZE_MS = 10 * 60 * 1000;  // 10 minutes in milliseconds
```

### Camera Colors
Edit `playback.js`:
```javascript
const COLORS = [
    '#3b82f6',  // Blue
    '#10b981',  // Green
    // ... add or modify colors
];
```

### Event Icons
Edit `playback.js`:
```javascript
const EVENT_ICONS = {
    'Motion Detected': '👁️',
    'Person Detected': '👤',
    // ... add or modify event types and icons
};
```

### Timeline Zoom Limits
Edit `playback.js` in `setupTimelineZoom`:
```javascript
const newZoomLevel = Math.max(1.0, Math.min(24.0, timelineZoomLevel * zoomDelta));
// 1.0 = full day minimum, 24.0 = 1-hour maximum
```

### Video Controls
Edit `playback.js` in `updateVideoGrid`:
```javascript
video.controls = false;  // Set to true to show individual controls
```

---

## 🔍 Monitoring & Logs

### Logs
```bash
docker logs -f simplenvr
```

### Common Log Messages
* `Starting recording for {camera}` - Recording initiated
* `Completed 10-minute segment for {camera}` - Segment saved
* `Recording error for {camera}` - Connection issue, retrying
* `Cleaned up old recording` - Retention cleanup executed
* `Download request for camera {name}: {count} files` - Download initiated
* `Download completed successfully for camera {name}` - Download finished
* `Camera order updated` - Drag-drop reorder saved
* `Event added: {type} for {camera}` - New event logged
* `Skipping camera {name} - URL already exists` - go2rtc scan duplicate prevention
* `Found existing TLS certificates in /certs.` - HTTPS certs loaded
* `No TLS certificates found. Auto-generating self-signed TLS certificate...` - Cert generation trigger

---

## 🐛 Troubleshooting

### Recording Not Starting
* Check RTSP URL is correct.
* Verify camera is H.264 encoded.
* Ensure network connectivity to camera.
* Check logs for connection errors (`docker logs simplenvr`).

### Playback Issues
* Verify recordings exist in `/recordings/{camera_id}/`.
* Check browser console for errors.
* Ensure MP4 files are not corrupted.
* Disable browser extensions that might interfere with video playback.

### Timeline Not Showing Recordings
* Verify correct date is selected in the Date picker.
* Check that cameras are checked in the Cameras dropdown.
* Confirm recordings exist for that date in the filesystem.
* Check browser console for API errors.
* Try zooming out to full day view.

### Event Markers Not Appearing
* Verify events exist in `/recordings/events.json`.
* Check browser console for JavaScript errors.
* Confirm date matches event timestamps.
* Check that events are properly formatted in JSON.

### Timeline Zoom Not Working
* Ensure mouse cursor is over the timeline canvas element.
* Check browser console for JavaScript errors.
* Verify browser supports wheel events.

---

## 📊 Performance

### Resource Usage (per camera)
* **CPU:** ~2-5% per active recording
* **RAM:** ~50-100MB per camera
* **Disk I/O:** ~1-2 MB/s per camera (depends on stream bitrate)
* **Network:** Matches camera RTSP bitrate

### Recommended Hardware Sizing
* **10 cameras:** Standard hardware (4 cores, 8GB RAM)
* **25 cameras:** Mid-range server (8 cores, 16GB RAM)
* **50+ cameras:** Dedicated server (16+ cores, 32GB+ RAM)

### Storage Calculations
Example: 1080p camera @ 4 Mbps
* **Per hour:** ~1.8 GB
* **Per day:** ~43 GB
* **7-day retention:** ~301 GB
* **30-day retention:** ~1.29 TB

---

## 🔐 Security Considerations

### Network Isolation
* Runs on isolated Docker networks (`nvr_net`).
* Keeping cameras on a separate VLAN is highly recommended.
* go2rtc acts as the main RTSP gateway.

### Access Control & HTTPS
* **Native HTTPS:** Serves web interface and API natively over HTTPS on port 443.
* **VLAN/Firewalls:** Network isolation is strongly recommended since there is no built-in auth.

### Data Protection
* Recordings stored on local host filesystem.
* No cloud uploads (100% self-hosted).
* Container has limited access to recordings and certs directories.
* Events stored locally in JSON format.

---

## ⚠️ Disclaimer & Limitation of Liability

> **IMPORTANT NOTICE:** SimpleNVR is provided on an **"AS IS"** and **"AS AVAILABLE"** basis, without warranties or conditions of any kind, either express or implied.

### 1. No Guarantee of Performance or Recording
This software is strictly intended for personal, non-critical, or self-hosted experimental use and is **not certified for high-risk, critical surveillance, or life-safety applications**. The author(s) and contributor(s) make **no guarantees, representations, or warranties** that:
* The software will operate uninterrupted, continuously, or error-free.
* Video footage, motion events, audio, or metadata will be successfully captured, processed, stored, or retrieved.
* The software will survive hardware failures, network disruptions, power outages, software bugs, or unexpected system crashes.

### 2. User Responsibility & Testing
By using this software, you acknowledge and agree that you are solely responsible for:
* Independently testing, verifying, and monitoring the recording integrity and system health in your deployment environment.
* Maintaining appropriate backup, failover, and hardware redundancy protocols for critical footage.
* Ensuring compliance with local surveillance, privacy, and video retention laws.

### 3. Limitation of Liability
To the maximum extent permitted by applicable law, in no event shall the author(s), developer(s), or copyright holder(s) be liable for any claim, loss, damage, or liability whatsoever—whether direct, indirect, incidental, special, or consequential—arising out of or in connection with the software, including without limitation:
* Property damage, theft, burglary, robbery, or unrecorded security incidents.
* Data corruption, loss of video logs, or storage drive wear.
* Financial loss, business interruption, or legal claims resulting from software malfunction or failure to record.

**BY INSTALLING, DEPLOYING, OR USING THIS SOFTWARE, YOU ACKNOWLEDGE THAT YOU HAVE READ THIS DISCLAIMER, UNDERSTAND IT, AND AGREE TO ASSUME ALL ASSOCIATED RISKS.**

---

## 📄 License

[![License: CC BY-NC-SA 4.0](https://img.shields.io/badge/License-CC%20BY--NC--SA%204.0-lightgrey.svg)](https://creativecommons.org/licenses/by-nc-sa/4.0/)

This project is licensed under the **Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License (CC BY-NC-SA 4.0)**.

### Summary of Terms:
* **Attribution (BY):** Reusers must give appropriate credit, provide a link to the license, and indicate if changes were made.
* **NonCommercial (NC):** Reusers may not use the material for commercial purposes.
* **ShareAlike (SA):** Reusers who remix, transform, or build upon the material must distribute their contributions under the exact same license.

See the full [LICENSE](LICENSE) file for complete details.

---

## 🤝 Contributing

For issues, feature requests, or questions, please open an issue in the repository.

---

## 📞 Support

* **Review logs:** `docker logs simplenvr`
* **Check recording directory:** `/recordings/`
* **Check certs directory:** `/certs/`
* **Check event log:** `/recordings/events.json`
* **Verify go2rtc:** `http://go2rtc:1984`
* **Check configuration:** `/recordings/cameras.json`
