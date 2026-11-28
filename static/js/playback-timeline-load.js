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
