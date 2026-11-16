# Finite recording-safety close-out matrix

Preparation base: eb04f54610f435c45c5f529afa8ab32da21de5ab.
This supplements AUDIT_ACCEPTANCE.md; its historical checkboxes are not cleared.
This is a work/disposition matrix, not whole-branch acceptance or deployment permission.

| Original acceptance area | Current evidence and boundary | Remaining disposition |
| --- | --- | --- |
| Exact-candidate build, race, vet and required dependencies | User reports 623bbe1 and eb04f54 green. Source workflow requires build, race, vet, isolated storage, two Chrome jobs and resource job. | Review original core logs, or review the new candidate's retained core test execution evidence. The new assessor covers the race-test stream, NOT vet/build/storage success. |
| Recovery producer / strict reader | New lifecycle, pin-failure, snapshot mutation/replacement, strict-schema and fuzz-seed suites are committed. Focused source review is recorded in AUDIT_CLOSEOUT_REVIEW.md. | Require observed execution of seven top-level contracts and ten fuzz seeds; complete integration review of recorder/admission/publication/release call sites. No deletion/reassembly authority. |
| Storage/runtime failures | Existing isolated namespace, non-root/container and lifecycle suites are not counted as absent. | Review exact-head job logs and qualify intended filesystem, mount loss, blocked kernel I/O and runtime capacity policy separately. Admission is not a reservation. |
| Live Dahua/RTSP interruption | Synthetic/container evidence does not establish live acquisition behavior. | Separately approved disposable live-camera test: disconnect/reconnect and interrupted continuous/event recording; inspect retained output. |
| Shutdown/joining/persistence | Reviewed eb04f54 export report has both SIGTERM cases and three clean exits/persistence markers; external observations only. | Whole-branch ordering review and review other recorder/inspection shutdown evidence. No power-loss durability claim. |
| Authenticated exports | Four runtime-export cases passed in reviewed eb04f54 report, all 48 tooling tests passed; fixture teardown passed. | Credit scoped campaign; do not repeat the leader-thread defect or infer intended-host qualification. |
| Browser playback/download | Both eb04f54 isolated attempts report baseline and edges passed. Late TLS truncation uses separate native src injection. | Credit scoped cases; reconcile any remaining timeline/multicamera acceptance with existing tests before adding work. OS save dialog and live inputs remain outside evidence. |
| Whole-branch destructive/lock/security/publication review | Focused review has begun, not completed. Older cleanup remediations exist. | Inspect current delete/retention/camera removal/ownership/persistence/auth/proxy/frontend paths and interactions; only established defects justify fixes. |
| Evidence reconciliation | Reviewed Dropbox text is distinct from independently downloaded Actions results and byte-verified media. | Update current status without deleting historical failures. Any later source changes require exact-candidate checks; do not reuse old artifacts as new-head proof. |

## Separate decisions, not automatic implementation work

- Comparator remains test-only. Activation requires explicit approval and intended-host cost/latency acceptance; matching existing inputs is not uninterrupted ingestion coverage.
- Positive ingestion-time completeness remains unknown. No automatic cleanup, staging expiry, recovery or evidence deletion is inferred from CI, probe, comparison or manifest inspection.
- Optional audio-aware trimming/broader formats, synchronized multicamera advancement, next-day fetching, gapless transitions, recovery UI and legacy ID migration are not silently added to close-out.
- Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd; merge and deployment require a separate decision.

## Evidence already reviewed in this conversation

- Export report: https://www.dropbox.com/scl/fi/cpk1rmriqyf4kdhvyf12z
- Export tooling log: https://www.dropbox.com/scl/fi/xbl3v45qck46ehvhkclhe
- Fixture teardown: https://www.dropbox.com/scl/fi/1n0848paahdqoi1lkmj6f
- Browser aggregates: https://www.dropbox.com/scl/fi/axln3kfda0jksziimj07b and https://www.dropbox.com/scl/fi/bwvgwce01jifahh0klm94
- Correspondence assessment: https://www.dropbox.com/scl/fi/s1rhk05e6q75bthb2ze1u

These links record reviewed source identities, not signed run provenance. Keep the supplied artifacts and any failed/incomplete attempts.
