# Native event observer bootstrap correction

Base: 227798c7a5d13af66f9af33699b9acccadf1bb74, user-reported failed.
The supplied ac3f822 traces reached the second check_playing assertion: expected
source/clock/unpaused/intent/error checks passed, but recorded trusted metadata and
playing witnesses were missing. Baseline playback/exports and eleven tooling tests
passed in those logs. No detailed 227798c trace was supplied for this preparation;
its exact failure location is not independently confirmed. Do not call it green.

This batch corrects a concrete observer lifecycle weakness. It does not claim that
absence of script execution was proven from the earlier traceback alone, or that
this is the only possible issue. Per-event paths/trust and the native CI results
remain material. No media event, play promise, response or decoder is fabricated.

## Correction

Use page.add_init_script before navigation instead of a raw CDP Page-domain init
registration. Continue using CDP Runtime.evaluate with userGesture=false for setup
observations. Installation publishes a versioned application-document marker only
after all five capturing listeners are attached. Publish a dropped-event counter.
Check installation after navigation and again after timeline readiness, before any
acceptance action. No Playwright evaluate call is added to the unactivated autoplay
page. Bootstrap failures close the created page, including registration failures.

Snapshots carry observer identity/state and an explicit null for a missing buffer,
not a fabricated empty event list. Held-load, playback and denial checks require a
valid installed marker, zero dropped events and a bounded event buffer. A valid
empty buffer is permitted during the held request, but still cannot pass playback's
trusted loadedmetadata/playing assertions. Missing, untrusted, wrong-source or lost
witnesses remain failures; native decoded-frame and no-activation checks remain.
The marker is test instrumentation, not authenticated recording/cleanup evidence.

## Regression and execution

Retain the original eleven test methods with installation-aware fixtures. Add five
methods covering eleven malformed/missing observer cases, installed-empty versus
absent observations, registration-before-navigation, no user activation on handshake,
fail-fast missing bootstrap, and registration-failure page closure. Existing shell
discovery and both Chrome jobs execute these regressions and the native scenarios.
Grammar and patch round-trip preparation checks are not browser acceptance.

This is three files: the edge helper, its regressions and this document. No production
frontend/backend, workflow, required check, timeout, browser flag, media command,
cleanup, main, merge or deployment change. Exact-candidate CI remains required.
