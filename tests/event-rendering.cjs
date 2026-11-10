const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

// Deliberately reject every HTML parsing sink during the two renderers.
// This is a DOM-contract test, not a browser integration test.
class Element {
  constructor(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.style = {}; this.listeners = {}; this.text = ''; this.parent = null; }
  set textContent(value) { this.text = String(value); this.children = []; }
  get textContent() { return this.text + this.children.map(child => child.textContent).join(''); }
  set innerHTML(_) { throw new Error('HTML sink used'); }
  insertAdjacentHTML() { throw new Error('HTML sink used'); }
  set outerHTML(_) { throw new Error('HTML sink used'); }
  setAttribute(name) { if (/^on/i.test(name)) throw new Error('inline handler used'); }
  appendChild(child) { child.parent = this; this.children.push(child); return child; }
  replaceChildren(...children) { this.text = ''; this.children = []; children.forEach(child => this.appendChild(child)); }
  addEventListener(name,fn) { this.listeners[name] = fn; }
  remove() { if (this.parent) this.parent.children = this.parent.children.filter(child => child !== this); }
  click() { assert.equal(this.onclick,undefined); this.listeners.click?.(); }
}
const list = new Element('div');
const body = new Element('body');
const context = vm.createContext({console,document:{readyState:'loading',body,addEventListener(){},createElement:tag => new Element(tag),getElementById:id => id === 'events-list' ? list : null},window:{}});
for (const file of ['static/js/events.js','static/js/playback.js']) vm.runInContext(fs.readFileSync(file,'utf8'),context,{filename:file});
const payload = `<img src=x onerror="globalThis.pwned=1"><svg onload="globalThis.pwned=2">& ' \\ </script>`;
const camera = `front');globalThis.pwned=3;// ${payload}`;
const timestamp = `');globalThis.pwned=4;//`;
const event = {id:payload,label:payload,camera_name:camera,start_time:timestamp,duration_seconds:payload};
let navigation;
context.seekToEvent = (...args) => { navigation = args; };
const descendants = node => [node,...node.children.flatMap(descendants)];
context.renderEventsList([event]);
assert.ok(list.textContent.includes(payload)); assert.ok(list.textContent.includes(camera));
assert.equal(descendants(list).filter(node => node.tagName === 'IMG' || node.tagName === 'SVG' || node.tagName === 'SCRIPT').length,0);
const button = descendants(list).find(node => node.tagName === 'BUTTON');
assert.ok(button); button.click(); assert.deepEqual(navigation,[camera,timestamp]); assert.equal(context.pwned,undefined);
context.showEventTooltip(event,10,20);
assert.equal(body.children.length,1); assert.ok(body.children[0].textContent.includes(payload)); assert.ok(body.children[0].textContent.includes(camera));
for (const label of ['constructor','__proto__','toString','hasOwnProperty']) {
  assert.equal(context.eventIconText(label),'?');
  context.showEventTooltip({...event,label},10,20);
  assert.equal(body.children.length,1); assert.ok(body.children[0].textContent.startsWith('? '+label));
}
assert.equal(context.eventIconText('Motion Detected'),'???');
context.hideEventTooltip(); assert.equal(body.children.length,0);
context.renderEventsList([]); assert.equal(list.textContent,'No events to display');
context.renderEventsList([{id:'normal',label:'Motion Detected',camera_name:'Front Door',start_time:'2026-11-10T12:00:00Z',duration_seconds:10}]);
assert.ok(list.textContent.includes('Front Door')); assert.ok(list.textContent.includes('10 seconds'));
assert.equal(context.pwned,undefined);
console.log('Event-list and tooltip DOM-contract regressions passed');
