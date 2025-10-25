// Modal management

// Live Stream Modal
let currentVideo = null;

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

// Close modal when clicking outside
document.getElementById('liveModal')?.addEventListener('click', function(e) {
    if (e.target === this) {
        closeLiveStream();
    }
});

// Set All Retention Modal
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
        alert('Please enter a valid number of days (minimum 1)');
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
    })
    .catch(error => {
        console.error('Error updating retention for all cameras:', error);
        alert('Failed to update retention period');
    });
}

// Close modal when clicking outside
document.getElementById('setAllRetentionModal')?.addEventListener('click', function(e) {
    if (e.target === this) {
        closeSetAllRetentionModal();
    }
});
