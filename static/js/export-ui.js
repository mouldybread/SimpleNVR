// Monitored UI entry point. Legacy startDownload is no longer wired to the page.
const EXPORT_UI_LIMIT = 64 * 1024 * 1024;
const EXPORT_UI_TIMEOUT = 4 * 60 * 1000;
let exportUiBatch = null;

function exportUiError(kind,message) { const error=new Error(message); error.kind=kind; return error; }
function exportUiAbortError() { const error=new Error('Export canceled'); error.name='AbortError'; return error; }
function exportUiAwait(promise,signal) {
  return new Promise((resolve,reject)=>{
    let settled=false;
    const finish=(fn,value)=>{if(settled)return;settled=true;signal.removeEventListener('abort',abort);fn(value);};
    const abort=()=>finish(reject,exportUiAbortError());
    Promise.resolve(promise).then(value=>finish(resolve,value),error=>finish(reject,error));
    if(signal.aborted){abort();return;}
    signal.addEventListener('abort',abort,{once:true});
  });
}
function exportUiNode(tag,text='') {const node=document.createElement(tag);node.textContent=String(text);return node;}
function exportUiState(job,state,text) {job.state=state;job.row.dataset.state=state;job.status.textContent=text;}
function exportUiCancel(job) {
  if(job.finished)return;
  if(job.finishReady){job.finishReady(job.handed?'handed':'canceled',job.handed?'Browser hand-off requested; prepared URL released. Disk saving is not verified.':'Canceled; prepared file discarded.');return;}
  job.reason='canceled';job.controller.abort();exportUiState(job,'canceled','Canceled; no file handed to browser.');
}
function cancelExportDownloads() {if(exportUiBatch)exportUiBatch.jobs.forEach(exportUiCancel);}
function exportUiCancelReader(reader) {try{Promise.resolve(reader.cancel()).catch(()=>{});}catch(_){} }
async function exportUiErrorText(response,signal) {
  if(!response.body || typeof response.body.getReader!=='function')return '';
  const reader=response.body.getReader();const chunks=[];let size=0,done=false;
  try{
    while(size<2048){const part=await exportUiAwait(reader.read(),signal);if(part.done){done=true;break;}const chunk=part.value.slice(0,2048-size);chunks.push(chunk);size+=chunk.byteLength;}
    const raw=new Uint8Array(size);let offset=0;for(const chunk of chunks){raw.set(chunk,offset);offset+=chunk.byteLength;}
    return new TextDecoder().decode(raw).replace(/[\u0000-\u001f\u007f]/g,' ').slice(0,600);
  }finally{if(!done)exportUiCancelReader(reader);try{reader.releaseLock();}catch(_){} }
}
function exportUiFailureMessage(response) {
  const plan=response.headers.get('X-Export-Plan-Status');
  if(plan==='gap')return 'Missing recording coverage; adjust the requested range.';
  if(plan==='overlap')return 'Overlapping recordings require an explicit clip-choice policy.';
  if(response.status===401 || response.status===403)return 'Authentication expired or access denied; sign in again.';
  if(response.status===404)return 'Camera or recording coverage was not found.';
  if(response.status===413)return 'Server export limit exceeded; reduce the range.';
  if(response.status===422)return 'Trim profile unsupported, including audio-containing inputs; no silent audio removal.';
  if(response.status===504)return 'Server preparation or rendering timed out.';
  if(response.status===503)return 'Recording metadata unavailable, changed, or server shutting down.';
  return `Export request failed (HTTP ${response.status}).`;
}
function exportUiPolicy(response) {
  const policy=response.headers.get('X-Export-Range-Policy');
  if(policy==='whole-clips-untrimmed')return 'Whole-clip copy; timing is metadata-derived, not a calibrated camera clock.';
  if(policy==='decoded-video-frame-window'){
    const raw=response.headers.get('X-Export-Output-Duration-Seconds') || '';
    const duration=/^\d+(?:\.\d{1,9})?$/.test(raw) && Number(raw)>0 && Number(raw)<86400 ? ` Reported output: ${Number(raw)} seconds.` : '';
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
      const part=await exportUiAwait(reader.read(),job.controller.signal);
      if(part.done){done=true;break;}
      if(part.value.byteLength>EXPORT_UI_LIMIT-size)throw exportUiError('limit','Response exceeds the 64 MiB monitored-browser limit; reduce the range.');
      chunks.push(part.value);size+=part.value.byteLength;
      exportUiState(job,'receiving',`Receiving ${(size/(1024*1024)).toFixed(1)} MiB${expected===null?'':` of ${(expected/(1024*1024)).toFixed(1)} MiB`}; not saved.`);
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
  return `${name}_${job.start.slice(0,10)}.mp4`;
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
    job.save=exportUiNode('button','Hand received file to browser');job.save.type='button';
    job.save.addEventListener('click',()=>{
      if(finished || job.handed)return;
      const anchor=document.createElement('a');anchor.href=job.url;anchor.download=exportUiFilename(job);
      try{document.body.appendChild(anchor);anchor.click();anchor.remove();}
      catch(_){anchor.remove();job.finishReady('failed','Browser hand-off failed; no disk save was verified.');return;}
      job.handed=true;job.save.disabled=true;job.cancel.textContent='Release prepared file / continue';
      exportUiState(job,'handed','Browser hand-off requested; disk saving is not verified. Release after the browser accepts it, or wait 60 seconds.');
      clearTimeout(job.readyTimer);job.readyTimer=setTimeout(()=>job.finishReady?.('handed','Browser hand-off requested; prepared URL released. Disk saving is not verified.'),60000);
    });
    job.row.appendChild(job.save);
    job.readyTimer=setTimeout(()=>job.finishReady?.('expired','Prepared file expired without a browser hand-off; request again if needed.'),120000);
    exportUiState(job,'ready',`Received ${(received.size/(1024*1024)).toFixed(1)} MiB. Click to request a browser download; media completeness and disk saving are not verified.`);
  });
}
async function exportUiRunJob(job) {
  if(job.reason==='canceled'){job.finished=true;job.cancel.disabled=true;return;}
  let response;job.transferring=true;
  const timer=setTimeout(()=>{if(job.transferring && !job.finished){job.reason='timeout';job.controller.abort();}},EXPORT_UI_TIMEOUT);
  try{
    exportUiState(job,'preparing','Preparing export; waiting for the server.');
    const query=`camera_id=${encodeURIComponent(job.id)}&start_time=${encodeURIComponent(job.start)}&end_time=${encodeURIComponent(job.end)}`;
    response=await exportUiAwait(fetch(`/api/download?${query}`,{credentials:'same-origin',cache:'no-store',redirect:'error',signal:job.controller.signal}),job.controller.signal);
    if(!response.ok || response.status!==200){const excerpt=await exportUiErrorText(response,job.controller.signal);throw exportUiError('http',exportUiFailureMessage(response)+(excerpt?' Server: '+excerpt:''));}
    if(response.redirected || (response.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase()!=='video/mp4')throw exportUiError('type','Unexpected response, possibly a sign-in page; no file was offered.');
    job.policy.textContent=exportUiPolicy(response);
    const received=await exportUiReceive(response,job);
    job.transferring=false;clearTimeout(timer);
    if(job.controller.signal.aborted)throw exportUiAbortError();
    await exportUiOffer(job,received);
  }catch(error){
    job.controller.abort();
    if(job.reason==='timeout')exportUiState(job,'failed','Browser export deadline exceeded; request canceled.');
    else if(job.reason==='canceled')exportUiState(job,'canceled','Canceled; no file handed to browser.');
    else exportUiState(job,'failed',String(error.message || 'Export failed').slice(0,1000));
    if(response?.body){try{Promise.resolve(response.body.cancel()).catch(()=>{});}catch(_){} }
  }finally{clearTimeout(timer);job.transferring=false;job.finished=true;job.cancel.disabled=true;}
}
function startExportDownloads() {
  if(exportUiBatch?.running)return exportUiBatch.promise;
  const host=document.getElementById('download-status'),list=document.getElementById('download-progress-list');
  if(!host || !list){console.error('Export status region missing; no request started');return Promise.resolve();}
  host.style.display='block';list.replaceChildren();
  const start=downloadStartTime?.getTime(),end=downloadEndTime?.getTime();
  const ids=[...new Set(downloadCameras)];
  if(!Number.isFinite(start) || !Number.isFinite(end) || end<=start || ids.length<1 || ids.length>8){list.appendChild(exportUiNode('p','Choose a valid range and one to eight cameras. No request started.'));return Promise.resolve();}
  const cameras=ids.map(id=>allCameras.find(camera=>camera.id===id));
  if(cameras.some(camera=>!camera || typeof camera.id!=='string' || camera.id.length>256)){list.appendChild(exportUiNode('p','A selected camera is unavailable. Reload the camera list.'));return Promise.resolve();}
  const stop=exportUiNode('button','Cancel all exports');stop.type='button';stop.addEventListener('click',cancelExportDownloads);list.appendChild(stop);
  list.appendChild(exportUiNode('p','Monitored exports receive at most 64 MiB each, one at a time. The queue pauses while a received file is prepared for browser hand-off. No disk-save confirmation is available.'));
  const jobs=cameras.map(camera=>{
    const row=exportUiNode('div');row.className='export-job';row.dataset.cameraId=camera.id;
    const name=String(camera.name ?? camera.id).slice(0,256),status=exportUiNode('p','Queued'),policy=exportUiNode('p');
    status.setAttribute('aria-live','polite');row.appendChild(exportUiNode('h4',name));row.appendChild(status);row.appendChild(policy);
    const cancel=exportUiNode('button','Cancel');cancel.type='button';row.appendChild(cancel);list.appendChild(row);
    const job={id:camera.id,name,start:new Date(start).toISOString(),end:new Date(end).toISOString(),row,status,policy,cancel,controller:new AbortController(),reason:null,finished:false,handed:false,url:null};
    cancel.addEventListener('click',()=>exportUiCancel(job));exportUiState(job,'queued','Queued');return job;
  });
  const batch={jobs,running:true,promise:null};exportUiBatch=batch;
  const button=document.getElementById('export-start-btn');if(button)button.disabled=true;
  batch.promise=(async()=>{try{for(const job of jobs)await exportUiRunJob(job);}finally{batch.running=false;stop.disabled=true;if(button)button.disabled=false;}})();
  return batch.promise;
}
window.addEventListener('pagehide',cancelExportDownloads);
