# Checkpoint 5: retention/deletion readiness and acceptance

Plan v2 checkpoint 5 only; disk-usage caching (checkpoint 6) and the repository-wide
final audit are not included. This document is a source-level reconciliation. It was
written before acceptance; the acceptance record is at the end.

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
- `a52b3a121ab52ebb84139f78df70d0b9c4dd7257`: orphan day evidence is inspected
  fully once, then rechecked per member, per 64-removal batch and every 16 days,
  instead of a whole camera inspection before every removed file.
- `0415b83296f13c2e985126a38bc761dbf8dfabce`: tests that the time-based production
  retention entry applies the skip policy and that the retention root bound is exact.
- `6a912bde1a20e62fae65eda38ad0bcf589f3b1b0`: the 34-camera test counts the physical
  fixture instead of comparing constants.
- `fa6f7854574c1378b029c37774fa31711b43c50a`: the UTC classification test uses
  cutoffs whose local date differs from the UTC date. This is the accepted head.
- `69da10c68c772a14843a24c4b712b1d00a10ccb5` (after acceptance): a waiter counter on
  the shared media ownership gate and a stronger publication serialization test.
  See the acceptance record below.

## Requirement reconciliation

| Plan requirement | Implementation and committed evidence | Assessment |
| --- | --- | --- |
| Retention/delete-all intended production paths | cleanup tick -> cleanupRetentionSnapshot -> leased retention operation; deleteAllPublishedLeased uses the generic engine without retention skipping; slice A selector/delete-all tests; production entry test | Accepted on `fa6f785` |
| Day-aware retention cost, strict end cutoff | Retention skips days strictly newer than cutoff UTC day; older/boundary days retain publishedExpiredBefore and all individual validation; TestRetentionDaySelectionStrictCutoff and TestRetentionInspectedDayFallbackAndPartialPreservation | Accepted with the agreed conservative exception |
| No bulk recursive deletion | Existing shared engine retains candidate preflight, immediate revalidation, individual unlink and empty-only rmdir; no RemoveAll introduced | Preserved |
| Root/day bounds and deterministic work | publishedDeletionWork counts all returned batches separately from processed entries, metadata and mutation/sync operations; all real days spend 400-day budget; exact day/member/root-entry tests and TestRetention34CameraWorkIgnoresSkippedContents | Accepted on `fa6f785` |
| Camera day membership and existing lifecycle safeguards | Valid stamped regular day members matching UTC start day; root rule unchanged; stop/join/persistence/lease machinery unchanged | Accepted on `fa6f785` |
| One-level orphan cleanup and evidence preservation | cleanupOrphanDays and inspectDays; full inspection first, then identity, batch and root rechecks; shared pass budget, 400-day cap; evidence, partial, budget, replacement and recheck tests | Accepted within trusted-ancestor limits |
| Staging root-count behavior | scanEventStaging unchanged; TestEventStagingCountsDayRootsNotPublishedContents creates 8,760 files beneath 365 day roots and checks 369 root/staging entries, 5 bytes, one event directory | Accepted on `fa6f785` |
| Scale, exact limits and preservation evidence | 34 cameras/365 day folders with a counted fixture; skipped-file growth with unchanged work; exact 400/401 and 8192/8193; cutoff/fallback/partial, replacement, guards, cancellation, unlink/sync/metadata failures and publication/retention serialization | Accepted; no production-scale claim |
| Existing flat fixtures and unrelated safety tests | No earlier assertions removed; flat orphan call ordering unchanged without day entries; browser authenticated 206 preflight untouched | Accepted on `fa6f785` |
| Documentation and status | Recording layout, retention selection, camera/orphan cleanup, test inventory and this reconciliation | Committed |

## Agreed behavior and limits

Option 2 explicitly trades immediate cleanup of exceptional entries in skipped
newer-day folders for bounded retained-content work. Malformed or misplaced expired
files there are preserved until their folder ages into inspection. Inspected-day
fallback and generic/delete-all behavior are unchanged. Old-day classification is
not authority to erase arbitrary children; individual predicates remain mandatory.
Old and boundary days are handled identically; only the retained class changes
behavior, because no cheaper path for wholly old days is permitted.

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

The publication regression exercises two callers contending for shared ownership.
The publisher is observed blocked inside ownership admission through a waiter
counter on the shared gate before retention runs, and is observed still waiting
afterward, so the ordering is established rather than assumed. The counter is an
observation seam only and never gates admission. The test's validator and small
synthetic payload are not a real-ffmpeg publication qualification, and its timeouts
are deadlock watchdogs, not performance assertions.

The inspection and sync seams enable deterministic failure injection; they do not
make kernel filesystem I/O interruptible. Successful unlinks can precede a sync
failure, and no rollback restores those bytes.

## Acceptance record

Checkpoint 4 was recorded complete on `bed8d53631b4f656b497596c6bd27e1d6e20e057`,
run [45511972624](https://github.com/mouldybread/SimpleNVR/actions/runs/45511972624).

Checkpoint 5 was recorded complete in [issue #11](https://github.com/mouldybread/SimpleNVR/issues/11#issuecomment-7171324111)
on `fa6f7854574c1378b029c37774fa31711b43c50a`, run
[45676664390](https://github.com/mouldybread/SimpleNVR/actions/runs/45676664390),
all eight required checks successful. This was check-run verification; individual
job logs could not be inspected.

The accepted head predates `69da10c`, which adds the waiter counter in
`media_mutation_ownership.go` and strengthens the serialization test. That commit is
not covered by the checkpoint 5 run. It is included in the head that checkpoint 6's
acceptance run will verify, and that run should be read with attention to the
serialization test and to every code path that acquires the media ownership gate.
