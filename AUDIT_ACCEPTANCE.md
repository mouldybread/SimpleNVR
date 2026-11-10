# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported CI is green through 26a7bc0. Its preparation ran full-script Node syntax plus new failure/updated lifecycle stub suites in UTC and America/Chicago, not Go or browser integration. This recovery-inspection batch has not run Go compilation, vet, race tests or live-system validation. Re-record evidence for each exact committed SHA; never carry a prior green result forward.

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
