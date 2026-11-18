// Event log state and filtering.
let allEvents = [];
let filteredEvents = [];
let currentPage = 1;
let eventsPerPage = 10;
const COMMON_EVENT_TYPES = ['Motion Detected','Person Detected','Vehicle Detected','Intrusion','Loitering','Object Removed','Object Left Behind','Sound Alert','Line Crossing','Tamper Alert'];

function preloadEventsForTimeline() {
  fetch('/api/events/list').then(response => response.json()).then(events => {
    allEvents = events || []; window.allEvents = allEvents;
    console.log(`Events preloaded: ${allEvents.length} events available`);
  }).catch(error => { console.error('Failed to preload events:', error); allEvents = []; window.allEvents = []; });
}
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', preloadEventsForTimeline);
else preloadEventsForTimeline();

function openEventsModal() {
  let modal = document.getElementById('events-modal');
  if (!modal) { createEventsModal(); modal = document.getElementById('events-modal'); }
  modal.classList.add('active'); modal.style.display = 'block'; loadEventsList();
}
function closeEventsModal() {
  const modal = document.getElementById('events-modal');
  if (!modal) return;
  modal.classList.remove('active'); modal.style.display = 'none';
}
function createEventsModal() {
  const modalHtml = `
    <div id="events-modal" class="modal">
      <div class="modal-content" style="width: 90vw !important; max-width: 1200px !important; height: 85vh !important; max-height: 85vh !important; padding: 20px; display: flex; flex-direction: column;">
        <span class="close" id="close-events-modal">&times;</span>
        <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 15px; padding-bottom: 15px; border-bottom: 1px solid #333;">
          <h2 style="margin: 0; flex-shrink: 0;">📋 Event Log</h2>
          <div style="display: flex; gap: 10px; align-items: center; flex: 1;">
            <select id="events-filter-camera" style="width: 150px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;"><option value="">All Cameras</option></select>
            <select id="events-filter-type" style="width: 150px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;"><option value="">All Types</option></select>
            <input type="date" id="events-filter-date" style="width: 155px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
            <select id="events-per-page" style="width: 110px; height: 29px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
              <option value="10" selected>10 events</option><option value="25">25 events</option><option value="50">50 events</option><option value="100">100 events</option><option value="all">All events</option>
            </select>
            <div style="flex: 1;"></div>
            <button id="clear-filters-btn" class="btn btn-small" style="padding: 6px 12px; font-size: 13px; white-space: nowrap; flex-shrink: 0; box-sizing: border-box;">Clear</button>
            <button id="refresh-events-btn" class="btn btn-primary" style="padding: 6px 12px; font-size: 13px; white-space: nowrap; flex-shrink: 0; box-sizing: border-box;">🔄 Refresh</button>
          </div>
        </div>
        <div id="events-info" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; padding: 10px; background: #1a1a1a; border-radius: 4px;">
          <div id="events-count" style="color: #888; font-size: 14px;"></div><div id="events-pagination-info" style="color: #888; font-size: 14px;"></div>
        </div>
        <div style="flex: 1; overflow-y: auto; margin-bottom: 15px;"><div id="events-list"><p style="text-align: center; color: #888;">Loading events...</p></div></div>
        <div id="events-pagination" style="display: flex; justify-content: center; align-items: center; gap: 10px; padding: 10px 0; border-top: 1px solid #333;">
          <button id="first-page-btn" class="btn btn-small">⏮️ First</button><button id="prev-page-btn" class="btn btn-small">◀️ Previous</button>
          <span id="page-indicator" style="color: #888; min-width: 120px; text-align: center;">Page 1 of 1</span>
          <button id="next-page-btn" class="btn btn-small">Next ▶️</button><button id="last-page-btn" class="btn btn-small">Last ⏭️</button>
        </div>
      </div>
    </div>`;
  document.body.insertAdjacentHTML('beforeend', modalHtml);
  document.getElementById('close-events-modal').onclick = closeEventsModal;
  document.getElementById('events-filter-camera').onchange = applyFilters;
  document.getElementById('events-filter-type').onchange = applyFilters;
  document.getElementById('events-filter-date').onchange = applyFilters;
  document.getElementById('events-per-page').onchange = changeEventsPerPage;
  document.getElementById('clear-filters-btn').onclick = clearFilters;
  document.getElementById('refresh-events-btn').onclick = refreshEventsList;
  document.getElementById('first-page-btn').onclick = goToFirstPage;
  document.getElementById('prev-page-btn').onclick = previousPage;
  document.getElementById('next-page-btn').onclick = nextPage;
  document.getElementById('last-page-btn').onclick = goToLastPage;
  const modal = document.getElementById('events-modal');
  modal.onclick = e => { if (e.target === modal) closeEventsModal(); };
}
function loadEventsList() {
  fetch('/api/events/list').then(response => response.json()).then(events => {
    allEvents = events || []; window.allEvents = allEvents; populateFilterDropdowns(); applyFilters();
  }).catch(error => {
    console.error('Failed to load events:', error);
    document.getElementById('events-list').innerHTML = '<p style="text-align: center; color: #f44336;">Failed to load events</p>';
  });
}
function refreshEventsList() {
  currentPage = 1; loadEventsList();
  if (typeof renderTimeline === 'function') {
    const dateStr = localDateKey(new Date());
    if (typeof loadTimelineEvents === 'function') loadTimelineEvents(dateStr);
    renderTimeline();
  }
}
function populateFilterDropdowns() {
  const cameraSelect = document.getElementById('events-filter-camera');
  if (cameraSelect) {
    const cameraNames = [...new Set(allEvents.map(e => e.camera_name))].sort();
    cameraSelect.innerHTML = '<option value="">All Cameras</option>';
    cameraNames.forEach(name => { const option = document.createElement('option'); option.value = name; option.textContent = name; cameraSelect.appendChild(option); });
  }
  const typeSelect = document.getElementById('events-filter-type');
  if (typeSelect) {
    const eventTypes = [...new Set(allEvents.map(e => e.label))];
    typeSelect.innerHTML = '<option value="">All Types</option>';
    COMMON_EVENT_TYPES.forEach(type => { const option = document.createElement('option'); option.value = type; option.textContent = type; typeSelect.appendChild(option); });
    const customTypes = eventTypes.filter(t => !COMMON_EVENT_TYPES.includes(t));
    if (customTypes.length > 0) {
      const separator = document.createElement('option'); separator.disabled = true; separator.textContent = '──────────'; typeSelect.appendChild(separator);
      customTypes.sort().forEach(type => { const option = document.createElement('option'); option.value = type; option.textContent = type + ' (custom)'; typeSelect.appendChild(option); });
    }
  }
}
function applyFilters() {
  currentPage = 1;
  const cameraFilter = document.getElementById('events-filter-camera')?.value || '';
  const typeFilter = document.getElementById('events-filter-type')?.value || '';
  const dateFilter = document.getElementById('events-filter-date')?.value || '';
  filteredEvents = allEvents.filter(event => {
    if (cameraFilter && event.camera_name !== cameraFilter) return false;
    if (typeFilter && event.label !== typeFilter) return false;
    const date = new Date(event.start_time);
    if (dateFilter && (!Number.isFinite(date.getTime()) || localDateKey(date) !== dateFilter)) return false;
    return true;
  });
  renderPaginatedEvents();
}
function clearFilters() {
  document.getElementById('events-filter-camera').value = ''; document.getElementById('events-filter-type').value = ''; document.getElementById('events-filter-date').value = ''; currentPage = 1; applyFilters();
}
function changeEventsPerPage() {
  const selectValue = document.getElementById('events-per-page').value;
  eventsPerPage = selectValue === 'all' ? filteredEvents.length : parseInt(selectValue); currentPage = 1; renderPaginatedEvents();
}
function renderPaginatedEvents() {
  const totalEvents = filteredEvents.length;
  const totalPages = eventsPerPage > 0 ? Math.ceil(totalEvents / eventsPerPage) : 1;
  if (currentPage > totalPages && totalPages > 0) currentPage = totalPages;
  if (currentPage < 1) currentPage = 1;
  const startIndex = (currentPage - 1) * eventsPerPage;
  const endIndex = Math.min(startIndex + eventsPerPage, totalEvents);
  const countDiv = document.getElementById('events-count');
  if (countDiv) countDiv.textContent = totalEvents === 0 ? 'No events found' : `Total: ${totalEvents} event${totalEvents !== 1 ? 's' : ''}`;
  const paginationInfo = document.getElementById('events-pagination-info');
  if (paginationInfo) paginationInfo.textContent = totalEvents > 0 ? `Showing ${startIndex + 1}-${endIndex} of ${totalEvents}` : '';
  renderEventsList(filteredEvents.slice(startIndex, endIndex)); updatePaginationControls(totalPages);
}
function updatePaginationControls(totalPages) {
  const pageIndicator = document.getElementById('page-indicator');
  if (pageIndicator) pageIndicator.textContent = `Page ${currentPage} of ${totalPages}`;
  const firstBtn = document.getElementById('first-page-btn'); const prevBtn = document.getElementById('prev-page-btn');
  const nextBtn = document.getElementById('next-page-btn'); const lastBtn = document.getElementById('last-page-btn');
  if (firstBtn) firstBtn.disabled = currentPage === 1;
  if (prevBtn) prevBtn.disabled = currentPage === 1;
  if (nextBtn) nextBtn.disabled = currentPage >= totalPages;
  if (lastBtn) lastBtn.disabled = currentPage >= totalPages;
  const paginationDiv = document.getElementById('events-pagination');
  if (paginationDiv) paginationDiv.style.display = totalPages <= 1 ? 'none' : 'flex';
}
function goToFirstPage() { currentPage = 1; renderPaginatedEvents(); }
function previousPage() { if (currentPage > 1) { currentPage--; renderPaginatedEvents(); } }
function nextPage() { const totalPages = Math.ceil(filteredEvents.length / eventsPerPage); if (currentPage < totalPages) { currentPage++; renderPaginatedEvents(); } }
function goToLastPage() { currentPage = Math.ceil(filteredEvents.length / eventsPerPage); renderPaginatedEvents(); }

// All dynamic values enter text nodes, never HTML or JavaScript source.
function eventTextElement(tag, text, css = '') {
  const node = document.createElement(tag);
  node.textContent = String(text ?? ''); node.style.cssText = css;
  return node;
}
function eventIconText(label, fallback = '?') {
  if (typeof EVENT_ICONS !== 'undefined' && Object.prototype.hasOwnProperty.call(EVENT_ICONS, label) && typeof EVENT_ICONS[label] === 'string') return EVENT_ICONS[label];
  return fallback;
}
function renderEventsList(events) {
  const listDiv = document.getElementById('events-list');
  if (!listDiv) return;
  listDiv.replaceChildren();
  if (!events || events.length === 0) {
    listDiv.appendChild(eventTextElement('p', 'No events to display', 'text-align:center;color:#888;padding:40px 0;')); return;
  }
  const list = document.createElement('div'); list.style.cssText = 'display:flex;flex-direction:column;gap:10px;';
  events.forEach(event => {
    const startTime = new Date(event.start_time);
    const cameraName = String(event.camera_name ?? ''); const startTimeStr = String(event.start_time ?? '');
    const item = document.createElement('div'); item.className = 'event-item';
    item.style.cssText = 'background:#2a2a2a;padding:15px;border-radius:6px;border-left:4px solid #4CAF50;transition:transform 0.2s,box-shadow 0.2s;';
    const row = document.createElement('div'); row.style.cssText = 'display:flex;justify-content:space-between;align-items:start;gap:15px;';
    const details = document.createElement('div'); details.style.flex = '1';
    const heading = document.createElement('div'); heading.style.cssText = 'display:flex;align-items:center;gap:10px;margin-bottom:8px;';
    heading.appendChild(eventTextElement('div', eventIconText(event.label, '⚡'), 'font-size:20px;line-height:1;'));
    heading.appendChild(eventTextElement('div', event.label || 'Event', 'font-size:16px;font-weight:bold;color:#4CAF50;'));
    heading.appendChild(eventTextElement('div', event.id, 'font-size:11px;color:#666;background:#1a1a1a;padding:3px 8px;border-radius:3px;'));
    details.appendChild(heading);
    const camera = eventTextElement('div', '📹 Camera: ', 'font-size:14px;color:#ccc;margin-bottom:3px;');
    camera.appendChild(eventTextElement('strong', cameraName)); details.appendChild(camera);
    details.appendChild(eventTextElement('div', `🕐 ${startTime.toLocaleDateString()} at ${startTime.toLocaleTimeString()}`, 'font-size:13px;color:#888;'));
    details.appendChild(eventTextElement('div', `⏱️ Duration: ${event.duration_seconds || event.duration || 0} seconds`, 'font-size:13px;color:#888;margin-top:3px;'));
    const actions = document.createElement('div'); actions.style.flexShrink = '0';
    const button = eventTextElement('button', '📺 View Recording', 'white-space:nowrap;');
    button.type = 'button'; button.className = 'btn btn-small';
    button.addEventListener('click', () => seekToEvent(cameraName, startTimeStr));
    actions.appendChild(button); row.appendChild(details); row.appendChild(actions); item.appendChild(row); list.appendChild(item);
  });
  listDiv.appendChild(list);
}
function seekToEvent(cameraName, startTimeStr) {
  return navigateToEvent(cameraName, startTimeStr);
}
