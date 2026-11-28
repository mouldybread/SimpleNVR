'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const source=['static/js/playback-state.js','static/js/playback-camera-date.js','static/js/playback-timeline-load.js','static/js/playback-canvas.js','static/js/playback-seek-session.js','static/js/playback-video-controls.js'].map(f=>fs.readFileSync(f,'utf8')).join('');
class Video {
 constructor(){this.listeners=new Map();this.style={};this.message={style:{}};this.parentElement={querySelector:()=>this.message};this.duration=60;this.currentTime=0;this.ended=false;this.plays=0;this.pauses=0;this.nextPlay=null;this.assignments=[];}
 addEventListener(name,fn){if(!this.listeners.has(name))this.listeners.set(name,new Set());this.listeners.get(name).add(fn);}
 removeEventListener(name,fn){this.listeners.get(name)?.delete(fn);}
 emit(name){for(const fn of [...(this.listeners.get(name)||[])])fn();}
 set src(value){this.assignments.push(value);this.ended=false;}
 removeAttribute(){}pause(){this.pauses++;}play(){this.plays++;return this.nextPlay?this.nextPlay():Promise.resolve();}
}
function harness(next=true){
 const timers=new Map(),errors=[],handlers=new Map();let id=0;
 const button={textContent:'Play',attributes:{'aria-label':'Play recordings'},setAttribute(k,v){this.attributes[k]=v;}};
 const video=new Video(),container={innerHTML:'',style:{}};
 const document={readyState:'loading',addEventListener(name,fn){handlers.set(name,fn);},getElementById(name){return name==='play-pause'?button:name==='video-grid'?container:null;}};
 const c=vm.createContext({console,document,testVideo:video,showError:m=>errors.push(m),setTimeout(fn,delay){const key=++id;timers.set(key,{fn,delay});return key;},clearTimeout(key){timers.delete(key);}});
 vm.runInContext(source,c);const run=code=>vm.runInContext(code,c);
 run(`currentDate=new Date(2026,10,10);allCameras=[{id:'a',name:'A'}];selectedCameras=['a'];videoPlayers=[testVideo];`);
 const start=new Date(2026,10,10,12).getTime();
 const segment=(a,b,path)=>({camera_id:'a',start_time:new Date(start+a*1000).toISOString(),end_time:new Date(start+b*1000).toISOString(),file_path:path});
 c.segments=[segment(0,60,'/one.mp4'),...(next?[segment(60,120,'/two.mp4')]:[])];run('timelineSegments=segments');
 function flush(delay){for(const [key,timer]of [...timers])if(timer.delay===delay){timers.delete(key);timer.fn();}}
 function seek(seconds=5){c.seekToTime(new Date(start+seconds*1000));flush(100);flush(0);flush(10);}
 function state(playing){assert.equal(run('isPlaying'),playing);assert.equal(button.textContent,playing?'Pause':'Play');assert.equal(button.attributes['aria-label'],playing?'Pause recordings':'Play recordings');}
 function key(code,tag='BODY',extra={}){const e={code,target:{tagName:tag},prevented:false,preventDefault(){this.prevented=true;},...extra};handlers.get('keydown')(e);return e;}
 return {c,run,button,video,timers,errors,flush,seek,state,key};
}
async function settle(){await new Promise(resolve=>setImmediate(resolve));}
(async()=>{
 {const h=harness();h.c.setPlaybackButton();h.state(false);h.c.togglePlayPause();h.state(false);assert.equal(h.video.plays,0);assert.equal(h.errors.length,1);}
 {const h=harness();h.seek();h.state(true);assert.equal(h.video.plays,0);h.c.togglePlayPause();h.state(false);h.c.togglePlayPause();h.state(true);assert.equal(h.video.plays,0);h.video.emit('loadedmetadata');await settle();assert.equal(h.video.plays,1);h.c.togglePlayPause();h.state(false);h.c.togglePlayPause();await settle();h.state(true);assert.equal(h.video.plays,2);}
 {const h=harness();h.video.nextPlay=()=>Promise.reject(new Error('denied'));h.seek();h.video.emit('loadedmetadata');await settle();h.state(false);assert.equal(h.video.plays,1);h.video.nextPlay=null;h.c.togglePlayPause();await settle();h.state(true);assert.equal(h.video.plays,2);}
 {const h=harness();let reject;h.video.nextPlay=()=>new Promise((_,r)=>{reject=r;});h.seek();h.video.emit('loadedmetadata');h.c.togglePlayPause();h.video.nextPlay=null;h.c.togglePlayPause();reject(new Error('stale'));await settle();h.state(true);assert.equal(h.errors.length,0);}
 {const h=harness();h.seek();h.video.emit('loadedmetadata');h.video.emit('error');h.state(false);const plays=h.video.plays;h.c.togglePlayPause();h.state(false);assert.equal(h.video.plays,plays);}
 {const h=harness(false);h.seek();h.video.emit('loadedmetadata');h.video.currentTime=60;h.video.ended=true;h.video.emit('ended');h.state(false);assert.equal(h.run('playbackSessions.size'),0);}
 {const h=harness();h.seek();h.video.emit('loadedmetadata');h.c.togglePlayPause();h.video.currentTime=60;h.video.ended=true;h.video.emit('ended');h.state(false);h.c.togglePlayPause();h.flush(0);h.flush(10);h.video.emit('loadedmetadata');h.state(true);assert.deepEqual(h.video.assignments,['/one.mp4','/two.mp4']);}
 {const h=harness();h.seek();h.video.emit('loadedmetadata');h.c.invalidateTimelineLoad();h.state(false);assert.equal(h.run('playbackSessions.size'),0);assert.ok(h.video.pauses>0);}
 {const h=harness();h.seek();h.video.emit('loadedmetadata');h.run('selectedCameras=[]');h.c.updateVideoGrid();h.state(false);assert.equal(h.run('playbackSessions.size'),0);}
 {const h=harness(false);h.seek(100);h.state(false);assert.equal(h.video.plays,0);}
 {const h=harness();h.run('selectedCameras=[]');h.seek();h.state(false);assert.equal(h.video.plays,0);}
 {const h=harness();h.seek();h.video.emit('loadedmetadata');h.c.setupKeyboardShortcuts();assert.equal(h.key('Space').prevented,true);h.state(false);assert.equal(h.key('Space').prevented,true);h.state(true);
  for(const tag of ['BUTTON','A','INPUT','TEXTAREA','SELECT'])for(const code of ['Space','Enter','ArrowLeft','ArrowRight']){const before=h.run('playbackIntentVersion');assert.equal(h.key(code,tag).prevented,false);assert.equal(h.run('playbackIntentVersion'),before);h.state(true);}
  for(const extra of [{repeat:true},{ctrlKey:true},{metaKey:true},{altKey:true},{defaultPrevented:true},{target:{tagName:'SPAN',closest:()=>({})}},{target:{tagName:'DIV',isContentEditable:true}}]){const before=h.run('playbackIntentVersion');assert.equal(h.key('Space','BODY',extra).prevented,false);assert.equal(h.run('playbackIntentVersion'),before);}
 }
 console.log('Playback control, labeling and keyboard regressions passed');
})().catch(error=>{console.error(error);process.exitCode=1;});
