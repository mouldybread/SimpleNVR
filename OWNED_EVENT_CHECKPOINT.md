# Creator-owned checkpoint replacement, bounded subtask 6

Baseline: b66997ffd5fe6713fbc620ea6b2f2348275c01d6. Three additive files;
no production caller, existing writer, recorder or cleanup test is changed.
This is the checkpoint-writer prerequisite for later creator-side adoption.
It does not make b66997f's inventory part of the production job lifecycle.

writeOwnedEventRecovery borrows an active shared media lease and an inventory
whose methods are serialized by the caller. It never acquires/releases the gate
or takes registry/control/policy locks. It checks the existing checkpoint slot
against the previously recorded descriptor, refusing discovered, missing,
modified or replaced checkpoints and changed named root/parent identities.
Other inputs may still be active/unregistered: this is NOT full preflight.

A bounded payload is written to an exclusively created owner-only temporary
file through the pinned directory. The file is synced, a CLOEXEC creator-derived
descriptor is retained and the writer descriptor is closed before rename.
Both candidate and prior checkpoint are checked again. Initial publication uses
RENAME_NOREPLACE; replacement of a recorded checkpoint uses relative rename.
The pinned directory is synced before successful inventory replacement, with a
post-rename full file identity captured from the creator-derived descriptor.
Rename's legitimate ctime change is not compared to the pre-rename ctime.
The old inventory descriptor is closed when superseded; the persistent entry
limit remains maxRecoveryInputs+3. A write transiently adds at most two checkpoint-file
descriptors, in addition to the inventory's held descriptors.

Errors never remove temporary or final evidence or roll back a rename. After
rename, directory sync is still attempted despite cancellation. Failed writes
before inventory commit leave the old inventory unchanged (or the slot absent),
so leftover temporary/replaced checkpoint evidence refuses full preflight.
An old-descriptor close error after commit still returns failure: a future caller
must mark persistence failure/retain staging even if the new inventory is valid.
No filename discovery, implicit adoption or content/JSON validation is added.
Successful persistence or preflight grants NO release or complete-capture proof.

The gate coordinates cooperating application callers, not hostile external
writers. Checks plus replacement rename are not atomic compare-and-swap. No
ancestor/mount, blocked-kernel-I/O, crash/power-loss or transaction guarantee is
claimed. Kernel operations remain noninterruptible; context is checked between
phases. The existing writer and its on-disk schema remain unchanged.

Seven new test functions cover create/replace and descriptor lifetime, changed
ownership, write/short-write/file-sync/file-close/rename/directory-sync failures,
post-rename cancellation, foreign entries, persistent budget, invalid admission
and initial no-replace collision. Tests are authored, NOT locally executed:
Go/gofmt/Docker unavailable. Preparation checks are not build/race/vet evidence.

Remaining: creator adoption for prebuffer/capture/concat/checkpoint descriptors;
positive release eligibility; shared ownership through publication/checkpoint/
release; nonrecursive cleanup and checkpoint-last preservation/restoration;
slot-return, registry/orphan integration and exact-head acceptance. No production
staging-safety fix or merge readiness is claimed. Do not push until b66997f is
confirmed green and this exact payload receives separate content approval.
