# Native track isolation through bound jobs — prepared candidate

Base: b4daa50a9d92f4aa6ade4dd388c2c9bf788ff441, validation pending.

## Objective

Extend the previous selected-prefix ordering milestone to a different failure
class: one media track is correct while the other is wrong or absent. Existing
raw native fixtures exercise track swaps without full live-job bindings; this
batch requires real capture/publication/probe prerequisites before comparison.

## Controls and matrix

Generate four bounded 360p, 10fps H264/AAC clips: original, different video hue,
different audio tone, both changed. Use 6.4s aligned media and 6s event requests;
no prefix and no B frames. Independent per-track normalization/equality checks
first prove that the intended unchanged track actually matches and the changed
track differs. They run outside measured comparisons and create no owner receipt.
A fifth native video-only clip removes audio by stream-copy mapping.

The normal full job uses production assembly arguments. Negative controls replace
the assembler's output with a prebuilt native clip, while retaining the job's
original capture, frozen plan and publication/evidence paths. This is an explicit
wrong-output injection, not production media behavior or a real RTSP capture.
No successful receipt or post-hoc observation is assigned directly.

All five cases must reach the admitted native comparator, each twice: ten rows.
The original must match both streams. Video/audio/both changes must produce a
content-mismatch sentinel. Missing audio must refuse without a timeout/cancellation;
its exact error classification remains reported rather than assumed. Because a
mismatch cancels concurrent work, the combined report is not used to certify that
the other track finished equal; independent preflight establishes isolation.

Assert child/worker/pipe joining, admission-slot reuse after every comparison,
held lease, same receipt, unchanged job tree and capture/output hashes. Terminal
release must close inventory, return capacity and retain capture and publication.

## Execution and evidence

The existing TestCorrResource selector runs this opt-in native test and uploads
native-track-isolation.json with failed-case names and collection/lifecycle data.
No workflow change is needed. The assessment script at the base still validates
its existing bound/native-prebuffer matrices, not this additional report; the
new test's assertions are the gate for the track matrix. Existing tests remain
required. CI has not validated this candidate.

## Boundaries

No production code, budget, dependency, cleanup, merge or deployment changes.
RuntimeQualified and CleanupAuthorized stay false. These short cache-warm native
controls do not establish ingestion-time coverage, timestamp fidelity, all-codec
compatibility, audio duration correctness or a recorder-host resource budget.
Resource-cost acceptance and numerical review remain separate and pending.
