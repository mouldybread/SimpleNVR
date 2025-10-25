# SimpleNVR

A lightweight, self-hosted Network Video Recorder (NVR) built in Go with native RTSP recording and MP4 output. SimpleNVR integrates with go2rtc for camera stream management and provides a clean web interface for monitoring, recording, and playback.

## 🎯 Features

### Core Functionality
- **Native Go RTSP Recording** - Pure Go implementation using gortsplib, no ffmpeg dependency
- **MP4 Output** - Browser-compatible video files with H.264 codec
- **10-Minute Segments** - Automatic file splitting for better reliability and management
- **Configurable Retention** - Per-camera retention periods (1-365 days)
- **Live Stream Preview** - View camera feeds in real-time through the web interface
- **Playback System** - Browse and play recordings with a dedicated interface
- **go2rtc Integration** - Auto-discover and add cameras from go2rtc server

### Web Interface
- **Collapsible Sections** - Organized layout with Playback, Cameras, and Configuration
- **State Persistence** - Section collapse/expand state saved in browser localStorage
- **Responsive Design** - Works on desktop and mobile devices
- **Dark Mode Support** - Automatic dark/light theme based on system preferences
- **Real-time Updates** - Camera list refreshes every 5 seconds via HTMX

### Recording Features
- **Continuous Recording** - Start/stop recording per camera
- **Automatic Cleanup** - Old recordings deleted based on retention settings
- **Organized Storage** - Each camera gets its own directory with unique ID
- **Naming Convention** - `YYYYMMDD_HHMMSS_CameraName.mp4` format
- **Auto-reconnect** - Automatically retries on stream errors (5-second delay)

## 📋 Requirements

- Docker and Docker Compose
- go2rtc server (for camera stream management)
- RTSP-compatible cameras (H.264 video codec)

## 🚀 Installation

### 1. Directory Structure

```
/mnt/Central/containers/SimpleNVR/
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
│           └── playback.js
└── recordings/
```

### 2. Docker Compose Configuration

```yaml
networks:
  ha_net:
    external: True
    name: ix-ha_ha_net
  vlan2:
    external: True
    name: vlan2_net

services:
  simplenvr:
    container_name: simplenvr
    image: golang:1.23
    working_dir: /app
    command: >
      sh -c "
      go install github.com/air-verse/air@latest &&
      air
      "
    volumes:
      - /mnt/Central/containers/SimpleNVR/build:/app
      - /mnt/Central/containers/SimpleNVR/recordings:/recordings
    ports:
      - "8099:80"
    networks:
      - ha_net
      - vlan2
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
1. Open the **Configuration** section
2. Enter your go2rtc URL (default: `http://go2rtc:1984`)
3. Click **Scan go2rtc**
4. All streams will be automatically added

#### Method 2: Manual Entry
1. Open the **Configuration** section
2. Enter a camera name
3. Enter the RTSP URL (e.g., `rtsp://camera-ip:554/stream`)
4. Click **Add Camera**

### Recording Management

#### Start Recording
1. Navigate to the **Cameras** section
2. Click **Start Recording** for desired camera
3. Recording begins immediately in 10-minute segments

#### Stop Recording
1. Click **Stop Recording** to halt recording
2. Current segment completes before stopping

#### Set Retention Period
- **Per Camera**: Adjust the retention days and click **Update**
- **All Cameras**: Use **Set All Retention** button for bulk changes

### Viewing Live Streams
1. Click **👁️ View Live** on any camera
2. Stream opens in a modal window
3. Click **✕ Close** or press `Esc` to exit

### Playback
1. Navigate to the **Playback** section (top of page)
2. Select a camera from the dropdown
3. Browse recordings in the left sidebar
4. Click a recording to play in the main player
5. Use **Fullscreen** button to expand view

### Removing Cameras
1. Click **Remove** once to confirm
2. Click **Remove** again within 3 seconds to delete
3. Use **Remove All** for bulk deletion (requires two clicks)

## 📁 File Structure

### Recording Files
- **Location**: `/recordings/{camera_id}/`
- **Format**: `YYYYMMDD_HHMMSS_CameraName.mp4`
- **Example**: `20251025_143022_FrontDoor.mp4`
- **Segment Length**: 10 minutes per file
- **Files per day**: 144 per camera (24 hours × 6 segments/hour)

### Configuration
- **Location**: `/recordings/cameras.json`
- **Contains**: Camera settings, retention periods, recording states
- **Persistence**: Automatically saved on changes

### Application Files
- **main.go**: HTTP server, API handlers, camera management
- **recorder.go**: RTSP recording, MP4 muxing, cleanup routines
- **templates/**: HTML templates (index, camera cards)
- **static/**: CSS and JavaScript modules

## 🔧 Technical Details

### Recording Pipeline
1. **RTSP Connection**: gortsplib client connects to camera
2. **H.264 Decoding**: RTP packets depacketized to NAL units
3. **SPS/PPS Extraction**: Codec parameters parsed
4. **MP4 Muxing**: deepch/vdk writes MP4 container
5. **Segmentation**: New file every 10 minutes
6. **Cleanup**: Old files deleted based on retention settings

### API Endpoints

#### Camera Management
- `GET /api/cameras` - List all cameras (HTML partial)
- `POST /api/cameras/add` - Add new camera
- `POST /api/cameras/delete` - Remove camera
- `POST /api/cameras/delete-all` - Remove all cameras

#### Recording Control
- `POST /api/record/start` - Start recording camera
- `POST /api/record/stop` - Stop recording camera

#### Retention Management
- `POST /api/cameras/update-retention` - Update single camera
- `POST /api/cameras/update-retention-all` - Update all cameras

#### Playback
- `GET /api/recordings/list?camera_id={id}` - List recordings (JSON)
- `GET /recordings/{camera_id}/{filename}` - Serve recording file

#### go2rtc Integration
- `POST /api/go2rtc/scan` - Scan and import streams

#### Live Streaming
- `GET /api/stream/{camera_name}` - Proxy live stream from go2rtc

### Dependencies

#### Go Modules
- `github.com/bluenviron/gortsplib/v4` - RTSP client
- `github.com/bluenviron/mediacommon` - H.264 codec handling
- `github.com/deepch/vdk` - MP4 muxing
- `github.com/pion/rtp` - RTP packet processing

#### Frontend
- **HTMX** (1.9.10) - Dynamic HTML updates
- **Vanilla JavaScript** - No framework dependencies
- **CSS Variables** - Theming and customization

### Browser Compatibility
- **Chrome/Edge**: Full support
- **Firefox**: Full support
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

### Styling
Edit `static/style.css` - Uses CSS variables for easy theming

### UI Layout
- `templates/index.html` - Main page structure
- `templates/cameras.html` - Camera card template
- `static/js/` - Modular JavaScript functionality

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

### Health Checks
- Camera list updates every 5 seconds automatically
- Recording state displayed in real-time
- Auto-reconnect on stream failures

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

### Live Stream Fails
1. Confirm go2rtc is running and accessible
2. Check go2rtc has the camera configured
3. Verify internal Docker network connectivity
4. Review go2rtc logs

### High Disk Usage
1. Reduce retention period
2. Decrease segment duration (fewer files)
3. Stop recording on unused cameras
4. Monitor cleanup routine execution

### Air Not Rebuilding
1. Check `.air.toml` configuration
2. Verify `go.mod` has all dependencies
3. Run `go mod tidy` in container if needed
4. Restart container for fresh build

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

## 🔐 Security Considerations

### Network Isolation
- Runs on isolated Docker networks
- Cameras on separate VLAN recommended
- go2rtc acts as RTSP gateway

### Access Control
- **Port Security**: Bind to localhost only if using reverse proxy
- **Authentication**: Add reverse proxy with auth (nginx, Caddy)
- **HTTPS**: Terminate TLS at reverse proxy

### Data Protection
- Recordings stored on host filesystem
- No cloud uploads (fully self-hosted)
- Container has access to recordings directory

## 🚧 Future Enhancements

### Planned Features
- [ ] Motion detection recording mode
- [ ] Email/webhook notifications
- [ ] Timeline view with thumbnails
- [ ] Multi-camera simultaneous playback
- [ ] Export/download recordings
- [ ] Camera groups/locations
- [ ] PTZ camera control
- [ ] Audio recording support
- [ ] Snapshot capture on motion
- [ ] Object detection integration

### Under Consideration
- [ ] Mobile app (iOS/Android)
- [ ] ONVIF camera discovery
- [ ] Sub-stream recording for bandwidth savings
- [ ] Hardware acceleration (GPU encoding)
- [ ] Cloud backup integration

## 📝 Version History

### Current Version
- MP4 output with native Go recording
- 10-minute segmentation
- Collapsible UI sections with state persistence
- Playback system with fullscreen support
- Per-camera retention settings
- Auto-cleanup routine

## 📄 License

This is a custom-built project. Adjust licensing as needed.

## 🤝 Contributing

For issues, improvements, or questions, please refer to project documentation.

## 📞 Support

- Review logs: `docker logs simplenvr`
- Check recordings: `/mnt/Central/containers/SimpleNVR/recordings/`
- Verify go2rtc: `http://go2rtc:1984`

---

**Built with ❤️ using Go, HTMX, and modern web technologies**