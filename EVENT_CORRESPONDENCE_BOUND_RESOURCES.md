# Native full-adapter load qualification: prepared candidate

Base: cb94435da04299d90f43d3f047b6b1dad3fd055d, user-reported green. No local Go execution or new-head CI acceptance is claimed. This is test-only; no runtime launch site, limits, workflow, dependencies, cleanup eligibility or main changes.

## Purpose

Measure the entire admitted streaming adapter, including initial binding and synchronous revalidation, rather than only the collector. Use native H.264/AAC clips through real job setup, owned capture registration, stock FFmpeg assembly/publication and stock FFprobe combined evidence. Never assign a successful receipt or bypass a refused binding. The only executable substitution copies a generated native clip to the owned capture descriptor when the production capture command requests RTSP; other FFmpeg commands exec the saved native binary. This is not a real camera or real-time capture test.

## Narrow initial matrix

No prebuffer in this batch. Use 14s 360p/1 Mbps and 110s 720p/2 Mbps clips, each with idle and paced private segment-writer conditions, each repeated twice: eight observations. Generation uses zero B-frames for this initial native full-path compatibility fixture; these rows must not be treated as a strict apples-to-apples continuation of the earlier B-frame collector-only measurements. Existing native reorder/omission fixtures and 1080p ceiling refusal tests remain unchanged and required.

The background writer uses stock FFmpeg, paced input, stream-copy and one-second MPEG-TS segments in a private temporary directory outside the job inventory. It has a 45-second context, a 40-second media limit and existing two-second WaitDelay. Readiness requires a second nonempty segment. It must still be running before and after each full adapter invocation; all background work is joined. File sizes before/after are coarse progress observations, not proof of frame completeness, latency or recording-impact thresholds. A short measurement may span no segment-size increase.

## Accounting and evidence

The existing resource job discovers TestCorrResourceBoundAdapterLoad; it writes bound-adapter.json beside measurements.json, inside the existing artifact directory. The workflow and original paired measurements remain untouched. Ordinary core tests compile the harness and run its profile-definition check; expensive native measurements run only with the existing resource opt-in, without race instrumentation. Existing controlled adapter/admission tests continue to run under the core race suite.

AdapterSeconds measures gate entry through return, including evidence checks and teardown; CollectorSeconds is the narrower collector report. Record child/Go CPU and kernel block-operation deltas, owned lifecycle counts, background liveness and coarse output sizes. RUSAGE_CHILDREN excludes the background child while it remains unreaped; background lifetime CPU is reported separately after joining, not claimed as verification-only CPU. Parent CPU includes measurement/process bookkeeping. No per-comparison RSS or physical-device-byte claim is made.

Whole-file capture/output hashes are test assertions outside timing and warm the cache; they create no packet hashes or durable receipts. Check staging snapshots and held ownership after each call, then require normal inventory/lease/slot teardown and retained files. Refusal on these two compatible in-budget fixtures fails the test; errors are not converted into accepted performance rows. Completed observations and failed-profile names are retained on subtest failures. Hard runner termination may prevent report writing.

## Acceptance boundaries

RuntimeQualified and CleanupAuthorized stay false. Success would establish this narrow native no-prebuffer fixture and synthetic I/O overlap, not full prebuffer compatibility, all codecs, B-frame compatibility of the full pipeline, capture wall-clock coverage, recorder-host performance, cold-cache behavior, hard filesystem deadlines or positive cleanup proof. Existing five-second admitted adapter context and media limits are unchanged. Background recording is synthetic file playback, not a running SimpleNVR recorder. No automatic latency pass/fail threshold or production resource acceptance is inferred from two fixed-order repeats.
