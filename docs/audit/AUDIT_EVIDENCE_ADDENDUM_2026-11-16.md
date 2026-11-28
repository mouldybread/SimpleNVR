# Current-head evidence and scope addendum — 2026-11-16 23:55 UTC

This addendum records the user-reported frontend CI results and the exact-head
runtime-export failure shared in conversation on 2026-11-16. It supplements the
historical snapshots in AUDIT_ACCEPTANCE.md; it does not rewrite their timestamps,
reports, statuses, or provenance. The GitHub Actions artifacts themselves have not
been independently downloaded or replayed by this addendum.

## Repository identity and recent scope

At preparation, GitHub reported `fix/recording-safety-audit` at
a19b503ba216bfaa36f7e306096709cd2682ed48 and `main` at
c5be6bf0cfea03071c7df1c3b0f5064753c881dd. Keep main untouched and the audit
branch unmerged. No cleanup, metadata clear, camera removal, recording deletion,
recovery, retention expiry, comparator activation, or deployment was performed
while preparing this record.

The user reports green CI for these current-cycle commits:

| Commit | Bounded change | Evidence status |
| --- | --- | --- |
| 26a8127 | Bounded manual orphan-cleanup inspection and conservative limit results | Green by user report; not independently replayed here. |
| 598058c | Truthful orphan-cleanup result reporting in the active frontend | Green by user report. |
| fc8e0bd | Shared toast renderer creates text-only notifications | Green by user report. |
| e7fd171 | Fresh per-attempt confirmation for Delete All Recordings | Green by user report. |
| a2ea79c | Per-control, single-flight orphan-cleanup confirmation | Green by user report. |
| 81e22f0 | Fresh per-attempt confirmation for Remove All Cameras | Green by user report. |
| 926dd03 | Fresh per-attempt confirmation for Clear All Events metadata | Green by user report. |
| a19b503 | Single-component URL encoding/refusal for live-preview camera names | Green by user report. |

These are separate focused frontend batches. They do not close their separate
regression scopes until exact-head CI artifacts are independently inspected, and
they do not establish full UI/browser, backend, live-proxy, or security acceptance.
The client confirmation states are usability safeguards, not server-side authorization,
CSRF proof, cross-tab idempotency, or deletion permission.

## Failed runtime-export campaign at 926dd03

The user-provided output identifies the command as
`bash tests/acceptance/runtime-export-acceptance.sh simplenvr-safety-test "$RUNNER_TEMP/simplenvr-export-evidence"`.

| Campaign case | Reported state |
| --- | --- |
| Whole-copy slow reader | Passed. |
| Disconnect and both-admissions reuse | Failed after the report recorded a simultaneous two-child pressure witness. |
| SIGTERM under stream backpressure | Not started. |
| SIGTERM during preheader trim | Not started. |
| Campaign result | Failed; exit code 1. |

The report records the bounded synthetic observation at failure. It is not evidence
that those resources persisted after disconnection. The failure summary names only
`fixture command failed: docker`; it does not include Docker stderr, daemon state,
or the exact failed subcommand. In that case the driver operation after the recorded
pressure witness is `await_clean()` and its `docker exec ... PROBE`.

Commit 5fde2d1 changes a test-only observer: if a final child-identity read fails,
it omits the completion record instead of aborting the whole probe. Ten deterministic
tests passed locally against the complete observer and inspected campaign-predicate
excerpts. The user reports 5fde2d1 passed. The corrected Docker campaign has not been
reported in the conversation here as rerun/passed, so 926dd03 remains failed evidence
and the corrected runtime campaign remains an open gate. The observer-race mechanism
is reproducible, but it is not confirmed as the cause of this specific Docker failure.
See RUNTIME_EXPORT_THREAD_OBSERVER_FIX.md.

## Current state: implemented vs. still open

The bounded frontend fixes above are implemented in branch history. The export-
observer correction is also implemented. This is not a whole-branch source review or
exact-head verification statement.

Still required before any merge-readiness decision:

- Inspect exact-head CI artifacts for the recent user-reported frontend greens and
  run the corrected runtime-export campaign at the final candidate head. Keep the
  926dd03 failure and its not-started cases as historical evidence.
- Review exact-head `go vet`, full core tests, race tests, storage/lifecycle jobs,
  both isolated Chrome attempts, correspondence resources, and runtime-resource
  evidence with matching commit identity. Distinguish skipped jobs/tests from passes.
- Complete whole-branch interactions/security/destructive-path review, including
  uninvoked nonrecursive release, recovery writer/reader integration, checkpoint/
  publication ordering, persistence failures, live-proxy support scope, locks,
  resources, and frontend/API boundaries.
- Obtain separate approved live Dahua/RTSP interruption/reconnect and intended
  deployment-filesystem/mount/storage acceptance evidence. Synthetic fixtures and
  test containers do not establish those deployment properties.
- Reconcile current documents and the actual final diff on the then-final commit.

## Explicit restrictions remain

No evidence here authorizes production cleanup execution, staging expiry, recovery/
reassembly, evidence deletion, comparator activation, merge, or deployment. Continue
treating cleanup eligibility as test-only unless separately authorized. Live Dahua/
RTSP, host storage, filesystem/mount durability, full browser behavior, blocked-I/O
behavior, and intended-host resource cost are not certified by reported-green focused
CI or this addendum.
