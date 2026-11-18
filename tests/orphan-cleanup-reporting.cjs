const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/cameras.js', 'utf8');
const valid = {success:true,count:2,size_mb:1.25,skipped_recovery_dirs:0,failures:0,interrupted:false,limit_reached:false};

function element() {
    const classes = new Set();
    return {textContent:'Cleanup Complete',disabled:false,style:{},tagName:'BUTTON',isConnected:true,visible:true,
        classList:{add:n => classes.add(n),remove:n => classes.delete(n),contains:n => classes.has(n)},
        closest() { return this; }, contains(node) { return node === this || node?.ownerButton === this; },
        getClientRects() { return this.visible ? [{}] : []; },
        set innerHTML(value) { throw new Error('Cleanup result used innerHTML'); }};
}
function harness(body, options = {}) {
    const button = element(), other = element(), controls = [button,other];
    const elements = {}, handlers = new Map(), timers = new Map(); let now = 1000, next = 0;
    if (!options.noPanels) for (const prefix of ['', 'modal-']) {
        const heading = element();
        const panel = element(); panel.querySelector = () => heading; panel.heading = heading;
        elements[prefix + 'cleanup-results'] = panel;
        elements[prefix + 'cleanup-results-text'] = element();
    }
    const calls = []; const notices = [];
    const response = (value = body, status = options.status || 200, badJSON = options.badJSON) => ({
        ok:status >= 200 && status < 300,status,
        json:() => badJSON ? Promise.reject(new SyntaxError('invalid JSON')) : Promise.resolve(value)
    });
    class Clock extends Date { static now() { return now; } }
    const document = {
        hidden:false,
        addEventListener(name, fn, capture) {
            if (name === 'click' || name === 'keydown') assert.equal(capture,true);
            if (!handlers.has(name)) handlers.set(name, []); handlers.get(name).push(fn);
        },
        getElementById:id => elements[id] || null,
        querySelectorAll(selector) { assert.equal(selector,'button[onclick="cleanupOrphanedRecordings()"]'); return controls; }
    };
    const context = vm.createContext({
        Date:Clock,document,event:{target:button}, console:{error(){}},
        setTimeout(fn, delay) { const id = ++next; timers.set(id,{fn,delay}); return id; }, clearTimeout:id => timers.delete(id),
        fetch:(url, init) => {
            const call = {url,init}; calls.push(call);
            if (options.syncThrow) throw new Error('synchronous fetch failure');
            if (options.network) return Promise.reject(new Error('<img src=x onerror=bad()>'));
            if (options.deferred) return new Promise((resolve,reject) => { call.resolve=resolve; call.reject=reject; });
            return Promise.resolve(response());
        },
        showSuccess:message => notices.push({level:'success',message}),
        showWarning:message => notices.push({level:'warning',message}),
        showError:message => notices.push({level:'error',message})
    });
    vm.runInContext(source, context);
    const run = code => vm.runInContext(code,context);
    function dispatch(name,event) { for (const fn of handlers.get(name) || []) fn(event); }
    return {button,other,controls,elements,calls,notices,context,state:run,timers,document,dispatch,
        advance(ms) { now += ms; },
        flush(delay) { for (const [id,t] of [...timers]) if (t.delay === delay) { timers.delete(id); t.fn(); } },
        press(control = button, target = control) {
            dispatch('click',{target});
            return context.cleanupOrphanedRecordings({currentTarget:control,target});
        },
        reply(value=body,status=200,index=calls.length-1,badJSON=false) { calls[index].resolve(response(value,status,badJSON)); },
        checkSpent() {
            assert.equal(run('orphanedCleanupConfirmed'),false); assert.equal(run('orphanedCleanupButton'),null);
            assert.equal(run('orphanedCleanupPending'),false); assert.equal(run('orphanedCleanupDeadline'),0);
            for (const control of controls) { assert.equal(control.disabled,false); assert.equal(control.classList.contains('confirm'),false); }
        },
        async run() {
            context.cleanupOrphanedRecordings();
            assert.equal(calls.length, 0, 'first click must not send cleanup');
            await context.cleanupOrphanedRecordings();
            assert.equal(calls.length, 1, 'one request, never an automatic retry');
            assert.equal(calls[0].url, '/api/recordings/cleanup-orphaned');
            assert.equal(calls[0].init.method, 'POST');
            assert.equal(button.disabled, false);
            assert.equal(notices.length, 1);
            for (const prefix of ['', 'modal-']) {
                const panel = elements[prefix + 'cleanup-results'];
                if (!panel) continue;
                assert.equal(panel.style.display, 'block');
                assert.equal(elements[prefix + 'cleanup-results-text'].textContent, notices[0].message);
                assert.notEqual(panel.heading.textContent, 'Cleanup Complete');
                assert.ok(panel.style.borderLeftColor);
                if (notices[0].level !== 'success') {
                    assert.notEqual(panel.style.borderLeftColor, '#4CAF50');
                    assert.notEqual(panel.heading.textContent, 'Cleanup pass complete');
                }
            }
            return notices[0];
        }
    };
}

for (const [name, delta, level, detail] of [
    ['complete', {}, 'success', '2 orphaned file(s) deleted (1.25 MB)'],
    ['zero', {count:0,size_mb:0}, 'success', '0 orphaned file(s) deleted (0.00 MB)'],
    ['preserved', {skipped_recovery_dirs:3}, 'warning', '3 recovery/inspection-sensitive directory(s) preserved'],
    ['failures', {success:false,failures:2}, 'warning', '2 failure(s)'],
    ['interruption', {success:false,failures:1,interrupted:true}, 'warning', 'interrupted'],
    ['limit', {success:false,failures:1,limit_reached:true}, 'warning', 'inspection limit reached'],
    ['combined', {success:false,failures:2,interrupted:true,limit_reached:true,skipped_recovery_dirs:1}, 'warning', 'Remaining files may still exist'],
    ['contradictory-success', {failures:1,interrupted:true}, 'warning', 'Cleanup incomplete'],
    ['unsuccessful-without-detail', {success:false}, 'warning', 'server reported an unsuccessful pass']
]) test(name, async () => {
    const h = harness({...valid,...delta}); const notice = await h.run();
    assert.equal(notice.level, level); assert.ok(notice.message.includes(detail));
    assert.ok(!notice.message.includes('undefined'));
    if (delta.count !== 0) assert.ok(notice.message.includes('2 orphaned file(s) deleted'));
});

const missingCount = {...valid}; delete missingCount.count;
const oldSchema = {...missingCount,files_deleted:2};
const missingFlag = {...valid}; delete missingFlag.limit_reached;
for (const [name, body] of [
    ['null',null], ['array',[]], ['empty',{}], ['missing-count',missingCount],
    ['legacy-field',oldSchema], ['negative-count',{...valid,count:-1}],
    ['fraction-count',{...valid,count:1.5}], ['unsafe-count',{...valid,count:Number.MAX_SAFE_INTEGER+1}],
    ['string-count',{...valid,count:'2'}], ['missing-flag',missingFlag],
    ['negative-size',{...valid,size_mb:-1}], ['nan-size',{...valid,size_mb:NaN}],
    ['infinite-size',{...valid,size_mb:Infinity}], ['string-flag',{...valid,interrupted:'false'}]
]) test('invalid-' + name, async () => {
    const h = harness(body); const notice = await h.run();
    assert.equal(notice.level,'error');
    assert.ok(notice.message.includes('no deletion totals are confirmed'));
    assert.ok(!notice.message.includes('0 orphaned'));
});

for (const [name, options] of [
    ['root-413',{status:413}], ['shutdown-503',{status:503}],
    ['server-500',{status:500}], ['malformed-json',{badJSON:true}], ['network',{network:true}]
]) test(name, async () => {
    const h = harness(valid, options); const notice = await h.run();
    assert.equal(notice.level,'error');
    assert.ok(notice.message.includes('Files may have been removed'));
    assert.ok(notice.message.includes('No automatic retry was made'));
    assert.ok(!notice.message.includes('<img'));
    if (options.status) assert.ok(notice.message.includes('HTTP ' + options.status));
});

test('missing-panels-still-notifies', async () => {
    const h = harness(valid, {noPanels:true});
    assert.equal((await h.run()).level,'success');
});

// Confirmation regressions retain the original reporting assertions above.
test('confirmation-requires-an-identified-control', async () => {
    const h=harness(valid); delete h.context.event;
    await h.context.cleanupOrphanedRecordings(); await h.context.cleanupOrphanedRecordings();
    await h.context.cleanupOrphanedRecordings(element()); assert.equal(h.calls.length,0); h.checkSpent();
});
for (const kind of ['hidden','detached','disabled']) test('confirmation-refuses-' + kind, async () => {
    const h=harness(valid); await h.press();
    if (kind==='hidden') h.button.visible=false;
    if (kind==='detached') h.button.isConnected=false;
    if (kind==='disabled') h.button.disabled=true;
    await h.press(); assert.equal(h.calls.length,0); h.checkSpent();
});
test('confirmation-resolves-nested-target-to-current-button', async () => {
    const h=harness(valid), child={ownerButton:h.button,closest:() => h.button};
    await h.context.cleanupOrphanedRecordings({target:child});
    await h.context.cleanupOrphanedRecordings({target:child}); assert.equal(h.calls.length,1); h.checkSpent();
});
test('confirmation-cannot-transfer-between-controls', async () => {
    const h=harness(valid); await h.press(); await h.press(h.other);
    assert.equal(h.calls.length,0); assert.equal(h.state('orphanedCleanupButton'),h.other);
    await h.press(h.other); assert.equal(h.calls.length,1); h.checkSpent();
});
test('confirmation-refuses-replaced-button-and-requires-fresh-pair', async () => {
    const h=harness(valid); await h.press(); h.button.isConnected=false;
    const replacement=element(); h.controls.splice(0,1,replacement);
    await h.press(); assert.equal(h.calls.length,0);
    await h.press(replacement); assert.equal(h.calls.length,0);
    await h.press(replacement); assert.equal(h.calls.length,1); h.checkSpent();
});
test('confirmation-deadline-is-checked-with-delayed-timer', async () => {
    const h=harness(valid); await h.press(); h.advance(3000);
    await h.press(); assert.equal(h.calls.length,0);
    await h.press(); assert.equal(h.calls.length,1); h.checkSpent();
});
test('confirmation-timer-restores-both-controls', async () => {
    const h=harness(valid); await h.press(); h.flush(3000); h.checkSpent();
    await h.press(); assert.equal(h.calls.length,0);
});
for (const kind of ['same','other']) test('confirmation-stale-timer-cannot-disarm-' + kind, async () => {
    const h=harness(valid); await h.press(); const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    h.context.resetOrphanCleanupConfirmation(); const control=kind==='same'?h.button:h.other;
    await h.press(control); stale(); assert.equal(h.state('orphanedCleanupConfirmed'),true);
    await h.press(control); assert.equal(h.calls.length,1); h.checkSpent();
});
for (const kind of ['outside','escape','page-hidden']) test('confirmation-disarms-on-' + kind, async () => {
    const h=harness(valid); await h.press();
    if (kind==='outside') h.dispatch('click',{target:element()});
    if (kind==='escape') h.dispatch('keydown',{key:'Escape'});
    if (kind==='page-hidden') { h.document.hidden=true; h.dispatch('visibilitychange',{}); h.document.hidden=false; }
    h.checkSpent(); await h.press(); assert.equal(h.calls.length,0);
});
test('confirmation-consumed-before-request-and-duplicates-blocked', async () => {
    const h=harness(valid,{deferred:true}); await h.press(); const pending=h.press();
    assert.equal(h.calls.length,1); assert.equal(h.state('orphanedCleanupConfirmed'),false);
    assert.equal(h.state('orphanedCleanupButton'),null); assert.equal(h.state('orphanedCleanupPending'),true);
    assert.ok(h.controls.every(control => control.disabled));
    await h.press(); await h.press(h.other); assert.equal(h.calls.length,1);
    h.reply(); await pending; h.checkSpent();
});
test('confirmation-new-control-during-pending-cannot-arm-or-post', async () => {
    const h=harness(valid,{deferred:true}); await h.press(); const pending=h.press();
    const added=element(); h.controls.push(added); await h.press(added);
    assert.equal(h.calls.length,1); assert.equal(h.state('orphanedCleanupConfirmed'),false); assert.equal(added.disabled,true);
    h.reply(); await pending; h.checkSpent();
});
test('confirmation-stale-timer-cannot-enable-pending-controls', async () => {
    const h=harness(valid,{deferred:true}); await h.press(); const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    const pending=h.press(); stale(); assert.ok(h.controls.every(control => control.disabled));
    h.reply(); await pending; h.checkSpent();
});
test('confirmation-remains-pending-while-reading-result-body', async () => {
    const h=harness(valid,{deferred:true}); await h.press(); const pending=h.press(); let resolveBody;
    h.calls[0].resolve({ok:true,status:200,json:() => new Promise(resolve => { resolveBody=resolve; })});
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(h.state('orphanedCleanupPending'),true); await h.press(h.other); assert.equal(h.calls.length,1);
    resolveBody(valid); await pending; h.checkSpent();
});
test('confirmation-synchronous-fetch-throw-is-unknown-and-spent', async () => {
    const h=harness(valid,{syncThrow:true}); await h.press(); await h.press(); h.checkSpent();
    assert.equal(h.calls.length,1); assert.equal(h.notices[0].level,'error');
    await h.press(); assert.equal(h.calls.length,1);
});
for (const [name,body,status,badJSON,network] of [
    ['complete',valid,200,false,false],
    ['partial',{...valid,success:false,failures:1},200,false,false],
    ['interrupted',{...valid,success:false,failures:1,interrupted:true},200,false,false],
    ['limited',{...valid,success:false,failures:1,limit_reached:true},200,false,false],
    ['root-413',valid,413,false,false], ['shutdown-503',valid,503,false,false],
    ['invalid-json',valid,200,true,false], ['malformed',{},200,false,false],
    ['transport',valid,200,false,true]
]) test('confirmation-fresh-pair-after-' + name, async () => {
    const h=harness(body,{deferred:true}); await h.press(); const pending=h.press();
    if (network) h.calls[0].reject(new Error('network failed')); else h.reply(body,status,0,badJSON);
    await pending; h.checkSpent(); assert.equal(h.calls.length,1);
    await h.press(); assert.equal(h.calls.length,1);
    const again=h.press(); assert.equal(h.calls.length,2); h.reply(valid); await again; h.checkSpent();
});

test('confirmation-direct-handler-cannot-transfer-controls', async () => {
    const h=harness(valid);
    await h.context.cleanupOrphanedRecordings(h.button);
    await h.context.cleanupOrphanedRecordings(h.other);
    assert.equal(h.calls.length,0); assert.equal(h.state('orphanedCleanupButton'),h.other);
    await h.context.cleanupOrphanedRecordings(h.other); assert.equal(h.calls.length,1); h.checkSpent();
});
