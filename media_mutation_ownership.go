package main

import (
	"context"
	"sync"
	"sync/atomic"
)

// Global admission for application media mutations. Adoption is incremental:
// currently only managed retention passes enter this gate. Publication and
// other deletion paths MUST adopt it before it protects staging release.
var recordingMediaOwnership = newMediaMutationGate()

type mediaMutationGate struct {
	token chan struct{}
}

type mediaMutationLease struct {
	gate *mediaMutationGate
	once sync.Once
	active atomic.Bool
}

func newMediaMutationGate() *mediaMutationGate {
	return &mediaMutationGate{token: make(chan struct{}, 1)}
}

// Lock order for adopters: recordingControlMu (if needed), media ownership,
// retentionPolicyMu (if needed), then NVR.mu for brief registry checks only.
// Never wait for admission while holding NVR.mu or retentionPolicyMu.
// Camera deletion must cancel/join workers BEFORE admission. Workers must
// never acquire recordingControlMu while holding a lease. Do not hold a lease
// across RTSP capture or an FFmpeg process lifetime.
//
// The acquisition layer owns release. Leased helpers must not reacquire this
// non-reentrant gate. A lease must not be copied or released while borrowed.
func (g *mediaMutationGate) acquire(ctx context.Context) (*mediaMutationLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case g.token <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// A ready token can win select simultaneously with cancellation. Admission
	// does not authorize a destructive callback until cancellation is rechecked.
	if err := ctx.Err(); err != nil {
		<-g.token
		return nil, err
	}
	lease := &mediaMutationLease{gate: g}
	lease.active.Store(true)
	return lease, nil
}

func (lease *mediaMutationLease) held() bool {
	return lease != nil && lease.gate != nil && lease.active.Load()
}

func (lease *mediaMutationLease) release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		lease.active.Store(false)
		<-lease.gate.token
	})
}
