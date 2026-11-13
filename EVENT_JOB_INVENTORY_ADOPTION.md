# Production job inventory adoption, slice 1

Baseline: 3bade9c9983e2a139449bed161cec67f45ff559c. One existing file changes:
recorder.go adds the inventory field and dispatches pinBuffer/releaseJob into
focused helpers. Existing capture, concat, publication and tests remain unchanged.
This slice connects real jobs, prebuffer pins and every normal recovery checkpoint
to creator-derived inventory evidence. Capture/concat descriptor adoption is NOT
included, so checklist point 1 is only partially integrated by this batch.

Pinning preserves the existing selected-input snapshot, pruning read lock,
manifest state/revision sequence and link_failed diagnostic. A selected source
is opened nonfollowing/nonblocking, linked relative to the pinned root, then
Remember verifies that new entry against the pin-operation descriptor. Source
handles close on every return; inventory duplicates persist only until terminal
release. Pin/setup errors close the inventory but never delete evidence.

Initial checkpoints borrow the one setup lease; subsequent checkpoints acquire
a fresh bounded lease through the existing recoveryWriter seam. Metadata
finalization deliberately uses an independent recordingFinalizeGrace budget so
capture/queue cancellation can still persist retained checkpoints, as before.
This is not permission to capture or clean up after cancellation. Waiting on the
media gate observes that operation context; kernel calls are not interruptible.
The buffer read lock is acquired before media admission; no adopter may wait for
bufferMutex while holding media ownership. No NVR.mu is held by these helpers.
No media lease spans capture, FFmpeg, or the job's lifetime.

Terminal release defers inventory closure and capacity return independently of
checkpoint/cleanup results. Closed production jobs retain a refusing writer,
never fall back to filename-based checkpoint adoption. Manually constructed
legacy jobs and injected writers retain their existing compatibility behavior.
The inventory methods remain serialized by the job owner.

IMPORTANT: keepStaging defaults and recursive success cleanup are unchanged.
Inventory membership never grants cleanup eligibility. Existing cleanup tests
are not reconciled here. Publication/checkpoint/release still lack one continuous
lease. Pruning can change a hard-linked inode's ctime and conservatively invalidate
its snapshot; no implicit refreshing/readoption is introduced. Unknown entries
remain opaque; no new orphan or published-file deletion policy is added.

Seven test functions cover production pin/checkpoint adoption, superseded and
terminal descriptor closure, pin-error progress, canceled queued/active workers,
checkpoint failure/slot return, canceled media waits and pin-name refusal. They are authored,
not executed: Go/gofmt/Docker unavailable. Preparation checks are not build/race/
vet/browser/container or final acceptance evidence. No whole-stage or merge claim.
