// Core functionality: Section management, initialization, utilities

// Protected dashboard requests return 401 rather than a document redirect.
(function installSessionRedirect() {
    let redirecting = false;
    function handleUnauthorized(status, url) {
        if (status !== 401 || redirecting || !url) return;
        const target = new URL(url, window.location.href);
        if (target.origin !== window.location.origin || target.pathname.startsWith('/auth/')) return;
        redirecting = true;
        window.location.replace('/auth/login');
    }

    const fetch = window.fetch.bind(window);
    window.fetch = async function (...args) {
        const response = await fetch(...args);
        handleUnauthorized(response.status, response.url);
        return response;
    };

    document.body.addEventListener('htmx:beforeSwap', function(event) {
        const xhr = event.detail.xhr;
        if (xhr) handleUnauthorized(xhr.status, xhr.responseURL);
    });
})();

// Collapsible section toggle
function toggleSection(sectionId) {
    const content = document.getElementById(sectionId);
    const icon = document.getElementById(sectionId + '-icon');
    const collapsed = content.classList.toggle('collapsed');
    icon.classList.toggle('collapsed', collapsed);
    content.style.maxHeight = collapsed ? '0' : content.scrollHeight + 'px';
    saveSectionState(sectionId, !collapsed);
}

function saveSectionState(sectionId, isExpanded) {
    try {
        localStorage.setItem('section_' + sectionId, isExpanded ? 'expanded' : 'collapsed');
    } catch (e) {
        console.warn('Failed to save section state:', e);
    }
}

function loadSectionState(sectionId) {
    try {
        return localStorage.getItem('section_' + sectionId) !== 'collapsed';
    } catch (e) {
        console.warn('Failed to load section state:', e);
        return true;
    }
}

function updateSectionHeight(sectionId) {
    const content = document.getElementById(sectionId);
    if (content && !content.classList.contains('collapsed')) {
        setTimeout(function() {
            content.style.maxHeight = content.scrollHeight + 'px';
        }, 50);
    }
}

window.addEventListener('DOMContentLoaded', function() {
    const sections = ['playback-section', 'cameras-section', 'config-section'];
    sections.forEach(function(sectionId) {
        const content = document.getElementById(sectionId);
        const icon = document.getElementById(sectionId + '-icon');
        if (!content || !icon) return;
        const isExpanded = loadSectionState(sectionId);
        content.classList.toggle('collapsed', !isExpanded);
        icon.classList.toggle('collapsed', !isExpanded);
        content.style.maxHeight = isExpanded ? 'none' : '0';
        if (isExpanded) {
            setTimeout(function() {
                content.style.maxHeight = content.scrollHeight + 'px';
            }, 100);
        }
    });

    setTimeout(function() {
        updateSectionHeight('cameras-section');
        if (typeof populatePlaybackCameras === 'function') {
            populatePlaybackCameras();
        }
    }, 1000);
});

document.body.addEventListener('htmx:afterSwap', function(event) {
    if (event.detail.target.id === 'camera-list') {
        updateSectionHeight('cameras-section');
    }
});

function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        if (typeof closeLiveStream === 'function') closeLiveStream();
        if (typeof closeSetAllRetentionModal === 'function') closeSetAllRetentionModal();
    }
});
