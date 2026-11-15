#!/usr/bin/env python3
"""Validate CI correspondence artifacts; never grant runtime acceptance."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import re
import statistics

PROFILES = ("360p_14s_1mbps", "720p_110s_2mbps")
MODES = ("idle", "paced segment writer")
NORMAL = ("one-prefix-idle", "one-prefix-paced", "two-prefix-idle", "two-prefix-paced")
MISMATCH = ("omit-first", "omit-second", "duplicate-first", "reorder")
EARLY = ("capture-only", "truncate-capture")
LIMIT = 8 * 1024 * 1024


def require(value, message):
    if not value:
        raise ValueError(message)


def number(value, label):
    require(type(value) in (int, float) and math.isfinite(value) and value >= 0, label)
    return value


def load(path, head):
    require(path.stat().st_size <= LIMIT, "artifact exceeds assessment read ceiling")
    raw = path.read_bytes()
    def pairs(items):
        result = {}
        for key, value in items:
            require(key not in result, "duplicate JSON key: " + key)
            result[key] = value
        return result
    data = json.loads(raw, object_pairs_hook=pairs,
                      parse_constant=lambda value: (_ for _ in ()).throw(ValueError("nonfinite JSON")))
    require(isinstance(data, dict) and data.get("Head") == head, "artifact head mismatch")
    require(data.get("RuntimeQualified") is False and data.get("CleanupAuthorized") is False,
            "artifact cannot grant runtime or cleanup acceptance")
    return data, hashlib.sha256(raw).hexdigest()


def lifecycle(collection, matched):
    require(isinstance(collection, dict), "missing collection")
    require(collection.get("ChildrenStarted") == 2 and collection.get("ChildrenJoined") == 2,
            "children not exactly two started/joined")
    require(collection.get("PeakUnjoinedChildren") == 2, "child peak changed")
    require(collection.get("PipeDescriptorsCreated") == 8, "pipe contract changed")
    require(collection.get("NormalizedScratchBytes") == 0, "normalized scratch appeared")
    require(collection.get("WatcherStarted") is True and collection.get("WatcherJoined") is True,
            "watcher not joined")
    require(collection.get("ComparatorWorkersStarted") == 2 and collection.get("ComparatorWorkersJoined") == 2,
            "comparators not joined")
    for kind in ("Video", "Audio"):
        pair = collection.get(kind, {})
        require(pair.get("ReaderWorkersStarted") == 2 and pair.get("ReaderWorkersJoined") == 2,
                "readers not joined")
        if matched:
            require(pair.get("Equal") is True, "positive content did not match")
        for name in ("ReferenceBytes", "ActualBytes"):
            number(pair.get(name), "invalid normalized byte count")
    if not matched:
        require(not (collection["Video"].get("Equal") is True and collection["Audio"].get("Equal") is True),
                "negative comparison claims both streams equal")


def unique(rows, keys, expected):
    require(isinstance(rows, list), "matrix absent")
    found = {}
    for row in rows:
        require(isinstance(row, dict), "invalid row")
        key = tuple(row.get(k) for k in keys)
        require(key in expected and key not in found, "duplicate or unexpected matrix row")
        found[key] = row
    require(set(found) == expected, "incomplete matrix")
    return found


def assess(bound, native):
    require(not bound.get("FailedProfiles") and not native.get("FailedCases"), "reported failed cases")
    expected = {(p, m, r) for p in PROFILES for m in MODES for r in (1, 2)}
    rows = unique(bound.get("Rows"), ("Profile", "Mode", "Repeat"), expected)
    groups = []
    for profile in PROFILES:
        for mode in MODES:
            selected = [rows[profile, mode, r] for r in (1, 2)]
            for row in selected:
                require(row.get("Outcome") == "matched" and not row.get("Error"), "bound adapter refusal")
                lifecycle(row.get("Collection"), True)
                generated, requested = (14, 13) if profile == PROFILES[0] else (110, 109)
                require(row.get("GeneratedSeconds") == generated and row.get("RequestedSeconds") == requested,
                        "fixture/request length missing or changed")
                elapsed = number(row.get("AdapterSeconds"), "invalid adapter seconds")
                inner = number(row.get("CollectorSeconds"), "invalid collector seconds")
                require(elapsed > 0 and 0 <= inner <= elapsed, "collector time outside adapter interval")
                for field in ("ChildUserCPUSeconds", "ChildSystemCPUSeconds", "CollectorUserCPUSeconds",
                              "CollectorSystemCPUSeconds", "ChildInputBlockOps", "ChildOutputBlockOps",
                              "CollectorInputBlockOps", "CollectorOutputBlockOps"):
                    number(row.get(field), "invalid CPU/I/O measurement: " + field)
                if mode != "idle":
                    require(row.get("BackgroundRunningBefore") is True and row.get("BackgroundRunningAfter") is True,
                            "background did not overlap adapter")
                    before = number(row.get("BackgroundBytesBefore"), "background byte count absent")
                    after = number(row.get("BackgroundBytesAfter"), "background byte count absent")
                    require(after >= before, "background byte count regressed")
            times = [r["AdapterSeconds"] for r in selected]
            cpu = [sum(r[k] for k in ("ChildUserCPUSeconds", "ChildSystemCPUSeconds",
                                      "CollectorUserCPUSeconds", "CollectorSystemCPUSeconds")) for r in selected]
            groups.append({"profile": profile, "mode": mode, "samples": len(times),
                           "adapter_median_seconds": statistics.median(times), "adapter_max_seconds": max(times),
                           "observed_cpu_median_seconds": statistics.median(cpu)})
    background = unique(bound.get("Background"), ("Profile",), {(p,) for p in PROFILES})
    for row in background.values():
        require(row.get("Joined") is True, "background process not joined")
        for field in ("Bytes", "UserCPUSeconds", "SystemCPUSeconds"):
            number(row.get(field), "invalid background summary")
    native_keys = {(name, repeat) for name in NORMAL + MISMATCH for repeat in (1, 2)}
    native_keys |= {(name, 0) for name in EARLY}
    observations = unique(native.get("Rows"), ("Name", "Repeat"), native_keys)
    for (name, repeat), row in observations.items():
        require(row.get("Prefixes") == (1 if name.startswith("one-prefix") else 2), "wrong prefix count")
        if name in EARLY:
            require(row.get("Stage") == "evidence refusal" and row.get("Error") not in (None, "", "<nil>"),
                    "early control did not refuse")
            continue
        matched = name in NORMAL
        require(row.get("Stage") == ("matched" if matched else "comparison refusal"), "wrong refusal layer")
        require(not row.get("Error") if matched else "stream content mismatch" in row.get("Error", ""),
                "wrong native comparison outcome")
        number(row.get("Seconds"), "invalid native elapsed time")
        lifecycle(row.get("Collection"), matched)
    return {"evidence_complete": True, "bound_rows": 8, "native_rows": 18,
            "runtime_qualified": False, "cleanup_authorized": False,
            "resource_cost_accepted": False, "groups": groups}


TRACKS = ("normal", "video-changed", "audio-changed", "both-changed", "missing-audio")
COMPLETE_CPU = ("ChildUserCPUSeconds", "ChildSystemCPUSeconds", "GoUserCPUSeconds", "GoSystemCPUSeconds")


def strict_collection(value, matched):
    require(isinstance(value, dict) and all(isinstance(value.get(k), dict) for k in ("Video", "Audio")),
            "missing stream collection")
    lifecycle(value, matched)
    for key in ("ChildrenStarted", "ChildrenJoined", "PeakUnjoinedChildren", "PipeDescriptorsCreated",
                "ComparatorWorkersStarted", "ComparatorWorkersJoined", "NormalizedScratchBytes"):
        require(type(value.get(key)) is int, "noninteger lifecycle counter: " + key)
    for kind in ("Video", "Audio"):
        pair = value[kind]
        require(type(pair.get("Equal")) is bool, "stream equality is not boolean")
        for key in ("ReaderWorkersStarted", "ReaderWorkersJoined", "ReferenceBytes", "ActualBytes"):
            require(type(pair.get(key)) is int, "noninteger reader counter: " + key)
        if matched:
            require(pair["ReferenceBytes"] > 0 and pair["ReferenceBytes"] == pair["ActualBytes"],
                    "positive stream byte counts inconsistent")
    return value


def checked_rows(report, failures, keys, expected):
    require(isinstance(report, dict) and failures in report and report[failures] in (None, []),
            "missing failure status or reported failures")
    require(isinstance(report.get("Rows"), list), "matrix absent")
    for row in report["Rows"]:
        require(isinstance(row, dict) and type(row.get("Repeat")) is int, "invalid repeat")
        require(all(type(row.get(key)) is str for key in keys if key != "Repeat"), "invalid row key")
    return unique(report["Rows"], keys, expected)


def snapshot(value):
    require(isinstance(value, dict), "writer snapshot absent")
    for key in ("Bytes", "Files"):
        require(type(value.get(key)) is int and value[key] >= 0, "invalid writer snapshot")
    return value


def follows(before, after, strict=False):
    before, after = snapshot(before), snapshot(after)
    require(all(after[k] > before[k] if strict else after[k] >= before[k] for k in ("Bytes", "Files")),
            "writer snapshot did not advance" if strict else "writer snapshot regressed")


def assess_extended(complete, tracks):
    rows = checked_rows(complete, "FailedProfiles", ("Profile", "Mode", "Repeat"),
                        {(p, m, r) for p in PROFILES for m in MODES for r in (1, 2)})
    groups = []
    for profile in PROFILES:
        for mode in MODES:
            selected = [rows[profile, mode, r] for r in (1, 2)]
            for row in selected:
                require(row.get("Error") == "", "complete validation refused")
                expected = (14, 13) if profile == PROFILES[0] else (110, 109)
                lengths = tuple(row.get(k) for k in ("GeneratedSeconds", "RequestedSeconds"))
                require(all(type(v) is int for v in lengths) and lengths == expected, "complete fixture length changed")
                validation = row.get("Validation")
                require(isinstance(validation, dict) and validation.get("EvidenceCompleted") is True,
                        "fresh evidence collection incomplete")
                evidence, comparison, total = [number(validation.get(k), "invalid complete elapsed time") for k in
                                               ("EvidenceSeconds", "ComparisonSeconds", "TotalSeconds")]
                require(evidence > 0 and comparison > 0 and total > 0 and evidence + comparison <= total + 1e-6,
                        "complete stage times exceed total")
                collection = strict_collection(validation.get("Collection"), True)
                elapsed = collection.get("Elapsed")
                require(type(elapsed) is int and 0 < elapsed <= (comparison + 1e-6) * 1e9,
                        "collector nanoseconds outside comparison stage")
                for field in COMPLETE_CPU:
                    number(row.get(field), "invalid complete CPU: " + field)
                expected_live = mode != "idle"
                require(row.get("BackgroundRunningBefore") is expected_live and
                        row.get("BackgroundRunningAfter") is expected_live, "complete background liveness inconsistent")
                follows(row.get("BackgroundBefore"), row.get("BackgroundAfter"))
                if not expected_live:
                    require(row["BackgroundBefore"] == row["BackgroundAfter"] == {"Bytes": 0, "Files": 0},
                            "idle writer observations are not zero")
            group = {"profile": profile, "mode": mode, "samples": 2}
            for field in ("EvidenceSeconds", "ComparisonSeconds", "TotalSeconds"):
                values = [row["Validation"][field] for row in selected]
                group[field + "Median"] = statistics.median(values)
                group[field + "Max"] = max(values)
            group["observed_cpu_median_seconds"] = statistics.median(sum(row[k] for k in COMPLETE_CPU) for row in selected)
            require(all(math.isfinite(v) for v in group.values() if type(v) is float), "complete summary overflow")
            groups.append(group)
    backgrounds = unique(complete.get("Background"), ("Profile",), {(p,) for p in PROFILES})
    for (profile,), row in backgrounds.items():
        require(row.get("StopRequested") is True and row.get("Joined") is True, "complete writer not stopped/joined")
        require(type(row.get("ExitError")) is str, "writer termination diagnostic absent")
        for field in ("ProgressWaitSeconds", "UserCPUSeconds", "SystemCPUSeconds"):
            number(row.get(field), "invalid complete writer accounting")
        for before, after, strict in (("Before", "AfterValidation", False),
                                     ("AfterValidation", "AfterProgress", True), ("AfterProgress", "AfterJoin", False)):
            follows(row.get(before), row.get(after), strict)
        paced = [rows[profile, MODES[1], r] for r in (1, 2)]
        follows(row["Before"], paced[0]["BackgroundBefore"])
        follows(paced[0]["BackgroundAfter"], paced[1]["BackgroundBefore"])
        require(row["AfterValidation"] == paced[1]["BackgroundAfter"], "writer final measured snapshot mismatch")
        observed = row["AfterValidation"]["Bytes"] > row["Before"]["Bytes"]
        require(row.get("ProgressDuringValidation") is observed, "writer progress flag inconsistent")
    observations = checked_rows(tracks, "FailedCases", ("Name", "Repeat"),
                                {(name, repeat) for name in TRACKS for repeat in (1, 2)})
    for (name, repeat), row in observations.items():
        matched = name == "normal"
        require(row.get("Stage") == ("matched" if matched else "comparison refusal"), "wrong track refusal layer")
        require(number(row.get("Seconds"), "invalid track elapsed time") > 0, "zero track elapsed time")
        strict_collection(row.get("Collection"), matched)
        error = row.get("Error")
        require(type(error) is str, "track error is not text")
        if matched:
            require(error == "", "normal track refused")
        else:
            allowed = ("stream content mismatch",) if name != "missing-audio" else (
                "stream content mismatch", "empty normalized stream", "offline correspondence prototype refused")
            require(any(line in allowed for line in error.splitlines()) and "context deadline exceeded" not in error,
                    "track missing semantic refusal or exceeded deadline")
    return {"complete_validation_rows": 8, "native_track_rows": 10, "complete_validation_groups": groups,
            "complete_writer_summaries": 2,
            "profiles_with_observed_validation_window_byte_progress":
                [p for (p,), row in backgrounds.items() if row["ProgressDuringValidation"]]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--expected-head", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--summary", type=Path, required=True)
    args = parser.parse_args()
    result = {"head": args.expected_head, "evidence_complete": False, "runtime_qualified": False,
              "cleanup_authorized": False, "resource_cost_accepted": False, "source_sha256": {}}
    try:
        require(re.fullmatch(r"[0-9a-f]{40}", args.expected_head), "expected head must be a full commit SHA")
        inputs = []
        for name in ("bound-adapter.json", "native-prebuffer.json", "complete-validation.json", "native-track-isolation.json"):
            data, digest = load(args.directory / name, args.expected_head)
            result["source_sha256"][name] = digest
            inputs.append(data)
        candidate = assess(*inputs[:2])
        candidate.update(assess_extended(*inputs[2:]))
        result.update(candidate)
    except (ValueError, OSError, TypeError, KeyError, OverflowError, RecursionError) as error:
        result["error"] = str(error)
    args.output.write_text(json.dumps(result, indent=2, allow_nan=False) + "\n")
    lines = ["# Correspondence evidence assessment", "", "Commit: `" + args.expected_head + "`", "",
             "Evidence complete: " + str(result["evidence_complete"]),
             "Runtime qualification, cleanup authorization and resource-cost acceptance: false.", ""]
    if "error" in result:
        lines += ["Assessment error: " + result["error"], ""]
    for group in result.get("groups", []):
        lines.append("- {profile}, {mode}: n={samples}, median={adapter_median_seconds:.6f}s, max={adapter_max_seconds:.6f}s, observed CPU median={observed_cpu_median_seconds:.6f}s".format(**group))
    for group in result.get("complete_validation_groups", []):
        lines.append("- Complete validation {profile}, {mode}: n={samples}, evidence median={EvidenceSecondsMedian:.6f}s, comparison median={ComparisonSecondsMedian:.6f}s, total median={TotalSecondsMedian:.6f}s, total max={TotalSecondsMax:.6f}s, observed CPU median={observed_cpu_median_seconds:.6f}s".format(**group))
    if result["evidence_complete"]:
        lines += ["", "Observed writer byte progress across the two-repeat validation observation window: " +
                  ", ".join(result["profiles_with_observed_validation_window_byte_progress"]),
                  "Both profile writers separately demonstrated segment/byte progress after measurement and were stopped/joined."]
    lines += ["", "Matrices required: bound adapter 8, native prebuffer 18, complete validation 8, native tracks 10; two complete-writer summaries.",
              "Complete validation excludes capture, assembly and publication; collector counts exclude probe children.",
              "Track text is diagnostic, not a reconstructed Go error tree or caller-context receipt.",
              "Writer snapshots bracket observations, not only inspector execution; post-measurement progress is separate.",
              "No writer latency, continuous-ingestion or recorder-host acceptance is claimed."]
    lines += ["", "Two cache-warm repeats are descriptive, not tail-latency or recorder-host acceptance.",
              "CPU includes child and Go collector accounting; background lifetime CPU is separate.",
              "Zero kernel block operations do not mean zero logical reads. Native matrix uses small controlled clips."]
    args.summary.write_text("\n".join(lines) + "\n")
    return 0 if result["evidence_complete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
