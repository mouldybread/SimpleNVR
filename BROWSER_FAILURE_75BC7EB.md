# 75bc7eb browser failure: bounded diagnostic follow-up

Base: 75bc7eba2f40c08d2a34725324d57911eb637bce. The user reported CI failure at the first native-playback wait on November 11, 2026. The branch head and complete browser driver were re-read before preparation. This base is failed, not green.

The supplied traceback establishes that the driver reached the first native-playback assertion. Login, application camera loading, date selection and the preceding timeline predicate therefore returned. The earlier wget connection-refused line is not the eventual failing assertion; the shell readiness loop completed sufficiently to launch the browser driver. The excerpt does not establish the native video error, private-media HTTP status, play rejection or grid state at failure.

A source-level harness synchronization gap is visible: application_ready waited for matching timeline data and one video DOM element, but not isUpdatingGrid becoming false or timeline request/timer completion. The candidate adds an observable grid/load barrier before the scripted seek. This is a defensible readiness correction, not proof that a grid race caused the reported timeout. No production playback source is overwritten; its connector response was truncated.

The same focused driver change adds:

- A real authenticated bytes=0-11 private-media preflight, requiring 206, video/mp4 and the expected MP4 prefix. This fails distinctly if private-media access or serving is wrong. It does not replace the browser request or certify a decodable file.
- Before-seek and failure snapshots of application intent/generation/session state, selected segments, current source, native ready/network state, paused/muted state, time/duration, media-error code/message and reported MP4/H264 capability.
- Bounded page-error and media/timeline HTTP-failure/status history, excluding credential headers, request bodies and query strings. Diagnostics are printed in CI as well as retained in the existing report artifact.
- Early failure on a native media error while retaining the original successful playback conditions and decoded-frame assertions. A decoded-frame callback now has its own five-second bound.

It does not increase the 30-second playback wait, force autoplay permission, override play(), stub events, mock routes, bypass authentication or count unavailable codec support as a pass. All subsequent export/browser checks remain enabled. If the next run still fails, the evidence should distinguish missing/wrong private-media serving, browser decode support, failed play intent and source/seek lifecycle problems without another blind timeout.

Preparation verifies the original driver's Git blob, parses the modified Python and embedded JavaScript expressions, and checks an exact zero-fuzz patch apply/reverse. No Docker/Go/full application rerun is claimed. This is an unexecuted diagnostic/readiness candidate, not an established root-cause fix or acceptance closure. The runtime-browser-report.json and runtime-container.log from the failed run remain useful original evidence; preserve them and do not replace them with a later result. main remains untouched and the audit branch unmerged.
