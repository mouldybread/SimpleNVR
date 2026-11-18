# Conservative retention for name-keyed events

Event records currently carry camera_name, not a stable camera ID. When several registered cameras share that name, event metadata cleanup now uses the longest validated retention among the matching cameras. It no longer overwrites the name's policy according to camera-map iteration order. Legacy zero-valued retention still contributes its seven-day default. A longer policy for an unrelated camera name has no effect.

If any matching camera has an invalid runtime policy, event metadata for that shared name remains preserved under the existing fail-closed rule. Otherwise metadata expires only once older than the longest matching policy. Events with no matching registered camera retain the existing orphan-event cleanup behavior.

Only metadata expiry changes here. Published media still uses each camera directory's own validated retention and the snapshot/serialization protections in RETENTION_SNAPSHOT.md. This patch does not resolve ownership of legacy events after camera renames, deletion/name reuse, or duplicate names. It does not migrate event records to camera IDs or make ambiguous event-trigger/navigation requests acceptable.

The production wrapper still persists changes with the same NVR.save callback. Tests inject persistence to avoid writing the real configuration path and cover duplicate policies, legacy zero, unrelated names, expired/orphaned metadata, invalid-policy precedence and cancellation. Event pruning and policy reads remain within the registry lock; a retention increase cannot restore metadata pruned before that update took effect. Keep the audit branch unmerged pending remaining work and final review.
