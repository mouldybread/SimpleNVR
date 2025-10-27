# SimpleNVR

A lightweight, self-hosted Network Video Recorder (NVR) built in Go that uses FFmpeg for RTSP recording and MP4 output. SimpleNVR integrates with go2rtc for camera stream management and provides a web interface for monitoring, recording, and playback.

Built with go2rtc in mind, this NVR assumes you already have adequate live viewing arrangements from your go2rtc headend to endpoints such as Home Assistant or TVs. It provides basic live stream preview for troubleshooting but focuses on recording and playback.

> **⚠️ Security Notice**  
> Like go2rtc, it makes no attempt at security. It has no authentication or encryption. If required, you will need to bring your own in the form of a reverse proxy or the like. Otherwise, keep it isolated alongside go2rtc on a trusted network.

It ingests RTSP streams via FFmpeg, usually from a go2rtc server, and saves them to disk in 10-minute segments with configurable retention periods per camera (1-365 days).

It provides an intuitive timeline-based interface to replay recordings with multi-camera synchronized playback and download merged clips as needed. It runs from a Docker container and tries to be as unobtrusive as possible. It pairs well with TrueNAS Scale or similar storage solutions.

## 🎯 Features

### Core Functionality

- **Native Go RTSP Recording** - Pure Go implementation using gortsplib
- **MP4 Output** - Browser-compatible video files with H.264 codec
- **10-Minute Segments** - Automatic file splitting for reliability and management
- **Configurable Retention** - Per-camera retention periods (1-365 days)
- **Timeline Playback** - Interactive timeline with multi-camera synchronized playback
- **Recording Downloads** - Export merged recordings with customizable time ranges
- **Live Stream Preview** - Real-time camera feeds through web interface
- **go2rtc Integration** - Auto-discover and import cameras from go2rtc server
- **Drag-and-Drop Reordering** - Organize camera order with visual feedback

### Web Interface

- **Unified Control Header** - All playback controls accessible from single header bar
- **Timeline View** - Visual representation of recording availability with 24-hour timeline
- **Timeline Zoom** - Mouse wheel zoom with adaptive time markers (hourly to 1-minute granularity)
- **Multi-Camera Playback** - View up to 16 cameras simultaneously in grid layout
- **Dropdown Camera Selector** - Multi-select dropdown with color-coded badges
- **Fullscreen Mode** - Immersive viewing with timeline controls
- **Download Manager** - Select cameras and time ranges for merged video downloads
- **Collapsible Sections** - Organized layout with state persistence
- **Responsive Design** - Desktop and mobile compatible with consistent spacing
- **Dark Mode Support** - System preference detection
- **Color-coded Identification** - 16 distinct colors for camera differentiation

### Playback Features

- **Interactive Timeline** - Click timeline to jump to specific times
- **Timeline Zoom** - Scroll to zoom in/out centered on cursor position
  - Full day view: Hourly markers
  - 4-12 hours: 30-minute intervals
  - 2-4 hours: 10-minute intervals with HH:MM labels
  - <2 hours: 1-minute precision with detailed markers
- **Time Range Selection** - Easily navigate through recordings
- **Playback Controls** - Unified header with play/pause, navigation, speed, and fullscreen
- **Keyboard Shortcuts** - Space for play/pause, arrow keys for navigation
- **Multi-Camera Sync** - Synchronized playback across selected cameras
- **Fullscreen with Timeline** - Access timeline and controls in fullscreen mode
- **Persistent Camera Order** - Order set in configuration maintains in playback view
- **Clean Video Display** - Individual video controls hidden for streamlined interface

### Recording Features

- **Continuous Recording** - Start/stop recording per camera
- **Automatic Cleanup** - Old recordings deleted based on retention settings
- **Organized Storage** - Each camera gets unique directory with ID
- **Auto-reconnect** - Automatic retry on stream errors (5-second delay)
- **Recording Downloads** - Export concatenated recordings as single MP4 files

### Camera Management

- **Drag-and-Drop Ordering** - Reorder cameras with visual feedback
- **Real-time Updates** - Camera list updates immediately without refresh
- **Persistent Order** - Camera arrangement saved and synchronized across views
- **Color Assignment** - Automatic color assignment from 16-color palette
- **Bulk Operations** - Set retention, remove cameras, cleanup recordings

## 📋 Requirements

- Docker and Docker Compose
- go2rtc server (for camera stream management)
- RTSP-compatible cameras (H.264 video codec)
- FFmpeg (for video concatenation during downloads)

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
└── recordings/
```

### 2. Docker Compose Configuration

```yaml
services:
  simplenvr:
    container_name: simplenvr
    image: golang:1.23-alpine
    working_dir: /app
    command: >
      sh -c "
      apk add --no-cache ffmpeg &&
      go install github.com/air-verse/air@latest &&
      air
      "
    volumes:
      - /path/to/SimpleNVR/build:/app
      - /path/to/SimpleNVR/recordings:/recordings
    ports:
      - "8099:80"
    networks:
      - your_network
    restart: unless-stopped
```

### 3. Starting the Container

```bash
docker-compose up -d
```

The application will be available at `http://your-server-ip:8099`

## 🎮 Usage

### Unified Header Interface

The main interface features a unified header bar containing:

- **Simple NVR** title (left)
- **Camera Selector** dropdown with color badges (first control)
- **Date Picker** for selecting playback date
- **Speed Control** for playback speed adjustment (0.25x to 4x)
- **Playback Controls** - Previous, Play/Pause, Next, Mute All, Fullscreen, Download (right side)
- **Settings Icon** for camera configuration (far right)

### Adding Cameras

#### Method 1: Auto-discover from go2rtc

1. Click the ⚙️ **Settings** icon in the top right
2. This opens the Camera Management modal
3. Enter your go2rtc URL (default: `http://go2rtc:1984`)
4. Click **Scan & Import Streams**
5. All streams will be automatically added with assigned colors

#### Method 2: Manual Entry

1. Open the Camera Management modal via ⚙️ **Settings**
2. Click **Add Camera Manually**
3. Enter camera name and RTSP URL
4. Click **Add Camera**
5. Camera receives automatic color assignment

### Organizing Cameras

#### Drag-and-Drop Reordering

1. Open Camera Management modal
2. Click and drag any camera card
3. Visual feedback shows drop position
4. Drop camera in desired position
5. Order automatically saves and applies to playback view
6. Camera arrangement persists in playback grid layout

#### Color Identification

- Each camera automatically receives one of 16 distinct colors
- Colors appear in:
  - Dropdown selector badges
  - Timeline recording segments
  - Video stream labels
- Colors persist and remain stable across sessions

### Recording Management

#### Start Recording

1. Navigate to Camera Management (⚙️ icon)
2. Click **Start Recording** for desired camera
3. Recording begins immediately in 10-minute segments

#### Stop Recording

1. Click **Stop Recording** to halt recording
2. Current segment completes before stopping

#### Set Retention Period

- **Per Camera:** Adjust retention days and click **Update**
- **All Cameras:** Use **Set All Retention** for bulk changes

### Timeline Playback

#### Camera Selection

1. Click the **Cameras** dropdown in the header
2. Multi-select cameras using checkboxes
3. Color badges indicate each camera's color
4. Selection persists in browser
5. Selected cameras appear in ordered grid layout

#### Basic Navigation

1. Choose date using date picker in header
2. Click timeline to jump to specific time
3. Use **Previous/Next** buttons to navigate in 10-minute increments
4. Timeline displays colored segments for each camera's recordings

#### Timeline Zoom

1. Hover mouse over timeline
2. Scroll with mouse wheel to zoom in/out
3. Zoom centers on cursor position
4. Time markers automatically adjust:
   - **Full day:** Hour markers (00, 01, 02...)
   - **4-12 hours:** 30-minute intervals
   - **2-4 hours:** 10-minute intervals with HH:MM format
   - **<2 hours:** 1-minute precision markers
5. Click anywhere on zoomed timeline to seek precisely
6. Zoom persists during session

#### Playback Controls (Header Bar)

- **Play/Pause:** Start or pause playback across all cameras
- **Previous/Next:** Jump 10 minutes backward/forward
- **Speed Control:** Adjust playback speed (0.25x, 0.5x, 1x, 2x, 4x)
- **Mute All:** Toggle audio for all cameras simultaneously
- **Fullscreen:** Expand to fullscreen with timeline controls
- **Download:** Export recordings with custom time ranges

#### Keyboard Shortcuts

- **Space:** Play/Pause
- **←:** Previous 10 minutes
- **→:** Next 10 minutes

#### Multi-Camera Viewing

Grid layout automatically adjusts for number of selected cameras:

- 1 camera: Full screen
- 2 cameras: 2x1 grid
- 3-4 cameras: 2x2 grid
- 5-6 cameras: 3x2 grid
- 7-9 cameras: 3x3 grid
- 10+ cameras: 4x? grid

All cameras sync to same timeline position. Video stream labels show camera name with assigned color. Individual video controls hidden for clean interface. Camera arrangement matches order set in configuration.

### Download Recordings

1. Navigate to desired time on timeline
2. Click **Download** button in header
3. Adjust time range using **-10 min** and **+10 min** buttons
4. Click **Reset** to return to current playback time
5. Select/deselect cameras for download
6. Click **Download Merged Videos**
7. Each camera's recordings will be concatenated and downloaded as single MP4 file

### Viewing Live Streams

1. Open Camera Management (⚙️ icon)
2. Click **👁️ View Live** on any camera
3. Stream opens in modal window
4. Click **✕ Close** or press **Esc** to exit

### Removing Cameras

- Click **Remove** once to confirm
- Click **Remove** again within 3 seconds to delete
- Use **Remove All Cameras** for bulk deletion (requires confirmation code)

### Bulk Operations

- **Set All Retention:** Update retention for all cameras at once
- **Delete All Recordings:** Remove all recordings from all cameras (requires confirmation code)
- **Delete All Cameras:** Remove all cameras (requires confirmation code)
- **Cleanup Orphaned Recordings:** Remove recordings from deleted cameras

## 📁 File Structure

### Recording Files

- **Location:** `/recordings/{camera_id}/`
- **Format:** `YYYYMMDD_HHMMSS_CameraName.mp4`
- **Example:** `20251025_143022_FrontDoor.mp4`
- **Segment Length:** 10 minutes per file
- **Files per day:** 144 per camera (24 hours × 6 segments/hour)

### Configuration

- **Location:** `/recordings/cameras.json`
- **Contains:** Camera settings, retention periods, recording states, camera order, color assignments
- **Persistence:** Automatically saved on changes

### Application Files

- `main.go`: HTTP server, API handlers, camera management
- `recorder.go`: RTSP recording, MP4 muxing, cleanup routines
- `handlers.go`: HTTP request handlers, camera operations
- `playback.go`: Timeline data, recording queries
- `events.go`: Server-Sent Events for real-time updates
- `cleanup.go`: Retention and orphan cleanup
- `templates/`: HTML templates (index, camera cards)
- `static/js/`: Modular JavaScript (playback, cameras, modals, core, recordings)
- `static/style.css`: Application styling with responsive design

## 🔧 Technical Details

### Recording Pipeline

1. **RTSP Connection:** gortsplib client connects to camera
2. **H.264 Decoding:** RTP packets depacketized to NAL units
3. **SPS/PPS Extraction:** Codec parameters parsed
4. **MP4 Muxing:** deepch/vdk writes MP4 container
5. **Segmentation:** New file every 10 minutes
6. **Cleanup:** Old files deleted based on retention settings

### Timeline System

- **Date-based Query:** API returns all recordings for selected date
- **Visual Timeline:** Canvas-based 24-hour timeline showing recording availability
- **Zoom Implementation:**
  - Mouse wheel event with preventDefault
  - Dynamic zoom level (1.0 = full day, up to 24.0 = 1-hour view)
  - Cursor-centered zooming preserves context
  - Adaptive time markers based on visible range
  - Efficient rendering of only visible segments
- **Color-coded Cameras:** 16 distinct colors automatically assigned
- **Real-time Sync:** Multiple cameras play synchronized to selected time
- **Segment Matching:** Finds appropriate recording segment based on timestamp
- **Persistent Selection:** Camera selection stored in localStorage

### Camera Order System

- **Drag-and-Drop:** HTML5 drag and drop with visual feedback
- **Configuration Sync:** Order set in modal applies to playback grid
- **Stable Colors:** Color assignments tied to camera ID, not position
- **Label Synchronization:** Video labels update when order changes
- **Persistent Storage:** Order saved to cameras.json

### Download System

- **FFmpeg Concatenation:** Uses concat demuxer to merge segments
- **Streaming Output:** Progressive download as FFmpeg processes
- **Time Range Filtering:** Selects only recordings within specified time range
- **Per-Camera Output:** Creates separate merged file for each selected camera
- **Browser-friendly:** MP4 output with fragmented headers for streaming

### API Endpoints

#### Camera Management

- `GET /api/cameras` - List all cameras (HTML partial with drag-drop markup)
- `POST /api/cameras/add` - Add new camera with color assignment
- `POST /api/cameras/delete` - Remove camera
- `POST /api/cameras/delete-all` - Remove all cameras
- `POST /api/cameras/update-retention` - Update single camera retention
- `POST /api/cameras/update-retention-all` - Update all cameras retention
- `POST /api/cameras/reorder` - Update camera display order

#### Recording Control

- `POST /api/record/start` - Start recording camera
- `POST /api/record/stop` - Stop recording camera

#### Playback & Timeline

- `GET /api/recordings/list?camera_id={id}` - List recordings (JSON)
- `GET /api/timeline/recordings?date=YYYY-MM-DD` - Get timeline data with color info
- `GET /recordings/{camera_id}/{filename}` - Serve recording file

#### Downloads

- `GET /api/download?camera_id={id}&start_time={iso8601}&end_time={iso8601}` - Download merged recording

#### go2rtc Integration

- `POST /api/go2rtc/scan` - Scan and import streams from go2rtc

#### Live Streaming

- `GET /api/stream/{camera_name}` - Proxy live stream from go2rtc

#### Bulk Operations

- `POST /api/recordings/delete-all` - Delete all recordings from all cameras
- `POST /api/recordings/cleanup-orphaned` - Remove orphaned recordings

### Dependencies

#### Go Modules

- `github.com/bluenviron/gortsplib/v4` - RTSP client
- `github.com/bluenviron/mediacommon` - H.264 codec handling
- `github.com/deepch/vdk` - MP4 muxing
- `github.com/pion/rtp` - RTP packet processing

#### System Dependencies

- **FFmpeg** - Video concatenation for downloads

#### Frontend

- **Vanilla JavaScript** - No framework dependencies
- **HTML5 Canvas** - Timeline rendering with zoom
- **HTML5 Drag and Drop** - Camera reordering
- **CSS Variables** - Theming and customization
- **localStorage** - Persistent camera selection

### Browser Compatibility

- **Chrome/Edge:** Full support with hardware acceleration
- **Firefox:** Full support with compatibility optimizations
- **Safari:** Full support (H.264 native)
- **Mobile:** Responsive design, touch-friendly

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

### Port

Edit `docker-compose.yml`:

```yaml
ports:
  - "8099:80"  # Change 8099 to desired port
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

### Grid Layout

Automatic layout based on number of selected cameras (defined in `updateVideoGrid` function)

### Spacing and Margins

Edit `style.css`:

```css
#playback-section {
    margin: 20px;  /* Adjust for overall spacing */
}
```

### Styling

Edit `static/style.css` - Uses CSS variables for theming

## 🔍 Monitoring

### Logs

```bash
docker logs -f simplenvr
```

### Log Messages

- `Starting recording for {camera}` - Recording initiated
- `Completed 10-minute segment for {camera}` - Segment saved
- `Recording error for {camera}` - Connection issue, retrying
- `Cleaned up old recording` - Retention cleanup executed
- `Download request for camera {name}: {count} files` - Download initiated
- `Download completed successfully for camera {name}` - Download finished
- `Camera order updated` - Drag-drop reorder saved

### Health Checks

- Camera list updates automatically via Server-Sent Events
- Recording state displayed in real-time
- Auto-reconnect on stream failures
- Timeline updates when date, camera selection, or zoom changes
- Drag-and-drop feedback confirms successful reordering

## 🐛 Troubleshooting

### Recording Not Starting

- Check RTSP URL is correct
- Verify camera is H.264 encoded
- Ensure network connectivity to camera
- Check logs for connection errors

### Playback Issues

- Verify recordings exist in `/recordings/{camera_id}/`
- Check browser console for errors
- Ensure MP4 files are not corrupted
- Try different browser
- Clear browser cache and reload
- Disable browser extensions that might interfere with video

### Timeline Not Showing Recordings

- Verify correct date is selected
- Check that cameras are selected in dropdown
- Confirm recordings exist for that date
- Check browser console for API errors
- Try zooming out to full day view

### Timeline Zoom Not Working

- Ensure mouse is over timeline canvas
- Check browser console for JavaScript errors
- Try refreshing the page
- Verify browser supports wheel events

### Camera Order Not Persisting

- Check browser localStorage is enabled
- Verify `cameras.json` is writable
- Review browser console for errors
- Try manually reordering in configuration modal

### Download Fails

- Verify FFmpeg is installed in container
- Check that recordings exist for selected time range
- Review server logs for FFmpeg errors
- Ensure sufficient disk space
- Try smaller time range

### Live Stream Fails

- Confirm go2rtc is running and accessible
- Check go2rtc has the camera configured
- Verify Docker network connectivity
- Review go2rtc logs

### High Disk Usage

- Reduce retention period
- Decrease segment duration
- Stop recording on unused cameras
- Monitor cleanup routine execution
- Use **Delete All Recordings** if needed

### Videos Disappear on Seek (Firefox)

- Firefox compatibility optimizations are in place
- Debounced seek operations prevent overload
- Staggered video loading (150ms between cameras)
- If issues persist, try Chrome/Edge

### Dropdown Selector Issues

- Clear browser cache and refresh
- Check JavaScript console for errors
- Verify `cameras.json` contains color assignments
- Try re-scanning from go2rtc to reset colors

## 📊 Performance

### Resource Usage (per camera)

- **CPU:** ~2-5% per active recording
- **RAM:** ~50-100MB per camera
- **Disk I/O:** ~1-2 MB/s per camera (depends on bitrate)
- **Network:** Matches camera RTSP bitrate

### Recommended Limits

- **10 cameras:** Standard hardware (4 cores, 8GB RAM)
- **25 cameras:** Mid-range server (8 cores, 16GB RAM)
- **50+ cameras:** Dedicated server (16+ cores, 32GB+ RAM)

### Storage Requirements

Example: 1080p camera @ 4 Mbps

- **Per hour:** ~1.8 GB
- **Per day:** ~43 GB
- **7-day retention:** ~301 GB
- **30-day retention:** ~1.29 TB

### Timeline Performance

- Optimized for fast date queries
- Efficient segment matching algorithm
- Canvas rendering for smooth timeline visualization
- Zoom rendering only displays visible segments
- Debounced API calls to prevent overload
- Staggered video loading prevents server strain

## 🔐 Security Considerations

### Network Isolation

- Runs on isolated Docker networks
- Cameras on separate VLAN recommended
- go2rtc acts as RTSP gateway

### Access Control

- **Port Security:** Bind to localhost if using reverse proxy
- **Authentication:** Add reverse proxy with auth (nginx, Caddy, Traefik)
- **HTTPS:** Terminate TLS at reverse proxy

### Data Protection

- Recordings stored on host filesystem
- No cloud uploads (fully self-hosted)
- Container has limited access to recordings directory

### Download Security

- Downloads are temporary (FFmpeg streams directly)
- No persistent export files on server
- Time-based access control possible via reverse proxy

## 📝 Recent Updates

### Timeline Zoom System

- Mouse wheel zoom on timeline canvas
- Adaptive time markers (hourly → 10-minute → 1-minute)
- Cursor-centered zoom preserves context
- Zoom level persists during session
- Efficient rendering of visible segments only

### Unified Header Interface

- All controls consolidated into single header bar
- Dropdown camera selector with color badges
- Streamlined layout with consistent spacing
- Settings icon for camera configuration access
- Clean, professional appearance

### Camera Organization

- Drag-and-drop reordering in configuration modal
- Visual feedback during drag operations
- Order persists across playback and configuration views
- Camera labels synchronized with order changes
- Stable color assignments independent of position

### Enhanced Color System

- 16 distinct colors for camera identification
- Automatic color assignment on camera creation
- Colors visible in dropdown, timeline, and video labels
- Maximum differentiation between cameras
- Persistent across sessions

### Interface Improvements

- Consistent 20px margins throughout
- Hidden individual video controls for clean display
- Responsive layout adapts to content
- Persistent camera selection via localStorage
- Real-time updates without page refresh

### Timeline Playback System

- Interactive 24-hour timeline visualization
- Multi-camera synchronized playback
- Grid layout with automatic arrangement
- Fullscreen mode with embedded timeline
- Color-coded camera identification
- Persistent camera selection in localStorage

### Download Manager

- Export merged recordings as single MP4 files
- Customizable time range selection
- Per-camera download selection
- Progress tracking for downloads
- Reset button for time range
- FFmpeg-based concatenation

### Browser Compatibility

- Firefox-specific optimizations
- Debounced seek operations
- Staggered video loading to prevent server overload
- Proper video state management
- Error handling for missing segments

### UI Improvements

- Collapsible sections with state persistence
- Bulk operations with confirmation codes
- Orphaned recording cleanup
- Improved modal designs
- Responsive layouts

## 📄 License

This is a custom-built project. Adjust licensing as needed.

## 🤝 Contributing

For issues, improvements, or questions, please refer to project documentation.

## 📞 Support

- **Review logs:** `docker logs simplenvr`
- **Check recording directory:** `/recordings/`
- **Verify go2rtc:** `http://go2rtc:1984`
- Browser console for frontend errors
- API endpoints for debugging
- Check `cameras.json` for configuration issues
