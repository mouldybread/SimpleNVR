import asyncio
import copy
import signal
import sys
import unittest
from collections import deque
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch
from browser_network_ready import ReadinessError, ReadinessGate, network_snapshot, network_epoch, wait_ready, run_driver, stop_driver


def ready(**changes):
    return {"signature": "a", "tentative": 0, "dadfailed": 0, "addresses": 1, "ipv6_disabled": False, "link_ready": True} | changes


def diagnostics():
    d = SimpleNamespace(route_attached=True, route_socket=object(), dropped=0, events=deque())
    d.record = lambda event, **fields: d.events.append(dict(event=event, **fields))
    return d


class GateTests(unittest.TestCase):
    def test_tentative_then_full_quiet_window(self):
        gate = ReadinessGate(0)
        self.assertFalse(gate.observe(0, ready(tentative=1), 0))
        self.assertFalse(gate.observe(1, ready(), 0))
        self.assertFalse(gate.observe(2, ready(), 0))
        self.assertTrue(gate.observe(3, ready(), 0))

    def test_state_and_notification_changes_reset_quiet(self):
        for state, epoch in [(ready(signature="b"), 0), (ready(), 1)]:
            gate = ReadinessGate(0)
            self.assertFalse(gate.observe(0, ready(), 0))
            self.assertFalse(gate.observe(1, state, epoch))
            self.assertFalse(gate.observe(2, state, epoch))
            self.assertTrue(gate.observe(3, state, epoch))

    def test_missing_address_or_link_cannot_be_ready(self):
        for state in [ready(addresses=0), ready(link_ready=False), ready(tentative=1)]:
            gate = ReadinessGate(0)
            self.assertFalse(gate.observe(0, state, 0))
            self.assertFalse(gate.observe(19, state, 0))
            with self.assertRaisesRegex(ReadinessError, "deadline"):
                gate.observe(20, state, 0)

    def test_failed_detection_refuses_and_existing_disabled_mode_is_observed(self):
        with self.assertRaisesRegex(ReadinessError, "dadfailed"):
            ReadinessGate(0).observe(0, ready(dadfailed=1), 0)
        gate = ReadinessGate(0)
        self.assertFalse(gate.observe(0, ready(ipv6_disabled=True, addresses=0), 0))
        self.assertTrue(gate.observe(2, ready(ipv6_disabled=True, addresses=0), 0))

    def test_deadline_precedes_success(self):
        gate = ReadinessGate(0)
        gate.observe(0, ready(), 0)
        with self.assertRaisesRegex(ReadinessError, "deadline"):
            gate.observe(20, ready(), 0)

    def test_monitor_failure_and_loss_are_not_quiet(self):
        for change in [dict(route_attached=False), dict(route_socket=None), dict(dropped=1)]:
            d = diagnostics()
            d.__dict__.update(change)
            with self.assertRaises(ReadinessError): network_epoch(d)
        d = diagnostics()
        d.record("rtnetlink-loss")
        with self.assertRaises(ReadinessError): network_epoch(d)


class SnapshotTests(unittest.TestCase):
    def setUp(self):
        self.state = {"links": [{"name": "veth-test", "ifindex": "13", "operstate": "up", "carrier": "1"}], "fingerprints": {"/etc/resolv.conf": "resolver", "/proc/net/route": "raw"}, "read_status": {p: "ok" for p in ("/proc/net/route", "/proc/net/ipv6_route", "/etc/resolv.conf")}, "route_structure": {p: {"status": "ok", "sha256": "route"} for p in ("/proc/net/route", "/proc/net/ipv6_route")}}
        self.files = {"/proc/sys/net/ipv6/conf/veth-test/disable_ipv6": "0", "/sys/class/net/veth-test/ifindex": "13", "/proc/net/if_inet6": "0" * 32 + " 0d 40 20 80 veth-test"}

    def snapshot(self):
        return network_snapshot(13, lambda: copy.deepcopy(self.state), lambda p: (self.files[p], "ok") if p in self.files else (None, "unavailable"))

    def test_flags_and_raw_counter_only_changes(self):
        initial = self.snapshot()
        self.state["fingerprints"]["/proc/net/route"] = "counter-only-change"
        self.assertEqual(self.snapshot(), initial)
        self.files["/proc/net/if_inet6"] = self.files["/proc/net/if_inet6"].replace("20 80", "20 40")
        self.assertEqual(self.snapshot()["tentative"], 1)
        self.assertNotEqual(self.snapshot()["signature"], initial["signature"])
        self.files["/proc/net/if_inet6"] = self.files["/proc/net/if_inet6"].replace("20 40", "20 08")
        self.assertEqual(self.snapshot()["dadfailed"], 1)

    def test_unavailable_incomplete_and_replaced_identity_refuse(self):
        for kind in ["read", "route", "identity", "address", "missing"]:
            with self.subTest(kind=kind):
                self.setUp()
                if kind == "read": self.state["read_status"]["/proc/net/route"] = "unavailable"
                if kind == "route": self.state["route_structure"]["/proc/net/route"] = {"status": "malformed"}
                if kind == "identity": self.files["/sys/class/net/veth-test/ifindex"] = "14"
                if kind == "address": self.files["/proc/net/if_inet6"] = "malformed"
                if kind == "missing": self.files.pop("/proc/net/if_inet6")
                with self.assertRaises(ReadinessError): self.snapshot()

    def test_addresses_are_hashed_not_reported(self):
        import json
        self.assertNotIn("0" * 32, json.dumps(self.snapshot()))


class AsyncTests(unittest.IsolatedAsyncioTestCase):
    async def test_wait_uses_observations_without_fixed_startup_sleep(self):
        now, d = [0.0], diagnostics()
        async def advance(seconds): now[0] += seconds
        await wait_ready(d, 13, snapshot=lambda _: ready(), clock=lambda: now[0], sleep=advance)
        self.assertGreaterEqual(now[0], 2)
        self.assertEqual(d.events[-1]["event"], "network-ready")

    async def test_loss_during_inspection_cannot_admit(self):
        d = diagnostics()
        def snapshot(_):
            d.record("rtnetlink-loss")
            return ready()
        with self.assertRaises(ReadinessError): await wait_ready(d, 13, snapshot=snapshot)
        self.assertFalse(any(e["event"] == "network-ready" for e in d.events))

    async def test_driver_failure_is_preserved_without_retry(self):
        process = SimpleNamespace(returncode=7, wait=AsyncMock(return_value=7))
        spawn = AsyncMock(return_value=process)
        self.assertEqual(await run_driver(["fixture"], diagnostics(), spawn=spawn), 7)
        spawn.assert_awaited_once_with("fixture", start_new_session=True)

    async def test_cancellation_invokes_driver_cleanup(self):
        entered = asyncio.Event()
        async def wait():
            entered.set()
            await asyncio.Event().wait()
        process = SimpleNamespace(wait=wait)
        cleanup = AsyncMock()
        with patch("browser_network_ready.stop_driver", cleanup):
            task = asyncio.create_task(run_driver(["fixture"], diagnostics(), spawn=AsyncMock(return_value=process)))
            await entered.wait()
            task.cancel()
            with self.assertRaises(asyncio.CancelledError): await task
        cleanup.assert_awaited_once_with(process)

    async def test_forced_cleanup_escalates_and_waits(self):
        sent = []
        process = SimpleNamespace(pid=12345, returncode=None, wait=AsyncMock(return_value=-9))
        calls = []
        async def deadline(awaitable, seconds):
            calls.append(seconds)
            if len(calls) == 1:
                awaitable.close()
                raise asyncio.TimeoutError
            return await awaitable
        with patch("browser_network_ready.asyncio.wait_for", deadline):
            await stop_driver(process, send=lambda pid, sig: sent.append((pid, sig)))
        self.assertEqual(sent, [(12345, signal.SIGTERM), (12345, signal.SIGKILL)])
        self.assertEqual(calls, [5, 5])

    async def test_sigterm_handler_is_registered_and_removed(self):
        from browser_network_ready import main
        from unittest.mock import Mock
        loop = Mock()
        with patch("browser_network_ready.asyncio.get_running_loop", return_value=loop), patch("browser_network_ready.run", AsyncMock(return_value=143)):
            self.assertEqual(await main(object()), 143)
        self.assertEqual(loop.add_signal_handler.call_args.args[0], signal.SIGTERM)
        loop.remove_signal_handler.assert_called_once_with(signal.SIGTERM)

    async def test_real_local_driver_is_terminated_and_joined(self):
        process = await asyncio.create_subprocess_exec(sys.executable, "-c", "import time; time.sleep(30)", start_new_session=True)
        try:
            await stop_driver(process)
            self.assertIsNotNone(process.returncode)
        finally:
            if process.returncode is None:
                process.kill()
                await process.wait()


if __name__ == "__main__":
    unittest.main()
