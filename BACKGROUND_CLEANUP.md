# Background retention cleanup shutdown

The server registers periodic cleanup with shutdownGate before launching its goroutine. Shutdown closes admission, cancels this worker alongside tracked media jobs, and joins it through the existing mutations wait group before final persistence. Listener failures use the same shutdown path. Late background job registration is rejected.

Cleanup checks cancellation before each cycle, during event filtering, between cameras and between published-file deletions. Camera retention targets are snapshotted under the registry read lock; filesystem retention work runs after that lock is released. The worker operates on its supplied NVR instance rather than requiring the package-global registry.

Cancellation is cooperative: it cannot interrupt an already blocked filesystem call, save, or mutex acquisition, and a cancellation can race the last check before a removal. The shutdown join still waits for the worker; the existing overall shutdown deadline reports failure rather than claiming completed cleanup. SIGKILL/power loss and hostile filesystem replacement are not solved.

Root .tmp files and nested recovery staging remain preserved. Published root .mp4 retention, including published partial files, remains unchanged. This does not add staging expiry, quotas, free-space admission, a durable recovery manifest, or a complete disk-full policy. Existing manual orphan cleanup is unchanged.

Tests cover background cancellation and joining before persistence, rejection of late work, canceled ready ticks, and a real cleanup cycle preserving staging while retaining published-media deletion policy. Full live-camera/container shutdown and filesystem-stall integration remain separate validation. Keep the audit branch unmerged.
