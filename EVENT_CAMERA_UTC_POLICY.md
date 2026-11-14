# Camera-capture UTC policy and implementation review

Decision: the user selected camera-capture UTC (option 1).
Audit base: 01b98d15e9adfaa7125e9e8cd437145b02cd1f01.
Base green is user-reported, not independently retrieved Actions evidence.
Main verified unchanged at c5be6bf0cfea03071c7df1c3b0f5064753c881dd.

Status: DOCUMENTATION ONLY. Camera-to-UTC mapping is NOT implemented by this
commit. This record does not add a runtime evidence reader, coverage gate or cleanup
permit. Existing publication and unconditional staging retention remain unchanged.

## Binding policy for future positive coverage authorization

The requested start_time refers to the camera capture timeline expressed in UTC,
NOT recorder ingestion time, process start, file creation, filename time or mux time.
Missing, untrusted, discontinuous or ambiguous mapping must leave cleanup ineligible.
No duration, contiguous-PTS observation, successful process, input-plan receipt,
NTP-shaped field or PRFT-box presence alone provides that mapping or authorization.
Existing publication and unconditional staging retention must continue independently.
This is the selected evidence policy, not a claim that a positive UTC verifier exists.

## Verified local feasibility finding

Local FFmpeg 8.0.1-3ubuntu2 was exercised with generated lavfi MPEG-4, with NO camera,
RTSP or RTCP producer reference. Current-style faststart without PRFT produced no
PRFT box. Faststart with write_prft wallclock also produced no box in this local
case. Fragmented MP4 with write_prft wallclock produced one PRFT box, flags 24,
correlated to host generation time. Fragmented MP4 with write_prft pts also produced
one PRFT box with flags 24. These are offline muxer experiments, NOT application,
Go, camera, RTSP or deployment acceptance tests. They establish why neither a box
nor these flags alone distinguish trusted capture UTC. The diagnostic host-generation
comparison window is not a proposed coverage tolerance.

The inspected FFmpeg MOV source uses packet producer-reference time when available
in wallclock mode, otherwise falling back to current host NTP time. The inspected RTP
source can derive packet producer-reference time from RTCP sender correlations.
Those paths must not be conflated. No deployed-version guarantee follows from local
8.0.1 or current upstream source. The current Dockerfile installs ffmpeg from
alpine:latest without a fixed FFmpeg version; deployment compatibility remains open.

## Minimum evidence contract before a positive mapping path

- Identify the approved camera producer, selected stream/track, session/generation
  and timestamp time base. A relay's clock is not silently the camera's clock.
- Retain producer-media timestamp to claimed UTC correlation, including its origin,
  BEFORE timestamp reset/rebasing, linked explicitly to the resulting owned file.
- Establish clock accuracy AND the producer timestamp's meaning relative to camera
  capture. A sender correlation is not clock-health verification or a guarantee
  about sensor/encoder delay.
- Retain explicit uncertainty bounds and their provenance. Do not invent a slack
  percentage, reuse filename second precision as accuracy, or accept a client trust bit.
- Refuse unsupported session resets, clock steps, stale/missing anchors, timestamp
  wrap and discontinuities. Do not extrapolate across unsupported transitions.
- Bind the observations to the existing frozen request, selected prebuffer, capture,
  execution, inventory and output identities, with failure-preserving revalidation.

## Runtime implementation scope requiring review

First demonstrate an extraction/preservation route in the EXISTING ingest/segmenter/
capture processes that retains real producer-origin correlations and makes host
fallback impossible to mistake for source data. Selecting this policy does not itself
authorize turning on write_prft, switching to host wall-clock timestamps, changing to
fragmented MP4, replacing capture commands, adding a camera connection/probe or
adding leases. Behavior/container/schema changes require explicit review and fresh
exact-content approval before any push.

Only after that route is demonstrated should the runtime patch integrate anchor
observations. Missing/untrusted data is an explicit refusal, not a manufactured
successful receipt. Existing necessary evidence checks remain distinct from future
positive cleanup eligibility; they must not falsely certify requested coverage.

Then compose the selected prebuffer and capture on established camera-UTC intervals
and establish assembled-output correspondence. Preserve the exact ordered input
plan, same-lease ownership, independent metadata budgets, output/probe caps, terminal
teardown and slot return. Keep cleanup disabled until the full proof and explicit
partial-release/restoration handling are integrated and audited.

## Acceptance tests required for runtime work

Cover real producer correlation; host-generated PRFT fallback; synthetic PTS-based
PRFT; missing/zero/stale/inconsistent anchors; wrong producer/track/session; proxy-
clock substitution; RTP wrapping and reset; clock steps/drift; time-base conversion
and rebasing; prebuffer/capture boundary gaps; uncertainty-bound insufficiency;
shifted requests; output correspondence; cancellation; failed capture/checkpoint/
publication; normal deferred cancellation; no retries/additional media processes;
lease/descriptor/slot retention; both isolated Chrome runs and aggregate checks.
Synthetic successes must remain distinguishable from camera/deployment acceptance.

## Scope and sources

No runtime code, production media arguments, process counts, leases, descriptors,
publication/persistence behavior, retention, schema, workflow or HTTP behavior is
changed by this document. No Go tests were run for the local feasibility work.
No merge, deployment or cleanup authorization is included.

Sources inspected:
https://ffmpeg.org/doxygen/6.1/rtpdec_8c_source.html
https://www.ffmpeg.org/doxygen/trunk/rtpdec_8c_source.html
https://www.ffmpeg.org/doxygen/trunk/movenc_8c_source.html
https://github.com/mouldybread/SimpleNVR/blob/01b98d15e9adfaa7125e9e8cd437145b02cd1f01/Dockerfile
