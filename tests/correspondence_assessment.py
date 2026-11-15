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
        for name in ("bound-adapter.json", "native-prebuffer.json"):
            data, digest = load(args.directory / name, args.expected_head)
            result["source_sha256"][name] = digest
            inputs.append(data)
        result.update(assess(*inputs))
    except (ValueError, OSError, TypeError, KeyError) as error:
        result["error"] = str(error)
    args.output.write_text(json.dumps(result, indent=2, allow_nan=False) + "\n")
    lines = ["# Correspondence evidence assessment", "", "Commit: `" + args.expected_head + "`", "",
             "Evidence complete: " + str(result["evidence_complete"]),
             "Runtime qualification, cleanup authorization and resource-cost acceptance: false.", ""]
    if "error" in result:
        lines += ["Assessment error: " + result["error"], ""]
    for group in result.get("groups", []):
        lines.append("- {profile}, {mode}: n={samples}, median={adapter_median_seconds:.6f}s, max={adapter_max_seconds:.6f}s, observed CPU median={observed_cpu_median_seconds:.6f}s".format(**group))
    lines += ["", "Two cache-warm repeats are descriptive, not tail-latency or recorder-host acceptance.",
              "CPU includes child and Go collector accounting; background lifetime CPU is separate.",
              "Zero kernel block operations do not mean zero logical reads. Native matrix uses small controlled clips."]
    args.summary.write_text("\n".join(lines) + "\n")
    return 0 if result["evidence_complete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
