const assert = require('node:assert/strict');
const {environment, VIEW_START, VIEW_END} = require('./export-ui-fixture.cjs');
const env = environment(url => {
  if (url === '/api/exports/list') return Promise.resolve(new Response('{"jobs":[]}', {status: 200, headers: {'Content-Type': 'application/json'}}));
  throw Error('Range controls must not start export requests');
}), run = env.run;
const S = () => run('downloadStartTime.getTime()'), E = () => run('downloadEndTime.getTime()');

run('openExportPanel()');
assert.equal(env.find('download-range-step').value, '60000'); assert.equal(E() - S(), 1200000);
const start = S(), end = E();
for (const boundary of ['start', 'end']) for (const direction of [-1, 1]) {
  run('resetDownloadTimeRange()'); env.find(`download-${boundary}-${direction < 0 ? 'earlier' : 'later'}`).click();
  assert.equal(S(), boundary === 'start' ? start + direction * 60000 : start);
  assert.equal(E(), boundary === 'end' ? end + direction * 60000 : end);
}
for (const step of [1000, 10000, 60000, 600000]) { run('resetDownloadTimeRange()'); env.find('download-range-step').value = String(step); run('adjustDownloadBoundary("start",1)'); assert.equal(S(), start + step); }
run('downloadStartTime=new Date(1000);downloadEndTime=new Date(2000)'); env.find('download-range-step').value = '1000'; run('updateDownloadTimeDisplay()');
assert.equal(env.find('download-start-later').disabled, true); assert.equal(env.find('download-end-earlier').disabled, true); assert.equal(run('adjustDownloadBoundary("start",1)'), false);
assert.equal(run('exportRangeCandidate(NaN,2000,"start",1000)'), null); assert.equal(run('exportRangeCandidate(0,1000,"end",1e20)'), null);
run('resetDownloadTimeRange()'); assert.equal(S(), start);
assert.match(env.find('download-start-display').textContent, /2026/); assert.match(env.find('download-end-display').textContent, /2026/);
run('downloadStartTime=new Date("2026-11-01T06:59:59Z");downloadEndTime=new Date("2026-11-01T08:00:00Z")');
run('adjustDownloadBoundary("start",1)'); assert.equal(run('downloadStartTime.toISOString()'), '2026-11-01T07:00:00.000Z');

// Reopening an already open panel keeps the user's range; reopening after close resets around the playhead.
run(`downloadStartTime=new Date(${start + 5000})`); run('openExportPanel()'); assert.equal(S(), start + 5000);
run('closeExportPanel();openExportPanel()'); assert.equal(S(), start);

// Keyboard on the floating handles: arrows, page keys, Home/End, always start < end.
const sh = env.find('timeline-range-start-handle'), eh = env.find('timeline-range-end-handle');
const key = (h, k) => h.dispatch('keydown', {key: k});
env.find('download-range-step').value = '60000';
let e = key(sh, 'ArrowLeft'); assert.equal(S(), start - 60000); assert.ok(e.defaultPrevented && e.propagationStopped);
key(sh, 'ArrowRight'); assert.equal(S(), start);
key(eh, 'PageUp'); assert.equal(E(), end + 600000); key(eh, 'PageDown'); assert.equal(E(), end);
key(sh, 'Home'); assert.equal(S(), VIEW_START); key(eh, 'End'); assert.equal(E(), VIEW_END);
run('resetDownloadTimeRange()');
key(sh, 'End'); assert.equal(S(), end - 1000, 'start clamps below end'); key(sh, 'ArrowRight'); assert.equal(S(), end - 1000);
run('resetDownloadTimeRange()');
key(eh, 'Home'); assert.equal(E(), start + 1000, 'end clamps above start');
run('resetDownloadTimeRange()');
e = key(sh, 'a'); assert.equal(e.defaultPrevented, false, 'unrelated keys are left alone'); assert.equal(S(), start);
assert.equal(sh.getAttribute('aria-valuenow'), String(Math.round((start - VIEW_START) / (VIEW_END - VIEW_START) * 100)));

// Pointer dragging maps through the timeline viewport and clamps to the range.
const frac = f => VIEW_START + f * (VIEW_END - VIEW_START);
e = sh.dispatch('pointerdown', {pointerId: 7, clientX: 25});
assert.equal(sh.captured, 7); assert.ok(e.defaultPrevented && e.propagationStopped); assert.equal(S(), frac(0.25));
sh.dispatch('pointermove', {pointerId: 7, clientX: -40}); assert.equal(S(), VIEW_START, 'drag clamps to the viewport start');
sh.dispatch('pointermove', {pointerId: 7, clientX: 99}); assert.equal(S(), E() - 1000, 'start cannot pass end');
sh.dispatch('pointerup', {pointerId: 7}); assert.equal(sh.captured, null);
const settled = S(); sh.dispatch('pointermove', {pointerId: 7, clientX: 10}); assert.equal(S(), settled, 'moves after release are ignored');
run('resetDownloadTimeRange()');
eh.dispatch('pointerdown', {pointerId: 8, clientX: 200}); assert.equal(E(), VIEW_END, 'drag clamps to the viewport end');
eh.dispatch('pointermove', {pointerId: 8, clientX: 0}); assert.equal(E(), S() + 1000, 'end cannot pass start');
eh.dispatch('pointercancel', {pointerId: 8}); assert.equal(eh.captured, null);
assert.ok(S() < E());
assert.equal(sh.dispatch('click').propagationStopped, true, 'handle clicks do not seek the timeline');

// Busy: buttons, handles and keyboard do nothing.
run('resetDownloadTimeRange();exportUiBatch={running:true,jobs:[]};updateDownloadTimeDisplay()');
assert.equal(env.find('download-start-earlier').disabled, true); assert.equal(sh.disabled, true); assert.equal(run('adjustDownloadBoundary("start",-1)'), false);
key(sh, 'ArrowLeft'); sh.dispatch('pointerdown', {pointerId: 9, clientX: 5}); assert.equal(S(), start);
assert.ok(env.urls().every(u => u === '/api/exports/list'));
console.log('PASS range buttons, steps, guards, reset, full dates, DST arithmetic, handle keyboard and pointer clamping, busy controls');
