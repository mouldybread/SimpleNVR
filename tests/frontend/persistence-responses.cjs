const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const sources = ['cameras.js', 'modals.js'].map(name => ({
  name,
  text: fs.readFileSync(path.join(__dirname, '..', '..', 'web', 'static', 'js', name), 'utf8')
}));
const html = '<div class="camera-box" data-camera-id="camera-test">Updated camera list</div>';
const intentMessage = 'Recording enabled; footage capture is not yet verified.';
const actions = [
  {name: 'start', url: '/api/record/start'},
  {name: 'modal-add', url: '/api/cameras/add'},
  {name: 'modal-retention', url: '/api/cameras/update-retention-all'}
];
const outcomes = [200, 500, 503, 507, 'network'];

async function check(action, outcome) {
  const requests = [], successes = [], errors = [], timers = [], refreshes = [];
  let resets = 0, bodyReads = 0, prevented = 0;
  const main = {innerHTML: 'original-main'};
  const modal = {innerHTML: 'original-modal'};
  const form = {reset() { resets++; }};
  const elements = {
    'camera-list': main,
    'modal-camera-list': modal,
    'modal-add-camera-form': form,
    'modal-retention-all': {value: '14'}
  };
  const context = vm.createContext({
    console: {error() {}, warn() {}, log() {}},
    document: {
      getElementById(id) { return elements[id] || null; },
      addEventListener() {},
      querySelectorAll() { return []; }
    },
    window: {},
    FormData: class MockFormData { constructor(value) { this.form = value; } },
    setTimeout(callback, delay) { timers.push({callback, delay}); return timers.length; },
    clearTimeout() {},
    showSuccess(message) { successes.push(message); },
    showError(message) { errors.push(message); },
    showWarning(message) { throw new Error('Unexpected warning: ' + message); },
    showInfo(message) { throw new Error('Unexpected info: ' + message); },
    updateSectionHeight(id) { refreshes.push(id); },
    loadTimelineCameras() { throw new Error('Unexpected timeline reload'); },
    fetch(url, options) {
      requests.push({url, options});
      if (outcome === 'network') return Promise.reject(new Error('network unavailable'));
      return Promise.resolve({
        ok: outcome === 200,
        status: outcome,
        text() {
          bodyReads++;
          return Promise.resolve(outcome === 200 ? html : 'SERVER-ERROR-BODY');
        }
      });
    }
  });
  for (const source of sources) vm.runInContext(source.text, context, {filename: source.name});
  if (action.name === 'start') context.startRecording('camera-test');
  else if (action.name === 'modal-add') {
    context.setupModalCameraForms();
    assert.equal(typeof form.onsubmit, 'function');
    form.onsubmit({preventDefault() { prevented++; }});
  } else context.setAllRetentionFromModal();
  for (let i = 0; i < 8; i++) await new Promise(resolve => setImmediate(resolve));

  assert.equal(requests.length, 1, 'exactly one request; no retries');
  assert.equal(requests[0].url, action.url);
  assert.equal(requests[0].options.method, 'POST');
  if (action.name === 'start') assert.deepEqual(JSON.parse(requests[0].options.body), {id: 'camera-test'});
  if (action.name === 'modal-retention') assert.deepEqual(JSON.parse(requests[0].options.body), {retention_days: 14});
  if (action.name === 'modal-add') {
    assert.equal(requests[0].options.body.form, form);
    assert.equal(prevented, 1);
  }
  if (outcome !== 200) {
    assert.deepEqual(successes, [], 'failure must not announce success');
    assert.equal(errors.length, 1, 'failure must notify once');
    assert.equal(main.innerHTML, 'original-main');
    assert.equal(modal.innerHTML, 'original-modal');
    assert.equal(resets, 0, 'failure must preserve add form');
    assert.equal(timers.length, 0, 'failure must not schedule success initialization');
    assert.equal(refreshes.length, 0);
    assert.equal(bodyReads, 0, 'non-success response must not enter HTML handling');
    assert.doesNotMatch(errors[0], /rolled back|nothing (?:was )?changed|no changes (?:were )?made/i);
  } else {
    assert.deepEqual(errors, []);
    assert.equal(bodyReads, 1);
    assert.equal(modal.innerHTML, html);
    if (action.name === 'start') {
      assert.equal(main.innerHTML, html);
      assert.deepEqual(successes, [intentMessage]);
      assert.equal(resets, 0);
      assert.equal(timers.length, 0);
      assert.deepEqual(refreshes, ['cameras-content']);
    } else if (action.name === 'modal-add') {
      assert.equal(main.innerHTML, html);
      assert.deepEqual(successes, ['Camera added successfully']);
      assert.equal(resets, 1);
      assert.equal(timers.length, 1);
      assert.equal(timers[0].delay, 50);
      assert.equal(refreshes.length, 0);
    } else {
      assert.equal(main.innerHTML, 'original-main', 'retain current modal-only success behavior');
      assert.deepEqual(successes, ['Retention period updated for all cameras']);
      assert.equal(resets, 0);
      assert.equal(timers.length, 0);
      assert.equal(refreshes.length, 0);
    }
  }
}

(async () => {
  let count = 0, failed = 0;
  console.log('TAP version 13');
  for (const action of actions) for (const outcome of outcomes) {
    count++;
    try {
      await check(action, outcome);
      console.log(`ok ${count} - ${action.name} ${outcome}`);
    } catch (error) {
      failed++;
      console.log(`not ok ${count} - ${action.name} ${outcome}`);
      console.error(error.stack || error);
    }
  }
  console.log(`1..${count}`);
  if (failed) process.exitCode = 1;
})().catch(error => { console.error(error); process.exitCode = 1; });
