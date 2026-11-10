'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=fs.readFileSync('static/js/playback.js','utf8');
class Video {
 constructor(){this.listeners=new Map();this.assignments=[];this.style={};this.message={style:{}};this.duration=60;this.currentTime=0;this.playCount=0;this.pauseCount=0;this.parentElement={querySelector:()=>this.message};}
 addEventListener(name,fn){if(!this.listeners.has(name))this.listeners.set(name,new Set());this.listeners.get(name).add(fn);}
 removeEventListener(name,fn){this.listeners.get(name)?.delete(fn);}
 handlers(name){return [...(this.listeners.get(name)||[])];}
 emit(name){for(const fn of this.handlers(name))fn();}
 set src(value){this.assignments.push(value);this.source=value;}
 removeAttribute(name){if(name==='src')this.source=null;}
 pause(){this.pauseCount++;}
 play(){this.playCount++;return Promise.resolve();}
}
function harness(){
 const timers=new Map();let next=0;
 const c=vm.createContext({console,document:{readyState:'loading',addEventListener(){},getElementById(){return null;}},setTimeout(fn,delay){const id=++next;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);}});
 vm.runInContext(source,c,{filename:'static/js/playback.js'});
 const run=code=>vm.runInContext(code,c);
 const video=new Video();c.testVideo=video;
 run(`allCameras=[{id:'a',name:'A'}];selectedCameras=['a'];videoPlayers=[testVideo];timelineSegments=[{camera_id:'a',start_time:'2026-11-10T12:00:00Z',end_time:'2026-11-10T12:01:00Z',file_path:'/first.mp4'}];`);
 function flush(delay){for(const [id,timer]of [...timers])if(timer.delay===delay){timers.delete(id);timer.fn();}}
 function seek(seconds=5){c.seekToTime(new Date(Date.parse('2026-11-10T12:00:00Z')+seconds*1000));flush(100);flush(0);flush(10);}
 return {c,run,video,timers,flush,seek};
}
{
 const h=harness();h.seek(12.5);assert.deepEqual(h.video.assignments,['/first.mp4']);h.video.emit('loadedmetadata');
 assert.equal(h.video.currentTime,12.5);assert.equal(h.video.playCount,1);assert.equal(h.video.playbackRate,1);
 assert.equal(h.video.handlers('loadedmetadata').length,0);assert.equal(h.video.handlers('error').length,0);
 assert.equal(h.run('mediaCleanups.length'),0);
}
{
 const h=harness();h.seek();const metadata=h.video.handlers('loadedmetadata')[0],failed=h.video.handlers('error')[0];
 h.c.invalidateTimelineLoad();assert.equal(h.video.handlers('loadedmetadata').length,0);assert.equal(h.video.handlers('error').length,0);
 h.video.style.display='sentinel';h.video.message.style.display='sentinel';metadata();failed();
 assert.equal(h.video.playCount,0);assert.equal(h.video.currentTime,0);assert.equal(h.video.style.display,'sentinel');assert.equal(h.video.message.style.display,'sentinel');
}
{
 const h=harness();h.c.seekToTime(new Date('2026-11-10T12:00:05Z'));
 const stale=[...h.timers.values()].find(timer=>timer.delay===100).fn;
 h.c.seekToTime(new Date('2026-11-10T12:00:20Z'));stale();h.flush(100);h.flush(0);h.flush(10);h.video.emit('loadedmetadata');
 assert.deepEqual(h.video.assignments,['/first.mp4']);assert.equal(h.video.currentTime,20);
}
{
 const h=harness();h.c.seekToTime(new Date('2026-11-10T12:00:05Z'));h.flush(100);
 const stale=[...h.timers.values()].find(timer=>timer.delay===0).fn;h.c.invalidateTimelineLoad();stale();h.flush(10);
 assert.equal(h.video.assignments.length,0);assert.equal(h.timers.size,0);
}
{
 const h=harness();h.c.seekToTime(new Date('2026-11-10T12:00:05Z'));h.flush(100);h.flush(0);
 const stale=[...h.timers.values()].find(timer=>timer.delay===10).fn;h.c.invalidateTimelineLoad();stale();
 assert.equal(h.video.assignments.length,0);assert.equal(h.timers.size,0);
}
{
 const h=harness();h.seek();const metadata=h.video.handlers('loadedmetadata')[0];
 const replacement=new Video();h.c.replacement=replacement;h.run('videoPlayers=[replacement]');metadata();
 assert.equal(h.video.playCount,0);assert.equal(replacement.playCount,0);assert.equal(replacement.assignments.length,0);
}
{
 const h=harness();h.seek();h.video.emit('error');
 assert.equal(h.video.style.display,'none');assert.equal(h.video.message.style.display,'block');assert.equal(h.video.playCount,0);
 assert.equal(h.video.handlers('loadedmetadata').length,0);assert.equal(h.video.handlers('error').length,0);assert.equal(h.run('mediaCleanups.length'),0);
}
{
 const h=harness();h.seek(60);assert.equal(h.video.assignments.length,0);assert.equal(h.video.style.display,'none');assert.equal(h.video.message.style.display,'block');
}
{
 const h=harness();h.seek();h.c.togglePlayPause();assert.equal(h.run('isPlaying'),false);h.video.emit('loadedmetadata');assert.equal(h.video.playCount,0);
}
console.log('Playback media lifecycle regressions passed');
