"""Contract tests for acceptance tooling; not substitutes for native Chrome cases."""
import asyncio
import copy
import json
from pathlib import Path
import unittest

import browser_playback_edges as edge


def playing(path=edge.FIRST):
    return {"path": path, "playingIntent": True, "paused": False, "error": None, "currentTime": 1.5,
            "observer": {"version": 1, "installed": True, "document": "/", "dropped": 0},
            "events": [{"name": name, "trusted": True, "path": path} for name in ("loadedmetadata", "playing")]}


def denied():
    return {"path": edge.AUDIO, "active": False, "muted": False, "volume": 1, "readyState": 1,
            "error": None, "sessions": 1, "paused": True, "playingIntent": False,
            "text": "Playback could not start; press Play to retry or select another time",
            "observer": {"version": 1, "installed": True, "document": "/", "dropped": 0}, "events": []}


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
                "playingIntent": False, "observer": {"version": 1, "installed": True, "document": "/", "dropped": 0}, "events": []}
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


class ObserverTests(unittest.TestCase):
    def test_observer_presence_and_loss_are_explicit(self):
        edge.require_observer(playing())
        for kind in ("missing", "null", "not-installed", "wrong-version", "bool-version", "wrong-document",
                     "lost-events", "bool-dropped", "missing-buffer", "null-buffer", "oversized"):
            with self.subTest(kind=kind):
                state=playing()
                if kind=="missing": del state["observer"]
                elif kind=="null": state["observer"]=None
                elif kind=="not-installed": state["observer"]["installed"]=False
                elif kind=="wrong-version": state["observer"]["version"]=2
                elif kind=="bool-version": state["observer"]["version"]=True
                elif kind=="wrong-document": state["observer"]["document"]="/auth/login"
                elif kind=="lost-events": state["observer"]["dropped"]=1
                elif kind=="bool-dropped": state["observer"]["dropped"]=False
                elif kind=="missing-buffer": del state["events"]
                elif kind=="null-buffer": state["events"]=None
                elif kind=="oversized": state["events"]=[{}]*129
                with self.assertRaisesRegex(AssertionError,"observer"):
                    edge.require_observer(state)

    def test_installed_empty_is_not_successful_playback(self):
        state=playing();state["events"]=[]
        edge.require_observer(state)
        with self.assertRaisesRegex(AssertionError,"native trusted"):
            edge.check_playing(state,edge.FIRST)
        state["observer"]=None
        with self.assertRaisesRegex(AssertionError,"observer"):
            edge.check_playing(state,edge.FIRST)


class ObserverBootstrapTests(unittest.IsolatedAsyncioTestCase):
    async def bootstrap(self, installed=True, registration_fails=False):
        from types import SimpleNamespace
        order=[];state=playing()
        if not installed: state["observer"]=None
        cdp=FakeCDP(state)
        class Page:
            url="about:blank"
            closed=False
            async def add_init_script(self,script):
                order.append("register")
                if registration_fails: raise RuntimeError("init registration failed")
                if script!=edge.EVENT_SCRIPT: raise AssertionError("wrong init script")
            async def goto(self,url,wait_until):
                order.append("navigate");self.url=url
                return SimpleNamespace(status=200)
            async def close(self): self.closed=True
        page=Page()
        class Context:
            async def new_page(self): return page
            async def new_cdp_session(self,value):
                order.append("cdp");return cdp
        class Diagnostics:
            async def attach(self,context,value): order.append("diagnostics")
        async def ready(value,origin): order.append("ready")
        try:
            result=await edge.prepare_page(Context(),"https://fixture",SimpleNamespace(application_ready=ready),Diagnostics())
            return result,page,cdp,order
        except BaseException as error:
            return error,page,cdp,order

    async def test_register_before_navigation_and_check_without_activation(self):
        result,page,cdp,order=await self.bootstrap()
        self.assertIsInstance(result,tuple)
        self.assertEqual(order,["register","cdp","diagnostics","navigate","ready"])
        evaluations=[params for method,params in cdp.calls if method=="Runtime.evaluate"]
        self.assertEqual(len(evaluations),2)
        self.assertTrue(all(params["userGesture"] is False for params in evaluations))
        self.assertFalse(any(method=="Page.addScriptToEvaluateOnNewDocument" for method,_ in cdp.calls))

    async def test_missing_observer_stops_before_acceptance_and_closes_page(self):
        result,page,_,order=await self.bootstrap(installed=False)
        self.assertIsInstance(result,AssertionError)
        self.assertIn("observer",str(result))
        self.assertNotIn("ready",order)
        self.assertTrue(page.closed)

    async def test_registration_failure_closes_page_without_navigation(self):
        result,page,_,order=await self.bootstrap(registration_fails=True)
        self.assertIsInstance(result,RuntimeError)
        self.assertEqual(order,["register"])
        self.assertTrue(page.closed)


if __name__ == "__main__":
    unittest.main()
