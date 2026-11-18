# Native selected-prebuffer qualification — prepared candidate

Base: 24f16a34b0dc3f416097d408dfdbf5a8a56f54d7, green by user report.

## Milestone

Move from synthetic child/admission permutations to native encoded selected
prefixes through TriggerEvent pinning, capture registration, stock assembly,
durable publication, FFprobe observations and the test-only admitted comparator.
No successful receipt is assigned directly. Positive controls must pass every
prerequisite and match; comparator-negative controls must reach comparison.

## Native matrix

Generated clips use H264/AAC at 128x72, 10 fps, no B frames. Distinct prefix
clips use different hue and tone content. Two 1.6s prefixes and a 6.4s capture
align video frames and 1024-sample AAC packet periods at 48kHz; the synthetic
audio input has the same explicit priming offset as the corrected native
resource fixture. Event requests are 6 seconds. These are controlled fixture
choices, not production timestamp compensation or compatibility guarantees.

- One/two selected prefixes, idle/paced private segment writer: four positive
  cases, each compared twice (eight rows).
- Omit first, omit second, duplicate first, reorder: four native wrong-output
  cases, each compared twice (eight rows), requiring content-mismatch refusal.
- Capture-only and shortened capture: two necessary-bound refusal controls.
- Require 18 report rows and retain failed-case names.

Only camera capture is substituted for positives; assembly uses production
arguments. Negative controls deliberately substitute the native assembly input
plan while leaving the job's frozen expected plan unchanged. This models a
wrong assembler output, not an undetected in-process plan mutation. Any negative
control rejected earlier than its specified layer fails the test.

Segment completion notifications are simulated using the session notice API,
with coherent declared ranges and ordered receipts. They are NOT real packet
arrival times. No synthetic post-hoc coarse-timeline patch or successful media
receipt is inserted. All cases retain native inputs and published output through
normal job release. Comparison asserts joined children/workers, admission reuse,
held lease, unchanged staging and unchanged capture/output hashes.

## Evidence and execution

The existing resource job discovers TestCorrResourceNativePrebufferQualification
and uploads native-prebuffer.json beside its other reports, including on failure.
Failures also log native presentation boundaries using the existing diagnostic
helper. Native test generation is bounded; no new dependency or workflow change.
These elapsed measurements are observations, not a recorder impact benchmark.
Other existing checks remain required. CI has not validated this candidate.

## Acceptance still pending

No production launch, budget change, cleanup authorization, merge or deployment.
RuntimeQualified and CleanupAuthorized remain false. Positive native prefix
compatibility and layer-specific negative results remain pending CI. B frames,
other codecs, native camera timing, cold cache and recorder-host cost remain
unqualified. Review bound-adapter.json from the green native resource run before
accepting full-adapter cost; green status alone is not numerical acceptance.

Available prior paired measurements (16edded) showed the streaming collector at
about 0.079s for 360p and 0.260s for 720p versus scratch at 0.237-0.240s and
0.480-0.486s; these are cache-warm collector-only comparisons, not full-adapter
or live-host budgets. Zero kernel input block operations do not prove zero
logical reads. No new full-adapter resource numbers have been reviewed.
