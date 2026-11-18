// Modal management

// Live Stream Modal
let currentVideo = null;

function viewLiveStream(cameraName) {
    let streamPath;
    try {
        if (typeof cameraName !== 'string' || cameraName.length === 0 || cameraName === '.' || cameraName === '..') throw new Error('Unsupported camera name');
        streamPath = '/api/stream/' + encodeURIComponent(cameraName);
    } catch (_) {
        showError('Camera name cannot be used for live playback; review the camera configuration.');
        return;
    }
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
    currentVideo.src = streamPath;
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
        showError('Please enter a valid number of days (minimum 1)');
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
        showSuccess('Retention period updated');
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        showError('Failed to update retention period: ' + error.message);
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
        showError('Please select a recording mode');
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
        showSuccess('Recording mode updated');
    })
    .catch(error => {
        console.error('Error updating recording mode:', error);
        showError('Failed to update recording mode: ' + error.message);
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
            
            // CRITICAL: Initialize drag and drop after cameras are loaded
            setTimeout(() => {
                if (typeof initializeDragAndDrop === 'function') {
                    initializeDragAndDrop();
                } else {
                    console.error('initializeDragAndDrop function not found!');
                }
            }, 100);
        })
        .catch(error => {
            console.error('Failed to load cameras:', error);
            showError('Failed to load cameras');
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
                    <h3>Bulk Operations</h3>
                    <div class="bulk-ops" style="display: flex; justify-content: space-between; align-items: center; gap: 10px;">
                        <div class="form-group" style="display: flex; align-items: center; gap: 10px; margin: 0;">
                            <label for="modal-retention-all" style="margin: 0; white-space: nowrap;">Retention period (days):</label>
                            <input type="number" id="modal-retention-all" min="1" max="365" value="7" style="width: 80px;">
                            <button onclick="setAllRetentionFromModal()" class="btn">Set Retention</button>
                        </div>
                        <div style="display: flex; gap: 10px;">
                            <button onclick="openDeleteAllCamerasModal()" class="btn btn-danger">Remove All Cameras</button>
                            <button onclick="openDeleteAllRecordingsModal()" class="btn btn-danger">Delete All Recordings</button>
                            <button onclick="openClearEventsModal()" class="btn btn-danger">Clear All Events</button>
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
            .then(response => {
                if (!response.ok) throw new Error('Failed to add camera');
                return response.text();
            })
            .then(html => {
                // Update BOTH lists
                document.getElementById('modal-camera-list').innerHTML = html;
                const mainList = document.getElementById('camera-list');
                if (mainList) {
                    mainList.innerHTML = html;
                }
                addForm.reset();
                showSuccess('Camera added successfully');
                
                // Re-initialize drag and drop
                setTimeout(() => {
                    if (typeof initializeDragAndDrop === 'function') {
                        initializeDragAndDrop();
                    }
                }, 50);
            })
            .catch(error => {
                console.error('Error adding camera:', error);
                showError('Failed to add camera');
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
            .then(async response => {
                if (!response.ok) {
                    const errorMsg = await response.text();
                    throw new Error(errorMsg || 'Failed to scan go2rtc');
                }
                return response.text();
            })
            .then(html => {
                // Update BOTH lists
                document.getElementById('modal-camera-list').innerHTML = html;
                const mainList = document.getElementById('camera-list');
                if (mainList) {
                    mainList.innerHTML = html;
                }
                showSuccess('Go2RTC streams imported successfully');
                
                // Re-initialize drag and drop
                setTimeout(() => {
                    if (typeof initializeDragAndDrop === 'function') {
                        initializeDragAndDrop();
                    }
                }, 50);
            })
            .catch(error => {
                console.error('Error scanning go2rtc:', error);
                showError(error.message);
            });
        };
    }
}

function setAllRetentionFromModal() {
    const days = parseInt(document.getElementById('modal-retention-all').value);
    if (isNaN(days) || days < 1) {
        showError('Please enter a valid number of days (minimum 1)');
        return;
    }
    
    fetch('/api/cameras/update-retention-all', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ retention_days: days })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to update retention');
        return response.text();
    })
    .then(html => {
        document.getElementById('modal-camera-list').innerHTML = html;
        showSuccess('Retention period updated for all cameras');
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        showError('Failed to update retention');
    });
}

// Clear All Events: per-attempt UI confirmation, not server authorization.
let clearEventsCode = '';
let clearEventsConfirmCount = 0;
let clearEventsPending = false;
let clearEventsVersion = 0;
let clearEventsTimer = null;
let clearEventsDeadline = 0;

function disarmEventClear() {
    clearEventsVersion++;
    if (clearEventsTimer !== null) clearTimeout(clearEventsTimer);
    clearEventsTimer = null;
    clearEventsDeadline = 0;
    clearEventsConfirmCount = 0;
}
function resetEventClearConfirmation() {
    disarmEventClear();
    clearEventsCode = '';
    const input = document.getElementById('clear-events-code-input');
    if (input) { input.value = ''; input.disabled = clearEventsPending; }
    const code = document.getElementById('clear-events-code');
    if (code) code.textContent = 'Reopen this dialog to confirm another attempt';
    const btn = document.getElementById('clear-events-btn');
    if (btn) { btn.disabled = true; btn.textContent = 'Clear All Events'; btn.classList.remove('confirm'); }
}
function eventClearCodeValid() {
    const modal = document.getElementById('clear-events-modal');
    const input = document.getElementById('clear-events-code-input');
    const code = document.getElementById('clear-events-code');
    return !clearEventsPending && /^[0-9]{4}$/.test(clearEventsCode) &&
        modal?.classList.contains('active') && input?.value === clearEventsCode &&
        code?.textContent === clearEventsCode && !!document.getElementById('clear-events-btn');
}
function updateEventClearInput() {
    disarmEventClear();
    const btn = document.getElementById('clear-events-btn');
    if (!btn) return;
    const valid = eventClearCodeValid();
    btn.disabled = !valid;
    btn.textContent = clearEventsPending ? 'Clearing...' : (valid ? 'Click to Confirm' : 'Clear All Events');
    btn.classList.remove('confirm');
}
function openClearEventsModal() {
    if (clearEventsPending) {
        showWarning('An event-clear request is still pending. No additional request was started.');
        return;
    }
    resetEventClearConfirmation();
    const modal = document.getElementById('clear-events-modal');
    const input = document.getElementById('clear-events-code-input');
    const code = document.getElementById('clear-events-code');
    if (!modal || !input || !code || !document.getElementById('clear-events-btn')) return;
    clearEventsCode = Math.floor(1000 + Math.random() * 9000).toString();
    code.textContent = clearEventsCode;
    modal.classList.add('active'); modal.style.display = 'block';
    const version = clearEventsVersion;
    setTimeout(() => {
        if (version === clearEventsVersion && !clearEventsPending && modal.classList.contains('active')) input.focus();
    }, 100);
}
function closeClearEventsModal() {
    const modal = document.getElementById('clear-events-modal');
    if (modal) { modal.classList.remove('active'); modal.style.display = 'none'; }
    resetEventClearConfirmation();
}
function eventClearOutcome(data) {
    if (!data || Array.isArray(data) || typeof data !== 'object' || typeof data.success !== 'boolean') throw new Error('Invalid event-clear response');
    if (!data.success) return {
        confirmed:false,
        message:'Server did not confirm a successful event-metadata clear. Metadata may already have changed; no cleared count is confirmed. Review state before reopening this dialog for fresh confirmation.'
    };
    if (!Number.isSafeInteger(data.count) || data.count < 0) throw new Error('Invalid event-clear count');
    return {confirmed:true,message:`Server reported ${data.count} event record(s) cleared. This clears event metadata, not recording files.`};
}
function eventClearRefreshWarning(error) {
    console.error('Event-clear response received, but display refresh failed:', error);
    showWarning('Event-clear response received, but the event display could not refresh. Reload to review current metadata.');
}
async function clearAllEvents() {
    if (clearEventsPending) return;
    if (!eventClearCodeValid()) { updateEventClearInput(); return; }
    const btn = document.getElementById('clear-events-btn');
    if (clearEventsConfirmCount !== 1 || Date.now() >= clearEventsDeadline) {
        disarmEventClear();
        clearEventsConfirmCount = 1;
        clearEventsDeadline = Date.now() + 3000;
        const version = clearEventsVersion;
        btn.disabled = false; btn.textContent = 'Click Again to Confirm'; btn.classList.add('confirm');
        clearEventsTimer = setTimeout(() => {
            if (version === clearEventsVersion && !clearEventsPending) updateEventClearInput();
        }, 3000);
        return;
    }
    clearEventsPending = true;
    resetEventClearConfirmation();
    btn.textContent = 'Clearing...';
    let responseStatus = null, responseReceived = false;
    try {
        const response = await fetch('/api/events/clear-all', {method:'POST',redirect:'error'});
        responseStatus = response.status;
        if (!response.ok || response.status !== 200 || response.redirected) throw new Error('Unexpected event-clear response');
        const contentType = (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase();
        if (contentType !== 'application/json') throw new Error('Expected event-clear JSON');
        const outcome = eventClearOutcome(await response.json());
        responseReceived = true;
        if (!outcome.confirmed) { showWarning(outcome.message); return; }
        showSuccess(outcome.message);
        closeClearEventsModal();
        if (typeof refreshEventsList === 'function') Promise.resolve(refreshEventsList()).catch(eventClearRefreshWarning);
    } catch (error) {
        if (responseReceived) eventClearRefreshWarning(error);
        else {
            console.error('Unable to verify event-clear outcome:', error);
            const status = Number.isInteger(responseStatus) ? ` (HTTP ${responseStatus})` : '';
            showError(`Event-clear outcome could not be verified${status}. Event metadata may already have changed; completion and persistence are not verified and no cleared count is confirmed. Review server state before reopening this dialog for fresh confirmation. No automatic retry was made.`);
        }
    } finally {
        clearEventsPending = false;
        resetEventClearConfirmation();
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const input = document.getElementById('clear-events-code-input');
    if (input) input.oninput = updateEventClearInput;
    const modal = document.getElementById('clear-events-modal');
    if (modal) modal.onclick = e => { if (e.target === modal) closeClearEventsModal(); };
});

// Close modals when clicking outside
window.onclick = function(event) {
    const liveModal = document.getElementById('liveModal');
    if (event.target === liveModal) {
        closeLiveModal();
    }
};
