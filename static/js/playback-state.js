// Timeline playback with versioned loading and absolute event navigation.
let selectedCameras = [], allCameras = [], timelineSegments = [], videoPlayers = [];
let currentDate = new Date(), gridLayout = 1, playbackSpeed = 1.0, isPlaying = false;
let currentTimePosition = null, isUpdatingGrid = false, pendingSeekTime = null, lastSeekTime = null;
let seekTimeoutId = null, loadDataTimeoutId = null, pendingVideoTimeouts = [];
let downloadStartTime = null, downloadEndTime = null;
let timelineEvents = [], eventTooltip = null;
let timelineRequestVersion = 0, timelineAbort = null, pendingEventNavigation = null, cameraLoadVersion = 0;
let playbackIntentVersion = 0;
const playbackSessions = new Map();
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
  seekGeneration++; playbackIntentVersion++;
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
  isPlaying=false;videoPlayers.forEach(video=>{if(video)video.pause();});setPlaybackButton();
  return timelineRequestVersion;
}
function initializeTimelinePlayback() { setPlaybackButton(); setupDatePicker(); loadTimelineCameras(); setupKeyboardShortcuts(); setupTimelineControls(); setupTimelineZoom(); }
