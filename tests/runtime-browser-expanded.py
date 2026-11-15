"""Run unchanged baseline acceptance, then separately scoped native playback edges."""
import argparse
import asyncio
import importlib.util
import json
import os
from collections import deque
from urllib.parse import urlsplit
from pathlib import Path
import re

from playwright.async_api import async_playwright
from browser_ci_diagnostics import BrowserNetworkEvidence
from browser_playback_edges import CASES, metadata_case, paused_end_case, autoplay_case, require

spec = importlib.util.spec_from_file_location("runtime_browser_baseline", Path(__file__).with_name("runtime-browser-acceptance.py"))
legacy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(legacy)


def write(path, value):
    path.write_text(json.dumps(value, indent=2, allow_nan=False) + "\n")


async def run_edges(args):
    directory = args.evidence / "playback-edges"
    directory.mkdir(parents=True, exist_ok=True)
    report = {"commit": args.commit, "image_id": args.image_id, "status": "failed",
              "runtime_qualified": False, "cleanup_authorized": False,
              "response_bodies_replaced": False, "play_overridden": False, "media_events_dispatched": False,
              "strict_autoplay_policy": "document-user-activation-required",
              "request_holds": "one real media request before server delivery in each metadata case",
              "cases": {name: {"status": "not-started"} for name in CASES}, "page_errors": []}
    browser = context = None
    async with BrowserNetworkEvidence(directory, args.origin) as diagnostics, async_playwright() as p:
        try:
            browser = await p.chromium.launch(channel="chrome", headless=True,
                ignore_default_args=["--autoplay-policy=no-user-gesture-required"],
                args=["--autoplay-policy=document-user-activation-required",
                      "--disable-features=PreloadMediaEngagementData,MediaEngagementBypassAutoplayPolicies"])
            report["browser"] = browser.version
            report["playwright_version"] = diagnostics.playwright_version
            context = await browser.new_context(ignore_https_errors=True, timezone_id="UTC")
            context.set_default_timeout(30000)
            def observe_page(page):
                page.on("pageerror", lambda error: report["page_errors"].append(str(error)[:500])
                        if len(report["page_errors"]) < 20 else None)
            context.on("page", observe_page)
            login = await context.new_page()
            await diagnostics.attach(context, login)
            await login.goto(args.origin + "/auth/login")
            network = deque(maxlen=100)
            login.on("response", lambda response: network.append({"event": "response", "path": urlsplit(response.url).path,
                "method": response.request.method, "status": response.status, "resource_type": response.request.resource_type}))
            login.on("requestfailed", lambda request: network.append({"event": "requestfailed", "path": urlsplit(request.url).path,
                "method": request.method, "resource_type": request.resource_type, "failure": request.failure}))
            await login.locator('[name="username"]').fill("ci-browser")
            await login.locator('[name="password"]').fill(os.environ["SIMPLENVR_E2E_PASSWORD"])
            await legacy.submit_login_once(login, args.origin, report, network)
            await login.close()
            for name in CASES:
                row = report["cases"][name]
                row["status"] = "running"
                try:
                    if name.startswith("metadata-"):
                        await metadata_case(context, args.origin, legacy, diagnostics, row, retarget=name == "metadata-retarget")
                    elif name == "paused-native-end-resume":
                        await paused_end_case(context, args.origin, legacy, diagnostics, row)
                    else:
                        await autoplay_case(context, args.origin, legacy, diagnostics, row)
                    row["status"] = "passed"
                except BaseException as error:
                    row["status"] = "failed"
                    row["failure"] = type(error).__name__ + ": " + str(error)[:1500]
                    raise
            require(not report["page_errors"], "native edge page errors")
            report["status"] = "passed-with-navigation-recovery" if report.get("login_navigation_recovery") else "passed"
        except BaseException as error:
            report["failure"] = type(error).__name__ + ": " + str(error)[:1500]
            raise
        finally:
            try:
                try:
                    if context is not None:
                        await context.close()
                finally:
                    if browser is not None:
                        await browser.close()
            finally:
                write(directory / "playback-edge-report.json", report)


async def run(args):
    args.evidence.mkdir(parents=True, exist_ok=True)
    require(re.fullmatch(r"[0-9a-f]{40}", args.commit), "exact commit SHA required")
    aggregate = {"commit": args.commit, "image_id": args.image_id, "status": "failed",
                 "baseline": "not-started", "playback_edges": "not-started",
                 "runtime_qualified": False, "cleanup_authorized": False}
    try:
        aggregate["baseline"] = "running"
        await legacy.run(args)
        aggregate["baseline"] = "passed"
        aggregate["playback_edges"] = "running"
        await run_edges(args)
        aggregate["playback_edges"] = "passed"
        aggregate["status"] = "passed"
    except BaseException as error:
        aggregate["failure"] = type(error).__name__ + ": " + str(error)[:1500]
        raise
    finally:
        write(args.evidence / "runtime-browser-expanded-report.json", aggregate)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("origin", "container", "commit", "image-id"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    args = parser.parse_args()
    asyncio.run(asyncio.wait_for(run(args), timeout=300))
