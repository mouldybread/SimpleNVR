// Shared DOM/fetch fixture for the export UI tests. It loads the real timeline geometry
// helper, creates the page hosts the export UI expects, runs the registered
// DOMContentLoaded callbacks, and models the pointer and keyboard APIs used by handles.
// HTML sinks and inline handlers throw so any HTML injection path fails the tests.
const fs = require('node:fs'), vm = require('node:vm'), path = require('node:path');
const root = path.join(__dirname, '..', '..');
const source = fs.readFileSync(path.join(root, 'web/static/js/export-ui.js'), 'utf8');
const geometrySource = fs.readFileSync(path.join(root, 'web/static/js/timeline-geometry.js'), 'utf8');
const VIEW_START = Date.parse('2026-11-21T00:00:00Z'), VIEW_END = Date.parse('2026-11-22T00:00:00Z');

function environment(fetcher, ids = ['front'], options = {}) {
  const timers = new Map(), requests = [], downloads = [], ready = [], notices = [];
  let serial = 0;
  class Element {
    constructor(tag) {
      this.tagName = tag; this.children = []; this.parentElement = null; this.style = {}; this.dataset = {};
      this.listeners = {}; this.attributes = {}; this.disabled = false; this.hidden = false; this.text = '';
      this.value = ''; this.className = ''; this.href = ''; this.download = ''; this.captured = null;
      this.classList = {add() {}, remove() {}, toggle() {}, contains() { return false; }};
    }
    set textContent(v) { this.text = String(v); this.children = []; }
    get textContent() { return this.text + this.children.map(c => c.textContent).join(''); }
    set innerHTML(_) { throw Error('HTML sink'); }
    set outerHTML(_) { throw Error('HTML sink'); }
    insertAdjacentHTML() { throw Error('HTML sink'); }
    appendChild(c) { if (c.parentElement) c.remove(); c.parentElement = this; this.children.push(c); return c; }
    replaceChildren(...cs) { this.children = []; this.text = ''; cs.forEach(c => this.appendChild(c)); }
    setAttribute(k, v) { if (/^on/i.test(k)) throw Error('inline handler'); this.attributes[k] = String(v); }
    getAttribute(k) { return this.attributes[k] ?? null; }
    addEventListener(k, fn) { (this.listeners[k] ??= []).push(fn); }
    dispatch(k, event = {}) {
      const e = {type: k, target: this, defaultPrevented: false, propagationStopped: false,
        preventDefault() { this.defaultPrevented = true; }, stopPropagation() { this.propagationStopped = true; }, ...event};
      for (const fn of this.listeners[k] || []) fn(e);
      return e;
    }
    remove() { if (this.parentElement) this.parentElement.children = this.parentElement.children.filter(c => c !== this); this.parentElement = null; }
    click() { if (this.disabled) return; if (this.tagName === 'a') downloads.push(this.href); this.dispatch('click'); }
    focus() {}
    setPointerCapture(id) { this.captured = id; }
    releasePointerCapture(id) { if (this.captured === id) this.captured = null; }
    hasPointerCapture(id) { return this.captured === id; }
    getBoundingClientRect() { return {left: 0, top: 0, width: 100, height: 20}; }
  }
  const body = new Element('body');
  const make = (id, parent = body, tag = 'div') => { const e = new Element(tag); e.id = id; parent.appendChild(e); return e; };
  const button = make('download-btn', body, 'button'); button.setAttribute('aria-expanded', 'false');
  const timeline = make('timeline-container'); make('timeline-canvas', timeline, 'canvas'); make('timeline-range-overlay', timeline);
  const panel = make('export-panel'); panel.hidden = true;
  make('export-camera-guidance', panel); make('export-range-controls', panel); make('export-start-btn', panel, 'button');
  const status = make('download-status', panel); make('download-progress-list', status);
  make('export-refresh-btn', panel, 'button'); make('export-clear-btn', panel, 'button'); make('export-queue-list', panel);
  function find(id, node = body) { if (node.id === id) return node; for (const c of node.children) { const x = find(id, c); if (x) return x; } return null; }
  function all(predicate, node = body, out = []) { if (predicate(node)) out.push(node); for (const c of node.children) all(predicate, c, out); return out; }
  const context = vm.createContext({
    console, AbortController, TextDecoder, Uint8Array, Date, Map, Set, Promise, Number, Math, JSON, Array, String, Object, Error, RangeError, Response,
    document: {readyState: 'loading', addEventListener(e, fn) { if (e === 'DOMContentLoaded') ready.push(fn); }, body, createElement: t => new Element(t), getElementById: id => find(id)},
    setTimeout(fn, ms) { const id = ++serial; timers.set(id, {fn, ms}); return id; },
    clearTimeout(id) { timers.delete(id); },
    fetch(url, options) { requests.push({url, options}); return fetcher(url, options); },
    showSuccess(m) { notices.push(['success', m]); }, showError(m) { notices.push(['error', m]); },
  });
  vm.runInContext(geometrySource, context, {filename: 'web/static/js/timeline-geometry.js'});
  const cameras = options.cameras || ids.map((id, i) => ({id, name: `Camera ${i + 1}`}));
  vm.runInContext(`let downloadStartTime=new Date('2026-11-21T00:00:00Z'),downloadEndTime=new Date('2026-11-21T00:01:00Z'),currentTimePosition=Date.parse('2026-11-21T12:00:00Z'),selectedCameras=${JSON.stringify(ids)},allCameras=${JSON.stringify(cameras)};
function timelineViewport(){return {start:${VIEW_START},end:${VIEW_END}};}`, context);
  if (vm.runInContext('typeof TimelineGeometry', context) !== 'object') throw Error('timeline-geometry.js did not load');
  vm.runInContext(source, context, {filename: 'web/static/js/export-ui.js'});
  ready.forEach(fn => fn());
  const run = code => vm.runInContext(code, context);
  const flush = async () => { for (let i = 0; i < 12; i++) await new Promise(r => setImmediate(r)); };
  // Fire every pending timer with the given delay once (the fixture never runs timers itself).
  const fire = async ms => { for (const [id, t] of [...timers]) if (t.ms === ms) { timers.delete(id); t.fn(); } await flush(); };
  const urls = () => requests.map(r => r.url);
  return {context, run, find, all, timers, requests, downloads, notices, flush, fire, urls, body, ready, VIEW_START, VIEW_END,
    jobs: () => run('exportUiBatch')?.jobs || []};
}
const letterId = x => x.repeat(64);
const view = (x, state = 'ready', extra = {}) => ({id: letterId(x), state,
  message: state === 'failed' ? 'Export preparation failed; no download is ready' : 'State ' + state,
  bytes: state === 'ready' ? 1234 : 0, expires_at: new Date(Date.now() + 3600000).toISOString(),
  ...(state === 'ready' ? {download_url: '/api/exports/download?id=' + letterId(x)} : {}), ...extra});
function response(status, data, headers = {}) {
  return new Response(status === 204 ? null : (typeof data === 'string' ? data : JSON.stringify(data)), {status, headers: {'Content-Type': 'application/json', ...headers}});
}
module.exports = {environment, letterId, view, response, VIEW_START, VIEW_END};
