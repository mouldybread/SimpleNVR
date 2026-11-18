# Close-out review and core execution evidence

Prepared on fix/recording-safety-audit against eb04f54610f435c45c5f529afa8ab32da21de5ab.
No production change, new capture/probe behavior, comparator activation or cleanup authority.

## Concrete evidence gap addressed

The existing required core workflow runs go test -race -count=1 -timeout=120s ./...
but uploads only export artifacts from that job. A green report is useful, yet does
not give the artifact reviewer test-by-test recovery execution evidence. This is
an evidence accessibility gap, NOT proof that recovery contracts were skipped.

This candidate runs the SAME full race suite once, adding -json, retaining its
transcript and both pipeline exit statuses. No test selection, count, timeout,
media requirement, isolated storage check, build, vet, Chrome attempt or resource
gate is removed or relaxed. The existing 25-minute job timeout remains.

The read-only assessor requires metadata bound to the expected head, exact command,
successful test/tee exit statuses, completed package/test lifecycles, no failed test
or build events, all seven required top-level recovery contracts and ten named fuzz
seeds passing in the root package. Missing/skipped required contracts, partial
streams, duplicate run/terminal events and contradictory metadata refuse success.
Other explicit skips are counted, not hidden. This does NOT independently assess
vet/build/isolated-storage success or every media dependency/intentional skip.

Metadata is locally recorded CI evidence, not signed provenance. The assessor
refuses input logs above 64 MiB or individual lines above 1 MiB; it emits only a
fixed summary, not raw test output. The transcript itself remains an ordinary CI
test log and should be handled with the same access controls as Actions logs.
The writer is scoped to RUNNER_TEMP, not recordings. An always-run artifact upload
retains available evidence after failure; missing evidence fails rather than passes.

## Focused current-source review (not a whole-branch clearance)

Inspected event_recovery.go, event_recovery_inspection.go,
event_owned_checkpoint.go, all three event_recovery_*_contract_test.go files,
orphan_cleanup.go, retention_deletion_engine.go and safety-ci.yml at the base.

- Recovery producer latches persistence failures and preserves staging. Retention
  checkpointing keeps the processing state and stored coverage. The strict reader
  distinguishes processing failure, publication ambiguity and unverified published
  checkpoints; stored unknown coverage never becomes eligibility.
- Snapshot reads pin an O_PATH identity, reopen through procfs, compare metadata and
  recheck the named checkpoint; changed/canceled/incomplete reads refuse certainty.
  Artifact stats are NOT an atomic cross-artifact snapshot.
- Owned checkpoint writer checks the lease/inventory slot, creates privately and
  exclusively, syncs/closes, renames and syncs the directory, then adopts identity.
  Post-rename failure can leave the new disk checkpoint visible without a successful
  receipt; do NOT claim all failures preserve old checkpoint bytes. Existing injected
  lifecycle writer failures occur before delegation and have narrower semantics.
- Orphan cleanup preserves opaque temp/buffer and root .tmp evidence, rechecks before
  each unlink and never recurses into staging. It remains pathname-based with a
  last-check/unlink race and unbounded directory enumeration; this was already an
  explicit limitation, not a newly demonstrated exploit or authorization to run it.
- Retention's adapter borrows the existing lease/policy guard and uses the shared
  published deletion engine. Reviewing that adapter alone does not qualify the
  engine, caller serialization, camera removal or policy interactions.

No new production defect established by this focused pass. Remaining recorder,
publication/release, destructive engine, lock/security/frontend and deployment
reviews stay open in AUDIT_CLOSEOUT_MATRIX.md. Core logs for eb04f54 have not been
independently inspected; user-reported green and the reviewed export/browser
artifacts are credited separately. New candidate execution is an Actions gate,
not a retroactive receipt for the base run.
