# Prebuffer evidence preservation

Prepared against audit head 389974633b3d069f1cca4c8c82b63ead1555c6e3.

## Finding and scope

Both startup recovery entry points recursively removed `buffer/`. Event recorder
shutdown also recursively removed it. Segmenter-exit cleanup unlinked files with
no completion-list entry, even when those files contained capture bytes. These
operations could erase unreported or prior-process prebuffer evidence without a
recovery decision. A playable published output does not authorize that erasure.

## Changed behavior

- Startup recovery leaves `buffer/` and `temp/` entries unchanged, including
  unexpected nested contents and stable symlinks. Presence and inspection errors
  are logged without following entries or interpreting capture completeness.
- Event recorder shutdown releases its in-memory buffer index after child join,
  but does not recursively delete the stopped buffer directory.
- A new BufferManager starts with an empty index. Old files are not silently
  adopted into event capture or aged out by the new manager's prune loop.
- After the segmenter has joined, nonce-scoped unreported nonempty files,
  nonregular entries, and entries whose metadata cannot be inspected are kept.
  Only nonce-scoped, observed zero-length regular outputs remain disposable.
- Event recorder initialization can reuse real existing staging directories,
  but rejects stable symlinks or nondirectory entries rather than recording
  through them. Existing contents are not deleted or chmodded.

Active rolling-buffer pruning of this manager's registered completed segments
is unchanged. Event checkpoint writing, failed-event pin retention, successful
assembly cleanup, publication validation, and continuous-recording recovery are
unchanged. This is not automatic event recovery or capture-completeness proof.

## Capacity and operator boundary

Retained prebuffer bytes can accumulate across failures, stops, and restarts.
There is no new expiry, automatic reassembly, deletion endpoint, or implicit
permission to discard those bytes. Preserve/copy evidence before making an
explicit operator cleanup decision. Existing free-space admission remains
necessary; event-staging quotas do not comprehensively account for `buffer/`.
Retention/capacity management for these retained files remains an open item.

Camera-directory parents and mounts must remain operator-controlled and stable.
The initialization Lstat check and empty-file unlink are not atomic against
hostile concurrent pathname replacement. Blocking filesystem calls are not made
interruptible by this change. Recursive camera-delete handlers remain a separate
open destructive path, not fixed or endorsed by this preservation patch.

## Regression tests and evidence

`prebuffer_preservation_test.go` covers both startup entry points, stable buffer
symlinks, stopped indexed/unowned prebuffers and hard-linked event inputs,
non-adoption by a fresh manager, unreported nonempty versus empty nonce-scoped
outputs, unexpected directories/links, invalid glob preservation, and staging
initialization rejection/reuse. Tests use temporary synthetic bytes, not claims
about real FFmpeg capture, media validity, or live camera completeness.

The active-container fixture now requires fresh segment files in both cycles,
retains an unreported prebuffer sentinel, and hashes prior-cycle prebuffer files
across restart. The obsolete assertion requiring deletion of buffer/ is replaced
with positive byte-preservation and continued-capture assertions.

Go build/test/vet/race and runtime integration have not been executed by the
assistant. Run the normal safety verification on the resulting commit, including
the existing event-failure, storage, and active-container shutdown fixtures.
The local browser/export pass supplied for 3899746 is retained as historical
baseline evidence, not a pass for this changed tree. No merge clearance is given.
