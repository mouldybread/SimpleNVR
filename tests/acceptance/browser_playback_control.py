"""Rendered-control and stage-report contracts for native browser acceptance."""
from html.parser import HTMLParser
from browser_playback_edges import STATE, evaluate, wait, frame, require, FIRST


class Controls(HTMLParser):
    def __init__(self):
        super().__init__()
        self.controls = []
        self.ids = []
        self.active = None

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if attrs.get("id"):
            self.ids.append(attrs["id"])
        if attrs.get("id") == "play-pause":
            self.active = {"tag": tag, "attrs": attrs, "text": ""}
            self.controls.append(self.active)

    def handle_data(self, text):
        if self.active is not None:
            self.active["text"] += text

    def handle_endtag(self, tag):
        if self.active is not None and tag == self.active["tag"]:
            self.active = None


def template_contract(html):
    parsed = Controls();parsed.feed(html)
    require(parsed.ids.count("play-pause") == 1 and len(parsed.controls) == 1, "exactly one rendered playback button required")
    control = parsed.controls[0];attrs = control["attrs"]
    require(control["tag"] == "button" and attrs.get("type") == "button" and "disabled" not in attrs,
            "playback control must be an enabled native non-submit button")
    require(attrs.get("tabindex", "0") == "0" and "hidden" not in attrs and attrs.get("aria-hidden", "false").lower() != "true",
            "playback button must remain keyboard-accessible and exposed")
    require(control["text"].strip() == "Play" and attrs.get("aria-label") == "Play recordings",
            "initial playback label missing")
    require(attrs.get("aria-controls") == "video-grid" and parsed.ids.count("video-grid") == 1,
            "playback control target absent or ambiguous")
    require(attrs.get("onclick", "").strip().rstrip(';') == "togglePlayPause()", "playback control not wired to guarded application toggle")
    return True


def mark_failed_stages(report):
    for key in ("baseline", "playback_edges"):
        if report.get(key) == "running":
            report[key] = "failed"
    report["status"] = "failed"


async def labeled_state(cdp, playing):
    state = await evaluate(cdp, STATE)
    control = await evaluate(cdp, "(() => {const b=document.getElementById('play-pause');return {text:b.textContent,label:b.getAttribute('aria-label'),intent:playbackIntentVersion};})()")
    require(state["playingIntent"] is playing and control["text"] == ("Pause" if playing else "Play")
            and control["label"] == ("Pause recordings" if playing else "Play recordings"), "rendered control disagrees with playback intent")
    return {"state": state, "control": control}


async def native_control_contract(context, origin, legacy, diagnostics, report):
    # Deliberately a separate activated page; never the no-activation autoplay page.
    from browser_playback_edges import prepare_page
    page, cdp = await prepare_page(context, origin, legacy, diagnostics)
    try:
        button = page.locator("#play-pause")
        require(await button.count() == 1, "rendered playback button missing/duplicated")
        require(await button.is_visible() and await button.is_enabled(), "playback button is not operable")
        require(await button.get_attribute("type") == "button" and await button.get_attribute("aria-controls") == "video-grid",
                "rendered native control attributes changed")
        require(await page.get_by_role("button", name="Play recordings", exact=True).count() == 1, "initial accessible playback name absent")
        require(await button.get_attribute("tabindex") in (None, "0"), "playback button removed from keyboard order")
        await button.focus();await button.press("Enter")
        empty = await labeled_state(cdp, False)
        require(empty["state"]["sessions"] == 0 and "Select a recording time" in empty["state"]["text"],
                "Play without selected media falsely started playback")
        await evaluate(cdp, "videoPlayers[0].muted=true;seekToTime(new Date('2026-11-10T12:00:01Z'));true")
        await wait(cdp, "isPlaying && !videoPlayers[0].paused && videoPlayers[0].currentTime>1.2")
        started = await labeled_state(cdp, True)
        await button.focus();await button.press("Space")
        paused = await labeled_state(cdp, False)
        require(paused["state"]["paused"] is True and paused["control"]["intent"] == started["control"]["intent"] + 1,
                "focused Space did not toggle exactly once")
        import asyncio
        await asyncio.sleep(0.4)
        stable = await labeled_state(cdp, False)
        require(abs(stable["state"]["currentTime"] - paused["state"]["currentTime"]) < 0.1, "paused keyboard control did not stop clock")
        await button.press("Enter")
        await wait(cdp, "isPlaying && !videoPlayers[0].paused && videoPlayers[0].currentTime>" + str(paused["state"]["currentTime"] + 0.2))
        resumed = await labeled_state(cdp, True)
        require(resumed["control"]["intent"] == paused["control"]["intent"] + 1 and resumed["state"]["path"] == FIRST,
                "focused Enter did not resume exactly once")
        report.update({"empty_selection": empty, "started": started, "paused": paused,
                       "resumed": resumed, "decoded_frame": await frame(cdp), "status": "passed"})
    finally:
        await page.close()
