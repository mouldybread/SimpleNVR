# Export mode API integration (intermediate commit)

The HTTP job planner now calls the copy/accurate renderers instead of the legacy video-only trim renderer. POST /api/exports/create accepts mode (copy by default, or accurate). It never automatically switches modes. Job output accounting is populated from the validated output file size, including direct renderer writes.

For complete recordings, copy preserves original bytes (one file) or compatible compressed A/V packets (multiple files). Accurate explicitly re-encodes supported video and AAC audio and applies the existing renderer validation.

For subsections, copy scans decoded keyframe timestamps and supports non-reordered video only. It rounds the video start earlier and end later to available keyframe/file boundaries, rechecks expanded coverage for gaps/overlaps, and responds with 409 JSON code copy_range_approval_required before admitting a job or reserving output storage. The response includes requested_start/end, export_start/end and audio_allowance_ms (one AAC packet). Approval must send accepted_start/end matching the freshly recomputed proposal along with the unchanged original request. Stale approval never silently approves a different video interval. Audio packet edges can extend outside the video-aligned interval by the disclosed allowance.

Copy rendering uses pinned descriptors, explicit track mapping, bounded output, duration checks and an external decode before readiness. Reordered video, missing keyframe metadata, unsupported tracks, incompatible configurations and undecodable random-access output are refused; choose accurate explicitly. These checks are not a general proof of frame/sample completeness or calibrated wall-clock boundaries.

This is not yet a finished application change: the corresponding UI mode/approval controls still need to be committed, and the real-browser acceptance harness still requires migration. Go compilation/tests have not run in the assistant environment. A local FFmpeg command experiment, not the Go implementation, produced a decodable non-reordered H.264/AAC keyframe-aligned subsection. Do not merge/release on that evidence alone.
