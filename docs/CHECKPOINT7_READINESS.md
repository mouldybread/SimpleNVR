# Checkpoint 7: documentation readiness and acceptance

Plan v2 step 7 only: README, large-media export documentation, and current recording,
retention and storage documents. The optional camera filter (step 8) and the final
audit/closure (step 9) are out of scope. This is a documentation reconciliation,
not proof that any code compiles or passes. No CI is requested by this slice.

## Slice history

- Slice A audit: `docs/CHECKPOINT7_AUDIT.md` records source-verified facts, documents
  examined, test/workflow search and the exact edits planned.
- Slice B README: adds the day-folder layout, stamped-name selection, retention,
  disk-usage behavior, upgrade behavior for flat recordings and camera-removal refusal.
- Slice C exports/audit index: adds name-based source selection and bounded scan
  failure semantics to `docs/LARGE_MEDIA_EXPORTS.md`; marks three dated notes in
  `docs/audit/README.md` as superseded and links current docs. The historical notes
  remain unchanged.
- Slice D: this readiness review and acceptance gate.

## Plan reconciliation

| Plan v2 step 7 requirement | Location and coverage | Status before CI |
| --- | --- | --- |
| README describes layout and UTC start-day | README “Recording storage, retention and disk usage”; detailed contract in `docs/RECORDING_LAYOUT.md` | Documented |
| Inclusion rule and name-end policy; no duration selection probe | README user summary and `docs/LARGE_MEDIA_EXPORTS.md`; full API/header contract in `docs/RECORDING_LAYOUT.md` and `docs/EXPORT_API_MODES.md` | Source-checked and documented |
| Changed headers, malformed names and API bounds | Detailed recording layout/export docs; README explains name-based selection and omitted invalid names | Documented; not execution evidence |
| Retention rules and limits | README summary links `docs/RETENTION_SELECTION.md`; option-2 exception and 400/8192 bounds are detailed there | Documented |
| Disk usage behavior and limits | README summary links `docs/DISK_USAGE.md`; cache, staging, failure, cold-start and count semantics live there | Documented |
| Old flat recordings unsupported in timeline/exports | README upgrade note: not shown, not migrated; records remain on disk. Retention applies its documented filename-prefix/mtime fallback; explicit camera removal retains direct-root `.mp4` compatibility | Documented and reconciled to code |
| Export planning selection and failure behavior | `docs/LARGE_MEDIA_EXPORTS.md`: stamped names, strict overlap, no selection probes, wrong-name-end consequence, 400 days / 8,192 entries per day / 20,000 candidates per scan, 503/504 and no partial results | Documented |
| Historical audit notes not mistaken for current contract | `docs/audit/README.md` identifies three superseded notes and points to current docs without rewriting historical records | Documented |
| Preserve deployment-boundary evidence and existing docs | `docs/STORAGE_DEPLOYMENT_BOUNDARIES.md`, `EXPORT_API_MODES.md`, `EXPORT_RENDERING.md` and dated audit notes were not edited | Preserved |
| Update PR status and keep issue/PR state | PR body updated after this docs commit with the new exact acceptance target; #11 remains open and #13 draft | Pending approved PR-body write |

## Test and link review

The audit read `deployment_storage_boundaries_test.go`; it reads no documentation. Code search found no Go test matches for README/docs references, but the search index is incomplete and does not cover Python, shell or CommonJS helpers. The checkpoint workflow runs Go build, vet, race and browser suites, with no Markdown/link checker. No doc-derived claim is presented as test evidence.

New relative links in the README point to paths listed in the branch docs directory. Relative links added in `LARGE_MEDIA_EXPORTS.md` point to `RECORDING_LAYOUT.md`; links in `docs/audit/README.md` point either to sibling historical notes or current files in the parent docs directory. No edit was made to `STORAGE_DEPLOYMENT_BOUNDARIES.md` or `docs/audit/README.md`'s historical source notes beyond the new index section.

## Known limits

- The code index searches were not exhaustive. A test helper or non-Go test may read Markdown; the final CI is the remaining check for that risk.
- The dated audit directory contains more notes than were individually read. Its index labels three known contradictions and warns other notes may overlap.
- README claims about flat files, retention and camera deletion were cross-checked against source, but the full test suite has not been run by the assistant.
- Plan v2's playback exact-end-equality limitation is documented in README; no frontend change is in this checkpoint.
- The old/boundary retention classes are described accurately: only retained-day skipping changes behavior; old and boundary days are both inspected per file.

## Acceptance gate

After the final documentation commit, read PR #13's actual head SHA and set it as the
acceptance target. Only on explicit user request, require all eight checks on that
same exact source SHA:

1. safety / Core — build, race, storage & lifecycle
2. safety / Chrome acceptance — isolated run 1
3. safety / Chrome acceptance — isolated run 2
4. safety / Ordered content — bounded synthetic resource measurements
5. safety / Required checks — all must pass
6. prepared_go / Full Go build, race suite and vet
7. prepared_browser / Prepared jobs and native playback — attempt 1
8. prepared_browser / Prepared jobs and native playback — attempt 2

Record the run URL and inspect relevant job evidence, especially checks that may read
README/docs. Also inspect the race result for the media-ownership waiter counter and
the runtime of the 34-camera cache fixture. If logs cannot be accessed, state that
plainly; check-run success is not individual job-log evidence. Queued, skipped,
canceled, failed or missing checks do not qualify. Any later code or documentation
commit requires a fresh run on its new head.

Until accepted, checkpoint 7 remains pending, #11 stays open and PR #13 stays draft.
No CI, workflow, label, merge, release, issue closure or claim that the whole issue is
complete is part of this documentation slice.
