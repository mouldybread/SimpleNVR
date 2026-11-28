"""Bounded CI readiness before the browser driver; never retry authentication."""
import argparse
import asyncio
import hashlib
import json
import os
import re
import signal
import time
from pathlib import Path


class ReadinessError(RuntimeError):
    pass


def network_snapshot(ifindex, state_reader=None, read=None):
    if state_reader is None or read is None:
        from browser_ci_diagnostics import host_network_state, bounded_network_read
        state_reader, read = host_network_state, bounded_network_read
    state = state_reader()
    if any(state.get("read_status", {}).get(p) != "ok" for p in ("/proc/net/route", "/proc/net/ipv6_route", "/etc/resolv.conf")):
        raise ReadinessError("network-inspection-unavailable")
    if any(state.get("route_structure", {}).get(p, {}).get("status") != "ok" for p in ("/proc/net/route", "/proc/net/ipv6_route")):
        raise ReadinessError("route-inspection-incomplete")
    links = state.get("links", [])
    matches = [link for link in links if link.get("ifindex") == str(ifindex)]
    if len(links) >= 64 or len(matches) != 1:
        raise ReadinessError("interface-inspection-incomplete")
    name = matches[0].get("name", "")
    if not re.fullmatch(r"[A-Za-z0-9_.:-]{1,64}", name) or name in (".", ".."):
        raise ReadinessError("interface-identity-invalid")
    disabled, status = read("/proc/sys/net/ipv6/conf/" + name + "/disable_ipv6")
    if status != "ok" or disabled not in ("0", "1"):
        raise ReadinessError("ipv6-mode-unavailable")
    text, status = read("/proc/net/if_inet6")
    if status != "ok" or text is None or len(text) > 262144 or len(text.splitlines()) > 2048:
        raise ReadinessError("address-inspection-incomplete")
    rows, flags = [], []
    for line in text.splitlines():
        fields = line.split()
        if len(fields) != 6 or not re.fullmatch(r"[0-9a-fA-F]{32}", fields[0]):
            raise ReadinessError("address-inspection-malformed")
        try:
            row = [int(value, 16) for value in fields[:5]] + [fields[5]]
        except ValueError:
            raise ReadinessError("address-inspection-malformed") from None
        if row[1] <= 0 or not 0 <= row[2] <= 128 or any(value < 0 for value in row[:5]):
            raise ReadinessError("address-inspection-malformed")
        if row[1] == ifindex:
            if row[5] != name:
                raise ReadinessError("interface-identity-changed")
            flags.append(row[4])
        rows.append(row)
    current, status = read("/sys/class/net/" + name + "/ifindex")
    if status != "ok" or current != str(ifindex):
        raise ReadinessError("interface-identity-changed")
    signature = {"links": links, "routes": state["route_structure"], "resolver": state.get("fingerprints", {}).get("/etc/resolv.conf"), "addresses": sorted(rows), "disabled": disabled}
    return {"ifindex": ifindex, "link_ready": matches[0].get("operstate") == "up" and matches[0].get("carrier") == "1", "ipv6_disabled": disabled == "1", "addresses": len(flags), "tentative": sum(bool(f & 0x40) for f in flags), "dadfailed": sum(bool(f & 0x08) for f in flags), "signature": hashlib.sha256(json.dumps(signature, sort_keys=True).encode()).hexdigest()}


class ReadinessGate:
    def __init__(self, started, timeout=20, quiet=2):
        self.deadline, self.quiet = started + timeout, quiet
        self.previous, self.since = None, None

    def observe(self, now, state, epoch):
        if now >= self.deadline:
            raise ReadinessError("network-readiness-deadline")
        if state["dadfailed"]:
            raise ReadinessError("ipv6-dadfailed")
        token = (state["signature"], epoch)
        eligible = state["link_ready"] and not state["tentative"] and (state["ipv6_disabled"] or state["addresses"] > 0)
        if not eligible:
            self.since = None
        elif token != self.previous or self.since is None:
            self.since = now
        self.previous = token
        return self.since is not None and now - self.since >= self.quiet


def network_epoch(diagnostics):
    if not diagnostics.route_attached or diagnostics.route_socket is None:
        raise ReadinessError("network-monitor-unavailable")
    if diagnostics.dropped or any(e["event"] == "rtnetlink-loss" for e in diagnostics.events):
        raise ReadinessError("network-evidence-loss")
    return sum(e["event"] == "rtnetlink-notification" for e in diagnostics.events)


async def wait_ready(diagnostics, ifindex, snapshot=network_snapshot, clock=time.monotonic, sleep=asyncio.sleep):
    gate = ReadinessGate(clock())
    previous = None
    while True:
        network_epoch(diagnostics)
        state = await asyncio.to_thread(snapshot, ifindex)
        epoch = network_epoch(diagnostics)
        if state != previous:
            diagnostics.record("readiness-state", **state)
            previous = state
        if gate.observe(clock(), state, epoch):
            diagnostics.record("network-ready", quiet_seconds=2)
            return
        await sleep(0.1)


async def stop_driver(process, send=os.killpg):
    if process.returncode is not None:
        return
    try:
        send(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        await asyncio.wait_for(process.wait(), 5)
    except asyncio.TimeoutError:
        try:
            send(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        await asyncio.wait_for(process.wait(), 5)


async def run_driver(command, diagnostics, spawn=asyncio.create_subprocess_exec, timeout=300):
    diagnostics.record("browser-driver-launch")
    process = await spawn(*command, start_new_session=True)
    try:
        return await asyncio.wait_for(process.wait(), timeout)
    finally:
        await stop_driver(process)


async def run(args):
    from browser_ci_diagnostics import BrowserNetworkEvidence
    args.evidence.mkdir(parents=True, exist_ok=True)
    report = {"status": "failed", "ifindex": args.ifindex, "quiet_seconds": 2, "deadline_seconds": 20}
    try:
        async with BrowserNetworkEvidence(args.evidence / "startup-network", "https://127.0.0.1") as diagnostics:
            report["monitor_clock_origin_monotonic"] = diagnostics.started
            await wait_ready(diagnostics, args.ifindex)
            report["readiness"] = "ready"
            report["driver_launch_monotonic"] = time.monotonic()
            code = await run_driver(args.command, diagnostics)
            report.update(status="passed" if code == 0 else "failed", driver_exit_code=code)
            return code if code >= 0 else 1
    except asyncio.CancelledError:
        report["error_type"] = "CancelledError"
        return 143
    except Exception as error:
        # Fixed internal refusal codes only; never serialize command/env or an
        # arbitrary OS/subprocess exception message into startup diagnostics.
        report["error_type"] = type(error).__name__
        if isinstance(error, ReadinessError):
            report["reason"] = str(error)
        return 1
    finally:
        (args.evidence / "network-readiness.json").write_text(json.dumps(report, indent=2) + "\n")


async def main(args):
    loop, task = asyncio.get_running_loop(), asyncio.current_task()
    loop.add_signal_handler(signal.SIGTERM, task.cancel)
    try:
        return await run(args)
    finally:
        loop.remove_signal_handler(signal.SIGTERM)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--ifindex", type=int, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if args.command[:1] == ["--"]:
        args.command = args.command[1:]
    if args.ifindex < 1 or not args.command:
        parser.error("positive host interface index and browser driver command required")
    raise SystemExit(asyncio.run(main(args)))
