"""Tooling regressions; these are not runtime-image acceptance results."""
import copy
import importlib.util
import json
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import threading
import unittest
from types import SimpleNamespace
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("runtime_exports", Path(__file__).with_name("runtime_export_acceptance.py"))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def observation(count=1, queue="00100000", wait="pipe_write"):
    lines = []
    for index in range(count):
        pid, pipe, inode = 20 + index, 100 + index, 200 + index
        lines.extend([f"F\t{4+index}\t{m.SOURCE}\t0",
                      f"F\t{10+index}\t/tmp/simplenvr-export-fixture{index}.ffconcat (deleted)\t0",
                      f"F\t{20+index}\tpipe:[{pipe}]\t0",
                      f"F\t{30+index}\tsocket:[{inode}]\t0",
                      f"P\t{pid}\tffmpeg",
                      f"C\t{pid}\tpipe:[{pipe}]\t{wait}\t100000 65536\t10 999\tcopy\t{m.SOURCE}",
                      f"T\t{inode}\t{queue}"])
        lines.extend([f"D\t{pid}\t1\tpipe:[{pipe}]", f"B\t{pid}\t999\t1"])
        if wait in ("pipe_write", "anon_pipe_write"):
            lines.append(f"W\t{pid}\t{pid+100}\t1000\t{wait}\t65536")
    return "\n".join(lines)


class WitnessTests(unittest.TestCase):
    def test_owned_socket_and_source_child_correspondence(self):
        row = m.parse_probe(observation())
        self.assertEqual(row["pins"], 1)
        self.assertEqual(row["tx_queues"], [1 << 20])
        self.assertIsNotNone(m.pressure_key(row, 1))

    def test_queue_alone_is_not_pressure(self):
        row = m.parse_probe("F\t8\tsocket:[200]\t0\nT\t200\t00100000")
        self.assertIsNone(m.pressure_key(row, 1))

    def test_child_alone_is_not_pressure(self):
        self.assertIsNone(m.pressure_key(m.parse_probe(observation(queue="0")), 1))
        self.assertIsNone(m.pressure_key(m.parse_probe(observation(wait="hrtimer_nanosleep")), 1))

    def test_only_owned_socket_queues_count(self):
        text = observation().replace("T\t200", "T\t999")
        self.assertIsNone(m.pressure_key(m.parse_probe(text), 1))

    def test_pipe_must_be_owned_by_application(self):
        text = observation().replace("F\t20\tpipe:[100]\t0\n", "")
        self.assertIsNone(m.pressure_key(m.parse_probe(text), 1))

    def test_two_slots_require_two_simultaneous_children_and_pins(self):
        self.assertIsNone(m.pressure_key(m.parse_probe(observation()), 2))
        self.assertIsNotNone(m.pressure_key(m.parse_probe(observation(2)), 2))

    def test_non_source_media_child_blocks_clean_claim(self):
        self.assertFalse(m.clean(m.parse_probe("P\t90\tffprobe")))
        self.assertFalse(m.clean(m.parse_probe("P\t91\tffmpeg")))
        self.assertTrue(m.clean(m.parse_probe("")))

    def test_stability_needs_duration_and_three_observations(self):
        row = m.parse_probe(observation())
        witness = m.StablePressure(1)
        self.assertFalse(witness.observe(row, 0))
        self.assertFalse(witness.observe(row, 0.3))
        self.assertTrue(witness.observe(row, 0.6))

    def test_progress_and_identity_changes_reset_stability(self):
        for field in ("pid", "start", "written"):
            with self.subTest(field=field):
                row = m.parse_probe(observation())
                witness = m.StablePressure(1)
                witness.observe(row, 0)
                witness.observe(row, 0.3)
                changed = copy.deepcopy(row)
                if field == "written":
                    changed["children"][0]["pipe_writers"][0]["written"] += 1
                else:
                    changed["children"][0][field] += 1
                self.assertFalse(witness.observe(changed, 0.6))
                self.assertFalse(witness.observe(changed, 0.9))
                self.assertTrue(witness.observe(changed, 1.2))

    def test_queue_progress_or_missing_wait_resets_stability(self):
        row = m.parse_probe(observation())
        witness = m.StablePressure(1)
        witness.observe(row, 0)
        witness.observe(row, 0.3)
        self.assertFalse(witness.observe(m.parse_probe(observation(queue="00010000")), 0.6))
        self.assertFalse(witness.observe(m.parse_probe(observation(wait="0")), 1.2))
        self.assertFalse(witness.observe(row, 2))

    def test_explicit_kernel_pipe_wait_names_only(self):
        self.assertIsNotNone(m.pressure_key(m.parse_probe(observation(wait="anon_pipe_write")), 1))
        self.assertIsNone(m.pressure_key(m.parse_probe(observation(wait="unknown")), 1))

    def test_named_and_anonymous_temporary_resources_block_clean_claim(self):
        for text in ["N\tsimplenvr-trim-a.mp4", "F\t4\t/tmp/simplenvr-trim-a.mp4 (deleted)\t20",
                     "F\t4\t/tmp/simplenvr-export-a.ffconcat (deleted)\t0", f"F\t4\t{m.SOURCE}\t0"]:
            self.assertFalse(m.clean(m.parse_probe(text)))

    def test_malformed_unknown_and_oversize_probe_rejected(self):
        for text in ["SECRET\tvalue", "T\t200\tno", "C\t12", "x"*32769,
                     "F\t1\t/recordings/.auth/credentials.json\t1", "P\t1\tsh"]:
            with self.subTest(text=text[:20]), self.assertRaises(AssertionError):
                m.parse_probe(text)

    def test_probe_never_emits_raw_sensitive_proc_information(self):
        self.assertNotIn("environ", m.PROBE)
        self.assertNotIn("cat /proc/1/net", m.PROBE)
        self.assertNotIn("cat \"$p/cmdline\"", m.PROBE)
        self.assertNotIn("credentials.json", m.PROBE)

    def test_preheader_result_does_not_assume_one_status(self):
        self.assertTrue(m.no_video_response({"status": 200, "video": False, "disposition": False, "trim_applied": False}))
        self.assertTrue(m.no_video_response({"transport_error": "RemoteDisconnected"}))
        for field in ("video", "disposition", "trim_applied"):
            value = dict(status=200, video=False, disposition=False, trim_applied=False)
            value[field] = True
            self.assertFalse(m.no_video_response(value))
        self.assertFalse(m.no_video_response(None))

    def test_failed_case_and_cleanup_evidence_are_written(self):
        with tempfile.TemporaryDirectory() as tmp:
            campaign = m.Campaign(SimpleNamespace(commit="a"*40, image_id="sha256:test", evidence=Path(tmp)))
            def fail(row):
                row["partial_witness"] = True
                raise AssertionError("exercise condition not reached")
            with self.assertRaises(AssertionError):
                campaign.case("partial", fail)
            saved = json.loads((Path(tmp)/"runtime-export-report.json").read_text())
            self.assertEqual(saved["status"], "failed")
            self.assertEqual(saved["cases"]["partial"]["status"], "failed")
            self.assertTrue(saved["cases"]["partial"]["partial_witness"])
            self.assertFalse(saved["cleanup_authorized"])

    def test_case_does_not_record_arbitrary_exception_message(self):
        with tempfile.TemporaryDirectory() as tmp:
            campaign = m.Campaign(SimpleNamespace(commit="a"*40, image_id="test", evidence=Path(tmp)))
            with self.assertRaises(OSError):
                campaign.case("secret", lambda row: (_ for _ in ()).throw(OSError("sensitive-token")))
            self.assertNotIn("sensitive-token", (Path(tmp)/"runtime-export-report.json").read_text())

    def test_preheader_worker_joins_after_transport_failure(self):
        class Connection:
            def getresponse(self):
                raise OSError("private details")
        worker = m.PreheaderRequest(Connection())
        worker.thread.start()
        worker.join()
        self.assertTrue(worker.done.is_set())
        self.assertEqual(worker.result, {"transport_error": "OSError"})

    def test_real_loopback_tls_client_positive_control(self):
        with tempfile.TemporaryDirectory() as tmp:
            cert, key = Path(tmp)/"cert.pem", Path(tmp)/"key.pem"
            subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                            "-subj", "/CN=localhost", "-keyout", str(key), "-out", str(cert)],
                           check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, timeout=5)
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.load_cert_chain(cert, key)
            listener = socket.socket()
            listener.bind(("127.0.0.1", 0))
            listener.listen(2)
            listener.settimeout(3)
            errors = []
            def serve():
                try:
                    for _ in range(2):
                        raw, _ = listener.accept()
                        raw.settimeout(3)
                        with context.wrap_socket(raw, server_side=True) as conn:
                            data = b""
                            while b"\r\n\r\n" not in data:
                                data += conn.recv(1024)
                                if len(data)>4096:
                                    raise AssertionError("request cap")
                            conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
                except BaseException as error:
                    errors.append(type(error).__name__)
            thread = threading.Thread(target=serve, daemon=False)
            thread.start()
            try:
                for small in (False, True):
                    conn = m.FixtureTLS(listener.getsockname()[1], small_receive=small)
                    try:
                        conn.request("GET", "/control")
                        response = conn.getresponse()
                        self.assertEqual(response.status, 200)
                        self.assertEqual(response.read(), b"ok")
                    finally:
                        conn.close()
            finally:
                thread.join(4)
                listener.close()
                thread.join(1)
            self.assertFalse(thread.is_alive())
            self.assertFalse(errors)


    def test_tracked_response_and_connection_both_close(self):
        campaign = m.Campaign(SimpleNamespace(commit="a"*40, image_id="test"))
        closed = []
        campaign.responses = [SimpleNamespace(close=lambda: closed.append("response"))]
        campaign.connections = [SimpleNamespace(close=lambda: closed.append("connection"))]
        campaign.close_connections()
        self.assertEqual(closed, ["response", "connection"])
        self.assertFalse(campaign.responses or campaign.connections)

    def test_shutdown_accepts_markers_on_stderr_but_not_dirty_exit(self):
        for exit_code in (0, 137):
            with self.subTest(exit_code=exit_code):
                campaign = m.Campaign(SimpleNamespace(commit="a"*40, image_id="test", container="fixture"))
                before = {"config_mtime_ns": 1}
                after = {"config_mtime_ns": 2, "config_mode": 0o600}
                campaign.snapshot = lambda: before
                campaign.unchanged = lambda: after
                def docker(*args, **kwargs):
                    if args[0] == "inspect":
                        return json.dumps([{"State":{"Running":False,"ExitCode":exit_code,"OOMKilled":False}}])
                    return ""
                campaign.docker = docker
                observed = [SimpleNamespace(returncode=0,stdout=b"",stderr=b""),
                            SimpleNamespace(returncode=0,stdout=b"",stderr=m.MARKER.encode())]
                with patch.object(m.subprocess, "run", side_effect=observed):
                    if exit_code:
                        with self.assertRaises(AssertionError):
                            campaign.shutdown("test")
                    else:
                        row = campaign.shutdown("test")
                        self.assertEqual(row["new_completion_markers"], 1)
                        self.assertFalse(row["internal_persistence_order_observed"])

    def test_cleanup_error_cannot_leave_successful_case(self):
        campaign = m.Campaign(SimpleNamespace(commit="a"*40, image_id="test"))
        closed = []
        def fail_close():
            raise OSError("not recorded")
        campaign.responses = [SimpleNamespace(close=fail_close)]
        campaign.connections = [SimpleNamespace(close=lambda:closed.append(True))]
        with self.assertRaises(AssertionError):
            campaign.close_connections()
        self.assertEqual(closed, [True])


if __name__ == "__main__":
    unittest.main()
