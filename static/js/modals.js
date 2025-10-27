// Modal management

// Live Stream Modal
let currentVideo = null;

function viewLiveStream(cameraName) {
    let modal = document.getElementById('liveModal');
    
    // Create modal if it doesn't exist
    if (!modal) {
        createLiveModal();
        modal = document.getElementById('liveModal');
    }
    
    const title = document.getElementById('modalTitle');
    const container = document.getElementById('streamContainer');
    
    if (title) {
        title.textContent = 'Live Feed: ' + cameraName;
    }
    
    modal.classList.add('active');
    modal.style.display = 'block';
    
    container.innerHTML = '<video id="liveVideo" controls autoplay muted style="width: 100%; max-height: 70vh;"></video>';
    
    currentVideo = document.getElementById('liveVideo');
    currentVideo.src = '/api/stream/' + cameraName;
}

function createLiveModal() {
    const modalHtml = `
        <div id="liveModal" class="modal">
            <div class="modal-content" style="width: fit-content !important; max-width: 1280px !important; height: auto !important; max-height: 90vh !important; padding: 20px;">
                <span class="close" onclick="closeLiveModal()">&times;</span>
                <h2 id="modalTitle">Live Feed</h2>
                <div id="streamContainer"></div>
            </div>
        </div>
    `;
    
    document.body.insertAdjacentHTML('beforeend', modalHtml);
    
    // Close modal when clicking outside
    const modal = document.getElementById('liveModal');
    modal.onclick = (e) => {
        if (e.target === modal) {
            closeLiveModal();
        }
    };
}

function closeLiveModal() {
    const modal = document.getElementById('liveModal');
    if (!modal) return;
    
    modal.classList.remove('active');
    modal.style.display = 'none';
    
    if (currentVideo) {
        currentVideo.pause();
        currentVideo.src = '';
        currentVideo = null;
    }
}

// Retention Modal (now per-camera)
function showRetentionModal(cameraId, currentRetention) {
    const modal = document.getElementById('retention-modal');
    if (!modal) return;
    
    document.getElementById('retention-camera-id').value = cameraId;
    document.getElementById('retention-days-input').value = currentRetention;
    modal.classList.add('active');
    modal.style.display = 'block';
}

function closeRetentionModal() {
    const modal = document.getElementById('retention-modal');
    if (!modal) return;
    
    modal.classList.remove('active');
    modal.style.display = 'none';
}

function updateRetentionModal() {
    const cameraId = document.getElementById('retention-camera-id').value;
    const days = parseInt(document.getElementById('retention-days-input').value);
    
    if (isNaN(days) || days < 1) {
        alert('Please enter a valid number of days (minimum 1)');
        return;
    }
    
    fetch('/api/cameras/update-retention', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            id: cameraId,
            retention_days: days
        })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to update retention');
        return response.text();
    })
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        closeRetentionModal();
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        alert('Failed to update retention period: ' + error.message);
    });
}

// Recording Mode Modal
function showModeModal(cameraId, currentMode) {
    const modal = document.getElementById('mode-modal');
    if (!modal) {
        createModeModal();
        setTimeout(() => showModeModal(cameraId, currentMode), 100);
        return;
    }

    document.getElementById('mode-camera-id').value = cameraId;
    
    // Set radio button selection
    const radios = document.getElementsByName('recording-mode');
    for (let radio of radios) {
        radio.checked = (radio.value === currentMode);
    }

    modal.classList.add('active');
    modal.style.display = 'block';
}

function closeModeModal() {
    const modal = document.getElementById('mode-modal');
    if (!modal) return;
    
    modal.classList.remove('active');
    modal.style.display = 'none';
}

function updateRecordingMode() {
    const cameraId = document.getElementById('mode-camera-id').value;
    const radios = document.getElementsByName('recording-mode');
    let selectedMode = '';
    
    for (let radio of radios) {
        if (radio.checked) {
            selectedMode = radio.value;
            break;
        }
    }

    if (!selectedMode) {
        alert('Please select a recording mode');
        return;
    }

    fetch('/api/cameras/update-mode', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            id: cameraId,
            mode: selectedMode
        })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to update recording mode');
        return response.text();
    })
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        // Also update modal camera list if modal is open
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        closeModeModal();
    })
    .catch(error => {
        console.error('Error updating recording mode:', error);
        alert('Failed to update recording mode: ' + error.message);
    });
}

function createModeModal() {
    const modalHtml = `
        <div id="mode-modal" class="modal">
            <div class="modal-content" style="width: fit-content !important; max-width: 600px !important; height: auto !important; padding: 20px;">
                <span class="close" onclick="closeModeModal()">&times;</span>
                <h2>Change Recording Mode</h2>
                <input type="hidden" id="mode-camera-id">
                <div style="margin: 20px 0;">
                    <label style="display: block; margin: 10px 0; cursor: pointer;">
                        <input type="radio" name="recording-mode" value="continuous">
                        <strong>Continuous Recording</strong> - Records 10-minute segments continuously
                    </label>
                    <label style="display: block; margin: 10px 0; cursor: pointer;">
                        <input type="radio" name="recording-mode" value="event_based">
                        <strong>Event Based Recording</strong> - Maintains 30-second buffer, records on external event trigger
                    </label>
                    <label style="display: block; margin: 10px 0; cursor: pointer;">
                        <input type="radio" name="recording-mode" value="no_recording">
                        <strong>No Recording</strong> - Camera does not record
                    </label>
                </div>
                <div style="margin-top: 20px;">
                    <button onclick="updateRecordingMode()" class="btn btn-primary">Update Mode</button>
                    <button onclick="closeModeModal()" class="btn btn-secondary">Cancel</button>
                </div>
            </div>
        </div>
    `;
    
    document.body.insertAdjacentHTML('beforeend', modalHtml);
    
    // Close modal when clicking outside
    const modal = document.getElementById('mode-modal');
    modal.onclick = (e) => {
        if (e.target === modal) {
            closeModeModal();
        }
    };
}

// Camera Settings Modal
function openCameraSettings() {
    let modal = document.getElementById('camera-settings-modal');
    
    if (!modal) {
        createCameraSettingsModal();
        modal = document.getElementById('camera-settings-modal');
    }
    
    // Load camera list into modal
    fetch('/api/cameras')
        .then(response => response.text())
        .then(html => {
            document.getElementById('modal-camera-list').innerHTML = html;
        })
        .catch(error => {
            console.error('Failed to load cameras:', error);
        });
    
    modal.classList.add('active');
    modal.style.display = 'block';
}

function closeCameraSettings() {
    const modal = document.getElementById('camera-settings-modal');
    if (!modal) return;
    
    modal.classList.remove('active');
    modal.style.display = 'none';
}

function createCameraSettingsModal() {
    const modalHtml = `
        <div id="camera-settings-modal" class="modal">
            <div class="modal-content" style="width: 90vw !important; max-width: 1200px !important; height: 90vh !important; max-height: 90vh !important; padding: 20px; overflow-y: auto;">
                <span class="close" onclick="closeCameraSettings()">&times;</span>
                
                
                <div class="config-subsection">
                    <h3>🔗 Go2RTC Integration</h3>
                    <form id="modal-go2rtc-form" class="form" style="display: flex; align-items: center; gap: 10px;">
                        <label for="modal-go2rtc-url" style="margin: 0; white-space: nowrap;">Go2RTC URL:</label>
                        <input type="text" id="modal-go2rtc-url" name="go2rtc-url" value="http://go2rtc:1984" style="flex: 1;">
                        <button type="submit" class="btn btn-primary">Scan & Import Streams</button>
                    </form>
                </div>

                <div class="config-subsection">
                    <h3>➕ Add Camera Manually</h3>
                    <form id="modal-add-camera-form" class="form" style="display: flex; align-items: center; gap: 10px;">
                        <label for="modal-camera-name" style="margin: 0; white-space: nowrap;">Camera Name:</label>
                        <input type="text" id="modal-camera-name" name="camera-name" required style="width: 200px;">
                        <label for="modal-rtsp-url" style="margin: 0; white-space: nowrap;">RTSP URL:</label>
                        <input type="text" id="modal-rtsp-url" name="rtsp-url" placeholder="rtsp://..." required style="flex: 1;">
                        <button type="submit" class="btn btn-primary">Add Camera</button>
                    </form>
                </div>

                <div class="config-subsection">
                    <h3>⚙️ Bulk Operations</h3>
                    <div class="bulk-ops" style="display: flex; justify-content: space-between; align-items: center; gap: 10px;">
                        <div class="form-group" style="display: flex; align-items: center; gap: 10px; margin: 0;">
                            <label for="modal-retention-all" style="margin: 0; white-space: nowrap;">Retention period for all cameras (days):</label>
                            <input type="number" id="modal-retention-all" min="1" max="365" value="7" style="width: 80px;">
                            <button onclick="setAllRetentionFromModal()" class="btn">Set Retention</button>
                        </div>
                        <div style="display: flex; gap: 10px;">
                            <button onclick="openDeleteAllCamerasModal()" class="btn btn-danger">Remove All Cameras</button>
                            <button onclick="openDeleteAllRecordingsModal()" class="btn btn-danger">Delete All Recordings</button>
                            <button onclick="cleanupOrphanedRecordings()" class="btn btn-danger">Cleanup Orphaned Recordings</button>
                        </div>
                    </div>
                </div>
                
                <!-- Cleanup Results Display -->
                <div id="modal-cleanup-results" style="display: none; background: #1f1f1f; padding: 15px; margin: 15px 0; border-radius: 8px; border-left: 4px solid #4CAF50;">
                    <h4 style="margin: 0 0 10px 0; color: #4CAF50;">✓ Cleanup Complete</h4>
                    <p id="modal-cleanup-results-text" style="margin: 0; color: #ccc;"></p>
                </div>

                <div class="config-subsection">
                    <h3>📹 Your Cameras</h3>
                    <div id="modal-camera-list"></div>
                </div>
            </div>
        </div>
    `;
    
    document.body.insertAdjacentHTML('beforeend', modalHtml);
    
    // Setup form handlers
    setupModalCameraForms();
    
    // Close modal when clicking outside
    const modal = document.getElementById('camera-settings-modal');
    modal.onclick = (e) => {
        if (e.target === modal) {
            closeCameraSettings();
        }
    };
}

function setupModalCameraForms() {
    const addForm = document.getElementById('modal-add-camera-form');
    if (addForm) {
        addForm.onsubmit = (e) => {
            e.preventDefault();
            const formData = new FormData(addForm);
            
            fetch('/api/cameras/add', {
                method: 'POST',
                body: formData
            })
            .then(response => response.text())
            .then(html => {
                document.getElementById('modal-camera-list').innerHTML = html;
                addForm.reset();
            })
            .catch(error => {
                console.error('Error adding camera:', error);
                alert('Failed to add camera');
            });
        };
    }
    
    const scanForm = document.getElementById('modal-go2rtc-form');
    if (scanForm) {
        scanForm.onsubmit = (e) => {
            e.preventDefault();
            const formData = new FormData(scanForm);
            
            fetch('/api/go2rtc/scan', {
                method: 'POST',
                body: formData
            })
            .then(response => response.text())
            .then(html => {
                document.getElementById('modal-camera-list').innerHTML = html;
            })
            .catch(error => {
                console.error('Error scanning go2rtc:', error);
                alert('Failed to scan go2rtc');
            });
        };
    }
}

function setAllRetentionFromModal() {
    const days = parseInt(document.getElementById('modal-retention-all').value);
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
        document.getElementById('modal-camera-list').innerHTML = html;
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        alert('Failed to update retention');
    });
}

// Close modals when clicking outside
window.onclick = function(event) {
    const liveModal = document.getElementById('liveModal');
    if (event.target === liveModal) {
        closeLiveModal();
    }
};
