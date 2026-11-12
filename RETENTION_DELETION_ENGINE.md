# Managed retention engine wiring, subtask 3

Baseline: d02b1dccbd5817b7b1e2acb732b3898e56a53b2c. Only the periodic
registered-camera retention path is converted; standalone compatibility helpers
in cleanup.go and existing tests are unchanged. Explicit deletion is unchanged.

One admission layer acquires media ownership before the policy read lock. The
leased operation passes the existing lease and guard into the descriptor-relative
engine, without reacquiring either lock. The guard checks registry pointer, ID,
path, retention days/derived duration, cancellation and shutdown before engine
filesystem phases. NVR.mu is released before I/O; the policy read lock remains
held until the callback and compatibility temp-cleanup boundary return.

Expiry selection is unchanged: short names remain, valid UTC filename timestamps
win over mtime, invalid prefixes fall back to UTC mtime, and equality is retained.
Published partials remain subject to the same policy. The engine limits selection
to top-level regular .mp4 files; staging/.tmp is never traversed. Policy writers,
legacy zero-day default and valid-day bounds are unchanged. Engine errors stop
the pass within that camera and produce a bounded incomplete-pass log. The bool
return still means the validated operation was invoked, not successful deletion.

Void-callback test seams adapt into the same acquisition/validation path; they
are not the production filesystem callback. Six new test functions cover expiry,
actual managed dispatch/no nested admission, per-phase guard propagation, policy
and registry locks, explicit-deletion ordering, stale policy and refusal paths.
Tests are authored, not locally run: Go/gofmt/Docker unavailable. Preparation
checks are not build/race/vet/CI evidence. The policy-lock wait and kernel calls
are not made interruptible; media admission remains context-cancelable.

Publication/checkpoint/staging-release and orphan cleanup coordination remain
open. No recursive staging cleanup fix, transaction, hostile-writer protection,
ancestor/mount proof, power-loss proof or merge readiness is claimed.
