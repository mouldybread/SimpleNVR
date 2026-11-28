const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path');
const {environment, letterId, view, response} = require('./export-ui-fixture.cjs');
const source = fs.readFileSync(path.join(__dirname, '../../static/js/export-ui.js'), 'utf8');

// Static security policy: no video buffering, no HTML sinks, no legacy approval flow.
for (const banned of ['new Blob(', 'createObjectURL', '.blob()', 'arrayBuffer(', 'innerHTML', 'outerHTML', 'insertAdjacentHTML', 'document.write',
  'accepted_start', 'accepted_end', 'awaiting-consent', 'copy_range_approval', 'download-modal', 'downloadCameras', 'setInterval'])
  assert.ok(!source.includes(banned), `export-ui.js must not contain ${banned}`);
assert.equal(source.match(/\bfetch\(/g).length, 1, 'all requests go through the bounded control request helper');
for (const option of ["credentials: 'same-origin'", "cache: 'no-store'", "redirect: 'error'", 'signal: controller.signal']) assert.ok(source.includes(option), option);

const BODY_KEYS = ['camera_id', 'end_time', 'mode', 'start_time'];
const creates = env => env.requests.filter(r => r.url === '/api/exports/create').map(r => JSON.parse(r.options.body));
const byText = (env, text, tag) => env.all(n => (!tag || n.tagName === tag) && n.text === text);

(async () => {
  // 1. Overlay, handles, panel opening and queue refresh on open.
  {
    const list = {jobs: []};
    const env = environment(url => { if (url === '/api/exports/list') return Promise.resolve(response(200, list)); throw Error('unexpected ' + url); }, ['front', 'back']);
    const start = env.find('timeline-range-start-handle'), end = env.find('timeline-range-end-handle');
    assert.ok(env.find('timeline-range-band') && start && end, 'range band and both handles exist after DOMContentLoaded');
    for (const h of [start, end]) {
      assert.equal(h.getAttribute('role'), 'slider'); assert.ok(h.getAttribute('aria-label'));
      assert.equal(h.getAttribute('aria-orientation'), 'horizontal'); assert.equal(h.type, 'button');
    }
    assert.ok(env.find('download-range-step') && env.find('export-mode-copy') && env.find('export-mode-accurate'), 'range controls and mode toggle installed');
    assert.equal(env.find('export-panel').hidden, true); assert.equal(env.requests.length, 0, 'nothing is requested before opening');
    env.run('openExportPanel()'); await env.flush();
    assert.equal(env.find('export-panel').hidden, false); assert.equal(env.find('download-btn').getAttribute('aria-expanded'), 'true');
    assert.deepEqual(env.urls(), ['/api/exports/list'], 'opening refreshes the queue once');
    assert.equal(env.find('timeline-range-overlay').hidden, false);
    assert.equal(env.run('downloadEndTime-downloadStartTime'), 1200000, 'opening sets a 20 minute range around the playhead');
    assert.equal(env.find('timeline-range-band').style.left, `${(11 * 60 + 50) / 1440 * 100}%`);
    assert.ok(Math.abs(parseFloat(env.find('timeline-range-band').style.width) - 20 / 1440 * 100) < 1e-9);
    assert.match(start.getAttribute('aria-valuetext'), /2026/); assert.equal(start.getAttribute('aria-valuenow'), '49');
    assert.match(env.find('export-camera-guidance').textContent, /every camera selected for playback \(2\): Camera 1, Camera 2/);
    assert.match(env.find('export-queue-list').textContent, /No exports in this session/);
    env.run('closeExportPanel()');
    assert.equal(env.find('export-panel').hidden, true); assert.equal(env.find('timeline-range-overlay').hidden, true);
    assert.equal(env.find('download-btn').getAttribute('aria-expanded'), 'false');
    assert.equal(env.timers.size, 0, 'no polling timers when idle');
  }

  // 2. Every selected playback camera is queued in order, one direct create each, exact body.
  {
    let n = 0;
    const env = environment(url => {
      if (url === '/api/exports/create') return Promise.resolve(response(202, view(['c', 'd', 'e'][n++], 'queued', {queue_position: n})));
      if (url === '/api/exports/list') return Promise.resolve(response(200, {jobs: []}));
      throw Error('unexpected ' + url);
    }, ['front', 'back', 'side']);
    env.run('selectedCameras=["back","front"]');
    env.run('downloadStartTime=new Date("2026-11-21T00:00:10Z");downloadEndTime=new Date("2026-11-21T00:00:50Z");updateDownloadTimeDisplay()');
    const done = env.run('startExportDownloads()'); await env.flush();
    const bodies = creates(env);
    assert.equal(env.requests[0].url, '/api/exports/create', 'the first request is a direct create, with no proposal step');
    assert.deepEqual(bodies.map(b => b.camera_id), ['back', 'front'], 'only and all selected playback cameras, in selection order');
    for (const b of bodies) {
      assert.deepEqual(Object.keys(b).sort(), BODY_KEYS);
      assert.equal(b.start_time, '2026-11-21T00:00:10.000Z'); assert.equal(b.end_time, '2026-11-21T00:00:50.000Z'); assert.equal(b.mode, 'copy');
    }
    for (const r of env.requests) {
      assert.equal(r.options.credentials, 'same-origin'); assert.equal(r.options.cache, 'no-store'); assert.equal(r.options.redirect, 'error'); assert.ok(r.options.signal);
      if (r.options.body) assert.ok(!/accepted_|consent|approval/.test(r.options.body));
    }
    assert.deepEqual([...env.jobs().map(j => j.state)], ['queued', 'queued']);
    assert.match(env.find('download-progress-list').textContent, /Queued on the server, position 1/);
    assert.ok(env.jobs().every(j => j.state !== 'awaiting-consent'));
    assert.equal(env.run('exportUiBusy()'), true);
    await env.run('cancelExportDownloads()').catch(() => {});
  }

  // 3. Camera count and range validation refuse to send anything.
  for (const [ids, setup] of [[[], ''], [['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'], ''], [['front'], 'downloadEndTime=new Date(downloadStartTime)'], [['front'], 'downloadStartTime=new Date(NaN)']]) {
    const env = environment(() => { throw Error('must not request'); }, ids);
    if (setup) env.run(setup);
    await env.run('startExportDownloads()');
    assert.equal(env.requests.length, 0); assert.match(env.find('download-progress-list').textContent, /select one to 8 playback cameras/);
  }
  {
    const env = environment(() => { throw Error('must not request'); }, ['front', 'front', 'back']);
    env.run('refreshExportCameraGuidance()'); assert.match(env.find('export-camera-guidance').textContent, /\(2\)/, 'duplicates count once');
    const none = environment(() => { throw Error('x'); }, []); none.run('refreshExportCameraGuidance()');
    assert.match(none.find('export-camera-guidance').textContent, /No cameras are selected/);
  }

  // 4. Queue rendering: states, reasons, saved ranges, safe and unsafe downloads, no raw server text, no HTML.
  {
    const secret = 'ffmpeg: rtsp://admin:SECRET@10.0.0.2 /var/lib/nvr <img src=x onerror=alert(1)>';
    const jobs = [
      view('a', 'queued', {estimated_bytes: 2048, queue_position: 1, message: secret}),
      view('b', 'preparing', {estimated_bytes: 3 * 1048576}),
      view('c', 'ready', {bytes: 5000, saved_start: '2026-11-21T00:00:00Z', saved_end: '2026-11-21T00:01:00Z'}),
      view('d', 'ready', {download_url: 'https://evil.example/x'}),
      view('e', 'ready', {download_url: '/api/exports/download?id=' + letterId('a')}),
      view('f', 'failed', {reason: 'estimate_exceeded', message: secret}),
      view('1', 'failed', {reason: 'render_failed', message: secret}),
      view('2', 'canceled', {reason: 'cleared'}),
      view('3', 'expired', {reason: 'expired'}),
      view('4', 'failed', {reason: '<b>internal panic: SECRET</b>'}),
      view('5', 'ready', {saved_start: 'nonsense', saved_end: '2026-11-21T00:01:00Z'}),
      {id: '<script>', state: 'ready', message: secret, bytes: 1, expires_at: new Date().toISOString(), download_url: '/api/exports/download?id=<script>'},
      {id: letterId('9'), state: 'awaiting-consent', message: 'x', bytes: 0, expires_at: new Date().toISOString()},
    ];
    const env = environment(url => { if (url === '/api/exports/list') return Promise.resolve(response(200, {jobs})); return Promise.resolve(response(204)); });
    env.run('openExportPanel()'); await env.flush();
    const list = env.find('export-queue-list'), text = list.textContent;
    assert.equal(env.all(n => n.className === 'export-queue-item').length, 11);
    for (const expected of ['Queued on the server, position 1; estimated 2.0 KiB', 'Preparing on the server; reserved 3.0 MiB', 'Ready, 4.9 KiB',
      'grew beyond its estimated size', 'could not prepare this export', 'removed by Clear all', 'expired and was deleted', 'The export did not complete.',
      'Download unavailable', 'invalid saved range', '2 invalid queue entries were hidden', 'Saved range'])
      assert.ok(text.includes(expected), `queue text includes ${expected}`);
    for (const leaked of ['SECRET', 'rtsp://', '/var/lib', 'ffmpeg', '<img', '<script>', 'panic', 'awaiting-consent']) assert.ok(!text.includes(leaked), `queue hides ${leaked}`);
    const links = env.all(n => n.tagName === 'a');
    assert.deepEqual(links.map(l => l.href), ['/api/exports/download?id=' + letterId('c'), '/api/exports/download?id=' + letterId('5')], 'links only for exact same-job download URLs');
    assert.ok(env.all(n => /^(img|script|b)$/.test(n.tagName)).length === 0);
    links[0].click();
    assert.deepEqual(env.downloads, ['/api/exports/download?id=' + letterId('c')], 'safe click uses a normal same-origin link');
    assert.ok(env.urls().every(u => !u.includes('/api/exports/download')), 'the video is never fetched by the page');
    // Cancel/release from the queue: one cancel POST then a refresh.
    const before = env.requests.length;
    byText(env, 'Cancel job', 'button')[0].click(); await env.flush();
    const after = env.requests.slice(before);
    assert.deepEqual(after.map(r => r.url), ['/api/exports/cancel', '/api/exports/list']);
    assert.deepEqual(JSON.parse(after[0].options.body), {id: letterId('a')});
    assert.equal(byText(env, 'Release', 'button').length, 4, 'ready jobs offer release'); assert.equal(byText(env, 'Cancel job', 'button').length, 2);
  }

  // 5. Clear all posts {} once, stops session batch monitoring, refreshes; failures are shown safely.
  {
    let cleared = false;
    const env = environment(url => {
      if (url === '/api/exports/create') return Promise.resolve(response(202, view('a', 'queued')));
      if (url === '/api/exports/clear') { cleared = true; return Promise.resolve(response(200, {cleared: 1})); }
      if (url === '/api/exports/list') return Promise.resolve(response(200, {jobs: cleared ? [] : [view('a', 'queued')]}));
      throw Error('unexpected ' + url);
    });
    env.run('openExportPanel()'); await env.flush();
    const done = env.run('startExportDownloads()'); await env.flush();
    assert.equal(env.jobs()[0].state, 'queued');
    const before = env.requests.length;
    await env.run('clearExportQueue()'); await env.flush();
    const after = env.requests.slice(before);
    assert.equal(after[0].url, '/api/exports/clear'); assert.equal(after[0].options.method, 'POST'); assert.equal(after[0].options.body, '{}');
    assert.deepEqual(after.slice(1).map(r => r.url), ['/api/exports/list']);
    assert.equal(env.jobs()[0].state, 'canceled'); assert.match(env.jobs()[0].status.textContent, /removed by Clear all/);
    await done;
    const settled = env.requests.length; await env.fire(1500); await env.fire(1500);
    assert.equal(env.requests.length, settled, 'no status polling after Clear all');
    assert.equal(env.run('exportUiBusy()'), false);
    assert.match(env.find('export-queue-list').textContent, /No exports in this session/);
    assert.ok(env.notices.some(([k, m]) => k === 'success' && /Cleared 1/.test(m)));

    const failing = environment(url => Promise.resolve(url === '/api/exports/clear' ? response(500, 'panic: SECRET at /srv/nvr') : response(200, {jobs: []})));
    await failing.run('clearExportQueue()');
    const error = failing.notices.find(([k]) => k === 'error')[1];
    assert.match(error, /HTTP 500/); assert.ok(!error.includes('SECRET')); assert.equal(failing.find('export-clear-btn').disabled, false);
    assert.deepEqual(failing.urls(), ['/api/exports/clear'], 'no retry of a failed clear');
  }

  // 6. Bounded responses, non-JSON, redirects and raw error bodies.
  {
    const huge = {jobs: Array.from({length: 400}, (_, i) => view('a', 'queued', {message: 'x'.repeat(200) + i}))};
    const env = environment(() => Promise.resolve(response(200, huge)));
    await env.run('refreshExportQueue()');
    assert.match(env.find('export-queue-list').textContent, /Queue unavailable: Export control response exceeded its size limit/);
    const html = environment(() => Promise.resolve(response(200, '<html>login</html>', {'Content-Type': 'text/html'})));
    await html.run('refreshExportQueue()'); assert.match(html.find('export-queue-list').textContent, /Expected export JSON/);
    assert.ok(!html.find('export-queue-list').textContent.includes('<html>'));
    let calls = 0;
    const raw = environment(() => { calls++; return Promise.resolve(response(500, '{"error":"open /var/lib/nvr/SECRET.mp4: permission denied"}')); });
    await raw.run('startExportDownloads()');
    const status = raw.jobs()[0].status.textContent;
    assert.equal(raw.jobs()[0].state, 'failed'); assert.match(status, /HTTP 500/); assert.match(status, /no automatic retry/);
    assert.ok(!status.includes('SECRET') && !status.includes('/var/lib')); assert.equal(calls, 1);
    const oversized = environment(() => Promise.resolve(response(202, view('a', 'queued', {message: 'x'.repeat(9000)}))));
    await oversized.run('startExportDownloads()');
    assert.equal(oversized.jobs()[0].state, 'failed'); assert.equal(oversized.requests.length, 1, 'uncertain create is not retried');
    const redirected = environment(() => Promise.reject(new TypeError('Failed to fetch')));
    await redirected.run('startExportDownloads()');
    assert.equal(redirected.jobs()[0].state, 'failed'); assert.equal(redirected.requests.length, 1);
  }

  // 7. HTML in camera names stays text.
  {
    const env = environment(() => Promise.resolve(response(202, view('a', 'queued'))), ['x'], {cameras: [{id: 'x', name: '<img src=x onerror=alert(1)>'}]});
    env.run('refreshExportCameraGuidance()');
    await Promise.race([env.run('startExportDownloads()'), env.flush()]);
    assert.ok(env.find('download-progress-list').textContent.includes('<img src=x onerror=alert(1)>'));
    assert.equal(env.all(n => n.tagName === 'img').length, 0);
    await env.run('cancelExportDownloads()').catch(() => {});
  }
  console.log('PASS export UI: selected playback cameras, direct creates without accepted_*, overlay handles, queue states and reasons, unsafe download_url rejected, safe link download, cancel and session clear, bounded responses, no HTML injection or video buffering');
})().catch(e => { console.error(e); process.exitCode = 1; });
