// Export dialog controls and bounded, monitored browser downloads.
const EXPORT_UI_LIMIT = 64 * 1024 * 1024;
const EXPORT_UI_TIMEOUT = 4 * 60 * 1000;
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
// Replace the legacy expansion-only toolbar without changing playback controls.
function installExportRangeControls() {
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
  group.appendChild(exportUiNode('p','Times are local with time-zone labels. Exports require complete, unambiguous coverage. Trimming currently requires supported video-only inputs. After receiving, click Download prepared video.'));
  toolbar.replaceChildren(group);
}
function exportBoundaryLabel(time) {
  return time.toLocaleString(undefined,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',timeZoneName:'short',hour12:false});
}
// These dialog entry points supersede the legacy implementations in playback.js.
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
function exportUiError(kind,message) {const error=new Error(message);error.kind=kind;return error;}
function exportUiAbortError() {const error=new Error('Export canceled');error.name='AbortError';return error;}
function exportUiAwait(promise,signal) {
  return new Promise((resolve,reject)=>{
    let settled=false;
    const finish=(fn,value)=>{if(settled)return;settled=true;signal.removeEventListener('abort',abort);fn(value);};
    const abort=()=>finish(reject,exportUiAbortError());
    Promise.resolve(promise).then(value=>finish(resolve,value),error=>finish(reject,error));
    if(signal.aborted){abort();return;}signal.addEventListener('abort',abort,{once:true});
  });
}
function exportUiState(job,state,text) {job.state=state;job.row.dataset.state=state;job.status.textContent=text;}
function exportUiCancel(job) {
  if(job.finished)return;
  if(job.finishReady){job.finishReady(job.handed?'handed':'canceled',job.handed?'Browser download requested; prepared file released. Disk saving is not verified.':'Canceled; prepared file discarded.');return;}
  job.reason='canceled';job.controller.abort();exportUiState(job,'canceled','Canceled; no browser download requested.');
}
function cancelExportDownloads() {if(exportUiBatch)exportUiBatch.jobs.forEach(exportUiCancel);}
function exportUiCancelReader(reader) {try{Promise.resolve(reader.cancel()).catch(()=>{});}catch(_){} }
async function exportUiErrorText(response,signal) {
  if(!response.body || typeof response.body.getReader!=='function')return '';
  const reader=response.body.getReader(),chunks=[];let size=0,done=false;
  try{
    while(size<2048){const part=await exportUiAwait(reader.read(),signal);if(part.done){done=true;break;}const chunk=part.value.slice(0,2048-size);chunks.push(chunk);size+=chunk.byteLength;}
    const raw=new Uint8Array(size);let offset=0;for(const chunk of chunks){raw.set(chunk,offset);offset+=chunk.byteLength;}
    return new TextDecoder().decode(raw).replace(/[\u0000-\u001f\u007f]/g,' ').slice(0,600);
  }finally{if(!done)exportUiCancelReader(reader);try{reader.releaseLock();}catch(_){} }
}
function exportUiFailureMessage(response) {
  const plan=response.headers.get('X-Export-Plan-Status'),trim=response.headers.get('X-Export-Trim-Status');
  if(response.status===401 || response.status===403)return 'Authentication expired or access denied; sign in again.';
  if(plan==='gap')return 'The selected interval includes missing recording coverage. Some footage may exist, but the full interval is not covered. Move Start later or End earlier as appropriate; no gaps were silently skipped.';
  if(plan==='overlap')return 'The selected interval contains overlapping recordings. No automatic clip choice was made.';
  if(trim==='unsupported' || response.status===422)return 'Footage may be playable but this subsection cannot be trimmed with the current input profile. Audio-containing or otherwise incompatible inputs are unsupported; audio was not silently removed and the interval was not widened.';
  if(response.status===404)return 'Camera or recording coverage was not found for the selected interval.';
  if(response.status===413)return 'Server export limit exceeded; shorten the interval or select fewer clips.';
  if(response.status===504)return 'Server preparation or rendering timed out; no prepared download was received.';
  if(response.status===503)return 'Recording metadata unavailable, changed, or server shutting down; retry after reviewing recording availability.';
  return `Export request failed (HTTP ${response.status}).`;
}
function exportUiPolicy(response) {
  const policy=response.headers.get('X-Export-Range-Policy');
  if(policy==='whole-clips-untrimmed')return 'Whole-clip copy; timing is metadata-derived, not a calibrated camera clock.';
  if(policy==='decoded-video-frame-window'){
    const raw=response.headers.get('X-Export-Output-Duration-Seconds') || '';
    const duration=/^\d+(?:\.\d{1,9})?$/.test(raw) && Number(raw)>0 && Number(raw)<86400?` Reported output: ${Number(raw)} seconds.`:'';
    return 'Frame-granular trim; video-only inputs; not exact camera wall-clock boundaries.'+duration;
  }
  throw exportUiError('policy','Export policy missing or unrecognized; reload after updating the server.');
}
async function exportUiReceive(response,job) {
  const length=response.headers.get('Content-Length');let expected=null;
  if(length!==null){if(!/^\d+$/.test(length))throw exportUiError('length','Invalid response length.');expected=Number(length);if(!Number.isSafeInteger(expected) || expected>EXPORT_UI_LIMIT)throw exportUiError('limit','Response exceeds the 64 MiB monitored-browser limit; reduce the range.');}
  if(!response.body || typeof response.body.getReader!=='function')throw exportUiError('reader','Streaming response reading is unavailable; no unbounded buffering fallback was used.');
  const reader=response.body.getReader();let chunks=[],size=0,done=false;
  try{
    while(true){
      const part=await exportUiAwait(reader.read(),job.controller.signal);if(part.done){done=true;break;}
      if(part.value.byteLength>EXPORT_UI_LIMIT-size)throw exportUiError('limit','Response exceeds the 64 MiB monitored-browser limit; reduce the range.');
      chunks.push(part.value);size+=part.value.byteLength;
      exportUiState(job,'receiving',`Receiving ${(size/(1024*1024)).toFixed(1)} MiB${expected===null?'':` of ${(expected/(1024*1024)).toFixed(1)} MiB`}; not saved yet.`);
    }
    if(size===0 || (expected!==null && size!==expected))throw exportUiError('incomplete','Transfer ended with an empty or mismatched-length response; no file was offered.');
    const prefix=new Uint8Array(Math.min(12,size));let copied=0;
    for(const chunk of chunks){const count=Math.min(chunk.byteLength,prefix.length-copied);prefix.set(chunk.subarray(0,count),copied);copied+=count;if(copied===prefix.length)break;}
    if(size<12 || prefix[4]!==102 || prefix[5]!==116 || prefix[6]!==121 || prefix[7]!==112)throw exportUiError('format','Response did not contain the expected MP4 header; no file was offered.');
    const blob=new Blob(chunks,{type:'video/mp4'});chunks=[];return {blob,size};
  }finally{if(!done)exportUiCancelReader(reader);try{reader.releaseLock();}catch(_){} }
}
function exportUiFilename(job) {
  const name=job.name.replace(/[^A-Za-z0-9._-]/g,'_').slice(0,80) || 'camera';
  const stamp=value=>value.replace(/[^A-Za-z0-9]/g,'');
  return `${name}_${stamp(job.start)}_${stamp(job.end)}.mp4`;
}
function exportUiOffer(job,received) {
  return new Promise((resolve,reject)=>{
    try{job.url=URL.createObjectURL(received.blob);}catch(error){reject(error);return;}
    let finished=false;
    job.finishReady=(state,message)=>{
      if(finished)return;finished=true;clearTimeout(job.readyTimer);
      if(job.url){URL.revokeObjectURL(job.url);job.url=null;}
      job.save.disabled=true;job.cancel.disabled=true;job.finishReady=null;exportUiState(job,state,message);resolve();
    };
    job.save=exportUiNode('button','Download prepared video');job.save.type='button';job.save.className='btn btn-primary';
    job.save.addEventListener('click',()=>{
      if(finished || job.handed)return;
      const anchor=document.createElement('a');anchor.href=job.url;anchor.download=exportUiFilename(job);
      try{document.body.appendChild(anchor);anchor.click();anchor.remove();}
      catch(_){anchor.remove();job.finishReady('failed','Browser download request failed; no disk save was verified.');return;}
      job.handed=true;job.save.disabled=true;job.cancel.textContent='Release prepared file / continue';
      exportUiState(job,'handed','Browser download requested. Check your browser downloads; disk saving is not verified. Release after the browser accepts it, or wait 60 seconds.');
      clearTimeout(job.readyTimer);job.readyTimer=setTimeout(()=>job.finishReady?.('handed','Browser download requested; prepared file released. Disk saving is not verified.'),60000);
    });
    job.row.appendChild(job.save);
    job.readyTimer=setTimeout(()=>job.finishReady?.('expired','Prepared file expired after 2 minutes without a download request. Prepare the export again.'),120000);
    exportUiState(job,'ready',`Ready to download: ${(received.size/(1024*1024)).toFixed(1)} MiB received. Click Download prepared video below within 2 minutes. Nothing has been saved yet; media completeness and disk saving are not verified.`);
    job.save.focus?.();
  });
}
async function exportUiRunJob(job) {
  if(job.reason==='canceled'){job.finished=true;job.cancel.disabled=true;return;}
  let response;job.transferring=true;
  const timer=setTimeout(()=>{if(job.transferring && !job.finished){job.reason='timeout';job.controller.abort();}},EXPORT_UI_TIMEOUT);
  try{
    exportUiState(job,'preparing','Preparing export on server. After receiving it, click Download prepared video; preparing alone does not save a file.');
    const query=`camera_id=${encodeURIComponent(job.id)}&start_time=${encodeURIComponent(job.start)}&end_time=${encodeURIComponent(job.end)}`;
    response=await exportUiAwait(fetch(`/api/download?${query}`,{credentials:'same-origin',cache:'no-store',redirect:'error',signal:job.controller.signal}),job.controller.signal);
    if(!response.ok || response.status!==200){const excerpt=await exportUiErrorText(response,job.controller.signal);throw exportUiError('http',exportUiFailureMessage(response)+(excerpt?' Server: '+excerpt:''));}
    if(response.redirected || (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase()!=='video/mp4')throw exportUiError('type','Unexpected response, possibly a sign-in page; no file was offered.');
    job.policy.textContent=exportUiPolicy(response);
    const received=await exportUiReceive(response,job);
    job.transferring=false;clearTimeout(timer);if(job.controller.signal.aborted)throw exportUiAbortError();
    await exportUiOffer(job,received);
  }catch(error){
    job.controller.abort();
    if(job.reason==='timeout')exportUiState(job,'failed','Browser export deadline exceeded; request canceled.');
    else if(job.reason==='canceled')exportUiState(job,'canceled','Canceled; no browser download requested.');
    else exportUiState(job,'failed',String(error.message || 'Export failed').slice(0,1000));
    if(response?.body){try{Promise.resolve(response.body.cancel()).catch(()=>{});}catch(_){} }
  }finally{clearTimeout(timer);job.transferring=false;job.finished=true;job.cancel.disabled=true;}
}
function startExportDownloads() {
  if(exportUiBatch?.running)return exportUiBatch.promise;
  const host=document.getElementById('download-status'),list=document.getElementById('download-progress-list');
  if(!host || !list){console.error('Export status region missing; no request started');return Promise.resolve();}
  host.style.display='block';list.replaceChildren();
  const start=downloadStartTime?.getTime(),end=downloadEndTime?.getTime(),ids=[...new Set(downloadCameras)];
  if(!Number.isFinite(start) || !Number.isFinite(end) || end<=start || ids.length<1 || ids.length>8){list.appendChild(exportUiNode('p','Choose a valid range and one to eight cameras. No request started.'));return Promise.resolve();}
  const cameras=ids.map(id=>allCameras.find(camera=>camera.id===id));
  if(cameras.some(camera=>!camera || typeof camera.id!=='string' || camera.id.length>256)){list.appendChild(exportUiNode('p','A selected camera is unavailable. Reload the camera list.'));return Promise.resolve();}
  const stop=exportUiNode('button','Cancel all exports');stop.type='button';stop.addEventListener('click',cancelExportDownloads);list.appendChild(stop);
  list.appendChild(exportUiNode('p','Two steps: prepare and receive, then click Download prepared video for each camera. Maximum 64 MiB per export, one at a time. The queue pauses for your download action and file release. Disk saving cannot be verified.'));
  const jobs=cameras.map(camera=>{
    const row=exportUiNode('div');row.className='export-job';row.dataset.cameraId=camera.id;
    const name=String(camera.name ?? camera.id).slice(0,256),status=exportUiNode('p','Queued'),policy=exportUiNode('p');
    status.setAttribute('aria-live','polite');row.appendChild(exportUiNode('h4',name));row.appendChild(status);row.appendChild(policy);
    const cancel=exportUiNode('button','Cancel');cancel.type='button';row.appendChild(cancel);list.appendChild(row);
    const job={id:camera.id,name,start:new Date(start).toISOString(),end:new Date(end).toISOString(),row,status,policy,cancel,controller:new AbortController(),reason:null,finished:false,handed:false,url:null};
    cancel.addEventListener('click',()=>exportUiCancel(job));exportUiState(job,'queued','Queued');return job;
  });
  const batch={jobs,running:true,promise:null};exportUiBatch=batch;updateDownloadTimeDisplay();
  const button=document.getElementById('export-start-btn');if(button)button.disabled=true;
  batch.promise=(async()=>{try{for(const job of jobs)await exportUiRunJob(job);}finally{batch.running=false;stop.disabled=true;if(button)button.disabled=false;updateDownloadTimeDisplay();}})();
  return batch.promise;
}
window.addEventListener('pagehide',cancelExportDownloads);
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',installExportRangeControls);else installExportRangeControls();
