# Isolated real filesystem-failure validation

## Scope

This tests-and-CI batch exercises kernel filesystem failures on tiny disposable tmpfs mounts. It changes no production admission, recording, recovery, publication, retention or deletion policy. The tests are not a physical-drive, ZFS/NFS, hardware-corruption, unavailable-mount or power-loss simulation.

The existing ordinary race suite still compiles this file but skips these mount-requiring cases unless explicitly enabled. A separate mandatory Safety verification step enables the cases under sudo + unshare in a new private mount namespace and runs them with the race detector. The step has no continue-on-error or permission-denied fallback. A green workflow therefore requires the isolated cases to run successfully, not merely skip in the ordinary suite.

## Isolation and bounds

The CI shell records its original mount namespace before invoking unshare --mount --propagation private --fork. Each test requires SIMPLENVR_STORAGE_FAILURE_TESTS=1, effective root, an explicit original-namespace marker and a different current namespace. It then makes its own namespace recursively private before mounting anything. Missing isolation, privileges, namespace support or mounting capability fails the dedicated run.

Only a new testing.T.TempDir target is mounted. The fixture checks TMPFS_MAGIC before filling it. Byte exhaustion uses a 1 MiB tmpfs with 128 inodes and an 8 MiB maximum fill loop; inode exhaustion uses an 8 MiB tmpfs with 32 inodes and at most 256 empty-file attempts. A third 1 MiB tmpfs is remounted read-only. Mounts have nodev/nosuid/noexec flags. Source media generation happens in a separate healthy test directory and remains small.

Cleanup closes test output/pins, unmounts normally and lets Go remove its synthetic temporary tree; unmount failures are test failures, not silent lazy-detach success. No production path such as /recordings is used. Filling and teardown concern synthetic fixture data only. Do not copy these privileged commands into a running NVR environment or run them against real recording directories.

The dedicated step uses the setup-go binary and downloaded module cache, with a separate disposable Go build cache under RUNNER_TEMP. Cleanup removes only that generated cache directory. No module or production dependency is added, and dependency-file unchanged checks remain. The ordinary race-suite timeout remains 120 seconds; the isolated run has its own 120-second test timeout within the existing overall job budget. Runtime must be measured in CI rather than assumed.

## Three cases

### Actual byte exhaustion

The fixture seeds a valid recovery manifest, event capture, root temporary recording and valid configuration before filling tmpfs until a real ENOSPC write and zero available bytes are observed.

An already-open anonymous output is written through the real trimFileWriter. It must return ENOSPC rather than claim the application output-size cap was exceeded. Real recovery-manifest replacement and configuration persistence must also fail with ENOSPC while preserving the previous complete snapshot/configuration and seeded recording bytes.

Read-only storage inspection must still complete and report the enabled free-space reserve unmet. This directly demonstrates that inspection completeness is not storage sufficiency. Real synthetic FFmpeg inputs remain on a healthy directory while the trim renderer's output directory is full; rendering must return no prepared output and release pinned resources. That integration can fail during temporary allocation or writing—it does not claim every run reaches an active encoder write. The direct writer assertion separately establishes the actual write-failure path.

### Actual inode exhaustion

Empty fixture files are created until the kernel returns ENOSPC. Inspection must report zero free inodes while available bytes remain positive. Manifest replacement and configuration persistence must fail while preserving previous data.

An HTTP event-handler invocation with an active-mode test BufferManager must reject staging allocation, release its reserved slot, queue no capture, append no metadata and invoke no metadata-persistence callback. This uses real filesystem allocation failure, not an injected errno. It does not start an RTSP recorder or exercise an authenticated network request. The existing bytes-only reserve is deliberately not presented as an inode admission guard or reservation.

### Actual read-only storage

After seeding evidence, tmpfs is remounted read-only. Manifest replacement, configuration persistence and no-replace publication must fail with real EROFS while retaining source/previous data. The publication test uses a per-call accepting validator for known synthetic sentinel bytes; the rename itself is real and no weaker fallback is used. It is not a media-validity test.

Read-only recovery inspection must remain usable. The test does not assume byte/inode exhaustion necessarily prevents rename: some rename operations can succeed without allocating a new inode or data block. Publication failure is instead exercised on the deterministically read-only filesystem.

## Limits and evidence

All three cases assert protected fixture contents remain unchanged. Configuration persistence can fail while creating its backup before replacing the main configuration; that failure and preservation are part of the real production saveAt path. Failed manifest temporary snapshots may remain under the existing evidence-preservation policy; no cleanup policy is changed.

These tests establish only their observed namespace-local failure scenarios if they pass. They do not validate real recording into full storage, RTSP/camera behavior, live-camera shutdown, arbitrary disk I/O stalls, permissions denied to a non-root service account, lost mounts, persistent-device behavior, power loss, browser behavior, or every failure point in capture/assembly/publication. Admission still is not a quota/watchdog/reservation. Broader AUDIT_ACCEPTANCE.md items remain open.

Preparation verified the existing workflow blob, parsed the modified YAML, checked the added shell with bash -n, and checked patch application. No privileged namespace/mount command was executed during preparation. Go is unavailable locally, so these Go tests have not been compiled or run by the assistant. The preceding f0c2cb7 commit was reported green by the user before this batch was proposed for push; that result does not cover these new tests or the privileged CI step. Keep the audit branch unmerged.
