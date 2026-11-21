const assert=require('node:assert/strict');
const {environment}=require('./export-ui-fixture.cjs');
const env=environment(()=>{throw Error('Range controls must not start requests');}),run=env.run;
run('openDownloadModal()');assert.equal(env.find('download-range-step').value,'60000');assert.equal(run('downloadEndTime-downloadStartTime'),1200000);
const start=run('downloadStartTime.getTime()'),end=run('downloadEndTime.getTime()');
for(const boundary of ['start','end'])for(const direction of [-1,1]){
 run('resetDownloadTimeRange()');env.find(`download-${boundary}-${direction<0?'earlier':'later'}`).click();
 assert.equal(run('downloadStartTime.getTime()'),boundary==='start'?start+direction*60000:start);
 assert.equal(run('downloadEndTime.getTime()'),boundary==='end'?end+direction*60000:end);
}
for(const step of [1000,10000,60000,600000]){run('resetDownloadTimeRange()');env.find('download-range-step').value=String(step);run('adjustDownloadBoundary("start",1)');assert.equal(run('downloadStartTime.getTime()'),start+step);}
run('downloadStartTime=new Date(1000);downloadEndTime=new Date(2000)');env.find('download-range-step').value='1000';run('updateDownloadTimeDisplay()');
assert.equal(env.find('download-start-later').disabled,true);assert.equal(env.find('download-end-earlier').disabled,true);assert.equal(run('adjustDownloadBoundary("start",1)'),false);
assert.equal(run('exportRangeCandidate(NaN,2000,"start",1000)'),null);assert.equal(run('exportRangeCandidate(0,1000,"end",1e20)'),null);
run('currentTimePosition=0;resetDownloadTimeRange()');assert.equal(run('downloadStartTime.getTime()'),start);
assert.match(env.find('download-start-display').textContent,/2026/);assert.match(env.find('download-end-display').textContent,/2026/);
run('downloadStartTime=new Date("2026-11-01T06:59:59Z");downloadEndTime=new Date("2026-11-01T08:00:00Z")');run('adjustDownloadBoundary("start",1)');assert.equal(run('downloadStartTime.toISOString()'),'2026-11-01T07:00:00.000Z');
run('exportUiBatch={running:true};updateDownloadTimeDisplay()');assert.equal(env.find('download-start-earlier').disabled,true);assert.equal(run('adjustDownloadBoundary("start",-1)'),false);
assert.equal(env.requests.length,0);console.log('PASS range directions, steps, boundary guards, initial reset, full dates, DST elapsed arithmetic and busy controls');
