const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('web/static/js/core.js', 'utf8');
const ORIGIN = 'https://nvr.test';
const settle = () => new Promise(resolve => setImmediate(resolve));
const response = (status, path) => ({ status, url: new URL(path, ORIGIN + '/').href });

function harness({ visibility = 'visible', responder = path => response(204, path) } = {}) {
    const calls = [], replaced = [], intervals = [];
    const documentListeners = new Map(), bodyListeners = new Map();
    const state = { responder };
    const add = (map, name, fn) => { if (!map.has(name)) map.set(name, []); map.get(name).push(fn); };
    const window = {
        location: { href: ORIGIN + '/', origin: ORIGIN, replace: target => replaced.push(target) },
        fetch(input, options) {
            calls.push({ input, options });
            return Promise.resolve().then(() => state.responder(input, options));
        },
        addEventListener() {},
        setInterval(fn, delay) { intervals.push({ fn, delay }); return intervals.length; }
    };
    const document = {
        visibilityState: visibility,
        body: { addEventListener: (name, fn) => add(bodyListeners, name, fn) },
        addEventListener: (name, fn) => add(documentListeners, name, fn),
        getElementById: () => null
    };
    const context = vm.createContext({ window, document, URL, AbortSignal, console, setTimeout: () => 0 });
    vm.runInContext(source, context, { filename: 'web/static/js/core.js' });
    return {
        window, document, calls, replaced, intervals, state, bodyListeners,
        interval: () => intervals[0].fn(),
        visible: () => Promise.all((documentListeners.get('visibilitychange') || []).map(fn => fn({})))
    };
}

test('probe-runs-on-load-with-safe-request-options', async () => {
    const h = harness(); await settle();
    assert.equal(h.calls.length, 1);
    assert.equal(h.calls[0].input, '/auth/session');
    const options = h.calls[0].options;
    assert.equal(options.credentials, 'same-origin');
    assert.equal(options.cache, 'no-store');
    assert.equal(options.redirect, 'error');
    assert.ok(options.signal, 'probe must carry a timeout signal');
    assert.deepEqual(h.replaced, []);
});

test('probe-interval-is-thirty-seconds', () => {
    const h = harness();
    assert.deepEqual(h.intervals.map(item => item.delay), [30000]);
});

test('valid-session-never-redirects', async () => {
    const h = harness(); await settle();
    await h.interval(); await h.visible();
    assert.equal(h.calls.length, 3);
    assert.deepEqual(h.replaced, []);
});

test('expired-session-redirects-once-and-stops-probing', async () => {
    const h = harness({ responder: path => response(401, path) }); await settle();
    assert.deepEqual(h.replaced, ['/auth/login']);
    await h.interval(); await h.visible();
    assert.equal(h.calls.length, 1);
    assert.deepEqual(h.replaced, ['/auth/login']);
});

test('server-errors-and-network-failures-do-not-redirect-or-wedge-the-probe', async () => {
    const h = harness({ responder: path => response(503, path) }); await settle();
    h.state.responder = () => { throw new Error('network down'); };
    await h.interval();
    h.state.responder = path => response(500, path);
    await h.interval();
    assert.deepEqual(h.replaced, []);
    h.state.responder = path => response(204, path);
    await h.interval();
    assert.equal(h.calls.length, 4);
    assert.deepEqual(h.replaced, []);
});

test('hidden-tab-skips-probe-until-visible', async () => {
    const h = harness({ visibility: 'hidden' }); await settle();
    assert.equal(h.calls.length, 0);
    await h.interval();
    assert.equal(h.calls.length, 0);
    h.document.visibilityState = 'visible';
    await h.visible();
    assert.equal(h.calls.length, 1);
});

test('in-flight-probe-is-not-duplicated', async () => {
    let release;
    const h = harness({ responder: path => new Promise(resolve => { release = () => resolve(response(204, path)); }) });
    await settle();
    assert.equal(h.calls.length, 1);
    await Promise.all([h.interval(), h.visible()]);
    assert.equal(h.calls.length, 1);
    h.state.responder = path => response(204, path);
    release(); await settle();
    await h.interval();
    assert.equal(h.calls.length, 2);
});

test('dashboard-request-401-redirects-once', async () => {
    const h = harness(); await settle();
    h.state.responder = path => response(401, path);
    await Promise.all([h.window.fetch('/api/cameras'), h.window.fetch('/api/timeline/recordings?x=1')]);
    assert.deepEqual(h.replaced, ['/auth/login']);
});

test('auth-page-cross-origin-and-other-statuses-do-not-redirect', async () => {
    const h = harness(); await settle();
    for (const reply of [
        { status: 401, url: ORIGIN + '/auth/login' },
        { status: 401, url: 'https://other.test/api/cameras' },
        { status: 503, url: ORIGIN + '/api/cameras' },
        { status: 403, url: ORIGIN + '/api/cameras' },
        { status: 200, url: ORIGIN + '/api/cameras' }
    ]) {
        h.state.responder = () => reply;
        await h.window.fetch('/x');
    }
    assert.deepEqual(h.replaced, []);
    h.state.responder = () => ({ status: 401, url: ORIGIN + '/api/cameras' });
    await h.window.fetch('/api/cameras');
    assert.deepEqual(h.replaced, ['/auth/login']);
});

test('htmx-401-redirects-once-and-other-statuses-are-ignored', async () => {
    const h = harness(); await settle();
    const listeners = h.bodyListeners.get('htmx:beforeSwap');
    assert.equal(listeners.length, 1);
    listeners[0]({ detail: { xhr: { status: 503, responseURL: ORIGIN + '/api/cameras' } } });
    listeners[0]({ detail: {} });
    listeners[0]({ detail: { xhr: { status: 401, responseURL: ORIGIN + '/auth/login' } } });
    assert.deepEqual(h.replaced, []);
    for (let i = 0; i < 3; i++) listeners[0]({ detail: { xhr: { status: 401, responseURL: ORIGIN + '/api/cameras' } } });
    assert.deepEqual(h.replaced, ['/auth/login']);
});
