package main

import "context"

// Register before launching so beginShutdown cannot miss an admitted worker.
// The existing shutdown join includes this worker before final persistence.
func (g *shutdownGate) startBackgroundJob(work func(context.Context)) bool {
 if work == nil { return false }
 g.mu.Lock()
 if g.closing { g.mu.Unlock(); return false }
 ctx, cancel := context.WithCancel(context.Background())
 g.nextJob++
 id := g.nextJob
 g.jobs[id] = cancel
 g.mutations.Add(1)
 g.mu.Unlock()
 go func() {
  defer func() {
   cancel()
   g.mu.Lock()
   delete(g.jobs, id)
   g.mu.Unlock()
   g.mutations.Done()
  }()
  work(ctx)
 }()
 return true
}
