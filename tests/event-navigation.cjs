const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const timers = new Map(), requests = [], errors = [], seeks = [];
let nextTimer = 0;
const picker = {value:''};
const context = vm.createContext({console,AbortController,document:{readyState:'loading',addEventListener(){},getElementById:id=>id==='timeline-date-picker'?picker:null},window:{},localStorage:{getItem(){return null;},setItem(){}},showError:message=>errors.push(message),setTimeout(fn,delay){const id=++nextTimer;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);},fetch(url,options){return new Promise((resolve,reject)=>requests.push({url,options,resolve,reject}));}});
for (const file of ['static/js/events.js','static/js/playback.js']) vm.runInContext(fs.readFileSync(file,'utf8'),context,{filename:file});
vm.runInContext(`allCameras=[{id:'front',name:'Front Door'},{id:'back',name:'Back Door'}]; selectedCameras=['back'];`,context);
context.renderCameraSelector=()=>{}; context.renderTimeline=()=>{}; context.updateVideoGrid=()=>{};
context.performSeek=time=>seeks.push(time.getTime());
function runTimers(delay) { for (const [id,timer] of [...timers]) if (timer.delay===delay) {timers.delete(id);timer.fn();} }
const settle=()=>new Promise(resolve=>setImmediate(resolve));
const key=date=>`${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`;
async function complete(request,segments=[]) {request.resolve({ok:true,json:async()=>segments});await settle();}
(async()=>{
  for (const timestamp of ['2026-11-10T00:30:00Z','2026-11-10T23:30:00Z','2026-03-08T08:30:00Z','2026-11-01T07:30:00Z']) {
    const before=seeks.length;
    assert.equal(context.seekToEvent('Front Door',timestamp),true);
    assert.equal(picker.value,key(new Date(timestamp)));
    assert.ok(vm.runInContext(`selectedCameras.includes('front')`,context));
    runTimers(200); const request=requests.at(-1);
    assert.equal(new URL(request.url,'http://local').searchParams.get('date'),picker.value);
    runTimers(100); assert.equal(seeks.length,before,'seek ran before data arrived');
    await complete(request); runTimers(100); assert.equal(seeks.at(-1),Date.parse(timestamp));
  }
  const oldCount=requests.length;
  for (const value of ['',null,'not-a-date']) assert.equal(context.seekToEvent('Front Door',value),false);
  assert.equal(context.seekToEvent('Missing','2026-11-10T12:00:00Z'),false);
  vm.runInContext(`allCameras.push({id:'duplicate',name:'Front Door'})`,context);
  assert.equal(context.seekToEvent('Front Door','2026-11-10T12:00:00Z'),false);
  vm.runInContext(`allCameras.pop()`,context); runTimers(200); assert.equal(requests.length,oldCount);
  assert.equal(context.seekToTime(12.5),false); assert.equal(context.seekToTime(new Date(NaN)),false);
  context.seekToEvent('Front Door','2026-11-09T12:00:00Z'); runTimers(200); const old=requests.at(-1);
  context.seekToEvent('Front Door','2026-11-10T12:00:00Z'); runTimers(200); const latest=requests.at(-1);
  assert.equal(old.options.signal.aborted,true);
  await complete(latest,[{camera_id:'front',file_path:'latest'}]); runTimers(100);
  const position=seeks.at(-1), count=seeks.length;
  await complete(old,[{camera_id:'front',file_path:'stale'}]); runTimers(100);
  assert.equal(seeks.length,count); assert.equal(seeks.at(-1),position); assert.equal(vm.runInContext(`timelineSegments[0].file_path`,context),'latest');
  context.seekToEvent('Front Door','2026-11-10T13:00:00Z'); runTimers(200); const canceled=requests.at(-1);
  vm.runInContext(`currentDate=new Date(2026,10,11);loadTimelineData()`,context); runTimers(200); const manual=requests.at(-1);
  await complete(canceled); runTimers(100); assert.equal(seeks.length,count);
  await complete(manual);
  context.seekToEvent('Front Door','2026-11-10T14:00:00Z'); runTimers(200);
  requests.at(-1).resolve({ok:false}); await settle(); runTimers(100); assert.equal(seeks.length,count);
  assert.ok(errors.length>=7);
  // A previously queued seek cannot run after a new navigation invalidates it.
  context.seekToTime(new Date('2026-11-10T15:00:00Z'));
  const staleTimer=[...timers.values()].find(timer=>timer.delay===100).fn;
  context.seekToEvent('Front Door','2026-11-10T16:00:00Z'); staleTimer(); assert.equal(seeks.length,count);
  console.log('Event navigation regressions passed in '+process.env.TZ);
})().catch(error=>{console.error(error);process.exitCode=1;});
