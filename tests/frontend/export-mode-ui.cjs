const assert = require('node:assert/strict');
const {environment, letterId, view, response} = require('./export-ui-fixture.cjs');
const extended = {saved_start: '2026-11-20T23:59:59Z', saved_end: '2026-11-21T00:01:01Z'};
const creates = env => env.requests.filter(r => r.url === '/api/exports/create').map(r => JSON.parse(r.options.body));

(async () => {
  // Copy (default): exact body, extended saved range reported, download link handed to the browser, then release.
  {
    const env = environment(url => Promise.resolve(url === '/api/exports/create' ? response(202, view('a', 'ready', extended)) : response(204)));
    assert.equal(env.run('exportMode'), 'copy');
    assert.equal(env.find('export-mode-copy').getAttribute('aria-pressed'), 'true');
    assert.equal(env.find('export-mode-accurate').getAttribute('aria-pressed'), 'false');
    await env.run('startExportDownloads()');
    const [body] = creates(env);
    assert.equal(env.requests.length, 1, 'one direct create; no proposal or approval round trip');
    assert.deepEqual(Object.keys(body).sort(), ['camera_id', 'end_time', 'mode', 'start_time']);
    assert.equal(body.mode, 'copy'); assert.equal(body.start_time, '2026-11-21T00:00:00.000Z'); assert.equal(body.end_time, '2026-11-21T00:01:00.000Z');
    const j = env.jobs()[0];
    assert.equal(j.state, 'ready'); assert.equal(j.row.dataset.state, 'ready');
    assert.equal(env.run('exportUiBatch.jobs[0].saved.start'), extended.saved_start); assert.equal(env.run('exportUiBatch.jobs[0].saved.end'), extended.saved_end);
    const text = env.find('download-progress-list').textContent;
    assert.ok(text.includes('Original quality / copy')); assert.ok(text.includes('1.2 KiB')); assert.ok(text.includes('Saved range'));
    assert.ok(text.includes('extended to the nearest safe copy cut')); assert.ok(text.includes('not verified'));
    const save = env.all(n => n.text === 'Download prepared video')[0];
    save.click();
    assert.deepEqual(env.downloads, ['/api/exports/download?id=' + letterId('a')]);
    assert.equal(j.state, 'handed'); assert.equal(env.requests.length, 1, 'the video is not fetched by the page');
    save.click(); assert.equal(env.downloads.length, 1, 'download is handed off once');
    assert.equal(j.cancel.text, 'Release prepared file / continue');
    j.cancel.click(); await env.flush();
    assert.equal(env.requests[1].url, '/api/exports/cancel'); assert.deepEqual(JSON.parse(env.requests[1].options.body), {id: letterId('a')});
    assert.equal(j.state, 'released'); assert.equal(j.url, null);
  }

  // Exact saved range is not reported as extended.
  {
    const env = environment(() => Promise.resolve(response(202, view('a', 'ready', {saved_start: '2026-11-21T00:00:00Z', saved_end: '2026-11-21T00:01:00Z'}))));
    await env.run('startExportDownloads()');
    const text = env.jobs()[0].status.textContent;
    assert.ok(text.includes('Saved range')); assert.ok(!text.includes('extended'));
  }

  // Accurate via the toggle.
  {
    const env = environment(() => Promise.resolve(response(202, view('b'))));
    env.find('export-mode-accurate').click();
    assert.equal(env.run('exportMode'), 'accurate');
    assert.equal(env.find('export-mode-accurate').getAttribute('aria-pressed'), 'true'); assert.equal(env.find('export-mode-copy').getAttribute('aria-pressed'), 'false');
    await env.run('startExportDownloads()');
    assert.equal(creates(env)[0].mode, 'accurate');
    assert.ok(env.jobs()[0].status.textContent.includes('re-encoded')); assert.equal(env.run('exportUiBatch.jobs[0].saved'), null);
    assert.equal(env.run('exportSetMode("bogus")'), false); assert.equal(env.run('exportMode'), 'accurate');
  }

  // Controls are disabled while submission is running; the mode cannot change mid-batch.
  {
    let release;
    const env = environment(() => new Promise(r => { release = () => r(response(202, view('c', 'ready'))); }));
    env.run('openExportPanel()');
    const batch = env.run('startExportDownloads()'); await env.flush();
    for (const id of ['export-mode-copy', 'export-mode-accurate', 'download-range-step', 'download-range-reset', 'download-start-earlier', 'download-end-later',
      'export-start-btn', 'timeline-range-start-handle', 'timeline-range-end-handle'])
      assert.equal(env.find(id).disabled, true, `${id} disabled while running`);
    env.find('export-mode-accurate').disabled = false; env.find('export-mode-accurate').click();
    assert.equal(env.run('exportSetMode("accurate")'), false); assert.equal(env.run('exportMode'), 'copy');
    const startMs = env.run('downloadStartTime.getTime()');
    env.find('timeline-range-start-handle').dispatch('keydown', {key: 'ArrowRight'});
    env.find('timeline-range-start-handle').dispatch('pointerdown', {pointerId: 1, clientX: 90});
    assert.equal(env.run('downloadStartTime.getTime()'), startMs, 'range cannot change while running');
    assert.equal(env.run('startExportDownloads()'), env.run('exportUiBatch.promise'), 'a second submit joins the running batch');
    assert.equal(creates(env).length, 1);
    release(); await batch;
    for (const id of ['export-mode-copy', 'export-start-btn', 'timeline-range-start-handle']) assert.equal(env.find(id).disabled, false, `${id} re-enabled`);
  }

  // Saved ranges must be valid and cover the requested range.
  for (const bad of [{saved_start: '2026-11-21T00:00:01Z', saved_end: '2026-11-21T00:01:00Z'}, {saved_start: '2026-11-21T00:00:00Z', saved_end: '2026-11-21T00:00:59Z'},
    {saved_start: 'tomorrow', saved_end: '2026-11-21T00:01:00Z'}, {saved_start: '2026-11-21T00:01:00Z', saved_end: '2026-11-21T00:00:00Z'}, {saved_start: '2026-11-21T00:00:00Z'}]) {
    const env = environment(() => Promise.resolve(response(202, view('c', 'ready', bad))));
    await env.run('startExportDownloads()');
    assert.equal(env.jobs()[0].state, 'failed'); assert.ok(env.jobs()[0].status.textContent.includes('Invalid saved range'));
    assert.equal(env.downloads.length, 0); assert.equal(env.requests.length, 1, 'no retry');
    assert.equal(env.jobs()[0].cancel.disabled, false, 'the created server job can still be canceled');
  }

  // Legacy approval proposal is a failure, not a consent prompt; no retry, no accepted_* fields.
  {
    const env = environment(() => Promise.resolve(response(409, {code: 'copy_range_approval_required', requested_start: '2026-11-21T00:00:00.000Z', requested_end: '2026-11-21T00:01:00.000Z',
      export_start: '2026-11-20T23:59:59Z', export_end: '2026-11-21T00:01:01Z', audio_allowance_ms: 64})));
    await env.run('startExportDownloads()');
    const j = env.jobs()[0];
    assert.equal(j.state, 'failed'); assert.equal(env.requests.length, 1); assert.equal(env.downloads.length, 0);
    assert.ok(!/approv|consent|accept/i.test(j.status.textContent)); assert.ok(!env.body.textContent.includes('awaiting-consent'));
    assert.equal(env.all(n => n.text === 'Accept' || n.text === 'Approve').length, 0);
  }

  // Bounded polling: queued -> preparing -> ready, one status request per poll interval, reasons on terminal states.
  {
    const states = [view('d', 'preparing', {estimated_bytes: 4096}), view('d', 'ready', extended)];
    const env = environment(url => Promise.resolve(url === '/api/exports/create' ? response(202, view('d', 'queued', {queue_position: 2})) : response(200, states.shift())));
    const batch = env.run('startExportDownloads()'); await env.flush();
    assert.equal(env.jobs()[0].state, 'queued'); assert.match(env.jobs()[0].status.textContent, /position 2/);
    assert.equal(env.requests.length, 1, 'no status request before the poll interval');
    await env.fire(1500);
    assert.equal(env.requests[1].url, '/api/exports/status?id=' + letterId('d')); assert.equal(env.jobs()[0].state, 'preparing');
    assert.match(env.jobs()[0].status.textContent, /reserved 4.0 KiB/);
    await env.fire(1500); await batch;
    assert.equal(env.requests.length, 3); assert.equal(env.jobs()[0].state, 'ready');
    assert.ok(env.jobs()[0].status.textContent.includes('extended'));
    await env.fire(1500); assert.equal(env.requests.length, 3, 'polling stops once ready');
  }
  for (const [reason, expected] of [['output_limit', /larger than the server allows/], ['deadline_exceeded', /took too long/], ['empty_output', /No video was produced/],
    ['service_stopping', /server was stopping/], ['canceled', /was canceled/], ['mystery: SECRET', /did not complete/]]) {
    const env = environment(url => Promise.resolve(url === '/api/exports/create' ? response(202, view('e', 'queued'))
      : response(200, view('e', reason === 'canceled' ? 'canceled' : 'failed', {reason, message: 'raw SECRET detail'}))));
    const batch = env.run('startExportDownloads()'); await env.flush(); await env.fire(1500); await batch;
    const text = env.jobs()[0].status.textContent;
    assert.match(text, expected); assert.ok(!text.includes('SECRET'));
    await env.fire(1500); assert.equal(env.requests.length, 2, 'polling stops on a terminal state');
  }

  // Cancel while queued stops polling after one cancel request.
  {
    const env = environment(url => Promise.resolve(url === '/api/exports/create' ? response(202, view('f', 'queued')) : response(204)));
    const batch = env.run('startExportDownloads()'); await env.flush();
    env.jobs()[0].cancel.click(); await env.flush(); await batch;
    assert.deepEqual([...env.urls()], ['/api/exports/create', '/api/exports/cancel']); assert.equal(env.jobs()[0].state, 'canceled');
    await env.fire(1500); assert.equal(env.requests.length, 2);
  }
  console.log('PASS copy and accurate modes, controls disabled while running, extended and exact saved ranges, invalid saved range refusal, legacy proposal rejected without consent state, bounded polling, reasons, cancel');
})().catch(e => { console.error(e); process.exitCode = 1; });
