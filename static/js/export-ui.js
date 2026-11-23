// Job-based exports. This page only fetches small, bounded control JSON; it never
// fetches or buffers video. A prepared file is saved through a normal same-origin
// link to /api/exports/download, handled by the browser's own download manager.
//
// One export is queued per camera currently selected for playback. Users leave a
// camera out by deselecting it in the playback camera selector.
let exportUiBatch = null, exportInitialRange = null, exportMode = 'copy', exportPanelOpen = false;
const EXPORT_RANGE_STEPS = [1000, 10000, 60000, 600000];
const EXPORT_JOB_POLL_MS = 1500, EXPORT_JOB_REQUEST_MS = 45000, EXPORT_JOB_WAIT_MS = 7200000;
const EXPORT_CONTROL_LIMIT = 8192, EXPORT_LIST_LIMIT = 65536, EXPORT_LIST_MAX_JOBS = 64, EXPORT_MAX_CAMERAS = 8;
const EXPORT_MIN_RANGE_MS = 1000;
const EXPORT_STATES = ['queued', 'preparing', 'ready', 'failed', 'canceled', 'expired'];
// Server job id -> camera name and requested range, for jobs this page created.
const exportUiKnownJobs = new Map();

function exportUiNode(tag, text = '') { const e = document.createElement(tag); e.textContent = String(text); return e; }
function exportUiText(text) { return String(text ?? '').replace(/[\u0000-\u001f\u007f]/g, ' ').slice(0, 600); }
function exportUiSize(bytes) {
  if (!Number.isSafeInteger(bytes) || bytes < 0) return 'unknown size';
  if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MiB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${bytes} bytes`;
}
function exportBoundaryLabel(date) {
  return date.toLocaleString(undefined, {year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', timeZoneName: 'short', hour12: false});
}
function exportUiBusy() { return !!exportUiBatch?.running; }

// ---- Range selection -------------------------------------------------------------

function exportRangeCandidate(a, b, which, delta) {
  if (!Number.isFinite(a) || !Number.isFinite(b) || b <= a || !['start', 'end'].includes(which) || !Number.isFinite(delta)) return null;
  const next = (which === 'start' ? a : b) + delta;
  return !Number.isFinite(new Date(next).getTime()) || (which === 'start' ? next >= b : next <= a) ? null : next;
}
function exportRangeStep() {
  const n = Number(document.getElementById('download-range-step')?.value);
  return EXPORT_RANGE_STEPS.includes(n) ? n : 60000;
}
function exportSetBoundary(which, ms) {
  if (which === 'start') downloadStartTime = new Date(ms); else downloadEndTime = new Date(ms);
  updateDownloadTimeDisplay();
}
function adjustDownloadBoundary(which, direction) {
  if (exportUiBusy() || ![-1, 1].includes(direction)) return false;
  const next = exportRangeCandidate(downloadStartTime?.getTime(), downloadEndTime?.getTime(), which, direction * exportRangeStep());
  if (next === null) return false;
  exportSetBoundary(which, next);
  return true;
}
// Clamp a boundary so start < end by at least EXPORT_MIN_RANGE_MS. Returns null when
// the range is invalid or the boundary would not move.
function exportClampBoundary(which, ms) {
  const start = downloadStartTime?.getTime(), end = downloadEndTime?.getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start || !Number.isFinite(ms)) return null;
  const next = Math.round(which === 'start' ? Math.min(ms, end - EXPORT_MIN_RANGE_MS) : Math.max(ms, start + EXPORT_MIN_RANGE_MS));
  if (!Number.isFinite(new Date(next).getTime())) return null;
  return next === (which === 'start' ? start : end) ? null : next;
}
function exportSetMode(mode) {
  if (exportUiBusy() || !['copy', 'accurate'].includes(mode)) return false;
  exportMode = mode;
  updateDownloadTimeDisplay();
  return true;
}

function installExportRangeControls() {
  if (document.getElementById('download-range-step')) return;
  const host = document.getElementById('export-range-controls'); if (!host) return;
  const group = exportUiNode('div'); group.className = 'export-range-controls';
  const boundaries = exportUiNode('div'); boundaries.className = 'export-range-boundaries';
  for (const which of ['start', 'end']) {
    const row = exportUiNode('div'); row.className = 'export-range-boundary';
    row.appendChild(exportUiNode('span', which === 'start' ? 'Start' : 'End'));
    for (const direction of [-1, 1]) {
      const button = exportUiNode('button', direction < 0 ? 'Earlier' : 'Later');
      button.type = 'button'; button.className = 'btn btn-small';
      button.id = `download-${which}-${direction < 0 ? 'earlier' : 'later'}`;
      button.setAttribute('aria-label', `Move ${which} ${direction < 0 ? 'earlier' : 'later'}`);
      button.addEventListener('click', () => adjustDownloadBoundary(which, direction));
      row.appendChild(button);
      if (direction < 0) { const out = exportUiNode('output'); out.id = `download-${which}-display`; row.appendChild(out); }
    }
    boundaries.appendChild(row);
  }
  group.appendChild(boundaries);

  const settings = exportUiNode('div'); settings.className = 'export-range-settings';
  const stepLabel = exportUiNode('label', 'Step '), step = exportUiNode('select');
  step.id = 'download-range-step';
  ['1 second', '10 seconds', '1 minute', '10 minutes'].forEach((text, i) => { const o = exportUiNode('option', text); o.value = String(EXPORT_RANGE_STEPS[i]); step.appendChild(o); });
  step.value = '60000'; step.addEventListener('change', updateDownloadTimeDisplay);
  stepLabel.appendChild(step); settings.appendChild(stepLabel);

  const toggle = exportUiNode('div'); toggle.id = 'export-mode-toggle'; toggle.className = 'export-mode-toggle';
  toggle.setAttribute('role', 'group'); toggle.setAttribute('aria-label', 'Export mode');
  for (const [mode, text] of [['copy', 'Copy (original quality)'], ['accurate', 'Accurate (re-encode)']]) {
    const button = exportUiNode('button', text);
    button.type = 'button'; button.id = `export-mode-${mode}`; button.className = 'export-mode-option';
    button.setAttribute('aria-pressed', String(mode === exportMode));
    button.addEventListener('click', () => exportSetMode(mode));
    toggle.appendChild(button);
  }
  settings.appendChild(toggle);
  const reset = exportUiNode('button', 'Reset range'); reset.id = 'download-range-reset'; reset.type = 'button'; reset.className = 'btn btn-small';
  reset.addEventListener('click', resetDownloadTimeRange);
  settings.appendChild(reset);
  group.appendChild(settings);

  const duration = exportUiNode('p'); duration.id = 'download-duration-display'; duration.setAttribute('aria-live', 'polite');
  group.appendChild(duration);
  const help = exportUiNode('p', 'Copy keeps the original encoded video and audio. If a cut is not at a safe keyframe, the server extends the range outward to the nearest one (at most 30 seconds per end) and reports the saved range. Accurate cuts exactly but re-encodes, which is slower and may change colour range. The end time is exclusive.');
  help.id = 'export-mode-help'; help.className = 'export-help';
  group.appendChild(help);
  host.replaceChildren(group);
}

function refreshExportCameraGuidance() {
  const guidance = document.getElementById('export-camera-guidance'); if (!guidance) return;
  const ids = [...new Set(typeof selectedCameras === 'undefined' ? [] : selectedCameras)];
  const names = ids.map(id => allCameras.find(camera => camera.id === id)?.name ?? id).map(name => exportUiText(name).slice(0, 80));
  if (!ids.length) guidance.textContent = 'No cameras are selected for playback. Select the cameras to export in the Cameras selector above.';
  else if (ids.length > EXPORT_MAX_CAMERAS) guidance.textContent = `${ids.length} cameras are selected for playback; exports allow at most ${EXPORT_MAX_CAMERAS}. Deselect some in the Cameras selector above.`;
  else guidance.textContent = `Exports every camera selected for playback (${ids.length}): ${names.join(', ')}. Deselect a camera in the Cameras selector above to leave it out.`;
}

function updateDownloadTimeDisplay() {
  const a = downloadStartTime?.getTime(), b = downloadEndTime?.getTime(), valid = Number.isFinite(a) && Number.isFinite(b) && b > a, busy = exportUiBusy();
  for (const which of ['start', 'end']) {
    const display = document.getElementById(`download-${which}-display`);
    if (display) display.textContent = valid ? exportBoundaryLabel(which === 'start' ? downloadStartTime : downloadEndTime) : 'Invalid range';
    for (const direction of [-1, 1]) {
      const button = document.getElementById(`download-${which}-${direction < 0 ? 'earlier' : 'later'}`);
      if (button) button.disabled = busy || exportRangeCandidate(a, b, which, direction * exportRangeStep()) === null;
    }
  }
  const duration = document.getElementById('download-duration-display');
  if (duration) duration.textContent = valid ? `Duration: ${(b - a) / 1000} seconds (end is exclusive).` : 'Select a time on the timeline to set the export range.';
  for (const id of ['download-range-step', 'download-range-reset', 'export-mode-copy', 'export-mode-accurate']) {
    const element = document.getElementById(id); if (element) element.disabled = busy;
  }
  for (const mode of ['copy', 'accurate']) document.getElementById(`export-mode-${mode}`)?.setAttribute('aria-pressed', String(exportMode === mode));
  const submit = document.getElementById('export-start-btn'); if (submit) submit.disabled = busy;
  refreshExportCameraGuidance();
  renderExportRangeOverlay();
}

// ---- Floating range bar over the timeline ------------------------------------------

function exportTimelineReady() { return typeof TimelineGeometry !== 'undefined' && typeof timelineViewport === 'function'; }
function installExportRangeOverlay() {
  const host = document.getElementById('timeline-range-overlay');
  if (!host || host.dataset.installed) return;
  host.dataset.installed = 'true';
  const band = exportUiNode('div'); band.id = 'timeline-range-band'; band.className = 'timeline-range-band';
  host.appendChild(band);
  for (const [which, label] of [['start', 'Export start'], ['end', 'Export end']]) {
    const handle = exportUiNode('button');
    handle.id = `timeline-range-${which}-handle`; handle.type = 'button'; handle.className = `timeline-range-handle ${which}`;
    handle.setAttribute('role', 'slider'); handle.setAttribute('aria-label', label); handle.setAttribute('aria-orientation', 'horizontal');
    handle.setAttribute('aria-valuemin', '0'); handle.setAttribute('aria-valuemax', '100'); handle.setAttribute('aria-valuenow', '0');
    let dragging = false;
    const move = event => {
      if (!dragging || exportUiBusy() || !exportTimelineReady()) return;
      const rect = document.getElementById('timeline-canvas')?.getBoundingClientRect();
      if (!rect || !(rect.width > 0) || !Number.isFinite(event.clientX)) return;
      const fraction = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
      const next = exportClampBoundary(which, TimelineGeometry.timeAt(fraction, timelineViewport()));
      if (next !== null) exportSetBoundary(which, next);
    };
    const stop = event => {
      if (!dragging) return;
      dragging = false;
      try { handle.releasePointerCapture?.(event.pointerId); } catch (_) {}
    };
    handle.addEventListener('pointerdown', event => {
      if (exportUiBusy() || handle.disabled) return;
      dragging = true;
      try { handle.setPointerCapture?.(event.pointerId); } catch (_) {}
      event.preventDefault(); event.stopPropagation();
      move(event);
    });
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', stop);
    handle.addEventListener('pointercancel', stop);
    // Keep pointer interaction on the handle from also seeking the timeline.
    handle.addEventListener('click', event => event.stopPropagation());
    handle.addEventListener('keydown', event => {
      if (exportUiBusy() || !['ArrowLeft', 'ArrowRight', 'PageDown', 'PageUp', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault(); event.stopPropagation();
      const current = (which === 'start' ? downloadStartTime : downloadEndTime)?.getTime();
      if (!Number.isFinite(current)) return;
      const step = exportRangeStep() * (event.key === 'PageUp' || event.key === 'PageDown' ? 10 : 1);
      let target;
      if (event.key === 'Home' || event.key === 'End') {
        if (!exportTimelineReady()) return;
        const viewport = timelineViewport();
        target = event.key === 'Home' ? viewport.start : viewport.end;
      } else target = current + (event.key === 'ArrowLeft' || event.key === 'PageDown' ? -step : step);
      const next = exportClampBoundary(which, target);
      if (next !== null) exportSetBoundary(which, next);
    });
    host.appendChild(handle);
  }
}
function renderExportRangeOverlay() {
  const overlay = document.getElementById('timeline-range-overlay');
  if (!overlay || !exportTimelineReady()) return;
  installExportRangeOverlay();
  const start = downloadStartTime?.getTime(), end = downloadEndTime?.getTime();
  const valid = exportPanelOpen && Number.isFinite(start) && Number.isFinite(end) && end > start;
  overlay.hidden = !valid;
  if (!valid) return;
  const viewport = timelineViewport();
  const left = Math.max(0, Math.min(1, TimelineGeometry.fractionAt(start, viewport)));
  const right = Math.max(0, Math.min(1, TimelineGeometry.fractionAt(end, viewport)));
  const band = document.getElementById('timeline-range-band');
  if (band) { band.style.left = `${left * 100}%`; band.style.width = `${Math.max(0, right - left) * 100}%`; }
  const busy = exportUiBusy();
  for (const [which, fraction, date] of [['start', left, downloadStartTime], ['end', right, downloadEndTime]]) {
    const handle = document.getElementById(`timeline-range-${which}-handle`); if (!handle) continue;
    handle.style.left = `${fraction * 100}%`;
    handle.disabled = busy;
    handle.setAttribute('aria-valuenow', String(Math.round(fraction * 100)));
    handle.setAttribute('aria-valuetext', exportBoundaryLabel(date));
  }
}

// ---- Panel ---------------------------------------------------------------------------

function openExportPanel() {
  const panel = document.getElementById('export-panel'); if (!panel) return;
  installExportRangeControls(); installExportRangeOverlay();
  if (!exportPanelOpen && !exportUiBusy()) {
    if (Number.isFinite(currentTimePosition)) {
      exportInitialRange = {start: currentTimePosition - 600000, end: currentTimePosition + 600000};
      downloadStartTime = new Date(exportInitialRange.start); downloadEndTime = new Date(exportInitialRange.end);
    } else if (typeof showError === 'function') showError('Select a time on the timeline first');
  }
  exportPanelOpen = true; panel.hidden = false;
  document.getElementById('download-btn')?.setAttribute('aria-expanded', 'true');
  updateDownloadTimeDisplay();
  void refreshExportQueue();
}
function closeExportPanel() {
  const panel = document.getElementById('export-panel');
  exportPanelOpen = false; if (panel) panel.hidden = true;
  document.getElementById('download-btn')?.setAttribute('aria-expanded', 'false');
  renderExportRangeOverlay();
}
function resetDownloadTimeRange() {
  if (exportUiBusy() || !exportInitialRange) return;
  downloadStartTime = new Date(exportInitialRange.start); downloadEndTime = new Date(exportInitialRange.end);
  updateDownloadTimeDisplay();
}

// ---- Bounded control requests ----------------------------------------------------------

async function exportUiRead(response, signal, limit) {
  if (!response.body?.getReader) throw Error('Streaming JSON responses unavailable');
  const reader = response.body.getReader(), chunks = [];
  let size = 0, done = false;
  const cancel = () => { try { Promise.resolve(reader.cancel()).catch(() => {}); } catch (_) {} };
  signal.addEventListener('abort', cancel, {once: true});
  try {
    while (true) {
      if (signal.aborted) throw Error('Export control request timed out');
      const part = await reader.read();
      if (part.done) { done = true; break; }
      if (part.value.byteLength > limit - size) throw Error('Export control response exceeded its size limit');
      size += part.value.byteLength; chunks.push(part.value);
    }
    if (signal.aborted) throw Error('Export control request timed out');
    const bytes = new Uint8Array(size); let offset = 0;
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
    return new TextDecoder().decode(bytes);
  } finally {
    signal.removeEventListener('abort', cancel);
    if (!done) cancel();
    try { reader.releaseLock(); } catch (_) {}
  }
}
// Fixed, plain-language messages; raw server error bodies are never shown.
function exportUiHTTPMessage(response) {
  if (response.status === 401 || response.status === 403) return 'Authentication expired or access denied; sign in again.';
  if (response.status === 503) return 'Prepared exports are unavailable; check NVR_EXPORT_SCRATCH_DIR, recording metadata, or a server restart.';
  if (response.status === 429) return 'The export queue is full; release or clear prepared exports, or wait for them to expire.';
  if (response.headers.get('X-Export-Plan-Status') === 'gap') return 'Recordings do not cover the whole range; gaps are never silently skipped.';
  if (response.status === 422) return 'This range cannot be copied safely (the nearest safe cut is too far away, or the recording profile is not supported). Choose Accurate.';
  if (response.status === 404) return 'The export no longer exists on the server.';
  return `Export request failed (HTTP ${response.status}).`;
}
async function exportUiRequest(path, body, limit = EXPORT_CONTROL_LIMIT) {
  const controller = new AbortController(), timer = setTimeout(() => controller.abort(), EXPORT_JOB_REQUEST_MS);
  try {
    const options = {credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal: controller.signal};
    if (body !== undefined) { options.method = 'POST'; options.headers = {'Content-Type': 'application/json'}; options.body = JSON.stringify(body); }
    const response = await fetch(path, options);
    if (response.redirected) throw Error('Unexpected redirect; sign in again');
    if (response.status === 204) return {status: 204};
    const text = await exportUiRead(response, controller.signal, limit);
    const isJSON = (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase() === 'application/json';
    if (response.status !== 200 && response.status !== 202) throw Error(exportUiHTTPMessage(response));
    if (!isJSON) throw Error('Expected export JSON, not video or a login page');
    try { return {status: response.status, value: JSON.parse(text)}; } catch (_) { throw Error('Invalid export JSON'); }
  } finally { clearTimeout(timer); }
}

// ---- Job views and reasons -------------------------------------------------------------

function exportUiDownloadURL(id) { return `/api/exports/download?id=${id}`; }
function exportUiValidBase(v) {
  return !!v && typeof v === 'object' && !Array.isArray(v) && /^[0-9a-f]{64}$/.test(v.id || '') && EXPORT_STATES.includes(v.state)
    && typeof v.message === 'string' && Number.isSafeInteger(v.bytes) && v.bytes >= 0 && Number.isFinite(Date.parse(v.expires_at));
}
function exportUiView(v, id) {
  if (!exportUiValidBase(v) || (id && v.id !== id)) throw Error('Invalid export status response');
  if (v.state === 'ready' && (!v.bytes || v.download_url !== exportUiDownloadURL(v.id))) throw Error('Invalid prepared download response');
  return v;
}
const EXPORT_REASON_TEXT = {
  output_limit: 'The export was larger than the server allows for one file. Choose a shorter range.',
  estimate_exceeded: 'The export grew beyond its estimated size. Try a shorter range.',
  deadline_exceeded: 'The export took too long to start or prepare and was stopped. Try a shorter range or fewer cameras.',
  empty_output: 'No video was produced for this range.',
  render_failed: 'The server could not prepare this export. The server log has details.',
  canceled: 'The export was canceled.',
  cleared: 'The export was removed by Clear all.',
  expired: 'The prepared file expired and was deleted. Queue it again if you still need it.',
  service_stopping: 'The server was stopping, so the export was canceled.',
};
function exportUiReason(reason) {
  return Object.prototype.hasOwnProperty.call(EXPORT_REASON_TEXT, reason) ? EXPORT_REASON_TEXT[reason] : 'The export did not complete.';
}
// The server may extend a copy range outward to safe cuts and reports the saved range.
// It must be a valid interval that covers the requested range.
function exportUiSaved(requested, v) {
  if (v.saved_start === undefined && v.saved_end === undefined) return null;
  const a = Date.parse(v.saved_start), b = Date.parse(v.saved_end);
  if (typeof v.saved_start !== 'string' || typeof v.saved_end !== 'string' || !Number.isFinite(a) || !Number.isFinite(b) || b <= a
    || (requested && (a > Date.parse(requested.start) || b < Date.parse(requested.end)))) throw Error('Invalid saved range in export response');
  return {start: v.saved_start, end: v.saved_end};
}
function exportUiSavedText(requested, saved) {
  if (!saved) return '';
  const extended = !!requested && (Date.parse(saved.start) !== Date.parse(requested.start) || Date.parse(saved.end) !== Date.parse(requested.end));
  return `Saved range: ${exportBoundaryLabel(new Date(saved.start))} to ${exportBoundaryLabel(new Date(saved.end))}${extended ? ' (extended to the nearest safe copy cut)' : ''}.`;
}

// ---- Monitored batch (one job per selected camera) ---------------------------------------

function exportUiState(j, state, message) { j.state = state; j.row.dataset.state = state; j.status.textContent = String(message).slice(0, 1500); }
function exportUiRemoveDownload(j) { if (j.save) { j.save.disabled = true; j.save.remove(); j.save = null; } j.url = null; clearTimeout(j.expiryTimer); }
function exportUiFinish(j, state, text) { exportUiRemoveDownload(j); j.finished = true; exportUiState(j, state, text); exportUiScheduleQueueRefresh(); }
function exportUiWait(j) {
  return new Promise(resolve => {
    j.wake = () => { clearTimeout(j.pollTimer); j.wake = null; resolve(); };
    j.pollTimer = setTimeout(j.wake, EXPORT_JOB_POLL_MS);
  });
}
async function exportUiCancel(j) {
  if (j.canceling || ['canceled', 'expired', 'released'].includes(j.state)) return;
  j.reason = 'canceled'; j.wake?.();
  if (!j.serverId) {
    if (j.creating) { exportUiState(j, 'canceling', 'Waiting for the creation response to identify the job to cancel.'); return; }
    exportUiFinish(j, 'canceled', 'Canceled before a job was created.'); j.cancel.disabled = true; return;
  }
  j.canceling = true; j.cancel.disabled = true; exportUiRemoveDownload(j);
  try {
    const result = await exportUiRequest('/api/exports/cancel', {id: j.serverId});
    if (result.status !== 204) throw Error('Unexpected release response');
    exportUiFinish(j, j.handed ? 'released' : 'canceled', j.handed ? 'Released on the server. A transfer already in progress may continue; disk saving is not verified.' : 'Canceled on the server.');
    j.cancel.disabled = true;
  } catch (error) {
    exportUiFinish(j, 'release-failed', `Cancellation could not be confirmed. ${error.message} The job may remain until it expires.`);
    j.cancel.textContent = 'Retry cancellation/release'; j.cancel.disabled = false;
  } finally { j.canceling = false; }
}
function cancelExportDownloads() { return exportUiBatch ? Promise.all(exportUiBatch.jobs.map(exportUiCancel)) : Promise.resolve(); }
function exportUiModeLabel(mode) { return mode === 'copy' ? 'Original quality / copy' : 'Accurate cut / re-encoded'; }
function exportUiOffer(j, v) {
  j.url = exportUiDownloadURL(j.serverId);
  const expires = Date.parse(v.expires_at), remaining = expires - Date.now();
  if (remaining <= 0) { exportUiFinish(j, 'expired', exportUiReason('expired')); return; }
  j.save = exportUiNode('button', 'Download prepared video'); j.save.type = 'button'; j.save.className = 'btn btn-primary';
  j.save.addEventListener('click', () => {
    if (j.state !== 'ready' || j.canceling || j.handed) return;
    if (Date.now() >= expires) { exportUiFinish(j, 'expired', exportUiReason('expired')); return; }
    const link = exportUiNode('a'); link.href = j.url; link.download = `export-${j.serverId}.mp4`;
    try { document.body.appendChild(link); link.click(); }
    catch (_) { exportUiState(j, 'ready', 'The browser download request failed; try again before the file expires.'); return; }
    finally { link.remove(); }
    j.handed = true; j.save.disabled = true;
    exportUiState(j, 'handed', 'Browser download requested; disk saving is not verified. Release the prepared file after the browser has saved it.');
  });
  j.row.appendChild(j.save);
  j.cancel.textContent = 'Release prepared file / continue'; j.cancel.disabled = false;
  const span = exportUiSavedText(j, j.saved);
  exportUiState(j, 'ready', `${exportUiModeLabel(j.mode)}: ready on the server, ${exportUiSize(v.bytes)}. ${span ? span + ' ' : ''}Download before ${new Date(expires).toLocaleString()}. This page saves nothing itself; disk saving is not verified.`);
  j.finished = true;
  j.expiryTimer = setTimeout(() => { exportUiFinish(j, 'expired', `${exportUiReason('expired')} A transfer already in progress may continue.`); j.cancel.disabled = true; }, Math.min(remaining, 2147483647));
  exportUiScheduleQueueRefresh();
}
function exportUiProgressText(v) {
  if (v.state === 'queued') return `Queued on the server${Number.isSafeInteger(v.queue_position) && v.queue_position > 0 ? `, position ${v.queue_position}` : ''}${Number.isSafeInteger(v.estimated_bytes) && v.estimated_bytes > 0 ? `; estimated ${exportUiSize(v.estimated_bytes)}` : ''}.`;
  return `Preparing on the server${Number.isSafeInteger(v.estimated_bytes) && v.estimated_bytes > 0 ? `; reserved ${exportUiSize(v.estimated_bytes)}` : ''}.`;
}
// Creates the job. Returns the first view, or null when the job is already settled.
async function exportUiCreate(j) {
  if (j.reason === 'canceled') return null;
  try {
    let result;
    j.creating = true; exportUiState(j, 'preparing', 'Requesting server preparation');
    try { result = await exportUiRequest('/api/exports/create', {camera_id: j.id, start_time: j.start, end_time: j.end, mode: j.mode}); }
    finally { j.creating = false; }
    if (result.status !== 202) throw Error('Expected job acceptance');
    const v = exportUiView(result.value);
    j.serverId = v.id;
    exportUiKnownJobs.set(v.id, {name: j.name, start: j.start, end: j.end, mode: j.mode});
    if (j.reason === 'canceled') { await exportUiCancel(j); return null; }
    j.saved = exportUiSaved(j, v);
    return v;
  } catch (error) { exportUiFailed(j, error); return null; }
}
function exportUiFailed(j, error) {
  if (j.reason === 'canceled' && j.serverId) { void exportUiCancel(j); return; }
  exportUiFinish(j, 'failed', `${error.message} ${j.serverId ? 'Use the button below to cancel the server job, or wait for it to expire.' : 'Creation may be unknown if the response was lost; there is no automatic retry.'}`);
  j.cancel.disabled = !j.serverId;
  if (j.serverId) j.cancel.textContent = 'Cancel server job';
}
// Polls one job until it is ready or settled. Bounded: one request per poll interval,
// stops on any terminal state, on cancel, or after two hours.
async function exportUiMonitor(j, v) {
  try {
    const deadline = Date.now() + EXPORT_JOB_WAIT_MS;
    while (true) {
      if (j.reason === 'canceled' || j.detached) return;
      if (v.saved_start !== undefined || v.saved_end !== undefined) j.saved = exportUiSaved(j, v);
      if (v.state === 'ready') { exportUiOffer(j, v); return; }
      if (['failed', 'canceled', 'expired'].includes(v.state)) { exportUiFinish(j, v.state, exportUiReason(v.reason || (v.state === 'expired' ? 'expired' : v.state === 'canceled' ? 'canceled' : ''))); j.cancel.disabled = true; return; }
      exportUiState(j, v.state, exportUiProgressText(v));
      if (Date.now() >= deadline) throw Error('Stopped waiting after two hours; the server job may still exist.');
      await exportUiWait(j);
      if (j.reason === 'canceled' || j.detached) return;
      const result = await exportUiRequest(`/api/exports/status?id=${j.serverId}`);
      if (j.reason === 'canceled' || j.detached) return;
      if (result.status !== 200) throw Error('Unexpected status response');
      v = exportUiView(result.value, j.serverId);
    }
  } catch (error) { exportUiFailed(j, error); }
}
function exportUiDetach(batch) {
  for (const j of batch?.jobs || []) { j.detached = true; clearTimeout(j.pollTimer); clearTimeout(j.expiryTimer); j.wake?.(); }
}
function startExportDownloads() {
  if (exportUiBusy()) return exportUiBatch.promise;
  const list = document.getElementById('download-progress-list'); if (!list) return Promise.resolve();
  const a = downloadStartTime?.getTime(), b = downloadEndTime?.getTime();
  const ids = [...new Set(typeof selectedCameras === 'undefined' ? [] : selectedCameras)], mode = exportMode;
  if (!Number.isFinite(a) || !Number.isFinite(b) || b <= a || ids.length < 1 || ids.length > EXPORT_MAX_CAMERAS || !['copy', 'accurate'].includes(mode)) {
    list.replaceChildren(exportUiNode('p', `Choose a valid range and select one to ${EXPORT_MAX_CAMERAS} playback cameras.`));
    return Promise.resolve();
  }
  const cameras = ids.map(id => allCameras.find(c => c.id === id));
  if (cameras.some(c => !c || typeof c.id !== 'string' || c.id.length > 256)) { list.replaceChildren(exportUiNode('p', 'A selected playback camera is unavailable.')); return Promise.resolve(); }
  exportUiDetach(exportUiBatch);
  list.replaceChildren();
  const jobs = cameras.map(c => {
    const row = exportUiNode('div'); row.className = 'export-job'; row.dataset.cameraId = c.id;
    const name = exportUiText(c.name ?? c.id).slice(0, 256);
    const status = exportUiNode('p', 'Waiting to be queued'), cancel = exportUiNode('button', 'Cancel');
    cancel.type = 'button'; cancel.className = 'btn btn-small'; status.setAttribute('aria-live', 'polite');
    row.appendChild(exportUiNode('h4', name)); row.appendChild(status); row.appendChild(cancel); list.appendChild(row);
    const j = {id: c.id, name, start: new Date(a).toISOString(), end: new Date(b).toISOString(), mode, row, status, cancel, state: 'queued', reason: null, serverId: null, creating: false, canceling: false, finished: false, handed: false, detached: false, url: null, save: null, wake: null, saved: null};
    row.dataset.state = 'queued';
    cancel.addEventListener('click', () => { void exportUiCancel(j); });
    return j;
  });
  const batch = {jobs, running: true, promise: null};
  exportUiBatch = batch; updateDownloadTimeDisplay();
  batch.promise = (async () => {
    try {
      // Create in selection order; the server queue runs them as capacity frees up.
      const first = [];
      for (const j of jobs) first.push(await exportUiCreate(j));
      exportUiScheduleQueueRefresh();
      await Promise.all(jobs.map((j, i) => first[i] ? exportUiMonitor(j, first[i]) : null));
    } finally {
      // Each job already refreshed the queue when it settled.
      batch.running = false;
      if (exportUiBatch === batch) updateDownloadTimeDisplay();
    }
  })();
  return batch.promise;
}

// ---- Session queue panel -------------------------------------------------------------------

let exportUiQueueRefreshing = null, exportUiQueuePending = false, exportUiQueueHold = false;
function exportUiScheduleQueueRefresh() {
  if (exportUiQueueHold || !exportPanelOpen || !document.getElementById('export-queue-list')) return;
  void refreshExportQueue();
}
function exportUiBatchJob(serverId) { return exportUiBatch?.jobs.find(j => j.serverId === serverId) || null; }
// What a job was for. The server returns it with every job, so it survives a page reload;
// jobs created by this page fall back to what this page remembers.
function exportUiRequestedRange(v, known) {
  const a = Date.parse(v.requested_start), b = Date.parse(v.requested_end);
  if (typeof v.requested_start === 'string' && typeof v.requested_end === 'string' && Number.isFinite(a) && Number.isFinite(b) && b > a) return {start: v.requested_start, end: v.requested_end};
  return known ? {start: known.start, end: known.end} : null;
}
function exportUiQueueText(v, requested) {
  const parts = [];
  if (v.state === 'queued' || v.state === 'preparing') parts.push(exportUiProgressText(v));
  else if (v.state === 'ready') parts.push(`Ready, ${exportUiSize(v.bytes)}. Download before ${new Date(v.expires_at).toLocaleString()}.`);
  else parts.push(`${v.state === 'failed' ? 'Failed' : v.state === 'canceled' ? 'Canceled' : 'Expired'}: ${exportUiReason(v.reason || (v.state === 'failed' ? '' : v.state))}`);
  if (requested) parts.push(`Requested: ${exportBoundaryLabel(new Date(requested.start))} to ${exportBoundaryLabel(new Date(requested.end))}.`);
  try { const span = exportUiSavedText(requested, exportUiSaved(requested, v)); if (span) parts.push(span); }
  catch (_) { parts.push('The server reported an invalid saved range for this job.'); }
  return parts.join(' ');
}
function exportUiQueueItem(v) {
  const known = exportUiKnownJobs.get(v.id), requested = exportUiRequestedRange(v, known);
  const row = exportUiNode('div'); row.className = 'export-queue-item'; row.dataset.state = v.state; row.dataset.jobId = v.id;
  const name = exportUiText(typeof v.camera_name === 'string' ? v.camera_name : (known?.name ?? '')).slice(0, 80);
  const mode = ['copy', 'accurate'].includes(v.mode) ? v.mode : known?.mode;
  const title = name ? (mode ? `${name} — ${exportUiModeLabel(mode)}` : name) : `Export ${v.id.slice(0, 8)}`;
  row.appendChild(exportUiNode('h4', title));
  const status = exportUiNode('p', exportUiQueueText(v, requested)); row.appendChild(status);
  if (v.state === 'ready') {
    if (v.bytes > 0 && v.download_url === exportUiDownloadURL(v.id)) {
      const link = exportUiNode('a', 'Download'); link.href = exportUiDownloadURL(v.id); link.download = `export-${v.id}.mp4`; link.className = 'btn btn-small btn-primary';
      row.appendChild(link);
    } else row.appendChild(exportUiNode('p', 'Download unavailable: the server returned an unexpected download address.'));
  }
  if (['queued', 'preparing', 'ready'].includes(v.state)) {
    const cancel = exportUiNode('button', v.state === 'ready' ? 'Release' : 'Cancel job'); cancel.type = 'button'; cancel.className = 'btn btn-small';
    cancel.addEventListener('click', async () => {
      cancel.disabled = true;
      const j = exportUiBatchJob(v.id);
      try {
        if (j) await exportUiCancel(j);
        else { const result = await exportUiRequest('/api/exports/cancel', {id: v.id}); if (result.status !== 204) throw Error('Unexpected cancellation response'); }
        await refreshExportQueue();
      } catch (error) { cancel.disabled = false; status.textContent = `Could not cancel: ${error.message}`; }
    });
    row.appendChild(cancel);
  }
  return row;
}
function exportUiRenderQueue(jobs) {
  const list = document.getElementById('export-queue-list'); if (!list) return;
  const rows = [];
  let invalid = 0;
  for (const raw of jobs.slice(0, EXPORT_LIST_MAX_JOBS)) { if (exportUiValidBase(raw)) rows.push(exportUiQueueItem(raw)); else invalid++; }
  if (!rows.length && !invalid) rows.push(exportUiNode('p', 'No exports in this session.'));
  if (invalid) rows.push(exportUiNode('p', `${invalid} invalid queue ${invalid === 1 ? 'entry was' : 'entries were'} hidden.`));
  list.replaceChildren(...rows);
}
// Reads the session queue once. Concurrent calls coalesce into at most one follow-up read.
function refreshExportQueue() {
  if (exportUiQueueRefreshing) { exportUiQueuePending = true; return exportUiQueueRefreshing; }
  exportUiQueueRefreshing = (async () => {
    try {
      do {
        exportUiQueuePending = false;
        try {
          const result = await exportUiRequest('/api/exports/list', undefined, EXPORT_LIST_LIMIT);
          if (result.status !== 200 || !Array.isArray(result.value?.jobs)) throw Error('Invalid export queue response');
          exportUiRenderQueue(result.value.jobs);
        } catch (error) {
          document.getElementById('export-queue-list')?.replaceChildren(exportUiNode('p', `Queue unavailable: ${error.message}`));
        }
      } while (exportUiQueuePending);
    } finally { exportUiQueueRefreshing = null; }
  })();
  return exportUiQueueRefreshing;
}
async function clearExportQueue() {
  const button = document.getElementById('export-clear-btn'); if (button) button.disabled = true;
  try {
    const result = await exportUiRequest('/api/exports/clear', {});
    if (result.status !== 200 || !Number.isSafeInteger(result.value?.cleared) || result.value.cleared < 0) throw Error('Invalid clear response');
    // Every job this session owned is gone; stop monitoring them without further requests.
    // One queue read follows, so per-job refreshes are held while marking them.
    exportUiQueueHold = true;
    try {
      for (const j of exportUiBatch?.jobs || []) {
        if (j.serverId && (!j.finished || ['ready', 'handed', 'failed', 'release-failed'].includes(j.state))) {
          j.reason = 'canceled'; j.wake?.();
          exportUiFinish(j, 'canceled', exportUiReason('cleared')); j.cancel.disabled = true;
        } else if (!j.serverId && !j.finished) { j.reason = 'canceled'; j.wake?.(); }
      }
    } finally { exportUiQueueHold = false; }
    if (typeof showSuccess === 'function') showSuccess(`Cleared ${result.value.cleared} export job(s).`);
    await refreshExportQueue();
  } catch (error) {
    if (typeof showError === 'function') showError(`Could not clear exports: ${error.message}`);
  } finally { if (button) button.disabled = false; }
}

// Handed-off downloads continue across page navigation; the server's expiry cleans jobs.
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', () => { installExportRangeControls(); installExportRangeOverlay(); });
else { installExportRangeControls(); installExportRangeOverlay(); }
