"""Safe diagnostics and EXIT-trap regressions, not runtime acceptance."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

spec=importlib.util.spec_from_file_location("diagnostics",Path(__file__).with_name("runtime_export_diagnostics.py"))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class ExportDiagnosticTests(unittest.TestCase):
    def report(self):
        return {"status":"failed","failure_type":"AssertionError",
                "failure":"sustained source-matched pipe-write and owned TCP queue backpressure not reached",
                "cases":{m.CASE_NAMES[0]:{"status":"passed"},m.CASE_NAMES[1]:{"status":"failed"}},
                "observations":[{"pins":1,"anonymous_exports":1,"anonymous_trims":[],"tx_queues":[1009328],
                                 "media_children":[{"pid":20}],"named_temps":[],"children":[{"pid":20,"start":999,
                                 "written":7214374,"cpu":6,"wait":"futex_do_wait","mode":"copy",
                                 "writer_scan_complete":True,"pipe_writers":[{"tid":21,"start":1000,"wait":"pipe_write","written":7000000}]}]}]}

    def test_assertion_case_and_writer_details_survive(self):
        value=m.summarize(self.report());self.assertEqual(value["failure_reason"],self.report()["failure"])
        self.assertEqual(value["cases"][m.CASE_NAMES[1]]["status"],"failed")
        self.assertEqual(value["last_recorded_observation"]["children"][0]["pipe_writers"][0]["wait"],"pipe_write")

    def test_arbitrary_assertions_and_error_names_are_suppressed(self):
        value=self.report();value.update(failure="SECRET",failure_type="SECRET")
        self.assertNotIn("SECRET",json.dumps(m.summarize(value)))

    def test_raw_network_paths_commands_and_unknown_waits_are_suppressed(self):
        value=self.report();value.update(cookie="SECRET",origin="SECRET",stderr="SECRET")
        value["observations"][0]["children"][0].update(wait="SECRET",pipe="SECRET",cmdline="SECRET",pid="SECRET")
        value["observations"][0]["children"][0]["pipe_writers"][0].update(wait="SECRET",tid="SECRET")
        self.assertNotIn("SECRET",json.dumps(m.summarize(value)))

    def test_cleanup_failure_is_distinct(self):
        value=self.report();value.update(cleanup_failure_type="AssertionError",cleanup_failure="preheader client did not join")
        result=m.summarize(value);self.assertEqual(result["cleanup_failure_reason"],"preheader client did not join")
        self.assertEqual(result["failure_reason"],value["failure"])

    def test_unfinished_and_absent_cases_are_not_promoted(self):
        value=m.summarize({"status":"failed","cases":{m.CASE_NAMES[0]:{"status":"running"}}})
        self.assertEqual(value["cases"][m.CASE_NAMES[0]]["status"],"running")
        self.assertEqual(value["cases"][m.CASE_NAMES[1]]["status"],"unavailable")

    def test_success_does_not_authorize_cleanup_or_runtime_qualification(self):
        value=self.report();value.update(status="passed",runtime_qualified=True,cleanup_authorized=True)
        result=m.summarize(value);self.assertFalse(result["acceptance_inferred"]);self.assertNotIn("cleanup_authorized",result)

    def test_capped_history_is_not_labelled_failure_time_state(self):
        value=self.report();value["observations"]*=100;result=m.summarize(value)
        self.assertTrue(result["observations_at_or_above_recording_limit"]);self.assertNotIn("state_at_failure",result)

    def test_invalid_counters_are_not_forwarded(self):
        for value in (True,-1,1<<80,"SECRET",None,{}):self.assertIsNone(m.counter(value))
        self.assertEqual(m.counter(65536),65536)

    def test_hostile_shapes_and_child_counts_are_bounded(self):
        value=self.report();value["observations"][0]["children"]*=100
        self.assertLessEqual(len(m.summarize(value)["last_recorded_observation"]["children"]),4)
        for value in (None,[],"SECRET",1):self.assertEqual(m.summarize(value)["report_status"],"invalid")

    def test_loader_leaves_report_bytes_unchanged(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"report";raw=json.dumps(self.report());path.write_text(raw)
            self.assertEqual(m.load_summary(path)["report_status"],"failed");self.assertEqual(path.read_text(),raw)
            self.assertEqual(len(list(Path(temp).iterdir())),1)

    def test_missing_invalid_and_large_reports_are_explicit(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"report";self.assertEqual(m.load_summary(path)["report_status"],"missing")
            for raw in (b"SECRET",b"\xff",b"{"*2000):
                path.write_bytes(raw);self.assertEqual(m.load_summary(path)["report_status"],"invalid")
            path.write_bytes(b" "*(m.MAX_REPORT_BYTES+1));self.assertEqual(m.load_summary(path)["report_status"],"too_large")

    def test_symlink_fifo_and_directory_are_not_read(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);target=root/"target";target.write_text("SECRET");link=root/"link";link.symlink_to(target)
            fifo=root/"fifo";os.mkfifo(fifo)
            self.assertEqual(m.load_summary(link)["report_status"],"unavailable")
            self.assertEqual(m.load_summary(fifo)["report_status"],"not_regular")
            self.assertEqual(m.load_summary(root)["report_status"],"not_regular")

    def test_summary_does_not_mutate_input(self):
        value=self.report();original=copy.deepcopy(value);m.summarize(value);self.assertEqual(original,value)

    def test_real_cli_has_safe_reason_and_no_traceback(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"report";path.write_text(json.dumps(self.report()))
            result=subprocess.run([sys.executable,m.__file__,str(path)],capture_output=True,text=True,timeout=3)
            self.assertEqual(result.returncode,0);self.assertEqual(result.stderr,"")
            self.assertEqual(json.loads(result.stdout)["failure_reason"],self.report()["failure"])

    def test_exact_exit_trap_preserves_failure_and_cleanup_accounting(self):
        path=Path(__file__).with_name("runtime-export-acceptance.sh");shell=path.read_text()
        cleanup=shell[shell.index("cleanup() {"):shell.index("trap cleanup EXIT")]
        for code,fail in ((0,False),(1,False),(0,True)):
            with self.subTest(code=code,fail=fail),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);evidence=root/"evidence";evidence.mkdir();binary=root/"bin";binary.mkdir()
                value=self.report();value["status"]="failed" if code else "passed";raw=json.dumps(value)
                (evidence/"runtime-export-report.json").write_text(raw)
                (binary/"docker").write_text("#!/bin/sh\ncase \"$1\" in\n logs) printf 'Shutdown complete; recording intent persisted\\n' >&2;;\n ps) test \"$DIAGNOSTIC_TEST_RM_FAIL\" != 1 || printf '42\\n'; exit 0;;\n rm) exit 1;;\nesac\n")
                (binary/"sudo").write_text("#!/bin/sh\nexit 0\n")
                for name in ("docker","sudo"):(binary/name).chmod(0o700)
                env=dict(os.environ,PATH=str(binary)+os.pathsep+os.environ["PATH"],DIAGNOSTIC_TEST_RM_FAIL="1" if fail else "0")
                script='set -euo pipefail\nevidence=$1\nwork=$2\nname=simplenvr-runtime-export-123\n'+cleanup+'trap cleanup EXIT\nexit "$3"\n'
                result=subprocess.run(["bash","-c",script,"test",str(evidence),str(root),str(code)],cwd=path.parent.parent.parent,env=env,capture_output=True,text=True,timeout=5)
                self.assertEqual(result.returncode,1 if fail else code)
                self.assertIn('"diagnostic_only": true',result.stdout)
                self.assertEqual((evidence/"runtime-export-report.json").read_text(),raw)
                result=json.loads((evidence/"runtime-export-fixture-cleanup.json").read_text())
                self.assertEqual(result["campaign_exit"],code);self.assertEqual(result["fixture_cleanup_failed"],fail)


if __name__=="__main__":unittest.main()
