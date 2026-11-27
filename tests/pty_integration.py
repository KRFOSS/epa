#!/usr/bin/env python3
"""Exercise the real EPA PTY relay using harmless fixture processes."""
import errno
import fcntl
import os
import pty
import select
import signal
import struct
import sys
import termios
import time

def run(binary, scenario, expected, send_when=None, reply=None, resize=False):
    pid, fd = pty.fork()
    if pid == 0:
        os.environ["EPA_PTY_TEST"] = scenario
        os.environ["TERM"] = "xterm"
        os.environ["NO_COLOR"] = "1"
        os.execl(binary, binary, "-test.run=^TestPTYDriver$")
    output = bytearray()
    sent = False
    deadline = time.monotonic() + 12
    status = None
    try:
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 80, 0, 0))
        while time.monotonic() < deadline:
            if select.select([fd], [], [], 0.1)[0]:
                try:
                    chunk = os.read(fd, 8192)
                    if not chunk:
                        break
                    output.extend(chunk)
                except OSError as exc:
                    if exc.errno == errno.EIO:
                        break
                    raise
            if not sent and send_when and send_when in output:
                if resize:
                    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 20, 32, 0, 0))
                    os.kill(pid, signal.SIGWINCH)
                if reply:
                    os.write(fd, reply)
                sent = True
        while time.monotonic() < deadline:
            got, s = os.waitpid(pid, os.WNOHANG)
            if got:
                status = s
                break
            time.sleep(0.05)
        assert status is not None, (scenario, "timeout", bytes(output))
        code = os.waitstatus_to_exitcode(status)
        assert code == expected, (scenario, code, bytes(output))
        if send_when:
            assert sent, (scenario, "prompt/progress not visible", bytes(output))
        return bytes(output)
    finally:
        if status is None:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)
        os.close(fd)

binary = os.path.abspath(sys.argv[1])
data = run(binary, "prompt", 100, b"Replace configuration? [Y/n] ", b"n\n")
assert b"42%" in data and b"Answer: n" in data and b"signature verification failed" in data and b"exit 100" in data, data
data = run(binary, "success", 0)
assert b"42%" in data and b"90%" in data and b"Completed" in data, data
data = run(binary, "interrupt", 130, b"42%", b"\x03")
assert b"exit 130" in data, data
data = run(binary, "full-screen", 0, b"Choose an option: ", b"yes\n")
assert b"Answer: yes" in data and b"\x1b[?1049h" in data and b"\x1b[?1049l" in data, data
data = run(binary, "resize", 0, b"42%", resize=True)
assert b"20 32" in data, data
print("PTY integration: prompt, exit status, completion, Ctrl-C, full-screen passthrough, resize passed")
