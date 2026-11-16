// Camera management functionality

// State
let removeConfirmTimers = {};
let orphanedCleanupConfirmed = false;
let orphanedCleanupTimer = null;
let orphanedCleanupButton = null;
let orphanedCleanupPending = false;
let orphanedCleanupDeadline = 0;
let orphanedCleanupVersion = 0;
let deleteCamerasCode = '';
let deleteCamerasConfirmCount = 0;

// Load cameras on page load
document.addEventListener('DOMContentLoaded', () => {
    loadCameraList();
    setupForms();
    setupDeleteCamerasModal();
});

// Load cameras into the list
function loadCameraList() {
    fetch('/api/cameras')
        .then(response => {
            if (!response.ok) {
                throw new Error('Failed to load cameras');
            }
            return response.text();
        })
        .then(html => {
            const cameraList = document.getElementById('camera-list');
            if (cameraList) {
                cameraList.innerHTML = html;
                if (typeof updateSectionHeight === 'function') {
                    updateSectionHeight('cameras-content');
                }
            }
        })
        .catch(error => {
            console.error('Error loading cameras:', error);
            const cameraList = document.getElementById('camera-list');
            if (cameraList) {
                cameraList.innerHTML = '<p style="color: #f44336;">Failed to load cameras. Please refresh.</p>';
            }
        });
}

// Setup form handlers
function setupForms() {
    const addForm = document.getElementById('add-camera-form');
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
                document.getElementById('camera-list').innerHTML = html;
                if (typeof updateSectionHeight === 'function') {
                    updateSectionHeight('cameras-content');
                }
                
                // Reload timeline camera selector
                if (typeof loadTimelineCameras === 'function') {
                    loadTimelineCameras();
                }
                
                addForm.reset();
                showSuccess('Camera added successfully');
            })
            .catch(error => {
                console.error('Error adding camera:', error);
                showError('Failed to add camera: ' + error.message);
            });
        };
    }
    
    const go2rtcForm = document.getElementById('go2rtc-form');
    if (go2rtcForm) {
        go2rtcForm.onsubmit = (e) => {
            e.preventDefault();
            const formData = new FormData(go2rtcForm);
            
            fetch('/api/go2rtc/scan', {
                method: 'POST',
                body: formData
            })
            .then(response => {
                if (!response.ok) throw new Error('Failed to scan go2rtc');
                return response.text();
            })
            .then(html => {
                document.getElementById('camera-list').innerHTML = html;
                if (typeof updateSectionHeight === 'function') {
                    updateSectionHeight('cameras-content');
                }
                
                // Reload timeline camera selector
                if (typeof loadTimelineCameras === 'function') {
                    loadTimelineCameras();
                }
                
                showSuccess('Go2RTC streams imported successfully');
            })
            .catch(error => {
                console.error('Error scanning go2rtc:', error);
                showError('Failed to scan go2rtc streams: ' + error.message);
            });
        };
    }
}


// Setup delete all cameras modal
function setupDeleteCamerasModal() {
    const input = document.getElementById('delete-cameras-code-input');
    if (input) {
        input.oninput = () => {
            const btn = document.getElementById('delete-cameras-btn');
            if (input.value === deleteCamerasCode) {
                btn.disabled = false;
                btn.textContent = 'Click to Confirm';
                btn.classList.remove('confirm');
                deleteCamerasConfirmCount = 0;
            } else {
                btn.disabled = true;
                btn.textContent = 'Delete All Cameras';
            }
        };
    }
    
    const modal = document.getElementById('delete-cameras-modal');
    if (modal) {
        modal.onclick = (e) => {
            if (e.target === modal) {
                closeDeleteAllCamerasModal();
            }
        };
    }
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
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        showSuccess('Retention period updated');
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        showError('Failed to update retention period: ' + error.message);
    });
}

// Start recording for a camera
function startRecording(cameraId) {
    fetch('/api/record/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: cameraId })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to start recording');
        return response.text();
    })
    .then(html => {
        const cameraList = document.getElementById('camera-list');
        if (cameraList) {
            cameraList.innerHTML = html;
        }
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        showSuccess('Recording started');
    })
    .catch(error => {
        console.error('Error starting recording:', error);
        showError('Failed to start recording: ' + error.message);
    });
}

// Stop recording for a camera
function stopRecording(cameraId) {
    fetch('/api/record/stop', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: cameraId })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to stop recording');
        return response.text();
    })
    .then(html => {
        const cameraList = document.getElementById('camera-list');
        if (cameraList) {
            cameraList.innerHTML = html;
        }
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        showSuccess('Recording stopped');
    })
    .catch(error => {
        console.error('Error stopping recording:', error);
        showError('Failed to stop recording: ' + error.message);
    });
}

function setAllRetention() {
    const input = document.getElementById('retention-all');
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
    .then(response => {
        if (!response.ok) throw new Error('Failed to update retention');
        return response.text();
    })
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        showSuccess('Retention period updated for all cameras');
    })
    .catch(error => {
        console.error('Error updating retention for all cameras:', error);
        showError('Failed to update retention for all cameras: ' + error.message);
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
        .then(response => {
            if (!response.ok) throw new Error('Failed to remove camera');
            return response.text();
        })
        .then(html => {
            document.getElementById('camera-list').innerHTML = html;
            const modalList = document.getElementById('modal-camera-list');
            if (modalList) {
                modalList.innerHTML = html;
            }
            if (typeof updateSectionHeight === 'function') {
                updateSectionHeight('cameras-content');
            }
            
            // Reload timeline camera selector
            if (typeof loadTimelineCameras === 'function') {
                loadTimelineCameras();
            }
            
            showSuccess('Camera removed successfully');
        })
        .catch(error => {
            console.error('Error removing camera:', error);
            showError('Failed to remove camera: ' + error.message);
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


// Clear events for a specific camera
function clearCameraEvents(cameraName) {
    fetch('/api/events/clear-camera', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ camera_name: cameraName })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showSuccess(`Successfully deleted ${data.count} event(s) for ${cameraName}`);
        } else {
            showError('Failed to clear events: ' + (data.error || 'Unknown error'));
        }
    })
    .catch(error => {
        console.error('Error clearing events:', error);
        showError('Failed to clear events');
    });
}

// Open delete all cameras modal
function openDeleteAllCamerasModal() {
    deleteCamerasCode = Math.floor(1000 + Math.random() * 9000).toString();
    document.getElementById('delete-cameras-code').textContent = deleteCamerasCode;
    document.getElementById('delete-cameras-code-input').value = '';
    document.getElementById('delete-cameras-btn').disabled = true;
    deleteCamerasConfirmCount = 0;
    
    const modal = document.getElementById('delete-cameras-modal');
    modal.classList.add('active');
    modal.style.display = 'block';
    
    setTimeout(() => {
        document.getElementById('delete-cameras-code-input').focus();
    }, 100);
}

// Close delete all cameras modal
function closeDeleteAllCamerasModal() {
    const modal = document.getElementById('delete-cameras-modal');
    modal.classList.remove('active');
    modal.style.display = 'none';
    deleteCamerasCode = '';
    deleteCamerasConfirmCount = 0;
    
    const btn = document.getElementById('delete-cameras-btn');
    btn.textContent = 'Delete All Cameras';
    btn.classList.remove('confirm');
    btn.disabled = true;
}

// Delete all cameras with modal confirmation
function removeAllCameras() {
    const btn = document.getElementById('delete-cameras-btn');
    
    if (deleteCamerasConfirmCount === 0) {
        btn.textContent = 'Click Again to Confirm';
        btn.classList.add('confirm');
        deleteCamerasConfirmCount = 1;
        
        setTimeout(() => {
            if (deleteCamerasConfirmCount === 1) {
                btn.textContent = 'Click to Confirm';
                btn.classList.remove('confirm');
                deleteCamerasConfirmCount = 0;
            }
        }, 3000);
        return;
    }
    
    btn.disabled = true;
    btn.textContent = 'Deleting...';
    
    fetch('/api/cameras/delete-all', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to remove all cameras');
        return response.text();
    })
    .then(html => {
        document.getElementById('camera-list').innerHTML = html;
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
        
        // Reload timeline camera selector
        if (typeof loadTimelineCameras === 'function') {
            loadTimelineCameras();
        }
        
        showSuccess('Successfully deleted all cameras');
        closeDeleteAllCamerasModal();
    })
    .catch(error => {
        console.error('Error removing all cameras:', error);
        showError('Failed to remove all cameras: ' + error.message);
    })
    .finally(() => {
        btn.disabled = false;
        btn.textContent = 'Delete All Cameras';
        btn.classList.remove('confirm');
    });
}


// Interpret only the current server schema; never invent missing deletion totals.
function orphanCleanupOutcome(data) {
    const counter = value => Number.isSafeInteger(value) && value >= 0;
    if (!data || Array.isArray(data) || typeof data !== 'object' ||
        typeof data.success !== 'boolean' || !counter(data.count) ||
        !counter(data.failures) || !counter(data.skipped_recovery_dirs) ||
        typeof data.interrupted !== 'boolean' || typeof data.limit_reached !== 'boolean' ||
        !Number.isFinite(data.size_mb) || data.size_mb < 0) {
        throw new Error('Invalid cleanup response');
    }
    const complete = data.success && data.failures === 0 &&
        !data.interrupted && !data.limit_reached;
    const preserved = data.skipped_recovery_dirs > 0;
    const reasons = [];
    if (data.failures) reasons.push(`${data.failures} failure(s)`);
    if (data.interrupted) reasons.push('interrupted');
    if (data.limit_reached) reasons.push('inspection limit reached');
    if (!data.success && reasons.length === 0) reasons.push('server reported an unsuccessful pass');
    let message = `Server reported ${data.count} orphaned file(s) deleted (${data.size_mb.toFixed(2)} MB). `;
    message += `${data.skipped_recovery_dirs} recovery/inspection-sensitive directory(s) preserved.`;
    if (!complete) message += ` Cleanup incomplete: ${reasons.join('; ')}. Remaining files may still exist.`;
    return {
        title: !complete ? 'Cleanup incomplete' :
            (preserved ? 'Cleanup finished with preserved directories' : 'Cleanup pass complete'),
        level: complete && !preserved ? 'success' : 'warning',
        message
    };
}

function renderOrphanCleanupOutcome(outcome) {
    const color = outcome.level === 'success' ? '#4CAF50' :
        (outcome.level === 'warning' ? '#ffb300' : '#f44336');
    for (const prefix of ['', 'modal-']) {
        const panel = document.getElementById(prefix + 'cleanup-results');
        const text = document.getElementById(prefix + 'cleanup-results-text');
        if (panel && text) {
            const heading = panel.querySelector('h4');
            if (heading) {
                heading.textContent = outcome.title;
                heading.style.color = color;
            }
            text.textContent = outcome.message;
            panel.style.borderLeftColor = color;
            panel.style.display = 'block';
        }
    }
    if (outcome.level === 'success') showSuccess(outcome.message);
    else if (outcome.level === 'warning') showWarning(outcome.message);
    else showError(outcome.message);
}

// Existing page/modal controls; this UI guard is not server deletion authority.
function orphanCleanupButtons() {
    return Array.from(document.querySelectorAll('button[onclick="cleanupOrphanedRecordings()"]'));
}
function resetOrphanCleanupConfirmation() {
    orphanedCleanupVersion++;
    if (orphanedCleanupTimer !== null) clearTimeout(orphanedCleanupTimer);
    orphanedCleanupTimer = null;
    const buttons = new Set(orphanCleanupButtons());
    if (orphanedCleanupButton) buttons.add(orphanedCleanupButton);
    for (const button of buttons) {
        button.textContent = orphanedCleanupPending ? 'Cleaning up...' : 'Cleanup Orphaned Recordings';
        button.disabled = orphanedCleanupPending;
        button.classList.remove('confirm');
    }
    orphanedCleanupConfirmed = false;
    orphanedCleanupButton = null;
    orphanedCleanupDeadline = 0;
}
async function cleanupOrphanedRecordings(trigger = globalThis.event) {
    if (orphanedCleanupPending) { resetOrphanCleanupConfirmation(); return; }
    const target = trigger?.currentTarget || trigger?.target || trigger;
    const btn = target?.closest?.('button') || target;
    if (!btn || !orphanCleanupButtons().includes(btn) || !btn.isConnected ||
        btn.disabled || btn.getClientRects().length === 0) {
        resetOrphanCleanupConfirmation();
        return;
    }
    if (!orphanedCleanupConfirmed || orphanedCleanupButton !== btn || Date.now() >= orphanedCleanupDeadline) {
        resetOrphanCleanupConfirmation();
        orphanedCleanupConfirmed = true;
        orphanedCleanupButton = btn;
        orphanedCleanupDeadline = Date.now() + 3000;
        const version = orphanedCleanupVersion;
        btn.textContent = 'Click Again to Confirm';
        btn.classList.add('confirm');
        orphanedCleanupTimer = setTimeout(() => {
            if (version === orphanedCleanupVersion && !orphanedCleanupPending) resetOrphanCleanupConfirmation();
        }, 3000);
        return;
    }
    orphanedCleanupPending = true;
    resetOrphanCleanupConfirmation();
    for (const prefix of ['', 'modal-']) {
        const panel = document.getElementById(prefix + 'cleanup-results');
        if (panel) panel.style.display = 'none';
    }
    let responseStatus = null;
    try {
        const response = await fetch('/api/recordings/cleanup-orphaned', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        responseStatus = response.status;
        if (!response.ok) throw new Error('Cleanup HTTP response was unsuccessful');
        renderOrphanCleanupOutcome(orphanCleanupOutcome(await response.json()));
    } catch (error) {
        console.error('Unable to verify orphan cleanup result:', error);
        const status = Number.isInteger(responseStatus) ? ` (HTTP ${responseStatus})` : '';
        renderOrphanCleanupOutcome({
            title: 'Cleanup outcome not verified',
            level: 'error',
            message: `Cleanup outcome could not be verified${status}. Files may have been removed; no deletion totals are confirmed. Review server state before another attempt. No automatic retry was made.`
        });
    } finally {
        orphanedCleanupPending = false;
        resetOrphanCleanupConfirmation();
    }
}
document.addEventListener('click', e => {
    if (orphanedCleanupConfirmed && !orphanedCleanupButton?.contains(e.target)) resetOrphanCleanupConfirmation();
}, true);
document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && orphanedCleanupConfirmed) resetOrphanCleanupConfirmation();
}, true);
document.addEventListener('visibilitychange', () => {
    if (document.hidden && orphanedCleanupConfirmed) resetOrphanCleanupConfirmation();
});

// Initialize drag and drop for camera reordering
function initializeDragAndDrop() {
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    if (!container) return;

    const dragHandles = container.querySelectorAll('.drag-handle[draggable="true"]');
    if (dragHandles.length === 0) return;

    dragHandles.forEach(handle => {
        handle.addEventListener('dragstart', handleDragStart);
        handle.addEventListener('dragend', handleDragEnd);
        
        const box = handle.closest('.camera-box');
        if (box) {
            box.addEventListener('dragover', handleDragOver);
            box.addEventListener('dragleave', handleDragLeave);
            box.addEventListener('drop', handleDrop);
        }
    });
}

function handleDragStart(e) {
    const draggedBox = e.target.closest('.camera-box');
    if (!draggedBox) return;
    
    draggedBox.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/html', draggedBox.innerHTML);
}

function handleDragEnd(e) {
    const draggedBox = e.target.closest('.camera-box');
    if (!draggedBox) return;
    
    draggedBox.classList.remove('dragging');
    
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    const allBoxes = container.querySelectorAll('.camera-box');
    allBoxes.forEach(box => {
        box.classList.remove('drag-over');
    });
}

function handleDragOver(e) {
    if (e.preventDefault) {
        e.preventDefault();
    }
    e.dataTransfer.dropEffect = 'move';
    
    const targetBox = e.currentTarget;
    targetBox.classList.add('drag-over');
    
    return false;
}

function handleDragLeave(e) {
    // Only remove highlight if we're actually leaving the box
    // (not entering a child element)
    const relatedTarget = e.relatedTarget;
    if (!relatedTarget || !e.currentTarget.contains(relatedTarget)) {
        e.currentTarget.classList.remove('drag-over');
    }
}

function handleDrop(e) {
    if (e.stopPropagation) {
        e.stopPropagation();
    }
    
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    const draggingBox = container.querySelector('.dragging');
    const targetBox = e.currentTarget;
    
    if (draggingBox && targetBox && draggingBox !== targetBox) {
        const allBoxes = Array.from(container.querySelectorAll('.camera-box'));
        const draggingIndex = allBoxes.indexOf(draggingBox);
        const targetIndex = allBoxes.indexOf(targetBox);
        
        if (draggingIndex < targetIndex) {
            targetBox.parentNode.insertBefore(draggingBox, targetBox.nextSibling);
        } else {
            targetBox.parentNode.insertBefore(draggingBox, targetBox);
        }
        
        saveNewOrder();
    }
    
    return false;
}

function saveNewOrder() {
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    if (!container) return;

    const cameraBoxes = container.querySelectorAll('.camera-box');
    const order = Array.from(cameraBoxes).map(box => box.dataset.cameraId);

    fetch('/api/cameras/reorder', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ order: order })
    })
    .then(response => {
        if (!response.ok) throw new Error('Failed to save order');
        
        // Refresh the camera list to get updated HTML
        return fetch('/api/cameras');
    })
    .then(response => response.text())
    .then(html => {
        // Update both lists
        const mainList = document.getElementById('camera-list');
        if (mainList) {
            mainList.innerHTML = html;
        }
        
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        
        // CRITICAL: Re-initialize drag and drop after HTML update
        setTimeout(() => {
            initializeDragAndDrop();
        }, 50);
        
        // Refresh timeline camera selector
        if (typeof loadTimelineCameras === 'function') {
            loadTimelineCameras();
        }
    })
    .catch(error => {
        console.error('Error saving camera order:', error);
        showError('Failed to save camera order. Please try again.');
    });
}
