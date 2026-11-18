# Runtime storage-failure validation

## Added controlled cases

runtime_storage_failure_test.go extends the existing required private mount-namespace CI step. Its parent tests match ^TestStorageFailureMountNamespace and therefore need no new workflow wiring. Ordinary runs skip through the existing mountedStorageFailureFixture guard. The subprocess-only permission helper returns immediately unless explicitly enabled. No production code or cleanup policy changes.

Three subcases mount separate 4 MiB tmpfs fixtures, generate bounded real media on healthy storage, and invoke production recordSegment, recordEventDuration or concatenateEventRecording. They reuse the test-only input substitution and paced assembly described in ACTIVE_RECORDER_VALIDATION.md. Each observes nonempty output and a live real FFmpeg executable before filling storage. Filling requires TMPFS_MAGIC and a capacity at most 8 MiB, uses at most 8 MiB of writes, and requires real ENOSPC both from filling and an already-open independent write witness, plus zero available bytes.

Processing must terminate without test cancellation, reap the observed child, reject successful capture/assembly completion and retain nonempty failed output. Event cases must latch recovery-persistence failure, retain the last complete checkpoint, release their slot and preserve original pinned inputs; assembly also preserves its original capture and concat list. All cases preserve healthy source media and the pre-existing fixture recording, manifest and configuration. Continuous output may remain unpublished or become a minimally readable partial; neither is complete-footage evidence. This is not per-write syscall tracing of FFmpeg's errno.

The fourth case uses a 1 MiB fixture with synthetic directories mode 0555 and files 0644. A copied test executable in a separate generated directory runs with UID/GID 65534 and no supplementary groups. It reaches only the fixture through an inherited directory descriptor, verifies its credentials and tmpfs identity, then requires actual EACCES for file creation, recovery-manifest replacement, production saveAt backup/replacement and real no-replace publication. Parent assertions require original bytes unchanged and no published destination. The accepting publication validator uses sentinel bytes; this permission case is not media validation or recording as the deployment's actual service account. Fixture permission relaxation is not a change to production owner-only evidence permissions.

## Isolation and evidence limits

The existing explicit opt-in, root/original-namespace checks, recursive-private propagation, normal unmount and failure-on-unmount rules remain. Only generated test mounts are filled; no /recordings path, production data, lazy unmount or permission fallback is used. The copied helper and permission changes affect generated fixtures only. Test outputs/evidence are not deleted to make processing succeed; disposable mounts are torn down afterward. The established 120-second dedicated suite budget is unchanged and must be checked in CI.

Tests invoke recorder/assembly functions directly, not live RTSP or a full HTTP event-admission workflow. They do not exercise lost mounts, fallback into underlying directories, stuck filesystem calls, physical media, ZFS/NFS, corruption, power loss or every persistence boundary. The earlier tmpfs admission/export cases retain their narrower documented scope. Runtime size enforcement, true quotas and reservations remain absent; this batch does not silently impose new limits or authorize evidence deletion. Deployment filesystem/no-replace/procfs and storage-policy acceptance remain open.

## Preparation evidence

User-reported Safety verification is green through 9137668, not independently inspected Actions logs. Go/gofmt are unavailable locally. No Go compilation, vet, race or privileged namespace test execution is claimed for this batch.

Local preparation ran a separate Python helper as UID/GID 65534 with no supplementary groups, entered a generated read-only directory through an inherited descriptor, observed EACCES for creation and rename, and confirmed original sentinel bytes survived. This tests that native permission-fixture mechanism only, not production Go persistence/publication or mount orchestration. Record Safety verification for the exact resulting commit and keep the branch unmerged.
