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
                addForm.reset();
            })
            .catch(error => {
                console.error('Error adding camera:', error);
                alert('Failed to add camera: ' + error.message);
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
            })
            .catch(error => {
                console.error('Error scanning go2rtc:', error);
                alert('Failed to scan go2rtc streams: ' + error.message);
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
        const modalList = document.getElementById('modal-camera-list');
        if (modalList) {
            modalList.innerHTML = html;
        }
        if (typeof updateSectionHeight === 'function') {
            updateSectionHeight('cameras-content');
        }
    })
    .catch(error => {
        console.error('Error updating retention:', error);
        alert('Failed to update retention period: ' + error.message);
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
    })
    .catch(error => {
        console.error('Error starting recording:', error);
        alert('Failed to start recording: ' + error.message);
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
    })
    .catch(error => {
        console.error('Error stopping recording:', error);
        alert('Failed to stop recording: ' + error.message);
    });
}

function setAllRetention() {
    const input = document.getElementById('retention-all');
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
    })
    .catch(error => {
        console.error('Error updating retention for all cameras:', error);
        alert('Failed to update retention for all cameras: ' + error.message);
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
        })
        .catch(error => {
            console.error('Error removing camera:', error);
            alert('Failed to remove camera: ' + error.message);
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
        alert('Successfully deleted all cameras');
        closeDeleteAllCamerasModal();
    })
    .catch(error => {
        console.error('Error removing all cameras:', error);
        alert('Failed to remove all cameras: ' + error.message);
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
        
        if (resultsDiv && resultsText) {
            const sizeMB = data.size_mb.toFixed(2);
            resultsText.innerHTML = `Deleted <strong>${data.count}</strong> orphaned recording(s), freed <strong>${sizeMB} MB</strong> of disk space.`;
            resultsDiv.style.display = 'block';
        }
        if (modalResultsDiv && modalResultsText) {
            const sizeMB = data.size_mb.toFixed(2);
            modalResultsText.innerHTML = `Deleted <strong>${data.count}</strong> orphaned recording(s), freed <strong>${sizeMB} MB</strong> of disk space.`;
            modalResultsDiv.style.display = 'block';
        }
    })
    .catch(error => {
        console.error('Error cleaning up orphaned recordings:', error);
        alert('Failed to cleanup orphaned recordings: ' + error.message);
        btn.disabled = false;
        btn.textContent = 'Cleanup Orphaned Recordings';
    });
}

// Drag and Drop functionality for camera reordering
let draggedElement = null;

function handleDragStart(e) {
    draggedElement = this;
    this.classList.add('dragging');
    this.style.opacity = '0.5';
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/html', this.innerHTML);
}

function handleDragEnd(e) {
    this.classList.remove('dragging');
    this.style.opacity = '1';
    
    // Remove all drag-over highlights
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    if (container) {
        container.querySelectorAll('.camera-box').forEach(box => {
            box.classList.remove('drag-over');
        });
    }
}

function handleDragOver(e) {
    if (e.preventDefault) {
        e.preventDefault();
    }
    e.dataTransfer.dropEffect = 'move';
    
    // Add visual feedback - highlight drop target
    if (draggedElement && draggedElement !== this) {
        // Remove highlight from all other boxes first
        const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
        if (container) {
            container.querySelectorAll('.camera-box').forEach(box => {
                if (box !== this && box !== draggedElement) {
                    box.classList.remove('drag-over');
                }
            });
        }
        this.classList.add('drag-over');
    }
    
    return false;
}


function handleDrop(e) {
    if (e.stopPropagation) {
        e.stopPropagation();
    }
    
    // Remove drag-over highlight
    this.classList.remove('drag-over');
    
    if (draggedElement && draggedElement !== this) {
        const container = this.parentNode;
        const allBoxes = [...container.querySelectorAll('.camera-box[draggable="true"]')];
        const draggedIndex = allBoxes.indexOf(draggedElement);
        const targetIndex = allBoxes.indexOf(this);
        
        if (draggedIndex < targetIndex) {
            container.insertBefore(draggedElement, this.nextSibling);
        } else {
            container.insertBefore(draggedElement, this);
        }
        
        saveNewOrder();
    }
    
    return false;
}

function initializeDragAndDrop() {
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    if (!container) return;

    const cameraBoxes = container.querySelectorAll('.camera-box[draggable="true"]');
    if (cameraBoxes.length === 0) return;

    cameraBoxes.forEach(box => {
        box.addEventListener('dragstart', handleDragStart);
        box.addEventListener('dragend', handleDragEnd);
        box.addEventListener('dragover', handleDragOver);
        box.addEventListener('drop', handleDrop);
    });
}

function saveNewOrder() {
    const container = document.getElementById('modal-camera-list') || document.getElementById('camera-list');
    if (!container) return;

    const cameraBoxes = container.querySelectorAll('.camera-box[draggable="true"]');
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
        
        // Refresh timeline camera selector immediately
        if (typeof loadTimelineCameras === 'function') {
            loadTimelineCameras();
        }
    })
    .catch(error => {
        console.error('Error saving camera order:', error);
        alert('Failed to save camera order. Please try again.');
    });
}
