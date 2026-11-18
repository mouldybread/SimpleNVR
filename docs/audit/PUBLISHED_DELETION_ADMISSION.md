# Explicit published deletion, subtask 2

Baseline: 9bc8a4f87be76b912b8e911a3464f45e3d2dc194. The production
/api/recordings/delete-all handler now selects the leased engine path. Other
playback/export code is unchanged. Legacy pathname/injection helpers remain for
existing callers/tests; this HTTP entry point no longer invokes them.

The HTTP layer holds recordingControlMu, snapshots targets under brief NVR.mu,
and visits them in camera-ID order. Each target acquires the shared media lease
without NVR.mu, then revalidates context, shutdown, registry pointer, ID and path.
The engine receives that guard for its own rechecks and borrows the lease without
reacquisition. The admission layer releases it after engine cleanup on success
or failure. No media ownership spans capture or a worker join in this path.

Only top-level regular published .mp4 files are selected, including partials.
Staging/.tmp remains opaque. The existing JSON fields and partial-success
accounting remain; failures from the new engine count errored camera passes,
not every undeleted file. The engine fails fast within a camera; later valid
camera targets may still run. Cancellation/shutdown stops further admission.
Late arrivals outside a candidate snapshot remain; success is not a claim that
an actively written directory is empty. No registration/configuration is deleted.

Six new test functions include six blocked-admission state/cancel cases, guard
propagation, injected slow I/O, HTTP partial results, the real handler dispatch,
bulk partial success and method/shutdown/error boundaries. Tests are authored,
not locally executed: Go/gofmt/Docker unavailable. Patch/blob/JSON checks are
preparation only. Retention wiring is the next separate subtask. Publication,
checkpoint and staging release still do not share the complete boundary; no
hostile-writer, mount/ancestor, atomic-unlink, transaction or merge claim is made.
