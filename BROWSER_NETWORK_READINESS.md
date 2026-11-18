# Browser startup network readiness

Based on 6627e6dfdfdd85c9e8d2e268f6664c0fc7bd1eda; CI-only mitigation,
not proof of the ERR_NETWORK_CHANGED root cause or a production networking fix.

After HTTPS startup, resolve the host peer index from container eth0/iflink.
Before launching the unchanged browser driver, require the peer link up/carrier,
complete route/resolver/address inspection, no tentative IPv6 address, no failed
DAD, and two observed quiet seconds, within a 20-second readiness deadline.
An enabled IPv6 peer without any address remains pending; an already-disabled
peer is observed without changing its configuration. Structural routes ignore
reference/use counters. Address values are hashed, not emitted. Any monitored
link/address/route notification resets quiet time, even on another interface.
Missing monitoring, lost evidence or incomplete inspection fails readiness.

Parent monitoring continues through the driver in startup-network/; the child
retains its existing CDP evidence separately. network-readiness.json records
readiness and driver launch times. A driver-launch marker is not an exact Chrome
process-start timestamp; the parent and child artifacts have separate elapsed
origins. A quiet window does not prevent a later network change.

The driver retains its 300-second limit; the outer bound allows readiness and
TERM/KILL cleanup. SIGTERM cancels the wrapper, which cleans up a live driver
process group and waits for its direct child. Uninterruptible OS calls and all
descendant-reaping guarantees are not solved here. No authentication retry,
IPv6/DAD disabling, browser/version/flag change or acceptance assertion change.
Both independently named browser runs and the aggregate gate remain mandatory.

Sixteen new readiness tests executed locally, including a real local Python
child termination/join test; not a real Chrome, Docker, Go or GitHub CI run.
Existing diagnostic tests are unchanged and remain mandatory in core. Named
branch-protection checks, if configured, may need separate reconciliation with
new display names; no protection policy is modified here.
