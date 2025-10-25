// Core functionality: Section management, initialization, utilities

// Collapsible section toggle
function toggleSection(sectionId) {
    const content = document.getElementById(sectionId);
    const icon = document.getElementById(sectionId + '-icon');
    
    if (content.classList.contains('collapsed')) {
        content.classList.remove('collapsed');
        icon.classList.remove('collapsed');
        content.style.maxHeight = content.scrollHeight + 'px';
        // Save expanded state
        saveSectionState(sectionId, true);
    } else {
        content.classList.add('collapsed');
        icon.classList.add('collapsed');
        content.style.maxHeight = '0';
        // Save collapsed state
        saveSectionState(sectionId, false);
    }
}

// Save section state to localStorage
function saveSectionState(sectionId, isExpanded) {
    try {
        localStorage.setItem('section_' + sectionId, isExpanded ? 'expanded' : 'collapsed');
    } catch (e) {
        console.warn('Failed to save section state:', e);
    }
}

// Load section state from localStorage
function loadSectionState(sectionId) {
    try {
        const state = localStorage.getItem('section_' + sectionId);
        // Default to expanded if no state saved
        return state === 'collapsed' ? false : true;
    } catch (e) {
        console.warn('Failed to load section state:', e);
        return true; // Default to expanded
    }
}

// Update section height after content changes
function updateSectionHeight(sectionId) {
    const content = document.getElementById(sectionId);
    if (content && !content.classList.contains('collapsed')) {
        setTimeout(function() {
            content.style.maxHeight = content.scrollHeight + 'px';
        }, 50);
    }
}

// Initialize sections based on saved state
window.addEventListener('DOMContentLoaded', function() {
    const sections = ['playback-section', 'cameras-section', 'config-section'];
    
    sections.forEach(function(sectionId) {
        const content = document.getElementById(sectionId);
        const icon = document.getElementById(sectionId + '-icon');
        
        if (!content || !icon) return;
        
        // Load saved state
        const isExpanded = loadSectionState(sectionId);
        
        if (isExpanded) {
            // Expanded state
            content.classList.remove('collapsed');
            icon.classList.remove('collapsed');
            content.style.maxHeight = 'none';
            
            setTimeout(function() {
                content.style.maxHeight = content.scrollHeight + 'px';
            }, 100);
        } else {
            // Collapsed state
            content.classList.add('collapsed');
            icon.classList.add('collapsed');
            content.style.maxHeight = '0';
        }
    });
    
    // Initialize other modules after htmx settles
    setTimeout(function() {
        updateSectionHeight('cameras-section');
        if (typeof populatePlaybackCameras === 'function') {
            populatePlaybackCameras();
        }
    }, 1000);
});

// Listen for htmx events to update section heights
document.body.addEventListener('htmx:afterSwap', function(event) {
    if (event.detail.target.id === 'camera-list') {
        updateSectionHeight('cameras-section');
    }
});

// Utility: Format file size
function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

// Global keyboard shortcuts
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        if (typeof closeLiveStream === 'function') closeLiveStream();
        if (typeof closeSetAllRetentionModal === 'function') closeSetAllRetentionModal();
    }
});
