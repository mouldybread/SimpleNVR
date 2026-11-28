"""Task-level writer witness tests; supplied queues are not HTTPS acceptance."""
import copy
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest

spec = importlib.util.spec_from_file_location("writer_observer", Path(__file__).with_name("runtime_export_writer_observer.py"))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def row():
    result = {"pins": 1, "anonymous_exports": 1, "media_children": [{"pid": 20}],
              "tx_queues": [1009328], "named_temps": [], "pipes": ["pipe:[100]"],
              "children": [{"pid": 20, "start": 999, "pipe": "pipe:[100]", "wait": "futex_do_wait",
                            "read": 11072704, "written": 7214374, "cpu": 6, "mode": "copy"}]}
    return m.attach_thread_observations(result, [
        ["D", "20", "1", "pipe:[100]"], ["D", "20", "8", "pipe:[100]"],
        ["W", "20", "21", "1000", "pipe_write", "7000000"], ["B", "20", "999", "1"]])


class WriterObserverTests(unittest.TestCase):
    def test_futex_leader_with_real_pipe_wait_worker_can_qualify(self):
        self.assertIsNotNone(m.writer_pressure_key(row(), 1))

    def test_futex_alone_does_not_qualify(self):
        value=row();value["children"][0]["pipe_writers"]=[]
        self.assertIsNone(m.writer_pressure_key(value, 1))

    def test_process_logging_does_not_change_writer_identity(self):
        first=row();second=copy.deepcopy(first);second["children"][0]["written"]+=120
        self.assertEqual(m.writer_pressure_key(first,1),m.writer_pressure_key(second,1))

    def test_writer_progress_or_identity_changes_reset_key(self):
        for field in ("tid","start","written"):
            first=row();second=copy.deepcopy(first)
            second["children"][0]["pipe_writers"][0][field]+=1
            self.assertNotEqual(m.writer_pressure_key(first,1),m.writer_pressure_key(second,1))

    def test_stdout_aliases_must_reference_the_same_pipe(self):
        value=row();self.assertIsNotNone(m.writer_pressure_key(value,1))
        value["children"][0]["writable_pipes"][1]["pipe"]="pipe:[200]"
        self.assertIsNone(m.writer_pressure_key(value,1))

    def test_stderr_or_unrelated_pipe_cannot_masquerade_as_stdout(self):
        value=row();value["children"][0]["writable_pipes"].append({"fd":2,"pipe":"pipe:[201]"})
        self.assertIsNone(m.writer_pressure_key(value,1))

    def test_stdout_fd_and_parent_ownership_are_required(self):
        for kind in ("missing_fd_one","missing_parent_pipe"):
            value=row()
            if kind=="missing_fd_one":value["children"][0]["writable_pipes"]=[]
            else:value["pipes"]=[]
            self.assertIsNone(m.writer_pressure_key(value,1))

    def test_incomplete_scan_and_process_identity_mismatch_are_refused(self):
        value=row();value["children"][0]["writer_scan_complete"]=False
        self.assertIsNone(m.writer_pressure_key(value,1))
        for complete,started in (("0","999"),("1","1001")):
            value=m.attach_thread_observations(row(),[["B","20",started,complete]])
            self.assertFalse(value["children"][0]["writer_scan_complete"])

    def test_duplicate_unmatched_and_invalid_records_are_refused(self):
        for records in ([["B","20","999","1"],["B","20","999","1"]],
                        [["D","99","1","pipe:[100]"]],[["W","20","21","1000","futex_do_wait","1"]],
                        [["D","20","1","/private/path"]],[["W","20","bad","1000","pipe_write","1"]]):
            with self.assertRaises(AssertionError):m.attach_thread_observations(row(),records)

    def test_existing_queue_capacity_and_mode_guards_remain(self):
        for key,value in (("pins",0),("anonymous_exports",0),("media_children",[]),("tx_queues",[0]),("named_temps",["temp"])):
            candidate=row();candidate[key]=value;self.assertIsNone(m.writer_pressure_key(candidate,1))
        candidate=row();candidate["children"][0]["mode"]="trim"
        self.assertIsNone(m.writer_pressure_key(candidate,1))
        self.assertIsNone(m.writer_pressure_key(row(),2))

    def test_real_ffmpeg_thread_probe_with_unread_stdout(self):
        binary=shutil.which("ffmpeg");self.assertIsNotNone(binary,"required native FFmpeg missing")
        with tempfile.TemporaryDirectory() as temp:
            source=Path(temp)/"source.mp4"
            subprocess.run([binary,"-nostdin","-v","error","-f","lavfi","-i","testsrc2=size=256x144:rate=10",
                            "-filter_threads","1","-vf","noise=alls=100:allf=t+u","-t","2","-c:v","libx264",
                            "-threads","1","-preset","ultrafast","-crf","0","-g","10","-pix_fmt","yuv420p",
                            "-an","-y",str(source)],check=True,capture_output=True,timeout=5)
            with open(os.devnull,"wb") as stderr:
                process=subprocess.Popen([binary,"-nostdin","-i",str(source),"-c","copy","-f","mp4",
                                          "-movflags","frag_keyframe+empty_moov","pipe:1"],stdout=subprocess.PIPE,stderr=stderr)
                try:
                    deadline=time.monotonic()+5;stable=None;since=None;samples=0
                    while time.monotonic()<deadline:
                        proc=Path("/proc")/str(process.pid)
                        observed=subprocess.run(["sh","-c",m.THREAD_PROBE+'\nobserve_export_writer "$1"',"probe",str(proc)],
                                                check=True,capture_output=True,text=True,timeout=2)
                        identity=int((proc/"stat").read_text().split()[21]);pipe=os.readlink(proc/"fd/1")
                        self.assertEqual(pipe,os.readlink("/proc/self/fd/"+str(process.stdout.fileno())))
                        value={"pins":1,"anonymous_exports":1,"media_children":[{"pid":process.pid}],
                               "tx_queues":[1009328],"named_temps":[],"pipes":[pipe],
                               "children":[{"pid":process.pid,"start":identity,"pipe":pipe,"mode":"copy"}]}
                        m.attach_thread_observations(value,[line.split("\t") for line in observed.stdout.splitlines()])
                        key=m.writer_pressure_key(value,1);now=time.monotonic()
                        if key is None or key!=stable:stable,since,samples=key,now,1 if key else 0
                        else:samples+=1
                        if key and samples>=3 and now-since>=.5:break
                        time.sleep(.1)
                    else:self.fail("real stdout writer thread not witnessed")
                finally:
                    process.kill();process.wait(timeout=3);process.stdout.close()


if __name__=="__main__":
    unittest.main()
