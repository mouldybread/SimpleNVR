package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Done is observed when acquisition evaluates its blocking select. Holding the
// gate makes ordering deterministic; timeouts are failure guards, not evidence.
type mediaWaitObservedContext struct {
	context.Context
	waiting chan struct{}
	once sync.Once
}

func (ctx *mediaWaitObservedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestMediaOwnershipCanceledWaitReturnsWithoutOwnerRelease(t *testing.T) {
	gate := newMediaMutationGate()
	owner, err := gate.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	defer owner.release()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &mediaWaitObservedContext{Context: parent, waiting: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		lease, err := gate.acquire(ctx)
		if lease != nil { lease.release() }
		result <- err
	}()
	awaitShutdownStep(t, ctx.waiting, "waiter did not reach admission")
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) { t.Fatalf("admission error: %v", err) }
	case <-time.After(3*time.Second):
		t.Fatal("canceled waiter did not join while owner remained admitted")
	}
	if !owner.held() || len(gate.token) != 1 { t.Fatal("waiter disturbed owner") }
	owner.release()
	next, err := gate.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	next.release()
}

// Deterministically cancel at the post-admission Err check, after the token
// branch won. This exercises the ready-token/cancellation boundary without a
// probabilistic scheduling loop.
type mediaCancelOnAdmissionContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (ctx *mediaCancelOnAdmissionContext) Err() error {
	ctx.checks++
	if ctx.checks == 2 { ctx.cancel() }
	return ctx.Context.Err()
}

func TestMediaOwnershipCancellationAtAdmissionReturnsToken(t *testing.T) {
	gate := newMediaMutationGate()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &mediaCancelOnAdmissionContext{Context: parent, cancel: cancel}
	lease, err := gate.acquire(ctx)
	if lease != nil || !errors.Is(err, context.Canceled) || len(gate.token) != 0 {
		t.Fatalf("canceled admission leaked ownership: lease=%v err=%v tokens=%d", lease, err, len(gate.token))
	}
	lease, err = gate.acquire(parent)
	if lease != nil || !errors.Is(err, context.Canceled) { t.Fatal("pre-canceled admission succeeded") }
}

func TestMediaOwnershipExclusionAndIdempotentRelease(t *testing.T) {
	gate := newMediaMutationGate()
	first, err := gate.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	defer first.release()
	ctx := &mediaWaitObservedContext{Context: context.Background(), waiting: make(chan struct{})}
	admitted := make(chan *mediaMutationLease, 1)
	go func() { lease, _ := gate.acquire(ctx); admitted <- lease }()
	awaitShutdownStep(t, ctx.waiting, "second owner did not wait")
	select { case <-admitted: t.Fatal("overlapping admission"); default: }
	first.release()
	var second *mediaMutationLease
	select {
	case second = <-admitted:
	case <-time.After(3*time.Second): t.Fatal("second owner not admitted after release")
	}
	if second == nil { t.Fatal("missing second lease") }
	defer second.release()
	first.release()
	if first.held() || !second.held() || len(gate.token) != 1 { t.Fatal("duplicate release disturbed new owner") }
	second.release()
	if second.held() || len(gate.token) != 0 { t.Fatal("ownership not returned") }
}

func TestRetentionOwnershipWaitHoldsNeitherRegistryNorPolicy(t *testing.T) {
	n, _, _, target := retentionSnapshotFixture(t)
	gate := newMediaMutationGate()
	owner, err := gate.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	defer owner.release()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &mediaWaitObservedContext{Context: parent, waiting: make(chan struct{})}
	result := make(chan bool, 1)
	go func() { result <- cleanupRetentionSnapshotUsingGate(ctx, n, target, gate, func(context.Context, string, time.Duration) { t.Error("canceled deletion invoked") }) }()
	awaitShutdownStep(t, ctx.waiting, "retention did not wait for ownership")
	if !n.mu.TryLock() { t.Fatal("ownership wait holds registry lock") }
	n.mu.Unlock()
	if !retentionPolicyMu.TryLock() { t.Fatal("ownership wait holds policy lock") }
	retentionPolicyMu.Unlock()
	cancel()
	select {
	case invoked := <-result: if invoked { t.Fatal("canceled retention invoked deletion") }
	case <-time.After(3*time.Second): t.Fatal("canceled retention did not join")
	}
	if len(gate.token) != 1 { t.Fatal("canceled retention released another owner") }
}

func TestRetentionRevalidatesAfterOwnershipAdmission(t *testing.T) {
	for _, change := range []string{"remove", "replace", "id", "path", "policy", "shutdown"} {
		t.Run(change, func(t *testing.T) {
			n, cam, _, target := retentionSnapshotFixture(t)
			gate := newMediaMutationGate()
			owner, err := gate.acquire(context.Background())
			if err != nil { t.Fatal(err) }
			defer owner.release()
			ctx := &mediaWaitObservedContext{Context: context.Background(), waiting: make(chan struct{})}
			result := make(chan bool, 1)
			go func() { result <- cleanupRetentionSnapshotUsingGate(ctx, n, target, gate, func(context.Context, string, time.Duration) { t.Error("stale retention invoked deletion") }) }()
			awaitShutdownStep(t, ctx.waiting, "retention did not reach ownership wait")
			if change == "policy" {
				if err := setCameraRetention(n, cam.ID, 30); err != nil { t.Fatal(err) }
			} else if change == "shutdown" {
				n.shuttingDown.Store(true)
			} else {
				n.mu.Lock()
				switch change {
				case "remove": delete(n.cameras, target.id)
				case "replace": n.cameras[target.id] = &Camera{ID: cam.ID, RecordDir: cam.RecordDir, RetentionDays: cam.RetentionDays}
				case "id": cam.ID = "changed"
				case "path": cam.RecordDir = filepath.Join(cam.RecordDir, "changed")
				}
				n.mu.Unlock()
			}
			owner.release()
			select {
			case invoked := <-result: if invoked { t.Fatal("post-admission validation accepted stale state") }
			case <-time.After(3*time.Second): t.Fatal("retention did not finish after admission")
			}
			if len(gate.token) != 0 { t.Fatal("refused retention leaked lease") }
		})
	}
}

func TestRetentionIOBorrowsOwnershipWithoutRegistryLock(t *testing.T) {
	n, _, _, target := retentionSnapshotFixture(t)
	gate := newMediaMutationGate()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	result := make(chan bool, 1)
	go func() {
		result <- cleanupRetentionSnapshotUsingGate(context.Background(), n, target, gate, func(context.Context, string, time.Duration) { close(entered); <-release })
	}()
	awaitShutdownStep(t, entered, "retention I/O did not start")
	if len(gate.token) != 1 { t.Fatal("I/O did not retain media ownership") }
	if !n.mu.TryLock() { t.Fatal("I/O holds registry lock") }
	n.mu.Unlock()
	if retentionPolicyMu.TryLock() { retentionPolicyMu.Unlock(); t.Fatal("I/O lost policy synchronization") }
	once.Do(func() { close(release) })
	select {
	case invoked := <-result: if !invoked { t.Fatal("valid retention refused") }
	case <-time.After(3*time.Second): t.Fatal("retention callback did not join")
	}
	if len(gate.token) != 0 { t.Fatal("completed retention leaked lease") }
}

func TestRetentionLeasedHelperRejectsMissingOrReleasedLease(t *testing.T) {
	n, _, _, target := retentionSnapshotFixture(t)
	gate := newMediaMutationGate()
	lease, err := gate.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	lease.release()
	for _, invalid := range []*mediaMutationLease{nil, lease} {
		if cleanupRetentionSnapshotLeased(context.Background(), n, target, invalid, func(context.Context, string, time.Duration) { t.Error("invalid lease invoked deletion") }) {
			t.Fatal("invalid lease accepted")
		}
	}
}

func TestManagedRetentionUsesSharedOwnershipGate(t *testing.T) {
	n, _, _, target := retentionSnapshotFixture(t)
	owner, err := recordingMediaOwnership.acquire(context.Background())
	if err != nil { t.Fatal(err) }
	defer owner.release()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &mediaWaitObservedContext{Context: parent, waiting: make(chan struct{})}
	result := make(chan bool, 1)
	go func() { result <- cleanupRetentionSnapshotWith(ctx, n, target, func(context.Context, string, time.Duration) { t.Error("retention bypassed shared ownership") }) }()
	awaitShutdownStep(t, ctx.waiting, "managed retention did not enter shared admission")
	cancel()
	select {
	case invoked := <-result: if invoked { t.Fatal("managed retention bypassed shared ownership") }
	case <-time.After(3*time.Second): t.Fatal("managed retention waiter did not join")
	}
	if !owner.held() { t.Fatal("retention released another owner") }
}
