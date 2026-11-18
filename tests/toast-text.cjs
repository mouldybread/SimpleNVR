const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');
const source = fs.readFileSync('static/js/toast.js', 'utf8');

class Element {
    constructor(tag) { this.tagName = tag; this.children = []; this.parentElement = null;
        this.className = ''; this.value = ''; this.listeners = new Map();
        this.classList = {
            add: name => { const names = new Set(this.className.split(/\s+/).filter(Boolean)); names.add(name); this.className = [...names].join(' '); },
            remove: name => { this.className = this.className.split(/\s+/).filter(item => item !== name).join(' '); },
            contains: name => this.className.split(/\s+/).includes(name)
        };
    }
    set textContent(value) { this.value = String(value); this.children = []; }
    get textContent() { return this.value + this.children.map(child => child.textContent).join(''); }
    set innerHTML(_) { throw new Error('HTML insertion is forbidden'); }
    set outerHTML(_) { throw new Error('HTML insertion is forbidden'); }
    insertAdjacentHTML() { throw new Error('HTML insertion is forbidden'); }
    setAttribute(name, _) { assert.ok(!/^on/i.test(name), 'inline event handler'); }
    appendChild(child) { assert.ok(child instanceof Element); child.parentElement = this; this.children.push(child); return child; }
    addEventListener(name, fn) { if (!this.listeners.has(name)) this.listeners.set(name, []); this.listeners.get(name).push(fn); }
    click() { for (const fn of this.listeners.get('click') || []) fn({target:this}); }
    remove() { if (!this.parentElement) return; const parent = this.parentElement;
        parent.children = parent.children.filter(child => child !== this); this.parentElement = null;
    }
}
function harness(missing = false) {
    const container = new Element('div'), timers = new Map(); let next = 0, creations = 0;
    const context = vm.createContext({
        document:{getElementById:id => { assert.equal(id, 'toast-container'); return missing ? null : container; },
            createElement:tag => { creations++; return new Element(tag); }},
        window:{}, setTimeout(fn, delay) { const id = ++next; timers.set(id, {fn,delay}); return id; },
        fetch() { throw new Error('Notifications must not make requests'); }
    });
    vm.runInContext(source, context, {filename:'static/js/toast.js'});
    return {context,container,timers,get creations() { return creations; },
        flush(delay) { for (const [id, timer] of [...timers]) if (timer.delay === delay) { timers.delete(id); timer.fn(); } }
    };
}
function verify(h, toast, message, variant = 'info', glyph = 'ℹ') {
    assert.equal(h.container.children.at(-1), toast);
    assert.equal(toast.className, 'toast ' + variant);
    assert.deepEqual(toast.children.map(child => child.tagName), ['div','div','div']);
    const [icon, text, close] = toast.children;
    assert.equal(icon.className, 'toast-icon'); assert.equal(icon.textContent, glyph);
    assert.equal(text.className, 'toast-message'); assert.equal(text.textContent, String(message));
    assert.equal(text.children.length, 0, 'message created markup descendants');
    assert.equal(close.className, 'toast-close'); assert.equal(close.textContent, '×');
    assert.equal((close.listeners.get('click') || []).length, 1);
    return {icon,text,close};
}
for (const [name, variant, glyph] of [
    ['showSuccess','success','✓'], ['showError','error','✗'],
    ['showWarning','warning','⚠'], ['showInfo','info','ℹ']
]) test('wrapper-' + name, () => {
    const h = harness(), message = '<b>literal notification</b>';
    verify(h, h.context[name](message), message, variant, glyph);
});
for (const [name, message] of [
    ['image','<img src=x onerror="globalThis.injected=true">'],
    ['script','<script>globalThis.injected=true</script>'],
    ['svg','<svg onload="globalThis.injected=true"></svg>'],
    ['breakout','</div><div onclick="bad()">camera</div>'],
    ['entities','&lt;img&gt; &amp; &#60;script&#62;'],
    ['quotes-newline','"single\' & <>\nsecond line'],
    ['unicode','攝影機 Küche Привет ✓'],
    ['camera-caller',`Successfully deleted 3 event(s) for <img src=x onerror=bad()>`],
    ['server-error-caller','Failed to scan: <b>backend response</b>']
]) test('literal-' + name, () => {
    const h = harness(); verify(h, h.context.showToast(message), message);
    assert.equal(h.context.injected, undefined);
});
for (const [name, variant] of [
    ['unknown','other'], ['prototype','__proto__'], ['constructor','constructor'],
    ['html','"><img src=x onerror=bad()>'], ['non-string',{toString() { throw new Error('type coerced'); }}]
]) test('type-fallback-' + name, () => {
    const h = harness(); verify(h, h.context.showToast('message', variant), 'message');
});
for (const [name, value] of [['zero',0], ['boolean',false], ['null',null], ['undefined',undefined]])
    test('string-conversion-' + name, () => {
        const h = harness(); verify(h, h.context.showToast(value), value);
    });
test('default-animation-and-expiry', () => {
    const h = harness(), toast = h.context.showToast('message');
    assert.deepEqual([...h.timers.values()].map(timer => timer.delay), [10,3000]);
    assert.equal(toast.classList.contains('show'), false); h.flush(10);
    assert.equal(toast.classList.contains('show'), true); h.flush(3000);
    assert.equal(toast.classList.contains('show'), false); assert.equal(h.container.children.length, 1);
    assert.deepEqual([...h.timers.values()].map(timer => timer.delay), [300]); h.flush(300);
    assert.equal(h.container.children.length, 0);
});
test('custom-duration', () => {
    const h = harness(); h.context.showWarning('message', 6000);
    assert.deepEqual([...h.timers.values()].map(timer => timer.delay), [10,6000]);
});
for (const duration of [0,-1]) test('persistent-duration-' + duration, () => {
    const h = harness(), toast = h.context.showToast('message','info',duration);
    assert.deepEqual([...h.timers.values()].map(timer => timer.delay), [10]); h.flush(10);
    assert.equal(toast.parentElement, h.container); assert.equal(h.timers.size, 0);
});
test('close-only-selected-toast-and-late-timers-safe', () => {
    const h = harness(), one = h.context.showToast('one'), two = h.context.showToast('two');
    one.children[2].click(); assert.deepEqual(h.container.children, [two]);
    h.flush(10); h.flush(3000); h.flush(300); assert.equal(h.container.children.length, 0);
});
test('show-alert-compatibility', () => {
    const h = harness(); assert.equal(h.context.window.showAlert('<b>alert text</b>'), undefined);
    verify(h, h.container.children[0], '<b>alert text</b>');
    assert.deepEqual([...h.timers.values()].map(timer => timer.delay), [10,4000]);
});
test('missing-container-no-work', () => {
    const h = harness(true); assert.equal(h.context.showToast('<img src=x>'), undefined);
    assert.equal(h.creations, 0); assert.equal(h.timers.size, 0);
});
