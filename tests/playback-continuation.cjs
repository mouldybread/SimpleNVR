'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
class Video {
 constructor(){this.listeners=new Map();this.style={};this.message={style:{}};this.parentElement={querySelector:()=>this.message};this.assignments=[];this.duration=60;this.currentTime=0;this.ended=false;this.plays=0;}
 addEventListener(name,fn){if(!this.listeners.has(name))this.listeners.set(name,new Set());this.listeners.get(name).add(fn);}
 removeEventListener(name,fn){this.listeners.get(name)?.delete(fn);}
 handlers(name){return [...(this.listeners.get(name)||[])];}
 emit(name){for(const fn of this.handlers(name))fn();}
 set src(value){this.assignments.push(value);this.ended=false;}
 removeAttribute(){}pause(){}play(){this.plays++;return Promise.resolve();}
}
const source=fs.readFileSync('static/js/playback.js','utf8');
function harness(extra=[]){
 const timers=new Map(),errors=[];let id=0;const video=new Video();
 const c=vm.createContext({console,testVideo:video,document:{readyState:'loading',addEventListener(){},getElementById(){return null;}},showError:message=>errors.push(message),setTimeout(fn,delay){const key=++id;timers.set(key,{fn,delay});return key;},clearTimeout(key){timers.delete(key);}});
 vm.runInContext(source,c,{filename:'static/js/playback.js'});const run=code=>vm.runInContext(code,c);
 run(`currentDate=new Date(2026,10,10);allCameras=[{id:'a',name:'A'}];selectedCameras=['a'];videoPlayers=[testVideo];`);
 const start=new Date(2026,10,10,12).getTime(),segment=(a,b,path)=>({camera_id:'a',start_time:new Date(start+a*1000).toISOString(),end_time:new Date(start+b*1000).toISOString(),file_path:path});
 c.segments=[segment(0,60,'/first.mp4'),...extra.map(item=>segment(...item))];run('timelineSegments=segments');
 function flush(delay){for(const [key,timer]of [...timers])if(timer.delay===delay){timers.delete(key);timer.fn();}}
 function load(){flush(0);flush(10);video.emit('loadedmetadata');}
 function seek(){c.seekToTime(new Date(start+5000));flush(100);load();}
 function end(){video.currentTime=60;video.ended=true;video.emit('ended');}
 return {c,run,video,errors,start,segment,flush,load,seek,end};
}
{const h=harness([[60,120,'/second.mp4']]);h.seek();h.video.currentTime=25.5;h.video.emit('timeupdate');assert.equal(h.run('currentTimePosition'),h.start+25500);h.run('playbackSpeed=2');h.end();h.load();assert.deepEqual(h.video.assignments,['/first.mp4','/second.mp4']);assert.equal(h.video.currentTime,0);assert.equal(h.video.playbackRate,2);assert.equal(h.errors.length,0);assert.equal(h.run('isPlaying'),true);}
for(const extra of [[],[[61,120,'/gap.mp4']],[[59,120,'/overlap.mp4']],[[60,120,'/one.mp4'],[60,130,'/two.mp4']]]){const h=harness(extra);h.seek();h.end();h.load();assert.equal(h.video.assignments.length,1);assert.equal(h.run('isPlaying'),false);assert.equal(h.errors.length,1);assert.equal(h.run('segmentCleanups.length'),0);}
{const h=harness([[60,120,'/second.mp4']]);h.seek();const ended=h.video.handlers('ended')[0],update=h.video.handlers('timeupdate')[0];h.c.invalidateTimelineLoad();const position=h.run('currentTimePosition');h.video.currentTime=60;h.video.ended=true;ended();update();h.load();assert.equal(h.video.assignments.length,1);assert.equal(h.run('currentTimePosition'),position);assert.equal(h.video.handlers('ended').length,0);}
{const h=harness([[60,120,'/second.mp4']]);h.seek();h.end();h.c.togglePlayPause();h.load();assert.equal(h.run('isPlaying'),false);assert.equal(h.video.plays,1);}
{const h=harness([[60,120,'/second.mp4']]);h.run(`allCameras.push({id:'b',name:'B'});selectedCameras.push('b');`);h.seek();assert.equal(h.video.handlers('ended').length,0);assert.equal(h.video.handlers('timeupdate').length,0);}
{const h=harness();const end=new Date(2026,10,11).getTime();h.c.night=[{camera_id:'a',start_time:new Date(end-60000).toISOString(),end_time:new Date(end).toISOString(),file_path:'/night.mp4'},{camera_id:'a',start_time:new Date(end).toISOString(),end_time:new Date(end+60000).toISOString(),file_path:'/tomorrow.mp4'}];h.run('timelineSegments=night');h.c.seekToTime(new Date(end-5000));h.flush(100);h.load();h.end();h.load();assert.equal(h.video.assignments.length,1);assert.equal(h.run('isPlaying'),false);assert.equal(h.run('currentTimePosition'),end-1);assert.equal(h.errors.length,1);}
console.log('Conservative single-camera continuation regressions passed');
