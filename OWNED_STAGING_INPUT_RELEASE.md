# Owned staging input release and production retention

Current preparation base: c24bbe9196fa05500c4afbb76ef51744341adce5.

## Production default-retention follow-up

New jobs default to keepStaging=true. Terminal release always retains staging,
attempts the existing retained checkpoint, closes inventory descriptors and returns
capacity. A false legacy flag is never deletion authority, even after successful
publication or for manually constructed jobs. The event-release RemoveAll path
is removed. Capture/assembly, publication ordering and checkpoint schema are not
changed. Missing/failed checkpoints still retain bytes and return capacity.

Successful jobs intentionally retain their inputs and checkpoint until positive
cleanup authorization and a shared finalization lease are integrated. Staging
space therefore grows even for successful events and is not expired by published
retention. Monitor capacity and use existing admission limits; this change adds
no automatic staging deletion, expiry or emergency reclamation.

## Opt-in input-release primitive (c24bbe9)

The primitive remains without a production caller. It borrows one shared media
lease and a serialized creator-proven inventory, with an independent caller guard.
Inventory membership is not authorization. Full preflight precedes all unlinks;
only recorded event.mp4, concat.txt and pre_000.mp4 through pre_127.mp4 are accepted.
Each input rechecks directory/checkpoint/descriptor/name identity before relative
unlink. Unknown, changed or special entries refuse. The result reports actual
successful unlinks, not rollback. Removed pins close; directory sync is attempted
after any unlink despite later failure/cancellation. Final checks follow sync.
The checkpoint and root always survive. No recursive fallback or rmdir exists.

Trusted directories remain required: stat/unlink is not atomic against hostile
writers. Hard-link ctime changes conservatively refuse; no filename refresh is
allowed. Context does not make kernel I/O interruptible. Guard callbacks must not
reacquire the borrowed lease or wait for workers. Inventory teardown is serialized.

## Remaining integration and verification

Positive cleanup eligibility, publication -> published checkpoint -> release
ownership, production engine activation, checkpoint-last/final empty-directory
handling and integrated successful-cleanup acceptance remain open. Default
retention alone does not close the ownership subsection.

Existing failure/publication assertions are preserved; tests now expect retained
unprocessed and successfully published staging. A false-flag terminal-release
table covers fresh/manual jobs, unknown/nested evidence, failed checkpoints,
inventory closure and slot return. Existing Safety verification discovers these
regressions through its full-package race tests; workflows are unchanged.
