'use strict';
const assert = require('node:assert/strict');
const g = require('../static/js/timeline-geometry.js');
const expected = {'UTC':[24,24], 'Asia/Hong_Kong':[24,24], 'America/Chicago':[23,25]};
assert.ok(expected[process.env.TZ], 'configured test timezone required');
for (const [i, month] of [2,10].entries()) {
  const day = month === 2 ? 8 : 1;
  const start = new Date(2026, month, day).getTime(), end = new Date(2026, month, day + 1).getTime();
  const v = g.view(start, end);
  assert.equal(v.hours, expected[process.env.TZ][i]);
  assert.equal(g.timeAt(0,v),start); assert.equal(g.timeAt(1,v),end);
  for (const f of [0,0.1,0.5,0.9,1]) assert.ok(Math.abs(g.fractionAt(g.timeAt(f,v),v)-f)<1e-10);
  assert.deepEqual(g.clip(start-3600000,start+3600000,v),{start, end:start+3600000,left:0,right:1/v.hours});
  assert.equal(g.clip(end,end+1,v),null);
  assert.equal(g.clip(start-1,start,v),null);
  assert.equal(g.clip(NaN,end,v),null);
  assert.equal(g.clip(end,start,v),null);
  const zoomed = g.zoomAt(v,0.5,4);
  assert.equal(g.timeAt(0.5,zoomed),g.timeAt(0.5,v));
  assert.equal(g.zoomAt(v,0,4).start,start);
  assert.equal(g.zoomAt(v,1,4).end,end);
  assert.equal(g.view(start,end,4,999).end,end);
  assert.equal(g.view(start,end,4,-999).start,start);
  assert.equal(g.timeAt(-1,v),start); assert.equal(g.timeAt(2,v),end);
}
if (process.env.TZ === 'America/Chicago') {
  const start = new Date(2026,10,1).getTime(), end = new Date(2026,10,2).getTime(), v = g.view(start,end);
  const first = Date.parse('2026-11-01T01:30:00-05:00'), second = Date.parse('2026-11-01T01:30:00-06:00');
  assert.equal(g.label(first),'01:30 UTC-05:00'); assert.equal(g.label(second),'01:30 UTC-06:00');
  assert.ok(g.fractionAt(second,v)>g.fractionAt(first,v));
  assert.equal(g.timeAt(g.fractionAt(second,v),v),second);
  assert.equal(g.label(Date.parse('2026-03-08T01:30:00-06:00')),'01:30 UTC-06:00');
  assert.equal(g.label(Date.parse('2026-03-08T03:30:00-05:00')),'03:30 UTC-05:00');
}
for (const args of [[0,0],[1,0],[NaN,1],[0,27*3600000],[0,1,0],[0,1,25],[0,1,NaN],[0,1,1,NaN]]) assert.throws(()=>g.view(...args),RangeError);
assert.throws(()=>g.timeAt(NaN,g.view(0,1)),RangeError);
assert.throws(()=>g.label(NaN),RangeError);
console.log('Timeline geometry passed in '+process.env.TZ);
