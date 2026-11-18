# Recording-safety audit — final closeout ledger

Branch: fix/recording-safety-audit. PR #1 base (main): c5be6bf0cfea03071c7df1c3b0f5064753c881dd.
Code candidate: 49dbec007f28601eab9958c9d5b6242fbdd447e5. The commit adding this file is
documentation-only on top of it; the exact-candidate verdict binds to that commit's own
Safety verification run, not to this text.

This ledger supplements AUDIT_ACCEPTANCE.md, AUDIT_CLOSEOUT_MATRIX.md and
PERSISTENCE_CLOSEOUT_VERDICT.md. Historical failures, pending wording and checkboxes in
those files are preserved, not rewritten. It is not deployment qualification.

## Evidence classes used

- Source review: files read at 49dbec0 in this closeout.
- Local execution: Go 1.26 (go test -json -race -count=1 -timeout=120s ./...) on base 5832550
  plus the Item 2 patch: exit 0, 1697 pass, 0 fail, 15 skip. govulncheck v1.10.0 with
  go1.26.11: 0 reachable vulnerabilities.
- CI, inspected logs at 49dbec0: Safety verification push run 44295886654.
  - Core job 134772668915: success. Race suite 1698 pass, 0 fail, 15 skip. Assessor JSON:
    head 49dbec0, required_top_level_tests 7, required_fuzz_seeds 10, skipped_test_events 15,
    status passed, cleanup_authorized false, runtime_qualified false.
  - Named passes in that log: TestFrontendCameraRemovalConfirmation,
    TestCameraListEmptyStateSatisfiesRemovalResponseContract, six TestRecoveryContract*
    tests, FuzzRecoveryContractStrictManifest plus seed#0..seed#9.
  - govulncheck step: "Your code is affected by 0 vulnerabilities" (one unreachable module
    advisory).
  - Runtime image build resolved alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf.
    The active-container recording/SIGTERM/restart and runtime-export steps passed.
  - Resource job 134772668944: success; all TestCorrResource* executed and passed.
  - Chrome isolated run 1 (134772668767): success.
  - Chrome isolated run 2 (134772668771): failed in "Install synthetic media test tools"
    (apt, 5-minute step timeout) before any test ran. Required-checks job 134775501173
    therefore failed. No test failed.
  - Artifacts retained: core-test-execution-, correspondence-resources-, runtime-export-acceptance-
    and runtime-browser-acceptance-49dbec0…-1.
- Historical CI, preserved:
  - PR run 44295895919 at 49dbec0: core, Chrome 1 and Chrome 2 failed in the same apt
    install step; the resource job succeeded.
  - PR run 44277731397 at 5832550: same apt timeouts.
  - Push run 44277719063 at 5832550: all jobs succeeded.
  - Successful installs on this run took 17–37 s; failures hung for 5 minutes. This is
    runner mirror infrastructure, not code. The workflow timeout was not changed.

### Skip accounting (15 race-suite skips at 49dbec0)

- Nine TestStorageFailureMountNamespace* events: executed and passed in the same job's
  "Isolated real storage failures" step.
- Six TestCorrResource* skips: executed and passed in the resource job.
- One TestDeploymentFilesystemCompatibility skip: requires intended-deployment scratch
  storage. This is live/storage qualification (section 9), deferred by the owner.

## Section ledger

| # | Section | Status | Basis and limits |
| --- | --- | --- | --- |
| 1 | Persistence | Validated (closed at 5c4be96) | PERSISTENCE_CLOSEOUT_VERDICT.md. 5c4be96..49dbec0 adds no persistence-code change. |
| 2 | Camera deletion | Validated at 49dbec0 | Both routes are POST-only through deleteManagedCameras: validate → stop/join → stopped-intent save → guard → media lease → guard → remove → unregister → final save → release; 409/500/503 mapping. Item 2 frontend reports uncertain single-camera outcomes; the empty-list response contract is tested. Both named tests passed in CI. |
| 3 | Lifecycle/publication/recovery/shutdown | Validated at 49dbec0 (source review + CI) | recordingControlMu serializes control; WaitGroup Add only with a started worker; no_recording refused; state revalidated after unlocked admission I/O; event workers bounded (eventWorkers=2), all children joined, unprocessed jobs keep staging; capture contexts derive from the camera context; Background contexts appear only in bounded finalize/shutdown graces. Recovery remains inspection-only. Seven contracts and ten seeds passed. Active-container SIGTERM/restart passed. No power-loss claim. |
| 4 | Other destructive paths | Validated at 49dbec0, with restrictions | Retention: media lease + policy lock + registry guard + fd-relative published-deletion engine (8192-entry limit, preflight and per-entry dev/ino recheck, no recursion). Delete-all recordings (leased engine) and orphan cleanup are POST-only under recordingControlMu; event clears are POST-only and remove event metadata only, no media files. Opaque staging/.tmp is never deleted. cleanupOldRecordings* remains test-only (no production caller). Restriction: stat/unlink is not atomic; directory trust is required; pathname races are reduced, not eliminated. |
| 5 | Auth/frontend/network/live proxy | Validated at 49dbec0, accepted topology | Every route, including /static and /recordings, is behind session auth; bearer is limited to POST /api/events/trigger; all 15 mutation routes are POST-only; CrossOriginProtection; Secure/HttpOnly/Lax cookie; Argon2id parameter bounds; login throttle; Authorization/Cookie stripped downstream. Live proxy derives only the host from the camera RTSP URL and requests http://host:1984/api/stream.mp4?src=<camera name>. Owner-accepted as the only supported setup: go2rtc on its own IP, camera RTSP URLs pointing at it. |
| 6 | Release/container hygiene | Validated at 49dbec0, accepted restriction | The tag publish workflow requires the reusable Safety verification job (permissions: contents read); runtime base digest pinned; govulncheck gate. Accepted: the container runs as root by default (owner decision). Builder golang:1.26-alpine floats within 1.26 by design. There is no free-space emergency policy (operational limit). |
| 7 | Exact-candidate CI evidence | Implementation present; final green outstanding | All required content passed at 49dbec0 except Chrome run 2, which hit an infrastructure timeout before tests ran. A fully green required-checks job is still needed on the final docs commit (or a re-run). |
| 8 | Playback/export acceptance | Validated within CI scope | Real runtime HTTPS export and Chrome playback/download passed (run 1); phase and render cancellation tests are in the core suite. Not covered: OS save dialogs, other browsers, live inputs. These are left to section 9 and are not new code work. |
| 9 | Live-camera/storage qualification | Deferred by owner (post-merge) | The owner will perform it after merge. It is not a closeout or merge requirement. Not claimed. |

## Preserved non-activations

Event staging and retention stay unconditional. Recovery/reassembly, staging expiry, the
unused release engine, new deletion authority and the test-only comparator remain inactive.
No ingestion-time completeness or playable/complete footage is inferred from exit status,
size, existence or a probe.

## Verdict

Audit closeout: closed, except the exact-candidate required-checks green on the final commit.
Merge readiness: ready once that run is fully green. Deployment qualification: not qualified;
owner-run live/storage qualification after merge.
