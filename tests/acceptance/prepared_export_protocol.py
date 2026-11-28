"""Prepared-export control protocol assertions shared by acceptance checks."""
import asyncio
import re
import time


def job_view(value, expected=None):
    assert isinstance(value, dict) and re.fullmatch(r"[0-9a-f]{64}", value.get("id", "")), value
    assert expected is None or value["id"] == expected, value
    assert value.get("state") in ("queued", "preparing", "ready", "failed", "canceled", "expired"), value
    if value["state"] == "ready":
        assert type(value.get("bytes")) is int and value["bytes"] > 0, value
        assert value.get("download_url") == "/api/exports/download?id=" + value["id"], value
    return value


async def control(request, origin, path, body=None, expected=200):
    if body is None:
        response = await request.get(origin + path, timeout=90000)
    else:
        response = await request.post(origin + path, data=body, timeout=90000)
    assert response.status == expected, (path, response.status, (await response.text())[:1000])
    if expected == 204:
        return None
    assert response.headers.get("content-type", "").split(";")[0] == "application/json", path
    raw = await response.body()
    assert len(raw) <= 8192, "control response exceeded bound"
    return await response.json()


async def ready(request, origin, value, budget=90):
    value = job_view(value)
    deadline = time.monotonic() + budget
    while value["state"] != "ready":
        assert value["state"] in ("queued", "preparing"), value
        assert time.monotonic() < deadline, "job readiness deadline exceeded"
        await asyncio.sleep(0.15)
        value = job_view(await control(request, origin, "/api/exports/status?id=" + value["id"]), value["id"])
    return value


async def create_ready(request, origin, body):
    # No automatic POST retry: an uncertain creation must not duplicate a job.
    return await ready(request, origin, await control(request, origin, "/api/exports/create", body, 202))


async def release(request, origin, value):
    await control(request, origin, "/api/exports/cancel", {"id": value["id"]}, 204)
    response = await request.get(origin + value["download_url"])
    assert response.status == 404, "released file still admits a new reader"
    assert response.headers.get("content-type", "").split(";")[0] != "video/mp4"
