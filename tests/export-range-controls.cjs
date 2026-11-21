const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const root=process.argv[2] || '.';
const nodes=new Map();let downloads=0,revoked=0,nextTimer=0;
const timers=new Map();
class Node {
  constructor(tag){this.tagName=tag;this.children=[];this.style={};this.dataset={};this.handlers={};this.disabled=false;this.classList={add(){}};}
  set id(value){this._id=value;nodes.set(value,this);}get id(){return this._id;}
  appendChild(child){child.parentElement=this;this.children.push(child);return child;}
  replaceChildren(...children){const remove=node=>{if(node.id)nodes.delete(node.id);node.children.forEach(remove);};this.children.forEach(remove);this.children=[];children.forEach(child=>this.appendChild(child));}
  setAttribute(key,value){this[key]=value;}addEventListener(name,fn){this.handlers[name]=fn;}
  click(){if(!this.disabled){if(this.tagName==='a')downloads++;this.handlers.click?.();}}
  remove(){}focus(){this.focused=true;}
}
const document={readyState:'loading',addEventListener(){},createElement:tag=>new Node(tag),getElementById:id=>nodes.get(id)||null,body:new Node('body')};
const toolbar=new Node('div'),wrap=toolbar.appendChild(new Node('div')),old=wrap.appendChild(new Node('div'));old.id='download-time-display';
for(const id of ['download-modal','download-status','download-progress-list','export-start-btn']){const node=new Node('div');node.id=id;}
const context=vm.createContext({console,document,window:{addEventListener(){}},AbortController,Blob,TextDecoder,Uint8Array,URL:{createObjectURL:()=> 'blob:prepared',revokeObjectURL(){revoked++;}},setTimeout(fn,ms){const id=++nextTimer;timers.set(id,{fn,ms});return id;},clearTimeout(id){timers.delete(id);}});
vm.runInContext('let downloadStartTime=null,downloadEndTime=null,downloadCameras=[],currentTimePosition=Date.parse("2026-11-21T23:59:59Z"),selectedCameras=["one"],allCameras=[{id:"one",name:"Hall"}];function renderDownloadCameraSelection(){}',context);
vm.runInContext(fs.readFileSync(path.join(root,'static/js/export-ui.js'),'utf8'),context);
const run=code=>vm.runInContext(code,context);
const headers=values=>({get:key=>values[key]??null});
const mp4=new Uint8Array([0,0,0,12,102,116,121,112,105,115,111,109]);
function response(status,values,data=mp4){let sent=false;return {status,ok:status===200,redirected:false,headers:headers(values),body:{getReader(){return {async read(){if(sent)return {done:true};sent=true;return {done:false,value:data};},cancel(){},releaseLock(){}};},cancel(){}}};}
const flush=()=>new Promise(resolve=>setImmediate(resolve));
async function main(){
  run('openDownloadModal()');
  assert.equal(nodes.get('download-range-step').value,'60000');
  assert.equal(run('downloadEndTime-downloadStartTime'),1200000);
  const start=run('downloadStartTime.getTime()'),end=run('downloadEndTime.getTime()');
  for(const boundary of ['start','end'])for(const direction of [-1,1]){
    run('resetDownloadTimeRange()');nodes.get(`download-${boundary}-${direction<0?'earlier':'later'}`).click();
    assert.equal(run('downloadStartTime.getTime()'),boundary==='start'?start+direction*60000:start);
    assert.equal(run('downloadEndTime.getTime()'),boundary==='end'?end+direction*60000:end);
  }
  for(const step of [1000,10000,60000,600000]){run('resetDownloadTimeRange()');nodes.get('download-range-step').value=String(step);run('adjustDownloadBoundary("start",1)');assert.equal(run('downloadStartTime.getTime()'),start+step);}
  run('downloadStartTime=new Date(1000);downloadEndTime=new Date(2000)');nodes.get('download-range-step').value='1000';run('updateDownloadTimeDisplay()');
  assert.equal(nodes.get('download-start-later').disabled,true);assert.equal(nodes.get('download-end-earlier').disabled,true);
  assert.equal(run('adjustDownloadBoundary("start",1)'),false);assert.equal(run('downloadEndTime.getTime()'),2000);
  assert.equal(run('exportRangeCandidate(NaN,2000,"start",1000)'),null);
  assert.equal(run('exportRangeCandidate(0,1000,"end",1e20)'),null);
  run('currentTimePosition=0;resetDownloadTimeRange()');assert.equal(run('downloadStartTime.getTime()'),start);
  run('currentTimePosition=Date.parse("2026-11-21T23:59:59Z");openDownloadModal()');
  assert.match(nodes.get('download-start-display').textContent,/2026/);assert.match(nodes.get('download-end-display').textContent,/2026/);
  // Elapsed-time arithmetic crosses DST transitions without a calendar-hour jump.
  run('downloadStartTime=new Date("2026-11-01T06:59:59Z");downloadEndTime=new Date("2026-11-01T08:00:00Z")');
  nodes.get('download-range-step').value='1000';run('adjustDownloadBoundary("start",1)');assert.equal(run('downloadStartTime.toISOString()'),'2026-11-01T07:00:00.000Z');
  run('resetDownloadTimeRange()');
  context.fetch=async()=>response(200,{'Content-Type':'video/mp4','Content-Length':String(mp4.length),'X-Export-Range-Policy':'decoded-video-frame-window','X-Export-Output-Duration-Seconds':'120'});
  const promise=run('startExportDownloads()');await flush();
  assert.equal(run('exportUiBatch.jobs[0].state'),'ready');assert.equal(downloads,0);
  assert.equal(run('exportUiBatch.jobs[0].save.textContent'),'Download prepared video');
  assert.equal(nodes.get('download-start-earlier').disabled,true);
  run('exportUiBatch.jobs[0].save.click()');assert.equal(downloads,1);assert.equal(run('exportUiBatch.jobs[0].state'),'handed');
  run('exportUiBatch.jobs[0].save.click()');assert.equal(downloads,1);
  run('exportUiBatch.jobs[0].cancel.click()');await promise;assert.equal(revoked,1);assert.equal(nodes.get('download-start-earlier').disabled,false);
  context.fetch=async()=>response(409,{'X-Export-Plan-Status':'gap'},new TextEncoder().encode('Missing coverage'));
  await run('startExportDownloads()');assert.match(run('exportUiBatch.jobs[0].status.textContent'),/full interval is not covered/);assert.equal(downloads,1);
  context.fetch=async()=>response(422,{'X-Export-Trim-Status':'unsupported'},new TextEncoder().encode('Audio-containing inputs unsupported'));
  await run('startExportDownloads()');assert.match(run('exportUiBatch.jobs[0].status.textContent'),/audio was not silently removed/);assert.equal(downloads,1);
  context.fetch=async()=>response(200,{'Content-Type':'video/mp4','X-Export-Range-Policy':'whole-clips-untrimmed'});
  const expires=run('startExportDownloads()');await flush();
  const expiry=[...timers.values()].find(timer=>timer.ms===120000);assert.ok(expiry);expiry.fn();await expires;
  assert.equal(run('exportUiBatch.jobs[0].state'),'expired');assert.equal(revoked,2);assert.equal(downloads,1);
  assert.equal(timers.size,0);
  console.log('PASS range directions, steps, crossing, reset, DST, covered-subsection handoff, gap, unsupported audio, expiry');
}
main().catch(error=>{console.error(error);process.exitCode=1;});
