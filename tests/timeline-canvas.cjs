'use strict';
const assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
class Element {
 constructor(tag){this.tagName=tag.toUpperCase();this.children=[];this.style={};this.parent=null;this.text='';}
 set textContent(value){this.text=String(value);this.children=[];}
 get textContent(){return this.text+this.children.map(child=>child.textContent).join('');}
 set innerHTML(_){throw new Error('Unexpected HTML sink in tooltip');}
 appendChild(child){child.parent=this;this.children.push(child);return child;}
 remove(){if(this.parent){this.parent.children=this.parent.children.filter(child=>child!==this);this.parent=null;}}
}
const body=new Element('body');
const listeners={},rectangles=[],labels=[],lines=[],timers=new Map(),seeks=[];let timerID=0,path=[];
const ctx={fillRect(...a){rectangles.push(a);},fillText(s){labels.push(s);},measureText(s){return {width:s.length*5};},beginPath(){path=[];},moveTo(...a){path.push(a);},lineTo(...a){path.push(a);},stroke(){lines.push({color:this.strokeStyle,path:[...path]});},arc(){},fill(){}};
const rect={left:10,top:20,width:500,height:170};
const canvas={offsetWidth:1000,width:1000,height:85,style:{},getContext(){return ctx;},getBoundingClientRect(){return rect;},addEventListener(k,fn){listeners[k]=fn;}};
const context=vm.createContext({console,document:{readyState:'loading',body,createElement:tag=>new Element(tag),addEventListener(){},getElementById:id=>id==='timeline-canvas'?canvas:null},window:{},setTimeout(fn,delay){const id=++timerID;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);}});
for(const file of ['static/js/events.js','static/js/timeline-geometry.js','static/js/playback.js']) vm.runInContext(fs.readFileSync(file,'utf8'),context,{filename:file});
const run=code=>vm.runInContext(code,context);context.performSeek=time=>seeks.push(time.getTime());
function flush(){for(const [id,timer] of [...timers])if(timer.delay===100){timers.delete(id);timer.fn();}}
function click(f,y=80){listeners.click({clientX:rect.left+f*rect.width,clientY:rect.top+y*rect.height/canvas.height});flush();}
function hover(event){listeners.mousemove({clientX:rect.left+event._canvasX*rect.width/canvas.width,clientY:rect.top+event._canvasY*rect.height/canvas.height});}
function approx(a,b){assert.ok(Math.abs(a-b)<1e-7,`${a} != ${b}`);}
context.setupTimelineControls();context.setupTimelineZoom();
for(const [month,day] of [[2,8],[10,1]]){
 run(`currentDate=new Date(2026,${month},${day});timelineZoomLevel=1;timelineOffset=0;timelineEvents=[];`);
 const v=context.timelineViewport(),start=v.dayStart,end=v.dayEnd;
 run(`timelineSegments=[{camera_id:'a',start_time:'${new Date(start-3600000).toISOString()}',end_time:'${new Date(start+3600000).toISOString()}'}];`);
 rectangles.length=0;context.renderTimeline();assert.equal(rectangles.at(-1)[0],0);approx(rectangles.at(-1)[2],1000*3600000/(end-start));
 click(0.5);assert.equal(seeks.at(-1),start+(end-start)/2);
 run('currentTimePosition='+seeks.at(-1));lines.length=0;context.renderTimeline();approx(lines.find(line=>line.color==='#fff').path[0][0],500);
 const anchor=run('TimelineGeometry.timeAt(0.5,timelineViewport())');
 listeners.wheel({clientX:260,clientY:180,deltaY:-1,preventDefault(){}});assert.equal(run('TimelineGeometry.timeAt(0.5,timelineViewport())'),anchor);
 click(0.5);assert.equal(seeks.at(-1),anchor);
 run('timelineZoomLevel=1;timelineOffset=0;');click(1);assert.equal(seeks.at(-1),end-1);
 run(`timelineSegments=[{camera_id:'a',start_time:'${new Date(end-3600000).toISOString()}',end_time:'${new Date(end+3600000).toISOString()}'}];`);
 rectangles.length=0;context.renderTimeline();approx(rectangles.at(-1)[0]+rectangles.at(-1)[2],1000);
 run(`timelineSegments=[{camera_id:'a',start_time:'invalid',end_time:'invalid'}];timelineEvents=[{start_time:'invalid',_canvasX:1,_canvasY:2},{start_time:'${new Date(end).toISOString()}',_canvasX:3,_canvasY:4}];`);
 rectangles.length=0;context.renderTimeline();assert.equal(rectangles.length,1);assert.equal(run('timelineEvents.some(event=>Number.isFinite(event._canvasX))'),false);
}
if(process.env.TZ==='America/Chicago'){
 run(`currentDate=new Date(2026,10,1);timelineZoomLevel=4;timelineOffset=0;timelineSegments=[];timelineEvents=['2026-11-01T01:30:00-05:00','2026-11-01T01:30:00-06:00'].map(start_time=>({start_time,camera_name:'a',label:'Motion Detected'}));`);
 labels.length=0;context.renderTimeline();assert.ok(labels.includes('01:00 UTC-05:00'));assert.ok(labels.includes('01:00 UTC-06:00'));
 assert.ok(run('timelineEvents[1]._canvasX>timelineEvents[0]._canvasX'));
 const navigations=[];context.navigateToEvent=(name,time)=>navigations.push([name,time]);
 for(let i=0;i<2;i++){const event=run(`timelineEvents[${i}]`);click(event._canvasX/canvas.width,event._canvasY);assert.deepEqual(navigations.at(-1),['a',event.start_time]);hover(event);assert.equal(canvas.style.cursor,'pointer');assert.equal(body.children.length,1);assert.equal(body.children[0].id,'event-tooltip');assert.ok(body.textContent.includes('Motion Detected'));}
 listeners.mouseleave();assert.equal(body.children.length,0);
 run('timelineOffset=10;');context.renderTimeline();assert.equal(run('timelineEvents.some(event=>Number.isFinite(event._canvasX))'),false);
}
run(`currentDate=new Date(2026,10,10);timelineZoomLevel=1;timelineOffset=0;timelineSegments=[];timelineEvents=[{start_time:new Date(2026,10,10,12).toISOString(),camera_name:'Front <img src=x>',label:'Motion Detected',duration_seconds:10}];`);
context.renderTimeline();const event=run('timelineEvents[0]');
hover(event);assert.equal(canvas.style.cursor,'pointer');assert.equal(body.children.length,1);assert.ok(body.textContent.includes('Front <img src=x>'));
hover(event);assert.equal(body.children.length,1,'repeat hover replaces the previous tooltip');
listeners.mousemove({clientX:rect.left,clientY:rect.top+rect.height});assert.equal(canvas.style.cursor,'default');assert.equal(body.children.length,0);
hover(event);assert.equal(body.children.length,1);listeners.mouseleave();assert.equal(body.children.length,0);assert.equal(run('eventTooltip'),null);
rect.width=0;const count=seeks.length;click(0.5);assert.equal(seeks.length,count);
const page=fs.readFileSync('templates/index.html','utf8'),scripts=[...page.matchAll(/<script src="([^"]+)"><\/script>/g)].map(match=>match[1]);
assert.equal(scripts.filter(src=>src==='/static/js/timeline-geometry.js').length,1);assert.ok(scripts.indexOf('/static/js/timeline-geometry.js')<scripts.indexOf('/static/js/playback.js'));
console.log('Real-script timeline canvas regressions passed in '+process.env.TZ);
