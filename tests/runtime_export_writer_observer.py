"""Test-only FFmpeg task observation; no production process-control changes."""
PIPE_WAITS = {"pipe_write", "anon_pipe_write"}

THREAD_PROBE = r"""
observe_export_writer() {
  p=$1
  pid=${p##*/}
  complete=1
  fd_count=0
  for f in "$p"/fd/*; do
    fd_count=$((fd_count + 1))
    test "$fd_count" -le 64 || return 1
    flags=$(awk '$1=="flags:" {print $2}' "$p/fdinfo/${f##*/}") || { complete=0; continue; }
    case "$flags" in ''|*[!0-7]*) complete=0; continue;; esac
    access=$((flags & 3))
    test "$access" -eq 1 || test "$access" -eq 2 || continue
    test -p "$f" || continue
    target=$(readlink "$f") || { complete=0; continue; }
    case "$target" in
      pipe:\[*\]) printf 'D\t%s\t%s\t%s\n' "$pid" "${f##*/}" "$target";;
      *) complete=0;;
    esac
  done
  task_count=0
  for task in "$p"/task/[0-9]*; do
    task_count=$((task_count + 1))
    test "$task_count" -le 32 || return 1
    wait=$(cat "$task/wchan") || { complete=0; continue; }
    case "$wait" in pipe_write|anon_pipe_write) ;; *) continue;; esac
    written=$(awk '$1=="wchar:" {print $2}' "$task/io") || { complete=0; continue; }
    started=$(awk '{print $22}' "$task/stat") || { complete=0; continue; }
    printf 'W\t%s\t%s\t%s\t%s\t%s\n' "$pid" "${task##*/}" "$started" "$wait" "$written"
  done
  started=$(awk '{print $22}' "$p/stat") || return 1
  printf 'B\t%s\t%s\t%s\n' "$pid" "$started" "$complete"
}
"""


def attach_thread_observations(row, records):
    children = {child["pid"]: child for child in row["children"]}
    if len(children) != len(row["children"]):
        raise AssertionError("duplicate source-matched child")
    seen = set()
    for child in children.values():
        child.update(writable_pipes=[], pipe_writers=[], writer_scan_complete=False)
    for parts in records:
        if len(parts) < 2 or not parts[1].isdigit() or int(parts[1]) not in children:
            raise AssertionError("unmatched writer observation")
        child = children[int(parts[1])]
        tag = parts[0]
        if tag == "D" and len(parts) == 4:
            if not parts[2].isdigit() or not parts[3].startswith("pipe:[") or not parts[3].endswith("]") or not parts[3][6:-1].isdigit():
                raise AssertionError("invalid writable pipe observation")
            key = (tag, child["pid"], int(parts[2]))
            child["writable_pipes"].append({"fd": int(parts[2]), "pipe": parts[3]})
        elif tag == "W" and len(parts) == 6:
            if not all(parts[index].isdigit() for index in (2, 3, 5)) or parts[4] not in PIPE_WAITS:
                raise AssertionError("invalid writer task observation")
            key = (tag, child["pid"], int(parts[2]))
            child["pipe_writers"].append({"tid": int(parts[2]), "start": int(parts[3]), "wait": parts[4], "written": int(parts[5])})
        elif tag == "B" and len(parts) == 4:
            if not parts[2].isdigit() or parts[3] not in ("0", "1"):
                raise AssertionError("invalid writer scan observation")
            key = (tag, child["pid"])
            child["writer_scan_complete"] = parts[3] == "1" and int(parts[2]) == child["start"]
        else:
            raise AssertionError("malformed writer observation")
        if key in seen:
            raise AssertionError("duplicate writer observation")
        seen.add(key)
    return row


def writer_pressure_key(row, count):
    children = row["children"]
    if (row["pins"] != count or row["anonymous_exports"] != count or
            len(children) != count or len(row["media_children"]) != count or len(row["tx_queues"]) != count or
            not all(queue > 0 for queue in row["tx_queues"]) or row["named_temps"]):
        return None
    keys = []
    for child in children:
        writers = child.get("pipe_writers", [])
        if (child["mode"] != "copy" or child["pipe"] not in row["pipes"] or
                not child.get("writer_scan_complete") or not writers or
                not any(item["fd"] == 1 for item in child.get("writable_pipes", [])) or
                any(item["pipe"] != child["pipe"] for item in child.get("writable_pipes", [])) or
                any(task["wait"] not in PIPE_WAITS for task in writers)):
            return None
        keys.append((child["pid"], child["start"], tuple(sorted(
            (task["tid"], task["start"], task["written"]) for task in writers))))
    return tuple(sorted(keys)), tuple(row["tx_queues"])
