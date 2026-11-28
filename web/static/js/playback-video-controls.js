function togglePlayPause() {
  playbackIntentVersion++;
  if (isPlaying) {
    isPlaying=false;videoPlayers.forEach(video=>{if(video)video.pause();});setPlaybackButton();return;
  }
  const sessions=videoPlayers.filter(Boolean).map(video=>playbackSessions.get(video)).filter(session=>session && session.valid());
  if (!sessions.length && !mediaCleanups.length && !pendingVideoTimeouts.length && seekTimeoutId===null) {
    setPlaybackButton();playbackError('Select a recording time before starting playback');return;
  }
  isPlaying=true;setPlaybackButton();
  const ended=sessions.filter(session=>session.video.ended);
  if (ended.length) {
    if (sessions.length===1 && getOrderedSelectedCameras().length===1 && ended[0].resumeEnded) ended[0].resumeEnded();
    else stopMediaPlayback('Ended playback cannot advance safely; select another time');
    return;
  }
  sessions.forEach(playSession);
}
function toggleFullscreen() {
  const container=document.getElementById('fullscreen-wrapper'); if (!container) return;
  const promise=document.fullscreenElement?document.exitFullscreen():container.requestFullscreen(); promise.catch(error=>console.error('Fullscreen failed:',error));
}
function toggleMuteAll() {
  const muted=videoPlayers.filter(Boolean).every(video=>video.muted); videoPlayers.forEach(video=>{ if (video) video.muted=!muted; });
  const button=document.getElementById('mute-all-btn'); if (button) button.textContent=muted?'🔊 Unmute All':'🔇 Mute All';
}
function updateVideoGrid() {
  const container=document.getElementById('video-grid'); if (!container) return;
  cancelPlaybackTasks(); isPlaying=false;setPlaybackButton();isUpdatingGrid=true; videoPlayers.forEach(video=>{ if (video) { video.pause(); video.removeAttribute('src'); } });
  container.innerHTML=''; videoPlayers=[];
  const ordered=getOrderedSelectedCameras(), count=ordered.length;
  if (!count) { container.innerHTML='<div style="padding:40px;text-align:center;color:#888;font-size:18px;">Select cameras to begin playback</div>'; isUpdatingGrid=false; return; }
  const columns=count===1?1:count<=4?2:count<=9?3:4;
  container.style.display='grid'; container.style.gap='10px'; container.style.gridTemplateColumns=`repeat(${columns}, 1fr)`;
  ordered.forEach(id=>{
    const camera=allCameras.find(cam=>cam.id===id); if (!camera) return;
    const wrapper=document.createElement('div'); wrapper.className='video-cell'; wrapper.style.cssText='position:relative;background-color:#1a1a1a;aspect-ratio:16/9;';
    const label=document.createElement('div'); label.className='video-label'; label.textContent=camera.name; label.style.backgroundColor=cameraColorMap[id] || '#888';
    const message=document.createElement('div'); message.className='no-video-message'; message.style.cssText='position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);text-align:center;color:#888;font-size:18px;pointer-events:none;z-index:10;display:none;';
    message.innerHTML='<div>No Recording Available</div><div style="font-size:14px;margin-top:10px;">Select a different time</div>';
    const video=document.createElement('video'); video.className='timeline-video'; video.controls=false; video.muted=true; video.style.cssText='width:100%;height:100%;object-fit:contain;display:none;';
    wrapper.appendChild(label); wrapper.appendChild(message); wrapper.appendChild(video); container.appendChild(wrapper); videoPlayers.push(video);
  });
  isUpdatingGrid=false;
  if (pendingSeekTime!==null) { const time=new Date(pendingSeekTime); pendingSeekTime=null; seekToTime(time); }
  else if (currentTimePosition!==null) seekToTime(new Date(currentTimePosition));
}
function previousSegment() { const time=new Date(currentTimePosition ?? Date.now()); time.setMinutes(time.getMinutes()-10); seekToTime(time); }
function nextSegment() { const time=new Date(currentTimePosition ?? Date.now()); time.setMinutes(time.getMinutes()+10); seekToTime(time); }
function updatePlaybackSpeed() { videoPlayers.forEach(video=>{ if (video) video.playbackRate=playbackSpeed; }); }
function setupKeyboardShortcuts() {
  document.addEventListener('keydown',e=>{
    if (e.defaultPrevented || e.repeat || e.altKey || e.ctrlKey || e.metaKey) return;
    if (['INPUT','TEXTAREA','SELECT','BUTTON','A'].includes(e.target?.tagName) || e.target?.isContentEditable || e.target?.closest?.('input,textarea,select,button,a,[contenteditable]')) return;
    if (e.code==='Space') { e.preventDefault(); togglePlayPause(); } else if (e.code==='ArrowLeft') { e.preventDefault(); previousSegment(); } else if (e.code==='ArrowRight') { e.preventDefault(); nextSegment(); }
  });
}
if (document.readyState==='loading') document.addEventListener('DOMContentLoaded',initializeTimelinePlayback); else initializeTimelinePlayback();
