const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/modals.js', 'utf8');
const cleared = {success:true,count:3};
function element() {
    const classes=new Set();
    return {value:'',textContent:'',disabled:false,style:{},focuses:0,
        focus() { this.focuses++; },
        classList:{add:n=>classes.add(n),remove:n=>classes.delete(n),contains:n=>classes.has(n)},
        set innerHTML(_) { throw new Error('Event clear must not insert HTML'); }};
}
function harness(options={}) {
    const nodes=Object.fromEntries(['clear-events-code','clear-events-code-input','clear-events-btn','clear-events-modal'].map(id=>[id,element()]));
    for(const id of options.missing || []) delete nodes[id];
    const ready=[],timers=new Map(),calls=[],notices=[];let next=0,now=1000,refreshes=0;
    class Clock extends Date { static now() { return now; } }
    const c=vm.createContext({
        Date:Clock,window:{},document:{getElementById:id=>nodes[id] || null,
            addEventListener(name,fn) { assert.equal(name,'DOMContentLoaded');ready.push(fn); }},
        setTimeout(fn,delay) { const id=++next;timers.set(id,{fn,delay});return id; },clearTimeout:id=>timers.delete(id),
        console:{error(){}},showSuccess:message=>notices.push({level:'success',message}),
        showWarning:message=>notices.push({level:'warning',message}),showError:message=>notices.push({level:'error',message}),
        refreshEventsList() { refreshes++;if(options.refreshThrow) throw new Error('refresh failed');
            return options.refreshReject ? Promise.reject(new Error('refresh rejected')) : Promise.resolve(); },
        fetch(url,init) {
            assert.equal(url,'/api/events/clear-all');
            let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});
            calls.push({url,init,resolve,reject});
            if(options.syncThrow) throw new Error('<img src=x onerror=bad()>');return promise;
        }
    });
    vm.runInContext(source,c,{filename:'static/js/modals.js'});ready.forEach(fn=>fn());
    const state=code=>vm.runInContext(code,c);
    return {c,nodes,timers,calls,notices,state,get refreshes() { return refreshes; },
        advance(ms) { now+=ms; },flush(delay) { for(const [id,t] of [...timers]) if(t.delay===delay) {timers.delete(id);t.fn();} },
        type(value) { nodes['clear-events-code-input'].value=value;nodes['clear-events-code-input'].oninput(); },
        async arm() { c.openClearEventsModal();this.type(nodes['clear-events-code'].textContent);await c.clearAllEvents(); },
        reply(body=cleared,status=200,extra={},index=calls.length-1) {
            calls[index].resolve({status,ok:status>=200 && status<300,redirected:!!extra.redirected,
                headers:{get(name) {assert.equal(name,'Content-Type');return extra.type===undefined?'application/json; charset=utf-8':extra.type;}},
                json:()=>extra.badJSON?Promise.reject(new SyntaxError('bad JSON')):Promise.resolve(body)});
        },
        checkSpent() {
            assert.equal(state('clearEventsCode'),'');assert.equal(state('clearEventsConfirmCount'),0);
            assert.equal(state('clearEventsPending'),false);assert.equal(state('clearEventsDeadline'),0);
            assert.equal(nodes['clear-events-btn'].disabled,true);assert.equal(nodes['clear-events-code-input'].value,'');
            assert.equal(nodes['clear-events-btn'].classList.contains('confirm'),false);
        }
    };
}
test('closed-empty-and-wrong-code-cannot-submit', async()=>{
    const h=harness();await h.c.clearAllEvents();await h.c.clearAllEvents();h.c.openClearEventsModal();
    for(const value of ['', 'wrong']) {h.type(value);await h.c.clearAllEvents();await h.c.clearAllEvents();}
    assert.equal(h.calls.length,0);
});
for(const id of ['clear-events-code','clear-events-code-input','clear-events-btn','clear-events-modal'])
    test('missing-'+id,async()=>{
        const h=harness({missing:[id]});h.c.openClearEventsModal();await h.c.clearAllEvents();await h.c.clearAllEvents();assert.equal(h.calls.length,0);
    });
test('first-click-only-arms',async()=>{
    const h=harness();await h.arm();assert.equal(h.calls.length,0);assert.equal(h.state('clearEventsConfirmCount'),1);
});
for(const id of ['clear-events-code-input','clear-events-code']) test('second-click-rechecks-'+id,async()=>{
    const h=harness();await h.arm();h.nodes[id][id.endsWith('input')?'value':'textContent']='changed';
    await h.c.clearAllEvents();assert.equal(h.calls.length,0);assert.equal(h.state('clearEventsConfirmCount'),0);
});
test('input-edit-disarms-prior-click',async()=>{
    const h=harness();await h.arm();const code=h.nodes['clear-events-code'].textContent;h.type('');h.type(code);
    await h.c.clearAllEvents();assert.equal(h.calls.length,0);const request=h.c.clearAllEvents();h.reply();await request;h.checkSpent();
});
test('expired-deadline-refuses-even-before-timer-runs',async()=>{
    const h=harness();await h.arm();h.advance(3000);await h.c.clearAllEvents();assert.equal(h.calls.length,0);
    const request=h.c.clearAllEvents();h.reply();await request;
});
test('confirmation-timer-disarms',async()=>{
    const h=harness();await h.arm();h.flush(3000);assert.equal(h.state('clearEventsConfirmCount'),0);
    await h.c.clearAllEvents();assert.equal(h.calls.length,0);
});
for(const kind of ['edit','reopen']) test('stale-timer-cannot-disarm-new-'+kind,async()=>{
    const h=harness();await h.arm();const stale=[...h.timers.values()].find(t=>t.delay===3000).fn;
    if(kind==='reopen') {h.c.closeClearEventsModal();h.c.openClearEventsModal();}
    h.type(h.nodes['clear-events-code'].textContent);await h.c.clearAllEvents();stale();
    assert.equal(h.state('clearEventsConfirmCount'),1);const request=h.c.clearAllEvents();h.reply();await request;
});
test('confirmation-consumed-before-fetch-and-duplicate-input-calls-refused',async()=>{
    const h=harness();await h.arm();const request=h.c.clearAllEvents();
    assert.equal(h.calls.length,1);assert.equal(h.state('clearEventsCode'),'');assert.equal(h.state('clearEventsConfirmCount'),0);
    assert.equal(h.nodes['clear-events-btn'].disabled,true);assert.equal(h.nodes['clear-events-code-input'].disabled,true);
    assert.equal(h.calls[0].init.method,'POST');assert.equal(h.calls[0].init.redirect,'error');assert.equal(h.calls[0].init.body,undefined);
    await h.c.clearAllEvents();h.type('');await h.c.clearAllEvents();assert.equal(h.calls.length,1);
    h.reply();await request;h.checkSpent();assert.equal(h.refreshes,1);
    assert.equal(h.notices[0].level,'success');assert.ok(h.notices[0].message.includes('event metadata, not recording files'));
});
test('close-reopen-during-request-does-not-rearm',async()=>{
    const h=harness();await h.arm();const request=h.c.clearAllEvents();h.c.closeClearEventsModal();h.c.openClearEventsModal();
    await h.c.clearAllEvents();assert.equal(h.calls.length,1);assert.equal(h.state('clearEventsPending'),true);
    assert.equal(h.nodes['clear-events-modal'].classList.contains('active'),false);h.reply();await request;h.checkSpent();
});
test('outside-close-and-stale-focus-do-not-restore-state',async()=>{
    const h=harness();h.c.openClearEventsModal();const focus=[...h.timers.values()].find(t=>t.delay===100).fn;
    const modal=h.nodes['clear-events-modal'];modal.onclick({target:modal});focus();
    assert.equal(h.nodes['clear-events-code-input'].focuses,0);await h.c.clearAllEvents();await h.c.clearAllEvents();assert.equal(h.calls.length,0);
});
test('stale-timer-cannot-enable-pending-button',async()=>{
    const h=harness();await h.arm();const stale=[...h.timers.values()].find(t=>t.delay===3000).fn;
    const request=h.c.clearAllEvents();stale();assert.equal(h.nodes['clear-events-btn'].disabled,true);h.reply();await request;
});
test('pending-includes-json-body-read',async()=>{
    const h=harness();await h.arm();const request=h.c.clearAllEvents();let finish;
    h.calls[0].resolve({ok:true,status:200,redirected:false,headers:{get:()=>'application/json'},json:()=>new Promise(resolve=>{finish=resolve;})});
    await new Promise(resolve=>setImmediate(resolve));assert.equal(h.state('clearEventsPending'),true);
    await h.c.clearAllEvents();assert.equal(h.calls.length,1);finish(cleared);await request;h.checkSpent();
});
for(const [name,body,status,extra,network,level] of [
    ['success',cleared,200,{},false,'success'], ['zero',{success:true,count:0},200,{},false,'success'],
    ['negative',{success:false},200,{},false,'warning'],
    ['negative-count-untrusted',{success:false,count:999,error:'<img src=x onerror=bad()>'},200,{},false,'warning'],
    ['persistence-error',cleared,500,{},false,'error'], ['denied',cleared,403,{},false,'error'],
    ['redirected',cleared,200,{redirected:true},false,'error'],
    ['wrong-type',cleared,200,{type:'text/html'},false,'error'], ['missing-type',cleared,200,{type:null},false,'error'],
    ['bad-json',cleared,200,{badJSON:true},false,'error'], ['transport',cleared,200,{},true,'error']
]) test('fresh-confirmation-after-'+name,async()=>{
    const h=harness();await h.arm();const request=h.c.clearAllEvents();
    if(network) h.calls[0].reject(new Error('<img src=x onerror=bad()>'));else h.reply(body,status,extra);
    await request;h.checkSpent();assert.equal(h.notices[0].level,level);assert.ok(!h.notices[0].message.includes('<img'));
    if(level==='success') assert.ok(h.notices[0].message.includes(body.count+' event record(s) cleared'));
    else {assert.equal(h.refreshes,0);assert.ok(h.notices[0].message.includes('no cleared count is confirmed'));}
    if(name==='negative-count-untrusted') assert.ok(!h.notices[0].message.includes('999'));
    if(level==='error') assert.ok(h.notices[0].message.includes('metadata may already have changed'));
    await h.c.clearAllEvents();await h.c.clearAllEvents();assert.equal(h.calls.length,1);
    await h.arm();assert.equal(h.calls.length,1);const again=h.c.clearAllEvents();assert.equal(h.calls.length,2);
    h.reply();await again;h.checkSpent();
});
for(const [name,body] of [
    ['null',null],['array',[]],['empty',{}],['string-success',{success:'true',count:3}],
    ['missing-count',{success:true}],['string-count',{success:true,count:'3'}],
    ['negative-count',{success:true,count:-1}],['fraction-count',{success:true,count:1.5}],
    ['unsafe-count',{success:true,count:Number.MAX_SAFE_INTEGER+1}]
]) test('invalid-'+name+'-is-unknown-and-spent',async()=>{
    const h=harness();await h.arm();const request=h.c.clearAllEvents();h.reply(body);await request;h.checkSpent();
    assert.equal(h.notices[0].level,'error');assert.ok(h.notices[0].message.includes('no cleared count is confirmed'));assert.equal(h.refreshes,0);
});
test('sync-fetch-error-spends-confirmation',async()=>{
    const h=harness({syncThrow:true});await h.arm();await h.c.clearAllEvents();h.checkSpent();
    assert.equal(h.calls.length,1);assert.equal(h.notices[0].level,'error');
});
for(const option of ['refreshThrow','refreshReject']) test('known-result-not-reclassified-by-'+option,async()=>{
    const h=harness({[option]:true});await h.arm();const request=h.c.clearAllEvents();h.reply();await request;
    await new Promise(resolve=>setImmediate(resolve));h.checkSpent();
    assert.deepEqual(h.notices.map(n=>n.level),['success','warning']);
    assert.ok(!h.notices.some(n=>n.message.includes('outcome could not be verified')));
});
