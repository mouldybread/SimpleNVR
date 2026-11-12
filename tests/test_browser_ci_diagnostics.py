import asyncio
import json
import tempfile
import unittest
import socket
import struct
from unittest.mock import Mock, patch
from pathlib import Path
from types import SimpleNamespace
from browser_ci_diagnostics import BrowserNetworkEvidence, LoginSubmissionError, observe_login_post, structural_routes, route_notifications, bounded_network_read, NETWORK_BYTES, ROUTE_ROWS

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


class RouteEvidenceTests(unittest.TestCase):
    v6 = "0" * 32 + " 00 " + "0" * 32 + " 00 " + "0" * 32 + " 00000001 00000002 00000003 00000001 eth0"
    v4 = "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\neth0 00000000 0100007F 0003 0 1 10 00000000 0 0 0"

    def test_ipv6_counter_and_order_stability(self):
        other = self.v6.replace("00000002 00000003", "000000ff 000000fe")
        self.assertEqual(structural_routes(self.v6, True), structural_routes(other, True))
        self.assertEqual(structural_routes(self.v6 + "\n" + other, True), structural_routes(other + "\n" + self.v6, True))
        for changed in (self.v6.replace("eth0", "eth1"), self.v6.replace("00000001 00000002", "00000004 00000002"), self.v6.replace(" 00 ", " 01 ", 1)):
            self.assertNotEqual(structural_routes(self.v6, True), structural_routes(changed, True))

    def test_ipv4_counter_and_gateway_changes(self):
        self.assertEqual(structural_routes(self.v4), structural_routes(self.v4.replace("0003 0 1", "0003 8 9")))
        self.assertNotEqual(structural_routes(self.v4), structural_routes(self.v4.replace("0100007F", "0200007F")))

    def test_invalid_route_input_is_explicit(self):
        for text, status in [(None, "unavailable"), (SECRET, "malformed"), ("x" * (NETWORK_BYTES + 1), "truncated"), ("x\n" * (ROUTE_ROWS + 1), "truncated")]:
            self.assertEqual(structural_routes(text, True), {"status": status})
        self.assertEqual(structural_routes("bad header"), {"status": "malformed"})

    def test_bounded_reads(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture"
            for data, status in [(b"x" * (NETWORK_BYTES + 1), "truncated"), (b"\xff", "unavailable")]:
                path.write_bytes(data)
                self.assertEqual(bounded_network_read(path), (None, status))
            path.unlink()
            self.assertEqual(bounded_network_read(path), (None, "unavailable"))

    def test_notifications_are_metadata_only(self):
        for kind, size in [(16, 16), (17, 16), (20, 8), (21, 8), (24, 12), (25, 12)]:
            body = bytes(size) + SECRET.encode()
            events, problem = route_notifications(struct.pack("=IHHII", 16 + len(body), kind, 0, 0, 0) + body)
            self.assertIsNone(problem)
            self.assertEqual(len(events), 1)
            self.assertNotIn(SECRET, json.dumps(events))
            self.assertTrue(set(events[0]) <= {"kind", "action", "family", "ifindex", "table", "protocol", "scope", "route_type"})

    def test_netlink_errors_and_limits(self):
        for packet in (b"x", struct.pack("=IHHII", 15, 24, 0, 0, 0), struct.pack("=IHHII", 100, 24, 0, 0, 0), struct.pack("=IHHII", 16, 24, 0, 0, 0), struct.pack("=IHHII", 16, 4, 0, 0, 0), struct.pack("=IHHIIi", 20, 2, 0, 0, 0, -105)):
            self.assertIsNotNone(route_notifications(packet)[1])
        events, problem = route_notifications((struct.pack("=IHHII", 28, 24, 0, 0, 0) + bytes(12)) * 257)
        self.assertEqual(len(events), 256)
        self.assertIsNotNone(problem)

    def test_monitor_lifecycle_and_loss(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence = BrowserNetworkEvidence(directory, ORIGIN)
            sock, loop = Mock(), Mock()
            sock.fileno.return_value = 42
            with patch("browser_ci_diagnostics.socket.socket", return_value=sock), patch("browser_ci_diagnostics.asyncio.get_running_loop", return_value=loop):
                evidence.start_route_monitor()
            sock.bind.assert_called_once_with((0, 0x551))
            loop.add_reader.assert_called_once_with(42, evidence.read_route_events)
            sock.recvmsg.return_value = (b"", [], socket.MSG_TRUNC, None)
            evidence.read_route_events()
            self.assertTrue(any(e.get("reason") == "datagram-truncated" for e in evidence.events))
            sock.recvmsg.return_value = (struct.pack("=IHHII", 28, 24, 0, 0, 0) + bytes(12), [], 0, None)
            evidence.read_route_events()
            self.assertTrue(any(e["event"] == "rtnetlink-notification" for e in evidence.events))
            sock.recvmsg.side_effect = OSError(105, SECRET)
            evidence.read_route_events()
            evidence.stop_route_monitor()
            loop.remove_reader.assert_called_once_with(42)
            sock.close.assert_called_once()
            self.assertNotIn(SECRET, json.dumps(list(evidence.events)))
            self.assertTrue(any(e.get("errno") == 105 for e in evidence.events))

    def test_monitor_unavailable_is_sanitized(self):
        with tempfile.TemporaryDirectory() as directory:
            evidence, sock = BrowserNetworkEvidence(directory, ORIGIN), Mock()
            sock.bind.side_effect = PermissionError(SECRET)
            with patch("browser_ci_diagnostics.socket.socket", return_value=sock), patch("browser_ci_diagnostics.asyncio.get_running_loop", return_value=Mock()):
                evidence.start_route_monitor()
            sock.close.assert_called_once()
            self.assertIsNone(evidence.route_socket)
            self.assertFalse(evidence.route_attached)
            self.assertNotIn(SECRET, json.dumps(list(evidence.events)))


if __name__ == "__main__":
    unittest.main()
