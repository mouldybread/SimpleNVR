'use strict';
// Regression tests for web/static/js/timeline-camera-filter.js: the camera_id filter on timeline
// requests and the warning shown when the server leaves unreadable recordings out.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');

const source = fs.readFileSync('web/static/js/timeline-camera-filter.js', 'utf8');
const ORIGIN = 'https://nvr.test';
const TIMELINE = '/api/timeline/recordings';
const QUERY = 'start_time=2026-11-10T00%3A00%3A00.000Z&end_time=2026-11-11T00%3A00%3A00.000Z';
const OTHER_QUERY = 'start_time=2026-11-12T00%3A00%3A00.000Z&end_time=2026-11-13T00%3A00%3A00.000Z';
const WARNING_MANY = '2 recordings in this view could not be read and are missing from the timeline.';
const WARNING_ONE = '1 recording in this view could not be read and is missing from the timeline.';

function jsonResponse(count) {
    const headers = { 'Content-Type': 'application/json' };
    if (count !== undefined) headers['X-Timeline-Unreadable-Count'] = String(count);
    return () => Promise.resolve(new Response('[]', { status: 200, headers }));
}

// The script runs in its own context, as in the browser. selectedCameras is a global of
// playback.js, so the harness defines it the same way when a test selects cameras.
function harness({ selected, warn = true } = {}) {
    const calls = [], warnings = [], consoleWarnings = [];
    let reply = jsonResponse();
    const sandbox = {
        URL, Response,
        console: { warn: message => consoleWarnings.push(message) },
        window: {
            location: { origin: ORIGIN, href: ORIGIN + '/' },
            fetch: (input, init) => { calls.push({ input, init }); return reply(input, init); }
        }
    };
    if (warn) sandbox.showWarning = (message, duration) => { warnings.push({ message, duration }); };
    const context = vm.createContext(sandbox);
    if (selected) vm.runInContext('var selectedCameras = ' + JSON.stringify(selected) + ';', context);
    vm.runInContext(source, context, { filename: 'web/static/js/timeline-camera-filter.js' });
    return {
        context, calls, warnings, consoleWarnings,
        fetch: (input, init) => context.window.fetch(input, init),
        select: ids => vm.runInContext('selectedCameras = ' + JSON.stringify(ids) + ';', context),
        reply: fn => { reply = fn; }
    };
}

test('adds one camera_id per selected camera and keeps the rest of the request', async () => {
    const h = harness({ selected: ['front', 'side'] });
    const init = { signal: { aborted: false } };
    await h.fetch(TIMELINE + '?' + QUERY, init);
    assert.equal(h.calls.length, 1);
    const url = new URL(h.calls[0].input, ORIGIN);
    assert.equal(url.pathname, TIMELINE);
    assert.deepEqual(url.searchParams.getAll('camera_id'), ['front', 'side']);
    assert.equal(url.searchParams.get('start_time'), '2026-11-10T00:00:00.000Z');
    assert.equal(url.searchParams.get('end_time'), '2026-11-11T00:00:00.000Z');
    assert.equal(h.calls[0].init, init);
});

test('encodes camera ids that contain URL characters', async () => {
    const ids = ['a&b=c', 'x/y z'];
    const h = harness({ selected: ids });
    await h.fetch(TIMELINE + '?' + QUERY);
    assert.deepEqual(new URL(h.calls[0].input, ORIGIN).searchParams.getAll('camera_id'), ids);
});

test('filters an absolute same-origin timeline URL', async () => {
    const h = harness({ selected: ['a'] });
    await h.fetch(ORIGIN + TIMELINE + '?' + QUERY);
    const url = new URL(h.calls[0].input, ORIGIN);
    assert.equal(url.pathname, TIMELINE);
    assert.deepEqual(url.searchParams.getAll('camera_id'), ['a']);
});

test('makes no request and returns an empty list when no camera is selected', async () => {
    const h = harness({ selected: [] });
    const response = await h.fetch(TIMELINE + '?' + QUERY);
    assert.equal(h.calls.length, 0);
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), []);
});

test('leaves other requests, other origins and non-string inputs unchanged', async () => {
    const h = harness({ selected: ['a'] });
    const inputs = [
        '/api/cameras',
        TIMELINE + '/extra?x=1',
        '/api/exports/create',
        ORIGIN + '/api/cameras',
        'https://other.test' + TIMELINE + '?' + QUERY,
        { url: TIMELINE + '?' + QUERY }
    ];
    for (const input of inputs) await h.fetch(input);
    assert.equal(h.calls.length, inputs.length);
    inputs.forEach((input, index) => assert.equal(h.calls[index].input, input));
});

test('does not override a camera_id the request already has', async () => {
    const h = harness({ selected: ['a'] });
    const input = TIMELINE + '?' + QUERY + '&camera_id=zzz';
    await h.fetch(input);
    assert.equal(h.calls[0].input, input);
});

test('leaves the request unfiltered when the page has no selection state', async () => {
    const h = harness();
    const input = TIMELINE + '?' + QUERY;
    await h.fetch(input);
    assert.equal(h.calls[0].input, input);
});

test('warns with the right wording for one and for several recordings', async () => {
    const h = harness({ selected: ['a'] });
    h.reply(jsonResponse(2));
    await h.fetch(TIMELINE + '?' + QUERY);
    assert.deepEqual(h.warnings, [{ message: WARNING_MANY, duration: 8000 }]);
    const single = harness({ selected: ['a'] });
    single.reply(jsonResponse(1));
    await single.fetch(TIMELINE + '?' + QUERY);
    assert.deepEqual(single.warnings, [{ message: WARNING_ONE, duration: 8000 }]);
});

test('warns once per view and count, and again after a clean load', async () => {
    const h = harness({ selected: ['a'] });
    const get = async (count, query = QUERY) => { h.reply(jsonResponse(count)); return h.fetch(TIMELINE + '?' + query); };
    await get(2);
    await get(2);
    assert.equal(h.warnings.length, 1);
    await get(3);
    assert.equal(h.warnings.length, 2);
    await get(0);
    await get(3);
    assert.equal(h.warnings.length, 3);
    await get(3, OTHER_QUERY);
    assert.equal(h.warnings.length, 4);
});

test('warns again when the selected cameras change for the same day', async () => {
    const h = harness({ selected: ['a'] });
    h.reply(jsonResponse(2));
    await h.fetch(TIMELINE + '?' + QUERY);
    h.select(['b']);
    await h.fetch(TIMELINE + '?' + QUERY);
    assert.equal(h.warnings.length, 2);
});

test('ignores a missing or invalid unreadable count', async () => {
    const h = harness({ selected: ['a'] });
    for (const count of [undefined, '', '0', 'abc', '-1', '1.5']) {
        h.reply(jsonResponse(count));
        await h.fetch(TIMELINE + '?' + QUERY);
    }
    assert.equal(h.warnings.length, 0);
});

test('falls back to console.warn when the page has no toast helper', async () => {
    const h = harness({ selected: ['a'], warn: false });
    h.reply(jsonResponse(2));
    await h.fetch(TIMELINE + '?' + QUERY);
    assert.deepEqual(h.consoleWarnings, [WARNING_MANY]);
});

test('a failing toast cannot break the timeline response', async () => {
    const h = harness({ selected: ['a'] });
    h.context.showWarning = () => { throw new Error('toast failed'); };
    h.reply(jsonResponse(2));
    const response = await h.fetch(TIMELINE + '?' + QUERY);
    assert.equal(response.status, 200);
});

test('passes fetch failures through unchanged', async () => {
    const h = harness({ selected: ['a'] });
    const failure = new Error('network down');
    h.reply(() => Promise.reject(failure));
    await assert.rejects(() => h.fetch(TIMELINE + '?' + QUERY), error => error === failure);
    assert.equal(h.warnings.length, 0);
});

test('returns the original response with its body unread', async () => {
    const h = harness({ selected: ['a'] });
    const original = new Response('[{"camera_id":"a"}]', { status: 200, headers: { 'X-Timeline-Unreadable-Count': '1' } });
    h.reply(() => Promise.resolve(original));
    const response = await h.fetch(TIMELINE + '?' + QUERY);
    assert.equal(response, original);
    assert.equal(original.bodyUsed, false);
    assert.deepEqual(await response.json(), [{ camera_id: 'a' }]);
});
