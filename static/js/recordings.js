// Delete All Recordings functionality

let deleteCode = '';
let deleteConfirmCount = 0;

// Open delete all recordings modal
function openDeleteAllRecordingsModal() {
    // Generate random 4-digit code
    deleteCode = Math.floor(1000 + Math.random() * 9000).toString();
    
    document.getElementById('delete-code').textContent = deleteCode;
    document.getElementById('delete-code-input').value = '';
    document.getElementById('delete-recordings-btn').disabled = true;
    deleteConfirmCount = 0;
    
    const modal = document.getElementById('delete-recordings-modal');
    modal.classList.add('active');
    modal.style.display = 'block';
    
    // Focus on input
    setTimeout(() => {
        document.getElementById('delete-code-input').focus();
    }, 100);
}

// Close delete modal
function closeDeleteRecordingsModal() {
    const modal = document.getElementById('delete-recordings-modal');
    modal.classList.remove('active');
    modal.style.display = 'none';
    deleteCode = '';
    deleteConfirmCount = 0;
    
    const btn = document.getElementById('delete-recordings-btn');
    btn.textContent = 'Delete All Recordings';
    btn.classList.remove('confirm');
    btn.disabled = true;
}

// Validate code input
document.addEventListener('DOMContentLoaded', () => {
    const input = document.getElementById('delete-code-input');
    if (input) {
        input.oninput = () => {
            const btn = document.getElementById('delete-recordings-btn');
            if (input.value === deleteCode) {
                btn.disabled = false;
                btn.textContent = 'Click to Confirm';
                btn.classList.remove('confirm');
                deleteConfirmCount = 0;
            } else {
                btn.disabled = true;
                btn.textContent = 'Delete All Recordings';
            }
        };
    }
});

// Delete all recordings
function deleteAllRecordings() {
    const btn = document.getElementById('delete-recordings-btn');
    
    if (deleteConfirmCount === 0) {
        // First press
        btn.textContent = 'Click Again to Confirm';
        btn.classList.add('confirm');
        deleteConfirmCount = 1;
        
        // Reset after 3 seconds
        setTimeout(() => {
            if (deleteConfirmCount === 1) {
                btn.textContent = 'Click to Confirm';
                btn.classList.remove('confirm');
                deleteConfirmCount = 0;
            }
        }, 3000);
        return;
    }
    
    // Second press - actually delete
    btn.disabled = true;
    btn.textContent = 'Deleting...';
    
    fetch('/api/recordings/delete-all', {
        method: 'POST'
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            showSuccess(`Successfully deleted ${data.deleted} recordings (${data.size_mb.toFixed(2)} MB freed)`);
            closeDeleteRecordingsModal();
            
            // Reload timeline if on playback section
            if (typeof loadTimelineData === 'function') {
                loadTimelineData();
            }
        } else {
            showError('Failed to delete recordings');
        }
    })
    .catch(error => {
        console.error('Error deleting recordings:', error);
        showError('Failed to delete recordings');
    })
    .finally(() => {
        btn.disabled = false;
        btn.textContent = 'Delete All Recordings';
        btn.classList.remove('confirm');
    });
}

// Close modal on outside click
document.addEventListener('DOMContentLoaded', () => {
    const modal = document.getElementById('delete-recordings-modal');
    if (modal) {
        modal.onclick = (e) => {
            if (e.target === modal) {
                closeDeleteRecordingsModal();
            }
        };
    }
});
