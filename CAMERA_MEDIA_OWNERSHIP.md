# Camera deletion admission under shared media ownership

Baseline: 955fd63eb5e9dff5b33565ad1a5c04a4dac2fc64. Bounded follow-up to
retention admission, not completion of the ownership/staging safety stage.

Managed camera deletion still holds recordingControlMu for API serialization.
It validates the target, cancels and joins its recorder, and persists stopped
intent BEFORE acquiring media ownership. A finishing worker is therefore not
joined while deletion holds the media lease it may need. Admission is directly
context-cancelable with no waiter goroutine. NVR.mu is not held while waiting.

After admission the existing guard rechecks cancellation, shutdown, registry
pointer, camera ID and recording path. The callback borrows ownership and must
not reacquire/release it or take recordingControlMu. A per-target deferred release
covers guard refusals, removal failures, final-save failures and success. The
lease spans destructive removal, registry removal and final persistence, so
managed retention cannot pass its admission boundary during those phases.
The reviewed final-save order is media -> persistenceMu -> brief NVR.mu; the
current persistence implementation does not acquire media ownership.
Each completed bulk target is still saved before the next; removal failures
remain nontransactional and may follow partial unlinking. A failed final save
does not pretend to restore files or registration. Stopped intent is saved even
when later admission is canceled. Save failure before admission prevents unlink.

The descriptor-relative filesystem engine and its evidence refusals are
unchanged byte-for-byte. No recursive traversal, staging expiry, retention-policy
change, capture change or browser-CI change is introduced. Direct low-level
filesystem helper calls are not newly gated; production managed deletion owns
the acquisition layer. Existing test callbacks follow the same borrowed contract.

Six new test functions include five post-admission revalidation cases and two
terminal-error cases. Channels control ordering; timeouts only bound failures.
The finishing-worker test models lease use during join, not actual publication.
Slow injected removal/final-save tests check registry availability and retention
exclusion. Tests are authored, NOT executed here: Go/gofmt/Docker unavailable.
Patch/JSON/blob checks are preparation evidence, not build/race/vet evidence.

Publication, published checkpoints, staging release, explicit published deletion
and orphan/registration reconciliation still need coordinated adoption. Recursive
releaseJob cleanup remains open. A07/A08/A09/A10 have only bounded partial
coverage here; the full A01-A12 acceptance matrix is not satisfied. This is not
merge readiness or protection against hostile filesystem writers, mount/ancestor
replacement, uninterruptible I/O, compare-and-unlink races or power loss.
