// Timeline request helpers for the playback page. playback.js cannot be rewritten
// safely through the connector, so both helpers wrap fetch for timeline requests only.
//
// 1. Plan v2 step 8: scan only the cameras selected for playback. playback.js builds the
//    request without camera_id, so one camera_id is added per selected camera.
// 2. The server leaves an unreadable in-window recording out of the timeline and reports
//    how many in X-Timeline-Unreadable-Count. Without a notice that would look like a gap
//    in the footage, so a warning is shown when the count is above zero.
(function () {
  const TIMELINE_PATH = '/api/timeline/recordings';
  const originalFetch = window.fetch.bind(window);
  let lastNotice = '';

  function notifyUnreadable(response, viewKey) {
    try {
      const count = Number(response.headers.get('X-Timeline-Unreadable-Count'));
      if (!(Number.isInteger(count) && count > 0)) { lastNotice = ''; return; }
      // The same view with the same count is reported once; a clean load resets this.
      const notice = viewKey + '|' + count;
      if (notice === lastNotice) return;
      lastNotice = notice;
      const message = count + (count === 1 ? ' recording' : ' recordings') +
        ' in this view could not be read and ' + (count === 1 ? 'is' : 'are') +
        ' missing from the timeline.';
      if (typeof showWarning === 'function') showWarning(message, 8000); else console.warn(message);
    } catch (_) {
      // A notice must never break the timeline.
    }
  }

  window.fetch = function (input, init) {
    let viewKey = null;
    try {
      if (typeof input === 'string') {
        const url = new URL(input, window.location.href);
        if (url.origin === window.location.origin && url.pathname === TIMELINE_PATH) {
          if (!url.searchParams.has('camera_id')) {
            const ids = typeof selectedCameras !== 'undefined' && Array.isArray(selectedCameras) ? selectedCameras : null;
            if (ids && !ids.length) {
              // Nothing selected: the page would discard every segment, so skip the scan.
              return Promise.resolve(new Response('[]', {status: 200, headers: {'Content-Type': 'application/json'}}));
            }
            if (ids) {
              ids.forEach(id => url.searchParams.append('camera_id', id));
              input = url.pathname + url.search;
            }
          }
          viewKey = url.searchParams.get('start_time') + '|' + url.searchParams.getAll('camera_id').join(',');
        }
      }
    } catch (_) {
      // Leave the request unchanged if anything above fails.
    }
    const result = originalFetch(input, init);
    if (viewKey === null) return result;
    return result.then(response => { notifyUnreadable(response, viewKey); return response; });
  };
})();
