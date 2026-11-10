// Timeline playback with versioned loading and absolute event navigation.
let selectedCameras = [], allCameras = [], timelineSegments = [], videoPlayers = [];
let currentDate = new Date(), gridLayout = 1, playbackSpeed = 1.0, isPlaying = false;
let currentTimePosition = null, isUpdatingGrid = false, pendingSeekTime = null, lastSeekTime = null;
let seekTimeoutId = null, loadDataTimeoutId = null, pendingVideoTimeouts = [];
let downloadStartTime = null, downloadEndTime = null, downloadCameras = [];
let timelineEvents = [], eventTooltip = null;
let timelineRequestVersion = 0, timelineAbort = null, pendingEventNavigation = null, cameraLoadVersion = 0;
let seekGeneration = 0, mediaCleanups = [], segmentCleanups = [];
const EVENT_ICONS = {'Motion Detected':'???','Person Detected':'??','Vehicle Detected':'??','Intrusion':'??','Loitering':'??','Object Removed':'??','Object Left Behind':'??','Sound Alert':'??','Line Crossing':'??','Tamper Alert':'??'};
let cameraColorMap = {};
const COLORS = ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#ec4899','#06b6d4','#84cc16','#f97316','#14b8a6','#a855f7','#f43f5e','#0ea5e9','#eab308','#6366f1','#d946ef'];
let timelineZoomLevel = 1.0, timelineOffset = 0, nextColorIndex = 0;
const CHUNK_SIZE_MS = 10 * 60 * 1000;
function localDateKey(date) {
  return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
}
// Construct each boundary as local calendar midnight, not start + 24 hours.
function timelineWindowForDay(day) {
  const match = typeof day === 'string' && /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) throw new RangeError('Invalid timeline day');
  const year = Number(match[1]), month = Number(match[2]), date = Number(match[3]);
  const start = new Date(0); start.setFullYear(year,month-1,date); start.setHours(0,0,0,0);
  const end = new Date(0); end.setFullYear(year,month-1,date+1); end.setHours(0,0,0,0);
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || localDateKey(start) !== day || end <= start || end-start > 26*3600000) throw new RangeError('Invalid timeline day');
  return {start_time:start.toISOString(),end_time:end.toISOString()};
}
// Compatibility alias; this has always formatted local calendar fields.
function getUTCDateString(date) { return localDateKey(date); }
function playbackError(message) { if (typeof showError === 'function') showError(message); else console.error(message); }
function getOrderedSelectedCameras() { return allCameras.filter(cam => selectedCameras.includes(cam.id)).map(cam => cam.id); }
function cancelPlaybackTasks() {
  seekGeneration++;
  if (seekTimeoutId !== null) clearTimeout(seekTimeoutId);
  seekTimeoutId = null;
  pendingVideoTimeouts.forEach(clearTimeout); pendingVideoTimeouts = [];
  mediaCleanups.splice(0).forEach(cleanup => cleanup());
  segmentCleanups.splice(0).forEach(cleanup => cleanup());
}
function invalidateTimelineLoad() {
  timelineRequestVersion++;
  if (timelineAbort) timelineAbort.abort(); timelineAbort = null;
  if (loadDataTimeoutId !== null) clearTimeout(loadDataTimeoutId); loadDataTimeoutId = null;
  pendingEventNavigation = null; cancelPlaybackTasks();
  return timelineRequestVersion;
}
function initializeTimelinePlayback() { setupDatePicker(); loadTimelineCameras(); setupKeyboardShortcuts(); setupTimelineControls(); setupTimelineZoom(); }
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
function navigateToEvent(cameraName,startTimeStr) {
  const time = typeof startTimeStr === 'string' && startTimeStr.trim() ? new Date(startTimeStr) : new Date(NaN);
  if (!Number.isFinite(time.getTime())) { playbackError('Invalid event timestamp'); return false; }
  const matches = allCameras.filter(cam => cam.name === cameraName);
  if (matches.length !== 1) { playbackError(matches.length ? 'Event camera name is ambiguous' : 'Event camera is missing or cameras have not loaded'); return false; }
  cameraLoadVersion++;
  const camera = matches[0];
  if (!selectedCameras.includes(camera.id)) selectedCameras.push(camera.id);
  saveCameraSelection(); gridLayout = selectedCameras.length;
  currentDate = new Date(time); currentDate.setHours(0,0,0,0);
  const picker = document.getElementById('timeline-date-picker'); if (picker) picker.value = localDateKey(time);
  currentTimePosition = null; pendingSeekTime = null; lastSeekTime = null;
  renderCameraSelector(); closeEventsModal();
  loadTimelineData({targetMs:time.getTime(),cameraId:camera.id});
  return true;
}
function loadTimelineData(navigation = null) {
  const version = invalidateTimelineLoad(), day = localDateKey(currentDate), cameras = [...selectedCameras];
  pendingEventNavigation = navigation; timelineSegments = []; pendingSeekTime = null;
  if (currentTimePosition !== null && localDateKey(new Date(currentTimePosition)) !== day) currentTimePosition = null;
  videoPlayers.forEach(video => { if (video) { video.pause(); video.removeAttribute('src'); } });
  loadTimelineEvents(day); renderTimeline();
  loadDataTimeoutId = setTimeout(() => { loadDataTimeoutId = null; performLoadTimelineData(version,day,cameras,navigation); },200);
  return version;
}
async function performLoadTimelineData(version = timelineRequestVersion,day = localDateKey(currentDate),cameras = [...selectedCameras],navigation = pendingEventNavigation) {
  if (version !== timelineRequestVersion) return false;
  const controller = new AbortController(); timelineAbort = controller;
  try {
    const bounds = timelineWindowForDay(day);
    const query = `start_time=${encodeURIComponent(bounds.start_time)}&end_time=${encodeURIComponent(bounds.end_time)}`;
    const response = await fetch(`/api/timeline/recordings?${query}`,{signal:controller.signal});
    if (!response.ok) throw new Error('Timeline request failed');
    const segments = await response.json();
    if (version !== timelineRequestVersion || controller.signal.aborted || localDateKey(currentDate) !== day || JSON.stringify(cameras) !== JSON.stringify(selectedCameras)) return false;
    if (!Array.isArray(segments)) throw new Error('Invalid timeline response');
    timelineSegments = segments.filter(segment => cameras.includes(segment.camera_id));
    const eventTarget = navigation && pendingEventNavigation === navigation;
    if (eventTarget) { pendingEventNavigation = null; currentTimePosition = null; pendingSeekTime = null; lastSeekTime = null; }
    loadTimelineEvents(day); updateVideoGrid(); renderTimeline();
    if (eventTarget) seekToTime(new Date(navigation.targetMs));
    return true;
  } catch (error) {
    if (version === timelineRequestVersion && !controller.signal.aborted) { pendingEventNavigation = null; playbackError('Unable to load recordings for the selected day'); }
    return false;
  } finally { if (timelineAbort === controller) timelineAbort = null; }
}
function timelineViewport() {
  const bounds = timelineWindowForDay(localDateKey(currentDate));
  const viewport = TimelineGeometry.view(Date.parse(bounds.start_time),Date.parse(bounds.end_time),timelineZoomLevel,timelineOffset);
  timelineOffset = viewport.offsetHours;
  return viewport;
}
function timelinePointer(canvas,event) {
  const rect=canvas.getBoundingClientRect();
  if (!(rect.width>0) || !(rect.height>0)) return null;
  return {fraction:Math.max(0,Math.min(1,(event.clientX-rect.left)/rect.width)),x:(event.clientX-rect.left)*canvas.width/rect.width,y:(event.clientY-rect.top)*canvas.height/rect.height};
}
function renderTimeline() {
  const canvas=document.getElementById('timeline-canvas'); if (!canvas) return;
  const ctx=canvas.getContext('2d'),width=canvas.width=canvas.offsetWidth,height=canvas.height=85,viewport=timelineViewport();
  ctx.fillStyle='#1a1a1a'; ctx.fillRect(0,0,width,height); ctx.lineWidth=1;
  ctx.font='10px monospace'; ctx.textAlign='left'; ctx.textBaseline='alphabetic';
  const steps=viewport.visibleHours>=12?[2,1]:viewport.visibleHours>=4?[1,0.5]:viewport.visibleHours>=2?[0.5,1/6]:[1/6,1/60];
  function ticks(step,major) {
    const stepMs=step*3600000; let previousLabelRight=-Infinity;
    for (let index=Math.ceil((viewport.start-viewport.dayStart)/stepMs);;index++) {
      const time=viewport.dayStart+index*stepMs; if (time>viewport.end) break;
      const x=TimelineGeometry.fractionAt(time,viewport)*width;
      ctx.strokeStyle=major?'#555':'#333';ctx.beginPath();ctx.moveTo(x,major?0:height-10);ctx.lineTo(x,height);ctx.stroke();
      if (major) {
        const label=TimelineGeometry.label(time),labelWidth=ctx.measureText(label).width;
        if (x+2>=previousLabelRight+6 && x+2+labelWidth<=width) {ctx.fillStyle='#888';ctx.fillText(label,x+2,12);previousLabelRight=x+2+labelWidth;}
      }
    }
  }
  ticks(steps[1],false);ticks(steps[0],true);
  timelineEvents.forEach(event=>{
    delete event._canvasX;delete event._canvasY;
    const time=Date.parse(event.start_time);
    if (!Number.isFinite(time) || time<viewport.start || time>=viewport.end) return;
    const x=TimelineGeometry.fractionAt(time,viewport)*width,y=23;
    ctx.strokeStyle='rgba(76, 175, 80, 0.3)';ctx.lineWidth=1;ctx.beginPath();ctx.moveTo(x,30);ctx.lineTo(x,height);ctx.stroke();
    ctx.fillStyle='#4CAF50';ctx.beginPath();ctx.arc(x,y,6,0,Math.PI*2);ctx.fill();
    ctx.font='10px Arial';ctx.textAlign='center';ctx.textBaseline='middle';ctx.fillStyle='#fff';ctx.fillText(eventIconText(event.label),x,y);
    event._canvasX=x;event._canvasY=y;event._radius=6;
  });
  const cameras=[...new Set(timelineSegments.map(segment=>segment.camera_id))],blockHeight=(height-32)/Math.max(cameras.length,1);
  cameras.forEach((id,index)=>timelineSegments.filter(segment=>segment.camera_id===id).forEach(segment=>{
    const span=TimelineGeometry.clip(Date.parse(segment.start_time),Date.parse(segment.end_time),viewport);if (!span) return;
    ctx.fillStyle=cameraColorMap[id] || '#888';ctx.fillRect(span.left*width,32+index*blockHeight,(span.right-span.left)*width,blockHeight-2);
  }));
  if (Number.isFinite(currentTimePosition) && currentTimePosition>=viewport.start && currentTimePosition<viewport.end) {
    const x=TimelineGeometry.fractionAt(currentTimePosition,viewport)*width;
    ctx.strokeStyle='#fff';ctx.lineWidth=2;ctx.beginPath();ctx.moveTo(x,0);ctx.lineTo(x,height);ctx.stroke();ctx.lineWidth=1;
  }
}
function setupTimelineZoom() {
  const canvas=document.getElementById('timeline-canvas');if (!canvas) return;
  canvas.addEventListener('wheel',e=>{
    e.preventDefault();const pointer=timelinePointer(canvas,e);if (!pointer) return;
    const zoom=Math.max(1,Math.min(24,timelineZoomLevel*(e.deltaY>0?0.8:1.25)));if (zoom===timelineZoomLevel) return;
    const viewport=TimelineGeometry.zoomAt(timelineViewport(),pointer.fraction,zoom);
    timelineZoomLevel=zoom;timelineOffset=viewport.offsetHours;renderTimeline();
  },{passive:false});
}
function setupTimelineControls() {
  const canvas=document.getElementById('timeline-canvas');if (!canvas) return;
  function hit(pointer) {return timelineEvents.find(event=>Number.isFinite(event._canvasX) && Number.isFinite(event._canvasY) && Math.hypot(pointer.x-event._canvasX,pointer.y-event._canvasY)<=event._radius+5);}
  canvas.addEventListener('click',e=>{
    const pointer=timelinePointer(canvas,e);if (!pointer) return;const event=hit(pointer);
    if (event) {navigateToEvent(event.camera_name,event.start_time);return;}
    const viewport=timelineViewport();seekToTime(new Date(Math.min(viewport.dayEnd-1,TimelineGeometry.timeAt(pointer.fraction,viewport))));
  });
  canvas.addEventListener('mousemove',e=>{
    const pointer=timelinePointer(canvas,e),event=pointer && hit(pointer);
    canvas.style.cursor=event?'pointer':'default';if (event) showEventTooltip(event,e.clientX,e.clientY);else hideEventTooltip();
  });
  canvas.addEventListener('mouseleave',hideEventTooltip);
}
function showEventTooltip(event,x,y) {
  hideEventTooltip(); const tooltip=document.createElement('div'); tooltip.id='event-tooltip';
  tooltip.style.cssText='position:fixed;background:rgba(0,0,0,0.9);color:#fff;padding:8px 12px;border-radius:6px;font-size:12px;z-index:10000;pointer-events:none;max-width:250px;box-shadow:0 2px 8px rgba(0,0,0,0.3);';
  tooltip.style.left=(x+10)+'px'; tooltip.style.top=(y+10)+'px';
  tooltip.appendChild(eventTextElement('div',`${eventIconText(event.label)} ${event.label ?? ''}`,'font-weight:bold;color:#4CAF50;margin-bottom:4px;'));
  const details=document.createElement('div'); details.style.cssText='font-size:11px;color:#ccc;';
  details.appendChild(eventTextElement('div',`?? ${event.camera_name ?? ''}`)); details.appendChild(eventTextElement('div',`?? ${new Date(event.start_time).toLocaleTimeString()}`)); details.appendChild(eventTextElement('div',`?? ${event.duration_seconds || event.duration}s`));
  tooltip.appendChild(details); tooltip.appendChild(eventTextElement('div','Click to view recording','font-size:10px;color:#888;margin-top:4px;font-style:italic;'));
  document.body.appendChild(tooltip); eventTooltip=tooltip;
}
function hideEventTooltip() { if (eventTooltip) { eventTooltip.remove(); eventTooltip=null; } }
function seekToTime(time) {
  if (!time || typeof time.getTime!=='function' || !Number.isFinite(time.getTime())) { playbackError('Invalid playback timestamp'); return false; }
  const milliseconds=time.getTime();
  pendingEventNavigation=null; cancelPlaybackTasks();
  if (isUpdatingGrid) { pendingSeekTime=milliseconds; return true; }
  lastSeekTime=milliseconds; currentTimePosition=milliseconds;
  const generation=seekGeneration;
  seekTimeoutId=setTimeout(() => { seekTimeoutId=null; if (generation===seekGeneration) performSeek(new Date(milliseconds),generation); },100);
  return true;
}
function scheduleVideoTask(fn,delay,generation) {
  const timer=setTimeout(() => { pendingVideoTimeouts=pendingVideoTimeouts.filter(id=>id!==timer); if (generation===seekGeneration) fn(); },delay);
  pendingVideoTimeouts.push(timer);
}
// This first continuation policy deliberately excludes multi-camera playback.
function attachSingleCameraContinuation(video,index,segment,generation) {
  const cameras=getOrderedSelectedCameras();
  if (cameras.length!==1 || cameras[0]!==segment.camera_id) return;
  const day=localDateKey(currentDate),bounds=timelineWindowForDay(day),dayEnd=Date.parse(bounds.end_time);
  const start=Date.parse(segment.start_time),end=Date.parse(segment.end_time);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end<=start) return;
  const valid=()=>generation===seekGeneration && videoPlayers[index]===video && localDateKey(currentDate)===day && getOrderedSelectedCameras().length===1 && getOrderedSelectedCameras()[0]===segment.camera_id;
  function stop(time,message) {
    cancelPlaybackTasks();isPlaying=false;videoPlayers.forEach(player=>{if(player)player.pause();});
    currentTimePosition=Math.min(time,dayEnd-1);
    const button=document.getElementById('play-pause');if(button)button.textContent='? Play';
    renderTimeline();playbackError(message);
  }
  const update=()=>{
    if (!valid() || !isPlaying || !Number.isFinite(video.currentTime)) return;
    const time=start+Math.max(0,video.currentTime)*1000;
    if (time>=dayEnd) {stop(dayEnd,'Selected playback day ended; select another day to continue');return;}
    currentTimePosition=Math.min(time,end);renderTimeline();
  };
  const ended=()=>{
    if (!valid() || !isPlaying || !video.ended) return;
    if (end>=dayEnd) {stop(dayEnd,'Selected playback day ended; select another day to continue');return;}
    const next=timelineSegments.filter(item=>item.camera_id===segment.camera_id && Date.parse(item.start_time)<=end && Date.parse(item.end_time)>end);
    if (next.length!==1 || Date.parse(next[0].start_time)!==end || !next[0].file_path || next[0].file_path===segment.file_path) {
      stop(end,'No unique contiguous recording; select another time to continue');return;
    }
    cancelPlaybackTasks();currentTimePosition=end;lastSeekTime=end;
    performSeek(new Date(end),seekGeneration);
  };
  const cleanup=()=>{video.removeEventListener('timeupdate',update);video.removeEventListener('ended',ended);segmentCleanups=segmentCleanups.filter(fn=>fn!==cleanup);};
  segmentCleanups.push(cleanup);video.addEventListener('timeupdate',update);video.addEventListener('ended',ended);
}
function performSeek(time,generation=seekGeneration) {
  if (generation!==seekGeneration) return;
  getOrderedSelectedCameras().forEach((id,index) => scheduleVideoTask(() => {
    const video=videoPlayers[index]; if (!video) return;
    const segment=timelineSegments.find(s=>s.camera_id===id && time.getTime()>=new Date(s.start_time).getTime() && time.getTime()<new Date(s.end_time).getTime());
    const message=video.parentElement?.querySelector('.no-video-message');
    video.pause(); video.removeAttribute('src'); video.style.display=segment?'block':'none'; if (message) message.style.display=segment?'none':'block';
    if (!segment) return;
    scheduleVideoTask(() => {
      if (videoPlayers[index]!==video) return;
      const offset=(time.getTime()-new Date(segment.start_time).getTime())/1000;
      const cleanup=() => { video.removeEventListener('loadedmetadata',metadata); video.removeEventListener('error',failed); mediaCleanups=mediaCleanups.filter(fn=>fn!==cleanup); };
      const metadata=() => {
        cleanup(); if (generation!==seekGeneration || videoPlayers[index]!==video) return;
        video.currentTime=Math.max(0,Math.min(offset,video.duration)); video.playbackRate=playbackSpeed;
        attachSingleCameraContinuation(video,index,segment,generation);
        if (isPlaying) { const promise=video.play(); if (promise!==undefined) promise.catch(()=>{}); }
      };
      const failed=() => { cleanup(); if (generation!==seekGeneration || videoPlayers[index]!==video) return; video.style.display='none'; if (message) message.style.display='block'; };
      mediaCleanups.push(cleanup); video.addEventListener('loadedmetadata',metadata); video.addEventListener('error',failed); video.src=segment.file_path;
    },10,generation);
  },index*150,generation));
  renderTimeline();
  if (!isPlaying) { isPlaying=true; const button=document.getElementById('play-pause'); if (button) button.textContent='? Pause'; }
}
function togglePlayPause() {
  isPlaying=!isPlaying; videoPlayers.forEach(video=>{ if (!video) return; if (isPlaying) { const promise=video.play(); if (promise!==undefined) promise.catch(()=>{}); } else video.pause(); });
  const button=document.getElementById('play-pause'); if (button) button.textContent=isPlaying?'? Pause':'? Play';
}
function toggleFullscreen() {
  const container=document.getElementById('fullscreen-wrapper'); if (!container) return;
  const promise=document.fullscreenElement?document.exitFullscreen():container.requestFullscreen(); promise.catch(error=>console.error('Fullscreen failed:',error));
}
function toggleMuteAll() {
  const muted=videoPlayers.filter(Boolean).every(video=>video.muted); videoPlayers.forEach(video=>{ if (video) video.muted=!muted; });
  const button=document.getElementById('mute-all-btn'); if (button) button.textContent=muted?'?? Unmute All':'?? Mute All';
}
function updateVideoGrid() {
  const container=document.getElementById('video-grid'); if (!container) return;
  cancelPlaybackTasks(); isUpdatingGrid=true; videoPlayers.forEach(video=>{ if (video) { video.pause(); video.removeAttribute('src'); } });
  container.innerHTML=''; videoPlayers=[];
  const ordered=getOrderedSelectedCameras(), count=ordered.length;
  if (!count) { container.innerHTML='<div style="padding:40px;text-align:center;color:#888;font-size:18px;">Select cameras to begin playback</div>'; isUpdatingGrid=false; return; }
  const columns=count===1?1:count<=4?2:count<=9?3:4;
  container.style.display='grid'; container.style.gap='10px'; container.style.gridTemplateColumns=`repeat(${columns}, 1fr)`;
  ordered.forEach(id=>{
    const camera=allCameras.find(cam=>cam.id===id); if (!camera) return;
    const wrapper=document.createElement('div'); wrapper.className='video-cell'; wrapper.style.cssText='position:relative;background-color:#1a1a1a;aspect-ratio:16/9;';
    const label=document.createElement('div'); label.className='video-label'; label.textContent=camera.name; label.style.backgroundColor=cameraColorMap[id] || '#888';
    const message=document.createElement('div'); message.className='no-video-message'; message.style.cssText='position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);text-align:center;color:#888;font-size:18px;pointer-events:none;z-index:10;display:none;';
    message.innerHTML='<div>No Recording Available</div><div style="font-size:14px;margin-top:10px;">Select a different time</div>';
    const video=document.createElement('video'); video.className='timeline-video'; video.controls=false; video.muted=true; video.style.cssText='width:100%;height:100%;object-fit:contain;display:none;';
    wrapper.appendChild(label); wrapper.appendChild(message); wrapper.appendChild(video); container.appendChild(wrapper); videoPlayers.push(video);
  });
  isUpdatingGrid=false;
  if (pendingSeekTime!==null) { const time=new Date(pendingSeekTime); pendingSeekTime=null; seekToTime(time); }
  else if (currentTimePosition!==null) seekToTime(new Date(currentTimePosition));
}
function previousSegment() { const time=new Date(currentTimePosition ?? Date.now()); time.setMinutes(time.getMinutes()-10); seekToTime(time); }
function nextSegment() { const time=new Date(currentTimePosition ?? Date.now()); time.setMinutes(time.getMinutes()+10); seekToTime(time); }
function updatePlaybackSpeed() { videoPlayers.forEach(video=>{ if (video) video.playbackRate=playbackSpeed; }); }
function setupKeyboardShortcuts() {
  document.addEventListener('keydown',e=>{ if (e.target.tagName==='INPUT' || e.target.tagName==='TEXTAREA') return;
    if (e.code==='Space') { e.preventDefault(); togglePlayPause(); } else if (e.code==='ArrowLeft') { e.preventDefault(); previousSegment(); } else if (e.code==='ArrowRight') { e.preventDefault(); nextSegment(); }
  });
}
function openDownloadModal() {
  if (currentTimePosition===null) { playbackError('Please select a time on the timeline first'); return; }
  const modal=document.getElementById('download-modal'); if (!modal) return;
  downloadStartTime=new Date(currentTimePosition); downloadStartTime.setMinutes(downloadStartTime.getMinutes()-10);
  downloadEndTime=new Date(currentTimePosition); downloadEndTime.setMinutes(downloadEndTime.getMinutes()+10);
  downloadCameras=[...selectedCameras]; updateDownloadTimeDisplay(); renderDownloadCameraSelection(); modal.classList.add('active'); modal.style.display='block';
}
function closeDownloadModal() { const modal=document.getElementById('download-modal'); if (modal) { modal.classList.remove('active'); modal.style.display='none'; } }
function updateDownloadTimeDisplay() {
  const display=document.getElementById('download-time-display'), duration=document.getElementById('download-duration-display');
  if (display && downloadStartTime && downloadEndTime) display.textContent=`${formatTimeForDisplay(downloadStartTime)} - ${formatTimeForDisplay(downloadEndTime)}`;
  if (duration && downloadStartTime && downloadEndTime) duration.textContent=`Total duration: ${Math.round((downloadEndTime-downloadStartTime)/60000)} min`;
}
function expandDownloadTimeEarlier() { if (downloadStartTime) { downloadStartTime.setMinutes(downloadStartTime.getMinutes()-10); updateDownloadTimeDisplay(); } }
function expandDownloadTimeLater() { if (downloadEndTime) { downloadEndTime.setMinutes(downloadEndTime.getMinutes()+10); updateDownloadTimeDisplay(); } }
function resetDownloadTimeRange() {
  if (currentTimePosition===null) return;
  downloadStartTime=new Date(currentTimePosition); downloadStartTime.setMinutes(downloadStartTime.getMinutes()-10);
  downloadEndTime=new Date(currentTimePosition); downloadEndTime.setMinutes(downloadEndTime.getMinutes()+10); updateDownloadTimeDisplay();
}
function formatTimeForDisplay(date) { return date.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit',second:'2-digit'}); }
function renderDownloadCameraSelection() {
  const container=document.getElementById('download-camera-list'); if (!container) return; container.innerHTML='';
  selectedCameras.forEach(id=>{ const camera=allCameras.find(cam=>cam.id===id); if (!camera) return;
    const label=document.createElement('label'); label.className='camera-checkbox'; label.style.cssText='display:block;margin:5px 0;cursor:pointer;';
    const checkbox=document.createElement('input'); checkbox.type='checkbox'; checkbox.value=id; checkbox.checked=downloadCameras.includes(id); checkbox.onchange=()=>toggleDownloadCamera(id);
    label.appendChild(checkbox); label.appendChild(document.createTextNode(' '+camera.name)); container.appendChild(label);
  });
}
function toggleDownloadCamera(id) { downloadCameras=downloadCameras.includes(id)?downloadCameras.filter(camera=>camera!==id):[...downloadCameras,id]; renderDownloadCameraSelection(); }
if (document.readyState==='loading') document.addEventListener('DOMContentLoaded',initializeTimelinePlayback); else initializeTimelinePlayback();
