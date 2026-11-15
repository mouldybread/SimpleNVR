"""Contract tests for acceptance tooling; not substitutes for native Chrome cases."""
import asyncio
import copy
import json
from pathlib import Path
import unittest

import browser_playback_edges as edge


def playing(path=edge.FIRST):
    return {"path": path, "playingIntent": True, "paused": False, "error": None, "currentTime": 1.5,
            "events": [{"name": name, "trusted": True, "path": path} for name in ("loadedmetadata", "playing")]}


def denied():
    return {"path": edge.AUDIO, "active": False, "muted": False, "volume": 1, "readyState": 1,
            "error": None, "sessions": 1, "paused": True, "playingIntent": False,
            "text": "Playback could not start; press Play to retry or select another time"}


class ContractTests(unittest.TestCase):
    def test_native_playback_contract(self):
        edge.check_playing(playing(), edge.FIRST)
        for field, value in (("path", edge.SECOND), ("paused", True), ("error", 3), ("currentTime", None),
                             ("currentTime", 0), ("playingIntent", False), ("events", [])):
            with self.subTest(field=field, value=value):
                state = playing();state[field] = value
                with self.assertRaises(AssertionError): edge.check_playing(state, edge.FIRST)
        state = playing();state["events"][1]["trusted"] = False
        with self.assertRaises(AssertionError): edge.check_playing(state, edge.FIRST)

    def test_metadata_hold_contract(self):
        good = {"readyState": 0, "paused": True, "sessions": 0, "pendingMetadata": 1,
                "playingIntent": False, "events": []}
        edge.check_held(good, False)
        for key, value in (("readyState", 1), ("paused", False), ("sessions", 1), ("pendingMetadata", 0),
                           ("playingIntent", True), ("events", [{"name": "loadedmetadata", "trusted": True}])):
            state = dict(good);state[key] = value
            with self.assertRaises(AssertionError): edge.check_held(state, False)

    def test_autoplay_is_browser_denial_not_other_failure(self):
        edge.check_denied(denied(), "NotAllowedError")
        for key, value in (("path", edge.FIRST), ("active", True), ("muted", True), ("volume", 0),
                           ("readyState", 0), ("error", 4), ("sessions", 0), ("paused", False),
                           ("playingIntent", True), ("text", "")):
            state = denied();state[key] = value
            with self.assertRaises(AssertionError): edge.check_denied(state, "NotAllowedError")
        for rejection in (None, "fulfilled", "AbortError", "NotSupportedError"):
            with self.assertRaises(AssertionError): edge.check_denied(denied(), rejection)

    def test_only_known_cancelled_interception_is_tolerated(self):
        self.assertTrue(edge.cancelled_interception(Exception("Fetch.continueRequest: Invalid InterceptionId.")))
        for message in ("Target closed", "Session closed", "permission denied", "Timeout", "Network changed"):
            self.assertFalse(edge.cancelled_interception(Exception(message)))

    def test_expected_cases_and_nonreplacement_event_observer(self):
        self.assertEqual(len(edge.CASES), 4)
        self.assertEqual(len(set(edge.CASES)), 4)
        self.assertIn("event.isTrusted", edge.EVENT_SCRIPT)
        self.assertNotIn("dispatchEvent", edge.EVENT_SCRIPT)
        self.assertNotIn("prototype.play", edge.EVENT_SCRIPT)
        source = Path(edge.__file__).read_text()
        self.assertNotIn('"Fetch.fulfillRequest"', source)
        self.assertNotIn('"Fetch.failRequest"', source)


class FakeCDP:
    def __init__(self, value=True):
        self.value, self.calls, self.handlers = value, [], {}
        self.cancelled = False
        self.failure = None

    async def send(self, method, params=None):
        self.calls.append((method, params))
        if method == "Fetch.continueRequest":
            if self.failure: raise RuntimeError(self.failure)
            if self.cancelled: raise RuntimeError("Invalid InterceptionId.")
        return {"result": {"value": self.value}}

    def on(self, event, handler): self.handlers[event] = handler
    def remove_listener(self, event, handler): self.handlers.pop(event, None)


def request(path=edge.FIRST, request_id="one"):
    return {"requestId": request_id, "resourceType": "Media", "request": {"url": "https://fixture" + path}}


class AsyncContractTests(unittest.IsolatedAsyncioTestCase):
    async def test_cdp_evaluation_never_grants_user_activation(self):
        cdp = FakeCDP({"active": False})
        self.assertEqual(await edge.evaluate(cdp, "navigator.userActivation"), {"active": False})
        self.assertIs(cdp.calls[-1][1]["userGesture"], False)
        self.assertIs(cdp.calls[-1][1]["awaitPromise"], True)
        class ExceptionCDP(FakeCDP):
            async def send(self, method, params=None): return {"exceptionDetails": {"text": "failed"}}
        with self.assertRaises(AssertionError): await edge.evaluate(ExceptionCDP(), "bad")

    async def test_hold_first_real_request_and_continue_other_requests(self):
        cdp = FakeCDP();hold = edge.MediaHold(cdp, "https://fixture")
        await hold.start()
        await hold.handle(request())
        self.assertIsNotNone(await asyncio.wait_for(hold.queue.get(), 1))
        self.assertFalse(any(name == "Fetch.continueRequest" for name, _ in cdp.calls))
        await hold.handle(request(edge.SECOND, "two"))
        self.assertEqual(cdp.calls[-1], ("Fetch.continueRequest", {"requestId": "two"}))
        self.assertEqual(await hold.release(), "continued")
        await hold.close()
        self.assertEqual(hold.pending, None)
        self.assertEqual(cdp.handlers, {})
        self.assertEqual(cdp.calls[-1][0], "Fetch.disable")

    async def test_release_handles_browser_cancellation_but_not_arbitrary_errors(self):
        cdp = FakeCDP();hold = edge.MediaHold(cdp, "https://fixture")
        await hold.handle(request());cdp.cancelled = True
        self.assertEqual(await hold.release(), "already-cancelled-by-browser")
        cdp.cancelled = False;cdp.failure = "Target closed"
        hold.pending = "three"
        with self.assertRaises(RuntimeError): await hold.release()

    async def test_cleanup_continues_pending_and_joins_handler_tasks(self):
        cdp = FakeCDP();hold = edge.MediaHold(cdp, "https://fixture")
        await hold.start();await hold.handle(request())
        hold.dispatch(request(edge.SECOND, "two"))
        await hold.close()
        self.assertFalse(hold.tasks)
        self.assertFalse(hold.errors)
        self.assertIn(("Fetch.continueRequest", {"requestId": "one"}), cdp.calls)
        self.assertIn(("Fetch.continueRequest", {"requestId": "two"}), cdp.calls)

    async def test_handler_error_is_not_silently_ignored(self):
        cdp = FakeCDP();hold = edge.MediaHold(cdp, "https://fixture")
        await hold.start();cdp.failure = "permission denied"
        hold.dispatch(request(edge.SECOND))
        with self.assertRaises(AssertionError): await hold.close()
        self.assertTrue(hold.errors)

    async def test_native_frame_contract(self):
        valid = {"frames": 2, "width": 256, "height": 144}
        self.assertEqual(await edge.frame(FakeCDP(valid)), valid)
        for invalid in ({"timeout": True}, {"frames": True, "width": 256, "height": 144},
                        {"frames": 1, "width": 0, "height": 144}, None):
            with self.assertRaises(AssertionError): await edge.frame(FakeCDP(invalid))


if __name__ == "__main__":
    unittest.main()
