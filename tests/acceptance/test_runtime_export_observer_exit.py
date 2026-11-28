"""Deterministic observer-exit seams; synthetic rows are not HTTPS acceptance."""
import ast
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

import runtime_export_writer_observer as writer


def campaign_functions():
    path = Path(__file__).with_name("runtime_export_acceptance.py")
    tree = ast.parse(path.read_text())
    wanted = {"require", "parse_probe", "pressure_key", "StablePressure", "clean"}
    nodes = [node for node in tree.body if isinstance(node, (ast.FunctionDef, ast.ClassDef)) and node.name in wanted]
    if {node.name for node in nodes} != wanted:
        raise AssertionError("required campaign predicates unavailable")
    values = [ast.literal_eval(node.value) for node in tree.body if isinstance(node, ast.Assign)
              and any(isinstance(target, ast.Name) and target.id == "SOURCE" for target in node.targets)]
    if len(values) != 1 or not isinstance(values[0], str):
        raise AssertionError("required campaign source identity unavailable")
    namespace = {"SOURCE": values[0], "re": re,
                 "attach_thread_observations": writer.attach_thread_observations,
                 "writer_pressure_key": writer.writer_pressure_key}
    exec(compile(ast.Module(body=nodes, type_ignores=[]), str(path), "exec"), namespace)
    return namespace


campaign = campaign_functions()


def stat_text(start):
    return " ".join(["1"] * 21 + [str(start)]) + "\n"


def scan(mode="present", fd_count=1, task_count=1):
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp) / "20"
        (root / "fd").mkdir(parents=True)
        (root / "fdinfo").mkdir()
        (root / "task").mkdir()
        for index in range(1, fd_count + 1):
            path = root / "fd" / str(index)
            if index == 1:
                os.mkfifo(path, 0o600)
            else:
                path.write_text("synthetic descriptor")
            (root / "fdinfo" / str(index)).write_text("flags:\t" + ("01" if index == 1 else "00") + "\n")
        for index in range(task_count):
            task = root / "task" / str(21 + index)
            task.mkdir()
            (task / "wchan").write_text("anon_pipe_write\n" if index == 0 else "futex_do_wait\n")
            (task / "io").write_text("wchar: 7000000\n")
            (task / "stat").write_text(stat_text(1000 + index))
        (root / "stat").write_text("" if mode == "malformed" else stat_text(1001 if mode == "replaced" else 999))
        seam = r"""
readlink() {
  if test "$1" = "$OBS_PROC/fd/1"; then printf 'pipe:[100]\n'; else command readlink "$@"; fi
}
awk() {
  for argument in "$@"; do
    if test "$argument" = "$OBS_PROC/stat"; then
      case "$OBS_MODE" in
        gone) rm -- "$OBS_PROC/stat";;
        read-error) return 2;;
      esac
    fi
  done
  command awk "$@"
}
"""
        script = "set -eu\n" + seam + writer.THREAD_PROBE + '\nobserve_export_writer "$OBS_PROC"\nprintf "END\\n"\n'
        env = dict(os.environ, OBS_PROC=str(root), OBS_MODE=mode)
        result = subprocess.run(["sh", "-c", script], capture_output=True, text=True, timeout=5, env=env)
        lines = result.stdout.splitlines()
        return result.returncode, lines[-1:] == ["END"], "\n".join(line for line in lines if line != "END")


def observation(records, resources=True):
    rows = []
    if resources:
        rows += [["F", "4", campaign["SOURCE"], "0"],
                 ["F", "5", "/tmp/simplenvr-export-fixture (deleted)", "0"],
                 ["F", "6", "pipe:[100]", "0"], ["F", "7", "socket:[200]", "0"],
                 ["T", "200", "000186A0"]]
    rows += [["P", "20", "ffmpeg"],
             ["C", "20", "pipe:[100]", "futex_do_wait", "11072704 7214374", "6 999", "copy", campaign["SOURCE"]]]
    text = "\n".join("\t".join(row) for row in rows) + "\n" + records
    return campaign["parse_probe"](text)


class ObserverExitTests(unittest.TestCase):
    def test_healthy_control_can_qualify_pressure_but_is_not_clean(self):
        status, ended, records = scan()
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        row = observation(records)
        self.assertTrue(row["children"][0]["writer_scan_complete"])
        self.assertIsNotNone(writer.writer_pressure_key(row, 1))
        self.assertFalse(campaign["clean"](row))

    def test_disappearing_terminal_identity_keeps_a_dirty_incomplete_child(self):
        status, ended, records = scan("gone")
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        self.assertFalse(any(line.startswith("B\t") for line in records.splitlines()))
        row = observation(records)
        self.assertEqual(len(row["children"]), 1)
        self.assertFalse(row["children"][0]["writer_scan_complete"])
        self.assertIsNone(writer.writer_pressure_key(row, 1))
        self.assertFalse(campaign["clean"](row))

    def test_terminal_read_error_is_also_incomplete_not_a_clean_claim(self):
        status, ended, records = scan("read-error")
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        row = observation(records)
        self.assertFalse(row["children"][0]["writer_scan_complete"])
        self.assertIsNone(writer.writer_pressure_key(row, 1))
        self.assertFalse(campaign["clean"](row))

    def test_child_is_still_dirty_without_parent_resource_counters(self):
        status, _, records = scan("gone")
        self.assertEqual(status, 0)
        row = observation(records, resources=False)
        self.assertEqual(row["pins"], 0)
        self.assertEqual(row["anonymous_exports"], 0)
        self.assertFalse(campaign["clean"](row))

    def test_incomplete_sample_resets_existing_stability_requirement(self):
        _, _, healthy = scan()
        _, _, incomplete = scan("gone")
        good, bad = observation(healthy), observation(incomplete)
        tracker = campaign["StablePressure"](1)
        self.assertFalse(tracker.observe(good, 0.0))
        self.assertFalse(tracker.observe(good, 0.3))
        self.assertTrue(tracker.observe(good, 0.6))
        self.assertFalse(tracker.observe(bad, 0.7))
        self.assertFalse(tracker.observe(good, 1.0))
        self.assertFalse(tracker.observe(good, 1.3))
        self.assertTrue(tracker.observe(good, 1.6))

    def test_only_a_new_empty_observation_can_establish_clean_release(self):
        _, _, records = scan("gone")
        dirty = observation(records)
        self.assertFalse(campaign["clean"](dirty))
        fresh = campaign["parse_probe"]("")
        self.assertTrue(campaign["clean"](fresh))
        self.assertEqual(len(dirty["children"]), 1)
        self.assertFalse(campaign["clean"](dirty))

    def test_fd_cap_is_not_softened(self):
        status, ended, _ = scan(fd_count=64)
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        status, ended, _ = scan(fd_count=65)
        self.assertNotEqual(status, 0)
        self.assertFalse(ended)

    def test_task_cap_is_not_softened(self):
        status, ended, _ = scan(task_count=32)
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        status, ended, _ = scan(task_count=33)
        self.assertNotEqual(status, 0)
        self.assertFalse(ended)

    def test_malformed_identity_is_still_rejected(self):
        status, ended, records = scan("malformed")
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        with self.assertRaises(AssertionError):
            observation(records)

    def test_changed_identity_never_qualifies(self):
        status, ended, records = scan("replaced")
        self.assertEqual(status, 0)
        self.assertTrue(ended)
        row = observation(records)
        self.assertFalse(row["children"][0]["writer_scan_complete"])
        self.assertIsNone(writer.writer_pressure_key(row, 1))
        self.assertFalse(campaign["clean"](row))


if __name__ == "__main__":
    unittest.main()
