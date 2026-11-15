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


if __name__ == "__main__":
    unittest.main()
