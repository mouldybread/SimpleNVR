'use strict';
const TimelineGeometry = (() => {
  function view(start, end, zoom = 1, offsetHours = 0) {
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start || end - start > 26 * 3600000) throw new RangeError('Invalid timeline bounds');
    if (!Number.isFinite(zoom) || zoom < 1 || zoom > 24 || !Number.isFinite(offsetHours)) throw new RangeError('Invalid timeline viewport');
    const hours = (end - start) / 3600000, visibleHours = hours / zoom;
    const offset = Math.max(0, Math.min(hours - visibleHours, offsetHours));
    return {dayStart:start, dayEnd:end, hours, visibleHours, offsetHours:offset, start:start + offset * 3600000, end:start + (offset + visibleHours) * 3600000};
  }
  function fractionAt(time, viewport) { return (time - viewport.start) / (viewport.end - viewport.start); }
  function timeAt(fraction, viewport) {
    if (!Number.isFinite(fraction)) throw new RangeError('Invalid timeline position');
    return viewport.start + Math.max(0, Math.min(1, fraction)) * (viewport.end - viewport.start);
  }
  function clip(start, end, viewport) {
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return null;
    const a = Math.max(start, viewport.start), b = Math.min(end, viewport.end);
    return b > a ? {start:a, end:b, left:fractionAt(a, viewport), right:fractionAt(b, viewport)} : null;
  }
  function zoomAt(viewport, fraction, zoom) {
    const anchor = timeAt(fraction, viewport);
    const next = view(viewport.dayStart, viewport.dayEnd, zoom);
    return view(viewport.dayStart, viewport.dayEnd, zoom, (anchor - viewport.dayStart) / 3600000 - Math.max(0, Math.min(1, fraction)) * next.visibleHours);
  }
  function label(time) {
    const date = new Date(time);
    if (!Number.isFinite(date.getTime())) throw new RangeError('Invalid timeline timestamp');
    const offset = -date.getTimezoneOffset(), absolute = Math.abs(offset);
    const pad = value => String(value).padStart(2, '0');
    return `${pad(date.getHours())}:${pad(date.getMinutes())} UTC${offset < 0 ? '-' : '+'}${pad(Math.floor(absolute / 60))}:${pad(absolute % 60)}`;
  }
  return {view, fractionAt, timeAt, clip, zoomAt, label};
})();
if (typeof module !== 'undefined' && module.exports) module.exports = TimelineGeometry;
