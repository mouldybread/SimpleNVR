# Publication admission, bounded subtask 4

Baseline: 9abb2ae811a1efc4ab34d0916e1381ff50e7f6ed. Normal publication now
acquires the shared gate before candidate/destination filesystem inspection.
The lease covers validation, pinned-file sync, no-replace rename, all parent
syncs, final identity checking and descriptor closes. Admission waits directly
on the existing context-aware gate, with no waiter goroutine or NVR.mu held.
Existing finalization contexts still govern waiting/validation; contention can
consume that budget and leave candidates retained rather than publish them.

The durability algorithm is unchanged after an added lease check. File-sync
failure still prevents rename. Post-rename errors still leave the final and
return failure, without rollback or retries. Current recorder call sites enter
publication after FFmpeg Run/Wait has returned; validation FFprobe processes
remain inside publication. No RTSP capture or FFmpeg lifetime is newly locked.
No recordingControlMu, policy lock or registry lock is acquired by publication.

publishRecordingLeased/publishRecordingDurablyLeased borrow a held shared lease,
never reacquire it and never release it. They enable a later event caller to own
publication/checkpoint/release as one application boundary. The normal wrapper
must NOT be called while already borrowing a lease. The raw rename primitive
remains a low-level syscall helper; application callers use publication entries.

IMPORTANT: recorder.go and checkpoint ordering are unchanged in this commit.
Normal ownership ends when publication returns, BEFORE the existing published
checkpoint and staging release. Therefore the deletion/checkpoint/release gap
remains open, and no new staging-release capability or complete-capture proof is
introduced. Recursive releaseJob cleanup remains unresolved. Camera-registry
admission is not added here. Orphan cleanup remains outside the full boundary.
This is not whole-stage acceptance, hostile-writer protection, ancestor/mount
proof, power-loss proof or merge readiness.

Five test functions cover canceled admission, deletion exclusion through the
second parent sync (explicit and retention), borrowed-entry wiring with synthetic
probes, sync-failure lease release and invalid leases. Existing publication and
durability tests are unchanged. Tests are authored, NOT locally executed:
Go/gofmt/Docker unavailable. Blob/diff/JSON checks are preparation evidence only.
