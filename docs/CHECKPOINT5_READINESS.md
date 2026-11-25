# Checkpoint 5: retention/deletion readiness and acceptance

Plan v2 checkpoint 5 only; disk-usage caching (checkpoint 6) and the repository-wide
final audit are not included. Code/test baseline first reviewed:
`bef561b54e6860e7c5b164b6099f75d2ba09f67f`, plus the orphan evidence recheck fix
committed after documentation commit `ddff7e814bb977b0577b06e4f1a5c1c163b7564f`.
This document is a source-level reconciliation, not proof that the changes compile
or pass. No CI is requested.

## Implementation history

- `bb5f2d3500a118abdac0fe0e9eae3a0025e14fd0`: retention-specific conservative
  day selection, per-call work/injection, focused tests and selection documentation.
- `aef9db45339239ebded579f433b39d769d332668`: remove unused empty placeholder;
  the real retention entry point remains deleteExpiredPublishedLeasedAt.
- `f733b3933516a413aa73d6836c1e85aeaec68f51`: stamped camera day membership,
  budgeted one-level orphan handling, staging accounting and focused regressions.
- `bef561b54e6860e7c5b164b6099f75d2ba09f67f`: additional scale/boundary/failure
  regressions and the preserved-test inventory. No production changes in slice C.
- `ddff7e814bb977b0577b06e4f1a5c1c163b7564f`: documentation reconciliation.
- Review fix after `ddff7e8`: orphan day evidence is inspected fully once, then
  rechecked per day, per 64-removal batch and every 16 days, instead of a whole
  camera inspection before every removed file. Two orphan test cases changed (see
  the inventory); three tests and the orphan documentation were added or updated.

## Requirement reconciliation

| Plan requirement | Implementation and committed evidence | Assessment before CI |
| --- | --- | --- |
| Retention/delete-all intended production paths | cleanup tick -> cleanupRetentionSnapshot -> leased retention operation; deleteAllPublishedLeased uses the generic engine without retention skipping; slice A selector/delete-all tests | Source reviewed; execution pending |
| Day-aware retention cost, strict end cutoff | Retention skips days strictly newer than cutoff UTC day; older/boundary days retain publishedExpiredBefore and all individual validation; TestRetentionDaySelectionStrictCutoff and TestRetentionInspectedDayFallbackAndPartialPreservation | Implemented with agreed conservative exception |
| No bulk recursive deletion | Existing shared engine retains candidate preflight, immediate revalidation, individual unlink and empty-only rmdir; no RemoveAll introduced | Preserved |
| Root/day bounds and deterministic work | publishedDeletionWork counts all returned batches separately from processed entries, metadata and mutation/sync operations; all real days spend 400-day budget; exact day/member tests and TestRetention34CameraWorkIgnoresSkippedContents | Coverage committed; not run |
| Camera day membership and existing lifecycle safeguards | Valid stamped regular day members matching UTC start day; root rule unchanged; stop/join/persistence/lease machinery unchanged; membership and replacement tests plus existing lifecycle/sync suites | Source reviewed; execution pending |
| One-level orphan cleanup and evidence preservation | cleanupOrphanDays and inspectDays; full inspection first, then identity, batch and root rechecks; shared pass budget, 400-day cap; slice B evidence/partial/budget tests, slice C replacement tests and the review-fix recheck tests | Implemented within trusted-ancestor limits; recheck granularity documented |
| Staging root-count behavior | scanEventStaging unchanged; TestEventStagingCountsDayRootsNotPublishedContents creates 8,760 files beneath 365 day roots and checks 369 root/staging entries, 5 bytes, one event directory | Coverage committed; not run |
| Scale, exact limits and preservation evidence | 34 cameras/365 day folders; skipped-file growth with unchanged work; exact 400/401 and 8192/8193; cutoff/fallback/partial, replacement, guards, cancellation, unlink/sync/metadata failures and publication/retention serialization; 365-day orphan within the pass budget | See inventory; no production-scale claim |
| Existing flat fixtures and unrelated safety tests | No earlier assertions removed; flat orphan call ordering unchanged without day entries; browser authenticated 206 preflight untouched | Must still pass final CI |
| Documentation and status | Recording layout, retention selection, camera/orphan cleanup, test inventory and this reconciliation; PR description updated separately after the final commit | Documentation committed; PR update requires its own approved write |

## Agreed behavior and limits

Option 2 explicitly trades immediate cleanup of exceptional entries in skipped
newer-day folders for bounded retained-content work. Malformed or misplaced expired
files there are preserved until their folder ages into inspection. Inspected-day
fallback and generic/delete-all behavior are unchanged. Old-day classification is
not authority to erase arbitrary children; individual predicates remain mandatory.

Root enumeration still scales with retained day names, up to the cap; inspected
contents still cost work. The 34-camera test grows skipped contents without
changing root day count and performs real expired-file deletion in both passes.
It is not evidence that adding more root day names is free, and not a full
1.79-million-file or real-deployment performance qualification.

Orphan cleanup inspects a camera completely once, then rechecks evidence at
bounded granularity and charges every observation to one pass budget. Late
evidence inside a day is seen at the next 64-removal batch boundary, so up to 63
already validated stamped files can still be removed after it appears; the
evidence itself and any nonempty folder are preserved. Late root evidence is seen
within 16 days. A camera that has both legacy root files and day folders still
runs a full inspection per root file. Exhaustion may leave partial deletion;
count/bytes and flags must be reviewed before retrying. Registered/hidden
directories and root/day recovery evidence are preserved. Orphan operations retain
trusted-ancestor and pathname stat/remove race limitations, unlike the shared
fd-relative published engine's stronger confinement. No guarantee of atomic
compare-and-unlink is made.

The publication regression exercises two callers contending for shared ownership
and verifies the resulting day recreation/publication order. Its validator and
small synthetic payload are not a real-ffmpeg publication qualification. The
helper's started-channel handshake is before admission; it does not instrument
scheduler timing or prove the precise instant a goroutine starts waiting.

The inspection and sync seams enable deterministic failure injection; they do not
make kernel filesystem I/O interruptible. Successful unlinks can precede a sync
failure, and no rollback restores those bytes. Tests and source review are not a
substitute for final execution evidence.

## Final acceptance gate

Checkpoint 4 historical completion: `bed8d53631b4f656b497596c6bd27e1d6e20e057`,
run [45511972624](https://github.com/mouldybread/SimpleNVR/actions/runs/45511972624),
all eight required checks succeeded. That result does not cover checkpoint 5.

The earlier documentation head `ddff7e814bb977b0577b06e4f1a5c1c163b7564f` is no
longer the acceptance target. After the final documentation/code commit, read PR
#13's actual head SHA and put that immutable value in the PR's acceptance-target
field. Do not use this document's review baselines, an older green SHA, or a
synthetic merge SHA as the target. Updating the PR description does not itself
create a new code head.

Only on explicit user request, run the full checkpoint verification. Require all
of the following to complete successfully against that same final head:

1. safety / Core - build, race, storage & lifecycle
2. safety / Chrome acceptance - isolated run 1
3. safety / Chrome acceptance - isolated run 2
4. safety / Ordered content - bounded synthetic resource measurements
5. safety / Required checks - all must pass
6. prepared_go / Full Go build, race suite and vet
7. prepared_browser / Prepared jobs and native playback - attempt 1
8. prepared_browser / Prepared jobs and native playback - attempt 2

Record the source SHA and run URL. Inspect relevant job logs/evidence for new
retention/deletion/staging/orphan tests and preservation/resource assertions.
Queued, skipped, canceled, failed or missing checks do not qualify. A green summary
alone must not be described as individual job-evidence inspection. Any later
code/docs commit requires fresh acceptance on its new head. See
[CI procedure](CHECKPOINT_CI.md).

Until then checkpoint 5 stays acceptance-pending, issue #11 stays open, and PR #13
stays draft. No workflow, label, merge, release or issue closure is part of this
slice. Full issue closure remains subject to later Plan v2 checkpoints.
