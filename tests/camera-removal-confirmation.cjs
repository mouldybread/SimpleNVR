const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/cameras.js', 'utf8');
const fragment = '<div class="camera-box">Current camera list</div>';
function element(role, options) {
    const classes = new Set(); let html = '';
    return {value:'',textContent:'',disabled:false,style:{},focuses:0,
        focus() { this.focuses++; },
        classList:{add:n => classes.add(n),remove:n => classes.delete(n),contains:n => classes.has(n)},
        set innerHTML(value) {
            assert.ok(role==='camera-list' || role==='modal-camera-list', 'unexpected HTML sink');
            if (options.renderThrow) throw new Error('display failed'); html=value;
        }, get innerHTML() { return html; }
    };
}
function harness(options = {}) {
    const ids=['delete-cameras-code','delete-cameras-code-input','delete-cameras-btn','delete-cameras-modal','camera-list','modal-camera-list'];
    const nodes=Object.fromEntries(ids.map(id => [id,element(id,options)]));
    for (const id of options.missing || []) delete nodes[id];
    const timers=new Map(),calls=[],notices=[]; let next=0,now=1000,refreshes=0;
    class Clock extends Date { static now() { return now; } }
    const c=vm.createContext({
        Date:Clock,document:{getElementById:id => nodes[id] || null,addEventListener(){}},
        setTimeout(fn,delay) { const id=++next; timers.set(id,{fn,delay}); return id; },clearTimeout:id => timers.delete(id),
        console:{error(){}},
        showSuccess:message => notices.push({level:'success',message}),showInfo:message => notices.push({level:'info',message}),
        showWarning:message => notices.push({level:'warning',message}),showError:message => notices.push({level:'error',message}),
        updateSectionHeight(id) { assert.equal(id,'cameras-content'); if (options.heightThrow) throw new Error('height failed'); },
        loadTimelineCameras() {
            refreshes++; if (options.refreshThrow) throw new Error('refresh failed');
            return options.refreshReject ? Promise.reject(new Error('refresh rejected')) : Promise.resolve();
        },
        fetch(url,init) {
            assert.equal(url,'/api/cameras/delete-all');
            let resolve,reject; const promise=new Promise((a,b) => { resolve=a;reject=b; });
            calls.push({url,init,resolve,reject});
            if (options.syncThrow) throw new Error('<img src=x onerror=bad()>');
            return promise;
        }
    });
    vm.runInContext(source,c,{filename:'static/js/cameras.js'}); c.setupDeleteCamerasModal();
    const state=code => vm.runInContext(code,c);
    return {c,nodes,timers,calls,notices,state,get refreshes() { return refreshes; },
        advance(ms) { now+=ms; },flush(delay) { for (const [id,t] of [...timers]) if(t.delay===delay) { timers.delete(id);t.fn(); } },
        type(value) { nodes['delete-cameras-code-input'].value=value;nodes['delete-cameras-code-input'].oninput(); },
        async arm() { c.openDeleteAllCamerasModal();this.type(nodes['delete-cameras-code'].textContent);await c.removeAllCameras(); },
        reply(body=fragment,status=200,extra={},index=calls.length-1) {
            calls[index].resolve({status,ok:status>=200 && status<300,redirected:!!extra.redirected,
                headers:{get(name) { assert.equal(name,'Content-Type'); return extra.type === undefined ? 'text/html; charset=utf-8' : extra.type; }},
                text:() => extra.readError ? Promise.reject(new Error('body failed')) : Promise.resolve(body)});
        },
        checkSpent() {
            assert.equal(state('deleteCamerasCode'),'');assert.equal(state('deleteCamerasConfirmCount'),0);
            assert.equal(state('deleteCamerasPending'),false);assert.equal(state('deleteCamerasDeadline'),0);
            assert.equal(nodes['delete-cameras-code-input'].value,'');assert.equal(nodes['delete-cameras-btn'].disabled,true);
            assert.equal(nodes['delete-cameras-btn'].classList.contains('confirm'),false);
        }
    };
}
test('closed-and-empty-challenge-cannot-submit', async () => {
    const h=harness();await h.c.removeAllCameras();await h.c.removeAllCameras();
    h.c.openDeleteAllCamerasModal();h.type('');await h.c.removeAllCameras();await h.c.removeAllCameras();assert.equal(h.calls.length,0);
});
for (const id of ['delete-cameras-code','delete-cameras-code-input','delete-cameras-btn','delete-cameras-modal'])
    test('missing-' + id, async () => {
        const h=harness({missing:[id]});h.c.openDeleteAllCamerasModal();await h.c.removeAllCameras();await h.c.removeAllCameras();assert.equal(h.calls.length,0);
    });
test('wrong-code-cannot-submit', async () => {
    const h=harness();h.c.openDeleteAllCamerasModal();h.type('wrong');await h.c.removeAllCameras();await h.c.removeAllCameras();assert.equal(h.calls.length,0);
});
test('first-click-only-arms', async () => {
    const h=harness();await h.arm();assert.equal(h.calls.length,0);assert.equal(h.state('deleteCamerasConfirmCount'),1);
});
test('second-click-rechecks-input-and-displayed-code', async () => {
    for (const id of ['delete-cameras-code-input','delete-cameras-code']) {
        const h=harness();await h.arm();h.nodes[id][id.endsWith('input')?'value':'textContent']='changed';
        await h.c.removeAllCameras();assert.equal(h.calls.length,0);assert.equal(h.state('deleteCamerasConfirmCount'),0);
    }
});
test('editing-away-and-back-disarms', async () => {
    const h=harness();await h.arm();const code=h.nodes['delete-cameras-code'].textContent;h.type('');h.type(code);
    await h.c.removeAllCameras();assert.equal(h.calls.length,0);const pending=h.c.removeAllCameras();h.reply();await pending;h.checkSpent();
});
test('deadline-checked-with-delayed-timer', async () => {
    const h=harness();await h.arm();h.advance(3000);await h.c.removeAllCameras();assert.equal(h.calls.length,0);
    const pending=h.c.removeAllCameras();h.reply();await pending;h.checkSpent();
});
test('timer-expiry-disarms', async () => {
    const h=harness();await h.arm();h.flush(3000);assert.equal(h.state('deleteCamerasConfirmCount'),0);
    await h.c.removeAllCameras();assert.equal(h.calls.length,0);
});
for (const kind of ['edit','reopen']) test('stale-timer-cannot-disarm-new-' + kind, async () => {
    const h=harness();await h.arm();const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    if(kind==='reopen') { h.c.closeDeleteAllCamerasModal();h.c.openDeleteAllCamerasModal(); }
    h.type(h.nodes['delete-cameras-code'].textContent);await h.c.removeAllCameras();stale();
    assert.equal(h.state('deleteCamerasConfirmCount'),1);const pending=h.c.removeAllCameras();h.reply();await pending;
});
test('confirmation-consumed-before-fetch-and-duplicates-refused', async () => {
    const h=harness();await h.arm();const pending=h.c.removeAllCameras();
    assert.equal(h.calls.length,1);assert.equal(h.state('deleteCamerasCode'),'');assert.equal(h.state('deleteCamerasConfirmCount'),0);
    assert.equal(h.nodes['delete-cameras-code-input'].disabled,true);assert.equal(h.nodes['delete-cameras-btn'].disabled,true);
    assert.equal(h.calls[0].init.method,'POST');assert.equal(h.calls[0].init.redirect,'error');
    assert.equal(h.calls[0].init.headers['Content-Type'],'application/json');assert.equal(h.calls[0].init.body,undefined);
    await h.c.removeAllCameras();h.type('');await h.c.removeAllCameras();assert.equal(h.calls.length,1);
    h.reply();await pending;h.checkSpent();assert.equal(h.nodes['camera-list'].innerHTML,fragment);
    assert.equal(h.nodes['modal-camera-list'].innerHTML,fragment);assert.equal(h.refreshes,1);
    assert.equal(h.notices[0].level,'info');assert.ok(!h.notices[0].message.includes('all cameras'));
});
test('close-reopen-during-pending-cannot-start-new-attempt', async () => {
    const h=harness();await h.arm();const pending=h.c.removeAllCameras();h.c.closeDeleteAllCamerasModal();h.c.openDeleteAllCamerasModal();
    await h.c.removeAllCameras();assert.equal(h.calls.length,1);assert.equal(h.state('deleteCamerasPending'),true);
    assert.equal(h.nodes['delete-cameras-modal'].classList.contains('active'),false);h.reply();await pending;h.checkSpent();
});
test('outside-close-and-stale-focus-do-not-restore-state', async () => {
    const h=harness();h.c.openDeleteAllCamerasModal();const focus=[...h.timers.values()].find(t => t.delay===100).fn;
    const modal=h.nodes['delete-cameras-modal'];modal.onclick({target:modal});focus();assert.equal(h.nodes['delete-cameras-code-input'].focuses,0);
    await h.c.removeAllCameras();await h.c.removeAllCameras();assert.equal(h.calls.length,0);
});
test('stale-confirmation-timer-cannot-enable-pending-button', async () => {
    const h=harness();await h.arm();const stale=[...h.timers.values()].find(t => t.delay===3000).fn;
    const pending=h.c.removeAllCameras();stale();assert.equal(h.nodes['delete-cameras-btn'].disabled,true);h.reply();await pending;
});
test('pending-covers-the-response-body', async () => {
    const h=harness();await h.arm();const pending=h.c.removeAllCameras();let finish;
    h.calls[0].resolve({ok:true,status:200,redirected:false,headers:{get:()=>'text/html'},text:()=>new Promise(resolve=>{finish=resolve;})});
    await new Promise(resolve=>setImmediate(resolve));assert.equal(h.state('deleteCamerasPending'),true);
    await h.c.removeAllCameras();assert.equal(h.calls.length,1);finish(fragment);await pending;h.checkSpent();
});
for (const [name,body,status,extra,network,level] of [
    ['html',fragment,200,{},false,'info'],
    ['remaining-camera','<div class="camera-box">Remaining camera</div>',200,{},false,'info'],
    ['conflict','<b>retained evidence</b>',409,{},false,'error'],
    ['persistence','save failed',500,{},false,'error'],
    ['shutdown','stopping',503,{},false,'error'],
    ['redirected',fragment,200,{redirected:true},false,'error'],
    ['empty','',200,{},false,'error'],
    ['wrong-type','{}',200,{type:'application/json'},false,'error'],
    ['missing-type',fragment,200,{type:null},false,'error'],
    ['full-page','<!DOCTYPE html><html><body>Sign in</body></html>',200,{},false,'error'],
    ['read-error',fragment,200,{readError:true},false,'error'],
    ['network',fragment,200,{},true,'error']
]) test('fresh-confirmation-after-' + name, async () => {
    const h=harness();await h.arm();const pending=h.c.removeAllCameras();
    if(network) h.calls[0].reject(new Error('<img src=x onerror=bad()>'));else h.reply(body,status,extra);
    await pending;h.checkSpent();assert.equal(h.calls.length,1);assert.equal(h.notices[0].level,level);
    assert.ok(!h.notices[0].message.includes('<img'));assert.ok(!h.notices[0].message.includes('<b>'));
    if(level==='error') {
        assert.ok(h.notices[0].message.includes('may have been stopped or removed'));
        assert.ok(h.notices[0].message.includes('no removal totals are confirmed'));
        assert.equal(h.nodes['camera-list'].innerHTML,'');assert.equal(h.refreshes,0);
    }
    await h.c.removeAllCameras();await h.c.removeAllCameras();assert.equal(h.calls.length,1);
    await h.arm();assert.equal(h.calls.length,1);const again=h.c.removeAllCameras();assert.equal(h.calls.length,2);
    h.reply();await again;h.checkSpent();
});
test('sync-fetch-error-spends-confirmation', async () => {
    const h=harness({syncThrow:true});await h.arm();await h.c.removeAllCameras();h.checkSpent();assert.equal(h.calls.length,1);
    assert.equal(h.notices[0].level,'error');
});
for (const option of ['renderThrow','heightThrow','refreshThrow','refreshReject']) test('received-response-not-reclassified-by-' + option, async () => {
    const h=harness({[option]:true});await h.arm();const pending=h.c.removeAllCameras();h.reply();await pending;
    await new Promise(resolve=>setImmediate(resolve));h.checkSpent();
    assert.ok(h.notices.some(n=>n.level==='warning'));
    assert.ok(!h.notices.some(n=>n.level==='error' || n.message.includes('outcome could not be verified')));
});
test('missing-camera-lists-is-a-display-warning-after-acknowledgment', async () => {
    const h=harness({missing:['camera-list','modal-camera-list']});await h.arm();const pending=h.c.removeAllCameras();h.reply();await pending;
    h.checkSpent();assert.equal(h.notices[0].level,'warning');assert.ok(!h.notices[0].message.includes('outcome could not be verified'));
});
