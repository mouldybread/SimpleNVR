const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('web/static/js/modals.js', 'utf8');
const origin = 'https://nvr.invalid:9443';
const videoHTML = '<video id="liveVideo" controls autoplay muted style="width: 100%; max-height: 70vh;"></video>';
function harness(onDemand=false) {
    const nodes = new Map(), errors = [], assignments = [];
    let creations = 0, rewrites = 0;
    function modalNodes() {
        const classes = new Set();
        nodes.set('liveModal',{style:{},classList:{add:n=>classes.add(n),remove:n=>classes.delete(n),contains:n=>classes.has(n)}});
        nodes.set('modalTitle',{value:'',set textContent(value) {this.value=String(value);},get textContent() {return this.value;},
            set innerHTML(_) {throw new Error('Camera name reached HTML');}});
        nodes.set('streamContainer',{set innerHTML(value) {
            assert.equal(value,videoHTML,'Only the existing fixed video markup may be inserted');rewrites++;
            const video={pauses:0,pause() {this.pauses++;},value:'',
                set src(value) {this.value=value;assignments.push(value);},get src() {return this.value;}};
            nodes.set('liveVideo',video);
        }});
    }
    if (!onDemand) modalNodes();
    const context = vm.createContext({
        document:{getElementById:id=>nodes.get(id) || null,addEventListener(){},body:{insertAdjacentHTML(where,html) {
            assert.equal(where,'beforeend');assert.ok(html.includes('id="liveModal"'));assert.ok(html.includes('id="streamContainer"'));
            creations++;modalNodes();
        }}},
        window:{},showError:message=>errors.push(message),console,
        fetch() {throw new Error('Live-name tests must not contact a server');}
    });
    vm.runInContext(source,context,{filename:'web/static/js/modals.js'});
    return {context,nodes,errors,assignments,get creations() {return creations;},get rewrites() {return rewrites;},
        state:code=>vm.runInContext(code,context)};
}
function verify(h,name) {
    h.context.viewLiveStream(name);
    assert.equal(h.errors.length,0);
    const path=h.assignments.at(-1),url=new URL(path,origin);
    assert.equal(path,'/api/stream/'+encodeURIComponent(name));
    assert.equal(url.origin,origin);assert.equal(url.search,'');assert.equal(url.hash,'');
    assert.equal(url.pathname,'/api/stream/'+encodeURIComponent(name));
    assert.equal(decodeURIComponent(url.pathname.slice('/api/stream/'.length)),name);
    assert.equal(h.nodes.get('modalTitle').textContent,'Live Feed: '+name);
    assert.equal(h.nodes.get('liveModal').classList.contains('active'),true);
}
for (const [label,name] of [
    ['simple','Front'],['space','Front Door'],['query','Door?mode=admin&x=1'],['fragment','Door#West'],
    ['percent','Door%2FWest'],['slash','Door/West'],['backslash','Door\\West'],['dot-path','Door/../West'],
    ['network-looking','//example.invalid'],['url-looking','https://example.invalid/x?y#z'],
    ['unicode','攝影機 Küche Привет'],['quotes-markup','Door\'"<img src=x onerror=bad()>'],
    ['literal-encoded-dots','%2e%2e'],['question-only','?'],['hash-only','#'],['spaces-preserved',' .. ']
]) test('component-'+label,()=>verify(harness(),name));
for (const [label,name] of [
    ['empty',''],['dot','.'],['dot-dot','..'],['null',null],['undefined',undefined],
    ['object',{toString() {throw new Error('untrusted object');}}],['malformed-unicode','\ud800']
]) test('invalid-'+label+'-refused-before-modal-or-video-work',()=>{
    const h=harness(true);h.context.viewLiveStream(name);
    assert.equal(h.assignments.length,0);assert.equal(h.creations,0);assert.equal(h.rewrites,0);
    assert.equal(h.errors.length,1);assert.equal(h.errors[0],'Camera name cannot be used for live playback; review the camera configuration.');
});
test('on-demand-modal-still-works',()=>{
    const h=harness(true);verify(h,'Door#1');assert.equal(h.creations,1);assert.equal(h.rewrites,1);
});
test('reuse-does-not-create-another-modal',()=>{
    const h=harness(true);verify(h,'Door?1');verify(h,'Door#2');
    assert.equal(h.creations,1);assert.equal(h.rewrites,2);assert.equal(h.assignments.length,2);
});
test('invalid-name-does-not-relabel-or-replace-current-stream',()=>{
    const h=harness();verify(h,'Existing');const current=h.nodes.get('liveVideo');
    for (const name of ['..','\ud800',null]) h.context.viewLiveStream(name);
    assert.equal(h.assignments.length,1);assert.equal(h.nodes.get('liveVideo'),current);
    assert.equal(h.nodes.get('modalTitle').textContent,'Live Feed: Existing');assert.equal(h.rewrites,1);
});
test('close-still-pauses-and-clears-current-video',()=>{
    const h=harness();verify(h,'Door#1');const video=h.nodes.get('liveVideo');h.context.closeLiveModal();
    assert.equal(video.pauses,1);assert.equal(video.src,'');assert.equal(h.state('currentVideo'),null);
    assert.equal(h.nodes.get('liveModal').classList.contains('active'),false);
});
test('existing-outside-click-handler-still-closes-live-modal',()=>{
    const h=harness();verify(h,'Door?1');const video=h.nodes.get('liveVideo');
    h.context.window.onclick({target:h.nodes.get('liveModal')});assert.equal(video.pauses,1);assert.equal(h.state('currentVideo'),null);
});
