"""Bounded, read-only and allowlisted runtime-export CI diagnostics."""
import argparse
import json
import os
from pathlib import Path
import stat
MAX_REPORT_BYTES = 256 << 10
CASE_NAMES = ("whole-copy-slow-reader", "disconnect-and-both-admissions-reuse", "sigterm-stream-backpressure-restart", "sigterm-preheader-trim-restart")
STATUSES = {"not-started", "running", "passed", "failed"}
ERROR_TYPES = {"AssertionError", "TimeoutExpired", "OSError", "ValueError", "JSONDecodeError", "FileNotFoundError", "PermissionError", "TimeoutError", "RemoteDisconnected", "IncompleteRead", "SSLError", "SSLEOFError", "ConnectionResetError", "ConnectionRefusedError", "KeyError", "TypeError", "KeyboardInterrupt"}
WAIT_NAMES = {"pipe_write", "anon_pipe_write", "0", "hrtimer_nanosleep", "wait_woken", "futex_wait_queue", "futex_wait_queue_me", "futex_do_wait", "ep_poll", "do_wait"}
KNOWN_REASONS = set(['active source-matched rendering with output and CPU progress not reached', 'canceled preheader trim started a video response', 'client close failed', 'duplicate source-matched child', 'duplicate writer observation', 'exact candidate SHA required', 'fixture command failed: docker', 'fixture command failed: ffmpeg', 'fixture command failed: ffprobe', 'fixture command failed: sudo', 'fixture command output cap', 'fixture name required', 'fresh private persisted configuration not observed', 'invalid TCP counters', 'invalid child counters', 'invalid child identity', 'invalid descriptor observation', 'invalid media child', 'invalid session cookie', 'invalid temp observation', 'invalid writable pipe observation', 'invalid writer scan observation', 'invalid writer task observation', 'login response cap', 'malformed proc observation', 'malformed writer observation', 'named export descriptor', 'named trim descriptor', 'new successful persistence/shutdown marker missing', 'non-loopback fixture endpoint', 'not an owned export fixture', 'owned export descriptors/children/temp entries did not clear', 'preheader client did not join', 'private fixture state required', 'proc observation cap', 'runtime image identity changed', 'runtime login page unavailable', 'runtime restart HTTPS readiness timed out', 'runtime session login failed', 'runtime shutdown was not a clean exit', 'shutdown log observation failed', 'slow-copy duration mismatch', 'slow-reader control fixture too small', 'source fixture outside reviewed size bounds', 'source or retained synthetic evidence changed', 'sustained source-matched pipe-write and owned TCP queue backpressure not reached', 'trim headers arrived before SIGTERM', 'trim responded before active render/shutdown witness', 'unexpected descriptor target', 'unexpected runtime session cookies', 'unmatched writer observation', 'whole-copy body cap', 'whole-copy video response required'])


def counter(value):
    return value if type(value) is int and 0 <= value <= (1 << 63) - 1 else None


def reason(value):
    return value if isinstance(value,str) and value in KNOWN_REASONS else None


def failure_fields(row):
    result={}
    if "failure_type" in row:
        kind=row["failure_type"]
        result["failure_type"]=kind if isinstance(kind,str) and kind in ERROR_TYPES else "other"
    if "failure" in row:
        result["failure_reason"]=reason(row["failure"]) or "unrecognized assertion; inspect retained report"
    if "cleanup_failure_type" in row:
        kind=row["cleanup_failure_type"]
        result["cleanup_failure_type"]=kind if isinstance(kind,str) and kind in ERROR_TYPES else "other"
        result["cleanup_failure_reason"]=reason(row.get("cleanup_failure")) or "inspect retained report"
    return result


def observation(row):
    if not isinstance(row,dict):return None
    result={key:counter(row.get(key)) for key in ("pins","anonymous_exports")}
    for key in ("anonymous_trims","tx_queues"):
        values=row.get(key)
        result[key]=[counter(value) for value in values[:4]] if isinstance(values,list) else None
    for key in ("media_children","named_temps"):
        values=row.get(key);result[key+"_count"]=len(values) if isinstance(values,list) else None
    result["children"]=[]
    children=row.get("children")
    for child in children[:4] if isinstance(children,list) else []:
        if not isinstance(child,dict):continue
        item={key:counter(child.get(key)) for key in ("pid","start","written","cpu")}
        wait=child.get("wait");mode=child.get("mode")
        item["wait"]=wait if isinstance(wait,str) and wait in WAIT_NAMES else "other_or_unavailable"
        item["mode"]=mode if isinstance(mode,str) and mode in ("copy","trim") else "other"
        item["writer_scan_complete"]=child.get("writer_scan_complete") is True
        writers=child.get("pipe_writers");item["pipe_writers"]=[]
        for writer in writers[:4] if isinstance(writers,list) else []:
            if not isinstance(writer,dict):continue
            task={key:counter(writer.get(key)) for key in ("tid","start","written")}
            wait=writer.get("wait")
            task["wait"]=wait if isinstance(wait,str) and wait in ("pipe_write","anon_pipe_write") else "other_or_unavailable"
            item["pipe_writers"].append(task)
        result["children"].append(item)
    return result


def summarize(report):
    result={"diagnostics_version":1,"diagnostic_only":True,"acceptance_inferred":False}
    if not isinstance(report,dict):return result|{"report_status":"invalid"}
    status=report.get("status")
    result["report_status"]=status if isinstance(status,str) and status in STATUSES else "invalid"
    result.update(failure_fields(report));result["cases"]={}
    cases=report.get("cases");cases=cases if isinstance(cases,dict) else {}
    for name in CASE_NAMES:
        raw=cases.get(name);raw=raw if isinstance(raw,dict) else {};status=raw.get("status")
        row={"status":status if isinstance(status,str) and status in STATUSES else "unavailable"}
        row.update(failure_fields(raw))
        for key in ("pressure","both_slots_reused","restart_both_slots"):
            witness=raw.get(key)
            if isinstance(witness,dict):row[key]={"stable_samples":counter(witness.get("stable_samples")),"state":observation(witness.get("state"))}
        if isinstance(raw.get("render_before_headers"),dict):row["render_before_headers"]=observation(raw["render_before_headers"])
        result["cases"][name]=row
    records=report.get("observations");records=records if isinstance(records,list) else []
    result["recorded_observation_count"]=len(records)
    result["observations_at_or_above_recording_limit"]=len(records)>=100
    result["last_recorded_observation"]=observation(records[-1]) if records else None
    return result


def load_summary(path):
    fd=None
    try:
        fd=os.open(path,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NONBLOCK);info=os.fstat(fd)
        if not stat.S_ISREG(info.st_mode):return summarize(None)|{"report_status":"not_regular"}
        if info.st_size>MAX_REPORT_BYTES:return summarize(None)|{"report_status":"too_large"}
        with os.fdopen(fd,"rb") as source:
            fd=None;raw=source.read(MAX_REPORT_BYTES+1)
        if len(raw)>MAX_REPORT_BYTES:return summarize(None)|{"report_status":"too_large"}
        return summarize(json.loads(raw.decode("utf-8","strict")))
    except FileNotFoundError:return summarize(None)|{"report_status":"missing"}
    except (ValueError,UnicodeError,RecursionError):return summarize(None)|{"report_status":"invalid"}
    except OSError:return summarize(None)|{"report_status":"unavailable"}
    finally:
        if fd is not None:os.close(fd)


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument("report",type=Path);args=parser.parse_args()
    print(json.dumps(load_summary(args.report),sort_keys=True,allow_nan=False))


if __name__=="__main__":main()
