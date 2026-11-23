const assert = require('node:assert/strict');
const {environment, letterId, view, response} = require('./export-ui-fixture.cjs');

(async () => {
  // A reloaded page has no memory of its jobs: camera, mode and requested range must come
  // from the server's own list, and expired jobs stay visible as expired.
  const requested = {requested_start: '2026-11-21T00:00:00Z', requested_end: '2026-11-21T00:01:00Z'};
  const jobs = [
    view('a', 'ready', {...requested, camera_id: 'front', camera_name: 'Front door', mode: 'accurate', saved_start: '2026-11-20T23:59:58Z', saved_end: '2026-11-21T00:01:02Z'}),
    view('b', 'expired', {...requested, reason: 'expired', camera_name: 'Back <b>yard</b>', mode: 'copy'}),
    view('c', 'queued', {mode: 'bogus', camera_name: 42, requested_start: 'tomorrow', requested_end: '2026-11-21T00:01:00Z'}),
  ];
  const env = environment(url => Promise.resolve(url === '/api/exports/list' ? response(200, {jobs}) : response(204)));
  env.run('openExportPanel()'); await env.flush();
  assert.equal(env.run('exportUiKnownJobs.size'), 0, 'this page created none of these jobs');
  const items = env.all(n => n.className === 'export-queue-item');
  assert.equal(items.length, 3);
  const [ready, expired, queued] = items.map(item => item.textContent);

  assert.match(ready, /Front door — Accurate cut/);
  assert.match(ready, /Requested: .*2026.* to .*2026/);
  assert.match(ready, /Saved range: .*\(extended to the nearest safe copy cut\)/);
  assert.equal(items[0].children.filter(c => c.tagName === 'a' && c.href === '/api/exports/download?id=' + letterId('a')).length, 1);

  assert.match(expired, /Back <b>yard<\/b> — Original quality/);
  assert.match(expired, /Expired: The prepared file expired and was deleted/);
  assert.match(expired, /Requested: /);
  assert.equal(items[1].children.filter(c => c.tagName === 'button' || c.tagName === 'a').length, 0, 'an expired job offers no actions');
  assert.equal(env.all(n => n.tagName === 'b').length, 0, 'camera names are text, never markup');

  assert.match(queued, /^Export cccccccc/, 'invalid server fields fall back to the id');
  assert.ok(!queued.includes('Requested:'));
  assert.ok(!queued.includes('bogus'));
  console.log('PASS queue items show camera, mode, requested and saved range from the server after a reload; expired items stay visible without actions; invalid fields are ignored');
})().catch(error => { console.error(error); process.exitCode = 1; });
