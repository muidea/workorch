#!/usr/bin/env python3
"""以 PRD 为边界的 LSP-Bridge 黑盒观察器。

仅消费 MCP JSON-RPC 和明确授权的子进程观察，不读取现有实现。
测试使用临时合成 stdio 子进程；PRD 未规定的响应字段由外部 adapter 解释。
观察报告不产生 owner receipt，也不授予正式验收权。
"""
from __future__ import annotations

from dataclasses import asdict, dataclass, field
import copy
import hashlib
import json
import math
import os
import queue
import select
import stat
from pathlib import Path
import re
import subprocess
import tempfile
import threading
import time
from typing import Any, Protocol

SCHEMA_VERSION = "lsp-bridge.blackbox-observations/v1"
TOOL_NAMES = frozenset({
    "lsp_initialize", "lsp_sync", "lsp_discard_unsaved", "lsp_definition", "lsp_hover",
    "lsp_diagnostics", "lsp_references", "lsp_status", "lsp_shutdown", "lsp_repair",
})
REQUIREMENT_IDS = tuple(f"LSP-{index:02d}" for index in range(1, 21))
IDENTITY_KEYS = frozenset({"workspaceId", "runId", "startupIdentity", "deliveryCommit", "serviceSourceIdentity"})


class ProducerError(ValueError):
    pass


class MCPTransport(Protocol):
    def list_tools(self) -> list[dict[str, Any]]: ...
    def call_tool(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]: ...


class ProcessExecutor(Protocol):
    def run(self, grant: "CommandGrant") -> "ProcessObservation": ...


class MeasurementSource(Protocol):
    def measure(self, name: str) -> "MeasurementObservation": ...


class DefinitionResponseAdapter(Protocol):
    """显式部署／Profile adapter；PRD 不规定 wire 字段。"""
    def definition_locations(self, response: dict[str, Any]) -> list[dict[str, Any]] | None: ...


@dataclass(frozen=True)
class CommandGrant:
    """只允许 exact command vector、cwd、environment key set 和有限期限。"""
    argv: tuple[str, ...]
    cwd: str
    env: tuple[tuple[str, str], ...]
    allowed_argv: tuple[tuple[str, ...], ...]
    allowed_env_keys: tuple[str, ...]
    timeout_seconds: float

    def validate(self, delivery: Path) -> None:
        if not self.argv or any(type(arg) is not str or not arg or "\x00" in arg for arg in self.argv):
            raise ProducerError("argv must be non-empty strings without NUL")
        if any(type(row) is not tuple or len(row) != 2 or any(type(value) is not str for value in row) for row in self.env):
            raise ProducerError("explicit environment must contain exact string pairs")
        if any(type(key) is not str for key in self.allowed_env_keys):
            raise ProducerError("explicit environment grant keys must be strings")
        if self.argv not in self.allowed_argv:
            raise ProducerError("argv is not explicitly granted")
        root = _directory_without_symlinks(Path(delivery))
        cwd = _directory_without_symlinks(Path(self.cwd))
        if not cwd.is_dir() or not cwd.is_relative_to(root):
            raise ProducerError("cwd is outside the temporary delivery")
        keys = [key for key, _ in self.env]
        if len(keys) != len(set(keys)) or set(keys) != set(self.allowed_env_keys):
            raise ProducerError("explicit environment keys do not match grant")
        if any(not key or "=" in key or "\x00" in key or "\x00" in value for key, value in self.env):
            raise ProducerError("invalid explicit environment")
        if isinstance(self.timeout_seconds, bool) or not isinstance(self.timeout_seconds, (int, float)) or not math.isfinite(self.timeout_seconds) or self.timeout_seconds <= 0:
            raise ProducerError("timeout must be positive and finite")


@dataclass(frozen=True)
class ProcessObservation:
    argv: tuple[str, ...]
    cwd: str
    exit_code: int | None
    stdout: bytes
    stderr: bytes
    duration_ms: float
    timed_out: bool = False


@dataclass(frozen=True)
class MeasurementObservation:
    name: str
    value: float | int | None
    unit: str
    scope: str
    samples: int
    simultaneous: bool = False


@dataclass
class Probe:
    id: str
    kind: str
    request: dict[str, Any] | None = None
    response: dict[str, Any] | None = None
    error: str | None = None
    partial_stdout_sha256: str | None = None
    partial_stdout_bytes: int = 0
    partial_stderr_sha256: str | None = None
    partial_stderr_bytes: int = 0
    request_wire_ref: str | None = None
    response_wire_ref: str | None = None
    error_wire_ref: str | None = None
    exchange_indices: tuple[int, ...] = ()


@dataclass
class Artifact:
    ref: str
    relative_path: str
    sha256: str
    byte_length: int
    kind: str = "command_output"


class StdioJSONRPCClient:
    """仅为一个 explicitly granted 子进程建立真实 stdio JSON-RPC 传输。"""

    def __init__(self, grant: CommandGrant, delivery: Path):
        grant.validate(delivery)
        self.grant = grant
        self.process = subprocess.Popen(
            list(grant.argv), cwd=grant.cwd, env=dict(grant.env), shell=False,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, bufsize=0,
        )
        assert self.process.stdin is not None
        os.set_blocking(self.process.stdin.fileno(), False)
        self._lines: queue.Queue[bytes | BaseException] = queue.Queue()
        self._stdout = bytearray()
        self._stderr = bytearray()
        self._stdout_lock = threading.Lock()
        self._stderr_lock = threading.Lock()
        self._id = 0
        self._lock = threading.Lock()
        self.closed = False
        self._failed = False
        self.streams_complete = False
        self.reader_failures: list[str] = []
        self._reader_stop = threading.Event()
        self._stdout_eof = False
        self._stderr_eof = False
        self.exchanges: list[dict[str, Any]] = []
        self._out_thread = threading.Thread(target=self._read_stdout, daemon=True)
        self._err_thread = threading.Thread(target=self._read_stderr, daemon=True)
        self._out_thread.start()
        self._err_thread.start()

    def _read_stdout(self) -> None:
        assert self.process.stdout is not None
        try:
            fd = self.process.stdout.fileno()
            os.set_blocking(fd, False)
            pending = bytearray()
            while not self._reader_stop.is_set():
                if not select.select([fd], [], [], 0.05)[0]:
                    continue
                try:
                    chunk = os.read(fd, 4096)
                except BlockingIOError:
                    continue
                if not chunk:
                    self._stdout_eof = True
                    if pending:
                        self._lines.put(bytes(pending))
                    self._lines.put(EOFError("child stdout closed"))
                    return
                with self._stdout_lock:
                    self._stdout.extend(chunk)
                pending.extend(chunk)
                while b"\n" in pending:
                    end = pending.index(b"\n") + 1
                    self._lines.put(bytes(pending[:end]))
                    del pending[:end]
        except Exception as exc:
            self.reader_failures.append(f"stdout: {exc}")
            self._lines.put(exc)

    def _read_stderr(self) -> None:
        assert self.process.stderr is not None
        try:
            fd = self.process.stderr.fileno()
            os.set_blocking(fd, False)
            while not self._reader_stop.is_set():
                if not select.select([fd], [], [], 0.05)[0]:
                    continue
                try:
                    chunk = os.read(fd, 4096)
                except BlockingIOError:
                    continue
                if not chunk:
                    self._stderr_eof = True
                    return
                with self._stderr_lock:
                    self._stderr.extend(chunk)
        except Exception as exc:
            self.reader_failures.append(f"stderr: {exc}")

    def _save_partial(self, exchange: dict[str, Any]) -> None:
        with self._stdout_lock:
            out = bytes(self._stdout)
        with self._stderr_lock:
            err = bytes(self._stderr)
        exchange["partialStdout"] = out
        exchange["partialStderr"] = err

    def request(self, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        with self._lock:
            if self.closed:
                raise ProducerError("transport closed; no new request")
            self._id += 1
            request: dict[str, Any] = {"jsonrpc": "2.0", "id": self._id, "method": method}
            if params is not None:
                request["params"] = params
            raw = (json.dumps(request, ensure_ascii=False, separators=(",", ":")) + "\n").encode("utf-8")
            exchange = self._new_exchange(request, raw)
            try:
                deadline = time.monotonic() + self.grant.timeout_seconds
                self._send(exchange, deadline)
                line = self._lines.get(timeout=max(0, deadline - time.monotonic()))
                self._save_partial(exchange)
                if isinstance(line, BaseException):
                    raise ProducerError(f"partial response unavailable: {line}") from line
                # 在任何 decode/shape/identity 检查前保全收到的原始 bytes。
                exchange["rawResponse"] = line
                if not line.endswith(b"\n"):
                    raise ProducerError("malformed/partial JSON-RPC line: missing newline")
                try:
                    response = _strict_json(line)
                except (UnicodeError, ValueError) as exc:
                    raise ProducerError(f"malformed/partial JSON-RPC line: {exc}") from exc
                exchange["response"] = response
                if not isinstance(response, dict) or response.get("jsonrpc") != "2.0":
                    raise ProducerError("JSON-RPC protocol/version drift")
                if type(response.get("id")) is not int or response["id"] != request["id"]:
                    raise ProducerError(f"JSON-RPC id mismatch expected={request['id']} got={response.get('id')!r}")
                if ("result" in response) == ("error" in response):
                    raise ProducerError("JSON-RPC requires exactly one result/error")
                if "error" in response:
                    raise ProducerError(f"JSON-RPC error: {response['error']!r}")
                if not isinstance(response["result"], dict):
                    raise ProducerError("JSON-RPC result is not an object")
                return response["result"]
            except Exception as exc:
                self._failed = True
                exchange["error"] = (f"timeout method={method} id={self._id}" if isinstance(exc, queue.Empty) else str(exc))
                self._save_partial(exchange)
                raise ProducerError(exchange["error"]) from exc

    def _new_exchange(self, request: dict[str, Any], raw: bytes) -> dict[str, Any]:
        exchange = {"request": request, "rawRequest": raw, "rawSent": b"",
                    "response": None, "rawResponse": None,
                    "partialStdout": b"", "partialStderr": b"", "error": None}
        self.exchanges.append(exchange)
        return exchange

    def _send(self, exchange: dict[str, Any], deadline: float | None = None) -> None:
        if self.closed or self._failed:
            raise ProducerError("transport closed or previous request unresolved; no automatic replay")
        if self.process.poll() is not None:
            raise ProducerError("granted child exited before request")
        assert self.process.stdin is not None
        raw = exchange["rawRequest"]
        if deadline is None:
            deadline = time.monotonic() + self.grant.timeout_seconds
        fd = self.process.stdin.fileno()
        while len(exchange["rawSent"]) < len(raw):
            remaining = deadline - time.monotonic()
            if remaining <= 0 or not select.select([], [fd], [], max(0, remaining))[1]:
                raise ProducerError("stdin timeout; partial write preserved")
            start = len(exchange["rawSent"])
            try:
                count = os.write(fd, raw[start:start + 4096])
            except BlockingIOError:
                continue
            if count <= 0:
                raise ProducerError("stdin write did not complete")
            exchange["rawSent"] += raw[start:start + count]

    def initialize(self) -> dict[str, Any]:
        result = self.request("initialize", {
            "protocolVersion": "2024-11-05", "capabilities": {},
            "clientInfo": {"name": "workorch-lsp-bridge-verifier", "version": "1"},
        })
        if not isinstance(result.get("protocolVersion"), str) or not isinstance(result.get("capabilities"), dict):
            raise ProducerError("MCP initialize result protocolVersion/capabilities malformed")
        with self._lock:
            request = {"jsonrpc": "2.0", "method": "notifications/initialized"}
            raw = b'{"jsonrpc":"2.0","method":"notifications/initialized"}\n'
            exchange = self._new_exchange(request, raw)
            try:
                self._send(exchange)
            except Exception as exc:
                self._failed = True
                exchange["error"] = str(exc)
                raise ProducerError(str(exc)) from exc
            finally:
                self._save_partial(exchange)
        return result

    def list_tools(self) -> list[dict[str, Any]]:
        result = self.request("tools/list", {})
        tools = result.get("tools")
        if not isinstance(tools, list) or any(not isinstance(item, dict) for item in tools):
            raise ProducerError("tools/list result must contain tool object array")
        return tools

    def call_tool(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        # MCP envelope 保持不变；文本业务 payload 由 producer 的独立 adapter 解释。
        return self.request("tools/call", {"name": name, "arguments": arguments})

    def close(self) -> None:
        with self._lock:
            if self.closed:
                return
            self.closed = True
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=min(2.0, self.grant.timeout_seconds))
                except subprocess.TimeoutExpired:
                    # 只终止本对象通过 exact grant 启动的子进程。
                    self.process.kill()
                    self.process.wait()
            deadline = time.monotonic() + min(2.0, self.grant.timeout_seconds)
            for thread in (self._out_thread, self._err_thread):
                thread.join(timeout=max(0, deadline - time.monotonic()))
            if self._out_thread.is_alive() or self._err_thread.is_alive():
                # 不终止未知后代；结束本观察器读线程并明确报告 EOF 未观察完整。
                self._reader_stop.set()
                self._out_thread.join()
                self._err_thread.join()
            self.streams_complete = self._stdout_eof and self._stderr_eof and not self.reader_failures
            if self.exchanges:
                self._save_partial(self.exchanges[-1])
            for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
                if stream is not None:
                    stream.close()

    def stream_snapshot(self) -> tuple[bytes, bytes]:
        if not self.closed:
            raise ProducerError("close granted transport before final observation; streams still in flight")
        with self._stdout_lock, self._stderr_lock:
            return bytes(self._stdout), bytes(self._stderr)


@dataclass
class LSPBridgeProducer:
    transport: MCPTransport
    process_executor: ProcessExecutor
    measurement_source: MeasurementSource
    delivery: Path
    identities: dict[str, str]
    probes: list[Probe] = field(default_factory=list)
    artifacts: list[Artifact] = field(default_factory=list)
    partial_facts: list[dict[str, Any]] = field(default_factory=list)
    exchange_artifacts: list[Artifact] = field(default_factory=list)
    _artifact_directory: Path | None = field(default=None, init=False, repr=False)
    _finalized: bool = field(default=False, init=False, repr=False)
    _transport_observation: dict[str, Any] | None = field(default=None, init=False, repr=False)

    def __post_init__(self) -> None:
        if set(self.identities) != IDENTITY_KEYS or any(not isinstance(value, str) or not value.strip() for value in self.identities.values()):
            raise ProducerError("all exact external identity pins are required")
        self.delivery = _directory_without_symlinks(Path(self.delivery))
        if not self.delivery.is_dir():
            raise ProducerError("delivery must be a directory")
        self.identities = dict(self.identities)

    @classmethod
    def temporary(cls, transport: MCPTransport, process_executor: ProcessExecutor,
                  measurement_source: MeasurementSource, identities: dict[str, str],
                  parent: Path | None = None) -> "LSPBridgeProducer":
        directory = Path(tempfile.mkdtemp(prefix="lsp-prd-probe-", dir=parent))
        return cls(transport, process_executor, measurement_source, directory, identities)

    @classmethod
    def connect_stdio(cls, grant: CommandGrant, delivery: Path, process_executor: ProcessExecutor,
                      measurement_source: MeasurementSource, identities: dict[str, str]) -> tuple["LSPBridgeProducer", StdioJSONRPCClient]:
        transport = StdioJSONRPCClient(grant, delivery)
        try:
            return cls(transport, process_executor, measurement_source, delivery, identities), transport
        except Exception:
            transport.close()
            raise

    def _require_open(self) -> None:
        if self._finalized:
            raise ProducerError("observation finalized; no further probes")

    def inspect_tool_surface(self) -> dict[str, Any]:
        self._require_open()
        probe = Probe("tool-surface", "mcp")
        self.probes.append(probe)
        start = self._exchange_count()
        try:
            raw_tools = self.transport.list_tools()
        except Exception as exc:
            probe.error = str(exc)
            raise
        finally:
            probe.exchange_indices = tuple(range(start, self._exchange_count()))
        probe.response = {"tools": raw_tools}
        if not isinstance(raw_tools, list) or any(not isinstance(item, dict) for item in raw_tools):
            probe.error = "tools/list malformed"
            raise ProducerError(probe.error)
        names: list[str] = []
        observed_schemas: dict[str, Any] = {}
        for item in raw_tools:
            name = item.get("name")
            if not isinstance(name, str) or not name:
                probe.error = "tools/list has missing tool name"
                raise ProducerError(probe.error)
            names.append(name)
            if "inputSchema" in item:
                observed_schemas[name] = item["inputSchema"]
        if len(names) != len(set(names)):
            probe.error = "tools/list duplicate tool names"
            raise ProducerError(probe.error)
        if set(names) != TOOL_NAMES:
            probe.error = f"tool names drift missing={sorted(TOOL_NAMES - set(names))} extra={sorted(set(names) - TOOL_NAMES)}"
            raise ProducerError(probe.error)
        fact = {"expectedNames": sorted(TOOL_NAMES), "observedNames": sorted(names),
                "observedInputSchemas": observed_schemas,
                "schemaJudgement": "observed_only; PRD defines no per-tool input-schema contract"}
        self.partial_facts.append({"requirementId": "LSP-01", "fact": fact,
                                   "status": "VERIFIED_PARTIAL",
                                   "limitation": "PRD also requires documentation; this fact alone cannot pass LSP-01."})
        return fact

    def verify_documented_tool_names(self, relative_documents: list[str]) -> dict[str, Any]:
        """仅检查显式 delivery 文档提及工具名，不发现其它文档或判定内容质量。"""
        self._require_open()
        if not isinstance(relative_documents, list) or not relative_documents:
            raise ProducerError("explicit delivered documentation paths required")
        observed: dict[str, list[str]] = {name: [] for name in TOOL_NAMES}
        hashes: dict[str, str] = {}
        for relative in relative_documents:
            if not isinstance(relative, str) or not relative:
                raise ProducerError("documentation path must be non-empty relative path")
            relative_path = Path(relative)
            if relative_path.is_absolute() or ".." in relative_path.parts:
                raise ProducerError("documentation artifact escapes delivery")
            path = self.delivery / relative_path
            _reject_symlinks(path)
            if not path.is_file():
                raise ProducerError("documentation artifact escapes delivery or is not a file")
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
            with os.fdopen(fd, "rb") as stream:
                before = os.fstat(stream.fileno())
                if not stat.S_ISREG(before.st_mode):
                    raise ProducerError("documentation artifact must be a regular file")
                raw = stream.read()
                after = os.fstat(stream.fileno())
            if before != after or len(raw) != after.st_size:
                raise ProducerError("documentation changed during observation")
            text = raw.decode("utf-8", errors="strict")
            hashes[relative] = _sha256(raw)
            for name in TOOL_NAMES:
                if name in text:
                    observed[name].append(relative)
        missing = sorted(name for name, docs in observed.items() if not docs)
        result = {"documentHashes": hashes, "toolDocumentRefs": observed, "missingToolNames": missing,
                  "allToolNamesMentioned": not missing}
        self.probes.append(Probe("tool-documents", "delivery-artifact", response=result))
        if not missing:
            self.partial_facts.append({"requirementId": "LSP-01", "fact": result,
                                       "status": "VERIFIED_PARTIAL",
                                       "limitation": "Tool-name documentation references do not prove runtime availability or owner-backed receipt."})
        return result

    def observe_tool_call(self, probe_id: str, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        self._require_open()
        request = {"tool": name, "arguments": arguments}
        probe = Probe(probe_id, "mcp", request=request)
        self.probes.append(probe)
        start = self._exchange_count()
        try:
            response = self.transport.call_tool(name, arguments)
            probe.response = response
            if not isinstance(response, dict):
                raise ProducerError("MCP call response must be an object")
            if response.get("isError") is True:
                raise ProducerError(f"tool {name} returned isError=true")
            content = response.get("content")
            if not isinstance(content, list) or len(content) != 1 or not isinstance(content[0], dict) or content[0].get("type") != "text" or not isinstance(content[0].get("text"), str):
                raise ProducerError("tool response must have exactly one text block")
            try:
                payload = _strict_json(content[0]["text"].encode("utf-8", errors="strict"))
            except (UnicodeError, ValueError) as exc:
                raise ProducerError(f"tool {name} text is not JSON: {exc}") from exc
            if not isinstance(payload, dict):
                raise ProducerError("tool text payload must decode as JSON object")
            return payload
        except Exception as exc:
            probe.error = str(exc)
            raise
        finally:
            probe.exchange_indices = tuple(range(start, self._exchange_count()))

    def verify_definition_result(self, payload: dict[str, Any], expected_path: str,
                                 adapter: DefinitionResponseAdapter | None = None) -> dict[str, Any]:
        if adapter is None:
            raise ProducerError("PRD does not define definition response fields; an explicit response adapter is required")
        locations = adapter.definition_locations(payload)
        if locations is None:
            raise ProducerError("definition response adapter cannot interpret this response")
        if not isinstance(locations, list) or not locations:
            raise ProducerError("definition response has no validated locations")
        for row in locations:
            if not isinstance(row, dict) or not isinstance(row.get("path"), str) or not row["path"]:
                raise ProducerError("adapter location missing path")
            if type(row.get("line")) is not int or row["line"] < 0 or type(row.get("column")) is not int or row["column"] < 0:
                raise ProducerError("adapter location missing valid line/column")
        if not any(row["path"] == expected_path for row in locations):
            raise ProducerError("definition response does not match explicit fixture target path")
        return {"locations": locations, "expectedFixturePathMatched": True,
                "adapterRef": type(adapter).__name__}

    def run_child(self, probe_id: str, grant: CommandGrant) -> ProcessObservation:
        self._require_open()
        grant.validate(self.delivery)
        observation = self.process_executor.run(grant)
        if not isinstance(observation, ProcessObservation) or observation.argv != grant.argv or Path(observation.cwd).resolve() != Path(grant.cwd).resolve():
            raise ProducerError("child observation does not match exact grant")
        if observation.exit_code is not None and type(observation.exit_code) is not int:
            raise ProducerError("child exit code malformed")
        if not isinstance(observation.stdout, bytes) or not isinstance(observation.stderr, bytes):
            raise ProducerError("child output must remain raw bytes")
        if isinstance(observation.duration_ms, bool) or not isinstance(observation.duration_ms, (int, float)) or not math.isfinite(observation.duration_ms) or observation.duration_ms < 0 or type(observation.timed_out) is not bool:
            raise ProducerError("child measurement malformed")
        self._artifact(probe_id + "-stdout", observation.stdout)
        self._artifact(probe_id + "-stderr", observation.stderr)
        self.probes.append(Probe(probe_id, "child-process", request={"argv": list(grant.argv), "cwd": grant.cwd},
                                 response={"exitCode": observation.exit_code, "durationMs": observation.duration_ms,
                                           "timedOut": observation.timed_out, "stdoutBytes": len(observation.stdout),
                                           "stderrBytes": len(observation.stderr)}))
        return observation

    def measure(self, probe_id: str, name: str) -> MeasurementObservation:
        self._require_open()
        result = self.measurement_source.measure(name)
        if not isinstance(result, MeasurementObservation) or result.name != name:
            raise ProducerError("measurement observation type/name mismatch")
        if result.value is not None and (isinstance(result.value, bool) or not isinstance(result.value, (int, float)) or not math.isfinite(result.value)):
            raise ProducerError("measurement must be finite numeric or unavailable")
        if type(result.samples) is not int or result.samples < 0 or type(result.simultaneous) is not bool:
            raise ProducerError("measurement sample/simultaneous fields malformed")
        self.probes.append(Probe(probe_id, "measurement", response=asdict(result)))
        return result

    def _wire_artifact(self, ref: str, content: bytes) -> str:
        safe = re.sub(r"[^A-Za-z0-9_.-]", "_", ref)
        existing = {row.ref for row in self.artifacts}
        candidate = safe
        suffix = 2
        while candidate in existing:
            candidate = f"{safe}-{suffix}"
            suffix += 1
        self._artifact(candidate, content)
        artifact = self.artifacts[-1]
        self.exchange_artifacts.append(artifact)
        return artifact.ref

    def _exchange_count(self) -> int:
        if isinstance(self.transport, StdioJSONRPCClient):
            return len(self.transport.exchanges)
        return 0

    def _capture_transport(self) -> dict[str, Any]:
        if self._transport_observation is not None:
            return self._transport_observation
        if not isinstance(self.transport, StdioJSONRPCClient):
            return {"kind": "decoded_only", "rawWireObserved": False, "streamsComplete": False,
                    "reason": "transport supplied decoded values only; no raw wire fabricated", "exchanges": []}
        stdout, stderr = self.transport.stream_snapshot()
        rows = []
        fields = ("rawRequest", "rawSent", "rawResponse", "partialStdout", "partialStderr")
        for index, exchange in enumerate(self.transport.exchanges):
            row = {"index": index, "request": exchange["request"], "error": exchange["error"]}
            for key in fields:
                raw = exchange[key]
                row[key + "Ref"] = None if raw is None else self._wire_artifact(f"exchange-{index}-{key}", raw)
            row["requestWriteComplete"] = exchange["rawRequest"] == exchange["rawSent"]
            rows.append(row)
        observation = {"kind": "stdio", "rawWireObserved": True,
                       "streamsComplete": self.transport.streams_complete,
                       "readerFailures": list(self.transport.reader_failures),
                       "incompleteReason": None if self.transport.streams_complete else "reader failure or stream EOF not observed within granted close bound",
                       "stdoutRef": self._wire_artifact("transport-stdout", stdout),
                       "stderrRef": self._wire_artifact("transport-stderr", stderr),
                       "exchanges": rows}
        for probe in self.probes:
            if probe.exchange_indices:
                row = rows[probe.exchange_indices[-1]]
                probe.request_wire_ref = row["rawRequestRef"]
                probe.response_wire_ref = row["rawResponseRef"]
        self._transport_observation = observation
        return observation

    def _artifact(self, ref: str, content: bytes) -> None:
        safe = re.sub(r"[^A-Za-z0-9_.-]", "_", ref)
        if self._artifact_directory is None:
            self._artifact_directory = Path(tempfile.mkdtemp(prefix=".observations-", dir=self.delivery))
        path = self._artifact_directory / f"{safe}.bin"
        # 完整写入私有临时文件，再以不覆盖的 hardlink 原子发布。
        fd, temporary = tempfile.mkstemp(prefix=".pending-", dir=self._artifact_directory)
        try:
            with os.fdopen(fd, "wb") as stream:
                stream.write(content)
                stream.flush()
                os.fsync(stream.fileno())
            try:
                os.link(temporary, path, follow_symlinks=False)
            except FileExistsError as exc:
                raise ProducerError("artifact ref collision") from exc
            directory_fd = os.open(self._artifact_directory, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory_fd)
            finally:
                os.close(directory_fd)
            self.artifacts.append(Artifact(safe, str(path.relative_to(self.delivery)), _sha256(content), len(content)))
        finally:
            os.unlink(temporary)

    def finish(self) -> dict[str, Any]:
        transport_observation = self._capture_transport()
        self._finalized = True
        requirements = []
        for req in REQUIREMENT_IDS:
            partial = [row for row in self.partial_facts if row["requirementId"] == req]
            requirements.append({"id": req, "status": "NOT_VERIFIED", "partialFacts": partial,
                                 "reason": "PRD requirement is not fully proven by this producer observation."})
        return copy.deepcopy({"schemaVersion": SCHEMA_VERSION, **self.identities,
                "probes": [asdict(item) for item in self.probes],
                "partialFacts": self.partial_facts, "artifacts": [asdict(item) for item in self.artifacts],
                "transportObservation": transport_observation,
                "exchangeArtifacts": [asdict(item) for item in self.exchange_artifacts],
                "requirements": requirements, "acceptanceEstablished": False})


def _reject_symlinks(path: Path) -> None:
    # 先检查未 resolve 的各层；不让 symlink/.. 被规范化后隐去。
    if ".." in path.parts:
        raise ProducerError("parent traversal is not allowed")
    absolute = path.absolute()
    for current in (*reversed(absolute.parents), absolute):
        if current.is_symlink():
            raise ProducerError("symlink path is not allowed")


def _directory_without_symlinks(path: Path) -> Path:
    _reject_symlinks(path)
    directory = path.absolute()
    if not directory.is_dir():
        raise ProducerError("directory does not exist")
    return directory


def _strict_json(raw: bytes) -> Any:
    def pairs(rows: list[tuple[str, Any]]) -> dict[str, Any]:
        result = {}
        for key, value in rows:
            if key in result:
                raise ProducerError(f"duplicate JSON key: {key}")
            result[key] = value
        return result

    def nonfinite(value: str) -> Any:
        raise ProducerError(f"non-finite JSON value: {value}")

    result = json.loads(raw.decode("utf-8", errors="strict"), object_pairs_hook=pairs, parse_constant=nonfinite)

    def validate(value: Any) -> None:
        if isinstance(value, str):
            value.encode("utf-8", errors="strict")
        elif isinstance(value, float) and not math.isfinite(value):
            raise ProducerError("non-finite JSON value")
        elif isinstance(value, dict):
            for key, item in value.items():
                validate(key)
                validate(item)
        elif isinstance(value, list):
            for item in value:
                validate(item)
    validate(result)
    return result



def _sha256(raw: bytes) -> str:
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def encode_json(document: dict[str, Any]) -> bytes:
    return (json.dumps(document, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
