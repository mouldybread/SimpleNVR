'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('static/js/playback.js','utf8');
class Video {
 constructor(){this.listeners=new Map();this.style={};this.message={style:{}};this.parentElement={querySelector:()=>this.message};this.duration=60;this.currentTime=0;this.ended=false;this.assignments=[];this.plays=0;this.pauses=0;this.nextPlay=null;}
 addEventListener(name,fn){if(!this.listeners.has(name))this.listeners.set(name,new Set());this.listeners.get(name).add(fn);}
 removeEventListener(name,fn){this.listeners.get(name)?.delete(fn);}
 handlers(name){return [...(this.listeners.get(name)||[])];}
 emit(name){for(const fn of this.handlers(name))fn();}
 set src(value){this.assignments.push(value);this.ended=false;}
 removeAttribute(){}pause(){this.pauses++;}
 play(){this.plays++;return this.nextPlay?this.nextPlay():Promise.resolve();}
}
function harness(extra=[[60,120,'/second.mp4']]){
 const timers=new Map(),errors=[];let id=0;const video=new Video();
 const c=vm.createContext({console,testVideo:video,document:{readyState:'loading',addEventListener(){},getElementById(){return null;}},showError:message=>errors.push(message),setTimeout(fn,delay){const key=++id;timers.set(key,{fn,delay});return key;},clearTimeout(key){timers.delete(key);}});
 vm.runInContext(source,c,{filename:'static/js/playback.js'});const run=code=>vm.runInContext(code,c);
 run(`currentDate=new Date(2026,10,10);allCameras=[{id:'a',name:'A'}];selectedCameras=['a'];videoPlayers=[testVideo];`);
 const start=new Date(2026,10,10,12).getTime();
 const segment=(a,b,path)=>({camera_id:'a',start_time:new Date(start+a*1000).toISOString(),end_time:new Date(start+b*1000).toISOString(),file_path:path});
 c.segments=[segment(0,60,'/first.mp4'),...extra.map(item=>segment(...item))];run('timelineSegments=segments');
 function flush(delay){for(const [key,timer]of [...timers])if(timer.delay===delay){timers.delete(key);timer.fn();}}
 function load(){flush(0);flush(10);video.emit('loadedmetadata');}
 function seek(){c.seekToTime(new Date(start+5000));flush(100);load();}
 function end(){video.currentTime=60;video.ended=true;video.emit('ended');}
 return {c,run,video,errors,start,flush,load,seek,end,timers};
}
function deferred(){let reject;const promise=new Promise((_,r)=>{reject=r;});return {promise,reject};}
async function settle(){await new Promise(resolve=>setImmediate(resolve));}
(async()=>{
 {const h=harness();h.seek();assert.equal(h.video.handlers('error').length,1);h.video.emit('error');assert.equal(h.run('isPlaying'),false);assert.equal(h.video.style.display,'none');assert.equal(h.errors.length,1);assert.equal(h.run('playbackSessions.size'),0);assert.equal(h.run('segmentCleanups.length'),0);h.end();h.load();assert.equal(h.video.assignments.length,1);}
 {const h=harness(),d=deferred();h.video.nextPlay=()=>d.promise;h.seek();d.reject(new Error('<untrusted rejection>'));await settle();assert.equal(h.run('isPlaying'),false);assert.equal(h.errors.length,1);assert.ok(!h.errors[0].includes('<untrusted'));assert.equal(h.video.style.display,'block');h.video.nextPlay=null;h.c.togglePlayPause();await settle();assert.equal(h.run('isPlaying'),true);assert.equal(h.video.plays,2);}
 {const h=harness();h.video.nextPlay=()=>{throw new Error('synchronous failure');};h.seek();assert.equal(h.run('isPlaying'),false);assert.equal(h.errors.length,1);}
 for(const change of ['pause','seek','invalidate','replace']){
  const h=harness(),d=deferred();h.video.nextPlay=()=>d.promise;h.seek();
  if(change==='pause')h.c.togglePlayPause();
  if(change==='seek')h.c.seekToTime(new Date(h.start+20000));
  if(change==='invalidate')h.c.invalidateTimelineLoad();
  if(change==='replace'){h.c.replacement=new Video();h.run('videoPlayers=[replacement]');}
  d.reject(new Error('stale'));await settle();assert.equal(h.errors.length,0,change);
 }
 {const h=harness(),d=deferred();h.video.nextPlay=()=>d.promise;h.seek();h.c.togglePlayPause();h.video.nextPlay=null;h.c.togglePlayPause();d.reject(new Error('older play request'));await settle();assert.equal(h.run('isPlaying'),true);assert.equal(h.errors.length,0);}
 {const h=harness();h.seek();const stale=h.video.handlers('error')[0];h.c.invalidateTimelineLoad();h.video.style.display='sentinel';stale();assert.equal(h.video.style.display,'sentinel');assert.equal(h.errors.length,0);assert.equal(h.video.handlers('error').length,0);}
 {const h=harness();h.seek();const stale=h.video.handlers('error')[0];h.c.replacement=new Video();h.run('videoPlayers=[replacement]');stale();assert.equal(h.errors.length,0);}
 {const h=harness();h.seek();h.c.togglePlayPause();h.end();assert.equal(h.video.assignments.length,1);h.c.togglePlayPause();h.load();assert.deepEqual(h.video.assignments,['/first.mp4','/second.mp4']);assert.equal(h.run('isPlaying'),true);assert.equal(h.errors.length,0);}
 for(const extra of [[],[[61,120,'/gap.mp4']],[[59,120,'/overlap.mp4']],[[60,120,'/one.mp4'],[60,130,'/two.mp4']],[[60,120,'/first.mp4']]]){
  const h=harness(extra);h.seek();h.c.togglePlayPause();h.end();h.c.togglePlayPause();h.load();assert.equal(h.run('isPlaying'),false);assert.equal(h.video.assignments.length,1);assert.equal(h.errors.length,1);
 }
 {const h=harness();h.seek();h.end();h.c.togglePlayPause();h.load();assert.equal(h.run('isPlaying'),false);assert.equal(h.video.plays,1);}
 {const h=harness();h.seek();h.c.togglePlayPause();h.video.ended=true;h.run(`allCameras.push({id:'b',name:'B'});selectedCameras.push('b');`);h.c.togglePlayPause();assert.equal(h.run('isPlaying'),false);assert.equal(h.video.assignments.length,1);assert.equal(h.errors.length,1);}
 {const h=harness();h.c.seekToTime(new Date(h.start+5000));h.flush(100);h.flush(0);h.c.togglePlayPause();h.c.togglePlayPause();assert.equal(h.video.plays,0);h.flush(10);h.video.emit('loadedmetadata');assert.equal(h.video.plays,1);}
 {const h=harness();h.seek();h.video.emit('error');const plays=h.video.plays;h.c.togglePlayPause();assert.equal(h.video.plays,plays);assert.equal(h.run('isPlaying'),false);}
 console.log('Playback failure regressions passed');
})().catch(error=>{console.error(error);process.exitCode=1;});
