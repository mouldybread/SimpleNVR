// Enhanced Timeline-based Playback System
// FIXED: All issues + Firefox compatibility + Download feature with camera selection
// FIXED: Stable color mapping - colors persist across camera reordering

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

// Camera color mapping for UI consistency - stable colors by camera ID
let cameraColorMap = {};
const COLORS = [
    '#3b82f6',  // Blue
    '#10b981',  // Green
    '#f59e0b',  // Orange
    '#ef4444',  // Red
    '#8b5cf6',  // Purple
    '#ec4899',  // Pink
    '#06b6d4',  // Cyan
    '#84cc16',  // Lime
    '#f97316',  // Deep Orange
    '#14b8a6',  // Teal
    '#a855f7',  // Light Purple
    '#f43f5e',  // Rose
    '#0ea5e9',  // Sky Blue
    '#eab308',  // Yellow
    '#6366f1',  // Indigo
    '#d946ef'   // Fuchsia
];

// Timeline zoom state
let timelineZoomLevel = 1.0; // 1.0 = full 24 hours, 2.0 = 12 hours, etc.
let timelineOffset = 0; // Hours offset from start of day

let nextColorIndex = 0;

const CHUNK_SIZE_MS = 10 * 60 * 1000;

// Helper function to get cameras in configured order, filtered to selected ones
function getOrderedSelectedCameras() {
    return allCameras
        .filter(cam => selectedCameras.includes(cam.id))
        .map(cam => cam.id);
}

function initializeTimelinePlayback() {
    loadTimelineCameras();
    setupDatePicker();
    setupKeyboardShortcuts();
    setupTimelineControls();
    setupTimelineZoom(); 
}

function loadTimelineCameras() {
    fetch('/api/cameras')
        .then(response => response.text())
        .then(html => {
            const parser = new DOMParser();
            const doc = parser.parseFromString(html, 'text/html');
            
            const cameraBoxes = doc.querySelectorAll('.camera-box');
            allCameras = [];
            
            cameraBoxes.forEach(box => {
                const nameEl = box.querySelector('.camera-title');
                const buttons = box.querySelectorAll('button[onclick*="removeCamera"]');
                
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

            // Initialize color map for previously selected cameras
            selectedCameras.forEach(cameraId => {
                if (!cameraColorMap[cameraId]) {
                    cameraColorMap[cameraId] = COLORS[nextColorIndex % COLORS.length];
                    nextColorIndex++;
                }
            });

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

    // Create dropdown structure
    container.innerHTML = `
        <div class="camera-dropdown">
            <button class="dropdown-toggle" id="camera-dropdown-toggle" type="button">
                <span id="dropdown-label">Select Cameras</span>
                <span class="dropdown-arrow">▼</span>
            </button>
            <div class="dropdown-menu" id="camera-dropdown-menu">
                <!-- Options will be populated here -->
            </div>
        </div>
    `;

    const toggle = document.getElementById('camera-dropdown-toggle');
    const menu = document.getElementById('camera-dropdown-menu');
    
    // Populate dropdown options
    allCameras.forEach(cam => {
        const option = document.createElement('div');
        option.className = 'dropdown-option';
        option.dataset.cameraId = cam.id;
        
        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.checked = selectedCameras.includes(cam.id);
        checkbox.id = `timeline-cam-${cam.id}`;
        
        const label = document.createElement('label');
        label.htmlFor = `timeline-cam-${cam.id}`;
        label.textContent = cam.name;
        
        // Apply camera color if assigned
        if (cameraColorMap[cam.id]) {
            label.style.backgroundColor = cameraColorMap[cam.id];
            label.style.padding = '4px 10px';
            label.style.borderRadius = '4px';
            label.style.color = '#fff';
            label.style.marginLeft = '8px';
            label.style.cursor = 'pointer';
        }
        
        option.appendChild(checkbox);
        option.appendChild(label);
        
        option.onclick = (e) => {
            if (e.target === checkbox) return;
            e.stopPropagation();
            checkbox.checked = !checkbox.checked;
            toggleCamera(cam.id);
        };
        
        checkbox.onchange = (e) => {
            e.stopPropagation();
            toggleCamera(cam.id);
        };
        
        menu.appendChild(option);
    });
    
    // Toggle dropdown
    toggle.onclick = (e) => {
        e.stopPropagation();
        menu.classList.toggle('show');
    };
    
    // Close dropdown when clicking outside
    const closeHandler = (e) => {
        if (!container.contains(e.target)) {
            menu.classList.remove('show');
        }
    };
    document.removeEventListener('click', closeHandler);
    document.addEventListener('click', closeHandler);
    
    updateDropdownLabel();
}

function updateDropdownLabel() {
    const label = document.getElementById('dropdown-label');
    if (!label) return;
    
    const count = selectedCameras.length;
    if (count === 0) {
        label.textContent = 'Select Cameras';
    } else if (count === 1) {
        const cam = allCameras.find(c => c.id === selectedCameras[0]);
        label.textContent = cam ? cam.name : '1 camera';
    } else {
        label.textContent = `${count} cameras selected`;
    }
}

function toggleCamera(cameraId) {
    if (selectedCameras.includes(cameraId)) {
        selectedCameras = selectedCameras.filter(id => id !== cameraId);
    } else {
        selectedCameras.push(cameraId);
        
        if (!cameraColorMap[cameraId]) {
            cameraColorMap[cameraId] = COLORS[nextColorIndex % COLORS.length];
            nextColorIndex++;
        }
    }

    localStorage.setItem('selectedCameras', JSON.stringify(selectedCameras));
    console.log('Saved camera selection to localStorage:', selectedCameras);

    gridLayout = selectedCameras.length;
    updateDropdownLabel();
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

    // Calculate visible time range based on zoom
    const visibleHours = 24 / timelineZoomLevel;
    const startHour = timelineOffset;
    const endHour = startHour + visibleHours;

    // Adaptive time markers based on zoom level
    let majorStep, minorStep, showMinutes;
    
    if (visibleHours >= 12) {
        // Full day or half day: hourly markers
        majorStep = 2;
        minorStep = 1;
        showMinutes = false;
    } else if (visibleHours >= 4) {
        // 4-12 hours: hourly major, 30-min minor
        majorStep = 1;
        minorStep = 0.5;
        showMinutes = false;
    } else if (visibleHours >= 2) {
        // 2-4 hours: 30-min major, 10-min minor
        majorStep = 0.5;
        minorStep = 10/60;
        showMinutes = true;
    } else {
        // < 2 hours: 10-min major, 1-min minor
        majorStep = 10/60;
        minorStep = 1/60;
        showMinutes = true;
    }

    // Draw minor markers
    ctx.strokeStyle = '#333';
    const startMinutes = Math.floor(startHour * 60);
    const endMinutes = Math.ceil(endHour * 60);
    const minorStepMinutes = minorStep * 60;
    
    for (let minutes = Math.ceil(startMinutes / minorStepMinutes) * minorStepMinutes; 
         minutes <= endMinutes; 
         minutes += minorStepMinutes) {
        const hour = minutes / 60;
        const x = ((hour - startHour) / visibleHours) * width;
        
        ctx.beginPath();
        ctx.moveTo(x, height - 10);
        ctx.lineTo(x, height);
        ctx.stroke();
    }

    // Draw major markers with labels
    ctx.strokeStyle = '#555';
    const majorStepMinutes = majorStep * 60;
    
    for (let minutes = Math.ceil(startMinutes / majorStepMinutes) * majorStepMinutes; 
         minutes <= endMinutes; 
         minutes += majorStepMinutes) {
        const hour = minutes / 60;
        const x = ((hour - startHour) / visibleHours) * width;
        
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, height);
        ctx.stroke();

        // Format label
        let label;
        if (showMinutes) {
            const h = Math.floor(hour);
            const m = Math.round((hour - h) * 60);
            label = `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`;
        } else {
            label = Math.floor(hour).toString().padStart(2, '0');
        }
        
        ctx.fillStyle = '#888';
        ctx.fillText(label, x + 2, 12);
    }

    const camerasInView = [...new Set(timelineSegments.map(s => s.camera_id))];
    const blockHeight = (height - 20) / Math.max(camerasInView.length, 1);

    camerasInView.forEach((cameraId, idx) => {
        const segments = timelineSegments.filter(s => s.camera_id === cameraId);
        const color = cameraColorMap[cameraId] || '#888';

        segments.forEach(segment => {
            const startDate = new Date(segment.start_time);
            const endDate = new Date(segment.end_time);

            const segStartHour = startDate.getHours() + startDate.getMinutes() / 60;
            const segEndHour = endDate.getHours() + endDate.getMinutes() / 60;

            // Only render if segment is visible in current zoom window
            if (segEndHour >= startHour && segStartHour <= endHour) {
                const x1 = Math.max(0, ((segStartHour - startHour) / visibleHours) * width);
                const x2 = Math.min(width, ((segEndHour - startHour) / visibleHours) * width);
                const y = 20 + (idx * blockHeight);

                ctx.fillStyle = color;
                ctx.fillRect(x1, y, x2 - x1, blockHeight - 2);
            }
        });
    });

    if (currentTimePosition) {
        const date = new Date(currentTimePosition);
        const hour = date.getHours() + date.getMinutes() / 60 + date.getSeconds() / 3600;
        
        // Only draw playhead if it's in visible range
        if (hour >= startHour && hour <= endHour) {
            const x = ((hour - startHour) / visibleHours) * width;

            ctx.strokeStyle = '#fff';
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, height);
            ctx.stroke();
            ctx.lineWidth = 1;
        }
    }
}


function setupTimelineZoom() {
    const canvas = document.getElementById('timeline-canvas');
    if (!canvas) return;

    canvas.addEventListener('wheel', (e) => {
        e.preventDefault();
        
        const rect = canvas.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        const mouseHour = (mouseX / rect.width) * (24 / timelineZoomLevel) + timelineOffset;
        
        // Zoom in/out
        const zoomDelta = e.deltaY > 0 ? 0.8 : 1.25;
        const newZoomLevel = Math.max(1.0, Math.min(24.0, timelineZoomLevel * zoomDelta));
        
        if (newZoomLevel !== timelineZoomLevel) {
            timelineZoomLevel = newZoomLevel;
            
            // Adjust offset to keep mouse position centered
            const visibleHours = 24 / timelineZoomLevel;
            timelineOffset = mouseHour - (mouseX / rect.width) * visibleHours;
            timelineOffset = Math.max(0, Math.min(24 - visibleHours, timelineOffset));
            
            renderTimeline();
        }
    }, { passive: false });
}


function setupTimelineControls() {
    const canvas = document.getElementById('timeline-canvas');
    if (!canvas) return;

canvas.onclick = (e) => {
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const visibleHours = 24 / timelineZoomLevel;
    const clickedHour = (x / rect.width) * visibleHours + timelineOffset;

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

    const orderedSelectedCameras = getOrderedSelectedCameras();

    orderedSelectedCameras.forEach((cameraId, idx) => {
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
                console.log(`  End: ${segEnd.toISOString()} (${segEnd.toString()})`);
                console.log(`  Time: ${time.toISOString()} (${time.toString()})`);
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

    const orderedSelectedCameras = getOrderedSelectedCameras();
    const numCameras = orderedSelectedCameras.length;

    if (numCameras === 0) {
        container.innerHTML = '<div style="padding: 40px; text-align: center; color: #888; font-size: 18px;">Select cameras to begin playback</div>';
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

    orderedSelectedCameras.forEach((cameraId, idx) => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;

        const color = cameraColorMap[cameraId] || '#888';

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
        noVideoMessage.innerHTML = `<div>No Recording Available</div><div style="font-size: 14px; margin-top: 10px;">Select a different time</div>`;

        const video = document.createElement('video');
        video.className = 'timeline-video';
        video.controls = false;
        video.muted = true;
        video.style.width = '100%';
        video.style.height = '100%';
        video.style.objectFit = 'contain';
        video.style.display = 'none';

        wrapper.appendChild(label);
        wrapper.appendChild(noVideoMessage);
        wrapper.appendChild(video);
        container.appendChild(wrapper);

        videoPlayers.push(video);
    });

    isUpdatingGrid = false;

    if (pendingSeekTime !== null) {
        const time = new Date(pendingSeekTime);
        pendingSeekTime = null;
        console.log('Executing pending seek after grid update');
        seekToTime(time);
    } else if (currentTimePosition) {
        seekToTime(new Date(currentTimePosition));
    }
}

function previousSegment() {
    if (!currentTimePosition) {
        currentTimePosition = new Date().getTime();
    }

    const newTime = new Date(currentTimePosition);
    newTime.setMinutes(newTime.getMinutes() - 10);
    seekToTime(newTime);
}

function nextSegment() {
    if (!currentTimePosition) {
        currentTimePosition = new Date().getTime();
    }

    const newTime = new Date(currentTimePosition);
    newTime.setMinutes(newTime.getMinutes() + 10);
    seekToTime(newTime);
}

function updatePlaybackSpeed() {
    videoPlayers.forEach(video => {
        if (video) {
            video.playbackRate = playbackSpeed;
        }
    });
}

function setupKeyboardShortcuts() {
    document.addEventListener('keydown', (e) => {
        if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
            return;
        }

        if (e.code === 'Space') {
            e.preventDefault();
            togglePlayPause();
        } else if (e.code === 'ArrowLeft') {
            e.preventDefault();
            previousSegment();
        } else if (e.code === 'ArrowRight') {
            e.preventDefault();
            nextSegment();
        }
    });
}

// Download Modal Functions
function openDownloadModal() {
    if (!currentTimePosition) {
        alert('Please select a time on the timeline first');
        return;
    }

    const modal = document.getElementById('download-modal');
    if (!modal) return;

    downloadStartTime = new Date(currentTimePosition);
    downloadStartTime.setMinutes(downloadStartTime.getMinutes() - 10);

    downloadEndTime = new Date(currentTimePosition);
    downloadEndTime.setMinutes(downloadEndTime.getMinutes() + 10);

    downloadCameras = [...selectedCameras];

    updateDownloadTimeDisplay();
    renderDownloadCameraSelection();

    modal.style.display = 'block';
}

function closeDownloadModal() {
    const modal = document.getElementById('download-modal');
    if (modal) {
        modal.style.display = 'none';
    }
}

function updateDownloadTimeDisplay() {
    const startEl = document.getElementById('download-time-start');
    const endEl = document.getElementById('download-time-end');

    if (startEl) startEl.textContent = formatTimeForDisplay(downloadStartTime);
    if (endEl) endEl.textContent = formatTimeForDisplay(downloadEndTime);
}

function formatTimeForDisplay(date) {
    return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function adjustDownloadTime(delta) {
    downloadStartTime.setMinutes(downloadStartTime.getMinutes() + delta);
    downloadEndTime.setMinutes(downloadEndTime.getMinutes() + delta);
    updateDownloadTimeDisplay();
}

function resetDownloadTime() {
    if (!currentTimePosition) return;

    downloadStartTime = new Date(currentTimePosition);
    downloadStartTime.setMinutes(downloadStartTime.getMinutes() - 10);

    downloadEndTime = new Date(currentTimePosition);
    downloadEndTime.setMinutes(downloadEndTime.getMinutes() + 10);

    updateDownloadTimeDisplay();
}

function renderDownloadCameraSelection() {
    const container = document.getElementById('download-camera-list');
    if (!container) return;

    container.innerHTML = '';

    selectedCameras.forEach(cameraId => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;

        const label = document.createElement('label');
        label.className = 'camera-checkbox';

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.value = cameraId;
        checkbox.checked = downloadCameras.includes(cameraId);
        checkbox.onchange = () => toggleDownloadCamera(cameraId);

        label.appendChild(checkbox);
        label.appendChild(document.createTextNode(' ' + camera.name));
        container.appendChild(label);
    });
}

function toggleDownloadCamera(cameraId) {
    if (downloadCameras.includes(cameraId)) {
        downloadCameras = downloadCameras.filter(id => id !== cameraId);
    } else {
        downloadCameras.push(cameraId);
    }
    renderDownloadCameraSelection();
}

function startDownload() {
    if (downloadCameras.length === 0) {
        alert('Please select at least one camera to download');
        return;
    }

    downloadCameras.forEach(cameraId => {
        const camera = allCameras.find(c => c.id === cameraId);
        if (!camera) return;

        const startTimeStr = downloadStartTime.toISOString();
        const endTimeStr = downloadEndTime.toISOString();

        const url = `/api/download?camera_id=${encodeURIComponent(cameraId)}&start_time=${encodeURIComponent(startTimeStr)}&end_time=${encodeURIComponent(endTimeStr)}`;

        console.log('Starting download:', url);

        const a = document.createElement('a');
        a.href = url;
        a.download = `${camera.name}_${downloadStartTime.toISOString().split('T')[0]}.mp4`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
    });

    closeDownloadModal();
}

// Initialize on page load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initializeTimelinePlayback);
} else {
    initializeTimelinePlayback();
}
