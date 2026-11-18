const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/recordings.js', 'utf8');
const complete = {success:true,deleted:2,size_mb:1.25,failures:0};
function element() {
    const classes = new Set();
    return {value:'',textContent:'',disabled:false,style:{},focuses:0,
        focus() { this.focuses++; },
        classList:{add:n => classes.add(n),remove:n => classes.delete(n),contains:n => classes.has(n)},
        set innerHTML(_) { throw new Error('Unexpected HTML insertion'); }};
}
function harness(options = {}) {
    const nodes = Object.fromEntries(['delete-code','delete-code-input','delete-recordings-btn','delete-recordings-modal'].map(id => [id,element()]));
    if (options.missing) delete nodes[options.missing];
    const timers = new Map(), ready = [], calls = [], notices = []; let id = 0, now = 1000, refreshes = 0;
    class Clock extends Date { static now() { return now; } }
    const c = vm.createContext({
        Date:Clock, document:{getElementById:name => nodes[name] || null,addEventListener:(name,fn) => { assert.equal(name,'DOMContentLoaded'); ready.push(fn); }},
        setTimeout(fn,delay) { const key = ++id; timers.set(key,{fn,delay}); return key; },
        clearTimeout:key => timers.delete(key), console:{error(){}},
        showSuccess:message => notices.push({level:'success',message}),
        showWarning:message => notices.push({level:'warning',message}),
        showError:message => notices.push({level:'error',message}),
        loadTimelineData() { refreshes++; if (options.refreshThrow) throw new Error('refresh failed'); },
        fetch(url,init) {
            let resolve,reject; const promise = new Promise((a,b) => { resolve=a; reject=b; });
            calls.push({url,init,resolve,reject});
            if (options.syncThrow) throw new Error('<img src=x onerror=bad()>');
            return promise;
        }
    });
    vm.runInContext(source,c,{filename:'static/js/recordings.js'}); ready.forEach(fn => fn());
    const run = code => vm.runInContext(code,c);
    return {c,nodes,timers,calls,notices,run,get refreshes() { return refreshes; },
        advance(ms) { now += ms; },
        flush(delay) { for (const [key,timer] of [...timers]) if (timer.delay === delay) { timers.delete(key); timer.fn(); } },
        type(value) { nodes['delete-code-input'].value=value; nodes['delete-code-input'].oninput(); },
        async arm() { c.openDeleteAllRecordingsModal(); this.type(nodes['delete-code'].textContent); await c.deleteAllRecordings(); },
        reply(body=complete,status=200,index=calls.length-1,badJSON=false) {
            calls[index].resolve({status,ok:status>=200 && status<300,
                json:() => badJSON ? Promise.reject(new SyntaxError('bad JSON')) : Promise.resolve(body)});
        },
        checkSpent() {
            assert.equal(run('deleteCode'),''); assert.equal(run('deleteConfirmCount'),0);
            assert.equal(run('deleteConfirmDeadline'),0); assert.equal(run('deleteRecordingsPending'),false);
            assert.equal(nodes['delete-recordings-btn'].disabled,true);
            assert.equal(nodes['delete-code-input'].value,'');
            assert.equal(nodes['delete-recordings-btn'].classList.contains('confirm'),false);
        }
    };
}

test('closed-and-empty-state-cannot-submit', async () => {
    const h=harness(); await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings();
    h.c.openDeleteAllRecordingsModal(); h.type(''); await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings();
    assert.equal(h.calls.length,0); assert.equal(h.nodes['delete-recordings-btn'].disabled,true);
});
for (const missing of ['delete-code','delete-code-input','delete-recordings-btn','delete-recordings-modal'])
    test('missing-' + missing, async () => {
        const h=harness({missing}); h.c.openDeleteAllRecordingsModal();
        await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0);
    });
test('wrong-code-cannot-submit', async () => {
    const h=harness(); h.c.openDeleteAllRecordingsModal(); h.type('wrong');
    await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0);
});
test('one-confirmation-click-is-not-a-request', async () => {
    const h=harness(); await h.arm(); assert.equal(h.calls.length,0); assert.equal(h.run('deleteConfirmCount'),1);
});
test('successful-request-consumes-confirmation-before-await', async () => {
    const h=harness(); await h.arm(); const request=h.c.deleteAllRecordings();
    assert.equal(h.calls.length,1); assert.equal(h.run('deleteCode'),''); assert.equal(h.run('deleteConfirmCount'),0);
    assert.equal(h.nodes['delete-recordings-btn'].disabled,true); assert.equal(h.nodes['delete-code-input'].disabled,true);
    assert.equal(h.calls[0].url,'/api/recordings/delete-all'); assert.equal(h.calls[0].init.method,'POST');
    h.reply(); await request; h.checkSpent(); assert.equal(h.notices[0].level,'success'); assert.equal(h.refreshes,1);
    assert.equal(h.nodes['delete-recordings-modal'].classList.contains('active'),false);
});
test('duplicate-calls-and-input-during-request-do-not-enable-or-post', async () => {
    const h=harness(); await h.arm(); const request=h.c.deleteAllRecordings();
    await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings(); h.type('');
    assert.equal(h.calls.length,1); assert.equal(h.nodes['delete-recordings-btn'].disabled,true);
    h.reply(); await request; h.checkSpent();
});
test('code-is-rechecked-at-the-second-click', async () => {
    const h=harness(); await h.arm(); h.nodes['delete-code-input'].value='changed-without-input-event';
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0); assert.equal(h.run('deleteConfirmCount'),0);
});
test('displayed-challenge-is-rechecked', async () => {
    const h=harness(); await h.arm(); h.nodes['delete-code'].textContent='different challenge';
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0); assert.equal(h.run('deleteConfirmCount'),0);
});
test('editing-away-and-back-requires-two-new-clicks', async () => {
    const h=harness(); await h.arm(); const code=h.nodes['delete-code'].textContent;
    h.type(''); h.type(code); await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0);
    const request=h.c.deleteAllRecordings(); h.reply(); await request; assert.equal(h.calls.length,1);
});
test('deadline-expiry-does-not-depend-on-timer-delivery', async () => {
    const h=harness(); await h.arm(); h.advance(3000);
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0); assert.equal(h.run('deleteConfirmCount'),1);
    const request=h.c.deleteAllRecordings(); h.reply(); await request;
});
test('confirmation-timer-disarms', async () => {
    const h=harness(); await h.arm(); h.flush(3000); assert.equal(h.run('deleteConfirmCount'),0);
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0);
});
for (const change of ['reopen','edit']) test('stale-timer-cannot-disarm-new-' + change, async () => {
    const h=harness(); await h.arm(); const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    if (change==='reopen') { h.c.closeDeleteRecordingsModal(); h.c.openDeleteAllRecordingsModal(); }
    h.type(h.nodes['delete-code'].textContent); await h.c.deleteAllRecordings();
    stale(); assert.equal(h.run('deleteConfirmCount'),1);
    const request=h.c.deleteAllRecordings(); h.reply(); await request; assert.equal(h.calls.length,1);
});
test('stale-timer-cannot-enable-a-pending-request', async () => {
    const h=harness(); await h.arm(); const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    const request=h.c.deleteAllRecordings(); stale(); assert.equal(h.nodes['delete-recordings-btn'].disabled,true);
    h.reply(); await request;
});
test('close-and-reopen-while-pending-does-not-start-another-attempt', async () => {
    const h=harness(); await h.arm(); const request=h.c.deleteAllRecordings();
    h.c.closeDeleteRecordingsModal(); h.c.openDeleteAllRecordingsModal();
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,1); assert.equal(h.run('deleteRecordingsPending'),true);
    assert.equal(h.nodes['delete-recordings-modal'].classList.contains('active'),false);
    h.reply(); await request; h.checkSpent();
});
test('outside-close-and-stale-focus-cannot-restore-confirmation', async () => {
    const h=harness(); h.c.openDeleteAllRecordingsModal();
    const focus=[...h.timers.values()].find(t => t.delay===100).fn;
    const modal=h.nodes['delete-recordings-modal']; modal.onclick({target:modal}); focus();
    assert.equal(h.nodes['delete-code-input'].focuses,0);
    await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings(); assert.equal(h.calls.length,0);
});

for (const [name,body,options,level,detail] of [
    ['complete',complete,{},'success','2 recording(s) deleted (1.25 MB)'],
    ['zero',{success:true,deleted:0,size_mb:0,failures:0},{},'success','0 recording(s) deleted (0.00 MB)'],
    ['partial',{success:false,deleted:1,size_mb:0.5,failures:1},{},'warning','Deletion incomplete'],
    ['contradictory-success',{...complete,failures:1},{},'warning','Deletion incomplete'],
    ['unsuccessful',{success:false,deleted:0,size_mb:0,failures:0},{},'warning','Deletion incomplete'],
    ['http-503',complete,{status:503},'error','HTTP 503'],
    ['http-500',complete,{status:500},'error','HTTP 500'],
    ['bad-json',complete,{badJSON:true},'error','no deletion totals are confirmed'],
    ['network',complete,{network:true},'error','Files may have been removed']
]) test('fresh-confirmation-after-' + name, async () => {
    const h=harness(); await h.arm(); const request=h.c.deleteAllRecordings();
    if (options.network) h.calls[0].reject(new Error('<img src=x onerror=bad()>'));
    else h.reply(body,options.status || 200,0,options.badJSON);
    await request; h.checkSpent(); assert.equal(h.notices[0].level,level); assert.ok(h.notices[0].message.includes(detail));
    assert.ok(!h.notices[0].message.includes('<img'));
    if (level==='error') assert.equal(h.refreshes,0);
    if (name==='partial') { assert.equal(h.refreshes,1); assert.ok(h.notices[0].message.includes('1 recording(s) deleted (0.50 MB)')); }
    await h.c.deleteAllRecordings(); await h.c.deleteAllRecordings(); assert.equal(h.calls.length,1);
    h.c.openDeleteAllRecordingsModal(); h.type(h.nodes['delete-code'].textContent);
    await h.c.deleteAllRecordings(); assert.equal(h.calls.length,1);
    const again=h.c.deleteAllRecordings(); assert.equal(h.calls.length,2); h.reply(); await again; h.checkSpent();
});
const absentFailures={...complete}; delete absentFailures.failures;
for (const [name,body] of [
    ['null',null], ['empty',{}], ['missing-failures',absentFailures],
    ['string-count',{...complete,deleted:'2'}], ['negative-count',{...complete,deleted:-1}],
    ['fraction-count',{...complete,deleted:1.5}], ['nonfinite-size',{...complete,size_mb:Infinity}],
    ['string-success',{...complete,success:'true'}]
]) test('malformed-' + name + '-is-unknown-and-spent', async () => {
    const h=harness(); await h.arm(); const request=h.c.deleteAllRecordings(); h.reply(body); await request; h.checkSpent();
    assert.equal(h.notices[0].level,'error'); assert.ok(h.notices[0].message.includes('no deletion totals are confirmed'));
    assert.equal(h.calls.length,1); assert.equal(h.refreshes,0);
});
test('synchronous-fetch-failure-also-spends-confirmation', async () => {
    const h=harness({syncThrow:true}); await h.arm(); await h.c.deleteAllRecordings(); h.checkSpent();
    assert.equal(h.calls.length,1); assert.equal(h.notices[0].level,'error');
});
test('timeline-refresh-error-does-not-reclassify-known-deletion', async () => {
    const h=harness({refreshThrow:true}); await h.arm(); const request=h.c.deleteAllRecordings(); h.reply(); await request;
    h.checkSpent(); assert.deepEqual(h.notices.map(n => n.level),['success','warning']);
    assert.ok(!h.notices.some(n => n.message.includes('outcome could not be verified')));
});
