# Timeline window API migration

GET /api/timeline/recordings accepts either:

- Exactly one start_time and one end_time, both RFC3339 timestamps with explicit offsets or Z. Fractional seconds are supported. The end must follow the start and the elapsed window must not exceed 26 hours. The server compares absolute instants in UTC; it does not reinterpret these timestamps in Hong Kong or the server-local zone.
- A legacy date=YYYY-MM-DD request, interpreted as a Hong Kong calendar day. With no parameters, the default is the current Hong Kong day. Invalid dates return 400 rather than an apparently successful empty result. This date path is compatibility-only; the current browser no longer uses it.

Mixed date/absolute requests, duplicate boundary parameters, missing endpoints, malformed timestamps, reversed/empty ranges and oversized windows return 400.

Responses remain JSON arrays. X-Timeline-Window-Start and X-Timeline-Window-End expose normalized UTC boundaries; X-Timeline-Window-Mode is absolute or legacy-hkt. The current selection rule includes file start timestamps at the window start and excludes starts at the window end.

## Browser migration

The browser now constructs the selected local calendar day's midnight and the following local calendar day's midnight independently, converts both to UTC ISO timestamps, and sends start_time/end_time without date. It does not add a fixed 24 hours or ask the server to infer a browser time zone. Request-version and cancellation guards remain in place. An invalid day fails before a network request; failures do not silently fall back to the old Hong Kong interpretation.

The frontend regression suite checks exact outgoing UTC boundaries in UTC, Asia/Hong_Kong and America/Chicago, including Chicago's 23-hour spring day and 25-hour fall day. It also covers midnight-adjacent event navigation, year rollover, invalid dates, stale responses, cancellations and superseded seeks. These use a stubbed DOM/network and do not establish real-browser or live-camera playback correctness.

Deploy the backend absolute-window support before the new browser assets. Stale cached browser assets may still use date and retain legacy HKT behavior; reload the page after updating. The explicit-window request migration is implemented, but this is not completion of the broader timeline audit.

## Known limitations

X-Timeline-Duration-Policy remains estimated-600s. End times are still ten-minute estimates, files beginning before the window are still omitted even if their media overlaps it, and export selection still uses estimated durations. Actual-duration probing/caching and interval-overlap selection are separate work. This patch does not correct the canvas's fixed 24-hour display or repeated-hour ambiguity, export trimming/gaps, or live-camera timestamp drift.

Filesystem enumeration no longer holds the NVR state lock. Regular-file checks skip media symlinks, but this is not complete protection against hostile concurrent filesystem replacement. Backend tests use filename fixtures, not proof of media coverage or actual playable duration. Keep the audit branch unmerged pending the remaining fixes and final re-audit.
