"""Scoped Chromium export UI/DST checks; requires Python Playwright + Chromium.
Run with --repo REPO --media SYNTHETIC_MP4 --report REPORT_JSON.
Responses are routed fixtures, not a live authenticated SimpleNVR backend.
"""
import argparse
import asyncio
import hashlib
import json
import subprocess
import tempfile
from pathlib import Path
from playwright.async_api import async_playwright


def blob(data):
    return hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()


async def run(repo, media, report):
    ui = (repo / "static/js/export-ui.js").read_bytes()
    geometry = (repo / "static/js/timeline-geometry.js").read_bytes()
    payload = media.read_bytes()
    assert 12 <= len(payload) <= 8 << 20 and payload[4:8] == b"ftyp"
    checks = []
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True, args=["--no-sandbox"])
        version = browser.version
        try:
            for case in ["nine-cameras", "http-error", "length-cap", "prepared-cancel", "saved-download"]:
                context = await browser.new_context(accept_downloads=True)
                try:
                    page = await context.new_page()
                    errors, requests = [], []
                    page.on("pageerror", lambda error: errors.append(str(error)))
                    async def route(request):
                        if "/api/download?" in request.request.url:
                            requests.append(request.request.url)
                            if case == "http-error":
                                await request.fulfill(status=422, content_type="text/plain", body="<svg onload=window.injected=1>unsafe excerpt</svg>")
                            else:
                                headers = {"Content-Type": "video/mp4", "X-Export-Range-Policy": "whole-clips-untrimmed"}
                                if case == "length-cap":
                                    headers["Content-Length"] = str(64 * 1024 * 1024 + 1)
                                await request.fulfill(status=200, headers=headers, body=payload)
                        else:
                            await request.fulfill(status=200, content_type="text/html", body='<div id="download-status"><div id="download-progress-list"></div></div><button id="export-start-btn">Export</button>')
                    await page.route("**/*", route)
                    await page.goto("http://simplenvr-fixture.invalid/")
                    await page.add_script_tag(content="let allCameras=[{id:'a',name:'<img src=x onerror=window.injected=1>'}]; let downloadCameras=['a']; let downloadStartTime=new Date('2026-11-10T00:00:00Z'),downloadEndTime=new Date('2026-11-10T00:00:01Z');")
                    await page.add_script_tag(content=ui.decode())
                    if case == "nine-cameras":
                        await page.evaluate("() => {allCameras=Array.from({length:9},(_,i)=>({id:String(i),name:String(i)}));downloadCameras=allCameras.map(x=>x.id);}")
                    await page.evaluate("() => {startExportDownloads();}")
                    if case == "nine-cameras":
                        assert "one to eight" in await page.locator("#download-progress-list").inner_text()
                        assert not requests and await page.locator(".export-job").count() == 0
                    elif case in ["http-error", "length-cap"]:
                        await page.wait_for_selector('.export-job[data-state="failed"]')
                        text = await page.locator(".export-job").inner_text()
                        assert ("unsupported" if case == "http-error" else "64 MiB") in text
                        assert await page.locator(".export-job img,.export-job svg").count() == 0
                        assert await page.evaluate("typeof window.injected") == "undefined"
                    else:
                        await page.wait_for_selector('.export-job[data-state="ready"]')
                        url = await page.evaluate("exportUiBatch.jobs[0].url")
                        assert await page.evaluate("async url => (await fetch(url)).ok", url)
                        if case == "saved-download":
                            async with page.expect_download() as pending:
                                await page.get_by_role("button", name="Hand received file to browser", exact=True).click()
                            download = await pending.value
                            assert await download.failure() is None
                            with tempfile.TemporaryDirectory() as destination:
                                saved = Path(destination) / "saved.mp4"
                                await download.save_as(saved)
                                assert hashlib.sha256(saved.read_bytes()).digest() == hashlib.sha256(payload).digest()
                                subprocess.run(["ffmpeg", "-nostdin", "-v", "error", "-i", str(saved), "-f", "null", "-"], check=True, timeout=10, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
                            assert "not verified" in await page.locator(".export-job").inner_text()
                            assert await page.evaluate("async url => (await fetch(url)).ok", url)
                            await page.get_by_role("button", name="Release prepared file / continue", exact=True).click()
                        else:
                            await page.locator(".export-job").get_by_role("button", name="Cancel", exact=True).click()
                            await page.wait_for_selector('.export-job[data-state="canceled"]')
                        assert await page.evaluate("exportUiBatch.jobs[0].url") is None
                        assert await page.evaluate("async url => {try {await fetch(url);return false;} catch (_) {return true;}}", url)
                        await page.wait_for_function("!exportUiBatch.running")
                    assert not errors, errors
                    checks.append(case)
                finally:
                    await context.close()
            context = await browser.new_context(timezone_id="America/Chicago")
            try:
                page = await context.new_page()
                await page.add_script_tag(content=geometry.decode())
                result = await page.evaluate("""() => {
                    const hours=(month,day)=>{const start=new Date(2026,month,day),end=new Date(2026,month,day+1);return TimelineGeometry.view(+start,+end).hours;};
                    return {spring:hours(2,8),fall:hours(10,1),first:TimelineGeometry.label(Date.parse('2026-11-01T06:30:00Z')),second:TimelineGeometry.label(Date.parse('2026-11-01T07:30:00Z'))};
                }""")
                assert result["spring"] == 23 and result["fall"] == 25
                assert result["first"] != result["second"] and "01:30" in result["first"] and "01:30" in result["second"]
                checks.append("browser-DST-helper")
            finally:
                await context.close()
        finally:
            await browser.close()
    evidence = {"browser": version, "headless": True, "script_blobs": {"export-ui.js": blob(ui), "timeline-geometry.js": blob(geometry)}, "checks_passed": checks, "fixture_sha256": hashlib.sha256(payload).hexdigest(), "limits": "Routed responses and controlled DOM; browser-managed automated saving, not end-user OS dialogs, full backend, playback lifecycle, media-error/autoplay acceptance, or canvas integration."}
    report.parent.mkdir(parents=True, exist_ok=True)
    report.write_text(json.dumps(evidence, indent=2) + "\n")
    return evidence


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, default=Path("."))
    parser.add_argument("--media", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    asyncio.run(run(args.repo, args.media, args.report))
