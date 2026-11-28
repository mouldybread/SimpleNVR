function seekToTime(time) {
  if (!time || typeof time.getTime!=='function' || !Number.isFinite(time.getTime())) { playbackError('Invalid playback timestamp'); return false; }
  const milliseconds=time.getTime();
  pendingEventNavigation=null; cancelPlaybackTasks();
  if (isUpdatingGrid) { pendingSeekTime=milliseconds; return true; }
  lastSeekTime=milliseconds; currentTimePosition=milliseconds;
  const generation=seekGeneration;
  seekTimeoutId=setTimeout(() => { seekTimeoutId=null; if (generation===seekGeneration) performSeek(new Date(milliseconds),generation); },100);
  return true;
}
function scheduleVideoTask(fn,delay,generation) {
  const timer=setTimeout(() => { pendingVideoTimeouts=pendingVideoTimeouts.filter(id=>id!==timer); if (generation===seekGeneration) fn(); },delay);
  pendingVideoTimeouts.push(timer);
}
function setPlaybackButton() {
  const button=document.getElementById('play-pause');
  if (button) {
    button.textContent=isPlaying?'Pause':'Play';
    button.setAttribute?.('aria-label',isPlaying?'Pause recordings':'Play recordings');
  }
}
function stopMediaPlayback(message,video) {
  cancelPlaybackTasks();isPlaying=false;
  videoPlayers.forEach(player=>{if(player)player.pause();});
  if (video) {
    video.style.display='none';
    const unavailable=video.parentElement?.querySelector('.no-video-message');
    if (unavailable) unavailable.style.display='block';
  }
  setPlaybackButton();renderTimeline();playbackError(message);
}
function attachPlaybackSession(video,index,generation) {
  const session={video,index,generation,playToken:0,resumeEnded:null};
  session.valid=()=>generation===seekGeneration && videoPlayers[index]===video && playbackSessions.get(video)===session;
  const failed=()=>{if(session.valid())stopMediaPlayback('Playback media failed; select a time to retry',video);};
  const cleanup=()=>{
    video.removeEventListener('error',failed);
    if (playbackSessions.get(video)===session) playbackSessions.delete(video);
    segmentCleanups=segmentCleanups.filter(fn=>fn!==cleanup);
  };
  playbackSessions.set(video,session);segmentCleanups.push(cleanup);
  video.addEventListener('error',failed);return session;
}
function playSession(session) {
  if (!session || !session.valid() || !isPlaying) return;
  const intent=playbackIntentVersion,token=++session.playToken;
  const rejected=()=>{
    if (!session.valid() || !isPlaying || intent!==playbackIntentVersion || token!==session.playToken) return;
    playbackIntentVersion++;isPlaying=false;
    videoPlayers.forEach(player=>{if(player)player.pause();});
    setPlaybackButton();playbackError('Playback could not start; press Play to retry or select another time');
  };
  try {
    const result=session.video.play();
    if (result!==undefined) Promise.resolve(result).catch(rejected);
  } catch (_) {rejected();}
}
// This first continuation policy deliberately excludes multi-camera playback.
function attachSingleCameraContinuation(video,index,segment,generation) {
  const cameras=getOrderedSelectedCameras();
  if (cameras.length!==1 || cameras[0]!==segment.camera_id) return;
  const day=localDateKey(currentDate),bounds=timelineWindowForDay(day),dayEnd=Date.parse(bounds.end_time);
  const start=Date.parse(segment.start_time),end=Date.parse(segment.end_time);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end<=start) return;
  const valid=()=>generation===seekGeneration && videoPlayers[index]===video && localDateKey(currentDate)===day && getOrderedSelectedCameras().length===1 && getOrderedSelectedCameras()[0]===segment.camera_id;
  function stop(time,message) {
    cancelPlaybackTasks();isPlaying=false;videoPlayers.forEach(player=>{if(player)player.pause();});
    currentTimePosition=Math.min(time,dayEnd-1);
    setPlaybackButton();
    renderTimeline();playbackError(message);
  }
  const update=()=>{
    if (!valid() || !isPlaying || !Number.isFinite(video.currentTime)) return;
    const time=start+Math.max(0,video.currentTime)*1000;
    if (time>=dayEnd) {stop(dayEnd,'Selected playback day ended; select another day to continue');return;}
    currentTimePosition=Math.min(time,end);renderTimeline();
  };
  const ended=()=>{
    if (!valid() || !isPlaying || !video.ended) return;
    if (end>=dayEnd) {stop(dayEnd,'Selected playback day ended; select another day to continue');return;}
    const next=timelineSegments.filter(item=>item.camera_id===segment.camera_id && Date.parse(item.start_time)<=end && Date.parse(item.end_time)>end);
    if (next.length!==1 || Date.parse(next[0].start_time)!==end || !next[0].file_path || next[0].file_path===segment.file_path) {
      stop(end,'No unique contiguous recording; select another time to continue');return;
    }
    cancelPlaybackTasks();currentTimePosition=end;lastSeekTime=end;
    performSeek(new Date(end),seekGeneration);
  };
  const session=playbackSessions.get(video);
  if (session) session.resumeEnded=ended;
  const cleanup=()=>{video.removeEventListener('timeupdate',update);video.removeEventListener('ended',ended);if(session && session.resumeEnded===ended)session.resumeEnded=null;segmentCleanups=segmentCleanups.filter(fn=>fn!==cleanup);};
  segmentCleanups.push(cleanup);video.addEventListener('timeupdate',update);video.addEventListener('ended',ended);
}
function performSeek(time,generation=seekGeneration) {
  if (generation!==seekGeneration) return;
  if (!getOrderedSelectedCameras().length) {isPlaying=false;videoPlayers.forEach(video=>{if(video)video.pause();});setPlaybackButton();return;}
  getOrderedSelectedCameras().forEach((id,index) => scheduleVideoTask(() => {
    const video=videoPlayers[index]; if (!video) return;
    const segment=timelineSegments.find(s=>s.camera_id===id && time.getTime()>=new Date(s.start_time).getTime() && time.getTime()<new Date(s.end_time).getTime());
    const message=video.parentElement?.querySelector('.no-video-message');
    video.pause(); video.removeAttribute('src'); video.style.display=segment?'block':'none'; if (message) message.style.display=segment?'none':'block';
    if (!segment) {
      if (!pendingVideoTimeouts.length && !mediaCleanups.length && !playbackSessions.size) {isPlaying=false;setPlaybackButton();}
      return;
    }
    scheduleVideoTask(() => {
      if (videoPlayers[index]!==video) return;
      const offset=(time.getTime()-new Date(segment.start_time).getTime())/1000;
      const cleanup=() => { video.removeEventListener('loadedmetadata',metadata); video.removeEventListener('error',failed); mediaCleanups=mediaCleanups.filter(fn=>fn!==cleanup); };
      const metadata=() => {
        cleanup(); if (generation!==seekGeneration || videoPlayers[index]!==video) return;
        video.currentTime=Math.max(0,Math.min(offset,video.duration)); video.playbackRate=playbackSpeed;
        const session=attachPlaybackSession(video,index,generation);
        attachSingleCameraContinuation(video,index,segment,generation);
        if (isPlaying) playSession(session);
      };
      const failed=() => { cleanup(); if (generation!==seekGeneration || videoPlayers[index]!==video) return; stopMediaPlayback('Unable to load playback media; select a time to retry',video); };
      mediaCleanups.push(cleanup); video.addEventListener('loadedmetadata',metadata); video.addEventListener('error',failed); video.src=segment.file_path;
    },10,generation);
  },index*150,generation));
  renderTimeline();
  if (!isPlaying) isPlaying=true;
  setPlaybackButton();
}
