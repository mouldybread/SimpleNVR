const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('static/js/export-ui.js','utf8');
const template=fs.readFileSync('templates/index.html','utf8');
assert.ok(template.includes('onclick="startExportDownloads()"'));
assert.ok(!template.includes('onclick="startDownload()"'));
assert.ok(template.indexOf('/static/js/playback.js')<template.indexOf('/static/js/export-ui.js'));
const media=Uint8Array.from([0,0,0,12,102,116,121,112,105,115,111,109]);
function environment(fetcher,ids=['front']){
 const timers=new Map(),created=[],revoked=[],handed=[],requests=[];let next=0;
 class Element{
  constructor(tag){this.tagName=tag.toUpperCase();this.children=[];this.style={};this.dataset={};this.listeners={};this.text='';this.disabled=false;}
  set textContent(value){this.text=String(value);this.children=[];}
  get textContent(){return this.text+this.children.map(child=>child.textContent).join('');}
  set innerHTML(_){throw Error('HTML sink used');}
  set outerHTML(_){throw Error('HTML sink used');}
  insertAdjacentHTML(){throw Error('HTML sink used');}
  setAttribute(name){assert.ok(!/^on/i.test(name));}
  appendChild(child){child.parent=this;this.children.push(child);return child;}
  replaceChildren(){this.children=[];this.text='';}
  addEventListener(name,fn){this.listeners[name]=fn;}
  remove(){if(this.parent)this.parent.children=this.parent.children.filter(child=>child!==this);}
  click(){if(this.disabled)return;if(this.tagName==='A')handed.push({url:this.href,name:this.download});else this.listeners.click?.();}
 }
 const body=new Element('body'),list=new Element('div'),host=new Element('div'),button=new Element('button');
 const ctx=vm.createContext({console,AbortController,Blob,TextDecoder,Uint8Array,URL:{createObjectURL(blob){const url='blob:test-'+created.length;created.push({url,size:blob.size});return url;},revokeObjectURL(url){revoked.push(url);}},document:{body,createElement:tag=>new Element(tag),getElementById:id=>({'download-status':host,'download-progress-list':list,'export-start-btn':button}[id] || null)},window:{addEventListener(){}},setTimeout(fn,delay){const id=++next;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);},fetch(url,options){requests.push({url,options});return fetcher(url,options);}});
 vm.runInContext(`let downloadCameras=${JSON.stringify(ids)};let allCameras=[{id:'front',name:'<img src=x onerror=pwned=1>'},{id:'back',name:'Back'}];let downloadStartTime=new Date('2026-11-10T00:00:00Z');let downloadEndTime=new Date('2026-11-10T00:00:02Z');`,ctx);
 vm.runInContext(source,ctx);
 return {ctx,timers,created,revoked,handed,requests,list,jobs:()=>vm.runInContext('exportUiBatch.jobs',ctx)};
}
function response(status=200,data=media,extra={}){
 return {status,ok:status===200,redirected:false,headers:new Headers({'Content-Type':status===200?'video/mp4':'text/plain','X-Export-Range-Policy':'decoded-video-frame-window','X-Export-Output-Duration-Seconds':'2',...extra}),body:new ReadableStream({start(controller){if(data)controller.enqueue(data);controller.close();}})};
}
async function ticks(){for(let i=0;i<5;i++)await new Promise(resolve=>setImmediate(resolve));}
(async()=>{
 const good=environment(()=>Promise.resolve(response()),['front','back']);
 const batch=good.ctx.startExportDownloads();await ticks();
 assert.equal(good.requests.length,1);assert.equal(good.jobs()[0].state,'ready');assert.equal(good.created.length,1);assert.equal(good.handed.length,0);
 assert.ok(good.list.textContent.includes('<img src=x onerror=pwned=1>'));assert.equal(good.ctx.pwned,undefined);
 vm.runInContext(`downloadStartTime=new Date('2026-11-11T00:00:00Z')`,good.ctx);
 good.jobs()[0].save.click();assert.equal(good.handed.length,1);assert.equal(good.jobs()[0].state,'handed');assert.ok(good.jobs()[0].status.textContent.includes('not verified'));
 good.jobs()[0].cancel.click();await ticks();assert.equal(good.revoked.length,1);assert.equal(good.requests.length,2);assert.ok(good.requests[1].url.includes('2026-11-10T00%3A00%3A00.000Z'));
 good.ctx.cancelExportDownloads();await batch;assert.equal(good.revoked.length,2);assert.equal(good.jobs()[1].state,'canceled');
 for(const [status,headers] of [[409,{'X-Export-Plan-Status':'gap'}],[409,{'X-Export-Plan-Status':'overlap'}],[422,{}],[413,{}],[504,{}],[401,{}]]){
  const raw=new TextEncoder().encode('<svg onload=pwned=1>server message');const env=environment(()=>Promise.resolve(response(status,raw,headers)));
  await env.ctx.startExportDownloads();assert.equal(env.jobs()[0].state,'failed');assert.equal(env.created.length,0);assert.equal(env.handed.length,0);assert.ok(env.list.textContent.includes('<svg onload=pwned=1>'));assert.equal(env.ctx.pwned,undefined);
 }
 for(const value of [response(200,media,{'Content-Length':String(64*1024*1024+1)}),response(200,media,{'Content-Length':'13'}),response(200,media,{'Content-Type':'text/html'}),response(200,Uint8Array.from([1,2,3])),response(200,media,{'X-Export-Range-Policy':'unknown'})]){
  const env=environment(()=>Promise.resolve(value));await env.ctx.startExportDownloads();assert.equal(env.jobs()[0].state,'failed');assert.equal(env.created.length,0);assert.equal(env.handed.length,0);
 }
 const canceled=environment(()=>new Promise(()=>{}),['front','back']);const pending=canceled.ctx.startExportDownloads();await ticks();canceled.ctx.cancelExportDownloads();await pending;
 assert.equal(canceled.requests.length,1);assert.ok(canceled.requests[0].options.signal.aborted);assert.equal(canceled.jobs()[0].state,'canceled');assert.equal(canceled.jobs()[1].state,'canceled');
 const stalled=environment(()=>new Promise(()=>{}));const timed=stalled.ctx.startExportDownloads();await ticks();
 [...stalled.timers.values()].find(timer=>timer.delay===240000).fn();await timed;assert.equal(stalled.jobs()[0].state,'failed');assert.ok(stalled.jobs()[0].status.textContent.includes('deadline'));
 let readerCanceled=false;
 const streaming=environment(()=>Promise.resolve({...response(),body:{getReader(){return {read(){return new Promise(()=>{});},cancel(){readerCanceled=true;return Promise.resolve();},releaseLock(){}}}}}));
 const receiving=streaming.ctx.startExportDownloads();await ticks();streaming.ctx.cancelExportDownloads();await receiving;assert.ok(readerCanceled);assert.equal(streaming.created.length,0);
 const expired=environment(()=>Promise.resolve(response()));const expires=expired.ctx.startExportDownloads();await ticks();[...expired.timers.values()].find(timer=>timer.delay===120000).fn();await expires;
 assert.equal(expired.jobs()[0].state,'expired');assert.equal(expired.revoked.length,1);assert.equal(expired.handed.length,0);
 console.log('Export UI status, limits, text-only rendering and cancellation regressions passed');
})().catch(error=>{console.error(error);process.exitCode=1;});
