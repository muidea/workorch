#!/usr/bin/env python3
"""固定stdio relay：不生成LSP响应，仅转发显式private FIFO原bytes。

此helper由SUT按授权language command启动。父观察器独立核对/proc的
PPID/cmdline，不能信任本helper自报pid。没有网络、环境读取或子进程启动。
"""
from __future__ import annotations

import os
from pathlib import Path
import sys
import threading


def main() -> int:
    if len(sys.argv) != 4:
        return 2
    request, response, handshake = map(Path, sys.argv[1:])
    # 根由调用者新建为0700；FIFO不经过resolve隐去symlink。
    for path in (request, response, handshake):
        if not path.is_absolute() or ".." in path.parts or path.is_symlink():
            return 2
    import json
    control_fd = os.open(handshake, os.O_WRONLY | os.O_NOFOLLOW)
    def control(event: str) -> None:
        os.write(control_fd, (json.dumps({"event": event, "pid": os.getpid()}) + "\n").encode())
    control("startup")
    with request.open("wb", buffering=0) as outgoing, response.open("rb", buffering=0) as incoming:
        def upstream() -> None:
            try:
                while True:
                    raw = os.read(sys.stdin.fileno(), 65536)
                    if not raw:
                        control("stdin_eof")
                        outgoing.close()
                        return
                    view = memoryview(raw)
                    while view:
                        count = outgoing.write(view)
                        if not count:
                            return
                        view = view[count:]
            except (BrokenPipeError, OSError):
                control("upstream_error")
        thread = threading.Thread(target=upstream, daemon=True)
        thread.start()
        try:
            while True:
                raw = incoming.read(65536)
                if not raw:
                    control("backend_eof")
                    return 0
                view = memoryview(raw)
                while view:
                    count = os.write(sys.stdout.fileno(), view)
                    view = view[count:]
        finally:
            control("exit")
            os.close(control_fd)


if __name__ == "__main__":
    raise SystemExit(main())
