# Complete validation and native refusal correction — combined candidate

Base: 4ebb4d6a6e842c901be771fe14a2a7324d44efde, FAILED by user report.
The previous unpushed three-file composition candidate is included here, together
with its track assertion correction and dedicated regression coverage. This
combined payload supersedes the previous composition-only push payload.

## Missing-audio failure diagnosis and correction

The supplied CI log contains a genuine stream-content-mismatch sentinel, joined
with child exits/kills and context cancellation after roughly 53ms. Two children,
comparators, watcher and reader workers were joined. The test incorrectly treated
ANY joined context.Canceled as external cancellation and rejected a valid refusal.
This is an assertion correction, not evidence that missing audio was accepted.

Use the live manager context as the native comparison caller. Require it to stay
live and require a recognized content/empty-stream/process-refusal sentinel.
Allow context.Canceled joined during internal teardown only alongside such a
semantic refusal. Still reject nil, bare cancellation, deadline errors (even
joined with mismatch), stopped/expired callers, unrelated errors and matching
error text without the actual sentinel. Never accept missing audio as a match.
Keep two-child/pipe and full lifecycle assertions, both native repetitions, gate
reuse, evidence preservation and the ten-row completeness check unchanged.

Regression coverage includes twelve deterministic error-tree/caller cases plus
a controlled two-child empty-output refusal followed by a successful retry.
No comparator cancellation, error joining or cleanup behavior is changed.
The nine-of-ten-row error was secondary to the first-repeat assertion failure.

## Complete post-publication validation composition

The reviewed b4daa50 numbers measured comparison AFTER evidence existed; user
accepted those CI-fixture costs as enough to continue development, not production
resource acceptance. Include fresh duration/presentation collection plus comparison
under one slot and a five-second candidate aggregate deadline. Internal deadlines
may shorten, never renew, that context. Fresh genuine observations remain local;
never assign or replace borrowed.duration. This is test-only composition.

Controlled tests cover admission throughout collection, shared/shorter deadlines,
evidence errors and cancellation, absent/incomplete receipts, no comparator launch
on refusal, unchanged stored evidence and successful admission reuse afterward.
Native tests use 14s 360p and 110s 720p fixtures (requests 13s/109s), idle/paced
writer, two repeats: eight required rows in complete-validation.json. Measure
evidence/comparison/total elapsed and child/Go CPU across both stages. Assert
joined comparison work, ownership/media/staging preservation and terminal closure.

Capture, assembly and publication are OUTSIDE these measurements. Publication
already collected evidence once; reinspection is deliberate extra BENCHMARK work,
not added production probes. Comparison lifecycle counts do not count FFprobe
children; their joins remain enforced by existing production probe code. No
fabricated observed probe count is introduced. Existing assessment checks its
existing matrices, not these new complete-validation rows; the Go assertions gate
them. All existing selectors, workflow requirements and limits remain unchanged.

## Scope

Five files: one existing track test modified; three new Go test files and this
document. No production source, dependencies, workflow, runtime flag, budget,
cleanup authority, main, merge or deployment changes. Runtime qualification and
cleanup authorization remain false. The combined five-second context is only a
candidate being tested, not approved runtime policy. Native refusals fail rather
than silently expanding budgets. Intended-host cost and ingestion-time coverage
remain unqualified. Nothing is pushed or CI-validated by preparing this payload.
