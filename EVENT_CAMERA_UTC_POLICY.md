# Camera-capture UTC policy - superseded

Status: SUPERSEDED by [EVENT_NVR_INGESTION_POLICY.md](EVENT_NVR_INGESTION_POLICY.md).

The user changed the selected clock from camera-capture UTC to NVR ingestion time
on November 14, 2026. Camera-capture UTC is no longer the required meaning of
start_time. The custom-FFmpeg proposal is withdrawn; use an unmodified toolchain.

This supersedes the policy recorded in commit
73ef2430556cccf0b5527cdd0c0e8556edd02a5b. The original decision and camera-UTC
feasibility findings remain available in that commit's history; they are not active
requirements for the selected NVR-ingestion timeline.

This replacement does not implement ingestion timing, certify existing recordings,
change publication or failure preservation, or authorize cleanup. See the active
policy for implementation boundaries and outstanding qualification work.
