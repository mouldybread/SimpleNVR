// Camera management functionality

// State
let removeConfirmTimers = {};
let removeAllConfirmed = false;
let removeAllTimer = null;

// Update retention for single camera
function updateRetention(cameraId) {
    const input = document.getElementById('retention-' + cameraId);
    const days = parseInt(input.value);
    
    if (isNaN(days) || days < 1) {
        alert('Please enter a valid number of days (minimum 1)');
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
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        alert('Failed to update retention period');
    });
}

// Remove single camera with confirmation
function removeCamera(cameraId) {
    const button = event.target;
    
    if (removeConfirmTimers[cameraId]) {
        // Second press - actually delete
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
            if (typeof populatePlaybackCameras === 'function') {
                populatePlaybackCameras();
            }
        })
        .catch(error => {
            console.error('Error removing camera:', error);
            alert('Failed to remove camera');
        });
    } else {
        // First press - ask for confirmation
        button.textContent = 'Click Again to Confirm';
        button.classList.add('confirm');
        
        removeConfirmTimers[cameraId] = setTimeout(() => {
            button.textContent = 'Remove';
            button.classList.remove('confirm');
            delete removeConfirmTimers[cameraId];
        }, 3000);
    }
}

// Remove all cameras with confirmation
function confirmRemoveAll() {
    const button = document.getElementById('remove-all-btn');
    
    if (removeAllConfirmed) {
        // Second press - actually delete all
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
            if (typeof populatePlaybackCameras === 'function') {
                populatePlaybackCameras();
            }
        })
        .catch(error => {
            console.error('Error removing all cameras:', error);
            alert('Failed to remove all cameras');
        });
    } else {
        // First press - ask for confirmation
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
