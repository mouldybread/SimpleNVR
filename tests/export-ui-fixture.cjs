const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const root=process.argv[2] || '.';
const source=fs.readFileSync(path.join(root,'static/js/export-ui.js'),'utf8');
function environment(fetcher,ids=['front']){
 const timers=new Map(),requests=[],downloads=[];let serial=0;
 class Element{
  constructor(tag){this.tagName=tag;this.children=[];this.style={};this.dataset={};this.listeners={};this.disabled=false;this.text='';this.classList={add(){}};}
  set textContent(v){this.text=String(v);this.children=[];}get textContent(){return this.text+this.children.map(c=>c.textContent).join('');}
  set innerHTML(_){throw Error('HTML sink');}set outerHTML(_){throw Error('HTML sink');}insertAdjacentHTML(){throw Error('HTML sink');}
  appendChild(c){c.parentElement=this;this.children.push(c);return c;}replaceChildren(...children){this.children=[];this.text='';children.forEach(c=>this.appendChild(c));}
  setAttribute(k,v){if(/^on/i.test(k))throw Error('inline handler');this[k]=v;}addEventListener(k,fn){this.listeners[k]=fn;}
  remove(){if(this.parentElement)this.parentElement.children=this.parentElement.children.filter(c=>c!==this);}
  click(){if(this.disabled)return;if(this.tagName==='a')downloads.push(this.href);this.listeners.click?.();}focus(){}
 }
 const body=new Element('body');const make=(id,parent=body)=>{const e=new Element('div');e.id=id;parent.appendChild(e);return e;};
 const toolbar=make('toolbar'),wrap=make('wrap',toolbar);make('download-time-display',wrap);make('download-duration-display',wrap);
 for(const id of ['download-modal','download-status','download-progress-list','export-start-btn'])make(id);
 function find(id,node=body){if(node.id===id)return node;for(const child of node.children){const result=find(id,child);if(result)return result;}return null;}
 const context=vm.createContext({console,AbortController,TextDecoder,Uint8Array,document:{readyState:'loading',addEventListener(){},body,createElement:t=>new Element(t),getElementById:find},setTimeout(fn,ms){const id=++serial;timers.set(id,{fn,ms});return id;},clearTimeout(id){timers.delete(id);},fetch(url,options){requests.push({url,options});return fetcher(url,options);}});
 vm.runInContext(`let downloadStartTime=new Date('2026-11-21T00:00:00Z'),downloadEndTime=new Date('2026-11-21T00:01:00Z'),downloadCameras=${JSON.stringify(ids)},currentTimePosition=Date.parse('2026-11-21T23:59:59Z'),selectedCameras=${JSON.stringify(ids)},allCameras=[{id:'front',name:'<img src=x onerror=pwned=1>'},{id:'back',name:'Back'}];function renderDownloadCameraSelection(){}`,context);
 vm.runInContext(source,context);
 const run=code=>vm.runInContext(code,context);
 const flush=async()=>{for(let i=0;i<8;i++)await new Promise(resolve=>setImmediate(resolve));};
 const fire=async ms=>{const pair=[...timers].find(([,t])=>t.ms===ms);if(!pair)throw Error('Missing timer '+ms);timers.delete(pair[0]);pair[1].fn();await flush();};
 return {context,run,flush,fire,find,timers,requests,downloads,jobs:()=>run('exportUiBatch.jobs')};
}
const id=letter=>letter.repeat(64);
const view=(letter,state='ready',extra={})=>({id:id(letter),state,message:state,bytes:state==='ready'?5*1024*1024*1024:0,expires_at:new Date(Date.now()+3600000).toISOString(),...(state==='ready'?{download_url:'/api/exports/download?id='+id(letter)}:{}),...extra});
function response(status,data,headers={}){return new Response(status===204?null:(typeof data==='string'?data:JSON.stringify(data)),{status,headers:{'Content-Type':'application/json',...headers}});}
module.exports={environment,source,id,view,response};
