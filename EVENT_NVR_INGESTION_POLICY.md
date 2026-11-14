# NVR-ingestion time policy and qualification

Decision: the user selected NVR ingestion time on November 14, 2026.
Supersedes EVENT_CAMERA_UTC_POLICY.md from audit head
73ef2430556cccf0b5527cdd0c0e8556edd02a5b.

Status: POLICY AND OFFLINE QUALIFICATION ONLY. No production timing implementation,
media arguments, schema, publication behavior or cleanup authorization is changed.
The custom-FFmpeg proposal is withdrawn. No patched FFmpeg build is required or
approved by this policy.

## Selected clock

start_time refers to the NVR's media-ingestion timeline expressed using the NVR
host's UTC clock, not the camera's capture clock. Camera RTCP/PRFT correlations and
camera-clock health are no longer prerequisites for this selected timeline.
Network/camera buffering can separate capture time from NVR ingestion time; the
application must not describe ingestion-time coverage as camera-capture coverage.

The runtime proposal must identify the actual media-observation point and preserve
its association with selected media. Do not call output/progress notification time
socket-arrival time. Process Run bookends, file creation, filename seconds, mux
completion and duration/PTS continuity alone do not establish ingestion bounds.
Existing filename/CSV declarations and Run bookends are not retroactively certified.

## Implementation boundaries

Use supported, unmodified FFmpeg interfaces. No custom build, new camera connection,
additional media process, new probe, container change or extra lease is authorized
by this policy. Changes to runtime flags, I/O, ownership or persistence need separate
review and exact-content approval before push.

Preserve source presentation/decode relationships and playable stream-copy output.
Do not enable use_wallclock_as_timestamps merely because its name matches the
selected policy. Qualify timestamp rebasing, segmentation, capture duration and A/V
behavior first. Retaining NVR-clock observations without rewriting media timestamps
is another route to investigate, not an implemented or qualified solution here.

Keep requested interval, ordered prebuffer, capture/session and resulting owned-file
identities bound through assembly. Missing observations, unsupported clock jumps,
session resets, timing gaps or ambiguous output correspondence cannot authorize
cleanup. Do not invent coverage slack or a successful receipt to bypass missing data.
Clock uncertainty/transition handling must be explicit before positive authorization.

Preserve publication, persistence-failure handling, unconditional event-staging
retention, descriptor ownership, leases, slot return, probe counts/budgets and
interrupt/grace/kill/join teardown. Cleanup authorization remains disabled. No merge
or deployment is included.

## Reproducible qualification fixture

Run manually with stock ffmpeg/ffprobe and libx264/AAC available:

    python3 tests/qualify_ingestion_time.py ingestion-qualification.json

This developer tool generates three seconds of H.264 B-frames and AAC and sends
it over loopback UDP/MPEG-TS with deliberately non-UTC source timestamps. It compares
an ordinary segmenter with the CANDIDATE combination of copyts and
use_wallclock_as_timestamps=1, using reset_timestamps=1. It uses temporary files and
bounded child-process waits. These extra media processes exist only in the developer
fixture, never in application runtime. It changes no workflow or runtime arguments.

Exit 0 means only the listed limited checks passed; 1 means candidate unqualified;
2 means fixture/infrastructure failure. The tool always reports runtime_qualified
as false. An unqualified result must not be presented as a passing application test.

On local FFmpeg 8.0.1-3ubuntu2, the ordinary control retained all 30 video packets
and nonzero composition offsets. The candidate retained 30 packets but reduced the
observed PTS-DTS offsets to zero, emitted non-monotonic audio DTS warnings, and left
the first segment CSV start at zero. The candidate therefore failed qualification.
This is a counterexample to that tested flag combination, not a proof that every
stock-toolchain ingestion-time approach is unsuitable.

This fixture is not RTSP, a real camera, deployed-version acceptance, decoded-frame
comparison, AAC timing verification, capture -t qualification, clock-step testing,
uncertainty analysis or assembled-output correspondence. It does not certify UTC
accuracy or requested-interval coverage. No Go/race/browser suite was run for this
local experiment; application acceptance remains outstanding for runtime changes.

FFmpeg interface references:
https://ffmpeg.org/ffmpeg-formats.html (use_wallclock_as_timestamps, reset_timestamps)
https://ffmpeg.org/ffmpeg.html (copyts and muxer timestamp-processing caveats)
