// Server-prepared export UI; only bounded control JSON is fetched.
let exportUiBatch = null;
let exportInitialRange = null;
const EXPORT_RANGE_STEPS = [1000,10000,60000,600000];

function exportUiNode(tag,text='') {const node=document.createElement(tag);node.textContent=String(text);return node;}
function exportRangeCandidate(start,end,boundary,delta) {
  if(!Number.isFinite(start) || !Number.isFinite(end) || end<=start || !['start','end'].includes(boundary) || !Number.isFinite(delta))return null;
  const value=(boundary==='start'?start:end)+delta;
  if(!Number.isFinite(new Date(value).getTime()) || (boundary==='start'?value>=end:value<=start))return null;
  return value;
}
function exportRangeStep() {const value=Number(document.getElementById('download-range-step')?.value);return EXPORT_RANGE_STEPS.includes(value)?value:60000;}
function adjustDownloadBoundary(boundary,direction) {
  if(exportUiBatch?.running || ![-1,1].includes(direction))return false;
  const value=exportRangeCandidate(downloadStartTime?.getTime(),downloadEndTime?.getTime(),boundary,direction*exportRangeStep());
  if(value===null)return false;
  if(boundary==='start')downloadStartTime=new Date(value);else downloadEndTime=new Date(value);
  updateDownloadTimeDisplay();return true;
}
function installExportRangeControls() {
  const button=document.getElementById('export-start-btn');if(button)button.textContent='Prepare exports on server';
  if(document.getElementById('download-range-step'))return;
  const old=document.getElementById('download-time-display');
  const toolbar=old?.parentElement?.parentElement;if(!toolbar)return;
  const group=exportUiNode('div');group.style.cssText='display:flex;flex-direction:column;gap:12px;width:100%';
  for(const boundary of ['start','end']) {
    const row=exportUiNode('div');row.style.cssText='display:flex;align-items:center;gap:10px;flex-wrap:wrap';
    row.appendChild(exportUiNode('span',boundary==='start'?'Start':'End'));
    for(const direction of [-1,1]) {
      const button=exportUiNode('button',direction<0?'Earlier':'Later');button.type='button';button.className='btn';
      button.id=`download-${boundary}-${direction<0?'earlier':'later'}`;
      button.setAttribute('aria-label',`Move ${boundary} ${direction<0?'earlier':'later'}`);
      button.addEventListener('click',()=>adjustDownloadBoundary(boundary,direction));row.appendChild(button);
      if(direction===-1){const time=exportUiNode('output');time.id=`download-${boundary}-display`;row.appendChild(time);}
    }
    group.appendChild(row);
  }
  const label=exportUiNode('label','Adjustment step: '),step=exportUiNode('select');step.id='download-range-step';
  for(const [index,text] of ['1 second','10 seconds','1 minute','10 minutes'].entries()){
    const option=exportUiNode('option',text);option.value=String(EXPORT_RANGE_STEPS[index]);step.appendChild(option);
  }
  step.value='60000';step.addEventListener('change',updateDownloadTimeDisplay);label.appendChild(step);group.appendChild(label);
  const reset=exportUiNode('button','Reset to initial selection');reset.type='button';reset.className='btn';reset.id='download-range-reset';
  reset.addEventListener('click',resetDownloadTimeRange);group.appendChild(reset);
  for(const id of ['download-time-display','download-duration-display']){const node=exportUiNode('div');node.id=id;group.appendChild(node);}
  group.appendChild(exportUiNode('p','Times are local with time-zone labels. Exports require complete coverage. Existing media compatibility restrictions still apply. Prepare on the server, then click Download prepared video.'));
  toolbar.replaceChildren(group);
}
function exportBoundaryLabel(time) {
  return time.toLocaleString(undefined,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',timeZoneName:'short',hour12:false});
}
// Loaded after playback.js; these entry points own the export dialog.
function updateDownloadTimeDisplay() {
  const start=downloadStartTime?.getTime(),end=downloadEndTime?.getTime(),valid=Number.isFinite(start)&&Number.isFinite(end)&&end>start;
  const busy=!!exportUiBatch?.running;
  for(const boundary of ['start','end']) {
    const display=document.getElementById(`download-${boundary}-display`);
    if(display)display.textContent=valid?exportBoundaryLabel(boundary==='start'?downloadStartTime:downloadEndTime):'Select a valid range';
    for(const direction of [-1,1]) {
      const button=document.getElementById(`download-${boundary}-${direction<0?'earlier':'later'}`);
      if(button)button.disabled=busy || exportRangeCandidate(start,end,boundary,direction*exportRangeStep())===null;
    }
  }
  const summary=document.getElementById('download-time-display');if(summary)summary.textContent=valid?'Selected interval; end is exclusive.':'';
  const duration=document.getElementById('download-duration-display');
  if(duration)duration.textContent=valid?`Duration: ${(end-start)/1000} seconds (${((end-start)/60000).toFixed(2)} minutes)`:'Invalid or empty interval';
  const step=document.getElementById('download-range-step');if(step)step.disabled=busy;
  const reset=document.getElementById('download-range-reset');if(reset)reset.disabled=busy || !exportInitialRange;
}
function openDownloadModal() {
  const modal=document.getElementById('download-modal');if(!modal)return;
  installExportRangeControls();
  if(!exportUiBatch?.running){
    if(!Number.isFinite(currentTimePosition)){if(typeof showError==='function')showError('Select a time on the timeline first');return;}
    const start=currentTimePosition-600000,end=currentTimePosition+600000;
    if(!Number.isFinite(new Date(start).getTime()) || !Number.isFinite(new Date(end).getTime()))return;
    exportInitialRange={start,end};downloadStartTime=new Date(start);downloadEndTime=new Date(end);downloadCameras=[...selectedCameras];
    renderDownloadCameraSelection();
  }
  updateDownloadTimeDisplay();modal.classList.add('active');modal.style.display='block';
}
function resetDownloadTimeRange() {
  if(exportUiBatch?.running || !exportInitialRange)return;
  downloadStartTime=new Date(exportInitialRange.start);downloadEndTime=new Date(exportInitialRange.end);updateDownloadTimeDisplay();
}

const EXPORT_JOB_POLL_MS=1500;
const EXPORT_JOB_REQUEST_MS=45000;
const EXPORT_JOB_WAIT_MS=2*60*60*1000;
function exportUiState(job,state,message){job.state=state;job.row.dataset.state=state;job.status.textContent=String(message).slice(0,1200);}
function exportUiText(text){return String(text ?? '').replace(/[\u0000-\u001f\u007f]/g,' ').slice(0,600);}
function exportUiError(message){return new Error(message);}
async function exportUiRead(response,signal,limit=8192){
  if(!response.body || typeof response.body.getReader!=='function')throw exportUiError('Streaming JSON responses are unavailable.');
  const reader=response.body.getReader(),chunks=[];let size=0,done=false;
  const canceled=()=>{try{Promise.resolve(reader.cancel()).catch(()=>{});}catch(_){}};
  signal.addEventListener('abort',canceled,{once:true});
  try{
    while(true){
      if(signal.aborted)throw exportUiError('Export control request timed out.');
      const part=await reader.read();if(part.done){done=true;break;}
      if(part.value.byteLength>limit-size)throw exportUiError('Export control response exceeded its size limit.');
      size+=part.value.byteLength;chunks.push(part.value);
    }
    if(signal.aborted)throw exportUiError('Export control request timed out.');
    const bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
    return new TextDecoder().decode(bytes);
  }finally{signal.removeEventListener('abort',canceled);if(!done)canceled();try{reader.releaseLock();}catch(_){}}
}
function exportUiHTTPMessage(response){
  if(response.status===401 || response.status===403)return 'Authentication expired or access denied; sign in again.';
  if(response.status===503)return 'Prepared exports unavailable: check NVR_EXPORT_SCRATCH_DIR, recording metadata, and whether the server is restarting.';
  if(response.status===429)return 'Server export capacity is full. Release earlier prepared files or wait for expiry, then retry.';
  if(response.status===404)return 'Export or camera unavailable; the job may have expired or the server restarted.';
  const plan=response.headers.get('X-Export-Plan-Status');
  if(plan==='gap')return 'The full interval is not covered by recordings; adjust Start or End. No gaps were silently skipped.';
  if(plan==='overlap')return 'Overlapping recordings require an explicit clip choice.';
  if(response.status===413)return 'Server export limit exceeded; reduce the selected interval.';
  if(response.status===422)return 'The recording profile is not supported by the current trim renderer.';
  return `Export control request failed (HTTP ${response.status}).`;
}
// Only bounded JSON/control bodies use fetch. Video uses a normal download URL.
async function exportUiRequest(path,body){
  const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),EXPORT_JOB_REQUEST_MS);
  try{
    const options={credentials:'same-origin',cache:'no-store',redirect:'error',signal:controller.signal};
    if(body!==undefined){options.method='POST';options.headers={'Content-Type':'application/json'};options.body=JSON.stringify(body);}
    const response=await fetch(path,options);
    if(response.redirected)throw exportUiError('Unexpected redirect; sign in again.');
    if(response.status===204)return {status:204};
    const text=await exportUiRead(response,controller.signal);
    if(response.status!==200 && response.status!==202)throw exportUiError(exportUiHTTPMessage(response)+(text?' Server: '+exportUiText(text):''));
    if((response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase()!=='application/json')throw exportUiError('Expected export JSON, not a video or login page.');
    let value;try{value=JSON.parse(text);}catch(_){throw exportUiError('Invalid export JSON response.');}
    return {status:response.status,value};
  }finally{clearTimeout(timer);}
}
function exportUiView(value,expected){
  if(!value || typeof value!=='object' || Array.isArray(value) || !/^[0-9a-f]{64}$/.test(value.id || '') || (expected && value.id!==expected) || !['queued','preparing','ready','failed','canceled','expired'].includes(value.state) || typeof value.message!=='string' || !Number.isSafeInteger(value.bytes) || value.bytes<0 || !Number.isFinite(Date.parse(value.expires_at)))throw exportUiError('Invalid export status response.');
  if(value.state==='ready' && (value.bytes===0 || value.download_url!==`/api/exports/download?id=${value.id}`))throw exportUiError('Invalid prepared download response.');
  return value;
}
function exportUiRemoveDownload(job){if(job.save){job.save.disabled=true;job.save.remove();job.save=null;}job.url=null;clearTimeout(job.expiryTimer);}
function exportUiFinish(job,state,text){exportUiRemoveDownload(job);job.finished=true;exportUiState(job,state,text);if(job.finishReady){const finish=job.finishReady;job.finishReady=null;finish();}}
function exportUiWait(job){return new Promise(resolve=>{job.wake=()=>{clearTimeout(job.pollTimer);job.wake=null;resolve();};job.pollTimer=setTimeout(job.wake,EXPORT_JOB_POLL_MS);});}
async function exportUiCancel(job){
  if(job.canceling || ['canceled','expired','released'].includes(job.state))return;
  job.reason='canceled';job.wake?.();
  if(!job.serverId){
    if(job.creating){exportUiState(job,'canceling','Cancellation requested; waiting for the creation response to identify the server job.');return;}
    exportUiFinish(job,'canceled','Canceled before a server job was created.');job.cancel.disabled=true;return;
  }
  job.canceling=true;job.cancel.disabled=true;exportUiRemoveDownload(job);
  exportUiState(job,'canceling','Requesting cancellation/release on the server. Existing browser transfers may continue.');
  try{
    const response=await exportUiRequest('/api/exports/cancel',{id:job.serverId});
    if(response.status!==204)throw exportUiError('Unexpected cancellation response.');
    exportUiFinish(job,job.handed?'released':'canceled',job.handed?'Server confirmed release. Any already-started download may continue; disk saving is not verified.':'Server confirmed export cancellation/release.');job.cancel.disabled=true;
  }catch(error){
    exportUiFinish(job,'release-failed',`Cancellation could not be confirmed. ${error.message} The job may remain until server expiry.`);job.cancel.textContent='Retry cancellation/release';job.cancel.disabled=false;
  }finally{job.canceling=false;}
}
function cancelExportDownloads(){return exportUiBatch?Promise.all(exportUiBatch.jobs.map(job=>exportUiCancel(job))):Promise.resolve();}
function exportUiOffer(job,view){
  return new Promise(resolve=>{
    job.finishReady=resolve;job.url=`/api/exports/download?id=${job.serverId}`;
    const expires=Date.parse(view.expires_at),remaining=expires-Date.now();
    if(remaining<=0){exportUiFinish(job,'expired','Prepared export expired; prepare it again.');return;}
    job.save=exportUiNode('button','Download prepared video');job.save.type='button';job.save.className='btn btn-primary';
    job.save.addEventListener('click',()=>{
      if(job.finished || job.canceling || job.handed)return;
      if(Date.now()>=expires){exportUiFinish(job,'expired','Prepared export expired; prepare it again.');job.cancel.disabled=true;return;}
      const anchor=exportUiNode('a');anchor.href=job.url;anchor.download=`export-${job.serverId}.mp4`;
      try{document.body.appendChild(anchor);anchor.click();}catch(_){exportUiState(job,'ready','Browser download request failed. Try again before expiry.');return;}finally{anchor.remove();}
      job.handed=true;job.save.disabled=true;
      exportUiState(job,'handed','Browser download requested; check your browser downloads. Disk saving is not verified. Release after the browser accepts it; existing transfers retain their server file lease.');
    });
    job.row.appendChild(job.save);job.cancel.textContent='Release prepared file / continue';job.cancel.disabled=false;
    exportUiState(job,'ready',`Ready on server: ${(view.bytes/(1024*1024)).toFixed(1)} MiB. Click Download prepared video before ${new Date(expires).toLocaleString()}. Nothing has been saved by this page; disk saving is not verified.`);
    job.expiryTimer=setTimeout(()=>{exportUiFinish(job,'expired','Prepared export reached its expiry time; request a new export if needed. An already-started browser transfer may continue.');job.cancel.disabled=true;},Math.min(remaining,2147483647));
    job.save.focus?.();
  });
}
async function exportUiRunJob(job){
  if(job.reason==='canceled'){job.cancel.disabled=true;return;}
  try{
    job.creating=true;exportUiState(job,'preparing','Requesting server preparation. Video will not be buffered in this page.');
    let result;
    try{result=await exportUiRequest('/api/exports/create',{camera_id:job.id,start_time:job.start,end_time:job.end});}finally{job.creating=false;}
    if(result.status!==202)throw exportUiError('Expected server job acceptance.');
    let view=exportUiView(result.value);job.serverId=view.id;
    if(job.reason==='canceled'){await exportUiCancel(job);return;}
    const deadline=Date.now()+EXPORT_JOB_WAIT_MS;
    while(true){
      if(job.reason==='canceled')return;
      if(view.state==='ready'){await exportUiOffer(job,view);return;}
      if(['failed','canceled','expired'].includes(view.state)){exportUiFinish(job,view.state,exportUiText(view.message));job.cancel.disabled=true;return;}
      exportUiState(job,view.state,view.state==='queued'?'Queued on server.':'Preparing on server; large exports can take time.');
      if(Date.now()>=deadline)throw exportUiError('Stopped waiting after two hours. The job may continue until its server deadline or expiry.');
      await exportUiWait(job);if(job.reason==='canceled')return;
      result=await exportUiRequest(`/api/exports/status?id=${job.serverId}`);
      if(job.reason==='canceled')return;
      if(result.status!==200)throw exportUiError('Unexpected export status response.');
      view=exportUiView(result.value,job.serverId);
    }
  }catch(error){
    if(job.reason==='canceled' && job.serverId){await exportUiCancel(job);return;}
    exportUiFinish(job,'failed',`${error.message} ${job.serverId?'The server job may still exist; use cancellation below or wait for expiry.':'Creation outcome may be unknown if the response was lost; no automatic retry was made.'}`);
    job.cancel.disabled=!job.serverId;if(job.serverId)job.cancel.textContent='Cancel server job';
  }
}
function startExportDownloads(){
  if(exportUiBatch?.running)return exportUiBatch.promise;
  const host=document.getElementById('download-status'),list=document.getElementById('download-progress-list');
  if(!host || !list)return Promise.resolve();
  // Keep unresolved release failures visible rather than orphaning their controls.
  if(exportUiBatch?.jobs.some(job=>job.state==='release-failed' || (job.serverId && job.state==='failed' && !job.cancel.disabled)))return exportUiBatch.promise;
  host.style.display='block';list.replaceChildren();
  const start=downloadStartTime?.getTime(),end=downloadEndTime?.getTime(),ids=[...new Set(downloadCameras)];
  if(!Number.isFinite(start) || !Number.isFinite(end) || end<=start || ids.length<1 || ids.length>8){list.appendChild(exportUiNode('p','Choose a valid range and one to eight cameras. No request started.'));return Promise.resolve();}
  const cameras=ids.map(id=>allCameras.find(cam=>cam.id===id));
  if(cameras.some(cam=>!cam || typeof cam.id!=='string' || cam.id.length>256)){list.appendChild(exportUiNode('p','Selected camera unavailable; reload the camera list.'));return Promise.resolve();}
  const stop=exportUiNode('button','Cancel all exports');stop.type='button';stop.addEventListener('click',()=>{void cancelExportDownloads();});list.appendChild(stop);
  list.appendChild(exportUiNode('p','Prepare on the server, then download with your browser. Server storage limits apply; this page does not buffer video or impose a 64 MiB file limit. The next camera starts after release or expiry of the current prepared export. Existing rendering compatibility restrictions still apply.'));
  const jobs=cameras.map(cam=>{
    const row=exportUiNode('div');row.className='export-job';row.dataset.cameraId=cam.id;
    const status=exportUiNode('p','Queued'),cancel=exportUiNode('button','Cancel');cancel.type='button';status.setAttribute('aria-live','polite');
    row.appendChild(exportUiNode('h4',String(cam.name ?? cam.id).slice(0,256)));row.appendChild(status);row.appendChild(cancel);list.appendChild(row);
    const job={id:cam.id,start:new Date(start).toISOString(),end:new Date(end).toISOString(),row,status,cancel,state:'queued',reason:null,serverId:null,creating:false,canceling:false,finished:false,handed:false,url:null,save:null,finishReady:null,wake:null};
    cancel.addEventListener('click',()=>{void exportUiCancel(job);});return job;
  });
  const batch={jobs,running:true,promise:null};exportUiBatch=batch;updateDownloadTimeDisplay();
  const button=document.getElementById('export-start-btn');if(button)button.disabled=true;
  batch.promise=(async()=>{try{for(const job of jobs)await exportUiRunJob(job);}finally{batch.running=false;if(button)button.disabled=false;updateDownloadTimeDisplay();}})();return batch.promise;
}
// Do not auto-release on navigation: downloading can itself trigger pagehide,
// and aborting creation can lose the server ID. Server TTL bounds abandoned jobs.
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',installExportRangeControls);else installExportRangeControls();
