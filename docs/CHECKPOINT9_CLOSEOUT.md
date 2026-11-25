# Checkpoint 9: closeout record for issue #11

Plan v2 step 9 is the final audit and closeout. This file records the acceptance
criteria, evidence and limits. It does not itself close issue #11 or mark PR #13
ready. It was written before a final-head CI run; checkpoint 9 remains pending.

## Current state

The latest branch head checked before this file was created is
`a13a50ad2f0dcfc1ed3845dc392c4c75760de3b3`. The last verified run that can be
identified here is run 45696730551 on `2e1b33bd4f6f23ca54339a423c0109517089cf1b`,
with all eight required check runs successful. That run predates the dead-code
removal in `3ab0c1d` and frontend-wrapper test in `6540fb5`; it does not verify
those commits. The owner stated all checkpoints except checkpoint 9 are green;
this record does not invent a run ID or SHA for any later run that has not been
provided or independently read.

The changes since the `2e1b33b` run are:

- `3ab0c1dd279a301cd5cd1c30207785feab25ea61`: removes unused
  `maxRecordingScanEntries` and test-only `cleanupOldRecordings*`; cleanup tests
  now use the production retention entry. One redundant direct-helper assertion
  was removed from the canceled-tick test; no test function was deleted.
- `6540fb5c7f392ec857ba41b7f0f6b39f17fdf858`: adds the Node regression for the
  timeline fetch wrapper plus its Go test entry point.
- `e21119f7d9b5bd2ea2d021972a49beacdb5f5e52` and
  `a13a50ad2f0dcfc1ed3845dc392c4c75760de3b3`: checkpoint 9 documentation and
  stale-status reconciliation.
- This file is another documentation-only commit. It does not change code or
  tests, but it changes the head after the most recently identified green run.

## Acceptance criteria

The issue's original acceptance criteria, with the owner-approved design target
of 34 cameras x 365 days, are mapped below. This is source/test mapping, not an
execution claim for tests added after `2e1b33b`.

| Criterion | Evidence in the branch | What the evidence does not prove |
| --- | --- | --- |
| Timeline and export work at 365-day retention (design target: 34 cameras x 365 days) | `TestStampedScan34CamerasIgnoresOlderRetention`; `TestStampedScanLargeRetentionNarrowWindow`; `TestPreparedExportUsesStampedDayFolder`; `TestWholeClipExportDeclaresRangePolicy`; trim, network and runtime-browser fixtures using day-folder paths. | Not a 34-camera live deployment, not 1.79 million physical files, and not a real camera with 365 days of footage. The named tests in commits after `2e1b33b` have not been covered by an identified run. |
| Request work grows with the requested window rather than total retention | Per-call scan counters and `TestStampedScan34CamerasIgnoresOlderRetention` compare a short-retention tree to a 365-day tree; `TestRetentionSkippedContentsDoNotIncreaseWork`; `TestRecordingIntervalsTrustNamesAndNeverProbe`; disk-usage cache counters in `TestDiskUsage34CamerasYearOfDayFolders`. | These assert bounded operations, not elapsed-time performance. Opened folders are still enumerated by name. Root listings for retention, camera deletion and disk usage still grow with day-folder count up to their 400-day bound. Cold-start disk-usage time is unmeasured. |
| Tests cover storage well above 8,192 files | `TestStampedScanLargeRetentionNarrowWindow` creates more than 20,000 files across day folders; retention, disk-usage, camera-deletion and orphan tests cover exact 8,192/8,193 and 400/401 boundaries. | A single opened folder above 8,192 entries deliberately fails closed. The large fixture is not a production-scale physical run. |

## Related problem statements

- **Per-file duration probing:** removed from timeline/export selection. The separate probes used for export validation remain.
- **Whole-directory glob retention:** the production retention tick uses the day-aware published-deletion engine. `cleanupOldRecordings*` was confirmed test-only in the audit and removed in `3ab0c1d`; tests now enter the production retention path.
- **One unusable recording breaking the timeline:** the timeline now skips an unusable in-window candidate and reports `X-Timeline-Unreadable-Count`; the playback page shows a transient warning. The timeline still fails whole for cancellation, other inspection errors, unreadable folders, and the 8,192-entry, 400-day or 20,000-candidate bounds. Downloads and exports stay strict. Skipped candidates still count toward the 20,000 budget.

## Checkpoint record

All results below refer only to their named source heads. Check-run summaries were
read; individual job logs were not inspected unless explicitly stated otherwise.

| Plan v2 checkpoint | Status and evidence |
| --- | --- |
| 1–2 | Recorded complete in issue #11; checkpoint 2: `e4f77353f63d91c28a81e96b6aab79c837892300`, run `45314682415`, all eight checks successful. |
| 3 — Serving | Complete on `5887e1129f0e16dafe9c4771bf8c4d184a3fba4f`, run `45427373922`, all eight checks successful. |
| 4 — Scan | Complete on `bed8d53631b4f656b497596c6bd27e1d6e20e057`, run `45511972624`, all eight checks successful. |
| 5 — Retention and deletion | Complete on `fa6f7854574c1378b029c37774fa31711b43c50a`, run `45676664390`, all eight checks successful. |
| 6 — Disk usage | Complete on `d5f9665cc964d47daf4769dd4e774bad9d55f406`, run `45686302269`, all eight checks successful. |
| 7 — Documentation | Recorded complete on `9efa57d1bb3675e3c6a43c192ebeba113268e4fb`, no CI by owner decision because only documentation changed; recorded in issue comment 7173476310. The docs at that head were later included in the full run on `2e1b33b`. |
| 8 — Optional camera filter | Implemented after checkpoint 7: selected-camera `camera_id` filter, timeline skip-and-report, unreadable warning, dead-code cleanup and wrapper regression. **A successful check-run summary for `2e1b33b` does not cover the later commits `3ab0c1d` or `6540fb5`. Final acceptance for this code is pending a run on the current final head.** |
| 9 — Documentation closeout | In progress. This record and the stale-status updates are documentation-only. Final closeout decision remains pending the required final-head CI run for post-`2e1b33b` code and reconciliation with the owner-provided run. |

The identified run 45696730551 on `2e1b33b` had eight completed successful check
runs: safety Core, two isolated Chrome attempts, ordered-content measurements,
required-checks aggregate, prepared Go build/race/vet and two prepared-browser
attempts. Its run page and individual job logs could not be retrieved by the
assistant. It does not cover later code commits, and no assertion is made that the
wrapper test or dead-code cleanup passed in that run.

## Dated audit note review

All 95 notes in `docs/audit/` were read for this review. Eighteen were found to
contain statements superseded by this work; they are named and mapped to current
documents in [`docs/audit/README.md`](audit/README.md). The dated notes remain
historical records and were not edited. The other 77 notes had no stale claims
identified by this review.

## Limits and accepted restrictions

- No 1.79-million-file physical run, real-camera 365-day run or wall-clock scaling measurement is claimed. The owner accepts bounded fixture/counter evidence for the issue's stated criteria.
- Job logs have not been read for checkpoints 3–8. The owner asked to inspect those logs only if a check fails or supplies further insight. The 34-camera fixture duration and race-detector output therefore remain unknown.
- Disk-usage cold-start time at 34 cameras x 365 days is unmeasured; slower cameras may show “Unavailable” after restart until a refresh. The cache is in memory and trusts published-media immutability without periodic revalidation.
- The timeline warning is transient and reports unreadable files, not malformed-name counts. It is not a persistent timeline marker.
- Playback continues between clips only on exact end/start equality; wall-clock stamped times can differ slightly.
- Retention's agreed option-2 policy delays cleanup of exceptional content in newer skipped days. Orphan cleanup uses batched evidence rechecks and retains trusted-ancestor/pathname-race limits.
- Tests added by `3ab0c1d` and `6540fb5` have source-level coverage but no identified run yet.
- Issue #11 remains open and PR #13 remains draft. No merge, release, deployment, recovery, staging expiry or production cleanup action is authorized by this document.

## Closeout gate

Before declaring Plan v2 step 9 complete:

1. Read the actual PR #13 head and the owner's full run identity. Require all eight required checks to have completed successfully on that exact source SHA. The existing green run on `2e1b33b` is useful historical evidence but cannot qualify its later code descendants by itself.
2. Record the run URL and SHA here. Per the owner's direction, inspect detailed job logs only if a check fails or the owner supplies logs; if they are not inspected, state that plainly. Do not claim individual job evidence from a green summary.
3. Verify no later code/docs commit moved the head. Any subsequent commit changes the acceptance target.
4. Update PR #13's description and title/readiness only after the head and evidence are reconciled. Keep the issue open and PR draft until then. Write “Closes #11” only when the owner decides to close it after reviewing this record.

No CI is requested by this documentation commit. The owner will run the final
verification after the desired documentation state is in place. This file does
not mark step 9 accepted.
