"""Focused-CI-only read-only capture before exec of the real media tool."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import sys

tool, *args = sys.argv[1:]
if tool not in ('ffmpeg', 'ffprobe'):
    raise SystemExit('unsupported media tool')
real = shutil.which(tool, path=os.environ['SIMPLENVR_CAPTURE_PATH'])
if real is None:
    raise SystemExit('missing real media tool: ' + tool)
root = Path(os.environ['SIMPLENVR_CAPTURE_DIR'])
root.mkdir(parents=True, exist_ok=True)
record = {'tool': tool, 'argv': args, 'inputs': []}
for i, arg in enumerate(args[:-1]):
    if arg != '-i':
        continue
    path = args[i+1]
    # /proc descriptors belong to this process; pread preserves their offsets.
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
    except OSError:
        continue
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 16 << 20:
            continue
        data = os.pread(fd, info.st_size, 0)
        if len(data) != info.st_size:
            raise RuntimeError('short diagnostic input read')
        kind = 'manifest' if data.startswith(b'ffconcat version 1.0') else 'media'
        if kind == 'media' and b'ftyp' not in data[:64]:
            continue
        suffix = '.ffconcat' if kind == 'manifest' else '.mp4'
        name = kind + '-' + hashlib.sha256(data).hexdigest() + suffix
        (root / name).write_bytes(data)
        record['inputs'].append({'argument': path, 'artifact': name})
    finally:
        os.close(fd)
# ffprobe also accepts the test output filename as a positional argument.
if tool == 'ffprobe' and args and not args[-1].startswith('-'):
    path = args[-1]
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK)
    except OSError:
        fd = None
    if fd is not None:
        try:
            info = os.fstat(fd)
            if stat.S_ISREG(info.st_mode) and 0 < info.st_size <= 16 << 20:
                data = os.pread(fd, info.st_size, 0)
                if len(data) != info.st_size:
                    raise RuntimeError('short diagnostic output read')
                if b'ftyp' in data[:64]:
                    name = 'media-' + hashlib.sha256(data).hexdigest() + '.mp4'
                    (root / name).write_bytes(data)
                    record['inputs'].append({'argument': path, 'artifact': name})
        finally:
            os.close(fd)
(root / ('command-' + str(os.getpid()) + '.json')).write_text(json.dumps(record, indent=2)+'\n')
# Same PID, descriptors, stdout and stderr: retain FFmpeg signal/cancel semantics.
os.execv(real, [real, *args])
