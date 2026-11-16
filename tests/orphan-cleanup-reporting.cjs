const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/cameras.js', 'utf8');
const valid = {success:true,count:2,size_mb:1.25,skipped_recovery_dirs:0,failures:0,interrupted:false,limit_reached:false};

function element() {
    return {textContent:'Cleanup Complete',disabled:false,style:{},
        classList:{add(){},remove(){}},
        set innerHTML(value) { throw new Error('Cleanup result used innerHTML'); }};
}
function harness(body, options = {}) {
    const button = element();
    const elements = {};
    if (!options.noPanels) for (const prefix of ['', 'modal-']) {
        const heading = element();
        const panel = element(); panel.querySelector = () => heading; panel.heading = heading;
        elements[prefix + 'cleanup-results'] = panel;
        elements[prefix + 'cleanup-results-text'] = element();
    }
    const calls = []; const notices = [];
    const context = vm.createContext({
        document:{addEventListener(){}, getElementById:id => elements[id] || null},
        event:{target:button}, console:{error(){}},
        setTimeout:() => 1, clearTimeout(){},
        fetch:(url, init) => {
            calls.push({url, init});
            if (options.network) return Promise.reject(new Error('<img src=x onerror=bad()>'));
            const status = options.status || 200;
            return Promise.resolve({ok:status >= 200 && status < 300,status,
                json:() => options.badJSON ? Promise.reject(new SyntaxError('invalid JSON')) : Promise.resolve(body)});
        },
        showSuccess:message => notices.push({level:'success',message}),
        showWarning:message => notices.push({level:'warning',message}),
        showError:message => notices.push({level:'error',message})
    });
    vm.runInContext(source, context);
    return {button,elements,calls,notices, async run() {
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
    }};
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
