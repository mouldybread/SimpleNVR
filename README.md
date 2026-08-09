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

*   **MP4 Output** - Browser-compatible video files with H.264 codec
*   **10-Minute Segments** - Automatic file splitting for reliability and management
*   **Configurable Retention** - Per-camera retention periods (1-365 days)
*   **Timeline Playback** - Interactive timeline with multi-camera synchronized playback
*   **Event Timeline Integration** - Visual event markers with click-to-seek
*   **Recording Downloads** - Export merged recordings with customizable time ranges
*   **Live Stream Preview** - Real-time camera feeds through web interface
*   **go2rtc Integration** - Auto-discover and import cameras (prevents duplicate imports by URL)
*   **Drag-and-Drop Reordering** - Organize camera order with visual feedback
*   **Toast Notifications** - Non-intrusive feedback for all operations
*   **Per-Camera Storage Tracking** - View disk space consumed by each camera
*   **Native HTTPS/TLS Support** - Auto-generates self-signed certificates in `/certs` on first boot, or loads custom certificates

### Event System

SimpleNVR includes a comprehensive event tracking and visualization system:

#### Event Types Supported
*   Motion Detected (👁️)
*   Person Detected (👤)
*   Vehicle Detected (🚗)
*   Intrusion (⚠️)
*   Loitering (⏱️)
*   Object Removed (📦)
*   Object Left Behind (🎒)
*   Sound Alert (🔔)
*   Line Crossing (➡️)
*   Tamper Alert (🚨)

#### Event Features
*   **Timeline Visualization** - Events appear as interactive markers on playback timeline
*   **Event Markers** - Distinct emoji icons for each event type with color-coded indicators
*   **Interactive Markers** - Click any event marker to jump directly to recording at that moment
*   **Event Tooltips** - Hover over markers to see event details (type, camera, time, duration)
*   **Event Log Page** - Dedicated page with filterable, paginated event history
*   **Event Filtering** - Filter events by camera, event type, and date range
*   **Pagination Control** - Adjustable display (10-100 events per page)
*   **Per-Camera Event Management** - Clear events for individual cameras
*   **Bulk Event Operations** - Delete all events with confirmation code
*   **Event Storage** - JSON-based storage in `/recordings/events.json`
*   **Real-time Updates** - Events update automatically via Server-Sent Events

#### Event Management
*   Add events manually or import from external systems
*   Events tied to specific cameras and timestamps
*   Duration tracking for each event
*   Automatic synchronization with timeline playback
*   Event count display in camera management

### Web Interface

*   **Unified Control Header** - All playback controls accessible from single header bar
*   **Timeline View** - Visual representation of recording availability with 24-hour timeline
*   **Timeline Zoom** - Mouse wheel zoom with adaptive time markers (hourly to 1-minute granularity)
*   **Event Markers** - Visual indicators for camera events on timeline
*   **Multi-Camera Playback** - View up to 16 cameras simultaneously in grid layout
*   **Dropdown Camera Selector** - Multi-select dropdown with color-coded badges
*   **Fullscreen Mode** - Immersive viewing with timeline controls
*   **Download Manager** - Select cameras and time ranges for merged video downloads
*   **Collapsible Sections** - Organized layout with state persistence
*   **Responsive Design** - Desktop and mobile compatible with consistent spacing
*   **Dark Mode Support** - System preference detection
*   **Color-coded Identification** - 16 distinct colors for camera differentiation

### Playback Features

*   **Interactive Timeline** - Click timeline to jump to specific times or event markers
*   **Timeline Zoom** - Scroll to zoom in/out centered on cursor position
    *   Full day view: Hourly markers
    *   4-12 hours: 30-minute intervals
    *   2-4 hours: 10-minute intervals with HH:MM labels
    *   <2 hours: 1-minute precision with detailed markers
*   **Event-based Navigation** - Click event markers to seek directly to events
*   **Event Tooltips** - Rich event information on hover
*   **Time Range Selection** - Easily navigate through recordings
*   **Playback Controls** - Unified header with play/pause, navigation, speed, and fullscreen
*   **Keyboard Shortcuts** - Space for play/pause, arrow keys for navigation
*   **Multi-Camera Sync** - Synchronized playback across selected cameras
*   **Fullscreen with Timeline** - Access timeline and controls in fullscreen mode
*   **Persistent Camera Order** - Order set in configuration maintains in playback view
*   **Clean Video Display** - Individual video controls hidden for streamlined interface

### Recording Features

*   **Continuous Recording** - Start/stop recording per camera
*   **Automatic Cleanup** - Old recordings deleted based on retention settings
*   **Organized Storage** - Each camera gets unique directory with ID
*   **Auto-reconnect** - Automatic retry on stream errors (5-second delay)
*   **Recording Downloads** - Export concatenated recordings as single MP4 files
*   **Storage Metrics** - Per-camera disk usage displayed in camera management

### Camera Management

*   **Drag-and-Drop Ordering** - Reorder cameras with visual feedback and hover highlighting
*   **Real-time Updates** - Camera list updates immediately across all views
*   **Persistent Order** - Camera arrangement saved and synchronized
*   **Color Assignment** - Automatic color assignment from 16-color palette
*   **Bulk Operations** - Set retention, remove cameras, cleanup recordings, clear events
*   **Duplicate Prevention** - go2rtc scan prevents importing duplicate RTSP URLs
*   **Storage Display** - View disk space used by each camera's recordings

---

## 📋 Requirements

*   Docker and Docker Compose
*   go2rtc server (for camera stream management)
*   RTSP-compatible cameras (H.264 video codec)
*   FFmpeg (for video concatenation during downloads)

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
  vlan2_net:
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
      vlan2_net:
        ipv4_address: 192.168.2.180
        priority: 900
    volumes:
      - /mnt/backup_pool/operational/containers/simplenvr/build:/app
      - /mnt/backup_pool/operational/containers/simplenvr/recordings:/recordings
      - /mnt/backup_pool/operational/containers/simplenvr/certs:/certs
```

### 3. Starting the Container

```bash
docker-compose up -d
```

The application will be available securely at `https://192.168.2.180` (or `https://your-server-ip:8443` if port mapping).

---

## 🎮 Usage

### Unified Header Interface

The main interface features a unified header bar containing:

*   **Simple NVR** title (left)
*   **Camera Selector** dropdown with color badges (first control)
*   **Date Picker** for selecting playback date
*   **Speed Control** for playback speed adjustment (0.25x to 4x)
*   **Playback Controls** - Previous, Play/Pause, Next, Mute All, Fullscreen, Download (right side)
*   **Settings Icon** for camera configuration (far right)

### Adding Cameras

#### Method 1: Auto-discover from go2rtc

1.  Click the ⚙️ **Settings** icon in the top right
2.  This opens the Camera Management modal
3.  Enter your go2rtc URL (default: `http://go2rtc:1984`)
4.  Click **Scan & Import Streams**
5.  All streams will be automatically added with assigned colors
6.  Duplicate URLs are automatically skipped

#### Method 2: Manual Entry

1.  Open the Camera Management modal via ⚙️ **Settings**
2.  Click **Add Camera Manually**
3.  Enter camera name and RTSP URL
4.  Click **Add Camera**
5.  Camera receives automatic color assignment

### Organizing Cameras

#### Drag-and-Drop Reordering

1.  Open Camera Management modal
2.  Hover over camera card to see drag handle (6 dots icon on left)
3.  Click and drag the left side of any camera card
4.  Visual highlight shows drop position
5.  Drop camera in desired position
6.  Order automatically saves and applies to playback view
7.  Camera arrangement persists across all views

#### Color Identification

*   Each camera automatically receives one of 16 distinct colors
*   Colors appear in:
    *   Dropdown selector badges
    *   Timeline recording segments
    *   Video stream labels
    *   Event markers
*   Colors persist and remain stable across sessions

### Recording Management

#### Start Recording

1.  Navigate to Camera Management (⚙️ icon)
2.  Click **Start Recording** for desired camera
3.  Recording begins immediately in 10-minute segments

#### Stop Recording

1.  Click **Stop Recording** to halt recording
2.  Current segment completes before stopping

#### Set Retention Period

*   **Per Camera:** Adjust retention days and click **Update**
*   **All Cameras:** Use **Set All Retention** for bulk changes

#### View Storage Usage

*   Each camera card displays disk space consumed by recordings
*   Storage updates automatically as recordings are created/deleted

### Event Management

#### Viewing Events

1.  Navigate to Events page from main menu
2.  Events displayed in chronological order with pagination
3.  Each event shows: type icon, camera name, timestamp, duration
4.  Filter events by camera, type, or date range
5.  Adjust pagination (10-100 events per page)

#### Event Timeline Visualization

1.  Events appear as circular markers on playback timeline
2.  Each marker displays the event type emoji
3.  Hover over marker to see full event details
4.  Click marker to jump directly to recording at event time
5.  Subtle vertical line indicates event position

#### Clearing Events

*   **Per Camera:** Click "Clear Events" button on camera card, then click again to confirm
*   **All Cameras:** Use "Clear All Events" in bulk operations (requires confirmation code)

### Timeline Playback

#### Camera Selection

1.  Click the **Cameras** dropdown in the header
2.  Multi-select cameras using checkboxes
3.  Color badges indicate each camera's color
4.  Selection persists in browser
5.  Selected cameras appear in ordered grid layout

#### Basic Navigation

1.  Choose date using date picker in header
2.  Click timeline to jump to specific time
3.  Click event markers to jump to events
4.  Use **Previous/Next** buttons to navigate in 10-minute increments
5.  Timeline displays colored segments for each camera's recordings

#### Timeline Zoom

1.  Hover mouse over timeline
2.  Scroll with mouse wheel to zoom in/out
3.  Zoom centers on cursor position
4.  Time markers automatically adjust:
    *   **Full day:** Hour markers (00, 01, 02...)
    *   **4-12 hours:** 30-minute intervals
    *   **2-4 hours:** 10-minute intervals with HH:MM format
    *   **<2 hours:** 1-minute precision markers
5.  Click anywhere on zoomed timeline to seek precisely
6.  Zoom persists during session
7.  Event markers remain visible and clickable at all zoom levels

#### Playback Controls (Header Bar)

*   **Play/Pause:** Start or pause playback across all cameras
*   **Previous/Next:** Jump 10 minutes backward/forward
*   **Speed Control:** Adjust playback speed (0.25x, 0.5x, 1x, 2x, 4x)
*   **Mute All:** Toggle audio for all cameras simultaneously
*   **Fullscreen:** Expand to fullscreen with timeline controls
*   **Download:** Export recordings with custom time ranges

#### Keyboard Shortcuts

*   **Space:** Play/Pause
*   **←:** Previous 10 minutes
*   **→:** Next 10 minutes

#### Multi-Camera Viewing

Grid layout automatically adjusts for number of selected cameras:

*   1 camera: Full screen
*   2 cameras: 2x1 grid
*   3-4 cameras: 2x2 grid
*   5-6 cameras: 3x2 grid
*   7-9 cameras: 3x3 grid
*   10+ cameras: 4x? grid

All cameras sync to same timeline position. Video stream labels show camera name with assigned color. Individual video controls hidden for clean interface. Camera arrangement matches order set in configuration.

### Download Recordings

1.  Navigate to desired time on timeline
2.  Click **Download** button in header
3.  Adjust time range using **-10 min** and **+10 min** buttons
4.  Click **Reset** to return to current playback time
5.  Select/deselect cameras for download
6.  Click **Download Merged Videos**
7.  Each camera's recordings will be concatenated and downloaded as single MP4 file

### Viewing Live Streams

1.  Open Camera Management (⚙️ icon)
2.  Click **👁️ View Live** on any camera
3.  Stream opens in modal window
4.  Click **✖ Close** or press **Esc** to exit

### Removing Cameras

*   Click **Remove** once to confirm
*   Click **Remove** again within 3 seconds to delete
*   Use **Remove All Cameras** for bulk deletion (requires confirmation code)

### Bulk Operations

*   **Set All Retention:** Update retention for all cameras at once
*   **Delete All Recordings:** Remove all recordings from all cameras (requires confirmation code)
*   **Delete All Cameras:** Remove all cameras (requires confirmation code)
*   **Clear All Events:** Remove all events from all cameras (requires confirmation code)
*   **Cleanup Orphaned Recordings:** Remove recordings from deleted cameras

---

## 🏡 Home Assistant Integration

SimpleNVR can easily ingest events from Home Assistant (such as AI detections, line crossings, or standard motion sensors) using Home Assistant's native `rest_command` integration. By mapping a helper toggle to your camera events, you can track the exact duration of an event and drop it seamlessly onto your SimpleNVR timeline.

### 1. Configure the REST Command
Add the following to your Home Assistant `configuration.yaml` file to define how to format API requests to SimpleNVR. *(Ensure you adjust the IP address and port to match your SimpleNVR instance):*

```yaml
rest_command:
  simplenvr_trigger_event:
    url: "[https://192.168.2.180/api/events/trigger](https://192.168.2.180/api/events/trigger)"
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
*Note: You must restart Home Assistant after modifying `configuration.yaml` to load the new REST command.*

### 2. Create an Event Toggle (Helper)
To accurately track the start and end of an alarm, create a Toggle helper in Home Assistant:
1. Go to **Settings** > **Devices & Services** > **Helpers**.
2. Click **+ Create Helper** and select **Toggle**.
3. Name it something recognizable like `Cam1 Event Toggle` (this will create `input_boolean.cam1_event_toggle`).

### 3. Example Duration-Calculating Automation
This automation listens for the toggle to turn off. Once the event ends, it calculates exactly how long the toggle was active, then fires the formatted data to SimpleNVR. 

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
      # MUST exactly match the camera name configured inside SimpleNVR
      camera_name: "Cam1" 
      
      # The text label / type of the event (e.g., Line Crossing, Motion Detected)
      label: "Line Crossing" 
      
      # Grabs the exact UTC timestamp of when the toggle originally turned ON
      start_time: "{{ trigger.from_state.last_updated.isoformat() }}"
      
      # Subtracts the ON time from the OFF time to generate the duration in seconds
      duration_seconds: >
        {{ (as_timestamp(trigger.to_state.last_updated) - as_timestamp(trigger.from_state.last_updated)) | int(default=5) }}
```
**To use this workflow:** Simply create a standard Home Assistant automation that turns `input_boolean.cam1_event_toggle` **ON** when your camera triggers an alarm, and **OFF** when it clears. The automation above will handle the math and the API payload automatically!

---

## 🔒 SSL / TLS Certificates

SimpleNVR runs over HTTPS on port `443` internally. 

*   **Auto-Generated Self-Signed Certs:** On first run, if no certificate files are present in the mounted `/certs` directory, SimpleNVR automatically creates a self-signed `cert.pem` and `key.pem`.
*   **Custom Certificates:** To use custom or Let's Encrypt certificates, place your certificate and private key in the mounted `/certs` folder named `cert.pem` and `key.pem`. SimpleNVR will automatically detect and load them on startup.

---

## 📁 File Structure

### Recording Files

*   **Location:** `/recordings/{camera_id}/`
*   **Format:** `YYYYMMDD_HHMMSS_CameraName.mp4`
*   **Example:** `20251025_143022_FrontDoor.mp4`
*   **Segment Length:** 10 minutes per file
*   **Files per day:** 144 per camera (24 hours × 6 segments/hour)

### Configuration & Certificates

*   **cameras.json** - Camera settings, retention periods, recording states, camera order, color assignments (`/recordings/cameras.json`)
*   **events.json** - Event log with timestamps, cameras, types, and durations (`/recordings/events.json`)
*   **cert.pem / key.pem** - SSL/TLS certificates (`/certs/`)

### Application Files

*   `main.go`: HTTP/HTTPS server, API handlers, camera management
*   `recorder.go`: RTSP recording, MP4 muxing, cleanup routines
*   `handlers.go`: HTTP request handlers, camera operations
*   `playback.go`: Timeline data, recording queries
*   `events.go`: Event management, storage, and API endpoints
*   `cleanup.go`: Retention and orphan cleanup
*   `templates/`: HTML templates (index, camera cards)
*   `static/js/`: Modular JavaScript (playback, cameras, modals, core, recordings)
*   `static/style.css`: Application styling with responsive design

---

## 🔧 Technical Details

### Recording Pipeline

1.  **RTSP Connection:** gortsplib client connects to camera
2.  **H.264 Decoding:** RTP packets depacketized to NAL units
3.  **SPS/PPS Extraction:** Codec parameters parsed
4.  **MP4 Muxing:** deepch/vdk writes MP4 container
5.  **Segmentation:** New file every 10 minutes
6.  **Cleanup:** Old files deleted based on retention settings

### Event System Architecture

*   **Storage:** JSON file with event array
*   **Structure:** Each event contains timestamp, camera_name, label, duration_seconds
*   **Timeline Integration:** Events loaded with date-based queries
*   **Canvas Rendering:** Events drawn as colored circles with emoji icons
*   **Click Detection:** Distance calculation from cursor to event marker
*   **Tooltip System:** Dynamic DOM elements created/destroyed on hover
*   **Seek Integration:** Event click triggers video seek to event timestamp

### Timeline System

*   **Date-based Query:** API returns all recordings and events for selected date
*   **Visual Timeline:** Canvas-based 24-hour timeline showing recording availability
*   **Event Overlay:** Events rendered on top of recording segments
*   **Zoom Implementation:**
    *   Mouse wheel event with preventDefault
    *   Dynamic zoom level (1.0 = full day, up to 24.0 = 1-hour view)
    *   Cursor-centered zooming preserves context
    *   Adaptive time markers based on visible range
    *   Efficient rendering of only visible segments and events
*   **Color-coded Cameras:** 16 distinct colors automatically assigned
*   **Real-time Sync:** Multiple cameras play synchronized to selected time
*   **Segment Matching:** Finds appropriate recording segment based on timestamp
*   **Persistent Selection:** Camera selection stored in localStorage

### Camera Order System

*   **Drag-and-Drop:** HTML5 drag and drop with visual feedback
*   **Hover Highlighting:** Drag-over class shows valid drop zones
*   **Drag Leave Detection:** Highlight clears when dragging away from target
*   **Configuration Sync:** Order set in modal applies to playback grid
*   **Stable Colors:** Color assignments tied to camera ID, not position
*   **Label Synchronization:** Video labels update when order changes
*   **Persistent Storage:** Order saved to cameras.json
*   **Real-time Updates:** Changes immediately reflected in all views

### Download System

*   **FFmpeg Concatenation:** Uses concat demuxer to merge segments
*   **Streaming Output:** Progressive download as FFmpeg processes
*   **Time Range Filtering:** Selects only recordings within specified time range
*   **Per-Camera Output:** Creates separate merged file for each selected camera
*   **Browser-friendly:** MP4 output with fragmented headers for streaming

### API Endpoints

#### Camera Management

*   `GET /api/cameras` - List all cameras (HTML partial with drag-drop markup)
*   `POST /api/cameras/add` - Add new camera with color assignment
*   `POST /api/cameras/delete` - Remove camera
*   `POST /api/cameras/delete-all` - Remove all cameras
*   `POST /api/cameras/update-retention` - Update single camera retention
*   `POST /api/cameras/update-retention-all` - Update all cameras retention
*   `POST /api/cameras/reorder` - Update camera display order

#### Recording Control

*   `POST /api/record/start` - Start recording camera
*   `POST /api/record/stop` - Stop recording camera

#### Playback & Timeline

*   `GET /api/recordings/list?camera_id={id}` - List recordings (JSON)
*   `GET /api/timeline/recordings?date=YYYY-MM-DD` - Get timeline data with color info
*   `GET /recordings/{camera_id}/{filename}` - Serve recording file

#### Event Management

*   `GET /api/events/list` - List all events (JSON)
*   `POST /api/events/add` - Add new event
*   `POST /api/events/clear-camera` - Clear events for specific camera
*   `POST /api/events/clear-all` - Clear all events (requires confirmation)

#### Downloads

*   `GET /api/download?camera_id={id}&start_time={iso8601}&end_time={iso8601}` - Download merged recording

#### go2rtc Integration

*   `POST /api/go2rtc/scan` - Scan and import streams (skips duplicate URLs)

#### Live Streaming

*   `GET /api/stream/{camera_name}` - Proxy live stream from go2rtc

#### Bulk Operations

*   `POST /api/recordings/delete-all` - Delete all recordings from all cameras
*   `POST /api/recordings/cleanup-orphaned` - Remove orphaned recordings

### Dependencies

#### Go Modules

*   `github.com/bluenviron/gortsplib/v4` - RTSP client
*   `github.com/bluenviron/mediacommon` - H.264 codec handling
*   `github.com/deepch/vdk` - MP4 muxing
*   `github.com/pion/rtp` - RTP packet processing

#### System Dependencies

*   **FFmpeg** - Video concatenation for downloads

#### Frontend

*   **Vanilla JavaScript** - No framework dependencies
*   **HTML5 Canvas** - Timeline rendering with zoom and events
*   **HTML5 Drag and Drop** - Camera reordering
*   **CSS Variables** - Theming and customization
*   **localStorage** - Persistent camera selection

### Browser Compatibility

*   **Chrome/Edge:** Full support with hardware acceleration
*   **Firefox:** Full support with compatibility optimizations
*   **Safari:** Full support (H.264 native)
*   **Mobile:** Responsive design, touch-friendly

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

## 🔍 Monitoring

### Logs

```bash
docker logs -f simplenvr
```

### Log Messages

*   `Starting recording for {camera}` - Recording initiated
*   `Completed 10-minute segment for {camera}` - Segment saved
*   `Recording error for {camera}` - Connection issue, retrying
*   `Cleaned up old recording` - Retention cleanup executed
*   `Download request for camera {name}: {count} files` - Download initiated
*   `Download completed successfully for camera {name}` - Download finished
*   `Camera order updated` - Drag-drop reorder saved
*   `Event added: {type} for {camera}` - New event logged
*   `Skipping camera {name} - URL already exists` - go2rtc scan duplicate prevention
*   `Found existing TLS certificates in /certs.` - HTTPS certs loaded
*   `No TLS certificates found. Auto-generating self-signed TLS certificate...` - Cert generation trigger

---

## 🐛 Troubleshooting

### Recording Not Starting

*   Check RTSP URL is correct
*   Verify camera is H.264 encoded
*   Ensure network connectivity to camera
*   Check logs for connection errors

### Playback Issues

*   Verify recordings exist in `/recordings/{camera_id}/`
*   Check browser console for errors
*   Ensure MP4 files are not corrupted
*   Try different browser
*   Clear browser cache and reload
*   Disable browser extensions that might interfere with video

### Timeline Not Showing Recordings

*   Verify correct date is selected
*   Check that cameras are selected in dropdown
*   Confirm recordings exist for that date
*   Check browser console for API errors
*   Try zooming out to full day view

### Event Markers Not Appearing

*   Verify events exist in `/recordings/events.json`
*   Check browser console for JavaScript errors
*   Confirm date matches event timestamps
*   Try refreshing the page
*   Check that events are properly formatted in JSON

### Timeline Zoom Not Working

*   Ensure mouse is over timeline canvas
*   Check browser console for JavaScript errors
*   Try refreshing the page
*   Verify browser supports wheel events

---

## 📊 Performance

### Resource Usage (per camera)

*   **CPU:** ~2-5% per active recording
*   **RAM:** ~50-100MB per camera
*   **Disk I/O:** ~1-2 MB/s per camera (depends on bitrate)
*   **Network:** Matches camera RTSP bitrate

### Recommended Limits

*   **10 cameras:** Standard hardware (4 cores, 8GB RAM)
*   **25 cameras:** Mid-range server (8 cores, 16GB RAM)
*   **50+ cameras:** Dedicated server (16+ cores, 32GB+ RAM)

### Storage Requirements

Example: 1080p camera @ 4 Mbps

*   **Per hour:** ~1.8 GB
*   **Per day:** ~43 GB
*   **7-day retention:** ~301 GB
*   **30-day retention:** ~1.29 TB

---

## 🔐 Security Considerations

### Network Isolation

*   Runs on isolated Docker networks
*   Cameras on separate VLAN recommended
*   go2rtc acts as RTSP gateway

### Access Control & HTTPS

*   **Native HTTPS:** Serves web interface and API natively over HTTPS on port 443
*   **VLAN/Firewalls:** Network isolation recommended since there is no built-in auth

### Data Protection

*   Recordings stored on host filesystem
*   No cloud uploads (fully self-hosted)
*   Container has limited access to recordings and certs directories
*   Events stored locally in JSON format

---

## 📄 License

This is a custom-built project. Adjust licensing as needed.

## 🤝 Contributing

For issues, improvements, or questions, please refer to project documentation.

## 📞 Support

*   **Review logs:** `docker logs simplenvr`
*   **Check recording directory:** `/recordings/`
*   **Check certs directory:** `/certs/`
*   **Check event log:** `/recordings/events.json`
*   **Verify go2rtc:** `http://go2rtc:1984`
*   Browser console for frontend errors
*   API endpoints for debugging
*   Check `cameras.json` for configuration issues
