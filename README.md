# SimpleNVR

A lightweight, self-hosted Network Video Recorder (NVR) built in Go that uses FFmpeg for RTSP recording and MP4 output. SimpleNVR integrates with go2rtc for camera stream management and provides a web interface for monitoring, recording, and playback.

Built with go2rtc in mind, this NVR assumes you already have adequate live viewing arrangements from your go2rtc headend to endpoints such as Home Assistant or TVs. It provides basic live stream preview for troubleshooting but focuses on recording and playback.

Like go2rtc, it makes no attempt at security. It has no authentication or encryption. If required, you will need to bring your own in the form of a reverse proxy or the like. Otherwise, keep it isolated alongside go2rtc on a trusted network.

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

### Web Interface
- **Timeline View** - Visual representation of recording availability with 24-hour timeline
- **Multi-Camera Playback** - View up to 9 cameras simultaneously in grid layout
- **Fullscreen Mode** - Immersive viewing with timeline controls
- **Download Manager** - Select cameras and time ranges for merged video downloads
- **Collapsible Sections** - Organized layout with state persistence
- **Responsive Design** - Desktop and mobile compatible
- **Dark Mode Support** - System preference detection

### Playback Features
- **Interactive Timeline** - Click timeline to jump to specific times
- **Time Range Selection** - Easily navigate through recordings
- **Playback Controls** - Play/pause, previous/next segment, speed control
- **Keyboard Shortcuts** - Space for play/pause, arrow keys for navigation
- **Multi-Camera Sync** - Synchronized playback across selected cameras
- **Fullscreen with Timeline** - Access timeline and controls in fullscreen mode

### Recording Features
- **Continuous Recording** - Start/stop recording per camera
- **Automatic Cleanup** - Old recordings deleted based on retention settings
- **Organized Storage** - Each camera gets unique directory with ID
- **Auto-reconnect** - Automatic retry on stream errors (5-second delay)
- **Recording Downloads** - Export concatenated recordings as single MP4 files

## 📋 Requirements

- Docker and Docker Compose
- go2rtc server (for camera stream management)
- RTSP-compatible cameras (H.264 video codec)
- FFmpeg (for video concatenation during downloads)

## 🚀 Installation

### 1. Directory Structure

```
/path/to/SimpleNVR/
├── build/
│   ├── main.go
│   ├── recorder.go
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

### Adding Cameras

#### Method 1: Auto-discover from go2rtc
1. Open the **Camera Management** section
2. Enter your go2rtc URL (default: `http://go2rtc:1984`)
3. Click **Scan & Import Streams**
4. All streams will be automatically added

#### Method 2: Manual Entry
1. Open the **Camera Management** section
2. Click **Add Camera Manually**
3. Enter camera name and RTSP URL
4. Click **Add Camera**

### Recording Management

#### Start Recording
1. Navigate to **Camera Management**
2. Click **Start Recording** for desired camera
3. Recording begins immediately in 10-minute segments

#### Stop Recording
1. Click **Stop Recording** to halt recording
2. Current segment completes before stopping

#### Set Retention Period
- **Per Camera**: Adjust retention days and click **Update**
- **All Cameras**: Use **Set All Retention** for bulk changes

### Timeline Playback

#### Basic Navigation
1. Open the **Timeline Playback** section
2. Select cameras using checkboxes (selection persists in browser)
3. Choose date using date picker
4. Click timeline to jump to specific time
5. Use Previous/Next buttons to navigate in 10-minute increments

#### Playback Controls
- **Play/Pause**: Start or pause playback
- **Previous/Next**: Jump 10 minutes backward/forward
- **Speed Control**: Adjust playback speed (0.25x to 4x)
- **Mute All**: Toggle audio for all cameras
- **Fullscreen**: Expand to fullscreen with timeline controls
- **Download**: Export recordings with custom time ranges

#### Keyboard Shortcuts
- `Space`: Play/Pause
- `←`: Previous 10 minutes
- `→`: Next 10 minutes

### Download Recordings

1. Navigate to desired time on timeline
2. Click **Download** button
3. Adjust time range using **-10 min** and **+10 min** buttons
4. Click **Reset** to return to current playback time
5. Select/deselect cameras for download
6. Click **Download Merged Videos**
7. Each camera's recordings will be concatenated and downloaded as single MP4 file

### Viewing Live Streams
1. Click **👁️ View Live** on any camera
2. Stream opens in modal window
3. Click **✕ Close** or press `Esc` to exit

### Removing Cameras
1. Click **Remove** once to confirm
2. Click **Remove** again within 3 seconds to delete
3. Use **Remove All Cameras** for bulk deletion

### Bulk Operations
- **Set All Retention**: Update retention for all cameras at once
- **Delete All Recordings**: Remove all recordings from all cameras (requires confirmation code)
- **Delete All Cameras**: Remove all cameras (requires confirmation code)
- **Cleanup Orphaned Recordings**: Remove recordings from deleted cameras

## 📁 File Structure

### Recording Files
- **Location**: `/recordings/{camera_id}/`
- **Format**: `YYYYMMDD_HHMMSS_CameraName.mp4`
- **Example**: `20251025_143022_FrontDoor.mp4`
- **Segment Length**: 10 minutes per file
- **Files per day**: 144 per camera (24 hours × 6 segments/hour)

### Configuration
- **Location**: `/recordings/cameras.json`
- **Contains**: Camera settings, retention periods, recording states, camera order
- **Persistence**: Automatically saved on changes

### Application Files
- **main.go**: HTTP server, API handlers, camera management, download handler
- **recorder.go**: RTSP recording, MP4 muxing, cleanup routines
- **templates/**: HTML templates (index, camera cards)
- **static/js/**: Modular JavaScript (playback, cameras, modals, core, recordings)
- **static/style.css**: Application styling

## 🔧 Technical Details

### Recording Pipeline
1. **RTSP Connection**: gortsplib client connects to camera
2. **H.264 Decoding**: RTP packets depacketized to NAL units
3. **SPS/PPS Extraction**: Codec parameters parsed
4. **MP4 Muxing**: deepch/vdk writes MP4 container
5. **Segmentation**: New file every 10 minutes
6. **Cleanup**: Old files deleted based on retention settings

### Timeline System
- **Date-based Query**: API returns all recordings for selected date
- **Visual Timeline**: Canvas-based 24-hour timeline showing recording availability
- **Color-coded Cameras**: Each camera has unique color for easy identification
- **Real-time Sync**: Multiple cameras play synchronized to selected time
- **Segment Matching**: Finds appropriate recording segment based on timestamp

### Download System
- **FFmpeg Concatenation**: Uses concat demuxer to merge segments
- **Streaming Output**: Progressive download as FFmpeg processes
- **Time Range Filtering**: Selects only recordings within specified time range
- **Per-Camera Output**: Creates separate merged file for each selected camera
- **Browser-friendly**: MP4 output with fragmented headers for streaming

### API Endpoints

#### Camera Management
- `GET /api/cameras` - List all cameras (HTML partial)
- `POST /api/cameras/add` - Add new camera
- `POST /api/cameras/delete` - Remove camera
- `POST /api/cameras/delete-all` - Remove all cameras
- `POST /api/cameras/update-retention` - Update single camera retention
- `POST /api/cameras/update-retention-all` - Update all cameras retention

#### Recording Control
- `POST /api/record/start` - Start recording camera
- `POST /api/record/stop` - Stop recording camera

#### Playback & Timeline
- `GET /api/recordings/list?camera_id={id}` - List recordings (JSON)
- `GET /api/timeline/recordings?date=YYYY-MM-DD` - Get timeline data for date
- `GET /recordings/{camera_id}/{filename}` - Serve recording file

#### Downloads
- `GET /api/download?camera_id={id}&start_time={iso8601}&end_time={iso8601}` - Download merged recording

#### go2rtc Integration
- `POST /api/go2rtc/scan` - Scan and import streams from go2rtc

#### Live Streaming
- `GET /api/stream/{camera_name}` - Proxy live stream from go2rtc

#### Bulk Operations
- `POST /api/recordings/delete-all` - Delete all recordings from all cameras

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
- **HTML5 Canvas** - Timeline rendering
- **CSS Variables** - Theming and customization

### Browser Compatibility
- **Chrome/Edge**: Full support with hardware acceleration
- **Firefox**: Full support with compatibility optimizations
- **Safari**: Full support (H.264 native)
- **Mobile**: Responsive design, touch-friendly

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

### Grid Layout
Automatic layout based on number of selected cameras:
- 1 camera: 1x1 grid
- 2 cameras: 2x1 grid
- 3-4 cameras: 2x2 grid
- 5-6 cameras: 3x2 grid
- 7-9 cameras: 3x3 grid
- 10+ cameras: 4x? grid

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

### Health Checks
- Camera list updates automatically
- Recording state displayed in real-time
- Auto-reconnect on stream failures
- Timeline updates when date or camera selection changes

## 🐛 Troubleshooting

### Recording Not Starting
1. Check RTSP URL is correct
2. Verify camera is H.264 encoded
3. Ensure network connectivity to camera
4. Check logs for connection errors

### Playback Issues
1. Verify recordings exist in `/recordings/{camera_id}/`
2. Check browser console for errors
3. Ensure MP4 files are not corrupted
4. Try different browser
5. Clear browser cache and reload

### Timeline Not Showing Recordings
1. Verify correct date is selected
2. Check that cameras are selected (checkboxes)
3. Confirm recordings exist for that date
4. Check browser console for API errors

### Download Fails
1. Verify FFmpeg is installed in container
2. Check that recordings exist for selected time range
3. Review server logs for FFmpeg errors
4. Ensure sufficient disk space
5. Try smaller time range

### Live Stream Fails
1. Confirm go2rtc is running and accessible
2. Check go2rtc has the camera configured
3. Verify Docker network connectivity
4. Review go2rtc logs

### High Disk Usage
1. Reduce retention period
2. Decrease segment duration
3. Stop recording on unused cameras
4. Monitor cleanup routine execution
5. Use **Delete All Recordings** if needed

### Videos Disappear on Seek (Firefox)
- Firefox compatibility optimizations are in place
- Debounced seek operations prevent overload
- Staggered video loading (150ms between cameras)
- If issues persist, try Chrome/Edge

## 📊 Performance

### Resource Usage (per camera)
- **CPU**: ~2-5% per active recording
- **RAM**: ~50-100MB per camera
- **Disk I/O**: ~1-2 MB/s per camera (depends on bitrate)
- **Network**: Matches camera RTSP bitrate

### Recommended Limits
- **10 cameras**: Standard hardware (4 cores, 8GB RAM)
- **25 cameras**: Mid-range server (8 cores, 16GB RAM)
- **50+ cameras**: Dedicated server (16+ cores, 32GB+ RAM)

### Storage Requirements
**Example: 1080p camera @ 4 Mbps**
- Per hour: ~1.8 GB
- Per day: ~43 GB
- 7-day retention: ~301 GB
- 30-day retention: ~1.29 TB

### Timeline Performance
- Optimized for fast date queries
- Efficient segment matching algorithm
- Canvas rendering for smooth timeline visualization
- Debounced API calls to prevent overload

## 🔐 Security Considerations

### Network Isolation
- Runs on isolated Docker networks
- Cameras on separate VLAN recommended
- go2rtc acts as RTSP gateway

### Access Control
- **Port Security**: Bind to localhost if using reverse proxy
- **Authentication**: Add reverse proxy with auth (nginx, Caddy, Traefik)
- **HTTPS**: Terminate TLS at reverse proxy

### Data Protection
- Recordings stored on host filesystem
- No cloud uploads (fully self-hosted)
- Container has limited access to recordings directory

### Download Security
- Downloads are temporary (FFmpeg streams directly)
- No persistent export files on server
- Time-based access control possible via reverse proxy

## 📝 Recent Updates

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

## 🚧 Future Enhancements

### Planned Features
- Motion detection recording mode
- Email/webhook notifications
- Thumbnail generation for timeline
- Export to cloud storage
- Camera groups/locations
- PTZ camera control
- Audio recording support
- Snapshot capture on motion
- Object detection integration

### Under Consideration
- Mobile app (iOS/Android)
- ONVIF camera discovery
- Sub-stream recording for bandwidth savings
- Hardware acceleration (GPU encoding)
- HLS/DASH streaming support
- Event-based recording triggers

## 📄 License

This is a custom-built project. Adjust licensing as needed.

## 🤝 Contributing

For issues, improvements, or questions, please refer to project documentation.

## 📞 Support

- Review logs: `docker logs simplenvr`
- Check recording directory: `/recordings/`
- Verify go2rtc: `http://go2rtc:1984`
- Browser console for frontend errors
- API endpoints for debugging
