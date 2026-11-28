function timelineViewport() {
  const bounds = timelineWindowForDay(localDateKey(currentDate));
  const viewport = TimelineGeometry.view(Date.parse(bounds.start_time),Date.parse(bounds.end_time),timelineZoomLevel,timelineOffset);
  timelineOffset = viewport.offsetHours;
  return viewport;
}
function timelinePointer(canvas,event) {
  const rect=canvas.getBoundingClientRect();
  if (!(rect.width>0) || !(rect.height>0)) return null;
  return {fraction:Math.max(0,Math.min(1,(event.clientX-rect.left)/rect.width)),x:(event.clientX-rect.left)*canvas.width/rect.width,y:(event.clientY-rect.top)*canvas.height/rect.height};
}
function renderTimeline() {
  const canvas=document.getElementById('timeline-canvas'); if (!canvas) return;
  const ctx=canvas.getContext('2d'),width=canvas.width=canvas.offsetWidth,height=canvas.height=85,viewport=timelineViewport();
  ctx.fillStyle='#1a1a1a'; ctx.fillRect(0,0,width,height); ctx.lineWidth=1;
  ctx.font='10px monospace'; ctx.textAlign='left'; ctx.textBaseline='alphabetic';
  const steps=viewport.visibleHours>=12?[2,1]:viewport.visibleHours>=4?[1,0.5]:viewport.visibleHours>=2?[0.5,1/6]:[1/6,1/60];
  function ticks(step,major) {
    const stepMs=step*3600000; let previousLabelRight=-Infinity;
    for (let index=Math.ceil((viewport.start-viewport.dayStart)/stepMs);;index++) {
      const time=viewport.dayStart+index*stepMs; if (time>viewport.end) break;
      const x=TimelineGeometry.fractionAt(time,viewport)*width;
      ctx.strokeStyle=major?'#555':'#333';ctx.beginPath();ctx.moveTo(x,major?0:height-10);ctx.lineTo(x,height);ctx.stroke();
      if (major) {
        const label=TimelineGeometry.label(time),labelWidth=ctx.measureText(label).width;
        if (x+2>=previousLabelRight+6 && x+2+labelWidth<=width) {ctx.fillStyle='#888';ctx.fillText(label,x+2,12);previousLabelRight=x+2+labelWidth;}
      }
    }
  }
  ticks(steps[1],false);ticks(steps[0],true);
  timelineEvents.forEach(event=>{
    delete event._canvasX;delete event._canvasY;
    const time=Date.parse(event.start_time);
    if (!Number.isFinite(time) || time<viewport.start || time>=viewport.end) return;
    const x=TimelineGeometry.fractionAt(time,viewport)*width,y=23;
    ctx.strokeStyle='rgba(76, 175, 80, 0.3)';ctx.lineWidth=1;ctx.beginPath();ctx.moveTo(x,30);ctx.lineTo(x,height);ctx.stroke();
    ctx.fillStyle='#4CAF50';ctx.beginPath();ctx.arc(x,y,6,0,Math.PI*2);ctx.fill();
    ctx.font='10px Arial';ctx.textAlign='center';ctx.textBaseline='middle';ctx.fillStyle='#fff';ctx.fillText(eventIconText(event.label),x,y);
    event._canvasX=x;event._canvasY=y;event._radius=6;
  });
  const cameras=[...new Set(timelineSegments.map(segment=>segment.camera_id))],blockHeight=(height-32)/Math.max(cameras.length,1);
  cameras.forEach((id,index)=>timelineSegments.filter(segment=>segment.camera_id===id).forEach(segment=>{
    const span=TimelineGeometry.clip(Date.parse(segment.start_time),Date.parse(segment.end_time),viewport);if (!span) return;
    ctx.fillStyle=cameraColorMap[id] || '#888';ctx.fillRect(span.left*width,32+index*blockHeight,(span.right-span.left)*width,blockHeight-2);
  }));
  if (Number.isFinite(currentTimePosition) && currentTimePosition>=viewport.start && currentTimePosition<viewport.end) {
    const x=TimelineGeometry.fractionAt(currentTimePosition,viewport)*width;
    ctx.strokeStyle='#fff';ctx.lineWidth=2;ctx.beginPath();ctx.moveTo(x,0);ctx.lineTo(x,height);ctx.stroke();ctx.lineWidth=1;
  }
  // Keep the export range bar aligned with the timeline after zoom, pan or day changes.
  if (typeof renderExportRangeOverlay==='function') renderExportRangeOverlay();
}
function setupTimelineZoom() {
  const canvas=document.getElementById('timeline-canvas');if (!canvas) return;
  canvas.addEventListener('wheel',e=>{
    e.preventDefault();const pointer=timelinePointer(canvas,e);if (!pointer) return;
    const zoom=Math.max(1,Math.min(24,timelineZoomLevel*(e.deltaY>0?0.8:1.25)));if (zoom===timelineZoomLevel) return;
    const viewport=TimelineGeometry.zoomAt(timelineViewport(),pointer.fraction,zoom);
    timelineZoomLevel=zoom;timelineOffset=viewport.offsetHours;renderTimeline();
  },{passive:false});
}
function setupTimelineControls() {
  const canvas=document.getElementById('timeline-canvas');if (!canvas) return;
  function hit(pointer) {return timelineEvents.find(event=>Number.isFinite(event._canvasX) && Number.isFinite(event._canvasY) && Math.hypot(pointer.x-event._canvasX,pointer.y-event._canvasY)<=event._radius+5);}
  canvas.addEventListener('click',e=>{
    const pointer=timelinePointer(canvas,e);if (!pointer) return;const event=hit(pointer);
    if (event) {navigateToEvent(event.camera_name,event.start_time);return;}
    const viewport=timelineViewport();seekToTime(new Date(Math.min(viewport.dayEnd-1,TimelineGeometry.timeAt(pointer.fraction,viewport))));
  });
  canvas.addEventListener('mousemove',e=>{
    const pointer=timelinePointer(canvas,e),event=pointer && hit(pointer);
    canvas.style.cursor=event?'pointer':'default';if (event) showEventTooltip(event,e.clientX,e.clientY);else hideEventTooltip();
  });
  canvas.addEventListener('mouseleave',hideEventTooltip);
}
function showEventTooltip(event,x,y) {
  hideEventTooltip(); const tooltip=document.createElement('div'); tooltip.id='event-tooltip';
  tooltip.style.cssText='position:fixed;background:rgba(0,0,0,0.9);color:#fff;padding:8px 12px;border-radius:6px;font-size:12px;z-index:10000;pointer-events:none;max-width:250px;box-shadow:0 2px 8px rgba(0,0,0,0.3);';
  tooltip.style.left=(x+10)+'px'; tooltip.style.top=(y+10)+'px';
  tooltip.appendChild(eventTextElement('div',`${eventIconText(event.label)} ${event.label ?? ''}`,'font-weight:bold;color:#4CAF50;margin-bottom:4px;'));
  const details=document.createElement('div'); details.style.cssText='font-size:11px;color:#ccc;';
  details.appendChild(eventTextElement('div',`📹 ${event.camera_name ?? ''}`)); details.appendChild(eventTextElement('div',`🕐 ${new Date(event.start_time).toLocaleTimeString()}`)); details.appendChild(eventTextElement('div',`⏱️ ${event.duration_seconds || event.duration}s`));
  tooltip.appendChild(details); tooltip.appendChild(eventTextElement('div','Click to view recording','font-size:10px;color:#888;margin-top:4px;font-style:italic;'));
  document.body.appendChild(tooltip); eventTooltip=tooltip;
}
function hideEventTooltip() { if (eventTooltip) { eventTooltip.remove(); eventTooltip=null; } }
