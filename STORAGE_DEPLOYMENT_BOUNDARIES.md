# Storage deployment and shutdown boundaries

## Candidate and evidence status

Prepared against 8526aa56d4cf37422195c84aef721c1635c41350. That base is being verified; no green report is assumed. Prior green through 19187ae is user-reported, not independently inspected Actions logs. This is a group-A remediation/validation batch with a shutdown-persistence finding also relevant to group C. It does not close A, B, C or D and adds no storage-management UI, quota service, automatic mount recovery, staging expiry or evidence deletion.

Source review included storage_admission.go, runtime_storage_failure_test.go, storage_failure_integration_test.go, events.go, shutdown.go/tests, lifecycle_test.go, recording_publication.go, Dockerfile and safety-ci.yml. The bdc1ef5 runtime storage tests remain in the tree: active continuous/capture/assembly writes on bounded private tmpfs and UID/GID 65534 permission failures. They are selected by the mandatory isolated storage CI step. This confirms source/selection, not independent historical execution evidence.

## Material shutdown finding and remedy

Previously shutdownApplication launched a coordinator that waited unconditionally for workers, admitted mutations and HTTP shutdown, then invoked persistence. The caller could time out while that coordinator remained alive. If a blocked phase subsequently unblocked, the abandoned coordinator could start a final save after shutdown had already reported failure.

The candidate joins those phases in the caller with context-aware waits. A deadline before completion returns failure without leaving a waiting save coordinator. Persistence is admitted only after all joins and a context check. Its goroutine rechecks context before invoking the callback. Tests cover worker/mutation/HTTP waits, an already canceled context, error propagation and an HTTP-gated blocked recording-admission phase that later releases without launching a worker or final save. The admission fixture is an injected pause, not a blocked kernel Statfs.

A save already admitted can still be executing when the context expires; its writes/fsync cannot be canceled or rolled back by this code. Cancellation can race the last check before callback invocation. The existing blocked-persistence test remains applicable. Recorder waits, global locks and uninterruptible filesystem calls may also outlive the deadline. The process must not report clean shutdown in those cases. The runtime currently exits on shutdown failure; restarting or remounting beneath a still-running old process is not safe recovery.

## Mount loss: explicit non-guarantee

SIMPLENVR_MIN_FREE_BYTES is an optional capacity observation, disabled when unset or zero. With a positive reserve, a missing camera directory fails admission. A disappeared mount can instead expose an existing writable underlying directory whose free space looks adequate. Capacity does not identify the intended storage volume, prevent mid-capture loss, reserve space, enforce per-camera quotas or monitor runtime mount health.

The new TestStorageFailureMountNamespaceDetachedMountFallback deliberately demonstrates that limitation using a nested, bounded tmpfs under the existing guarded private-namespace fixture. It lazy-detaches only its fixture mount, observes the underlying directory still passing capacity admission, and reads the detached capture evidence through a retained directory descriptor. It does not alter /recordings, inject an NFS stall or certify data durability. Passing this test means the hazard was reproduced and documented, not fixed by automatic mount detection.

## Restricted deployment profile to validate

These are required restrictions for the proposed supported profile, not claims of completed deployment acceptance:

1. Use a dedicated, operator-controlled, responsive local recordings filesystem with a stable container bind mount. Verify the intended host volume and mount identity before container creation, not just that its directory exists. Use host service ordering/mount dependencies that refuse startup without that volume. Avoid Docker's implicit missing-source directory creation by supplying an explicit existing bind source.
2. Do not unmount, lazy-detach, hot-swap or replace recordings paths while SimpleNVR is running. Stop and verify the old process has exited before maintenance or restart. A bind mount is not proof that the correct host volume was present when it was created. Mount identity and maintenance exclusion are operator controls; this candidate adds no automatic enforcement.
3. Use a read-only container root, a writable /recordings bind and explicit writable /certs and bounded /tmp mounts. Do not grant mount-management privileges. A read-only root limits some container-root fallback writes; it does not prevent a wrongly bound host source or guarantee storage availability.
4. Enable a positive free-byte reserve, monitor capacity externally and establish an operator-approved filesystem/output-space budget. No universal reserve, quota or maximum recording size is invented here. A reserve cannot predict runtime ENOSPC, inode exhaustion, read-only transitions or storage faults.
5. Run the filesystem compatibility check below on the actual host volume/kernel and with relevant container permissions. RENAME_NOREPLACE and procfs descriptor reopening are required. Do not add a check-then-rename fallback on unsupported storage.
6. Remote/FUSE or otherwise indefinitely blocking storage, online mount replacement and automatic failover are outside this proposed acceptance scope. An application deadline is not a bound on a kernel I/O operation. If deployment requires those properties, A remains blocked on an explicit supported design and evidence rather than a documentation waiver.

## Deployment-filesystem compatibility check

The opt-in Go test is skipped unless SIMPLENVR_DEPLOYMENT_TEST_ROOT names a real scratch parent on the intended filesystem. Example, using an operator-owned directory on that filesystem:

    SIMPLENVR_DEPLOYMENT_TEST_ROOT=/operator-supplied/scratch go test -count=1 -v -run '^TestDeploymentFilesystemCompatibility$' .

It creates only a uniquely named synthetic scratch directory, then removes that test directory. It does not inspect or delete existing recordings. It checks actual no-replace rename/collision preservation, reopening an original inode through procfs after pathname replacement, and the application's publication boundary with an accepting synthetic probe. Save the exact commit, runtime/kernel/filesystem context and DEPLOYMENT_FILESYSTEM_COMPATIBILITY log. Run under the intended permissions and container procfs policy. This is not a media-decode, power-loss, fsync-durability or lost-mount test.

## Active runtime-container CI fixture

The added CI step builds on the unchanged runtime Dockerfile and runs tests/active-container-storage.sh against that exact CI-built image. It requires active continuous and event-buffer FFmpeg processes, continuous output bytes and at least two prebuffer segments before SIGTERM. It checks clean container exit, persisted restart intent for both cameras, a new nonempty published partial per stop, transient-buffer cleanup and byte-identical synthetic retained event inputs/checkpoints. It repeats start/stop in the same container to exercise persisted restart intent.

The fixture uses the real application, Alpine runtime and real FFmpeg, a read-only root, writable recordings/certs/tmp mounts, dropped capabilities except NET_BIND_SERVICE, no-new-privileges and no networking. A wrapper replaces RTSP input with a paced looping synthetic file. It does not validate Dahua RTSP disconnect/reconnect, queued event capture/assembly in the runtime image, authenticated export traffic, genuine browser behavior, physical storage or production mount-loss handling. It checks published partial presence, not complete footage. Synthetic fixture directories are removed by the test; production evidence is never touched. Existing empty-camera SIGTERM smoke and all prior media/storage tests remain enabled.

## Preparation and remaining acceptance

Modified source blobs and the exact patch round trip are verified during preparation. bash -n checks the new shell grammar only. Go/gofmt and Docker are unavailable locally, so no Go/build/vet/race/container or mount test execution is claimed. The new deployment compatibility test has not run against deployment storage. CI must run this candidate after a separately approved push.

Remaining A evidence: successful candidate CI including the new active-container and guarded mount-loss fixtures; actual live Dahua disconnect/reconnect; actual deployment-filesystem/procfs/no-replace results; an enforceable host mount/maintenance policy; a bounded storage/output budget decision; and confirmation that uninterruptible-I/O behavior is acceptable under the restricted profile. The new tests are not substitutes for those records. C's destructive-path/recovery/persistence review, B's runtime export/browser checks and D's final exact-candidate review remain open. main stays untouched; the audit branch stays unmerged.
