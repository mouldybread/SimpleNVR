const assert=require('node:assert/strict');
const {environment,source,id,view,response}=require('./export-ui-fixture.cjs');
assert.ok(!source.includes('new Blob('));assert.ok(!source.includes('createObjectURL'));assert.ok(!source.includes('arrayBuffer()'));assert.ok(!source.includes('EXPORT_UI_LIMIT'));
(async()=>{
 let creates=0;
 const good=environment((url,options)=>{
  if(url==='/api/exports/create')return Promise.resolve(response(202,view(++creates===1?'a':'b','queued')));
  if(url.startsWith('/api/exports/status'))return Promise.resolve(response(200,view(url.endsWith(id('a'))?'a':'b')));
  if(url==='/api/exports/cancel')return Promise.resolve(response(204));
  throw Error('Video must not be fetched by JavaScript: '+url);
 },['front','back']);
 const batch=good.run('startExportDownloads()');await good.flush();
 assert.equal(good.requests.length,1);assert.equal(good.jobs()[0].state,'queued');assert.equal(good.requests[0].options.method,'POST');
 assert.equal(good.requests[0].options.credentials,'same-origin');assert.equal(JSON.parse(good.requests[0].options.body).camera_id,'front');
 good.run("downloadStartTime=new Date('2026-12-01T00:00:00Z')");
 await good.fire(1500);assert.equal(good.jobs()[0].state,'ready');assert.ok(good.jobs()[0].status.textContent.includes('5120.0 MiB'));assert.equal(good.downloads.length,0);
 assert.ok(good.find('download-progress-list').textContent.includes('<img src=x onerror=pwned=1>'));assert.equal(good.context.pwned,undefined);
 good.jobs()[0].save.click();assert.deepEqual(good.downloads,['/api/exports/download?id='+id('a')]);assert.equal(good.jobs()[0].state,'handed');assert.ok(good.jobs()[0].status.textContent.includes('not verified'));
 await good.run('exportUiCancel(exportUiBatch.jobs[0])');await good.flush();assert.equal(good.jobs()[0].state,'released');assert.equal(creates,2);
 assert.equal(JSON.parse(good.requests.findLast(r=>r.url==='/api/exports/create').options.body).start_time,'2026-11-21T00:00:00.000Z');
 await good.fire(1500);await good.run('cancelExportDownloads()');await batch;assert.equal(good.jobs()[1].state,'canceled');assert.equal(good.timers.size,0);
 for(const [status,header,word] of [[401,{},'Authentication'],[503,{},'NVR_EXPORT_SCRATCH_DIR'],[429,{},'capacity'],[409,{'X-Export-Plan-Status':'gap'},'full interval'],[422,{},'profile']]){
  const env=environment(()=>Promise.resolve(response(status,'<svg onload=pwned=1>test',header)));
  await env.run('startExportDownloads()');assert.equal(env.jobs()[0].state,'failed');assert.ok(env.jobs()[0].status.textContent.includes(word));assert.equal(env.downloads.length,0);assert.equal(env.requests.length,1);assert.equal(env.context.pwned,undefined);
 }
 let finishCreate;
 const late=environment((url)=>url==='/api/exports/create'?new Promise(resolve=>{finishCreate=resolve;}):Promise.resolve(response(204)));
 const lateBatch=late.run('startExportDownloads()');await late.flush();await late.run('cancelExportDownloads()');assert.equal(late.jobs()[0].state,'canceling');finishCreate(response(202,view('c','queued')));await lateBatch;
 assert.equal(late.jobs()[0].state,'canceled');assert.equal(late.requests[1].url,'/api/exports/cancel');assert.equal(JSON.parse(late.requests[1].options.body).id,id('c'));
 const expired=environment(()=>Promise.resolve(response(202,view('d','ready',{expires_at:new Date(Date.now()-1000).toISOString()}))));await expired.run('startExportDownloads()');assert.equal(expired.jobs()[0].state,'expired');assert.equal(expired.downloads.length,0);
 const malicious=environment(()=>Promise.resolve(response(202,view('e','ready',{download_url:'https://evil.invalid/'}))));await malicious.run('startExportDownloads()');assert.equal(malicious.jobs()[0].state,'failed');assert.equal(malicious.downloads.length,0);
 const huge=environment(()=>Promise.resolve(response(202,'x'.repeat(8193))));await huge.run('startExportDownloads()');assert.equal(huge.jobs()[0].state,'failed');assert.equal(huge.timers.size,0);
 let releases=0;
 const releaseFail=environment(url=>Promise.resolve(url==='/api/exports/create'?response(202,view('f')):++releases===1?response(503,'busy'):response(204)));
 const failBatch=releaseFail.run('startExportDownloads()');await releaseFail.flush();await releaseFail.run('cancelExportDownloads()');await failBatch;
 assert.equal(releaseFail.jobs()[0].state,'release-failed');assert.ok(releaseFail.jobs()[0].status.textContent.includes('could not be confirmed'));assert.equal(releaseFail.jobs()[0].cancel.disabled,false);
 await releaseFail.run('cancelExportDownloads()');assert.equal(releaseFail.jobs()[0].state,'canceled');assert.equal(releases,2);
 console.log('PASS prepared-job UI: 5 GiB metadata, no media buffering, bounded JSON, auth/configuration errors, safe text, immutable ranges, cancel race, release confirmation and expiry');
})().catch(error=>{console.error(error);process.exitCode=1;});
