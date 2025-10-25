// Playback functionality

// State
let playbackCameras = [];
let currentPlaybackVideo = null;

// Populate camera dropdown for playback
function populatePlaybackCameras() {
    fetch('/api/cameras')
        .then(response => response.text())
        .then(html => {
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, 'text/html');
            const cameraCards = doc.querySelectorAll('.camera-card');
            
            playbackCameras = [];
            cameraCards.forEach(card => {
                const nameEl = card.querySelector('.camera-name');
                const buttons = card.querySelectorAll('button[onclick*="removeCamera"]');
                if (buttons.length > 0) {
                    const onclickAttr = buttons[0].getAttribute('onclick');
                    const match = onclickAttr.match(/removeCamera\('([^']+)'\)/);
                    if (match && nameEl) {
                        playbackCameras.push({
                            id: match[1],
                            name: nameEl.textContent.trim()
                        });
                    }
                }
            });
            
            // Update dropdown
            const select = document.getElementById('playback-camera-select');
            if (select) {
                select.innerHTML = '<option value="">-- Select a camera --</option>';
                playbackCameras.forEach(cam => {
                    const option = document.createElement('option');
                    option.value = cam.id;
                    option.textContent = cam.name;
                    select.appendChild(option);
                });
            }
        })
        .catch(error => {
            console.error('Error loading cameras for playback:', error);
        });
}

// Load recordings for selected camera
function loadRecordings(cameraId) {
    const recordingsList = document.getElementById('recordings-list');
    
    if (!cameraId) {
        recordingsList.innerHTML = '<p class="placeholder-text">Select a camera to view recordings</p>';
        return;
    }
    
    recordingsList.innerHTML = '<p class="placeholder-text">Loading recordings...</p>';
    
    fetch('/api/recordings/list?camera_id=' + encodeURIComponent(cameraId))
        .then(response => response.json())
        .then(recordings => {
            if (recordings.length === 0) {
                recordingsList.innerHTML = '<p class="placeholder-text">No recordings found</p>';
                return;
            }
            
            recordingsList.innerHTML = '';
            recordings.forEach(recording => {
                const item = document.createElement('div');
                item.className = 'recording-item';
                item.onclick = function() {
                    playRecording(recording.path, item);
                };
                
                const timestamp = new Date(recording.timestamp);
                const timeStr = timestamp.toLocaleTimeString();
                const dateStr = timestamp.toLocaleDateString();
                const sizeStr = formatFileSize(recording.size);
                
                item.innerHTML = `
                    <div class="recording-time">${timeStr}</div>
                    <div class="recording-date">${dateStr}</div>
                    <div class="recording-size">${sizeStr}</div>
                `;
                
                recordingsList.appendChild(item);
            });
        })
        .catch(error => {
            console.error('Error loading recordings:', error);
            recordingsList.innerHTML = '<p class="placeholder-text">Error loading recordings</p>';
        });
}

// Play selected recording
function playRecording(path, itemElement) {
    const container = document.getElementById('playback-video-container');
    
    // Remove active class from all items
    document.querySelectorAll('.recording-item').forEach(item => {
        item.classList.remove('active');
    });
    
    // Add active class to selected item
    if (itemElement) {
        itemElement.classList.add('active');
    }
    
    // Stop current video if playing
    if (currentPlaybackVideo) {
        currentPlaybackVideo.pause();
        currentPlaybackVideo.src = '';
    }
    
    // Create new video element
    const video = document.createElement('video');
    video.controls = true;
    video.autoplay = false;
    video.src = path;
    
    video.onerror = function() {
        container.innerHTML = '<p class="placeholder-text" style="color: #f44336;">Failed to load recording</p>';
    };
    
    container.innerHTML = '';
    container.appendChild(video);
    currentPlaybackVideo = video;
}

// Toggle fullscreen mode
function toggleFullscreen() {
    const container = document.querySelector('.playback-container');
    const icon = document.getElementById('fullscreen-icon');
    
    if (container.classList.contains('fullscreen')) {
        container.classList.remove('fullscreen');
        icon.textContent = '⛶';
    } else {
        container.classList.add('fullscreen');
        icon.textContent = '⛉';
    }
}
