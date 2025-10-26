// Enhanced Timeline-based Playback System
// FIXED: All issues + Firefox compatibility + Download feature with camera selection

// State
let selectedCameras = [];
let allCameras = [];
let currentDate = new Date();
let timelineSegments = [];
let gridLayout = 1;
let playbackSpeed = 1.0;
let isPlaying = false;
let currentTimePosition = null;
let videoPlayers = [];
let isUpdatingGrid = false;
let pendingSeekTime = null;
let lastSeekTime = null;
let seekTimeoutId = null;
let loadDataTimeoutId = null;
let pendingVideoTimeouts = [];

// Download state
let downloadStartTime = null;
let downloadEndTime = null;
let downloadCameras = [];
const CHUNK_SIZE_MS = 10 * 60 * 1000;

function initializeTimelinePlayback() {
    loadTimelineCameras();
    setupDatePicker();
    setupKeyboardShortcuts();
    setupTimelineControls();
}

function loadTimelineCameras() {
    fetch('/api/cameras')
        .then(response => response.text())
        .then(html => {
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, 'text/html');
            const cameraCards = doc.querySelectorAll('.camera-card');
            
            allCameras = [];
            cameraCards.forEach(card => {
                const nameEl = card.querySelector('.camera-name');
                const buttons = card.querySelectorAll('button[onclick*="removeCamera"]');
                
                if (buttons.length > 0) {
                    const onclickAttr = buttons[0].getAttribute('onclick');
                    const match = onclickAttr.match(/removeCamera\('([^']+)'\)/);
                    
                    if (match && nameEl) {
                        allCameras.push({
                            id: match[1],
                            name: nameEl.textContent.trim()
                        });
                    }
                }
            });

            const savedSelection = localStorage.getItem('selectedCameras');
            if (savedSelection) {
                try {
                    const parsed = JSON.parse(savedSelection);
                    selectedCameras = parsed.filter(id => 
                        allCameras.some(c => c.id === id)
                    );
                    console.log('Loaded camera selection from localStorage:', selectedCameras);
                } catch (e) {
                    console.error('Failed to parse saved camera selection:', e);
                    selectedCameras = allCameras.slice(0, 4).map(c => c.id);
                }
            } else {
                selectedCameras = allCameras.slice(0, 4).map(c => c.id);
            }

            if (selectedCameras.length === 0 && allCameras.length > 0) {
                selectedCameras = allCameras.slice(0, 4).map(c => c.id);
            }

            renderCameraSelector();
            loadTimelineData();
        })
        .catch(err => {
            console.error('Failed to load cameras for timeline:', err);
        });
}

function renderCameraSelector() {
    const container = document.getElementById('timeline-camera-selector');
    if (!container) return;

    container.innerHTML = '';
    
    allCameras.forEach(cam => {
        const label = document.createElement('label');
        label.className = 'camera-checkbox';
        
        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.value = cam.id;
        checkbox.checked = selectedCameras.includes(cam.id);
        checkbox.onchange = () => toggleCamera(cam.id);
        
        label.appendChild(checkbox);
        label.appendChild(document.createTextNode(' ' + cam.name));
        container.appendChild(label);
    });
}

function toggleCamera(cameraId) {
    if (selectedCameras.includes(cameraId)) {
        selectedCameras = selectedCameras.filter(id => id !== cameraId);
    } else {
        selectedCameras.push(cameraId);
    }

    localStorage.setItem('selectedCameras', JSON.stringify(selectedCameras));
    console.log('Saved camera selection to localStorage:', selectedCameras);

    gridLayout = selectedCameras.length;
    loadTimelineData();
    updateVideoGrid();
}

function setupDatePicker() {
    const datePicker = document.getElementById('timeline-date-picker');
    if (!datePicker) return;

    datePicker.valueAsDate = currentDate;
    datePicker.onchange = () => {
        currentDate = new Date(datePicker.value);
        loadTimelineData();
    };
}

function loadTimelineData() {
    if (loadDataTimeoutId) {
        clearTimeout(loadDataTimeoutId);
    }
    
    loadDataTimeoutId = setTimeout(() => {
        performLoadTimelineData();
    }, 200);
}

function performLoadTimelineData() {
    const year = currentDate.getFullYear();
    const month = String(currentDate.getMonth() + 1).padStart(2, '0');
    const day = String(currentDate.getDate()).padStart(2, '0');
    const dateStr = `${year}-${month}-${day}`;
    
    console.log('=== TIMELINE LOAD ===');
    console.log('Current date object:', currentDate);
    console.log('Querying for date:', dateStr);
    
    fetch(`/api/timeline/recordings?date=${dateStr}`)
        .then(response => response.json())
        .then(segments => {
            console.log('API returned', segments.length, 'segments');
            
            timelineSegments = segments.filter(s => {
                const include = selectedCameras.includes(s.camera_id);
                if (include) {
                    const start = new Date(s.start_time);
                    console.log(`Segment: ${s.camera_name} at ${start.toString()}`);
                }
                return include;
            });
            
            console.log('Filtered to', timelineSegments.length, 'visible segments');
            renderTimeline();
            updateVideoGrid();
        })
        .catch(err => {
            console.error('Failed to load timeline data:', err);
        });
}

function renderTimeline() {
    const canvas = document.getElementById('timeline-canvas');
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    const width = canvas.width = canvas.offsetWidth;
    const height = canvas.height = 80;

    ctx.fillStyle = '#1a1a1a';
    ctx.fillRect(0, 0, width, height);

    ctx.strokeStyle = '#444';
    ctx.fillStyle = '#888';
    ctx.font = '10px monospace';

    for (let hour = 0; hour < 24; hour++) {
        const x = (hour / 24) * width;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, height);
        ctx.stroke();
        ctx.fillText(hour.toString().padStart(2, '0'), x + 2, 12);
    }

    const cameraColors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899'];
    const camerasInView = [...new Set(timelineSegments.map(s => s.camera_id))];
    const blockHeight = (height - 20) / Math.max(camerasInView.length, 1);

    camerasInView.forEach((cameraId, idx) => {
        const segments = timelineSegments.filter(s => s.camera_id === cameraId);
        const color = cameraColors[idx % cameraColors.length];

        segments.forEach(segment => {
            const startDate = new Date(segment.start_time);
            const endDate = new Date(segment.end_time);

            const startHour = startDate.getHours() + startDate.getMinutes() / 60;
            const endHour = endDate.getHours() + endDate.getMinutes() / 60;

            const x1 = (startHour / 24) * width;
            const x2 = (endHour / 24) * width;
            const y = 20 + (idx * blockHeight);

            ctx.fillStyle = color;
            ctx.fillRect(x1, y, x2 - x1, blockHeight - 2);
        });
    });

    if (currentTimePosition) {
        const date = new Date(currentTimePosition);
        const hour = date.getHours() + date.getMinutes() / 60 + date.getSeconds() / 3600;
        const x = (hour / 24) * width;

        ctx.strokeStyle = '#fff';
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, height);
        ctx.stroke();
        ctx.lineWidth = 1;
    }

    renderTimelineLegend(camerasInView, cameraColors);
}

function renderTimelineLegend(camerasInView, cameraColors) {
    const legendContainer = document.getElementById('timeline-legend');
    if (!legendContainer) return;

    legendContainer.innerHTML = '';

    camerasInView.forEach((cameraId, idx) => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;

        const color = cameraColors[idx % cameraColors.length];

        const legendItem = document.createElement('div');
        legendItem.className = 'legend-item';

        const colorBox = document.createElement('span');
        colorBox.className = 'legend-color';
        colorBox.style.backgroundColor = color;

        const label = document.createElement('span');
        label.textContent = camera.name;

        legendItem.appendChild(colorBox);
        legendItem.appendChild(label);
        legendContainer.appendChild(legendItem);
    });
}

function setupTimelineControls() {
    const canvas = document.getElementById('timeline-canvas');
    if (!canvas) return;

    canvas.onclick = (e) => {
        const rect = canvas.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const clickedHour = (x / rect.width) * 24;

        const targetTime = new Date(currentDate);
        targetTime.setHours(Math.floor(clickedHour));
        targetTime.setMinutes((clickedHour % 1) * 60);
        targetTime.setSeconds(0);

        console.log('Timeline clicked at hour:', clickedHour, 'Target time:', targetTime);
        seekToTime(targetTime);
    };

    const playPauseBtn = document.getElementById('play-pause');
    const prevBtn = document.getElementById('prev-segment');
    const nextBtn = document.getElementById('next-segment');
    const muteBtn = document.getElementById('mute-all-btn');
    const fullscreenBtn = document.getElementById('fullscreen-btn');
    const downloadBtn = document.getElementById('download-btn');

    if (playPauseBtn) playPauseBtn.addEventListener('click', togglePlayPause);
    if (prevBtn) prevBtn.addEventListener('click', previousSegment);
    if (nextBtn) nextBtn.addEventListener('click', nextSegment);
    if (muteBtn) muteBtn.addEventListener('click', toggleMuteAll);
    if (fullscreenBtn) fullscreenBtn.addEventListener('click', toggleFullscreen);
    if (downloadBtn) downloadBtn.addEventListener('click', openDownloadModal);

    const speedControl = document.getElementById('speed-control');
    if (speedControl) {
        speedControl.addEventListener('change', (e) => {
            playbackSpeed = parseFloat(e.target.value);
            updatePlaybackSpeed();
        });
    }

    document.addEventListener('fullscreenchange', () => {
        console.log('Fullscreen change detected');
        setTimeout(() => {
            if (currentTimePosition) {
                console.log('Re-syncing videos after fullscreen change');
                seekToTime(new Date(currentTimePosition));
            }
        }, 200);
    });
}

function seekToTime(time) {
    const timeMs = time.getTime();
    if (lastSeekTime === timeMs && seekTimeoutId) {
        console.log('Skipping duplicate seek to same time');
        return;
    }
    
    if (seekTimeoutId) {
        clearTimeout(seekTimeoutId);
    }
    
    if (isUpdatingGrid) {
        pendingSeekTime = timeMs;
        console.log('Grid is updating, deferring seek');
        return;
    }

    lastSeekTime = timeMs;
    currentTimePosition = timeMs;
    
    seekTimeoutId = setTimeout(() => {
        performSeek(time);
        seekTimeoutId = null;
    }, 100);
}

function performSeek(time) {
    pendingVideoTimeouts.forEach(timeoutId => clearTimeout(timeoutId));
    pendingVideoTimeouts = [];
    
    console.log('=== SEEK DEBUG ===');
    console.log('Seeking to:', time.toISOString(), '(Local:', time.toString(), ')');
    console.log('Searching through', timelineSegments.length, 'segments');

    selectedCameras.forEach((cameraId, idx) => {
        const timeoutId = setTimeout(() => {
            pendingVideoTimeouts = pendingVideoTimeouts.filter(id => id !== timeoutId);
            
            if (!videoPlayers[idx]) {
                console.warn(`Video player at index ${idx} does not exist, skipping`);
                return;
            }

            console.log(`\n--- Camera ${cameraId} ---`);
            
            const segment = timelineSegments.find(s => {
                if (s.camera_id !== cameraId) return false;
                
                const segStart = new Date(s.start_time);
                const segEnd = new Date(s.end_time);
                
                const timeMs = time.getTime();
                const startMs = segStart.getTime();
                const endMs = segEnd.getTime();
                
                const matches = timeMs >= startMs && timeMs < endMs;
                
                console.log(`Segment: ${s.file_path}`);
                console.log(`  Start: ${segStart.toISOString()} (${segStart.toString()})`);
                console.log(`  End:   ${segEnd.toISOString()} (${segEnd.toString()})`);
                console.log(`  Time:  ${time.toISOString()} (${time.toString()})`);
                console.log(`  Match: ${matches} (${timeMs} >= ${startMs} && ${timeMs} < ${endMs})`);
                
                return matches;
            });

            const video = videoPlayers[idx];
            
            if (segment) {
                const filePath = segment.file_path;
                
                console.log(`✓ MATCH FOUND: Loading ${filePath}`);
                
                const videoCell = video.parentElement;
                if (videoCell) {
                    const message = videoCell.querySelector('.no-video-message');
                    if (message) {
                        message.style.display = 'none';
                    }
                }
                
                video.style.display = 'block';
                video.pause();
                video.removeAttribute('src');
                
                setTimeout(() => {
                    video.src = filePath;
                    
                    const segmentStart = new Date(segment.start_time).getTime();
                    const offset = (time.getTime() - segmentStart) / 1000;
                    
                    console.log(`Offset within segment: ${offset.toFixed(2)}s`);
                    
                    const metadataHandler = () => {
                        const seekTime = Math.max(0, Math.min(offset, video.duration));
                        console.log(`Video loaded, duration: ${video.duration}s, seeking to ${seekTime.toFixed(2)}s`);
                        video.currentTime = seekTime;
                        
                        if (isPlaying) {
                            const playPromise = video.play();
                            if (playPromise !== undefined) {
                                playPromise.catch(err => {
                                    console.log('Autoplay prevented (browser policy)');
                                });
                            }
                        }
                        video.removeEventListener('loadedmetadata', metadataHandler);
                    };
                    
                    const errorHandler = (e) => {
                        console.error(`✗ Video load error for ${filePath}:`, video.error);
                        video.style.display = 'none';
                        const videoCell = video.parentElement;
                        if (videoCell) {
                            const message = videoCell.querySelector('.no-video-message');
                            if (message) {
                                message.style.display = 'block';
                            }
                        }
                        video.removeEventListener('error', errorHandler);
                    };
                    
                    video.addEventListener('loadedmetadata', metadataHandler);
                    video.addEventListener('error', errorHandler);
                }, 10);
            } else {
                console.log(`✗ No segment found for camera ${cameraId} at ${time}`);
                
                video.pause();
                video.removeAttribute('src');
                video.style.display = 'none';
                
                const videoCell = video.parentElement;
                if (videoCell) {
                    const message = videoCell.querySelector('.no-video-message');
                    if (message) {
                        message.style.display = 'block';
                    }
                }
            }
        }, idx * 150);
        
        pendingVideoTimeouts.push(timeoutId);
    });

    renderTimeline();
    
    if (!isPlaying) {
        isPlaying = true;
        const btn = document.getElementById('play-pause');
        if (btn) btn.textContent = '⏸ Pause';
    }
}

function togglePlayPause() {
    isPlaying = !isPlaying;

    videoPlayers.forEach(video => {
        if (video) {
            if (isPlaying) {
                const playPromise = video.play();
                if (playPromise !== undefined) {
                    playPromise.catch(err => console.log('Play prevented by browser'));
                }
            } else {
                video.pause();
            }
        }
    });

    const btn = document.getElementById('play-pause');
    if (btn) btn.textContent = isPlaying ? '⏸ Pause' : '▶ Play';
}

function toggleFullscreen() {
    const container = document.getElementById('fullscreen-wrapper');
    if (!container) return;

    if (!document.fullscreenElement) {
        container.requestFullscreen().catch(err => {
            console.error('Error attempting to enable fullscreen:', err);
        });
    } else {
        document.exitFullscreen().catch(err => {
            console.error('Error attempting to exit fullscreen:', err);
        });
    }
}

function toggleMuteAll() {
    const allMuted = videoPlayers.filter(v => v).every(v => v.muted);
    
    videoPlayers.forEach(video => {
        if (video) {
            video.muted = !allMuted;
        }
    });
    
    const btn = document.getElementById('mute-all-btn');
    if (btn) {
        btn.textContent = allMuted ? '🔊 Unmute All' : '🔇 Mute All';
    }
}

function updateVideoGrid() {
    const container = document.getElementById('video-grid');
    if (!container) return;

    isUpdatingGrid = true;

    videoPlayers.forEach(video => {
        if (video) {
            video.pause();
            video.removeAttribute('src');
        }
    });

    container.innerHTML = '';
    videoPlayers = [];

    const numCameras = selectedCameras.length;
    if (numCameras === 0) {
        container.innerHTML = '<p style="color: #888; text-align: center; padding: 2rem;">Select cameras to begin playback</p>';
        isUpdatingGrid = false;
        return;
    }

    let cols;
    if (numCameras === 1) {
        cols = 1;
    } else if (numCameras === 2) {
        cols = 2;
    } else if (numCameras <= 4) {
        cols = 2;
    } else if (numCameras <= 6) {
        cols = 3;
    } else if (numCameras <= 9) {
        cols = 3;
    } else {
        cols = 4;
    }

    container.style.display = 'grid';
    container.style.gap = '10px';
    container.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;

    const cameraColors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899'];
    const camerasInView = [...new Set(timelineSegments.map(s => s.camera_id))];

    selectedCameras.forEach((cameraId, idx) => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;

        const colorIdx = camerasInView.indexOf(cameraId);
        const color = colorIdx >= 0 ? cameraColors[colorIdx % cameraColors.length] : '#888';

        const wrapper = document.createElement('div');
        wrapper.className = 'video-cell';
        wrapper.style.position = 'relative';
        wrapper.style.backgroundColor = '#1a1a1a';
        wrapper.style.aspectRatio = '16/9';

        const label = document.createElement('div');
        label.className = 'video-label';
        label.textContent = camera.name;
        label.style.backgroundColor = color;

        const noVideoMessage = document.createElement('div');
        noVideoMessage.className = 'no-video-message';
        noVideoMessage.style.cssText = `
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            text-align: center;
            color: #888;
            font-size: 18px;
            pointer-events: none;
            z-index: 10;
            display: none;
        `;
        noVideoMessage.innerHTML = `
            <div style="font-size: 48px; margin-bottom: 10px;">📹</div>
            <div>No Recording Available</div>
        `;

        const video = document.createElement('video');
        video.controls = false;
        video.muted = true;
        video.playbackRate = playbackSpeed;
        video.style.width = '100%';
        video.style.height = '100%';
        video.style.backgroundColor = '#000';

        video.addEventListener('error', () => {
            video.style.display = 'none';
            const videoCell = video.parentElement;
            if (videoCell) {
                const message = videoCell.querySelector('.no-video-message');
                if (message) {
                    message.style.display = 'block';
                }
            }
        });

        video.addEventListener('loadeddata', () => {
            video.style.display = 'block';
            const videoCell = video.parentElement;
            if (videoCell) {
                const message = videoCell.querySelector('.no-video-message');
                if (message) {
                    message.style.display = 'none';
                }
            }
        });

        wrapper.appendChild(label);
        wrapper.appendChild(noVideoMessage);
        wrapper.appendChild(video);
        container.appendChild(wrapper);

        videoPlayers.push(video);
    });

    if (currentTimePosition && !pendingSeekTime) {
        pendingSeekTime = currentTimePosition;
    }

    isUpdatingGrid = false;
    
    if (pendingSeekTime) {
        const timeToSeek = pendingSeekTime;
        pendingSeekTime = null;
        seekToTime(new Date(timeToSeek));
    }
}

function updatePlaybackSpeed() {
    videoPlayers.forEach(video => {
        if (video) {
            video.playbackRate = playbackSpeed;
        }
    });
}

function previousSegment() {
    if (!currentTimePosition) {
        const start = new Date(currentDate);
        start.setHours(0, 0, 0, 0);
        seekToTime(start);
        return;
    }

    const current = new Date(currentTimePosition);
    current.setMinutes(current.getMinutes() - 10);
    seekToTime(current);
}

function nextSegment() {
    if (!currentTimePosition) {
        const start = new Date(currentDate);
        start.setHours(0, 0, 0, 0);
        seekToTime(start);
        return;
    }

    const current = new Date(currentTimePosition);
    current.setMinutes(current.getMinutes() + 10);
    seekToTime(current);
}

function setupKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
        if (e.target.tagName === 'INPUT') return;

        switch(e.key) {
            case ' ':
                e.preventDefault();
                togglePlayPause();
                break;
            case 'ArrowLeft':
                e.preventDefault();
                previousSegment();
                break;
            case 'ArrowRight':
                e.preventDefault();
                nextSegment();
                break;
        }
    });
}

// Download functionality
function openDownloadModal() {
    if (!currentTimePosition) {
        alert('Please select a time on the timeline first');
        return;
    }
    
    downloadCameras = [...selectedCameras];
    resetDownloadTimeRange();
    updateDownloadModalDisplay();
    
    const modal = document.getElementById('download-modal');
    if (modal) modal.style.display = 'block';
}

function closeDownloadModal() {
    const modal = document.getElementById('download-modal');
    if (modal) modal.style.display = 'none';
    
    const status = document.getElementById('download-status');
    if (status) status.style.display = 'none';
}

function resetDownloadTimeRange() {
    if (!currentTimePosition) return;
    
    const currentTime = new Date(currentTimePosition);
    downloadStartTime = new Date(currentTime.getTime() - (CHUNK_SIZE_MS / 2));
    downloadEndTime = new Date(currentTime.getTime() + (CHUNK_SIZE_MS / 2));
    
    updateDownloadModalDisplay();
}

function expandDownloadTimeEarlier() {
    downloadStartTime = new Date(downloadStartTime.getTime() - CHUNK_SIZE_MS);
    updateDownloadModalDisplay();
}

function expandDownloadTimeLater() {
    downloadEndTime = new Date(downloadEndTime.getTime() + CHUNK_SIZE_MS);
    updateDownloadModalDisplay();
}

function toggleDownloadCamera(cameraId) {
    if (downloadCameras.includes(cameraId)) {
        downloadCameras = downloadCameras.filter(id => id !== cameraId);
    } else {
        downloadCameras.push(cameraId);
    }
    updateDownloadModalDisplay();
}

function updateDownloadModalDisplay() {
    const timeDisplay = document.getElementById('download-time-display');
    if (timeDisplay) {
        const startStr = downloadStartTime.toLocaleTimeString();
        const endStr = downloadEndTime.toLocaleTimeString();
        timeDisplay.textContent = `${startStr} - ${endStr}`;
    }
    
    const durationDisplay = document.getElementById('download-duration-display');
    if (durationDisplay) {
        const durationMs = downloadEndTime - downloadStartTime;
        const minutes = Math.floor(durationMs / 60000);
        durationDisplay.textContent = `Duration: ${minutes} minutes`;
    }
    
    const cameraList = document.getElementById('download-camera-list');
    if (cameraList) {
        cameraList.innerHTML = '';
        selectedCameras.forEach(cameraId => {
            const camera = allCameras.find(c => c.id === cameraId);
            if (camera) {
                const isChecked = downloadCameras.includes(cameraId);
                
                const label = document.createElement('label');
                label.style.cssText = 'display: flex; align-items: center; padding: 8px; background: #2a2a2a; margin: 5px 0; border-radius: 4px; cursor: pointer;';
                label.innerHTML = `
                    <input type="checkbox" 
                           ${isChecked ? 'checked' : ''} 
                           onchange="toggleDownloadCamera('${cameraId}')"
                           style="margin-right: 10px; cursor: pointer;">
                    <span>📹 ${camera.name}</span>
                `;
                cameraList.appendChild(label);
            }
        });
        
        if (downloadCameras.length === 0) {
            const warning = document.createElement('div');
            warning.style.cssText = 'color: #f44336; padding: 8px; background: #3a1f1f; margin: 5px 0; border-radius: 4px;';
            warning.textContent = '⚠️ No cameras selected';
            cameraList.appendChild(warning);
        }
    }
}

function startDownload() {
    if (downloadCameras.length === 0) {
        alert('Please select at least one camera to download');
        return;
    }
    
    const statusDiv = document.getElementById('download-status');
    const progressList = document.getElementById('download-progress-list');
    
    if (statusDiv) statusDiv.style.display = 'block';
    if (progressList) progressList.innerHTML = '';
    
    downloadCameras.forEach(cameraId => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;
        
        const progressItem = document.createElement('div');
        progressItem.style.cssText = 'padding: 10px; background: #2a2a2a; margin: 5px 0; border-radius: 4px;';
        progressItem.innerHTML = `
            <div style="display: flex; justify-content: space-between; align-items: center;">
                <span>${camera.name}</span>
                <span id="progress-${cameraId}" style="color: #4CAF50;">⏳ Preparing...</span>
            </div>
        `;
        if (progressList) progressList.appendChild(progressItem);
        
        const params = new URLSearchParams({
            camera_id: cameraId,
            start_time: downloadStartTime.toISOString(),
            end_time: downloadEndTime.toISOString()
        });
        
        const downloadUrl = `/api/download?${params.toString()}`;
        
        const statusSpan = document.getElementById(`progress-${cameraId}`);
        if (statusSpan) statusSpan.textContent = '⬇ Downloading...';
        
        const link = document.createElement('a');
        link.href = downloadUrl;
        link.download = `${camera.name}_${downloadStartTime.toISOString().split('T')[0]}.mp4`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        
        setTimeout(() => {
            if (statusSpan) statusSpan.textContent = '✓ Download started';
        }, 500);
    });
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeTimelinePlayback);
} else {
    initializeTimelinePlayback();
}
