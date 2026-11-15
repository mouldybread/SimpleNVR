# Disabled ordered-content prototype: safety-test candidate

Base: a7245b3d06eb2c993a6978957cf0973f3e6a829f, user-reported green.
Candidate only: no repository write, production integration, merge or deployment.
All four Go files end in _test.go. The ordinary application build excludes them.

## What this batch adds

The prior experimental adapter now has a synchronous normalizer injection seam for deterministic live-job boundary faults. The non-injected wrapper still uses the actual stock-FFmpeg normalizer. Injection is never evidence of media equality. The adapter now explicitly refuses manager cancellation between passes; byte comparison checks caller cancellation before filesystem inspection. PID reporting is test-only and used to verify that real child processes have been reaped, not as reusable process identity.

## Live binding matrix

Fourteen faults at five boundaries (before entry and after each of four normalizer calls): selected descriptor replacement, selected bytes changed, selected pathname replaced, output pathname replaced, concat bytes changed, capture bytes changed, output bytes changed, output descriptor binding changed, requested duration changed, job inventory binding changed, borrowed lease binding changed, real lease released, manager cancelled, caller cancelled.

Each case expects refusal at the injecting boundary with no later normalizer call. Tests snapshot evidence AFTER their intentional mutation and require the adapter to leave it unchanged, preserve published bytes except where deliberately altered by the test, retain the lease unless the fault explicitly releases it, and close/remove only private scratch files. Teardown checks staging retention, closed inventory, released ownership and empty job slots. Pointer/request changes are restored by the test owner, never by the adapter.

## Other prepared checks

Success controls with and without selected prebuffer; an invalid initial binding followed by restoration of genuine observations; exact same child context/deadline across all four passes; expected process/output/comparison/logical-size/descriptor counters. Mid-collection errors and cancellation at all four boundaries, plus independent video/audio mismatch refusal, verify scratch descriptors close and artifacts disappear without touching recordings.

Real shell children cover successful payload, zero-exit diagnostics, nonzero exit, timeout using exec sleep, excessive stdout, excessive stderr and empty output. Wait4 must report ECHILD after the normalizer returns. Admission checks cover cancelled context, exhausted process budget, missing executable and closed donor before successful spawn. These controlled children use exec rather than leaving descendant processes behind; arbitrary descendant/process-group containment is NOT established.

Retained unit tests exercise exact bytes, same-length reorder, late differences, empty/oversized content, unchanged donor offsets, sink limits, command construction and absent binding. Native stock-FFmpeg tests exercise normal, omitted, capture-only, duplicated, reordered, truncated, video-only and audio-only reorder, and timestamp-shift content comparisons. Native tests skip only if ffmpeg is missing; unavailable codec/filter support fails. This fixture does not independently establish timestamp-shift magnitude or B-frame offsets; those were explicitly checked by the earlier Python experiment.

## Costs and limitations remain explicit

Four sequential additional FFmpeg children on the successful adapter path. Experimental 20-second context plus up to two seconds pipe-join grace, not a change to production's two-probe/five-second contract. Two 64 KiB comparison buffers; normalized output limited to 32 MiB per pass, two scratch descriptors at a time, four scratch files over the run. Media input size limit 128 MiB per descriptor, logical size/pass budget 512 MiB. Logical file sizes are NOT measured physical I/O or a hard read-byte limit inside the demuxer. Reported descriptor counts exclude transient inventory scans and os/exec pipes. Filesystem calls can block despite context checks.

Private scratch directory and owner-serialized invocation are prerequisites. Selected inputs plus capture and output are borrowed from existing bound witnesses. The adapter never opens media by mutable source names and never closes donor descriptors itself. Scratch cleanup removes only files it created. No per-packet hashes, decoding pass or persistent checkpoint fields are added. H264/AAC normalization is a narrow extraction profile, not comprehensive codec/topology qualification. AAC ordinal timestamps exist only on disposable content output, never recording or timing evidence.

## Validation and acceptance

These are prepared Go tests, not passing test results. The earlier native/Python experiment and descriptor-command smoke are separate evidence, not acceptance of this Go implementation. Exact-head CI must compile and run the core/race tests, native matrix and both isolated Chrome checks after a separately approved push. This batch does not modify workflows. No full camera, deployment filesystem, hard deadline, production resource fit, timestamp mapping, source identity, positive coverage or cleanup acceptance is claimed.

Keep the prototype outside production finalization/revalidation and cleanup eligibility. Keep main unchanged, staging retained, and cleanup disabled. Any runtime resource-policy change needs separate review.
