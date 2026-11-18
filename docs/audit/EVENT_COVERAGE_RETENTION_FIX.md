# Retention-only checkpoint regression in 2503e86

The provided core Actions log fails TestEventStagingHTTPAdmissionBatch/disabled
and /below_both_limits at event_staging_test.go:111: retention changed processing
evidence. This is a production checkpoint-semantics regression, not justification
to weaken the existing test or dismiss it as a stale expectation.

Production job setup writes pinned before creating the execution/selection record.
Its pinned checkpoint therefore has no coverage assessment. 2503e86 recomputes
coverage unconditionally, including during retained, so an accepted but unprocessed
job unexpectedly gains an assessment when released. The same unconditional refresh
can replace a processed job's stored assessment with unavailable diagnostics after
manager cancellation. Retention must preserve that historical processing evidence.

The fix snapshots coverage only for processing checkpoints, not retained. Nil stays
nil for unprocessed jobs; the last processing assessment remains unchanged for
processed jobs, even after cancellation. Existing retained revision/ObservedAt and
pinned-count behavior stays intact. ObservedAt on a retained checkpoint is the
retention-write observation; it does NOT mean coverage was reassessed at that time.
Stored diagnostics remain unverified and never authorize cleanup.

The new regression uses real pinning/checkpoint persistence for nil and non-nil
assessment cases, cancels the manager, compares the entire persisted manifest except
the established revision/ObservedAt updates, checks strict readback, and verifies
terminal descriptor closure, slot return and staging retention. The original HTTP
admission comparison is unchanged and remains a required regression gate.

No FFmpeg flags/media path, checkpoint schema, probe/lease/descriptor budget,
publication path, failure semantics or cleanup eligibility change. No merge,
deployment or deletion is authorized. Application acceptance remains exact-head
Actions core/race/vet/storage/lifecycle and both isolated Chrome runs.

Tests are prepared, not reported as executed. The supplied log is failure evidence,
not a successful rerun. This batch only addresses the identified retention regression.
