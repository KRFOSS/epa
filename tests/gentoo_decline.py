#!/usr/bin/env python3
"""Reject an actual native merge prompt in a disposable Gentoo container."""
import errno
import os
import pty
import select
import subprocess
import sys
import time

master, slave = pty.openpty()
proc = subprocess.Popen(sys.argv[1:], stdin=slave, stdout=slave, stderr=slave,
                        close_fds=True, env=dict(os.environ, TERM="dumb"))
os.close(slave)
seen = bytearray()
sent = False
deadline = time.monotonic() + 180
try:
    while time.monotonic() < deadline:
        if not select.select([master], [], [], 0.2)[0]:
            if proc.poll() is not None:
                break
            continue
        try:
            data = os.read(master, 65536)
        except OSError as exc:
            if exc.errno == errno.EIO:
                break
            raise
        if not data:
            break
        seen.extend(data)
        if not sent and b"Would you like to merge these packages?" in seen:
            os.write(master, b"n\n")
            sent = True
    if proc.poll() is None:
        proc.kill()
        raise RuntimeError("native confirmation timed out")
    if not sent:
        raise RuntimeError("native confirmation was not observed")
finally:
    os.close(master)
    if proc.poll() is None:
        proc.kill()
    proc.wait()
    sys.stdout.buffer.write(seen)
