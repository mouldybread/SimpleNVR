// Camera management functionality

// State
let removeConfirmTimers = {};
let orphanedCleanupConfirmed = false;
let orphanedCleanupTimer = null;
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


// Cleanup orphaned recordings from deleted cameras
function cleanupOrphanedRecordings() {
    if (!orphanedCleanupConfirmed) {
        const btn = event.target;
        btn.textContent = 'Click Again to Confirm';
        btn.classList.add('confirm');
        orphanedCleanupConfirmed = true;
        
        clearTimeout(orphanedCleanupTimer);
        orphanedCleanupTimer = setTimeout(() => {
            btn.textContent = 'Cleanup Orphaned Recordings';
            btn.classList.remove('confirm');
            orphanedCleanupConfirmed = false;
        }, 3000);
        return;
    }
    
    const btn = event.target;
    const resultsDiv = document.getElementById('cleanup-results');
    const resultsText = document.getElementById('cleanup-results-text');
    const modalResultsDiv = document.getElementById('modal-cleanup-results');
    const modalResultsText = document.getElementById('modal-cleanup-results-text');
    
    clearTimeout(orphanedCleanupTimer);
    orphanedCleanupConfirmed = false;
    btn.textContent = 'Cleanup Orphaned Recordings';
    btn.classList.remove('confirm');
    btn.disabled = true;
    btn.textContent = 'Cleaning up...';
    
    if (resultsDiv) {
        resultsDiv.style.display = 'none';
    }
    if (modalResultsDiv) {
        modalResultsDiv.style.display = 'none';
    }
    
    fetch('/api/recordings/cleanup-orphaned', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
    })
    .then(response => {
        if (!response.ok) {
            throw new Error('Failed to cleanup orphaned recordings');
        }
        return response.json();
    })
    .then(data => {
        btn.disabled = false;
        btn.textContent = 'Cleanup Orphaned Recordings';
        
        const sizeMB = data.size_mb.toFixed(2);
        const message = `Deleted ${data.files_deleted} orphaned file(s), freed ${sizeMB} MB`;
        
        if (resultsDiv && resultsText) {
            resultsText.innerHTML = `Deleted <strong>${data.files_deleted}</strong> orphaned file(s), freed <strong>${sizeMB} MB</strong>`;
            resultsDiv.style.display = 'block';
        }
        if (modalResultsDiv && modalResultsText) {
            modalResultsText.innerHTML = `Deleted <strong>${data.files_deleted}</strong> orphaned file(s), freed <strong>${sizeMB} MB</strong>`;
            modalResultsDiv.style.display = 'block';
        }
        
        showSuccess(message);
    })
    .catch(error => {
        console.error('Error cleaning up orphaned recordings:', error);
        showError('Failed to cleanup orphaned recordings: ' + error.message);
        btn.disabled = false;
        btn.textContent = 'Cleanup Orphaned Recordings';
    });
}

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
