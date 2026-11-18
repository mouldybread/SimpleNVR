# Atomic no-replace recording publication

## Reason for this batch

The previous publishRecording implementation checked the destination with Lstat, validated the temporary media, checked the destination again, then used os.Rename. Another publisher or operator could create a destination between the last check and rename; ordinary replacement rename could overwrite it. Random recording IDs reduce accidental collision probability but do not enforce evidence preservation.

The public publisher now delegates to per-call validation/publication operations. Production still uses validateRecording and the real filesystem publisher. No process-global test hooks or production media-validation bypass are introduced.

## Publication contract

The final publication operation is Linux renameat2 with RENAME_NOREPLACE. Existing destination files, symlinks or directories are not replaced by that operation. The preliminary destination check is only an early rejection optimization, not the safety boundary. Late collisions return an os.LinkError wrapping EEXIST; os.IsExist/errors.Is remain usable by callers.

There is deliberately no fallback to os.Rename or a check-then-rename sequence. Unsupported kernels/filesystems (including ENOSYS/EINVAL/EOPNOTSUPP), cross-filesystem moves, permissions, read-only storage, quota/space failures and other rename errors return failure. This is an intentional compatibility restriction: deployment validation must establish that the actual recording filesystem supports no-replace rename. A system can otherwise retain candidates instead of publishing them, which consumes space and requires operator attention. This patch does not delete retained evidence to relieve that pressure.

The publisher checks cancellation before work and after validation, requires a nonempty regular temporary file, and rechecks its device/inode identity, regular type, size and modification time after validation. A detected source change fails before rename. Cancellation is cooperative: a cancellation racing the final syscall can still coincide with completed publication. There is no rollback or deletion of a successfully published destination.

Source revalidation is not descriptor-pinned publication. A source pathname can still be externally replaced after the last check, and in-place writes or hostile parents are not fully excluded. The guarantee added here is atomic destination no-clobber, not a complete filesystem sandbox or proof that every published byte is the probed byte. Do not interpret it as solving those remaining review items.

## Caller and failure behavior

Continuous capture, event assembly, context-aware startup recovery and the legacy helper all reach the same public publisher and therefore receive the no-replace operation. Existing callers continue to retain nonempty failed root temporary outputs; event failure staging follows EVENT_FAILURE_PRESERVATION.md and EVENT_RECOVERY.md. Successful complete event assembly still follows its existing staging-removal policy; published partials still follow normal published-media retention. No new retry, filename alteration, recovery action, expiry or deletion policy is introduced.

The publication helper never removes candidates as error cleanup and never retries using weaker semantics. On real filesystem/storage faults, particularly remote-filesystem or hardware failures, an error does not prove the namespace is unchanged: operators must inspect both source and destination. Injected errno tests are not a physical disk-full/power-loss simulation.

No new media fsync or directory fsync is added. Atomic namespace replacement is not proof of durable media, power-loss recovery, requested duration, every-frame validity or complete footage. Existing validation only establishes a video stream and readable packet. Synchronous filesystem calls can exceed cooperative deadlines; the shutdown budget must continue to report failure rather than claim completion.

## Regression scope

New Go tests exercise destination creation precisely at the final publication boundary, 16 competing real no-replace renames, existing symlink/directory destinations, injected ENOSPC/EDQUOT/EIO/EROFS/EACCES/EXDEV/ENOSYS/EINVAL/EOPNOTSUPP failures, cancellation during validation and changed source identity/size/type. Failure tests verify preserved fixture bytes and absence of fallback retries. A real-FFmpeg synthetic-media test reaches the public publisher, checks byte-preserving publication and verifies a subsequent collision leaves the candidate intact. Existing real-media cancellation/publication tests remain applicable.

The existing Safety verification go test -race -count=1 -timeout=120s ./... step automatically discovers these tests; no separate workflow or dispatch is needed. SIMPLENVR_REQUIRE_FFMPEG=1 makes missing FFmpeg/FFprobe a failure in the existing media test helper.

During preparation, a separate Python/ctypes native Linux renameat2 probe produced exactly one winner among 16 competing publishers, preserved all 15 loser source files, and rejected existing symlink/directory destinations with EEXIST. This validates only that native primitive on the preparation filesystem, not the Go implementation, repository tests, deployment mount or media behavior. The assistant has not compiled or executed the Go tests; Go is unavailable in the preparation environment. No CI result for this batch is claimed. Keep the audit branch unmerged under AUDIT_ACCEPTANCE.md.
