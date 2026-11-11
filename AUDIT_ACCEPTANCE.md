# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported Safety verification is green through bdc1ef587fe516c5c5d1104055c86b3f04643ce7 (bdc1ef5), including the required private-tmpfs step. Actions logs were not independently inspected. Earlier reported-green milestones include recovery inspection (1cff006), publication (607bf98), HTTPS exports (117ac28), forced render cancellation (f0c2cb7), initial storage failures (2074785), queued checkpoint preservation (45a6083), active-recorder/full-server shutdown (9137668) and runtime storage/non-root failure cases (bdc1ef5).

Historical preparation-time nonexecution remains historical. The 26a7bc0 preparation ran Node syntax and stub suites in UTC and America/Chicago, not Go or browser integration. The 45a6083, 9137668 and bdc1ef5 preparations did not run local Go compilation/vet/race tests; subsequent CI green was user-reported. Focused docs retain the exact native prototype scope.

The export-phase batch adds authenticated duration/profile probe cancellation and input-paced, output-producing trim cancellation without SIGSTOP, including full fixture HTTP-server shutdown. Probe fixtures exec real ffprobe with an intentionally blocked pipe, not a stalled regular recording filesystem. Local preparation exercised the native shims, not the Go suite. Go/gofmt remain unavailable; record CI for the resulting exact SHA.

Local headless Chromium 134.0.6998.35 executed six controlled checks against the complete, hash-verified committed export UI and geometry scripts: nine-camera rejection, error-text safety, response-length cap, prepared cancellation, browser-managed download/save/hash/full external decode and DST helper behavior. See EXPORT_PHASE_VALIDATION.md and tests/evidence/export-browser-bdc1ef5.json. These use routed responses and a controlled DOM, not the full authenticated application, browser media playback/decoding or end-user save dialogs. Broader browser gates remain unchecked.

Earlier server=nil fixtures now have additional non-nil fixture-server coverage; runtime-image SIGTERM, live RTSP interruption/reconnect and real startup resume remain unverified. Tmpfs is not deployment storage or power-loss evidence. Lost mounts/fallback writes, blocked filesystem I/O, deployment no-replace/procfs compatibility and runtime quota policy remain unresolved. No inspection/checkpoint, readable partial or passing CI authorizes evidence deletion, expiry or reassembly. Record final-candidate evidence and complete whole-branch review before proposing merge.

## Required before proposing a merge

- [ ] Exact candidate commit passes go vet ./... and go test -race -count=1 -timeout=120s ./..., with required media/frontend dependencies present rather than silently skipped.
- [ ] Recovery manifest writer/strict read-only reader integration is reviewed together; incomplete, unsupported and changed observations preserve uncertainty and never authorize deletion or reassembly.
- [ ] Runtime failure behavior is tested for exhausted bytes/inodes, permission failures, unavailable mounts and failed capture/assembly/publication/persistence. Admission observations are not accepted as runtime reservations or a full disk-full policy.
- [ ] Live Dahua/RTSP disconnect/reconnect and interrupted continuous/event capture/assembly are validated with retained evidence inspected.
- [ ] Active-recorder SIGTERM and in-flight export/inspection shutdown are tested; cancellation and joining precede final persistence, and timeout reports failure.
- [ ] Authenticated real-FFmpeg export rendering/streaming is tested with slow clients, disconnects, cancellation and shutdown.
- [ ] Real-browser playback covers late media errors, autoplay denial, explicit retry, paused-ended resume and metadata delay. Monitored-download tests distinguish browser handoff from actual disk saving.
- [ ] Final whole-branch review covers destructive paths, lock order/long-held locks, resource bounds, authentication, frontend text sinks, proxy/SSRF boundaries, media publication and persistence ordering.
- [ ] Older lifecycle/timeline/export summaries are reconciled with scoped source behavior and focused documentation; no completed work is counted twice and no validation gap is described as solved.

## Explicit scope decisions, not automatic feature additions

Audio-aware trimming/broader media profiles, synchronized multi-camera advancement, next-day fetching, gapless transitions, a storage/recovery UI and legacy event-ID migration can be deferred if original acceptance criteria permit. Automatic recovery, staging expiry and evidence deletion are not implicitly authorized by this checklist. Any future recovery write needs separate validated semantics and explicit approval.
