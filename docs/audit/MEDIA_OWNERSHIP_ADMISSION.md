# Media ownership admission: first bounded implementation

Baseline: `337cdd0165dbbdca385d8379971dcc9bca5df147` on
`fix/recording-safety-audit`. This is not completion of the ownership stage.

## Implemented boundary

A process-wide, capacity-one gate admits managed retention passes. Admission
waits directly on a channel and context cancellation; it launches no helper
goroutine. It checks cancellation before waiting and again after obtaining the
token. Release is idempotent. The acquiring layer releases its lease with defer;
leased helpers must not reacquire the non-reentrant gate or release a borrowed
lease. Lease objects must not be copied or released concurrently with their use.

Managed retention acquires ownership before its retention-policy read lock and
brief registry check. After admission it revalidates cancellation, shutdown,
registry pointer, camera ID, recording path, retention days and derived duration.
NVR.mu is not held across ownership waits or the deletion callback. The policy
read lock and media lease remain held through the callback and compatibility
temp-cleanup call. Policy writers continue to exclude the deletion phase.

The intended adoption order is recordingControlMu, when needed, then media
ownership, retentionPolicyMu, when needed, and brief NVR.mu checks. Camera
deletion must cancel and join workers before media admission. Workers must never
wait for recordingControlMu while holding media ownership. Capture and FFmpeg
process lifetimes must remain outside media ownership. Only retention is wired
in this commit; these other ordering rules are requirements for later adopters,
not claims that those paths have already been converted.

## Unchanged and still open

Publication, published checkpoints, staging release, explicit recording deletion,
camera deletion and orphan cleanup do not yet share this gate. Direct calls to
cleanupOldRecordingsContext also remain outside managed-retention admission.
Therefore this commit does not close the publication/deletion race, establish
output-backed staging cleanup eligibility, or implement A07 as a whole.

The existing recursive releaseJob cleanup remains unresolved. No staging-release
capability is added here. Creation-time entry ownership, nonrecursive release,
checkpoint restoration after partial release, descriptor-relative published
unlinking and orphan/registration reconciliation are required follow-up work.
No deletion scope, expiry policy, recovery behavior or existing fixture is
broadened or weakened by this commit.

A lease is application coordination, not filesystem ownership proof. It does
not solve hostile external writers, compare-and-unlink atomicity, ancestor or
mount identity, uninterruptible I/O or power-loss behavior. Cancellation does
not interrupt an already-running callback or the existing policy-lock wait;
callbacks must continue to honor their context. Shutdown is rechecked at
admission, not used as a standalone wakeup signal for an uncanceled waiter.

## Authored coverage and validation limits

Eight new test functions cover cancellation while blocked, cancellation at the
post-admission boundary, mutual exclusion, idempotent release, lock availability
while waiting, post-admission identity/path/policy/shutdown revalidation, registry
availability and policy exclusion during slow injected I/O, invalid leases and
the production managed-retention entry point. Revalidation has six subcases.
Ordering uses channels; timeouts are failure guards, not ordering evidence.

Existing retention, staging and failure-preservation tests are unchanged. New
coverage partially addresses A09 and A10; A07 and A08 remain open. A01-A06 and
A11 receive no new acceptance coverage in this batch. A12 remains pending.

Preparation checks include original Git blob verification, exact JSON round trip,
Git patch application/reversal and whitespace inspection. No Go, gofmt, race,
vet, Docker, browser, active-recorder, SIGTERM or restart execution is claimed.
No CI result for this candidate has been inspected. This is not merge readiness.
