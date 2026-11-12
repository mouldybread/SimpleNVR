import asyncio
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from browser_ci_diagnostics import BrowserNetworkEvidence, LoginSubmissionError, observe_login_post

ORIGIN = "https://fixture.invalid"
SECRET = "credential-must-never-be-written"


class Page:
    def __init__(self, events):
        self.events = events
        self.listeners = {"response": [], "requestfailed": []}
        self.clicks = 0

    def on(self, name, callback):
        self.listeners[name].append(callback)

    def remove_listener(self, name, callback):
        self.listeners[name].remove(callback)

    def get_by_role(self, *args, **kwargs):
        return self

    async def click(self, **kwargs):
        assert kwargs == {"no_wait_after": True}
        self.clicks += 1
        for name, value in self.events:
            for callback in list(self.listeners[name]):
                callback(value)


def request(method="POST", path="/auth/login", failure="net::ERR_NETWORK_CHANGED"):
    return SimpleNamespace(url=ORIGIN + path, method=method, failure=failure)


def response(status=303):
    return SimpleNamespace(request=request(), status=status)


class LoginTests(unittest.IsolatedAsyncioTestCase):
    async def test_response_is_observed_once(self):
        expected = response()
        page, report = Page([("response", expected)]), {}
        self.assertIs(await observe_login_post(page, ORIGIN, report), expected)
        self.assertEqual(report["login_submission"]["status"], 303)
        self.assertEqual(page.clicks, 1)
        self.assertFalse(any(page.listeners.values()))

    async def test_failed_post_is_immediate_and_never_retried(self):
        page, report = Page([("requestfailed", request())]), {}
        with self.assertRaisesRegex(LoginSubmissionError, "ERR_NETWORK_CHANGED"):
            await asyncio.wait_for(observe_login_post(page, ORIGIN, report), 1)
        self.assertEqual(report["login_submission"]["outcome"], "requestfailed")
        self.assertFalse(report["login_submission"]["response_observed"])
        self.assertEqual(page.clicks, 1)
        self.assertFalse(any(page.listeners.values()))

    async def test_other_requests_do_not_settle_login(self):
        expected = response()
        events = [("requestfailed", request(method="GET")), ("requestfailed", request(path="/")), ("response", expected)]
        self.assertIs(await observe_login_post(Page(events), ORIGIN, {}), expected)

    async def test_http_refusal_is_not_a_transport_retry(self):
        page, report = Page([("response", response(401))]), {}
        self.assertEqual((await observe_login_post(page, ORIGIN, report)).status, 401)
        self.assertTrue(report["login_submission"]["response_observed"])
        self.assertEqual(page.clicks, 1)

    async def test_duplicate_terminal_events_are_harmless(self):
        expected = response()
        page = Page([("response", expected), ("response", expected), ("requestfailed", request())])
        self.assertIs(await observe_login_post(page, ORIGIN, {}), expected)
        self.assertFalse(any(page.listeners.values()))

    async def test_timeout_cleans_listeners_without_retry(self):
        page, report = Page([]), {}
        with self.assertRaises(LoginSubmissionError):
            await observe_login_post(page, ORIGIN, report, timeout=0.01)
        self.assertEqual(report["login_submission"]["outcome"], "response-timeout")
        self.assertEqual(page.clicks, 1)
        self.assertFalse(any(page.listeners.values()))

    async def test_cancel_cleans_listeners_without_retry(self):
        page = Page([])
        task = asyncio.create_task(observe_login_post(page, ORIGIN, {}))
        await asyncio.sleep(0)
        task.cancel()
        with self.assertRaises(asyncio.CancelledError):
            await task
        self.assertEqual(page.clicks, 1)
        self.assertFalse(any(page.listeners.values()))

    async def test_unknown_failure_text_is_not_logged(self):
        page, report = Page([("requestfailed", request(failure=SECRET))]), {}
        with self.assertRaises(LoginSubmissionError) as error:
            await observe_login_post(page, ORIGIN, report)
        self.assertNotIn(SECRET, str(error.exception) + json.dumps(report))


class EvidenceTests(unittest.IsolatedAsyncioTestCase):
    async def test_metadata_allowlist_and_failure_flush(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence = BrowserNetworkEvidence(directory, ORIGIN, sampler=lambda: {"links": []})
            with self.assertRaisesRegex(RuntimeError, "fixture"):
                async with evidence:
                    evidence.request({"requestId": "1", "type": "Document", "request": {"url": ORIGIN + "/auth/login?token=" + SECRET, "method": "POST", "headers": {"Cookie": SECRET}, "postData": SECRET}})
                    evidence.response({"requestId": "1", "response": {"url": ORIGIN + "/?token=" + SECRET, "status": 303, "protocol": "http/1.1", "headers": {"Set-Cookie": SECRET}}})
                    evidence.failure({"requestId": "2", "type": "Document", "errorText": "net::ERR_NETWORK_CHANGED"})
                    await asyncio.sleep(0.01)
                    raise RuntimeError("fixture")
            raw = (Path(directory) / "browser-network-evidence.json").read_text()
            self.assertNotIn(SECRET, raw)
            self.assertNotIn("postData", raw)
            data = json.loads(raw)
            self.assertTrue(any(e["event"] == "host-network-state" for e in data["events"]))
            self.assertTrue(any(e.get("failure") == "net::ERR_NETWORK_CHANGED" for e in data["events"]))
            self.assertEqual(data["events"][-1]["event"], "diagnostics-end")
            self.assertTrue(evidence.task.done())

    async def test_cdp_setup_and_allowlisted_callbacks(self):
        class Client:
            def __init__(self): self.callbacks = {}; self.commands = []
            def on(self, name, callback): self.callbacks[name] = callback
            async def send(self, name): self.commands.append(name)
        client = Client()
        class Context:
            async def new_cdp_session(self, page): return client
        with tempfile.TemporaryDirectory() as directory:
            async with BrowserNetworkEvidence(directory, ORIGIN, sampler=lambda: {}) as evidence:
                await evidence.attach(Context(), object())
                self.assertTrue(evidence.cdp_attached)
                self.assertEqual(client.commands, ["Network.enable"])
                self.assertEqual(len(client.callbacks), 3)

    async def test_cdp_failure_is_explicit(self):
        class Context:
            async def new_cdp_session(self, page): raise RuntimeError(SECRET)
        with tempfile.TemporaryDirectory() as directory:
            async with BrowserNetworkEvidence(directory, ORIGIN, sampler=lambda: {}) as evidence:
                await evidence.attach(Context(), object())
            raw = evidence.path.read_text()
            self.assertNotIn(SECRET, raw)
            self.assertIn("cdp-unavailable", raw)

    async def test_evidence_is_bounded_and_counts_drops(self):
        with tempfile.TemporaryDirectory() as directory:
            async with BrowserNetworkEvidence(directory, ORIGIN, sampler=lambda: {}) as evidence:
                for _ in range(600): evidence.record("fixture")
            data = json.loads(evidence.path.read_text())
            self.assertEqual(len(data["events"]), 512)
            self.assertGreater(data["dropped_events"], 0)


if __name__ == "__main__":
    unittest.main()
