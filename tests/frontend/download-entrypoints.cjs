const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const context=vm.createContext({console,document:{readyState:'loading',addEventListener(){},getElementById(){return null;}},window:{addEventListener(){}}});
vm.runInContext(['web/static/js/playback-state.js','web/static/js/playback-camera-date.js','web/static/js/playback-timeline-load.js','web/static/js/playback-canvas.js','web/static/js/playback-seek-session.js','web/static/js/playback-video-controls.js'].map(f=>fs.readFileSync(f,'utf8')).join(''),context,{filename:'web/static/js/playback.js'});
assert.equal(vm.runInContext('typeof startDownload',context),'undefined');
assert.equal(Object.prototype.hasOwnProperty.call(context,'startDownload'),false);
// The old export modal and its separate camera checklist are gone; export uses the playback selection.
for (const legacy of ['openDownloadModal','closeDownloadModal','toggleDownloadCamera','renderDownloadCameraSelection','expandDownloadTimeEarlier','expandDownloadTimeLater'])
 assert.equal(vm.runInContext(`typeof ${legacy}`,context),'undefined',legacy);
vm.runInContext(fs.readFileSync('web/static/js/export-ui.js','utf8'),context,{filename:'web/static/js/export-ui.js'});
assert.equal(vm.runInContext('typeof startDownload',context),'undefined');
assert.equal(vm.runInContext('typeof startExportDownloads',context),'function');
for (const entry of ['cancelExportDownloads','openExportPanel','closeExportPanel','refreshExportQueue','clearExportQueue','renderExportRangeOverlay','refreshExportCameraGuidance'])
 assert.equal(vm.runInContext(`typeof ${entry}`,context),'function',entry);
console.log('Legacy direct-download and modal globals absent; export panel entry points available');
