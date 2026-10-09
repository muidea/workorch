#!/usr/bin/env python3
"""LSP-Bridge PRD 的主动黑盒 suite；合成校准不是正式验收。

Profile 只把参数名和 JSON pointer 映射到部署协议，不能提供 pass/facts。
授权的 MCP transport、独立 backend 边界观察器、进程执行器由调用者注入。
此模块不发现服务、不读取环境、不安装依赖、不生成 owner receipt。
"""
from __future__ import annotations

from dataclasses import dataclass, field, asdict
import base64
import copy
import hashlib
import math
import os
from pathlib import Path
import re
import select
import stat
import subprocess
import tempfile
import threading
import time
from typing import Any, Protocol
from urllib.parse import unquote, urlparse

import lsp_bridge_blackbox_producer as bb

LANGUAGES = {"Go": ("go", "main.go", "package main\nfunc marker() int { return 7 }\nfunc main() { _ = marker() }\n", 1, 5, 2, 18),
             "Rust": ("rust", "main.rs", "fn marker() -> i32 { 7 }\nfn main() { let _ = marker(); }\n", 0, 3, 1, 20),
             "Python": ("python", "main.py", "def marker():\n    return 7\nvalue = marker()\n", 0, 4, 2, 9),
             "TypeScript": ("typescript", "main.ts", "function marker(): number { return 7; }\nconst value = marker();\n", 0, 9, 1, 14),
             "Shell": ("shellscript", "main.sh", "marker() { printf '7'; }\nmarker\n", 0, 0, 1, 1)}
COMMANDS = {"build": ("go", "build", "-buildvcs=false", "./..."),
            "test": ("go", "test", "./..."), "race": ("go", "test", "-race", "./...")}
DOC_SECTIONS = {"integration": ("MCP", "lsp_initialize"), "development": ("go build", "go test"),
                "performance": ("50", "64", "TTL"), "dod": ("DoD",), "systemPrompt": ("lsp_definition", "lsp_sync")}


class Unverified(bb.ProducerError):
    """缺明确授权／观测边界，不把未知误判成失败或通过。"""


def require(condition: bool, message: str) -> None:
    if not condition:
        raise bb.ProducerError(message)


def number(value: Any, minimum: float = 0) -> float:
    require(type(value) in (int, float) and math.isfinite(value) and value >= minimum, "数值类型／边界错误")
    return float(value)


def integer(value: Any, minimum: int = 0) -> int:
    require(type(value) is int and value >= minimum, "整数类型／边界错误")
    return value


def digest(raw: bytes) -> str:
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def pointer(value: Any, path: str) -> Any:
    require(type(path) is str and (path == "" or path.startswith("/")), "Profile selector 必须是 JSON pointer")
    for part in path.split("/")[1:] if path else []:
        key = part.replace("~1", "/").replace("~0", "~")
        try:
            value = value[int(key)] if isinstance(value, list) else value[key]
        except (KeyError, IndexError, TypeError, ValueError) as exc:
            raise bb.ProducerError(f"响应字段缺失: {path}") from exc
    return value


@dataclass(frozen=True)
class Profile:
    """显式 wire 合同：参数重命名与字段选择，无执行回调和期望通过值。

    字段语义固定在 suite。位置统一为 0-based；其它部署用 locationBase 声明转换。
    不能将聚合 passed/facts 作为字段，也不能为缺失值提供默认值。
    """
    name: str
    arguments: dict[str, dict[str, str]]
    fields: dict[str, str]
    location_base: int = 0

    def __post_init__(self) -> None:
        require(type(self.name) is str and bool(self.name) and type(self.location_base) is int and self.location_base in (0, 1), "Profile identity/base 无效")
        for tool, mapping in self.arguments.items():
            require(tool in bb.TOOL_NAMES and isinstance(mapping, dict), "Profile tool 无效")
            require(all(type(k) is str and type(v) is str and v for k, v in mapping.items()), "参数映射无效")
            require(len(set(mapping.values())) == len(mapping), "参数映射冲突")
        for key, path in self.fields.items():
            require(type(key) is str and type(path) is str and (not path or path.startswith("/")), "字段映射无效")

    def args(self, tool: str, values: dict[str, Any]) -> dict[str, Any]:
        if tool not in self.arguments or not values.keys() <= self.arguments[tool].keys():
            raise Unverified(f"Profile 没有闭合 {tool} 参数: {sorted(values)}")
        return {self.arguments[tool][k]: v for k, v in values.items()}

    def get(self, payload: Any, field: str) -> Any:
        if field not in self.fields:
            raise Unverified(f"Profile 未声明 {field} 响应字段")
        return pointer(payload, self.fields[field])


@dataclass(frozen=True)
class ResourceGrant:
    """资源／稳定性／性能前置授权；不存在时对应探针不可执行。"""
    purposes: frozenset[str]
    max_fixture_bytes: int
    max_wait_seconds: float
    language_commands: dict[str, bb.CommandGrant]

    def validate(self, root: Path) -> None:
        integer(self.max_fixture_bytes, 1)
        require(number(self.max_wait_seconds) > 0, "等待授权必须有限且正数")
        for lang, grant in self.language_commands.items():
            require(lang in LANGUAGES, "未知语言授权")
            grant.validate(root)
            require(Path(grant.argv[0]).is_absolute(), "LSP executable 必须明确绝对路径，不能借 PATH")
            require(bool(grant.env) or "empty-environment" in self.purposes, "空环境必须专门授权，不能作为隐式 LSP 默认")


@dataclass(frozen=True)
class BackendEvent:
    """独立边界观察器记录的原 backend JSON-RPC message（不是 SUT 自报统计）。"""
    instance: str
    direction: str
    monotonic_ns: int
    raw: bytes
    framing: bytes = b""
    source: str = "unknown"
    generation: int = 0


@dataclass(frozen=True)
class RssSample:
    pid: int
    proc_statm: bytes
    page_size: int
    monotonic_ns: int


class BoundaryObserver(Protocol):
    """调用者持有观察管道和已授权 child handle；Profile 不可实现此角色。

    events 必须来自独立捕获的 backend request/response bytes。kill_owned 必须只
    终止该观察器 exact CommandGrant 创建且仍持有 Popen handle 的子进程。
    """
    def events(self) -> list[BackendEvent]: ...
    def kill_owned(self, instance: str) -> bb.ProcessObservation: ...
    def sample_server_rss(self) -> RssSample: ...
    def alive_backends(self, mcp: subprocess.Popen, languages: tuple[str, ...]) -> list[dict[str, Any]]: ...


@dataclass(frozen=True)
class RssGrant:
    """只授权已启动 MCP handle 的 Linux statm，不接受任意 PID。"""
    process: subprocess.Popen
    statm_path: str
    page_size: int


class GrantedBoundaryRecorder:
    """用于独立 tap 的可调用 recorder；不从 MCP response 猜 backend 信息。

    capture 在 observer 控制的实际转发点调用；同时保留 framing 原文和完整 JSON
    body。注册 owned child 只能接受本对象 start_owned 创建的句柄，不接受 PID。
    真实观测的信任/签名仍由外层 managed owner 收口。
    """
    def __init__(self, root: Path, resources: ResourceGrant, rss: RssGrant | None = None):
        resources.validate(root)
        self.root, self.resources, self.rss = root, resources, rss
        self._events: list[BackendEvent] = []
        self.frames: list[bytes] = []
        self._children: dict[str, tuple[subprocess.Popen, bb.CommandGrant]] = {}
        self._owned_history: list[subprocess.Popen] = []
        self._lineages: dict[str, tuple[subprocess.Popen, tuple[int, int]]] = {}
        self._stderr_bytes: dict[str, bytearray] = {}
        self._stderr_threads: list[threading.Thread] = []
        self._lock = threading.Lock()

    def capture(self, instance: str, direction: str, raw: bytes, framing: bytes) -> None:
        require(direction in ("request", "response") and type(raw) is bytes and type(framing) is bytes,
                "tap 原文必须完整 bytes")
        obj = bb._strict_json(raw)
        require(isinstance(obj, dict) and obj.get("jsonrpc") == "2.0", "tap body 非 JSON-RPC")
        with self._lock:
            self._events.append(BackendEvent(instance, direction, time.monotonic_ns(), raw, framing))
            self.frames.append(framing)

    def events(self) -> list[BackendEvent]:
        with self._lock:
            return list(self._events)

    def start_owned(self, instance: str, language: str) -> subprocess.Popen:
        if "owned-child-start" not in self.resources.purposes:
            raise Unverified("缺 owned-child-start 明确授权")
        grant = self.resources.language_commands.get(language)
        if grant is None:
            raise Unverified("owned language 没有明确 CommandGrant")
        grant.validate(self.root)
        require(instance not in self._children or self._children[instance][0].poll() is not None, "重复 owned handle")
        process = subprocess.Popen(grant.argv, cwd=grant.cwd, env=dict(grant.env), shell=False,
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self._children[instance] = (process, grant)
        self._owned_history.append(process)
        # stderr独立drain防阻塞，bytes原样保全；只能读取本对象ownedbackend。
        stderr = process.stderr
        if stderr is None:
            raise bb.ProducerError("ownedbackend缺stderr pipe")
        bucket = self._stderr_bytes.setdefault(instance, bytearray())
        def drain() -> None:
            while True:
                raw = stderr.read(65536)
                if not raw:
                    return
                with self._lock:
                    bucket.extend(raw)
        reader = threading.Thread(target=drain, daemon=True)
        self._stderr_threads.append(reader)
        reader.start()
        return process

    def bind_forwarding(self, instance: str, mcp: subprocess.Popen, mcp_input_fd: int, mcp_output_fd: int) -> None:
        raise Unverified("capture recorder不接真实转发；任意fd绑定不能授予real资格")

    def owned_lineage(self, instance: str, mcp: subprocess.Popen) -> dict[str, Any]:
        raise Unverified("capture recorder不是实际forwarder，不能证明SUT backend lineage")

    def kill_owned(self, instance: str) -> bb.ProcessObservation:
        if "owned-child-kill" not in self.resources.purposes or instance not in self._children:
            raise Unverified("不是 observer exact grant 创建的 child，禁止 kill")
        child, grant = self._children[instance]
        require(child.poll() is None, "owned child 已退出，不能伪装手动 kill")
        start = time.monotonic_ns()
        child.kill()
        out, err = child.communicate(timeout=grant.timeout_seconds)
        return bb.ProcessObservation(grant.argv, grant.cwd, child.returncode, out, err,
                                     (time.monotonic_ns() - start) / 1e6)

    def sample_server_rss(self) -> RssSample:
        if "rss" not in self.resources.purposes or self.rss is None:
            raise Unverified("缺明确 RSS handle/statm grant")
        child = self.rss.process
        require(child.poll() is None and self.rss.statm_path == f"/proc/{child.pid}/statm", "RSS 不属于存活的授权 handle")
        require(os.name == "posix" and Path("/proc").is_dir(), "只支持已授权 Linux proc RSS")
        fd = os.open(self.rss.statm_path, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, "rb") as handle:
            raw = handle.read()
        return RssSample(child.pid, raw, self.rss.page_size, time.monotonic_ns())


class LSPFrames:
    """严格Content-Length framing；原bytes完整保留，不重编码冒raw。"""
    def __init__(self, maximum: int):
        self.maximum = integer(maximum, 1)
        self.pending = bytearray()

    def feed(self, chunk: bytes) -> list[tuple[bytes, bytes]]:
        require(type(chunk) is bytes, "framing chunk必须bytes")
        self.pending.extend(chunk)
        require(len(self.pending) <= self.maximum + 8192, "framing超明确byte边界")
        rows = []
        while b"\r\n\r\n" in self.pending:
            header_end = self.pending.index(b"\r\n\r\n") + 4
            require(header_end <= 8192, "header超界")
            headers = self.pending[:header_end-4].decode("ascii", errors="strict").split("\r\n")
            lengths = [line.split(":", 1)[1].strip() for line in headers if line.lower().startswith("content-length:")]
            require(len(lengths) == 1 and lengths[0].isdigit(), "Content-Length缺失/重复/无效")
            require(all(":" in line for line in headers), "header无效")
            size = int(lengths[0])
            require(0 < size <= self.maximum, "LSP body超授权边界")
            end = header_end + size
            if len(self.pending) < end:
                break
            frame = bytes(self.pending[:end])
            body = bytes(self.pending[header_end:end])
            obj = bb._strict_json(body)
            require(isinstance(obj, dict) and obj.get("jsonrpc") == "2.0", "frame body非JSON-RPC")
            rows.append((frame, body))
            del self.pending[:end]
        return rows

    def eof(self) -> None:
        require(not self.pending, "EOF剩余partial LSP framing")


def frame_body(frame: bytes, maximum: int) -> bytes:
    decoder = LSPFrames(maximum)
    rows = decoder.feed(frame)
    decoder.eof()
    require(len(rows) == 1, "event framing必须exact单message")
    return rows[0][1]


class OwnedLSPForwarder(GrantedBoundaryRecorder):
    """actual双向relay；自己创建FIFO/child，转发点生成time/raw evidence。

    SUT只授予固定helper language command。hello的PID不被信任：独立/proc
    parent+cmdline+fd inode必须对应指定SUT及本对象创建FIFO。capture/bind
    不能注入事件或授予real资格。仅Linux运行，缺前置权限NOT_VERIFIED。
    """
    def __init__(self, root: Path, resources: ResourceGrant, helper_grant: bb.CommandGrant,
                 helper_bytes: bytes, rss: RssGrant | None = None):
        super().__init__(root, resources, rss)
        helper_grant.validate(root)
        require("backend-forwarding" in resources.purposes and "proc-lineage" in resources.purposes,
                "actual forwarder缺明确资源/lineage授权")
        require(len(helper_grant.argv) == 2 and Path(helper_grant.argv[0]).is_absolute(), "固定helper interpreter/源码argv错误")
        helper = Path(helper_grant.argv[1])
        bb._reject_symlinks(helper)
        require(helper.read_bytes() == helper_bytes, "helper源码bytes漂移")
        self.helper_grant, self.helper_bytes = helper_grant, helper_bytes
        self.routes: dict[str, dict[str, Any]] = {}
        self.failures: list[str] = []
        self._stop = threading.Event()
        self._threads: list[threading.Thread] = []
        self._next_delay = 0.0
        self._hold_gate: threading.Event | None = None
        self.stream_chunks: list[tuple[str, str, bytes]] = []
        self.exit_observations: list[bb.ProcessObservation] = []

    def capture(self, *args: Any, **kwargs: Any) -> None:
        raise Unverified("actual forwarder不接受外部capture注入")

    def bind_forwarding(self, *args: Any, **kwargs: Any) -> None:
        raise Unverified("actual forwarder不接受任意fd绑定")

    def route(self, instance: str, language: str) -> bb.CommandGrant:
        require(instance not in self.routes, "duplicate route")
        directory = Path(tempfile.mkdtemp(prefix=".lsp-relay-", dir=self.root))
        paths = [directory / name for name in ("request", "response", "hello")]
        for path in paths:
            os.mkfifo(path, mode=0o600)
        # 仅读端提前打开；不保留RDWR写端，helper退出能产生真实EOF。
        fds = [os.open(paths[0], os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW), -1,
               os.open(paths[2], os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)]
        argv = self.helper_grant.argv + tuple(str(path) for path in paths)
        grant = bb.CommandGrant(argv, self.helper_grant.cwd, self.helper_grant.env, (argv,),
                                self.helper_grant.allowed_env_keys, self.helper_grant.timeout_seconds)
        self.routes[instance] = {"language": language, "paths": paths, "fds": fds, "grant": grant,
                                 "wrapperPid": None, "wrapperStart": None, "mcp": None, "hello": bytearray(), "generation": 0,
                                 "controls": [], "identityBytes": []}
        return grant

    def attach_mcp(self, instance: str, mcp: subprocess.Popen) -> None:
        require(instance in self.routes and isinstance(mcp, subprocess.Popen) and mcp.poll() is None, "MCP actual handle无效")
        route = self.routes[instance]
        require(route["mcp"] is None, "route不能重绑其它MCP")
        route["mcp"] = mcp
        thread = threading.Thread(target=self._manager, args=(instance,), daemon=True)
        self._threads.append(thread)
        thread.start()

    def _identity(self, pid: int, mcp: subprocess.Popen, route: dict[str, Any]) -> None:
        raw = Path(f"/proc/{pid}/stat").read_bytes()
        tail = raw[raw.rfind(b")") + 2:].split()
        require(int(tail[1]) == mcp.pid and tail[0] != b"Z", "helper真实PPID不属于授权MCP或已zombie")
        start = int(tail[19])
        require(route["wrapperStart"] is None or route["wrapperStart"] == start, "helper PID reused/starttime漂移")
        route["wrapperStart"] = start
        route["identityBytes"].append(raw)
        cmdline = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\x00")[:-1]
        require(cmdline == [arg.encode() for arg in route["grant"].argv], "helper实际cmdline不符固定授权")
        observed = []
        for entry in Path(f"/proc/{pid}/fd").iterdir():
            try:
                info = entry.stat()
                observed.append((info.st_dev, info.st_ino))
            except FileNotFoundError:
                continue
        for path in route["paths"][:2]:
            info = path.stat()
            require((info.st_dev, info.st_ino) in observed, "helper未打开本对象actual FIFO")

    def _write(self, fd: int, raw: bytes) -> None:
        view = memoryview(raw)
        deadline = time.monotonic() + self.resources.max_wait_seconds
        while view:
            require(not self._stop.is_set() and time.monotonic() < deadline, "relay write timeout/stop")
            if not select.select([], [fd], [], 0.05)[1]:
                continue
            try:
                count = os.write(fd, view)
            except BlockingIOError:
                continue
            view = view[count:]

    def _manager(self, instance: str) -> None:
        route = self.routes[instance]
        while not self._stop.is_set() and route["mcp"].poll() is None:
            # 只有actual helper startup control可启动下一代backend；idle等待不计故障。
            if not select.select([route["fds"][2]], [], [], 0.05)[0]:
                continue
            # 控制FIFO无writer时select会报告EOF；不把它当startup，也不busy spin。
            if not route["hello"]:
                try:
                    startup = os.read(route["fds"][2], 65536)
                except BlockingIOError:
                    startup = b""
                if not startup:
                    time.sleep(0.01)
                    continue
                route["hello"].extend(startup)
            route["wrapperPid"] = None
            route["wrapperStart"] = None
            self._serve(instance)
            if self.failures:
                return

    def _serve(self, instance: str) -> None:
        route = self.routes[instance]
        outgoing, incoming, hello = route["fds"]
        decoder = LSPFrames(self.resources.max_fixture_bytes)
        child = None
        try:
            deadline = time.monotonic() + self.resources.max_wait_seconds
            while not self._stop.is_set() and time.monotonic() < deadline:
                if b"\n" in route["hello"] or select.select([hello], [], [], 0.05)[0]:
                    if b"\n" not in route["hello"]:
                        route["hello"].extend(os.read(hello, 4096))
                    if b"\n" in route["hello"]:
                        line, _, remaining = bytes(route["hello"]).partition(b"\n")
                        control = bb._strict_json(line)
                        route["hello"] = bytearray(remaining)
                        # previous helper退出控制消息不可跨generation冒新startup。
                        if isinstance(control, dict) and control.get("event") in ("stdin_eof", "backend_eof", "exit", "upstream_error"):
                            route["controls"].append(line + b"\n")
                            if b"\n" not in route["hello"]:
                                return
                            continue
                        require(isinstance(control, dict) and control.get("event") == "startup" and type(control.get("pid")) is int,
                                "独立control startup协议无效")
                        route["controls"].append(line + b"\n")
                        pid = control["pid"]
                        # 对端helper已打开request writer，现打开response writer解除helper reader open。
                        while True:
                            try:
                                incoming = os.open(route["paths"][1], os.O_WRONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
                                route["fds"][1] = incoming
                                break
                            except OSError as exc:
                                require(time.monotonic() < deadline and exc.errno == 6, "response FIFO open失败/timeout")
                                time.sleep(0.002)
                        # helper先write control，随后打开数据FIFO；等独立actual fd观察。
                        while time.monotonic() < deadline:
                            try:
                                self._identity(pid, route["mcp"], route)
                                break
                            except bb.ProducerError:
                                time.sleep(0.002)
                        else:
                            raise Unverified("actual helper pipe/parent identity未建立")
                        route["wrapperPid"] = pid
                        break
            require(route["wrapperPid"] is not None, "helper handshake timeout")
            child = self.start_owned(instance, route["language"])
            route["generation"] += 1
            stdin, stdout = child.stdin, child.stdout
            if stdin is None or stdout is None:
                raise bb.ProducerError("owned backend缺actual stdin/stdout pipe")
            os.set_blocking(stdin.fileno(), False)
            os.set_blocking(stdout.fileno(), False)
            reply_decoder = LSPFrames(self.resources.max_fixture_bytes)
            stdin_closed = False
            wrapper_gone = False
            close_deadline = None
            while not self._stop.is_set():
                monitored = [outgoing, stdout.fileno(), hello] if not stdin_closed else [stdout.fileno(), hello]
                ready = select.select(monitored, [], [], 0.05)[0]
                # 独立进程观察处理SIGKILL等无control事件退出，不向未知PID发signal。
                try:
                    raw_stat = Path(f"/proc/{route['wrapperPid']}/stat").read_bytes()
                    tail = raw_stat[raw_stat.rfind(b")") + 2:].split()
                    wrapper_gone = tail[0] == b"Z" or int(tail[19]) != route["wrapperStart"]
                except FileNotFoundError:
                    wrapper_gone = True
                if (wrapper_gone or route["mcp"].poll() is not None) and not stdin_closed:
                    # 仍先读取request FIFO排队bytes，EOF后才关闭backend stdin。
                    ready = list(set(ready + [outgoing]))
                for fd in ready:
                    if fd == hello:
                        control_raw = os.read(hello, 65536)
                        if control_raw:
                            route["hello"].extend(control_raw)
                            while b"\n" in route["hello"]:
                                line, _, rest = bytes(route["hello"]).partition(b"\n")
                                route["hello"] = bytearray(rest)
                                control = bb._strict_json(line)
                                require(control.get("pid") == route["wrapperPid"] and control.get("event") in
                                        ("stdin_eof", "upstream_error", "backend_eof", "exit"), "独立control generation/PID漂移")
                                route["controls"].append(line + b"\n")
                        continue
                    try:
                        raw = os.read(fd, 65536)
                    except BlockingIOError:
                        continue
                    if not raw:
                        if fd == stdout.fileno():
                            reply_decoder.eof()
                            return
                        decoder.eof()
                        if not stdin_closed:
                            stdin.close()
                            stdin_closed = True
                            close_deadline = time.monotonic() + self.resources.max_wait_seconds
                        continue
                    direction = "request" if fd == outgoing else "response"
                    with self._lock:
                        self.stream_chunks.append((instance, direction, raw))
                    parsed = decoder.feed(raw) if direction == "request" else reply_decoder.feed(raw)
                    for framing, body in parsed:
                        obj = bb._strict_json(body)
                        # LSP backend notifications也是backend->SUT；typed direction表示来源，不伪response shape。
                        event_direction = "request" if "method" in obj else "response"
                        event = BackendEvent(instance, event_direction, time.monotonic_ns(), body, framing,
                                             "sut_to_backend" if direction == "request" else "backend_to_sut", route["generation"])
                        with self._lock:
                            self._events.append(event)
                            delay = self._next_delay if direction == "response" and "result" in obj else 0
                            gate = self._hold_gate if direction == "response" and "result" in obj else None
                            if delay:
                                self._next_delay = 0
                            if gate is not None:
                                self._hold_gate = None
                        if delay:
                            require(not self._stop.wait(delay), "delay实验中停止")
                        if gate is not None:
                            require(gate.wait(timeout=self.resources.max_wait_seconds), "hold超等待授权")
                        if not (wrapper_gone and direction == "response"):
                            self._write(stdin.fileno() if direction == "request" else incoming, framing)
                if close_deadline is not None and time.monotonic() >= close_deadline:
                    # helper已真实结束，仅收敛自有backend Popen，不将收敛计为SUT恢复。
                    child.kill()
                    child.wait(timeout=self.resources.max_wait_seconds)
                    return
        except Exception as exc:
            self.failures.append(str(exc))
        finally:
            if route["fds"][1] >= 0:
                os.close(route["fds"][1])
                route["fds"][1] = -1
            if child is not None and child.poll() is None:
                child.kill()
                child.wait(timeout=self.resources.max_wait_seconds)

    def delay_next_response(self, seconds: float) -> None:
        if "timeout" not in self.resources.purposes:
            raise Unverified("delay experiment缺timeout grant")
        require(0 < number(seconds) <= self.resources.max_wait_seconds, "delay超授权等待")
        with self._lock:
            require(self._next_delay == 0, "delay已安排")
            self._next_delay = seconds

    def hold_query(self, arguments: dict[str, Any], transport: bb.MCPTransport) -> "HeldToolQuery":
        if "lifecycle" not in self.resources.purposes:
            raise Unverified("hold experiment缺lifecycle grant")
        require(isinstance(transport, bb.StdioJSONRPCClient), "hold必须实际raw MCP transport")
        gate = threading.Event()
        with self._lock:
            require(self._hold_gate is None, "hold已安排")
            self._hold_gate = gate
        future = HeldToolQuery(transport, arguments, gate, self.resources.max_wait_seconds)
        future.start()
        # 必须独立观察实际backend request已在途，再开始TTL计时。
        deadline = time.monotonic() + self.resources.max_wait_seconds
        while time.monotonic() < deadline:
            if future.started.is_set():
                events = self.events()
                if any(e.monotonic_ns >= future.start_ns and bb._strict_json(e.raw).get("method") == "textDocument/definition" for e in events):
                    return future
            time.sleep(0.002)
        gate.set()
        raise Unverified("hold没有观察到actual backend请求")

    def kill_owned(self, instance: str) -> bb.ProcessObservation:
        if "owned-child-kill" not in self.resources.purposes or instance not in self._children:
            raise Unverified("不是actualrelay创建的owned child")
        child, grant = self._children[instance]
        require(child.poll() is None, "owned relay child已退出")
        start = time.monotonic_ns()
        child.kill()
        child.wait(timeout=grant.timeout_seconds)
        with self._lock:
            stdout = b"".join(raw for ident, direction, raw in self.stream_chunks if ident == instance and direction == "response")
            stderr = bytes(self._stderr_bytes.get(instance, b""))
        result = bb.ProcessObservation(grant.argv, grant.cwd, child.returncode, stdout, stderr,
                                       (time.monotonic_ns() - start) / 1e6)
        self.exit_observations.append(result)
        return result

    def owned_lineage(self, instance: str, mcp: subprocess.Popen) -> dict[str, Any]:
        require(Path(self.helper_grant.argv[1]).read_bytes() == self.helper_bytes, "forwarder helper发生mutation")
        require(not self.failures, "relay failure: " + repr(self.failures))
        route = self.routes.get(instance)
        if route is None or route["wrapperPid"] is None or instance not in self._children:
            raise Unverified("actual route未建立")
        require(route["mcp"] is mcp and mcp.poll() is None, "actual route的MCP handle漂移")
        self._identity(route["wrapperPid"], mcp, route)
        child, grant = self._children[instance]
        return {"childHandle": child, "mcpHandle": mcp, "grant": grant, "generation": route["generation"]}

    def alive_backends(self, mcp: subprocess.Popen, languages: tuple[str, ...]) -> list[dict[str, Any]]:
        rows = []
        for language in languages:
            matches = [(instance, r) for instance, r in self.routes.items() if r["language"] == language and r["mcp"] is mcp]
            require(len(matches) == 1, "RSS language没有唯一actual relay")
            instance, _ = matches[0]
            lineage = self.owned_lineage(instance, mcp)
            child = lineage["childHandle"]
            require(child.poll() is None, "backend dead，SUT status自报running不能通过")
            rows.append({"language": language, "instance": instance, "pid": child.pid, "generation": lineage["generation"]})
        return rows

    def close(self) -> None:
        if self._stop.is_set():
            return
        self._stop.set()
        for child, _ in self._children.values():
            if child.poll() is None:
                child.kill()
                child.wait(timeout=self.resources.max_wait_seconds)
        for thread in self._threads:
            thread.join(timeout=self.resources.max_wait_seconds)
        for thread in self._stderr_threads:
            thread.join(timeout=self.resources.max_wait_seconds)
        require(not any(thread.is_alive() for thread in self._threads + self._stderr_threads), "relay线程未收敛")
        for child in self._owned_history:
            for stream in (child.stdin, child.stdout, child.stderr):
                if stream is not None:
                    stream.close()
        for route in self.routes.values():
            for fd in route["fds"]:
                if fd >= 0:
                    os.close(fd)
            for path in route["paths"]:
                require(stat.S_ISFIFO(path.lstat().st_mode), "自有FIFO路径被替换，拒清理未知文件")
                path.unlink()
            route["paths"][0].parent.rmdir()


class HeldToolQuery:
    """实际raw MCP request future；release只放行relay，不返回pass布尔。"""
    def __init__(self, transport: bb.StdioJSONRPCClient, arguments: dict[str, Any], gate: threading.Event, timeout: float):
        self.transport, self.arguments, self.gate, self.timeout = transport, arguments, gate, timeout
        self.started = threading.Event()
        self.start_ns = 0
        self.payload: dict[str, Any] | None = None
        self.error: BaseException | None = None
        self.thread = threading.Thread(target=self._run, daemon=True)

    def start(self) -> None:
        self.thread.start()

    def _run(self) -> None:
        self.start_ns = time.monotonic_ns()
        self.started.set()
        try:
            envelope = self.transport.call_tool("lsp_definition", self.arguments)
            require(envelope.get("isError") is not True, "held MCP error")
            blocks = envelope.get("content")
            require(isinstance(blocks, list) and len(blocks) == 1 and blocks[0].get("type") == "text", "held response shape无效")
            self.payload = bb._strict_json(blocks[0]["text"].encode())
        except BaseException as exc:
            self.error = exc

    def release(self) -> None:
        self.gate.set()

    def result(self) -> dict[str, Any]:
        self.thread.join(timeout=self.timeout)
        require(not self.thread.is_alive(), "held query未完成")
        if self.error is not None:
            raise self.error
        require(isinstance(self.payload, dict), "held query缺actual payload")
        if self.payload is None:
            raise bb.ProducerError("held query无结果")
        return self.payload


@dataclass
class SuitePlan:
    """所有读取路径均由交付调用者显式列举，不扫描真实仓库或配置。"""
    resources: ResourceGrant | None = None
    command_grants: dict[str, bb.CommandGrant] = field(default_factory=dict)
    documents: dict[str, str] = field(default_factory=dict)
    source_files: tuple[str, ...] = ()
    test_files: tuple[str, ...] = ()
    config_example: str | None = None
    # 两个独立 clean snapshot 内运行相同 build 后，显式列举输出以比较 bytes。
    reproducible_outputs: tuple[tuple[str, str], ...] = ()
    build_outputs: tuple[str, ...] = ()
    snapshot_source_root: str = ""
    delivery_inventory: tuple[str, ...] = ()
    git_repository_root: str | None = None
    git_executable: str | None = None
    git_grants: tuple[bb.CommandGrant, ...] = ()
    delivery_commit_file: str | None = None
    config_values: dict[str, Any] = field(default_factory=lambda: {
        "defaultTimeout": 0.05, "maxReferences": 2, "maxDiagnostics": 2, "maxHoverCharacters": 16,
        "idleTTL": 0.1, "maxInstances": 2})
    # 由外部显式控制的独立部署实例，避免运行期修改未知用户服务的配置。
    configured_transport: bb.MCPTransport | None = None
    configured_boundary: BoundaryObserver | None = None
    configured_values: dict[str, Any] = field(default_factory=dict)
    configured_launch_grant: bb.CommandGrant | None = None
    configured_input_files: tuple[str, ...] = ()
    goroutine_grant: bb.CommandGrant | None = None
    real_go_test_grant: bb.CommandGrant | None = None
    real_go_executable_sha256: str | None = None
    concurrent_mcp_grant: bb.CommandGrant | None = None


class GrantedExecutor:
    """仅运行 exact grant；无 shell／环境继承／未授权子进程终止。"""
    def __init__(self, root: Path):
        self.root = root

    def run(self, grant: bb.CommandGrant) -> bb.ProcessObservation:
        grant.validate(self.root)
        require(Path(grant.argv[0]).is_absolute(), "command executable 必须明确绝对路径")
        start = time.monotonic_ns()
        child = subprocess.Popen(grant.argv, cwd=grant.cwd, env=dict(grant.env), shell=False,
                                 stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        timed_out = False
        try:
            out, err = child.communicate(timeout=grant.timeout_seconds)
        except subprocess.TimeoutExpired:
            # 只停止自己通过 exact grant 创建的 handle。
            timed_out = True
            child.kill()
            out, err = child.communicate()
        return bb.ProcessObservation(grant.argv, grant.cwd, child.returncode, out, err,
                                     (time.monotonic_ns() - start) / 1e6, timed_out)

    def run_with_input(self, grant: bb.CommandGrant, raw: bytes) -> bb.ProcessObservation:
        grant.validate(self.root)
        require(Path(grant.argv[0]).is_absolute(), "harness executable 必须绝对路径")
        start = time.monotonic_ns()
        child = subprocess.Popen(grant.argv, cwd=grant.cwd, env=dict(grant.env), shell=False,
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        timed_out = False
        try:
            out, err = child.communicate(raw, timeout=grant.timeout_seconds)
        except subprocess.TimeoutExpired:
            timed_out = True
            child.kill()
            out, err = child.communicate()
        return bb.ProcessObservation(grant.argv, grant.cwd, child.returncode, out, err,
                                     (time.monotonic_ns() - start) / 1e6, timed_out)


class PRDSuite:
    """可调用完整 suite；PASS 是已观察断言，不代表 owner 正式验收。

    decoded-only transport、synthetic execution、缺授权或测量边界不会获得真实 PASS。
    每项场景失败隔离；报告保留完整 raw artifacts 和具体未闭合原因。
    """
    def __init__(self, producer: bb.LSPBridgeProducer, profile: Profile, plan: SuitePlan,
                 boundary: BoundaryObserver | None = None, execution: str = "real"):
        require(execution in ("real", "synthetic"), "execution 必须显式 real/synthetic")
        self.producer, self.profile, self.plan, self.boundary = producer, profile, plan, boundary
        self.execution = execution
        self.rows: list[dict[str, Any]] = []
        self.sequence = 0
        self.fixtures: dict[str, dict[str, Any]] = {}
        self.observations: list[dict[str, Any]] = []
        self.command_results: dict[str, bb.ProcessObservation] = {}
        self._finished = False
        self._report: dict[str, Any] | None = None

    def resources(self) -> ResourceGrant:
        grant = self.plan.resources
        if grant is None:
            raise Unverified("缺 explicit ResourceGrant")
        return grant

    def authorize(self, purpose: str, languages: tuple[str, ...] = ()) -> None:
        grant = self.plan.resources
        if grant is None or purpose not in grant.purposes:
            raise Unverified(f"缺 explicit resource grant: {purpose}")
        grant.validate(self.producer.delivery)
        for lang in languages:
            if lang not in grant.language_commands:
                raise Unverified(f"{lang} backend 未安装／未授权；不能 PASS")

    def artifact(self, label: str, raw: bytes, kind: str = "command_output") -> str:
        self.sequence += 1
        self.producer._artifact(f"suite-{self.sequence}-{label}", raw)
        self.producer.artifacts[-1].kind = kind
        return self.producer.artifacts[-1].ref

    def file(self, relative: str) -> bytes:
        require(type(relative) is str and bool(relative) and not Path(relative).is_absolute() and ".." not in Path(relative).parts,
                "交付文件必须在明确 delivery 中")
        path = self.producer.delivery / relative
        bb._reject_symlinks(path)
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, "rb") as handle:
            before = os.fstat(handle.fileno())
            require(stat.S_ISREG(before.st_mode), "交付输入必须 regular file，不能 device/FIFO")
            raw = handle.read()
            after = os.fstat(handle.fileno())
        stable_before = (before.st_dev, before.st_ino, before.st_mode, before.st_size, before.st_mtime_ns, before.st_ctime_ns)
        stable_after = (after.st_dev, after.st_ino, after.st_mode, after.st_size, after.st_mtime_ns, after.st_ctime_ns)
        require(stable_before == stable_after and len(raw) == after.st_size, "读取期文件发生变化")
        self.artifact("delivery", raw, "documentation_index" if relative in self.plan.documents.values() else "source_snapshot")
        return raw

    def make_fixture(self, lang: str) -> dict[str, Any]:
        self.authorize("fixtures", (lang,))
        if lang in self.fixtures:
            return self.fixtures[lang]
        language, filename, source, dl, dc, ql, qc = LANGUAGES[lang]
        root = self.producer.delivery / f"suite-fixture-{lang}"
        root.mkdir(mode=0o700, exist_ok=False)
        path = root / filename
        raw = source.encode()
        require(len(raw) <= self.resources().max_fixture_bytes, "fixture 超出授权 bytes")
        path.write_bytes(raw)
        if lang == "Go":
            (root / "go.mod").write_text("module example.invalid/blackbox\n\ngo 1.22\n", encoding="utf-8")
        if lang == "Rust":
            (root / "Cargo.toml").write_text('[package]\nname="blackbox"\nversion="0.1.0"\nedition="2021"\n[[bin]]\nname="blackbox"\npath="main.rs"\n', encoding="utf-8")
        fixture = {"root": str(root), "path": str(path), "language": language, "line": ql,
                   "column": qc, "targetLine": dl, "targetColumn": dc, "source": source}
        self.fixtures[lang] = fixture
        self.artifact("fixture-" + lang, raw, "source_snapshot")
        return fixture

    def call(self, tool: str, values: dict[str, Any], transport: bb.MCPTransport | None = None) -> Any:
        self.sequence += 1
        arguments = self.profile.args(tool, values)
        start = time.monotonic_ns()
        if transport is None or transport is self.producer.transport:
            payload = self.producer.observe_tool_call(f"suite-call-{self.sequence}", tool, arguments)
        else:
            require(isinstance(transport, bb.StdioJSONRPCClient), "第二部署必须 raw stdio，不能 decoded-only")
            envelope = transport.call_tool(tool, arguments)
            require(envelope.get("isError") is not True, "配置部署 tool error: " + repr(envelope))
            content = envelope.get("content")
            require(isinstance(content, list) and len(content) == 1 and content[0].get("type") == "text", "配置部署 envelope 无效")
            payload = bb._strict_json(content[0]["text"].encode())
        end = time.monotonic_ns()
        self.observations.append({"tool": tool, "arguments": copy.deepcopy(arguments), "payload": copy.deepcopy(payload),
                                  "startNs": start, "endNs": end})
        # 注意这里只记录 wall clock，不把它冒充排除 LSP 的内部耗时。
        return payload

    def init(self, lang: str, transport: bb.MCPTransport | None = None) -> dict[str, Any]:
        f = self.make_fixture(lang)
        self.call("lsp_initialize", {"root": f["root"], "language": f["language"]}, transport)
        return f

    def query(self, tool: str, f: dict[str, Any], transport: bb.MCPTransport | None = None) -> Any:
        observer = self.plan.configured_boundary if transport is not None and transport is self.plan.configured_transport else self.boundary
        mark = len(self.backend(observer)) if self.execution == "real" else 0
        payload = self.call(tool, {k: f[k] for k in ("root", "language", "path", "line", "column")}, transport)
        if self.execution == "real":
            if not isinstance(observer, OwnedLSPForwarder):
                raise Unverified("真实语义query必须actual forwarder，不以MCP自报内容冒backend")
            current = self.backend(observer)
            uri = f["path"] if f["path"].startswith("file:") else Path(f["path"]).as_uri()
            methods = {"lsp_definition": "textDocument/definition", "lsp_hover": "textDocument/hover", "lsp_references": "textDocument/references"}
            if tool in methods:
                candidates = [(e, obj) for e, obj in current[mark:] if e.source == "sut_to_backend" and obj.get("method") == methods[tool]
                              and obj.get("params", {}).get("textDocument", {}).get("uri") == uri]
                require(len(candidates) == 1, "query无current URI对应的独立backend原请求")
                event, request = candidates[0]
                require(event.generation == observer.routes[event.instance]["generation"], "query旧generation原请求")
                responses = [(e, obj) for e, obj in current[mark:] if e.source == "backend_to_sut" and e.instance == event.instance
                             and e.generation == event.generation and obj.get("id") == request.get("id") and "result" in obj]
                require(len(responses) == 1, "query无同URI/request/generation实际backend响应")
            elif tool == "lsp_diagnostics":
                candidates = [(e, obj) for e, obj in current if e.source == "backend_to_sut" and obj.get("method") == "textDocument/publishDiagnostics"
                              and obj.get("params", {}).get("uri") == uri and e.generation == observer.routes[e.instance]["generation"]]
                if not candidates:
                    raise Unverified("diagnostics缺current URI/generation实际publishDiagnostics原文，不能MCP自报")
        return payload

    def locations(self, payload: Any, field: str, expected: dict[str, Any] | None = None) -> list[dict[str, Any]]:
        rows = self.profile.get(payload, field)
        require(isinstance(rows, list) and bool(rows), "没有真实位置结果")
        normalized = []
        for row in rows:
            path = self.profile.get(row, "location.path")
            require(type(path) is str and bool(path), "location.path 无效")
            if path.startswith("file:"):
                parsed = urlparse(path)
                require(parsed.scheme == "file" and parsed.netloc in ("", "localhost"), "非本地 file URI")
                path = unquote(parsed.path)
            normalized.append({"path": path,
                               "line": integer(self.profile.get(row, "location.line"), self.profile.location_base) - self.profile.location_base,
                               "column": integer(self.profile.get(row, "location.column"), self.profile.location_base) - self.profile.location_base})
        if expected:
            require(any(row["path"] == expected["path"] and row["line"] == expected["targetLine"]
                        and row["column"] == expected["targetColumn"] for row in normalized), "definition 源码路径/位置不匹配 fixture")
        return normalized

    def backend(self, boundary: BoundaryObserver | None = None) -> list[tuple[BackendEvent, dict[str, Any]]]:
        observer = boundary or self.boundary
        if observer is None:
            raise Unverified("缺独立 backend wire/monotonic 观察边界")
        rows = []
        for event in observer.events():
            require(isinstance(event, BackendEvent) and event.direction in ("request", "response"), "backend event 无效")
            integer(event.monotonic_ns)
            require(type(event.raw) is bytes and type(event.framing) is bytes, "backend 原文必须 bytes")
            if self.execution == "real":
                if not isinstance(observer, OwnedLSPForwarder):
                    raise Unverified("real backend evidence必须actual OwnedLSPForwarder，capture自报不可用")
                require(event.source in ("sut_to_backend", "backend_to_sut"), "actual event缺真实来源方向")
                require(frame_body(event.framing, self.resources().max_fixture_bytes) == event.raw,
                        "backend framing/raw exact bytes不一致")
            obj = bb._strict_json(event.raw)
            require(isinstance(obj, dict) and obj.get("jsonrpc") == "2.0", "backend 原消息协议无效")
            if event.direction == "request":
                require(type(obj.get("method")) is str and "result" not in obj and "error" not in obj, "request direction/shape冲突")
            else:
                require("method" not in obj and "id" in obj and ("result" in obj) != ("error" in obj), "response direction/shape冲突")
            require(integer(event.generation) >= 0, "backend generation无效")
            rows.append((event, obj))
        return rows

    def status(self, transport: bb.MCPTransport | None = None) -> list[Any]:
        payload = self.call("lsp_status", {}, transport)
        rows = self.profile.get(payload, "status.instances")
        require(isinstance(rows, list), "status.instances 必须列表")
        return rows

    def selected_instance(self, rows: list[Any], f: dict[str, Any]) -> Any:
        matches = [r for r in rows if self.profile.get(r, "instance.root") == f["root"] and self.profile.get(r, "instance.language") == f["language"]]
        require(len(matches) == 1, "实例身份缺失或重复")
        return matches[0]

    def check_01(self) -> dict[str, Any]:
        observed = self.producer.inspect_tool_surface()
        if "integration" not in self.plan.documents:
            raise Unverified("缺显式交付 integration 文档")
        text = self.file(self.plan.documents["integration"]).decode("utf-8")
        require(all(re.search(r"\b" + re.escape(tool) + r"\b", text) for tool in bb.TOOL_NAMES), "十工具未全部文档化")
        return {"tools": observed["observedNames"], "document": self.plan.documents["integration"]}

    def check_02(self) -> dict[str, Any]:
        definitions = {}
        for lang in LANGUAGES:
            f = self.init(lang)
            before = self.status()
            self.init(lang)
            after = self.status()
            a = self.selected_instance(before, f)
            b = self.selected_instance(after, f)
            require(self.profile.get(a, "instance.id") == self.profile.get(b, "instance.id"), "重复 initialize 不幂等")
            definitions[lang] = self.locations(self.query("lsp_definition", f), "definition.locations", f)
        return {"definitions": definitions}

    def check_03(self) -> dict[str, Any]:
        self.authorize("unsaved", ("Python",))
        f = self.init("Python")
        before = self.backend()
        updated = "def replacement():\n    return 'latest-marker'\nvalue = replacement(\n"
        self.call("lsp_sync", {"root": f["root"], "language": f["language"], "path": f["path"], "content": updated})
        after = self.backend()
        changes = [(e, o) for e, o in after[len(before):] if o.get("method") == "textDocument/didChange"]
        require(len(changes) == 1 and changes[0][1]["params"]["contentChanges"][0]["text"] == updated, "未保存原内容没有正确 didChange")
        self.call("lsp_sync", {"root": f["root"], "language": f["language"], "path": f["path"], "content": updated})
        require(sum(o.get("method") == "textDocument/didChange" for _, o in self.backend()) ==
                sum(o.get("method") == "textDocument/didChange" for _, o in after), "unchanged 仍发送 didChange")
        edited = dict(f, line=2, column=9)
        loc = self.locations(self.query("lsp_definition", edited), "definition.locations", f)
        hover = self.profile.get(self.query("lsp_hover", edited), "hover.text")
        require(type(hover) is str and "replacement" in hover, "hover 未反映 latest symbol")
        diagnostic = self.diagnostics(self.query("lsp_diagnostics", edited), 2)
        require(Path(f["path"]).read_text() == f["source"], "sync 擅自改写磁盘")
        self.call("lsp_discard_unsaved", {"root": f["root"], "language": f["language"], "path": f["path"]})
        restored = self.query("lsp_hover", f)
        require("marker" in self.profile.get(restored, "hover.text") and "replacement" not in self.profile.get(restored, "hover.text"), "discard 没有恢复磁盘语义")
        self.locations(self.query("lsp_definition", f), "definition.locations", f)
        require(not self.profile.get(self.query("lsp_diagnostics", f), "diagnostics.items"), "discard 后仍有未保存 syntax diagnostics")
        return {"locations": loc, "latestDiagnostic": diagnostic, "diskSha256": digest(Path(f["path"]).read_bytes())}

    def diagnostics(self, payload: Any, expected_line: int) -> Any:
        rows = self.profile.get(payload, "diagnostics.items")
        require(isinstance(rows, list) and bool(rows), "syntax error 没有诊断")
        matches = []
        for row in rows:
            message = self.profile.get(row, "diagnostic.message")
            require(type(message) is str and bool(message.strip()), "诊断描述缺失")
            line = integer(self.profile.get(row, "diagnostic.line")) - self.profile.location_base
            column = integer(self.profile.get(row, "diagnostic.column")) - self.profile.location_base
            severity = self.profile.get(row, "diagnostic.severity")
            require(severity in ("error", "warning", "information", "hint", 1, 2, 3, 4) and type(severity) is not bool, "诊断级别无效")
            if line == expected_line and column >= 0 and severity in ("error", 1):
                matches.append({"message": message, "line": line, "column": column, "severity": severity})
        require(bool(matches), "syntax diagnostics 行号/列号/错误级别不准确")
        return matches

    def check_04(self) -> dict[str, Any]:
        self.authorize("syntax", ("Python",))
        f = self.init("Python")
        broken = "def marker(:\n    return 7\nvalue = marker()\n"
        self.call("lsp_sync", {"root": f["root"], "language": f["language"], "path": f["path"], "content": broken})
        result = self.diagnostics(self.query("lsp_diagnostics", f), 0)
        self.call("lsp_discard_unsaved", {"root": f["root"], "language": f["language"], "path": f["path"]})
        return {"diagnostic": result}

    def bounded(self, payload: Any, kind: str, expected_limit: int, overflow: bool) -> dict[str, Any]:
        content = self.profile.get(payload, kind + (".text" if kind == "hover" else ".items"))
        require(isinstance(content, str if kind == "hover" else list), "有界内容类型错误")
        returned = len(content)
        limit = integer(self.profile.get(payload, kind + ".limit"), 1)
        total = integer(self.profile.get(payload, kind + ".total"))
        truncated = self.profile.get(payload, kind + ".truncated")
        complete = self.profile.get(payload, kind + ".complete")
        reason = self.profile.get(payload, kind + ".reason")
        require(type(truncated) is bool and type(complete) is bool and type(reason) is str and bool(reason), "输出完整性元信息缺失")
        require(limit == expected_limit and returned == min(total, limit), "上限／实际返回量不一致")
        require(truncated == (total > limit) and complete == (total <= limit), "静默截断／完整性矛盾")
        require(not truncated or reason not in ("none", "within_limit"), "overflow 没有明确 reason")
        require((total > limit) == overflow, "没有真正触发要求的 overflow/within-limit")
        return {"limit": limit, "total": total, "returned": returned, "truncated": truncated, "complete": complete, "reason": reason}

    def configured(self) -> bb.MCPTransport:
        self.authorize("configuration")
        transport = self.plan.configured_transport
        if transport is None:
            raise Unverified("缺以显式配置启动的独立 raw MCP transport")
        require(isinstance(transport, bb.StdioJSONRPCClient), "配置部署必须原 stdio transport")
        grant = self.plan.configured_launch_grant
        if grant is None or not self.plan.configured_input_files:
            raise Unverified("缺 configured launch grant 与明确临时配置输入")
        grant.validate(self.producer.delivery)
        require(transport.grant == grant, "配置部署 launch grant 漂移")
        for path in self.plan.configured_input_files:
            doc = bb._strict_json(self.file(path))
            performance = self.profile.get(doc, "config.performance")
            runtime = self.profile.get(doc, "config.runtime")
            require(isinstance(performance, dict) and isinstance(runtime, dict), "配置输入结构未知")
            merged = performance | runtime
            for key in self.plan.config_values:
                require(key in merged and merged[key] == self.plan.config_values[key], "配置输入值与显式场景参数不一致")
        return transport

    def backend_output_size(self, events: list[tuple[BackendEvent, dict[str, Any]]], kind: str) -> int:
        """以独立 LSP 原结果校验 total，不能凭 MCP 自报 total 构造 overflow。"""
        if kind == "diagnostics":
            candidates = [o["params"]["diagnostics"] for _, o in events if o.get("method") == "textDocument/publishDiagnostics"]
            candidates += [o["result"]["items"] for _, o in events if isinstance(o.get("result"), dict) and "items" in o["result"]]
            require(bool(candidates) and all(isinstance(c, list) for c in candidates), "缺独立 diagnostics 原结果")
            return len(candidates[-1])
        method = "textDocument/hover" if kind == "hover" else "textDocument/references"
        requests = [(e, o) for e, o in events if o.get("method") == method and "id" in o]
        require(len(requests) == 1, "有界输出缺准确 backend 原请求")
        req, obj = requests[0]
        responses = [o["result"] for e, o in events if e.direction == "response" and e.instance == req.instance and o.get("id") == obj["id"] and "result" in o]
        require(len(responses) == 1, "有界输出缺准确 backend 原响应")
        result = responses[0]
        if kind == "references":
            require(isinstance(result, list), "backend references 非列表")
            return len(result)
        require(isinstance(result, dict) and "contents" in result, "backend hover 无 contents")
        contents = result["contents"]
        if isinstance(contents, str):
            return len(contents)
        if isinstance(contents, dict):
            require(type(contents.get("value")) is str, "backend hover markup 缺 value")
            return len(contents["value"])
        if isinstance(contents, list):
            texts = [row if isinstance(row, str) else row.get("value") for row in contents]
            require(all(type(t) is str for t in texts), "backend hover marked string 无效")
            return len("\n".join(texts))
        raise bb.ProducerError("未知 backend hover 格式")

    def check_05(self) -> dict[str, Any]:
        self.authorize("overflow", ("Python",))
        t = self.configured()
        f = self.init("Python", t)
        large = "def marker():\n    \"\"\"" + "documentation " * 30 + "\"\"\"\n    return 7\n" + "\n".join(f"v{i} = marker()" for i in range(8)) + "\n" + "broken(\n" * 8
        require(len(large.encode()) <= self.resources().max_fixture_bytes, "overflow fixture 超出授权")
        self.call("lsp_sync", {"root": f["root"], "language": f["language"], "path": f["path"], "content": large}, t)
        overflow = {}
        limits = {"hover": "maxHoverCharacters", "diagnostics": "maxDiagnostics", "references": "maxReferences"}
        if self.plan.configured_boundary is None:
            raise Unverified("有界输出缺独立 backend 观察器")
        for kind, key in limits.items():
            mark = len(self.backend(self.plan.configured_boundary))
            payload = self.query("lsp_" + kind, dict(f, line=0, column=5), t)
            overflow[kind] = self.bounded(payload, kind, integer(self.plan.config_values[key], 1), True)
            observed_total = self.backend_output_size(self.backend(self.plan.configured_boundary)[mark:], kind)
            require(overflow[kind]["total"] == observed_total, "MCP total 不符独立 backend 原结果，不能冒 overflow")
        mark = len(self.backend(self.plan.configured_boundary))
        self.call("lsp_discard_unsaved", {"root": f["root"], "language": f["language"], "path": f["path"]}, t)
        normal = {}
        # discard可能立即发布清空诊断，故首个 normal 检查保留该独立事件边界。
        for kind, key in limits.items():
            previous = mark if kind == "diagnostics" else len(self.backend(self.plan.configured_boundary))
            payload = self.query("lsp_" + kind, f, t)
            normal[kind] = self.bounded(payload, kind, integer(self.plan.config_values[key], 1), False)
            require(normal[kind]["total"] == self.backend_output_size(self.backend(self.plan.configured_boundary)[previous:], kind),
                    "within-limit total 不符 backend 原文")
        return {"boundedOutputs": {**normal, "overflowProbe": overflow}}

    def check_06(self) -> dict[str, Any]:
        f = self.init("Go")
        row = self.selected_instance(self.status(), f)
        state = self.profile.get(row, "instance.state")
        activity = self.profile.get(row, "instance.activity")
        restart = integer(self.profile.get(row, "instance.restartCount"))
        present = self.profile.get(row, "instance.dependencyPresent")
        running = self.profile.get(row, "instance.dependencyRunning")
        require(state in ("ready", "idle") and activity in ("active", "idle"), "status 状态/活跃字段无效")
        require(type(present) is bool and type(running) is bool and present and running, "status dependency 非实际运行状态")
        return {"status": {"instanceState": state, "activity": activity, "restartCount": restart,
                           "dependencyPresent": present, "dependencyRunning": running}}

    def check_07(self) -> dict[str, Any]:
        self.authorize("shutdown", ("Go", "Python"))
        a, b = self.init("Go"), self.init("Python")
        self.call("lsp_shutdown", {"root": a["root"], "language": a["language"]})
        if self.execution == "real":
            if not isinstance(self.boundary, OwnedLSPForwarder):
                raise Unverified("shutdown需actual ownedbackend退出观察")
            matching = [self.boundary._children[ident][0] for ident, r in self.boundary.routes.items() if r["language"] == "Go"]
            require(bool(matching) and all(child.poll() is not None for child in matching), "single shutdown仅status关闭，实际backend仍活")
        rows = self.status()
        require(not any(self.profile.get(r, "instance.root") == a["root"] and self.profile.get(r, "instance.state") not in ("stopped", "closed") for r in rows), "single shutdown 未关闭")
        self.locations(self.query("lsp_definition", b), "definition.locations", b)
        self.call("lsp_shutdown", {"all": True})
        if self.execution == "real" and isinstance(self.boundary, OwnedLSPForwarder):
            require(all(child.poll() is not None for child, _ in self.boundary._children.values()), "all shutdown实际ownedbackend仍活")
        require(all(self.profile.get(r, "instance.state") in ("stopped", "closed") for r in self.status()), "all shutdown 仍有运行实例")
        self.init("Go")
        self.locations(self.query("lsp_definition", a), "definition.locations", a)
        return {"singleClosed": True, "allClosed": True, "reinitialized": True}

    def check_08(self) -> dict[str, Any]:
        self.authorize("repair", ("Go",))
        f = self.init("Go")
        before = self.status()
        evidence_before = len(self.backend())
        advice = self.call("lsp_repair", {"root": f["root"], "language": f["language"]})
        actions = self.profile.get(advice, "repair.actions")
        require(isinstance(actions, list), "repair 未返回可解释 action 列表")
        after = self.status()
        require(before == after and len(self.backend()) == evidence_before, "默认 advice 改变了运行实例")
        # safe apply 对健康实例不能执行破坏性动作；退出实例由独立 ownedchild handle 制造。
        applied = self.call("lsp_repair", {"root": f["root"], "language": f["language"], "apply": True})
        executed = self.profile.get(applied, "repair.executed")
        require(isinstance(executed, list) and not executed, "healthy instance apply 进行了非必要动作")
        self.locations(self.query("lsp_definition", f), "definition.locations", f)
        row = self.selected_instance(self.status(), f)
        ident = self.instance_id(f, self.profile.get(row, "instance.id"))
        self.kill_owned(ident)
        advice = self.call("lsp_repair", {"root": f["root"], "language": f["language"]})
        require(any(self.profile.get(a, "action.kind") == "restart_exited_instance" for a in self.profile.get(advice, "repair.actions")), "退出实例没有安全恢复建议")
        require(not self.profile.get(advice, "repair.executed"), "advice 已执行恢复")
        applied = self.call("lsp_repair", {"root": f["root"], "language": f["language"], "apply": True})
        executed = self.profile.get(applied, "repair.executed")
        require(isinstance(executed, list) and bool(executed) and all(self.profile.get(a, "action.kind") == "restart_exited_instance" for a in executed), "apply 执行了非安全实例动作")
        self.locations(self.query("lsp_definition", f), "definition.locations", f)
        return {"defaultIsAdvice": True, "applySafeOnly": True, "allowedAction": "restart_exited_instance"}

    def check_09(self) -> dict[str, Any]:
        self.authorize("performance", ("Go",))
        f = self.init("Go")
        self.query("lsp_definition", f)
        self.query("lsp_hover", f)
        timings = {}
        for tool, method in (("lsp_definition", "textDocument/definition"), ("lsp_hover", "textDocument/hover")):
            samples = []
            for _ in range(3):
                previous = len(self.backend())
                self.query(tool, f)
                call = self.observations[-1]
                events = self.backend()[previous:]
                requests = [(e, o) for e, o in events if o.get("method") == method and "id" in o]
                require(len(requests) == 1, "性能缺准确单次 backend request 边界")
                req, obj = requests[0]
                require(req.direction == "request" and type(obj.get("id")) is int, "性能backend request方向/ID无效")
                responses = [e for e, o in events if e.direction == "response" and o.get("id") == obj["id"] and e.instance == req.instance
                             and "result" in o and "error" not in o]
                require(len(responses) == 1, "性能缺准确 backend response 边界")
                res = responses[0]
                require(call["startNs"] <= req.monotonic_ns <= res.monotonic_ns <= call["endNs"], "性能时间轴不在同一 monotonic clock／范围")
                internal = (call["endNs"] - call["startNs"] - (res.monotonic_ns - req.monotonic_ns)) / 1e6
                require(0 <= internal < 50, "warm MCP internal >=50ms")
                samples.append(internal)
            timings[tool] = {"samplesMs": samples, "maxMs": max(samples), "scope": "client_wall_minus_backend_wait_conservative_upper_bound",
                             "warm": True, "preciseMcpInternalMeasured": False,
                             "derivation": "wall subtracts independently observed LSP wait; client/IPC remains, so <50ms is a conservative bound, not exact MCP-only timing"}
        return timings

    def check_10(self) -> dict[str, Any]:
        self.authorize("rss", ("Go", "Python", "TypeScript"))
        fixtures = [self.init(lang) for lang in ("Go", "Python", "TypeScript")]
        for f in fixtures:
            self.query("lsp_definition", f)
        live_before = None
        if self.execution == "real":
            if not isinstance(self.boundary, OwnedLSPForwarder):
                raise Unverified("RSS真实3backend存活需actual forwarding ownedhandles，不信status自报")
            require(isinstance(self.producer.transport, bb.StdioJSONRPCClient), "RSS缺实际MCP handle")
            live_before = self.boundary.alive_backends(self.producer.transport.process, ("Go", "Python", "TypeScript"))
        before_rows = self.status()
        before_ids = []
        for f in fixtures:
            row = self.selected_instance(before_rows, f)
            require(self.profile.get(row, "instance.state") in ("ready", "idle")
                    and self.profile.get(row, "instance.dependencyRunning") is True
                    and self.profile.get(row, "instance.dependencyPresent") is True, "RSS前3language非同时alive")
            before_ids.append(self.profile.get(row, "instance.id"))
        if self.boundary is None:
            raise Unverified("缺 server owned PID 的 OS RSS 观察器")
        sample = self.boundary.sample_server_rss()
        if live_before is not None:
            require(isinstance(self.boundary, OwnedLSPForwarder) and isinstance(self.producer.transport, bb.StdioJSONRPCClient), "RSS真实handle缺失")
            live_after = self.boundary.alive_backends(self.producer.transport.process, ("Go", "Python", "TypeScript"))
            require(live_after == live_before, "RSS前后actual owned backend身份/存活漂移")
        after_rows = self.status()
        for f, ident in zip(fixtures, before_ids):
            row = self.selected_instance(after_rows, f)
            require(self.profile.get(row, "instance.id") == ident and self.profile.get(row, "instance.state") in ("ready", "idle")
                    and self.profile.get(row, "instance.dependencyRunning") is True, "RSS期间3language身份/存活漂移")
        require(isinstance(sample, RssSample), "RSS 不是 OS raw sample")
        require(isinstance(self.producer.transport, bb.StdioJSONRPCClient) and sample.pid == self.producer.transport.process.pid,
                "RSS PID 必须是 granted MCP 本身，不能 children／selfreport")
        parts = sample.proc_statm.split()
        require(len(parts) >= 2 and all(part.isdigit() for part in parts), "proc/statm malformed")
        value = int(parts[1]) * integer(sample.page_size, 1) / (1024 * 1024)
        self.artifact("server-statm", sample.proc_statm, "benchmark_report")
        require(0 < value < 64, "MCP RSS 不小于64MB")
        return {"rssMB": value, "pid": sample.pid, "languages": ["Go", "Python", "TypeScript"], "scope": "mcp_process_excluding_children"}

    def check_11(self) -> dict[str, Any]:
        t = self.configured()
        bounds = self.check_05()
        if self.plan.configured_boundary is None:
            raise Unverified("缺独立可控 backend 延迟观察；不能凭配置回显判 timeout 生效")
        # observer 必须提供独立 delay 实验，原始请求/响应及 wall deadline；没有该功能不假测。
        delay = getattr(self.plan.configured_boundary, "delay_next_response", None)
        if delay is None:
            raise Unverified("独立 backend 不支持明确授权 delay_next_response 实验")
        self.authorize("timeout")
        timeout = number(self.plan.config_values["defaultTimeout"])
        require(0 < timeout < self.resources().max_wait_seconds, "timeout 实验超出等待授权")
        f = self.init("Python", t)
        delay(timeout * 3)
        start = time.monotonic_ns()
        failed = None
        try:
            self.query("lsp_definition", f, t)
        except bb.ProducerError as exc:
            failed = str(exc)
        elapsed = (time.monotonic_ns() - start) / 1e9
        require(failed is not None and "timeout" in failed.lower(), "defaultTimeout 没有产生明确 timeout 错误")
        require(timeout * 0.5 <= elapsed < timeout * 2.5, "timeout 与实际等待不匹配")
        return {"configuration": {"keys": ["defaultTimeout", "maxReferences", "maxDiagnostics", "maxHoverCharacters"],
                                  "bounds": bounds, "timeoutObservedSeconds": elapsed}}

    def instance_id(self, f: dict[str, Any], status_id: Any) -> str:
        require(type(status_id) is str and bool(status_id), "status instance ID无效")
        if isinstance(self.boundary, OwnedLSPForwarder) and isinstance(self.producer.transport, bb.StdioJSONRPCClient):
            matches = [instance for instance, r in self.boundary.routes.items()
                       if r["mcp"] is self.producer.transport.process and r["language"] == next((lang for lang, values in LANGUAGES.items() if values[0] == f["language"]), "")]
            require(len(matches) == 1, "语言actualroute不唯一，不猜测instance mapping")
            instance = matches[0]
            requests = [(e, o) for e, o in self.backend() if e.instance == instance and e.source == "sut_to_backend"
                        and e.generation == self.boundary.routes[instance]["generation"]
                        and o.get("method") in ("textDocument/definition", "textDocument/hover")]
            require(any(o.get("params", {}).get("textDocument", {}).get("uri") == Path(f["path"]).as_uri() for _, o in requests),
                    "actual route没有当前fixture路径的语义转发，不能映射SUT实例")
            return instance
        if self.execution == "real":
            raise Unverified("缺actual relay用于映射SUT实例/ownedbackend")
        return status_id

    def kill_owned(self, instance: str) -> str:
        self.authorize("owned-child-kill")
        if self.boundary is None:
            raise Unverified("缺独立 owned child handle，禁止按 SUT PID kill")
        require(type(instance) is str and bool(instance), "instance id 必须明确非空")
        if self.execution == "real":
            lineage = getattr(self.boundary, "owned_lineage", None)
            if lineage is None or not isinstance(self.producer.transport, bb.StdioJSONRPCClient):
                raise Unverified("缺actual SUT-backend forwarding ownedhandle lineage")
            observed = lineage(instance, self.producer.transport.process)
            require(isinstance(observed, dict) and observed.get("mcpHandle") is self.producer.transport.process
                    and isinstance(observed.get("childHandle"), subprocess.Popen), "lineage不是实际进程handle")
            semantic = [(e, o) for e, o in self.backend() if e.instance == instance and o.get("method") in ("textDocument/definition", "textDocument/hover")]
            if not semantic:
                raise Unverified("lineage缺实际语义转发原request/response")
            require(any(e.direction == "response" and e.instance == instance and o.get("id") == semantic[-1][1].get("id") for e, o in self.backend()), "lineage无matched语义response")
        result = self.boundary.kill_owned(instance)
        require(isinstance(result, bb.ProcessObservation) and type(result.exit_code) is int and result.exit_code < 0 and not result.timed_out,
                "未观察 owned child 非零 signal 退出")
        grant = self.resources().language_commands
        require(any(result.argv == g.argv and result.cwd == g.cwd for g in grant.values()), "kill result 不属于明确 language CommandGrant")
        ref = self.artifact("owned-child-exit-stdout", result.stdout)
        self.artifact("owned-child-exit-stderr", result.stderr)
        self.artifact("owned-child-exit-process", bb.encode_json(asdict(result) | {"stdout": base64.b64encode(result.stdout).decode(), "stderr": base64.b64encode(result.stderr).decode()}))
        return ref

    def check_12(self) -> dict[str, Any]:
        self.authorize("restart", ("Go",))
        f = self.init("Go")
        self.locations(self.query("lsp_definition", f), "definition.locations", f)
        row = self.selected_instance(self.status(), f)
        previous = integer(self.profile.get(row, "instance.restartCount"))
        ident = self.instance_id(f, self.profile.get(row, "instance.id"))
        evidence = self.kill_owned(ident)
        mark = len(self.backend())
        locations = self.locations(self.query("lsp_definition", f), "definition.locations", f)
        events = self.backend()[mark:]
        require(any(o.get("method") == "initialize" for _, o in events), "next request 未重启并初始化")
        row = self.selected_instance(self.status(), f)
        require(integer(self.profile.get(row, "instance.restartCount")) == previous + 1, "restartCount 未增加一次")
        return {"trigger": "manual_kill", "killEvidenceRef": evidence, "locations": locations}

    def check_13(self) -> dict[str, Any]:
        self.authorize("lifecycle", ("Go", "Python", "TypeScript"))
        t = self.configured()
        ttl = number(self.plan.config_values["idleTTL"])
        capacity = integer(self.plan.config_values["maxInstances"], 1)
        require(capacity == 2 and ttl * 2 <= self.resources().max_wait_seconds, "此场景需2容量与授权内TTL")
        a, b = self.init("Go", t), self.init("Python", t)
        self.query("lsp_definition", a, t)
        self.query("lsp_definition", b, t)
        c = self.init("TypeScript", t)
        rows = self.status(t)
        live = [r for r in rows if self.profile.get(r, "instance.state") not in ("closed", "stopped")]
        require(len(live) == 2, "实例容量未收敛")
        require(not any(self.profile.get(r, "instance.root") == a["root"] for r in live), "LRU 未回收最旧 idle")
        self.selected_instance(live, b)
        self.selected_instance(live, c)
        observer = self.plan.configured_boundary
        hold = getattr(observer, "hold_query", None)
        if hold is None:
            raise Unverified("缺独立 backend 在途 hold，无法证明 only idle reclaimed")
        # 外部观察器持有真实 query future，不能返回 pass boolean。
        pending = hold(self.profile.args("lsp_definition", {k: c[k] for k in ("root", "language", "path", "line", "column")}), t)
        require(hasattr(pending, "release") and hasattr(pending, "result"), "hold 必须返回可释放实际请求 handle")
        try:
            time.sleep(ttl * 1.5)
            if self.execution == "real":
                if not isinstance(observer, OwnedLSPForwarder) or not isinstance(t, bb.StdioJSONRPCClient):
                    raise Unverified("active TTL需actualrelay ownedhandle")
                actual = observer.alive_backends(t.process, ("TypeScript",))
                require(len(actual) == 1, "active backend被TTL回收")
                # stdio client串行锁不能在held call期间用status证明idle回收，直接看实际ownedbackend退出。
                python_routes = [ident for ident, r in observer.routes.items() if r["language"] == "Python" and r["mcp"] is t.process]
                require(len(python_routes) == 1 and observer._children[python_routes[0]][0].poll() is not None,
                        "idle TTL没有actual backend退出")
            else:
                rows = self.status(t)
                live = [r for r in rows if self.profile.get(r, "instance.state") not in ("closed", "stopped")]
                require(not any(self.profile.get(r, "instance.root") == b["root"] for r in live), "idle TTL 未自动回收")
                self.selected_instance(live, c)
        finally:
            pending.release()
        self.locations(pending.result(), "definition.locations", c)
        return {"idleTtlReclaimed": True, "capacityLruReclaimed": True, "onlyIdleReclaimed": True}

    def check_14(self) -> dict[str, Any]:
        self.authorize("uri", ("Python",))
        f = self.init("Python")
        path = Path(f["path"]).parent / "空 格#%.py"
        path.write_text(f["source"], encoding="utf-8")
        special = dict(f, path=str(path))
        self.locations(self.query("lsp_definition", special), "definition.locations", special)
        uri = dict(special, path=path.as_uri())
        self.locations(self.query("lsp_definition", uri), "definition.locations", special)
        if "integration" not in self.plan.documents:
            raise Unverified("无法从交付文档判断 Windows 支持声明")
        text = self.file(self.plan.documents["integration"]).decode()
        windows = bool(re.search(r"(?i)(?:支持\s*Windows|Windows\s+(?:support|supported))", text))
        if windows:
            grant = self.plan.command_grants.get("windows-uri")
            if grant is None:
                raise Unverified("文档声明 Windows，缺显式 Windows URI 实际测试命令")
            result = self.run_command("windows-uri", grant)
            self.command_records(result, "windows-uri", required_tests=("drive_absolute", "drive_relative_rejected", "unc", "percent_encoding"))
        return {"linuxPassed": True, "windowsClaimed": windows, "windowsPassed": windows}

    def run_command(self, label: str, grant: bb.CommandGrant) -> bb.ProcessObservation:
        self.sequence += 1
        result = self.producer.run_child(f"suite-process-{self.sequence}-" + label, grant)
        require(result.exit_code == 0 and not result.timed_out, "命令失败/超时")
        self.command_results[label] = result
        return result

    def command_records(self, result: bb.ProcessObservation, label: str, required_tests: tuple[str, ...] = ()) -> list[str]:
        # Go JSON test stream 来自 actual command stdout，不接受一行 passed=true。
        actions: dict[str, list[str]] = {}
        for raw in result.stdout.splitlines():
            try:
                obj = bb._strict_json(raw)
            except ValueError:
                continue
            if isinstance(obj, dict) and type(obj.get("Test")) is str and obj.get("Action") in ("run", "pass", "fail", "skip"):
                actions.setdefault(obj["Test"], []).append(obj["Action"])
        require(bool(actions) and all("fail" not in v and "skip" not in v and "run" in v and v[-1] == "pass" for v in actions.values()),
                f"{label} 缺 actual run/pass 测试记录，skip/空退出码不能通过")
        for name in required_tests:
            require(any(name in test for test in actions), f"{label} 缺 {name} 实际测试记录")
        require(b"DATA RACE" not in result.stdout + result.stderr, "发现 race")
        return sorted(actions)

    def check_15(self) -> dict[str, Any]:
        self.authorize("concurrency", ("Go",))
        grant = self.plan.goroutine_grant
        if grant is None:
            raise Unverified("缺 actual Go 10-goroutine harness CommandGrant；Python threads 不算")
        f = self.init("Go")
        mcp = self.plan.concurrent_mcp_grant
        if mcp is None:
            raise Unverified("缺并发 harness 独立 MCP CommandGrant")
        mcp.validate(self.producer.delivery)
        # Go harness 本身创建 exact granted MCP，stdin 是明确计划，不借 inherited fd。
        run_input = getattr(self.producer.process_executor, "run_with_input", None)
        if run_input is None:
            raise Unverified("process executor 不支持明确 harness stdin 输入")
        requests = []
        for index in range(10):
            tool = "lsp_definition" if index % 2 == 0 else "lsp_hover"
            requests.append({"jsonrpc": "2.0", "id": index + 100,
                             "method": "tools/call", "params": {"name": tool,
                             "arguments": self.profile.args(tool, {k: f[k] for k in ("root", "language", "path", "line", "column")})}})
        setup = {"argv": list(mcp.argv), "cwd": mcp.cwd, "env": dict(mcp.env),
                 "initialize": {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
                     "protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "prd-suite", "version": "1"}}},
                 "workspace": {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {
                     "name": "lsp_initialize", "arguments": self.profile.args("lsp_initialize", {"root": f["root"], "language": f["language"]})}},
                 "timeoutMs": min(grant.timeout_seconds, mcp.timeout_seconds) * 1000,
                 "requests": requests}
        self.artifact("goroutine-input", bb.encode_json(setup))
        grant.validate(self.producer.delivery)
        harness_path = Path(grant.argv[-1])
        require(harness_path.name == "suite_goroutines.go", "并发命令未运行固定 harness 文件")
        require(Path(grant.argv[0]).name == "go" and tuple(grant.argv[1:-1]) == ("run", "-race"), "并发必须 actual go run -race 固定 harness")
        require(self.file(str(harness_path.relative_to(self.producer.delivery))) == GO_CONCURRENT_HARNESS.encode(), "并发 harness 源码不是交付固定合同")
        result = run_input(grant, bb.encode_json(setup))
        require(isinstance(result, bb.ProcessObservation) and result.argv == grant.argv and result.cwd == grant.cwd,
                "harness process observation 与 grant 不匹配")
        self.artifact("goroutine-stdout", result.stdout)
        self.artifact("goroutine-stderr", result.stderr)
        require(result.exit_code == 0 and not result.timed_out, "并发 harness 失败／超时")
        # 固定源码在任何执行前校验；这里只解析实际原 wire pair。
        exchanges = []
        for raw in result.stdout.splitlines():
            obj = bb._strict_json(raw)
            require(isinstance(obj, dict) and set(obj) == {"request", "response", "sentNs", "receivedNs"}, "并发 harness 输出不是原 wire pair/timeline")
            sent, received = integer(obj["sentNs"]), integer(obj["receivedNs"])
            require(received >= sent, "并发monotonic时间倒置")
            request = bb._strict_json(base64.b64decode(obj["request"], validate=True))
            response = bb._strict_json(base64.b64decode(obj["response"], validate=True))
            exchanges.append((request, response, sent, received))
        require(len(exchanges) == 10 and len({r[0].get("id") for r in exchanges}) == 10, "并发没有10 unique IDs")
        require(max(row[2] for row in exchanges) < min(row[3] for row in exchanges), "10请求没有同时pending overlap")
        for req, res, _, _ in exchanges:
            require(req.get("jsonrpc") == "2.0" and type(req.get("id")) is int and res.get("id") == req["id"] and "error" not in res, "并发 request ID mismatch/error")
            require(req.get("method") == "tools/call", "并发未执行不同实际查询")
            content = res.get("result", {}).get("content")
            require(isinstance(content, list) and len(content) == 1, "并发结果未完整")
            payload = bb._strict_json(content[0]["text"].encode())
            if req["params"]["name"] == "lsp_definition":
                self.locations(payload, "definition.locations", f)
            elif req["params"]["name"] == "lsp_hover":
                require("marker" in self.profile.get(payload, "hover.text"), "并发 hover mismatch")
            else:
                raise bb.ProducerError("并发 harness 出现未知 query")
        require({req["params"]["name"] for req, _, _, _ in exchanges} == {"lsp_definition", "lsp_hover"}, "并发缺不同查询")
        require({req["id"]: req for req, _, _, _ in exchanges} == {req["id"]: req for req in requests}, "harness 实际请求与授权原计划漂移")
        race = self.plan.command_grants.get("race")
        if race is None:
            raise Unverified("缺 SUT go test -race CommandGrant")
        require(tuple(race.argv[1:]) == COMMANDS["race"][1:], "race 命令不符合 PRD")
        race_result = self.run_command("race", race)
        require(any(re.match(rb"^ok\s+\S+\s+", line) and b"[no tests to run]" not in line for line in race_result.stdout.splitlines())
                and b"DATA RACE" not in race_result.stdout + race_result.stderr, "SUT race 没有实际测试输出")
        trace = self.plan.command_grants.get("race-json")
        if trace is None:
            raise Unverified("缺actual go test -race -json run/pass事件grant")
        require(tuple(trace.argv[1:]) == ("test", "-race", "-json", "./..."), "race-json命令错误")
        self.command_records(self.run_command("race-json", trace), "race-json")
        return {"goroutines": 10, "requestIds": sorted(r[0]["id"] for r in exchanges), "raceOutputSha256": digest(race_result.stdout)}

    def fresh_outputs(self, paths: tuple[str, ...]) -> None:
        if not paths:
            raise Unverified("缺明确 fresh build output 路径")
        for relative in paths:
            require(bool(relative) and not Path(relative).is_absolute() and ".." not in Path(relative).parts, "build产物必须受限路径")
            path = self.producer.delivery / relative
            bb._reject_symlinks(path)
            require(not path.exists(), "build产物预存，不能凭退出码冒新构建")

    def inventory(self, root: Path, expected: tuple[str, ...]) -> dict[str, str]:
        if not expected:
            raise Unverified("缺完整显式bounded delivery inventory")
        require(len(expected) == len(set(expected)) and len(expected) <= 10000, "inventory重复／超界")
        bb._reject_symlinks(root)
        actual = []
        for path in root.rglob("*"):
            bb._reject_symlinks(path)
            require(path.is_file() or path.is_dir(), "snapshot包含非regular项")
            if path.is_file():
                actual.append(str(path.relative_to(root)))
                require(len(actual) <= len(expected), "snapshot有extra文件")
        require(set(actual) == set(expected), "snapshot missing/extra文件")
        hashes = {}
        total = 0
        for relative in expected:
            raw = self.file(str((root / relative).relative_to(self.producer.delivery)))
            total += len(raw)
            require(self.plan.resources is not None and total <= self.resources().max_fixture_bytes, "snapshot超出明确bytes grant")
            hashes[relative] = digest(raw)
        return hashes

    def check_16(self) -> dict[str, Any]:
        self.authorize("engineering")
        output = {}
        for label, expected in COMMANDS.items():
            grant = self.plan.command_grants.get(label)
            if grant is None:
                raise Unverified(f"缺 exact {label} CommandGrant")
            require(tuple(grant.argv[1:]) == expected[1:] and Path(grant.argv[0]).name == "go", "工程命令不符合 PRD")
            if label == "build":
                self.fresh_outputs(self.plan.build_outputs)
            result = self.run_command(label, grant)
            if label != "build":
                rows = result.stdout.decode("utf-8", errors="strict").splitlines()
                require(any(bool(re.match(r"^ok\s+\S+\s+", line)) and "[no tests to run]" not in line for line in rows)
                        and b"DATA RACE" not in result.stdout + result.stderr,
                        "test/race 缺实际测试输出")
            else:
                if not self.plan.build_outputs:
                    raise Unverified("build 退出码不足：缺明确构建产物观察路径")
                for path in self.plan.build_outputs:
                    require(bool(self.file(path)), "build 未产生非空交付产物")
            output[label] = {"argv": list(result.argv), "stdoutSha256": digest(result.stdout), "stderrSha256": digest(result.stderr)}
        return output

    def check_17(self) -> dict[str, Any]:
        self.authorize("real-go", ("Go",))
        if not isinstance(self.boundary, OwnedLSPForwarder) or not isinstance(self.producer.transport, bb.StdioJSONRPCClient):
            raise Unverified("真实Go integration必须本suite控制test→MCP→actual owned gopls同session")
        backend_grant = self.resources().language_commands["Go"]
        expected = self.plan.real_go_executable_sha256
        if expected is None:
            raise Unverified("缺显式gopls executable bytes身份授权")
        executable = Path(backend_grant.argv[0])
        bb._reject_symlinks(executable)
        require(executable.name == "gopls" and digest(executable.read_bytes()) == expected,
                "backend executable不符明确gopls source identity")
        f = self.init("Go")
        mark = len(self.backend())
        start = time.monotonic_ns()
        definition = self.locations(self.query("lsp_definition", f), "definition.locations", f)
        hover = self.profile.get(self.query("lsp_hover", f), "hover.text")
        require(type(hover) is str and "marker" in hover, "integration hover不对应test fixture")
        end = time.monotonic_ns()
        instance = self.instance_id(f, self.profile.get(self.selected_instance(self.status(), f), "instance.id"))
        lineage = self.boundary.owned_lineage(instance, self.producer.transport.process)
        child = lineage["childHandle"]
        require(child.poll() is None and lineage["grant"] == backend_grant, "integration actualbackend不符grant")
        cmdline = Path(f"/proc/{child.pid}/cmdline").read_bytes()
        actual_argv = cmdline.split(b"\x00")[:-1]
        if self.execution == "synthetic" and executable.read_bytes().startswith(b"#!"):
            require(actual_argv == [executable.read_bytes().splitlines()[0][2:]] + [arg.encode() for arg in backend_grant.argv],
                    "synthetic shebang实际进程cmdline漂移")
        else:
            require(actual_argv == [arg.encode() for arg in backend_grant.argv], "gopls实际进程cmdline漂移")
        events = self.backend()[mark:]
        expected_uri = Path(f["path"]).as_uri()
        for method in ("textDocument/definition", "textDocument/hover"):
            requests = [(e, obj) for e, obj in events if e.instance == instance and e.generation == lineage["generation"]
                        and e.source == "sut_to_backend" and obj.get("method") == method
                        and obj.get("params", {}).get("textDocument", {}).get("uri") == expected_uri]
            require(len(requests) == 1 and start <= requests[0][0].monotonic_ns <= end, "test与同session/backend URI/request不关联")
            event, request = requests[0]
            matches = [(e, obj) for e, obj in events if e.instance == instance and e.generation == event.generation
                       and e.source == "backend_to_sut" and obj.get("id") == request.get("id") and "result" in obj]
            require(len(matches) == 1 and event.monotonic_ns <= matches[0][0].monotonic_ns <= end, "integration缺同generation matched backend原响应")
        self.artifact("real-go-proc-cmdline", cmdline)
        self.artifact("real-go-test-fixture", Path(f["path"]).read_bytes(), "source_snapshot")
        return {"test": "suite-controlled-real-go-definition-hover/v1", "definition": definition,
                "goplsExecutableSha256": expected, "mcpPid": self.producer.transport.process.pid,
                "backendPid": child.pid, "instance": instance, "generation": lineage["generation"],
                "execution": self.execution, "fixture": self.execution == "synthetic"}


    def check_18(self) -> dict[str, Any]:
        if self.plan.config_example is None:
            raise Unverified("缺显式交付 mcp-config.json 示例")
        doc = bb._strict_json(self.file(self.plan.config_example))
        runtime = self.profile.get(doc, "config.runtime")
        performance = self.profile.get(doc, "config.performance")
        languages = self.profile.get(doc, "config.languageCommands")
        require(isinstance(runtime, dict) and bool(runtime) and isinstance(performance, dict) and isinstance(languages, dict), "配置样例不完整")
        require(all(k in performance for k in ("defaultTimeout", "maxReferences", "maxDiagnostics", "maxHoverCharacters")), "性能配置示例缺键")
        for value in performance.values():
            number(value)
        require(set(languages) >= set(LANGUAGES), "配置样例缺五语言命令")
        for lang in LANGUAGES:
            require(isinstance(languages[lang], list) and bool(languages[lang]) and all(type(v) is str and bool(v) for v in languages[lang]), "语言命令不是 argv")
        return {"configExample": ["runtime", "performance", "languageCommands"]}

    def check_19(self) -> dict[str, Any]:
        observed = {}
        for section, tokens in DOC_SECTIONS.items():
            if section not in self.plan.documents:
                raise Unverified(f"缺显式 {section} 交付文档")
            raw = self.file(self.plan.documents[section])
            text = raw.decode("utf-8")
            require(len(text.strip()) >= 20 and all(token in text for token in tokens), f"{section} 文档没有所要求内容定位")
            observed[section] = {"path": self.plan.documents[section], "sha256": digest(raw), "locatedTerms": list(tokens)}
        return {"documents": observed, "limitation": "内容定位证明交付，不替代人工文档质量评审"}

    def git_snapshot(self, whole: dict[str, str]) -> dict[str, str]:
        """实际受限Git object读取反证commit/tree，不接受pin文本造source来源。"""
        self.authorize("git-snapshot")
        repo, executable = self.plan.git_repository_root, self.plan.git_executable
        if repo is None or executable is None:
            raise Unverified("缺explicit Git owner snapshot repository/grants")
        root = self.producer.delivery / repo
        bb._reject_symlinks(root)
        require(root.is_dir() and Path(executable).is_absolute() and Path(executable).name == "git", "Git snapshot边界无效")
        commit = self.producer.identities["deliveryCommit"]
        require(bool(re.fullmatch(r"[0-9a-f]{40}|[0-9a-f]{64}", commit)), "deliveryCommit不是actual commit hash")
        grants = {g.argv: g for g in self.plan.git_grants}
        def execute(arguments: tuple[str, ...]) -> bytes:
            argv = (executable,) + arguments
            grant = grants.get(argv)
            if grant is None:
                raise Unverified("缺exact readonly Git grant: " + repr(arguments))
            require(Path(grant.cwd) == root, "Git读取不在明确temporary snapshot repository")
            return self.run_command("git-" + str(self.sequence), grant).stdout
        kind = execute(("cat-file", "-t", commit))
        require(kind.strip() == b"commit", "pin不是Git commit object")
        tree = execute(("ls-tree", "-rz", "--full-tree", commit))
        entries = {}
        for raw in tree.split(b"\x00"):
            if not raw:
                continue
            metadata, name = raw.split(b"\t", 1)
            mode, object_type, object_id = metadata.split(b" ")
            relative = name.decode("utf-8", errors="strict")
            require(mode in (b"100644", b"100755") and object_type == b"blob" and relative in self.plan.delivery_inventory,
                    "Git tree extra/symlink/submodule/unknown文件")
            require(relative not in entries, "Git tree重复文件")
            require(bool(re.fullmatch(rb"[0-9a-f]{40}|[0-9a-f]{64}", object_id)), "Git blob ID错误")
            blob = execute(("cat-file", "blob", object_id.decode("ascii")))
            require(digest(blob) == whole[relative], "Git commit tree与实际交付snapshot bytes不符")
            entries[relative] = object_id.decode("ascii")
        require(set(entries) == set(whole), "Git commit tree missing文件")
        return entries

    def check_20(self) -> dict[str, Any]:
        self.authorize("reproducibility")
        require(bool(self.plan.source_files) and bool(self.plan.test_files), "未交付 source/tests snapshot")
        sources = {path: digest(self.file(path)) for path in self.plan.source_files}
        tests = {path: digest(self.file(path)) for path in self.plan.test_files}
        require(any(path.endswith(".go") for path in sources) and any(path.endswith("_test.go") for path in tests), "交付缺核心Go/source tests")
        for path in (*sources, *tests):
            require(bool(self.file(path).strip()), "交付空 source/tests")
        a, b = self.plan.command_grants.get("reproduce-a"), self.plan.command_grants.get("reproduce-b")
        if a is None or b is None or not self.plan.reproducible_outputs:
            raise Unverified("缺两独立 clean snapshot build grants/明确产物路径")
        require(a.cwd != b.cwd and tuple(a.argv[1:]) == tuple(b.argv[1:]) == COMMANDS["build"][1:], "复现必须两独立目录同build命令")
        source_root = self.producer.delivery / self.plan.snapshot_source_root
        bb._reject_symlinks(source_root)
        whole = self.inventory(source_root, self.plan.delivery_inventory)
        git_tree = self.git_snapshot(whole)
        commit = self.producer.identities["deliveryCommit"]
        roots = (Path(a.cwd), Path(b.cwd))
        for root in roots:
            require(self.inventory(root, self.plan.delivery_inventory) == whole, "clean snapshot完整tree bytes漂移")
        self.fresh_outputs(tuple(path for pair in self.plan.reproducible_outputs for path in pair))
        self.run_command("reproduce-a", a)
        self.run_command("reproduce-b", b)
        outputs = []
        for first, second in self.plan.reproducible_outputs:
            ra, rb = self.file(first), self.file(second)
            require(bool(ra) and ra == rb, "两次构建产物 bytes 不可复现")
            outputs.append({"first": first, "second": second, "sha256": digest(ra)})
        require(self.inventory(source_root, self.plan.delivery_inventory) == whole, "source snapshot在构建期mutation")
        for root in roots:
            expected_after = self.plan.delivery_inventory + tuple(str((self.producer.delivery / path).relative_to(root))
                for pair in self.plan.reproducible_outputs for path in pair if (self.producer.delivery / path).is_relative_to(root))
            after = self.inventory(root, expected_after)
            require(all(after[path] == sha for path, sha in whole.items()), "clean source/test在构建期mutation")
        return {"sourceHashes": sources, "testHashes": tests, "fullTreeHashes": whole,
                "deliveryCommit": commit, "gitTreeBlobs": git_tree, "outputs": outputs}

    def run(self) -> dict[str, Any]:
        require(not self._finished, "suite 不能重复执行")
        for index in range(1, 21):
            start_artifacts, start_calls = len(self.producer.artifacts), len(self.observations)
            method = getattr(self, f"check_{index:02d}")
            row: dict[str, Any] = {"id": f"LSP-{index:02d}", "execution": self.execution}
            try:
                row["derived"] = method()
                row["status"] = "SYNTHETIC_PASS" if self.execution == "synthetic" else "PASS"
            except Unverified as exc:
                row.update(status="NOT_VERIFIED", reason=str(exc))
            except (ValueError, OSError, TypeError, KeyError, AttributeError) as exc:
                row.update(status="FAILED", reason=str(exc))
            row["artifactRefs"] = [a.ref for a in self.producer.artifacts[start_artifacts:]]
            row["observationIndices"] = list(range(start_calls, len(self.observations)))
            self.rows.append(row)
        self._finished = True
        return self.finish()

    def finish(self) -> dict[str, Any]:
        require(self._finished, "先执行 suite.run")
        if self._report is not None:
            return copy.deepcopy(self._report)
        # 捕获全部独立边界原文；hash 引用不是原文替代。
        for observer in (self.boundary, self.plan.configured_boundary):
            if isinstance(observer, OwnedLSPForwarder):
                observer.close()
                for index, (instance, direction, chunk) in enumerate(observer.stream_chunks):
                    self.artifact(f"backend-stream-{index}-{direction}", chunk)
                for instance, stderr in observer._stderr_bytes.items():
                    self.artifact("backend-stderr-" + instance, bytes(stderr))
            if observer is not None and hasattr(observer, "events"):
                for event, _ in self.backend(observer):
                    ref = self.artifact("backend-wire", event.raw)
                    framing = self.artifact("backend-framing", event.framing)
                    self.artifact("backend-boundary", bb.encode_json({"instance": event.instance, "direction": event.direction, "source": event.source,
                                                                     "generation": event.generation,
                                                                     "monotonicNs": event.monotonic_ns, "rawRef": ref, "framingRef": framing}))
        transport = self.producer.transport
        if isinstance(transport, bb.StdioJSONRPCClient):
            transport.close()
        if self.plan.configured_transport is not None and self.plan.configured_transport is not transport:
            other = self.plan.configured_transport
            if isinstance(other, bb.StdioJSONRPCClient):
                other.close()
                out, err = other.stream_snapshot()
                self.artifact("configured-stdout", out)
                self.artifact("configured-stderr", err)
                for index, exchange in enumerate(other.exchanges):
                    for key in ("rawRequest", "rawSent", "rawResponse", "partialStdout", "partialStderr"):
                        if exchange[key] is not None:
                            self.artifact(f"configured-{index}-{key}", exchange[key])
                if not other.streams_complete:
                    for row in self.rows:
                        if row["id"] in ("LSP-05", "LSP-11", "LSP-13") and row["status"] == "PASS":
                            row.update(status="NOT_VERIFIED", reason="配置部署原始流不完整")
        report = self.producer.finish()
        if self.execution == "real" and (not report["transportObservation"]["rawWireObserved"] or not report["transportObservation"]["streamsComplete"]):
            for row in self.rows:
                if row["status"] == "PASS":
                    row.update(status="NOT_VERIFIED", reason="decoded-only／partial 原观察不能真实 PASS")
        self._report = {"schemaVersion": "lsp-bridge.prd-suite/v1", **self.producer.identities,
                        "profile": asdict(self.profile), "requirements": self.rows,
                        "observations": self.observations, "rawObservations": report,
                        "acceptanceEstablished": False,
                        "scope": "synthetic-calibration-not-real-acceptance" if self.execution == "synthetic" else "owner-receipt-required"}
        return copy.deepcopy(self._report)


# 此固定 harness 只建立授权 MCP stdio，不实现 LSP。实际10 goroutine同时发起
# 不同查询；stdout仅输出原 request/response bytes base64，suite独立派生断言。
GO_CONCURRENT_HARNESS = '''package main
import (
 "bufio"
 "bytes"
 "encoding/base64"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "sync"
 "time"
)
type Plan struct {
 Argv []string `json:"argv"`
 Cwd string `json:"cwd"`
 Env map[string]string `json:"env"`
 Initialize json.RawMessage `json:"initialize"`
 Workspace json.RawMessage `json:"workspace"`
 Requests []json.RawMessage `json:"requests"`
 TimeoutMs float64 `json:"timeoutMs"`
}
type Message struct {
 JSONRPC string `json:"jsonrpc"`
 ID *int `json:"id"`
 Method string `json:"method"`
 Result json.RawMessage `json:"result"`
 Error json.RawMessage `json:"error"`
}
type Received struct { Wire []byte; At int64 }
func decode(wire []byte) Message {
 var row Message
 if err := json.Unmarshal(wire, &row); err != nil { panic(err) }
 if row.JSONRPC != "2.0" { panic("wrong jsonrpc") }
 return row
}
func main() {
 var plan Plan
 if err := json.NewDecoder(os.Stdin).Decode(&plan); err != nil { panic(err) }
 if len(plan.Argv) == 0 || len(plan.Requests) != 10 || plan.TimeoutMs <= 0 || plan.TimeoutMs > 600000 { panic("invalid exact plan") }
 cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
 cmd.Dir = plan.Cwd
 cmd.Env = []string{}
 for k,v := range plan.Env { cmd.Env = append(cmd.Env, k+"="+v) }
 out, err := cmd.StdinPipe(); if err != nil { panic(err) }
 in, err := cmd.StdoutPipe(); if err != nil { panic(err) }
 cmd.Stderr = os.Stderr
 if err := cmd.Start(); err != nil { panic(err) }
 // 只管理本对象启动的MCP，不signal未知PID或后代。
 defer func() { out.Close(); cmd.Process.Kill(); cmd.Wait() }()
 epoch := time.Now()
 deadline := time.NewTimer(time.Duration(plan.TimeoutMs * float64(time.Millisecond)))
 defer deadline.Stop()
 wires := make(chan Received, 32)
 readerDone := make(chan error, 1)
 go func() {
  scanner := bufio.NewScanner(in)
  scanner.Buffer(make([]byte, 4096), 16*1024*1024)
  for scanner.Scan() {
   wire := append(append([]byte(nil), scanner.Bytes()...), '\\n')
   wires <- Received{wire, time.Since(epoch).Nanoseconds()}
  }
  err := scanner.Err(); if err == nil { err = fmt.Errorf("MCP stdout EOF") }
  readerDone <- err
 }()
 receive := func() Received {
  for {
   select {
   case got := <-wires:
    row := decode(got.Wire)
    if row.Method != "" {
     if row.ID != nil || len(row.Result) != 0 || len(row.Error) != 0 { panic("invalid server notification") }
     continue
    }
    return got
   case err := <-readerDone: panic(err)
   case <-deadline.C: panic("single granted deadline exceeded")
   }
  }
 }
 for _, setup := range []json.RawMessage{plan.Initialize, plan.Workspace} {
  request := decode(setup)
  if request.ID == nil || request.Method == "" { panic("invalid setup request") }
  if _, err := out.Write(append(append([]byte(nil), setup...), '\\n')); err != nil { panic(err) }
  reply := decode(receive().Wire)
  if reply.ID == nil || *reply.ID != *request.ID || len(reply.Result) == 0 || len(reply.Error) != 0 || bytes.Equal(reply.Result, []byte("null")) { panic("setup identity/result mismatch") }
  var result map[string]json.RawMessage
  if err := json.Unmarshal(reply.Result, &result); err != nil || result == nil { panic("setup result not object") }
  if request.Method == "initialize" {
   if len(result["protocolVersion"]) == 0 || len(result["capabilities"]) == 0 { panic("invalid initialize result") }
   if _, err := out.Write([]byte("{\\"jsonrpc\\":\\"2.0\\",\\"method\\":\\"notifications/initialized\\"}\\n")); err != nil { panic(err) }
  } else if bytes.Equal(result["isError"], []byte("true")) { panic("workspace tool error") }
 }
 channels := map[int]chan Received{}
 originals := map[int][]byte{}
 for _, wire := range plan.Requests {
  request := decode(wire)
  if request.ID == nil || request.Method != "tools/call" { panic("invalid query request") }
  if _, ok := channels[*request.ID]; ok { panic("duplicate id") }
  channels[*request.ID] = make(chan Received, 1)
  originals[*request.ID] = wire
 }
 stop := make(chan struct{})
 failure := make(chan error, 1)
 fail := func(err error) { select { case failure <- err: default: } }
 go func() {
  seen := map[int]bool{}
  for len(seen) < 10 {
   select {
   case got := <-wires:
    row := decode(got.Wire)
    if row.Method != "" {
     if row.ID != nil || len(row.Result) != 0 || len(row.Error) != 0 { fail(fmt.Errorf("invalid notification")); return }
     continue
    }
    if row.ID == nil || len(row.Result) == 0 || len(row.Error) != 0 { fail(fmt.Errorf("invalid query response")); return }
    ch, ok := channels[*row.ID]
    if !ok || seen[*row.ID] { fail(fmt.Errorf("unknown/duplicate query response")); return }
    seen[*row.ID] = true
    ch <- got
   case err := <-readerDone: fail(err); return
   case <-stop: return
   }
  }
 }()
 var writeMu, reportMu sync.Mutex
 var wg sync.WaitGroup
 start := make(chan struct{})
 for id, ch := range channels {
  wg.Add(1)
  go func(id int, ch chan Received) {
   defer wg.Done()
   <-start
   wire := append(append([]byte(nil), originals[id]...), '\\n')
   writeMu.Lock()
   if _, err := out.Write(wire); err != nil { writeMu.Unlock(); fail(err); return }
   sent := time.Since(epoch).Nanoseconds()
   writeMu.Unlock()
   select {
   case got := <-ch:
    reportMu.Lock()
    fmt.Printf("{\\"request\\":%q,\\"response\\":%q,\\"sentNs\\":%d,\\"receivedNs\\":%d}\\n", base64.StdEncoding.EncodeToString(wire), base64.StdEncoding.EncodeToString(got.Wire), sent, got.At)
    reportMu.Unlock()
   case <-stop: return
   }
  }(id, ch)
 }
 close(start)
 done := make(chan struct{})
 go func() { wg.Wait(); close(done) }()
 select {
 case <-done:
 case err := <-failure: close(stop); panic(err)
 case <-deadline.C: close(stop); panic("single granted deadline exceeded")
 }
 close(stop)
}
'''
