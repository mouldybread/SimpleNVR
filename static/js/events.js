// Events Modal
let allEvents = [];
let filteredEvents = [];
let currentPage = 1;
let eventsPerPage = 10;

// Predefined common event types for consistent filtering
const COMMON_EVENT_TYPES = [
  'Motion Detected',
  'Person Detected',
  'Vehicle Detected',
  'Intrusion',
  'Loitering',
  'Object Removed',
  'Object Left Behind',
  'Sound Alert',
  'Line Crossing',
  'Tamper Alert'
];

// Note: EVENT_ICONS is defined in playback.js and shared globally

// Preload events for timeline on page load
function preloadEventsForTimeline() {
  fetch('/api/events/list')
    .then(response => response.json())
    .then(events => {
      allEvents = events || [];
      window.allEvents = allEvents; // Make globally accessible for playback.js
      console.log(`Events preloaded: ${allEvents.length} events available`);
    })
    .catch(error => {
      console.error('Failed to preload events:', error);
      allEvents = [];
      window.allEvents = [];
    });
}

// Initialize events on page load
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', preloadEventsForTimeline);
} else {
  preloadEventsForTimeline();
}

function openEventsModal() {
  let modal = document.getElementById('events-modal');
  if (!modal) {
    createEventsModal();
    modal = document.getElementById('events-modal');
  }
  
  modal.classList.add('active');
  modal.style.display = 'block';
  
  // Load events and populate filters
  loadEventsList();
}

function closeEventsModal() {
  const modal = document.getElementById('events-modal');
  if (!modal) return;
  
  modal.classList.remove('active');
  modal.style.display = 'none';
}

function createEventsModal() {
  const modalHtml = `
    <div id="events-modal" class="modal">
      <div class="modal-content" style="width: 90vw !important; max-width: 1200px !important; height: 85vh !important; max-height: 85vh !important; padding: 20px; display: flex; flex-direction: column;">
        <span class="close" id="close-events-modal">&times;</span>
        
        <!-- Header with inline filters -->
        <div style="display: flex; align-items: center; gap: 12px; margin-bottom: 15px; padding-bottom: 15px; border-bottom: 1px solid #333;">
          <h2 style="margin: 0; flex-shrink: 0;">📋 Event Log</h2>
          
          <!-- Filters -->
          <div style="display: flex; gap: 10px; align-items: center; flex: 1;">
            
            <!-- Camera Filter -->
            <select id="events-filter-camera" style="width: 150px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
              <option value="">All Cameras</option>
            </select>
            
            <!-- Type Filter -->
            <select id="events-filter-type" style="width: 150px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
              <option value="">All Types</option>
            </select>
            
            <!-- Date Filter -->
            <input type="date" id="events-filter-date" style="width: 155px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
            
            <!-- Items Per Page -->
            <select id="events-per-page" style="width: 110px; height: 29px; background: #333; border: 1px solid #555; color: #fff; padding: 6px 10px; border-radius: 4px; font-size: 13px; flex-shrink: 0; box-sizing: border-box;">
              <option value="10" selected>10 events</option>
              <option value="25">25 events</option>
              <option value="50">50 events</option>
              <option value="100">100 events</option>
              <option value="all">All events</option>
            </select>
            
            <!-- Spacer to push buttons to the right -->
            <div style="flex: 1;"></div>
            
            <!-- Clear Button -->
            <button id="clear-filters-btn" class="btn btn-small" style="padding: 6px 12px; font-size: 13px; white-space: nowrap; flex-shrink: 0; box-sizing: border-box;">Clear</button>
            
            <!-- Refresh Button -->
            <button id="refresh-events-btn" class="btn btn-primary" style="padding: 6px 12px; font-size: 13px; white-space: nowrap; flex-shrink: 0; box-sizing: border-box;">🔄 Refresh</button>
          </div>
        </div>


        
        <!-- Event Count and Info -->
        <div id="events-info" style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; padding: 10px; background: #1a1a1a; border-radius: 4px;">
          <div id="events-count" style="color: #888; font-size: 14px;"></div>
          <div id="events-pagination-info" style="color: #888; font-size: 14px;"></div>
        </div>
        
        <!-- Events List (scrollable) -->
        <div style="flex: 1; overflow-y: auto; margin-bottom: 15px;">
          <div id="events-list">
            <p style="text-align: center; color: #888;">Loading events...</p>
          </div>
        </div>
        
        <!-- Pagination Controls -->
        <div id="events-pagination" style="display: flex; justify-content: center; align-items: center; gap: 10px; padding: 10px 0; border-top: 1px solid #333;">
          <button id="first-page-btn" class="btn btn-small">⏮️ First</button>
          <button id="prev-page-btn" class="btn btn-small">◀️ Previous</button>
          <span id="page-indicator" style="color: #888; min-width: 120px; text-align: center;">Page 1 of 1</span>
          <button id="next-page-btn" class="btn btn-small">Next ▶️</button>
          <button id="last-page-btn" class="btn btn-small">Last ⏭️</button>
        </div>
      </div>
    </div>
  `;
  
  document.body.insertAdjacentHTML('beforeend', modalHtml);
  
  // Attach event listeners after modal is created
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
  
  // Close modal when clicking outside
  const modal = document.getElementById('events-modal');
  modal.onclick = (e) => {
    if (e.target === modal) {
      closeEventsModal();
    }
  };
}



function loadEventsList() {
  fetch('/api/events/list')
    .then(response => response.json())
    .then(events => {
      allEvents = events || [];
      window.allEvents = allEvents; // Update global reference
      populateFilterDropdowns();
      applyFilters();
    })
    .catch(error => {
      console.error('Failed to load events:', error);
      document.getElementById('events-list').innerHTML = 
        '<p style="text-align: center; color: #f44336;">Failed to load events</p>';
    });
}

function refreshEventsList() {
  currentPage = 1;
  loadEventsList();
  
  // Trigger timeline refresh if it exists
  if (typeof renderTimeline === 'function') {
    const dateStr = new Date().toISOString().split('T')[0];
    if (typeof loadTimelineEvents === 'function') {
      loadTimelineEvents(dateStr);
    }
    renderTimeline();
  }
}

function populateFilterDropdowns() {
  // Populate camera filter
  const cameraSelect = document.getElementById('events-filter-camera');
  if (cameraSelect) {
    const cameraNames = [...new Set(allEvents.map(e => e.camera_name))].sort();
    cameraSelect.innerHTML = '<option value="">All Cameras</option>';
    cameraNames.forEach(name => {
      const option = document.createElement('option');
      option.value = name;
      option.textContent = name;
      cameraSelect.appendChild(option);
    });
  }
  
  // Populate type filter with common types + any custom types from events
  const typeSelect = document.getElementById('events-filter-type');
  if (typeSelect) {
    // Get unique event types from actual events
    const eventTypes = [...new Set(allEvents.map(e => e.label))];
    
    // Start with common types
    const allTypes = [...COMMON_EVENT_TYPES];
    
    // Add any custom types that aren't in the common list
    eventTypes.forEach(type => {
      if (!allTypes.includes(type)) {
        allTypes.push(type);
      }
    });
    
    typeSelect.innerHTML = '<option value="">All Types</option>';
    
    // Add common types first
    COMMON_EVENT_TYPES.forEach(type => {
      const option = document.createElement('option');
      option.value = type;
      option.textContent = type;
      typeSelect.appendChild(option);
    });
    
    // Add separator if there are custom types
    const customTypes = eventTypes.filter(t => !COMMON_EVENT_TYPES.includes(t));
    if (customTypes.length > 0) {
      const separator = document.createElement('option');
      separator.disabled = true;
      separator.textContent = '──────────';
      typeSelect.appendChild(separator);
      
      // Add custom types
      customTypes.sort().forEach(type => {
        const option = document.createElement('option');
        option.value = type;
        option.textContent = type + ' (custom)';
        typeSelect.appendChild(option);
      });
    }
  }
}

function applyFilters() {
  currentPage = 1;
  
  const cameraFilter = document.getElementById('events-filter-camera')?.value || '';
  const typeFilter = document.getElementById('events-filter-type')?.value || '';
  const dateFilter = document.getElementById('events-filter-date')?.value || '';
  
  filteredEvents = allEvents.filter(event => {
    // Camera filter
    if (cameraFilter && event.camera_name !== cameraFilter) {
      return false;
    }
    
    // Type filter
    if (typeFilter && event.label !== typeFilter) {
      return false;
    }
    
    // Date filter
    if (dateFilter) {
      const eventDate = new Date(event.start_time).toISOString().split('T')[0];
      if (eventDate !== dateFilter) {
        return false;
      }
    }
    
    return true;
  });
  
  renderPaginatedEvents();
}

function clearFilters() {
  document.getElementById('events-filter-camera').value = '';
  document.getElementById('events-filter-type').value = '';
  document.getElementById('events-filter-date').value = '';
  currentPage = 1;
  applyFilters();
}

function changeEventsPerPage() {
  const selectValue = document.getElementById('events-per-page').value;
  eventsPerPage = selectValue === 'all' ? filteredEvents.length : parseInt(selectValue);
  currentPage = 1;
  renderPaginatedEvents();
}

function renderPaginatedEvents() {
  const totalEvents = filteredEvents.length;
  const totalPages = eventsPerPage > 0 ? Math.ceil(totalEvents / eventsPerPage) : 1;
  
  // Ensure current page is valid
  if (currentPage > totalPages && totalPages > 0) {
    currentPage = totalPages;
  }
  if (currentPage < 1) {
    currentPage = 1;
  }
  
  // Calculate slice indices
  const startIndex = (currentPage - 1) * eventsPerPage;
  const endIndex = Math.min(startIndex + eventsPerPage, totalEvents);
  const pageEvents = filteredEvents.slice(startIndex, endIndex);
  
  // Update count display
  const countDiv = document.getElementById('events-count');
  if (countDiv) {
    if (totalEvents === 0) {
      countDiv.textContent = 'No events found';
    } else {
      countDiv.textContent = `Total: ${totalEvents} event${totalEvents !== 1 ? 's' : ''}`;
    }
  }
  
  // Update pagination info
  const paginationInfo = document.getElementById('events-pagination-info');
  if (paginationInfo) {
    if (totalEvents > 0) {
      paginationInfo.textContent = `Showing ${startIndex + 1}-${endIndex} of ${totalEvents}`;
    } else {
      paginationInfo.textContent = '';
    }
  }
  
  // Render events
  renderEventsList(pageEvents);
  
  // Update pagination controls
  updatePaginationControls(totalPages);
}

function updatePaginationControls(totalPages) {
  const pageIndicator = document.getElementById('page-indicator');
  const firstBtn = document.getElementById('first-page-btn');
  const prevBtn = document.getElementById('prev-page-btn');
  const nextBtn = document.getElementById('next-page-btn');
  const lastBtn = document.getElementById('last-page-btn');
  
  if (pageIndicator) {
    pageIndicator.textContent = `Page ${currentPage} of ${totalPages}`;
  }
  
  // Disable buttons appropriately
  if (firstBtn) firstBtn.disabled = currentPage === 1;
  if (prevBtn) prevBtn.disabled = currentPage === 1;
  if (nextBtn) nextBtn.disabled = currentPage >= totalPages;
  if (lastBtn) lastBtn.disabled = currentPage >= totalPages;
  
  // Show/hide pagination if needed
  const paginationDiv = document.getElementById('events-pagination');
  if (paginationDiv) {
    paginationDiv.style.display = totalPages <= 1 ? 'none' : 'flex';
  }
}

function goToFirstPage() {
  currentPage = 1;
  renderPaginatedEvents();
}

function previousPage() {
  if (currentPage > 1) {
    currentPage--;
    renderPaginatedEvents();
  }
}

function nextPage() {
  const totalPages = Math.ceil(filteredEvents.length / eventsPerPage);
  if (currentPage < totalPages) {
    currentPage++;
    renderPaginatedEvents();
  }
}

function goToLastPage() {
  const totalPages = Math.ceil(filteredEvents.length / eventsPerPage);
  currentPage = totalPages;
  renderPaginatedEvents();
}

function renderEventsList(events) {
  const listDiv = document.getElementById('events-list');
  
  if (!events || events.length === 0) {
    listDiv.innerHTML = '<p style="text-align: center; color: #888; padding: 40px 0;">No events to display</p>';
    return;
  }
  
  let html = '<div style="display: flex; flex-direction: column; gap: 10px;">';
  
  events.forEach(event => {
    const startTime = new Date(event.start_time);
    const formattedDate = startTime.toLocaleDateString();
    const formattedTime = startTime.toLocaleTimeString();
    const duration = event.duration_seconds || event.duration || 0;
    
    // Get event icon from EVENT_ICONS (defined in playback.js)
    const eventIcon = (typeof EVENT_ICONS !== 'undefined' && EVENT_ICONS[event.label]) 
      ? EVENT_ICONS[event.label] 
      : '⚡';
    
    html += `
      <div class="event-item" style="background: #2a2a2a; padding: 15px; border-radius: 6px; border-left: 4px solid #4CAF50; transition: transform 0.2s, box-shadow 0.2s;">
        <div style="display: flex; justify-content: space-between; align-items: start; gap: 15px;">
          <div style="flex: 1;">
            <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 8px;">
              <div style="font-size: 20px; line-height: 1;">
                ${eventIcon}
              </div>
              <div style="font-size: 16px; font-weight: bold; color: #4CAF50;">
                ${event.label || 'Event'}
              </div>
              <div style="font-size: 11px; color: #666; background: #1a1a1a; padding: 3px 8px; border-radius: 3px;">
                ${event.id}
              </div>
            </div>
            <div style="font-size: 14px; color: #ccc; margin-bottom: 3px;">
              📹 Camera: <strong>${event.camera_name}</strong>
            </div>
            <div style="font-size: 13px; color: #888;">
              🕐 ${formattedDate} at ${formattedTime}
            </div>
            <div style="font-size: 13px; color: #888; margin-top: 3px;">
              ⏱️ Duration: ${duration} seconds
            </div>
          </div>
          <div style="flex-shrink: 0;">
            <button onclick="seekToEvent('${event.camera_name}', '${event.start_time}')" 
                    class="btn btn-small" 
                    style="white-space: nowrap;">
              📺 View Recording
            </button>
          </div>
        </div>
      </div>
    `;
  });
  
  html += '</div>';
  listDiv.innerHTML = html;
}

function seekToEvent(cameraName, startTimeStr) {
  // Close events modal
  closeEventsModal();
  
  // Parse the start time
  const startTime = new Date(startTimeStr);
  const date = startTime.toISOString().split('T')[0];
  
  // Set the date picker
  const datePicker = document.getElementById('timeline-date-picker');
  if (datePicker) {
    datePicker.value = date;
  }
  
  // Calculate time of day in hours
  const hours = startTime.getHours();
  const minutes = startTime.getMinutes();
  const seconds = startTime.getSeconds();
  const timeInHours = hours + (minutes / 60) + (seconds / 3600);
  
  // If playback.js functions are available, seek to this time
  if (typeof seekToTime === 'function') {
    seekToTime(timeInHours);
  }
  
  // Load timeline for the selected date if function exists
  if (typeof loadTimelineForDate === 'function') {
    loadTimelineForDate(date);
  }
}
