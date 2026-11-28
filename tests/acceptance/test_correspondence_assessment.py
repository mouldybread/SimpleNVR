import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("assessment", Path(__file__).with_name("correspondence_assessment.py"))
a = importlib.util.module_from_spec(spec)
spec.loader.exec_module(a)


def collection(matched=True):
    pair = {"Equal": matched, "ReaderWorkersStarted": 2, "ReaderWorkersJoined": 2,
            "ReferenceBytes": 100, "ActualBytes": 100}
    return {"ChildrenStarted": 2, "ChildrenJoined": 2, "PeakUnjoinedChildren": 2,
            "PipeDescriptorsCreated": 8, "NormalizedScratchBytes": 0,
            "WatcherStarted": True, "WatcherJoined": True,
            "ComparatorWorkersStarted": 2, "ComparatorWorkersJoined": 2,
            "Video": dict(pair), "Audio": dict(pair)}


def fixtures():
    bound = {"Rows": [], "Background": [], "FailedProfiles": None}
    for profile in a.PROFILES:
        generated, requested = (14, 13) if profile == a.PROFILES[0] else (110, 109)
        bound["Background"].append({"Profile": profile, "Joined": True, "Bytes": 100,
                                    "UserCPUSeconds": 0.1, "SystemCPUSeconds": 0.1})
        for mode in a.MODES:
            for repeat in (1, 2):
                row = {"Profile": profile, "Mode": mode, "Repeat": repeat, "Outcome": "matched", "Error": "",
                       "GeneratedSeconds": generated, "RequestedSeconds": requested,
                       "AdapterSeconds": 0.5, "CollectorSeconds": 0.4, "Collection": collection(),
                       "BackgroundRunningBefore": True, "BackgroundRunningAfter": True,
                       "BackgroundBytesBefore": 100, "BackgroundBytesAfter": 100}
                for field in ("ChildUserCPUSeconds", "ChildSystemCPUSeconds", "CollectorUserCPUSeconds",
                              "CollectorSystemCPUSeconds", "ChildInputBlockOps", "ChildOutputBlockOps",
                              "CollectorInputBlockOps", "CollectorOutputBlockOps"):
                    row[field] = 0
                bound["Rows"].append(row)
    native = {"Rows": [], "FailedCases": None}
    for name in a.NORMAL + a.MISMATCH:
        for repeat in (1, 2):
            matched = name in a.NORMAL
            native["Rows"].append({"Name": name, "Repeat": repeat, "Prefixes": 1 if name.startswith("one-prefix") else 2,
                                   "Stage": "matched" if matched else "comparison refusal",
                                   "Error": "" if matched else "stream content mismatch", "Seconds": 0.1,
                                   "Collection": collection(matched)})
    for name in a.EARLY:
        native["Rows"].append({"Name": name, "Repeat": 0, "Prefixes": 2,
                              "Stage": "evidence refusal", "Error": "assembly inconsistent"})
    return bound, native


class AssessmentTests(unittest.TestCase):
    def test_complete(self):
        result = a.assess(*fixtures())
        self.assertEqual((result["bound_rows"], result["native_rows"]), (8, 18))
        self.assertFalse(result["resource_cost_accepted"])
        self.assertEqual(len(result["groups"]), 4)

    def test_bad_matrices_and_measurements(self):
        for kind in ("missing", "duplicate", "refused", "unjoined", "nan", "negative", "bool", "infinity",
                     "background", "background_join", "wrong_layer", "wrong_mismatch", "nil_refusal", "fixture_length"):
            with self.subTest(kind=kind):
                b, n = fixtures()
                r = b["Rows"][0]
                if kind == "missing": b["Rows"].pop()
                elif kind == "duplicate": b["Rows"][-1] = copy.deepcopy(r)
                elif kind == "refused": r["Outcome"] = "refused"
                elif kind == "unjoined": r["Collection"]["ChildrenJoined"] = 1
                elif kind == "nan": r["AdapterSeconds"] = float("nan")
                elif kind == "negative": r["ChildUserCPUSeconds"] = -1
                elif kind == "bool": r["AdapterSeconds"] = True
                elif kind == "infinity": r["AdapterSeconds"] = float("inf")
                elif kind == "background": b["Rows"][2]["BackgroundRunningAfter"] = False
                elif kind == "background_join": b["Background"][0]["Joined"] = False
                elif kind == "wrong_layer": n["Rows"][8]["Stage"] = "evidence refusal"
                elif kind == "wrong_mismatch": n["Rows"][8]["Error"] = "timeout"
                elif kind == "nil_refusal": n["Rows"][-1]["Error"] = "<nil>"
                elif kind == "fixture_length": r["RequestedSeconds"] = 14
                with self.assertRaises((ValueError, TypeError)):
                    a.assess(b, n)

    def test_report_identity_and_strict_json(self):
        head = "a" * 40
        base = {"Head": head, "RuntimeQualified": False, "CleanupAuthorized": False}
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "report.json"
            for text in (json.dumps(dict(base, Head="b" * 40)),
                         json.dumps(dict(base, CleanupAuthorized=True)),
                         '{"Head":"' + head + '","Head":"' + head + '"}',
                         '{"Head":NaN}'):
                p.write_text(text)
                with self.assertRaises(ValueError): a.load(p, head)
            p.write_text(json.dumps(base))
            data, digest = a.load(p, head)
            self.assertEqual(data, base)
            self.assertEqual(len(digest), 64)


def extended_fixtures():
    complete = {"Rows": [], "FailedProfiles": None, "Background": []}
    zero = {"Bytes": 0, "Files": 0}
    before, after = {"Bytes": 100, "Files": 2}, {"Bytes": 110, "Files": 2}
    for profile in a.PROFILES:
        for mode in a.MODES:
            for repeat in (1, 2):
                generated, requested = (14, 13) if profile == a.PROFILES[0] else (110, 109)
                c = collection();c["Elapsed"] = 100_000_000
                row = {"Profile": profile, "Mode": mode, "Repeat": repeat, "Error": "",
                       "GeneratedSeconds": generated, "RequestedSeconds": requested,
                       "BackgroundRunningBefore": mode != "idle", "BackgroundRunningAfter": mode != "idle",
                       "BackgroundBefore": dict(zero if mode == "idle" else before if repeat == 1 else after),
                       "BackgroundAfter": dict(zero if mode == "idle" else after),
                       "Validation": {"EvidenceCompleted": True, "EvidenceSeconds": 0.2,
                                      "ComparisonSeconds": 0.1, "TotalSeconds": 0.31, "Collection": c}}
                row.update({field: 0.01 for field in a.COMPLETE_CPU})
                complete["Rows"].append(row)
        complete["Background"].append({"Profile": profile, "Before": dict(before), "AfterValidation": dict(after),
                                       "AfterProgress": {"Bytes": 200, "Files": 3}, "AfterJoin": {"Bytes": 210, "Files": 3},
                                       "ProgressDuringValidation": True, "StopRequested": True, "Joined": True,
                                       "ExitError": "signal: killed", "ProgressWaitSeconds": 1.0,
                                       "UserCPUSeconds": 0.01, "SystemCPUSeconds": 0.01})
    tracks = {"Rows": [], "FailedCases": None}
    for name in a.TRACKS:
        for repeat in (1, 2):
            matched = name == "normal"
            tracks["Rows"].append({"Name": name, "Repeat": repeat, "Stage": "matched" if matched else "comparison refusal",
                                   "Seconds": 0.05, "Collection": collection(matched),
                                   "Error": "" if matched else "stream content mismatch\ncontext canceled\nsignal: killed"})
    return complete, tracks


class ExtendedAssessmentTests(unittest.TestCase):
    def test_complete_and_summaries(self):
        result = a.assess_extended(*extended_fixtures())
        self.assertEqual((result["complete_validation_rows"], result["native_track_rows"]), (8, 10))
        self.assertEqual(result["complete_writer_summaries"], 2)
        self.assertEqual(len(result["complete_validation_groups"]), 4)
        self.assertEqual(result["complete_validation_groups"][0]["TotalSecondsMax"], 0.31)

    def test_new_report_faults(self):
        cases = ("missing complete", "missing track", "duplicate complete", "duplicate track", "failed complete",
                 "failed track", "missing failure status", "repeat bool", "fixture bool", "fixture length", "collection incomplete",
                 "evidence nan", "total too small", "zero comparison", "CPU negative", "CPU bool", "CPU overflow",
                 "unjoined", "scratch", "reader bool", "empty positive", "unequal positive sizes", "elapsed outside",
                 "background ended", "idle claims background", "normal refused", "wrong layer", "bare cancel",
                 "deadline plus mismatch", "false positive", "nil error", "substring only", "track zero time")
        for kind in cases:
            with self.subTest(kind=kind):
                c, t = extended_fixtures();r = c["Rows"][0];v = r["Validation"];n = t["Rows"][-1]
                if kind == "missing complete": c["Rows"].pop()
                elif kind == "missing track": t["Rows"].pop()
                elif kind == "duplicate complete": c["Rows"][-1] = copy.deepcopy(r)
                elif kind == "duplicate track": t["Rows"][-1] = copy.deepcopy(t["Rows"][0])
                elif kind == "failed complete": c["FailedProfiles"] = ["failed"]
                elif kind == "failed track": t["FailedCases"] = ["failed"]
                elif kind == "missing failure status": del c["FailedProfiles"]
                elif kind == "repeat bool": r["Repeat"] = True
                elif kind == "fixture bool": r["GeneratedSeconds"] = True
                elif kind == "fixture length": r["RequestedSeconds"] = 14
                elif kind == "collection incomplete": v["EvidenceCompleted"] = False
                elif kind == "evidence nan": v["EvidenceSeconds"] = float("nan")
                elif kind == "total too small": v["TotalSeconds"] = 0.25
                elif kind == "zero comparison": v["ComparisonSeconds"] = 0
                elif kind == "CPU negative": r[a.COMPLETE_CPU[0]] = -1
                elif kind == "CPU bool": r[a.COMPLETE_CPU[0]] = True
                elif kind == "CPU overflow": r.update({field: 1e308 for field in a.COMPLETE_CPU})
                elif kind == "unjoined": v["Collection"]["ChildrenJoined"] = 1
                elif kind == "scratch": v["Collection"]["NormalizedScratchBytes"] = 1
                elif kind == "reader bool": v["Collection"]["Video"]["ReferenceBytes"] = True
                elif kind == "empty positive": v["Collection"]["Audio"].update(ReferenceBytes=0, ActualBytes=0)
                elif kind == "unequal positive sizes": v["Collection"]["Audio"]["ActualBytes"] = 99
                elif kind == "elapsed outside": v["Collection"]["Elapsed"] = 200_000_000
                elif kind == "background ended": c["Rows"][2]["BackgroundRunningAfter"] = False
                elif kind == "idle claims background": r["BackgroundRunningBefore"] = True
                elif kind == "normal refused": t["Rows"][0]["Error"] = "context canceled"
                elif kind == "wrong layer": n["Stage"] = "evidence refusal"
                elif kind == "bare cancel": n["Error"] = "context canceled"
                elif kind == "deadline plus mismatch": n["Error"] += "\ncontext deadline exceeded"
                elif kind == "false positive": n["Collection"] = collection(True)
                elif kind == "nil error": n["Error"] = None
                elif kind == "substring only": n["Error"] = "not a stream content mismatch"
                elif kind == "track zero time": n["Seconds"] = 0
                with self.assertRaises((ValueError, TypeError)):
                    a.assess_extended(c, t)

    def test_missing_audio_teardown_diagnostics(self):
        for semantic in ("stream content mismatch", "empty normalized stream", "offline correspondence prototype refused"):
            c, t = extended_fixtures()
            t["Rows"][-1]["Error"] = semantic + "\nsignal: killed\nexit status 234\ncontext canceled"
            a.assess_extended(c, t)

    def test_writer_faults(self):
        for kind in ("missing", "duplicate", "unjoined", "unstopped", "no progress", "snapshot bool", "snapshot absent",
                     "regressed row", "regressed join", "bad flag", "bad final measured", "idle bytes", "bad CPU"):
            with self.subTest(kind=kind):
                c, t = extended_fixtures();b = c["Background"][0]
                if kind == "missing": c["Background"].pop()
                elif kind == "duplicate": c["Background"][1] = copy.deepcopy(b)
                elif kind == "unjoined": b["Joined"] = False
                elif kind == "unstopped": b["StopRequested"] = False
                elif kind == "no progress": b["AfterProgress"] = dict(b["AfterValidation"])
                elif kind == "snapshot bool": b["Before"]["Bytes"] = True
                elif kind == "snapshot absent": del c["Rows"][2]["BackgroundBefore"]
                elif kind == "regressed row": c["Rows"][2]["BackgroundAfter"]["Bytes"] = 99
                elif kind == "regressed join": b["AfterJoin"]["Files"] = 1
                elif kind == "bad flag": b["ProgressDuringValidation"] = False
                elif kind == "bad final measured": b["AfterValidation"]["Bytes"] = 109
                elif kind == "idle bytes": c["Rows"][0]["BackgroundBefore"]["Bytes"] = 1
                elif kind == "bad CPU": b["UserCPUSeconds"] = -1
                with self.assertRaises((ValueError, TypeError)):
                    a.assess_extended(c, t)

    def test_no_progress_in_short_measurement_is_not_fabricated(self):
        c, t = extended_fixtures()
        for r in c["Rows"]:
            if r["Mode"] != "idle":
                r["BackgroundBefore"] = {"Bytes": 100, "Files": 2}
                r["BackgroundAfter"] = dict(r["BackgroundBefore"])
        for b in c["Background"]:
            b["AfterValidation"] = dict(b["Before"]);b["ProgressDuringValidation"] = False
        result = a.assess_extended(c, t)
        self.assertEqual(result["profiles_with_observed_validation_window_byte_progress"], [])

    def test_cli_all_four_reports_and_fail_closed(self):
        import subprocess
        import sys
        head = "a" * 40
        names = ("bound-adapter.json", "native-prebuffer.json", "complete-validation.json", "native-track-isolation.json")
        with tempfile.TemporaryDirectory() as directory:
            d = Path(directory)
            originals = dict(zip(names, fixtures() + extended_fixtures()))
            for scenario in ("complete", "missing", "wrong head", "duplicate key", "unauthorized", "nan", "matrix"):
                for name, data in originals.items():
                    report = dict(data, Head=head, RuntimeQualified=False, CleanupAuthorized=False)
                    (d / name).write_text(json.dumps(report))
                p = d / names[-1]
                if scenario == "missing": p.unlink()
                elif scenario == "wrong head": p.write_text(p.read_text().replace(head, "b" * 40))
                elif scenario == "duplicate key": p.write_text(p.read_text().replace('"Head":', '"Head":"' + head + '","Head":'))
                elif scenario == "unauthorized": p.write_text(p.read_text().replace('"CleanupAuthorized": false', '"CleanupAuthorized": true'))
                elif scenario == "nan": p.write_text(p.read_text().replace('"Seconds": 0.05', '"Seconds": NaN'))
                elif scenario == "matrix":
                    report = json.loads(p.read_text());report["Rows"].pop();p.write_text(json.dumps(report))
                command = [sys.executable, str(Path(a.__file__)), directory, "--expected-head", head,
                           "--output", str(d / "assessment.json"), "--summary", str(d / "assessment.md")]
                result = subprocess.run(command, capture_output=True, text=True, timeout=10)
                data = json.loads((d / "assessment.json").read_text())
                self.assertEqual(result.returncode, 0 if scenario == "complete" else 1, result.stderr)
                self.assertIs(data["evidence_complete"], scenario == "complete")
                for flag in ("runtime_qualified", "cleanup_authorized", "resource_cost_accepted"):
                    self.assertIs(data[flag], False)
                if scenario == "complete":
                    self.assertEqual(set(data["source_sha256"]), set(names))
                    self.assertIn("total median=0.310000s", (d / "assessment.md").read_text())
                else:
                    self.assertIn("error", data)
                    self.assertNotIn("complete_validation_groups", data)


if __name__ == "__main__":
    unittest.main()
