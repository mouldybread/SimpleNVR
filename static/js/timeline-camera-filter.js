// Plan v2 step 8: scan only the cameras selected for playback.
// playback.js builds the timeline request without camera_id. This wrapper adds one
// camera_id per selected camera, so a request scans those cameras only. It does not
// change playback.js, which the connector cannot rewrite safely.
(function () {
  const TIMELINE_PATH = '/api/timeline/recordings';
  const originalFetch = window.fetch.bind(window);
  window.fetch = function (input, init) {
    try {
      if (typeof input === 'string') {
        const url = new URL(input, window.location.href);
        if (url.origin === window.location.origin && url.pathname === TIMELINE_PATH && !url.searchParams.has('camera_id')) {
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
      }
    } catch (_) {
      // Leave the request unchanged if anything above fails.
    }
    return originalFetch(input, init);
  };
})();
