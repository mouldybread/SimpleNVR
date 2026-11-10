# Timeline window API migration

GET /api/timeline/recordings accepts either:

- Exactly one start_time and one end_time, both RFC3339 timestamps with explicit offsets or Z. Fractional seconds are supported. The end must follow the start and the elapsed window must not exceed 26 hours. The server compares absolute instants in UTC; it does not reinterpret these timestamps in Hong Kong or the server-local zone.
- A legacy date=YYYY-MM-DD request, interpreted as a Hong Kong calendar day. With no parameters, the default is the current Hong Kong day. Invalid dates now return 400 rather than an apparently successful empty result. This date path is compatibility-only.

Mixed date/absolute requests, duplicate boundary parameters, missing endpoints, malformed timestamps, reversed/empty ranges and oversized windows return 400.

Responses remain JSON arrays. X-Timeline-Window-Start and X-Timeline-Window-End expose normalized UTC boundaries; X-Timeline-Window-Mode is absolute or legacy-hkt. The current selection rule includes file start timestamps at the window start and excludes starts at the window end.

This is the backend prerequisite, not the completed browser migration. The browser still sends date requests until the next client patch. That patch must construct local midnight and the following local midnight using calendar arithmetic, then send both as absolute timestamps; adding a fixed 24 hours is incorrect across DST transitions. A 23- or 25-hour request is accepted without server time-zone inference.

## Known limitations

X-Timeline-Duration-Policy remains estimated-600s. End times are still ten-minute estimates, files beginning before the window are still omitted even if their media overlaps it, and export selection still uses estimated durations. Actual-duration probing/caching and interval-overlap selection are separate work. This patch does not correct the canvas's fixed 24-hour display or repeated-hour ambiguity, export trimming/gaps, or live-camera timestamp drift.

Filesystem enumeration no longer holds the NVR state lock. Regular-file checks skip media symlinks, but this is not complete protection against hostile concurrent filesystem replacement. Tests use filename fixtures, not proof of media coverage or actual playable duration. Keep the audit branch unmerged pending the remaining fixes and final re-audit.
