function loadTimelineEvents(date) {
  timelineEvents = typeof allEvents !== 'undefined' && Array.isArray(allEvents) ? allEvents.filter(event => {
    const time = new Date(event.start_time); return Number.isFinite(time.getTime()) && localDateKey(time) === date;
  }) : [];
}
function loadTimelineCameras() {
  const version = ++cameraLoadVersion; invalidateTimelineLoad();
  return fetch('/api/cameras').then(response => { if (!response.ok) throw new Error('Camera request failed'); return response.text(); }).then(html => {
    if (version !== cameraLoadVersion) return;
    const doc = new DOMParser().parseFromString(html,'text/html');
    allCameras = [];
    doc.querySelectorAll('.camera-box').forEach(box => {
      const name = box.querySelector('.camera-title'); const id = box.getAttribute('data-camera-id');
      if (id && name) allCameras.push({id,name:name.textContent});
    });
    try {
      const saved = JSON.parse(localStorage.getItem('selectedCameras') || 'null');
      selectedCameras = Array.isArray(saved) ? saved.filter(id => allCameras.some(cam => cam.id === id)) : allCameras.slice(0,4).map(cam => cam.id);
    } catch (_) { selectedCameras = allCameras.slice(0,4).map(cam => cam.id); }
    if (!selectedCameras.length && allCameras.length) selectedCameras = allCameras.slice(0,4).map(cam => cam.id);
    renderCameraSelector(); loadTimelineData();
  }).catch(error => { if (version === cameraLoadVersion) playbackError('Unable to load cameras for playback'); });
}
function renderCameraSelector() {
  const container = document.getElementById('timeline-camera-selector'); if (!container) return;
  allCameras.forEach(cam => { if (!cameraColorMap[cam.id]) cameraColorMap[cam.id] = COLORS[nextColorIndex++ % COLORS.length]; });
  container.innerHTML = '<div class="camera-dropdown"><button class="dropdown-toggle" id="camera-dropdown-toggle" type="button"><span id="dropdown-label">Select Cameras</span><span class="dropdown-arrow">&#9660;</span></button><div class="dropdown-menu" id="camera-dropdown-menu"></div></div>';
  const toggle = document.getElementById('camera-dropdown-toggle'), menu = document.getElementById('camera-dropdown-menu');
  allCameras.forEach(cam => {
    const option = document.createElement('div'); option.className = 'dropdown-option'; option.dataset.cameraId = cam.id;
    const checkbox = document.createElement('input'); checkbox.type = 'checkbox'; checkbox.checked = selectedCameras.includes(cam.id); checkbox.id = `timeline-cam-${cam.id}`;
    const label = document.createElement('label'); label.htmlFor = checkbox.id; label.textContent = cam.name;
    label.style.cssText = 'padding:4px 10px;border-radius:4px;color:#fff;margin-left:8px;cursor:pointer;display:inline-block;'; label.style.backgroundColor = cameraColorMap[cam.id] || COLORS[0];
    option.appendChild(checkbox); option.appendChild(label);
    option.onclick = e => { if (e.target === checkbox) return; e.stopPropagation(); checkbox.checked = !checkbox.checked; toggleCamera(cam.id); };
    checkbox.onchange = e => { e.stopPropagation(); toggleCamera(cam.id); }; menu.appendChild(option);
  });
  toggle.onclick = e => { e.stopPropagation(); menu.classList.toggle('show'); };
  const closeHandler = e => { if (!container.contains(e.target)) menu.classList.remove('show'); };
  document.removeEventListener('click',closeHandler); document.addEventListener('click',closeHandler); updateDropdownLabel();
}
function updateDropdownLabel() {
  const label = document.getElementById('dropdown-label'); if (!label) return;
  const count = selectedCameras.length, cam = allCameras.find(c => c.id === selectedCameras[0]);
  label.textContent = count === 0 ? 'Select Cameras' : count === 1 ? (cam ? cam.name : '1 camera') : `${count} cameras selected`;
}
function saveCameraSelection() { try { localStorage.setItem('selectedCameras',JSON.stringify(selectedCameras)); } catch (_) { console.warn('Unable to save camera selection'); } }
function toggleCamera(cameraId) {
  selectedCameras = selectedCameras.includes(cameraId) ? selectedCameras.filter(id => id !== cameraId) : [...selectedCameras,cameraId];
  if (!cameraColorMap[cameraId]) cameraColorMap[cameraId] = COLORS[nextColorIndex++ % COLORS.length];
  saveCameraSelection(); gridLayout = selectedCameras.length; updateDropdownLabel(); loadTimelineData(); updateVideoGrid();
  if (typeof refreshExportCameraGuidance === 'function') refreshExportCameraGuidance();
}
function setupDatePicker() {
  const picker = document.getElementById('timeline-date-picker'); if (!picker) return;
  currentDate = new Date(); currentDate.setHours(0,0,0,0); picker.value = localDateKey(currentDate);
  picker.onchange = () => {
    const [y,m,d] = picker.value.split('-').map(Number), date = new Date(y,m-1,d);
    if (!Number.isFinite(date.getTime()) || localDateKey(date) !== picker.value) { playbackError('Invalid playback date'); return; }
    currentDate = date; loadTimelineData();
  };
}
