# Publication-derived event output evidence

Preparation base: 38ac8e36f253ce9f750c2361edd10d2f1127ac39. Latest reported-green
checkpoint is d871167; a result for 38ac8e3 has not yet been reported.

## Production change

Event publication now uses a borrowed observer entry point. Only after successful
file Sync, no-replace rename, all parent-directory Syncs and the existing published
identity/context checks does the publication engine lend its checked output and
destination-directory descriptors to that observer. Legacy publication APIs retain
their signatures, positional durability-ops shape, ordering and nil-observer path.
Cross-directory publication lends the destination pin, not the source directory.

The observer retains two CLOEXEC duplicates and records post-rename file identity
and directory identity. It never reopens a discovered published name to establish
provenance. The new check method compares the held file with the pinned-directory
nonfollowing named entry (device/inode, mode/owner, size, mtime and ctime), and checks
the destination directory against its held descriptor and pathname identity. This
is metadata identity evidence, not a content hash or hostile-writer guarantee.
Directory size/mtime/ctime are not treated as immutable because legitimate sibling
changes modify them. File identity is not refreshed to accommodate hard-link ctime
changes; inspection refuses conservatively.

Evidence travels with the existing terminal lease handoff. A failed publication,
including a deferred close error after observer success, closes and clears retained
output pins locally without deleting the final file or claiming rollback. Successful
publication keeps pins through published/retained checkpoint processing, including
checkpoint failure, then closes them during terminal teardown before capacity and
media ownership are returned. Close errors do not prevent capacity/lease return.
Fake injected publication success produces no receipt and must never be promoted
into ownership proof. Manual/injected writer compatibility otherwise remains.

Two additional persistent descriptors are retained per finalizing job; production
job count is already bounded by event slots. Constructor duplicates are closed on
partial failure. The inventory entry limit and all input/checkpoint pins are
unchanged. Evidence methods and teardown are serialized; no new goroutine, pathname
adoption, deletion, expiry, workflow or metadata schema is introduced. Event output
evidence requires an absolute clean output path, as registered camera paths already
do. Private-media serving, continuous recording and ordinary publication callers
remain on their existing APIs.

## Still not cleanup eligibility

No staging deletion is enabled. An unchanged durably published output can still be
a partial, a short capture or otherwise insufficient for input release. Positive
complete-capture/assembly evidence, a persisted published checkpoint, live registry/
shutdown/cancellation checks, full input preflight and partial-receipt handling must
all be integrated before the opt-in release engine can run in production. A valid
output receipt is one necessary observation, not a cleanup capability. There is no
claim of ancestor/mount protection, power-loss proof or atomic external-writer
coordination. Context checks do not interrupt kernel I/O.

## Regression coverage

Nine functions cover source-descriptor closure versus retained pins/CLOEXEC,
replacement/symlink/FIFO/directory/missing/in-place mutation and camera-directory
replacement refusal; refusal to re-adopt a replacement introduced at observer entry;
transient-clone closure on injected duplicate failure; observer ordering and skipped
observation on file/rename/directory durability errors; production lifetime through
terminal success/checkpoint failure; late publication-close errors; context/lease
refusal; and cross-directory destination provenance. Files and accepting probes
are synthetic control-flow fixtures, not camera footage or new media acceptance.
Existing full-package GitHub Actions race/build/vet and both browser/verify jobs
remain unchanged and discover the added tests automatically.
