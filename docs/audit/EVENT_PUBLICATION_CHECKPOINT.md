# Event finalization ownership through terminal retention

Preparation base: 3681b7dd8542b18aa6c1714b602bdf0fcbfc9678, user-reported green.

## Ownership lifetime

After capture/concat Run has joined, event finalization acquires one shared media
lease for durable publication and the published checkpoint. If publication returns
success, acquisition ownership is transferred to the serialized job owner, including
when that published checkpoint failed. The job's recoveryBorrow is marked terminal;
checkpoint writers still only borrow, never release or reacquire the lease.

The normal worker's deferred releaseJob keeps that same lease through unconditional
retention, the retained checkpoint, inventory closure and capacity return. Only then
does it clear/finish the handoff and release ownership. There is no admission window
between publication, published checkpoint and terminal retention. Publication errors
still release locally and follow the existing independent failure-checkpoint path;
post-rename errors remain ambiguous and never imply rollback.

Every direct private finalization caller owns the same releaseJob obligation and
must return a reserved slot. Successful finalization must be followed by terminal
release, not another capture or concat. Media entry helpers explicitly refuse an
active recoveryBorrow before creation, gate admission or process execution. The
job is single-use, serialized and not concurrently closed or handed to another
worker. No goroutine, automatic expiry or abandoned lease waiter is introduced.

## Cancellation and metadata budgets

concatenateEventRecording cancels its publication context when it returns. Lease
ownership does not automatically end with that context. Terminal retention therefore
uses its established independent Background-rooted recordingFinalizeGrace budget
while borrowing the same held lease. Capture/manager cancellation does not prevent
that metadata attempt. A failed/expired published checkpoint still latches failure;
terminal release does not retry or clear the failure latch. Retention-write failure
still closes descriptors and returns capacity/ownership. Kernel I/O is not made
interruptible. Direct callers must not park a finalized job indefinitely.

File Sync, no-replace rename, directory Sync, published-path return on checkpoint
failure, partial-media preservation and publication validation are unchanged.
FFprobe validation remains inside publication ownership as before; capture and
concat FFmpeg execution remain outside it. Injected writers retain their test
compatibility and are not converted into proof of descriptor-owned persistence.

## Remaining safety work

All terminal jobs STILL retain staging. No cleanup capability is minted, no input-
release engine is enabled, and checkpoint/root removal is absent. The existing
keepStaging calculation is compatibility state, not deletion permission. Positive
complete-capture eligibility, registry/shutdown/output revalidation and orphan/add-
camera coordination are not closed by this lease-lifetime batch. No whole-stage,
hostile-writer atomicity or power-loss guarantee is claimed.

Existing publication/checkpoint tests now assert terminal ownership transfer and
terminal release instead of early lease release. Four new regression functions
cover a competing owner blocked during terminal persistence, caller cancellation
with an independent retained-checkpoint budget, terminal write failure with slot/
lease/descriptor return, real concat-to-release handoff using synthetic process
fixtures, and media reentry refusal. Existing capture, canceled queues, active-
recorder joining and storage-failure cases remain in the full Actions race suite.
No workflow change is needed; both browser runs and aggregate verify stay required.
