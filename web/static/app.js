// Collapsible section toggle
function toggleSection(sectionId) {
    const content = document.getElementById(sectionId);
    const icon = document.getElementById(sectionId + '-icon');
    
    if (content.classList.contains('collapsed')) {
        content.classList.remove('collapsed');
        icon.classList.remove('collapsed');
        content.style.maxHeight = content.scrollHeight + 'px';
    } else {
        content.classList.add('collapsed');
        icon.classList.add('collapsed');
        content.style.maxHeight = '0';
    }
}

// Update section height after content changes
function updateSectionHeight(sectionId) {
    const content = document.getElementById(sectionId);
    if (content && !content.classList.contains('collapsed')) {
        // Use setTimeout to ensure DOM has updated
        setTimeout(function() {
            content.style.maxHeight = content.scrollHeight + 'px';
        }, 50);
    }
}

// Initialize sections as expanded on load
window.addEventListener('DOMContentLoaded', function() {
    const sections = ['config-section', 'cameras-section', 'playback-section'];
    sections.forEach(function(sectionId) {
        const content = document.getElementById(sectionId);
        if (content && !content.classList.contains('collapsed')) {
            // Set initial maxHeight
            content.style.maxHeight = 'none';
            
            // After a brief delay, calculate proper height
            setTimeout(function() {
                content.style.maxHeight = content.scrollHeight + 'px';
            }, 100);
        }
    });
    
    // Also update after htmx settles
    setTimeout(function() {
        updateSectionHeight('cameras-section');
        populatePlaybackCameras();
    }, 1000);
});

// Listen for htmx events to update section heights
document.body.addEventListener('htmx:afterSwap', function(event) {
    if (event.detail.target.id === 'camera-list') {
        updateSectionHeight('cameras-section');
    }
});

let currentVideo = null;
let removeConfirmTimers = {};
let removeAllConfirmed = false;
let removeAllTimer = null;

function viewLiveStream(cameraName) {
    const modal = document.getElementById('liveModal');
    const title = document.getElementById('modalTitle');
    const container = document.getElementById('streamContainer');
    
    title.textContent = 'Live Feed: ' + cameraName;
    modal.classList.add('active');
    container.innerHTML = '<div class="stream-loading">Connecting to stream...</div>';
    
    const video = document.createElement('video');
    video.controls = true;
    video.autoplay = true;
    video.muted = false;
    video.playsInline = true;
    video.src = '/api/stream/' + encodeURIComponent(cameraName);
    
    video.onerror = function(e) {
        container.innerHTML = '<div class="stream-error">❌ Failed to load stream. Check that the camera is configured in go2rtc.</div>';
        console.error('Video error:', e);
    };
    
    video.onloadeddata = function() {
        console.log('Stream loaded successfully');
    };
    
    container.innerHTML = '';
    container.appendChild(video);
    currentVideo = video;
}

function closeLiveStream() {
    const modal = document.getElementById('liveModal');
    const container = document.getElementById('streamContainer');
    
    if (currentVideo) {
        currentVideo.pause();
        currentVideo.src = '';
        currentVideo = null;
    }
    
    modal.classList.remove('active');
    container.innerHTML = '<div class="stream-loading">Loading stream...</div>';
}

function openSetAllRetentionModal() {
    document.getElementById('setAllRetentionModal').classList.add('active');
}

function closeSetAllRetentionModal() {
    document.getElementById('setAllRetentionModal').classList.remove('active');
}

function applyRetentionToAll() {
    const input = document.getElementById('all-retention-days');
    const days = parseInt(input.value);
    
    if (isNaN(days) || days < 1) {
        showError('Please enter a valid number of days (minimum 1)');
        return;
    }
    
    fetch('/api/cameras/update-retention-all', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ retention_days: days })
    })
    .then(response => response.text())
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        updateSectionHeight('cameras-section');
        closeSetAllRetentionModal();
        showSuccess('Retention period updated for all cameras');
    })
    .catch(error => {
        console.error('Error updating retention for all cameras:', error);
        showError('Failed to update retention period');
    });
}

function updateRetention(cameraId) {
    const input = document.getElementById('retention-' + cameraId);
    const days = parseInt(input.value);
    
    if (isNaN(days) || days < 1) {
        showError('Please enter a valid number of days (minimum 1)');
        return;
    }
    
    fetch('/api/cameras/update-retention', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: cameraId, retention_days: days })
    })
    .then(response => response.text())
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        updateSectionHeight('cameras-section');
        showSuccess('Retention period updated');
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        showError('Failed to update retention period');
    });
}

function removeCamera(cameraId) {
    const button = event.target;
    
    if (removeConfirmTimers[cameraId]) {
        clearTimeout(removeConfirmTimers[cameraId]);
        delete removeConfirmTimers[cameraId];
        button.textContent = 'Remove';
        button.classList.remove('confirm');
        
        fetch('/api/cameras/delete', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: cameraId })
        })
        .then(response => response.text())
        .then(html => {
            document.getElementById('camera-list').innerHTML = html;
            updateSectionHeight('cameras-section');
            populatePlaybackCameras();
            showSuccess('Camera removed successfully');
        })
        .catch(error => {
            console.error('Error removing camera:', error);
            showError('Failed to remove camera');
        });
    } else {
        button.textContent = 'Click Again to Confirm';
        button.classList.add('confirm');
        
        removeConfirmTimers[cameraId] = setTimeout(() => {
            button.textContent = 'Remove';
            button.classList.remove('confirm');
            delete removeConfirmTimers[cameraId];
        }, 3000);
    }
}

function confirmRemoveAll() {
    const button = document.getElementById('remove-all-btn');
    
    if (removeAllConfirmed) {
        clearTimeout(removeAllTimer);
        removeAllConfirmed = false;
        button.textContent = 'Remove All';
        button.classList.remove('confirm');
        
        fetch('/api/cameras/delete-all', {
            method: 'POST'
        })
        .then(response => response.text())
        .then(html => {
            document.getElementById('camera-list').innerHTML = html;
            updateSectionHeight('cameras-section');
            populatePlaybackCameras();
            showSuccess('All cameras removed successfully');
        })
        .catch(error => {
            console.error('Error removing all cameras:', error);
            showError('Failed to remove all cameras');
        });
    } else {
        removeAllConfirmed = true;
        button.textContent = 'Click Again to Confirm';
        button.classList.add('confirm');
        
        removeAllTimer = setTimeout(() => {
            removeAllConfirmed = false;
            button.textContent = 'Remove All';
            button.classList.remove('confirm');
        }, 3000);
    }
}

document.getElementById('liveModal').addEventListener('click', function(e) {
    if (e.target === this) {
        closeLiveStream();
    }
});

document.getElementById('setAllRetentionModal').addEventListener('click', function(e) {
    if (e.target === this) {
        closeSetAllRetentionModal();
    }
});

document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        closeLiveStream();
        closeSetAllRetentionModal();
    }
});

// Playback functionality
let playbackCameras = [];
let currentPlaybackVideo = null;

// Populate camera dropdown for playback
function populatePlaybackCameras() {
    fetch('/api/cameras')
        .then(response => response.text())
        .then(html => {
            // Parse camera data from the HTML response
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, 'text/html');
            const cameraCards = doc.querySelectorAll('.camera-card');
            
            playbackCameras = [];
            cameraCards.forEach(card => {
                const nameEl = card.querySelector('.camera-name');
                // Extract camera ID from one of the buttons
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
            select.innerHTML = '<option value="">-- Select a camera --</option>';
            playbackCameras.forEach(cam => {
                const option = document.createElement('option');
                option.value = cam.id;
                option.textContent = cam.name;
                select.appendChild(option);
            });
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

// Format file size
function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}
