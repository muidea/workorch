#!/usr/bin/env python3
from __future__ import annotations

import concurrent.futures
import contextlib
import json
import os
import re
import shutil
import signal
import statistics
import subprocess
import sys
import tempfile
import textwrap
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


BASE_URL = os.environ.get("WORKORCH_LOAD_BASE_URL", os.environ.get("WORKORCH_BASE_URL", "http://127.0.0.1:8080")).rstrip("/")
LLM_MODE = os.environ.get("WORKORCH_LOAD_LLM_MODE", "mock").strip().lower()
CONCURRENCY = int(os.environ.get("WORKORCH_LOAD_CONCURRENCY", "1"))
TOTAL = int(os.environ.get("WORKORCH_LOAD_TOTAL", "10"))
MOCK_ADDR = os.environ.get("WORKORCH_LOAD_MOCK_ADDR", "127.0.0.1:18191")
PROVIDER_ID = os.environ.get("WORKORCH_LOAD_PROVIDER_ID", "")
BATCH_ID = os.environ.get("WORKORCH_LOAD_BATCH_ID", f"load-{datetime.now(timezone.utc).strftime('%Y%m%d%H%M%S')}")
ROOT_DIR = Path(__file__).resolve().parents[1]
os.chdir(ROOT_DIR)
ARTIFACT_DIR = Path(os.environ.get("WORKORCH_LOAD_ARTIFACT_DIR", tempfile.mkdtemp(prefix=f"workorch-loadtest-{BATCH_ID}-")))
MCP_ENDPOINT_ROOT = Path(os.environ.get("WORKORCH_MCP_ENDPOINT_DIR", ".workorch-data/system/mcp-endpoints"))
KEEP_ARTIFACTS = os.environ.get("WORKORCH_LOAD_KEEP_ARTIFACTS", "1") == "1"
CURL_MAX_TIME = os.environ.get("WORKORCH_LOAD_CURL_MAX_TIME", "300")
POLL_INTERVAL_SECONDS = float(os.environ.get("WORKORCH_LOAD_POLL_INTERVAL_SECONDS", "1"))
STATUS_WAIT_TIMEOUT_SECONDS = float(os.environ.get("WORKORCH_LOAD_STATUS_WAIT_TIMEOUT_SECONDS", "0"))
REAL_RUN_TIMEOUT_SECONDS = float(os.environ.get("WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS", "900"))
ACTIVATION_RUN_TIMEOUT_SECONDS = float(os.environ.get("WORKORCH_LOAD_ACTIVATION_RUN_TIMEOUT_SECONDS", "180"))
REAL_PROVIDER_TIMEOUT = os.environ.get("WORKORCH_LOAD_REAL_PROVIDER_TIMEOUT", "180s").strip()
METRICS_SAMPLE_INTERVAL_SECONDS = float(os.environ.get("WORKORCH_LOAD_METRICS_SAMPLE_INTERVAL_SECONDS", "0.2"))
COUNTRY = os.environ.get("WORKORCH_LOAD_COUNTRY", "中国")
PROVINCE = os.environ.get("WORKORCH_LOAD_PROVINCE", "浙江")
CITY = os.environ.get("WORKORCH_LOAD_CITY", "杭州")
LOOKUP_NAME = os.environ.get("WORKORCH_LOAD_LOOKUP_NAME", "Hangzhou")
EXPECT_ACTIVE_OVERLAP = os.environ.get("WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP", "0") == "1"
MCP_SESSION_POLICY = os.environ.get("WORKORCH_LOAD_MCP_SESSION_POLICY", "transient").strip().lower()
MIN_SUCCESS_RATE = float(os.environ.get("WORKORCH_LOAD_MIN_SUCCESS_RATE", "1.0"))

if LLM_MODE not in {"mock", "real"}:
    raise SystemExit(f"WORKORCH_LOAD_LLM_MODE must be mock or real, got {LLM_MODE!r}")
if CONCURRENCY <= 0 or TOTAL <= 0:
    raise SystemExit("WORKORCH_LOAD_CONCURRENCY and WORKORCH_LOAD_TOTAL must be positive")
if MCP_SESSION_POLICY not in {"transient", "long_lived"}:
    raise SystemExit(f"WORKORCH_LOAD_MCP_SESSION_POLICY must be transient or long_lived, got {MCP_SESSION_POLICY!r}")
if MIN_SUCCESS_RATE < 0 or MIN_SUCCESS_RATE > 1:
    raise SystemExit(f"WORKORCH_LOAD_MIN_SUCCESS_RATE must be between 0 and 1, got {MIN_SUCCESS_RATE!r}")
if POLL_INTERVAL_SECONDS <= 0:
    raise SystemExit(f"WORKORCH_LOAD_POLL_INTERVAL_SECONDS must be positive, got {POLL_INTERVAL_SECONDS!r}")
if STATUS_WAIT_TIMEOUT_SECONDS < 0:
    raise SystemExit(f"WORKORCH_LOAD_STATUS_WAIT_TIMEOUT_SECONDS must not be negative, got {STATUS_WAIT_TIMEOUT_SECONDS!r}")
if REAL_RUN_TIMEOUT_SECONDS <= 0:
    raise SystemExit(f"WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS must be positive, got {REAL_RUN_TIMEOUT_SECONDS!r}")
if ACTIVATION_RUN_TIMEOUT_SECONDS <= 0:
    raise SystemExit(f"WORKORCH_LOAD_ACTIVATION_RUN_TIMEOUT_SECONDS must be positive, got {ACTIVATION_RUN_TIMEOUT_SECONDS!r}")
if METRICS_SAMPLE_INTERVAL_SECONDS <= 0:
    raise SystemExit(f"WORKORCH_LOAD_METRICS_SAMPLE_INTERVAL_SECONDS must be positive, got {METRICS_SAMPLE_INTERVAL_SECONDS!r}")


class HTTPError(RuntimeError):
    def __init__(self, method: str, path: str, curl_code: int, stderr: str, *, http_status: int = 0, body: str = "", elapsed_ms: float = 0):
        status_text = f"HTTP {http_status}" if http_status else f"curl {curl_code}"
        details = stderr.strip() or body_preview(body) or "request failed"
        super().__init__(f"{method} {path} failed with {status_text}: {details}")
        self.method = method
        self.path = path
        self.curl_code = curl_code
        self.http_status = http_status
        self.status = http_status or curl_code
        self.stderr = stderr
        self.body = body
        self.elapsed_ms = elapsed_ms

    def to_dict(self) -> dict[str, Any]:
        return {
            "method": self.method,
            "path": self.path,
            "curlCode": self.curl_code,
            "httpStatus": self.http_status,
            "stderr": self.stderr,
            "body": self.body,
            "bodyPreview": body_preview(self.body),
            "elapsedMs": self.elapsed_ms,
        }


@dataclass
class StageTiming:
    name: str
    started_at: float
    ended_at: float

    @property
    def duration_ms(self) -> float:
        return (self.ended_at - self.started_at) * 1000


@dataclass
class MetricsSample:
    sampled_at: float
    values: dict[str, float]


@dataclass
class WorkspaceResult:
    index: int
    workspace_id: str = ""
    run_id: str = ""
    success: bool = False
    error: str = ""
    timings: list[StageTiming] = field(default_factory=list)
    started_at: float = 0
    ended_at: float = 0
    ticket_ids: list[str] = field(default_factory=list)
    approval_id: str = ""
    step_count: int = 0
    tool_call_count: int = 0
    validation_count: int = 0
    checkpoint_count: int = 0
    result_count: int = 0
    artifact_count: int = 0
    timeline_missing_run_state: int = 0
    llm_step_count: int = 0
    llm_terminal_step_count: int = 0
    llm_non_terminal_step_count: int = 0
    workspace_state: str = ""
    run_state: str = ""
    run_status_state: str = ""
    progress_state: str = ""
    progress_terminal: bool = False
    llm_usage_trace_count: int = 0
    llm_usage_summary_entry_total: int = 0
    llm_usage_provider_native_entries: int = 0
    llm_usage_estimated_entries: int = 0
    llm_protocol_errors: list[str] = field(default_factory=list)
    http_status: int = 0
    curl_code: int = 0
    failure_class: str = ""

    @property
    def e2e_ms(self) -> float:
        if not self.started_at or not self.ended_at:
            return 0
        return (self.ended_at - self.started_at) * 1000

    def timing_ms(self, name: str) -> float:
        values = [item.duration_ms for item in self.timings if item.name == name]
        return sum(values)


class MockLLMServer:
    def __init__(self, addr: str, mcp_server_id: str):
        self.addr = addr
        self.mcp_server_id = mcp_server_id
        self.call_counts: dict[str, int] = {}
        self.lock = threading.Lock()
        self.httpd: ThreadingHTTPServer | None = None
        self.thread: threading.Thread | None = None

    def start(self) -> None:
        host, port_text = self.addr.split(":", 1)
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                if self.path != "/v1/chat/completions":
                    self.send_response(404)
                    self.end_headers()
                    return
                size = int(self.headers.get("Content-Length", "0"))
                payload = json.loads(self.rfile.read(size) or b"{}")
                request = outer.parse_provider_request(payload)
                decision = outer.decision_payload(request)
                response = {
                    "choices": [{"message": {"content": json.dumps(decision, ensure_ascii=False, separators=(",", ":"))}}],
                    "usage": {"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
                }
                encoded = json.dumps(response, ensure_ascii=False).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)

            def log_message(self, fmt: str, *args: Any) -> None:
                return

        self.httpd = ThreadingHTTPServer((host, int(port_text)), Handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()
        self.wait_ready()

    def stop(self) -> None:
        if self.httpd:
            self.httpd.shutdown()
            self.httpd.server_close()
        if self.thread:
            self.thread.join(timeout=2)

    def wait_ready(self) -> None:
        deadline = time.time() + 10
        while time.time() < deadline:
            try:
                run_curl("POST", "/chat/completions", {"messages": [{"role": "user", "content": "{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}, base_url=f"http://{self.addr}/v1")
                return
            except Exception:
                time.sleep(0.1)
        raise RuntimeError(f"mock LLM provider did not become ready at {self.addr}")

    @staticmethod
    def parse_provider_request(payload: dict[str, Any]) -> dict[str, Any]:
        messages = payload.get("messages") or [{}]
        content = (messages[-1] or {}).get("content", "")
        all_content = "\n".join(str((message or {}).get("content") or "") for message in messages)
        try:
            request = json.loads(content)
        except Exception:
            request = {}
        request["raw"] = all_content or content
        if not request.get("runId"):
            match = re.search(r'"runId"\s*:\s*"([^"]+)"', all_content)
            if match:
                request["runId"] = match.group(1)
        if not request.get("workspaceId"):
            match = re.search(r'"workspaceId"\s*:\s*"([^"]+)"', all_content)
            if match:
                request["workspaceId"] = match.group(1)
        if not request.get("stepId"):
            match = re.search(r'"stepId"\s*:\s*"([^"]+)"', all_content)
            if match:
                request["stepId"] = match.group(1)
        return request

    def next_call(self, workspace_id: str, run_id: str) -> int:
        key = f"{workspace_id or 'unknown'}:{run_id}"
        with self.lock:
            next_value = self.call_counts.get(key, 0) + 1
            self.call_counts[key] = next_value
            return next_value

    def decision_payload(self, request: dict[str, Any]) -> dict[str, Any]:
        workspace_id = request.get("workspaceId") or "unknown"
        run_id = request.get("runId") or "unknown"
        step_id = request.get("stepId") or ""
        step_input = request.get("input") or {}
        raw = request.get("raw", "")
        if run_id == "probe":
            return {"operations": []}

        call = self.next_call(workspace_id, run_id)
        required_state = extract_json_string(raw, "requiredState")
        focus_id = extract_runtime_focus_id(raw) or "focus-001"
        coverage_id = extract_runtime_coverage_id(raw) or "cwi-001"
        binding_ref = extract_runtime_binding_ref(raw) or f"coverage-binding/{focus_id}/{coverage_id}"

        # Runtime executable candidates have one wire shape: an operations
        # array.  The operation itself is selected from the activation-sealed
        # focus/binding; provider prose and lifecycle fields are deliberately
        # not accepted by the candidate decoder.
        if required_state in {"need_state_change", "need_validation"} or (not required_state and call == 1):
            return {
                "operations": [
                    {
                        "kind": "mcp/tool_call",
                        "bindingRefs": [binding_ref],
                        "parameters": [
                            {"name": "serverId", "string": self.mcp_server_id},
                            {"name": "tool", "string": "loadtest.governed_result"},
                            {"name": "input.country", "string": COUNTRY},
                            {"name": "input.province", "string": PROVINCE},
                            {"name": "input.city", "string": CITY},
                            {"name": "input.lookupName", "string": LOOKUP_NAME},
                            {"name": "input.runId", "string": run_id},
                        ],
                    }
                ]
            }

        # Assessment is also a strict Runtime DTO.  Evidence references
        # are producer observations; the Orchestrator independently derives
        # canonical coverage from the persisted MCP owner receipt.
        evidence_refs = extract_runtime_evidence_refs(raw)
        if not evidence_refs:
            evidence_refs = [f"runtime-observation/{run_id}/loadtest-tool"]
        if required_state == "need_assessment" or call >= 2:
            return {
                "focusId": focus_id,
                "coverageWorkItemIds": [coverage_id],
                "disposition": "satisfied",
                "items": [
                    {"coverageWorkItemId": coverage_id, "satisfied": True, "evidenceRefs": evidence_refs},
                ],
                "summary": f"{PROVINCE}{CITY} governed MCP behavior was observed for the sealed load-test binding.",
                "evidenceRefs": evidence_refs,
            }

        # A provider request without a recognizable state is still kept within
        # the strict operation envelope so a fixture cannot accidentally
        # reintroduce the removed legacy candidate protocol.
        return {
            "operations": [
                {
                    "kind": "mcp/tool_call",
                    "bindingRefs": [binding_ref],
                    "parameters": [
                        {"name": "serverId", "string": self.mcp_server_id},
                        {"name": "tool", "string": "loadtest.governed_result"},
                        {"name": "input.country", "string": COUNTRY},
                        {"name": "input.province", "string": PROVINCE},
                        {"name": "input.city", "string": CITY},
                        {"name": "input.lookupName", "string": LOOKUP_NAME},
                        {"name": "input.runId", "string": run_id},
                    ],
                }
            ]
        }


def extract_json_string(raw: str, key: str) -> str:
    match = re.search(rf'"{re.escape(key)}"\s*:\s*"([^"]+)"', raw)
    return match.group(1) if match else ""


def extract_runtime_focus_id(raw: str) -> str:
    """Read the sealed focus identity from the provider-facing authority."""
    value = extract_json_string(raw, "focusId")
    if value:
        return value
    match = re.search(r'"id"\s*:\s*"(focus-[^"]+)"', raw)
    return match.group(1) if match else ""


def extract_runtime_coverage_id(raw: str) -> str:
    """Read the single CWI in this one-item load-test focus."""
    match = re.search(r'"coverageWorkItemIds"\s*:\s*\[\s*"([^"]+)"', raw)
    if match:
        return match.group(1)
    match = re.search(r'"coverageWorkItemId"\s*:\s*"([^"]+)"', raw)
    return match.group(1) if match else ""


def extract_runtime_binding_ref(raw: str) -> str:
    """Read the activation-sealed coverage binding, never inventing a CWI ref."""
    match = re.search(r'coverage-binding/[^"\\\s]+', raw)
    return match.group(0) if match else ""


def extract_runtime_evidence_refs(raw: str) -> list[str]:
    """Read bounded accepted evidence refs for the assessment observation."""
    refs = re.findall(r'runtime-operation\|[^"\\\s]+', raw)
    if refs:
        return list(dict.fromkeys(refs))
    match = re.search(r'"allowedAcceptedEvidenceRefs"\s*:\s*\[([^\]]*)\]', raw)
    if not match:
        return []
    return list(dict.fromkeys(re.findall(r'"([^"]+)"', match.group(1))))


def body_preview(body: str, limit: int = 1000) -> str:
    text = (body or "").strip()
    if len(text) <= limit:
        return text
    return text[:limit] + "...<truncated>"


def run_curl(method: str, path: str, body: Any | None = None, *, base_url: str = BASE_URL) -> Any:
    url = f"{base_url.rstrip('/')}{path}"
    cmd = ["curl", "-sS", "--max-time", CURL_MAX_TIME, "-X", method, "-H", "Content-Type: application/json", "-w", "\n%{http_code}", url]
    if body is not None:
        cmd.extend(["-d", json.dumps(body, ensure_ascii=False, separators=(",", ":"))])
    started = time.monotonic()
    completed = subprocess.run(cmd, text=True, capture_output=True)
    elapsed_ms = (time.monotonic() - started) * 1000
    response_body, http_status = split_curl_response(completed.stdout)
    if completed.returncode != 0:
        raise HTTPError(method, path, completed.returncode, completed.stderr, http_status=http_status, body=response_body, elapsed_ms=elapsed_ms)
    if http_status >= 400:
        raise HTTPError(method, path, completed.returncode, completed.stderr, http_status=http_status, body=response_body, elapsed_ms=elapsed_ms)
    if not response_body.strip():
        return {}
    return json.loads(response_body)


def run_curl_text(path: str) -> str:
    cmd = ["curl", "-sS", "--max-time", CURL_MAX_TIME, "-w", "\n%{http_code}", f"{BASE_URL}{path}"]
    started = time.monotonic()
    completed = subprocess.run(cmd, text=True, capture_output=True)
    elapsed_ms = (time.monotonic() - started) * 1000
    response_body, http_status = split_curl_response(completed.stdout)
    if completed.returncode != 0:
        raise HTTPError("GET", path, completed.returncode, completed.stderr, http_status=http_status, body=response_body, elapsed_ms=elapsed_ms)
    if http_status >= 400:
        raise HTTPError("GET", path, completed.returncode, completed.stderr, http_status=http_status, body=response_body, elapsed_ms=elapsed_ms)
    return response_body


def split_curl_response(stdout: str) -> tuple[str, int]:
    if not stdout:
        return "", 0
    body, sep, status_text = stdout.rpartition("\n")
    if not sep:
        return stdout, 0
    try:
        return body, int(status_text.strip())
    except ValueError:
        return stdout, 0


def url_quote(value: str) -> str:
    from urllib.parse import quote

    return quote(value, safe="")


def nested(data: dict[str, Any], path: str, default: Any = None) -> Any:
    current: Any = data
    for part in path.split("."):
        if not isinstance(current, dict) or part not in current:
            return default
        current = current[part]
    return current


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_text(path: Path, data: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(data, encoding="utf-8")


@contextlib.contextmanager
def timed(result: WorkspaceResult, name: str):
    started = time.monotonic()
    try:
        yield
    finally:
        result.timings.append(StageTiming(name=name, started_at=started, ended_at=time.monotonic()))


def extract_run_status(response: dict[str, Any]) -> dict[str, Any]:
    return response.get("status") or response.get("run") or nested(response, "activation.run", {}) or {}


def wait_activation_run(workspace_id: str, activation: dict[str, Any], case_dir: Path) -> dict[str, Any]:
    run = nested(activation, "activation.run", {}) or activation.get("run") or {}
    if run.get("id"):
        return run

    deadline = time.monotonic() + ACTIVATION_RUN_TIMEOUT_SECONDS
    last_snapshot: dict[str, Any] = {}
    last_runs: dict[str, Any] = {}
    while time.monotonic() <= deadline:
        try:
            snapshot = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(workspace_id)}/snapshot")
            last_snapshot = snapshot
            snap = snapshot.get("snapshot") or {}
            run = snap.get("run") or {}
            if run.get("id"):
                write_json(case_dir / "activation-run-snapshot.json", snapshot)
                return run
            active_run_id = nested(snapshot, "snapshot.workspace.activeRunId", "")
            if active_run_id:
                runs_response = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(workspace_id)}/runs")
                last_runs = runs_response
                for item in runs_response.get("runs") or []:
                    if item.get("id") == active_run_id:
                        write_json(case_dir / "activation-runs.json", runs_response)
                        return item
        except Exception as exc:
            last_snapshot = {"error": str(exc)}
        time.sleep(POLL_INTERVAL_SECONDS)

    write_json(case_dir / "activation-run-last-snapshot.json", last_snapshot)
    if last_runs:
        write_json(case_dir / "activation-run-last-runs.json", last_runs)
    raise RuntimeError(f"activation accepted but run did not materialize within {ACTIVATION_RUN_TIMEOUT_SECONDS}s")


def wait_run_state(workspace_id: str, run_id: str, expected_state: str, *, timeout_seconds: float = 180, interval_seconds: float = POLL_INTERVAL_SECONDS) -> dict[str, Any]:
    return wait_run_status(
        workspace_id,
        run_id,
        lambda status: status.get("state") == expected_state,
        f"state {expected_state}",
        wait_state=expected_state,
        timeout_seconds=timeout_seconds,
        interval_seconds=interval_seconds,
    )


def wait_run_status(
    workspace_id: str,
    run_id: str,
    predicate,
    description: str,
    *,
    wait_state: str = "",
    timeout_seconds: float = 180,
    interval_seconds: float = POLL_INTERVAL_SECONDS,
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_seconds
    last_status: dict[str, Any] = {}
    while time.monotonic() <= deadline:
        path = f"/api/v1/workorch/workspaces/{url_quote(workspace_id)}/runs/{url_quote(run_id)}/status"
        if wait_state and STATUS_WAIT_TIMEOUT_SECONDS > 0:
            path += (
                f"?waitState={url_quote(wait_state)}"
                f"&waitTimeoutMillis={int(STATUS_WAIT_TIMEOUT_SECONDS * 1000)}"
                f"&waitIntervalMillis={max(1, int(interval_seconds * 1000))}"
            )
        response = run_curl("GET", path)
        last_status = extract_run_status(response)
        if predicate(last_status):
            return response
        time.sleep(interval_seconds)
    raise RuntimeError(f"run {run_id} status={last_status}, want {description}")


LOADTEST_MODEL_ID = "verify-model"


def custom_providers_payload(
    items: list[dict[str, Any]] | dict[str, Any],
    *,
    include_api_key: bool = False,
    mock_base_url: str = "",
    timeout_override: str | None = None,
) -> dict[str, Any]:
    """Build the new customProviders update map from configuration read model."""
    payload: dict[str, Any] = {}
    if isinstance(items, dict):
        iterable = items.items()
        for provider_id, item in iterable:
            provider_id = str(provider_id or "").strip()
            if not provider_id:
                continue
            payload[provider_id] = _custom_provider_entry(
                item if isinstance(item, dict) else {},
                include_api_key=include_api_key,
                mock_base_url=mock_base_url,
                timeout_override=timeout_override,
            )
        return payload
    for item in items or []:
        if not isinstance(item, dict):
            continue
        provider_id = str(item.get("id") or "").strip()
        if not provider_id:
            continue
        payload[provider_id] = _custom_provider_entry(
            item,
            include_api_key=include_api_key,
            mock_base_url=mock_base_url,
            timeout_override=timeout_override,
        )
    return payload


def _custom_provider_entry(
    item: dict[str, Any],
    *,
    include_api_key: bool,
    mock_base_url: str,
    timeout_override: str | None,
) -> dict[str, Any]:
    models: dict[str, Any] = {}
    raw_models = item.get("models") or {}
    if isinstance(raw_models, dict):
        model_iter = raw_models.items()
        for model_id, model in model_iter:
            model_id = str(model_id or "").strip()
            if not model_id or not isinstance(model, dict):
                continue
            models[model_id] = {
                "contextWindowTokens": int(model.get("contextWindowTokens") or 128000),
                "maxOutputTokens": int(model.get("maxOutputTokens") or 8192),
                "operations": list(model.get("operations") or ["chat_completions"]),
            }
    else:
        for model in raw_models if isinstance(raw_models, list) else []:
            if not isinstance(model, dict):
                continue
            model_id = str(model.get("id") or "").strip()
            if not model_id:
                continue
            models[model_id] = {
                "contextWindowTokens": int(model.get("contextWindowTokens") or 128000),
                "maxOutputTokens": int(model.get("maxOutputTokens") or 8192),
                "operations": list(model.get("operations") or ["chat_completions"]),
            }
    if not models:
        models[LOADTEST_MODEL_ID] = {
            "contextWindowTokens": 128000,
            "maxOutputTokens": 8192,
            "operations": ["chat_completions", "embeddings"],
        }
    entry: dict[str, Any] = {
        "provider": item.get("provider") or "compatible",
        "apiSurface": item.get("apiSurface") or "openai_compatible_http",
        "baseURL": item.get("baseUrl") or item.get("baseURL") or mock_base_url,
        "timeout": timeout_override or item.get("timeout") or "60s",
        "models": models,
    }
    if include_api_key:
        entry["apiKey"] = item.get("apiKey") or "loadtest-secret"
    return entry


def snapshot_llm_update_payload(llm: dict[str, Any], *, mock_base_url: str = "") -> dict[str, Any]:
    """Capture a restore-safe LLM update payload using the new contract only."""
    ai_proxy = llm.get("aiProxy") or {}
    default_chat = llm.get("defaultChat") or {}
    default_embedding = llm.get("defaultEmbedding") or {}
    budget = llm.get("budget") or {}
    custom_providers = llm.get("customProviders") or []
    payload: dict[str, Any] = {
        "aiProxy": {
            "baseURL": ai_proxy.get("baseUrl") or ai_proxy.get("baseURL") or mock_base_url or "http://127.0.0.1:8080",
            "timeout": ai_proxy.get("timeout") or "180s",
            "catalogRequestTimeout": ai_proxy.get("catalogRequestTimeout") or "5s",
            "catalogTTL": ai_proxy.get("catalogTTL") or "5m",
            "catalogMaxStaleAge": ai_proxy.get("catalogMaxStaleAge") or "24h",
            "maxCatalogResponseBytes": int(ai_proxy.get("maxCatalogResponseBytes") or 1048576),
            "maxCatalogModels": int(ai_proxy.get("maxCatalogModels") or 1000),
        },
        "defaultChat": {
            "providerID": default_chat.get("providerId") or default_chat.get("providerID") or "ai-proxy",
            "modelID": default_chat.get("modelId") or default_chat.get("modelID") or "gpt-4o",
        },
        "budget": {
            "contextUtilizationRatio": float(budget.get("contextUtilizationRatio") or 0.75),
            "estimatedTokensPerByte": float(budget.get("estimatedTokensPerByte") or 0.25),
        },
        "customProviders": custom_providers_payload(custom_providers, include_api_key=False, mock_base_url=mock_base_url),
    }
    emb_model = default_embedding.get("modelId") or default_embedding.get("modelID")
    if emb_model:
        payload["defaultEmbedding"] = {
            "providerID": default_embedding.get("providerId") or default_embedding.get("providerID") or "ai-proxy",
            "modelID": emb_model,
        }
    return payload


def configure_mock_provider(provider_id: str) -> dict[str, Any]:
    config = run_curl("GET", "/api/v1/workorch/configuration")
    llm = nested(config, "configuration.llm", {})
    mock_base_url = f"http://{MOCK_ADDR}/v1"
    original = snapshot_llm_update_payload(llm, mock_base_url=mock_base_url)
    # Ensure restore has the loadtest custom provider shell without secret.
    if provider_id not in original["customProviders"]:
        original["customProviders"][provider_id] = {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "timeout": "60s",
            "models": {
                LOADTEST_MODEL_ID: {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        }
        original["defaultChat"] = {"providerID": provider_id, "modelID": LOADTEST_MODEL_ID}
        original["defaultEmbedding"] = {"providerID": provider_id, "modelID": LOADTEST_MODEL_ID}

    update = {
        "aiProxy": {
            "baseURL": mock_base_url,
            "apiKey": "loadtest-secret",
            "timeout": "180s",
            "catalogRequestTimeout": "5s",
            "catalogTTL": "5m",
            "catalogMaxStaleAge": "24h",
            "maxCatalogResponseBytes": 1048576,
            "maxCatalogModels": 1000,
            "modelOverrides": {
                LOADTEST_MODEL_ID: {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        },
        "defaultChat": {"providerID": provider_id, "modelID": LOADTEST_MODEL_ID},
        "defaultEmbedding": {"providerID": provider_id, "modelID": LOADTEST_MODEL_ID},
        "budget": original["budget"],
        "customProviders": {
            provider_id: {
                "provider": "compatible",
                "apiSurface": "openai_compatible_http",
                "baseURL": mock_base_url,
                "apiKey": "loadtest-secret",
                "timeout": "60s",
                "models": {
                    LOADTEST_MODEL_ID: {
                        "contextWindowTokens": 128000,
                        "maxOutputTokens": 8192,
                        "operations": ["chat_completions", "embeddings"],
                    }
                },
            }
        },
    }
    write_json(ARTIFACT_DIR / "configuration-original-llm.json", original)
    write_json(ARTIFACT_DIR / "configuration-loadtest-llm.json", update)
    run_curl("PUT", "/api/v1/workorch/configuration/llm", update)
    return original


def restore_llm_configuration(original: dict[str, Any] | None) -> None:
    if not original:
        return
    try:
        run_curl("PUT", "/api/v1/workorch/configuration/llm", original)
    except Exception as exc:
        print(f"failed to restore LLM configuration: {exc}", file=sys.stderr)


def select_real_provider() -> str:
    config = run_curl("GET", "/api/v1/workorch/configuration")
    write_json(ARTIFACT_DIR / "configuration-real.json", config)
    requested = PROVIDER_ID.strip()
    llm = nested(config, "configuration.llm", {})
    default_chat = llm.get("defaultChat") or {}
    provider_id = requested or default_chat.get("providerId") or default_chat.get("providerID") or ""
    if not provider_id:
        raise RuntimeError("no defaultChat provider configured")
    if provider_id == "ai-proxy":
        ai_proxy = llm.get("aiProxy") or {}
        if not ai_proxy.get("apiKeyConfigured") and not (ai_proxy.get("baseUrl") or ai_proxy.get("baseURL")):
            raise RuntimeError("ai-proxy is not configured")
        return provider_id
    customs = {item.get("id"): item for item in (llm.get("customProviders") or []) if isinstance(item, dict)}
    provider = customs.get(provider_id)
    if not provider:
        raise RuntimeError(f"custom provider {provider_id!r} is not configured")
    if not provider.get("apiKeyConfigured"):
        raise RuntimeError(f"custom provider {provider_id!r} has no configured API key")
    models = provider.get("models") or []
    has_chat = False
    for model in models:
        if not isinstance(model, dict):
            continue
        ops = model.get("operations") or []
        if "chat_completions" in ops or not ops:
            has_chat = True
            break
    if not has_chat:
        raise RuntimeError(f"custom provider {provider_id!r} has no chat_completions model")
    return provider_id


def configure_real_provider_timeout(provider_id: str) -> dict[str, Any] | None:
    if not REAL_PROVIDER_TIMEOUT:
        return None
    config = run_curl("GET", "/api/v1/workorch/configuration")
    llm = nested(config, "configuration.llm", {})
    original = snapshot_llm_update_payload(llm)
    updated = json.loads(json.dumps(original, ensure_ascii=False))
    if provider_id == "ai-proxy":
        updated["aiProxy"]["timeout"] = REAL_PROVIDER_TIMEOUT
    else:
        customs = updated.get("customProviders") or {}
        if provider_id not in customs:
            raise RuntimeError(f"provider {provider_id!r} is not present while applying real provider timeout")
        customs[provider_id]["timeout"] = REAL_PROVIDER_TIMEOUT
        updated["customProviders"] = customs
    write_json(ARTIFACT_DIR / "configuration-original-llm.json", original)
    write_json(ARTIFACT_DIR / "configuration-real-timeout-llm.json", updated)
    run_curl("PUT", "/api/v1/workorch/configuration/llm", updated)
    return original


def create_mcp_helper(path: Path) -> None:
    path.write_text(
        textwrap.dedent(
            f"""\
            #!/usr/bin/env python3
            import datetime as dt
            import json
            import sys

            def response(request_id, result=None, error=None):
                payload = {{"jsonrpc": "2.0", "id": request_id}}
                if error is not None:
                    payload["error"] = error
                else:
                    payload["result"] = result
                print(json.dumps(payload, ensure_ascii=False), flush=True)

            def governed_result(arguments):
                country = arguments.get("country") or "{COUNTRY}"
                province = arguments.get("province") or "{PROVINCE}"
                city = arguments.get("city") or "{CITY}"
                run_id = arguments.get("runId") or ""
                observed_at = dt.datetime.now(dt.timezone.utc).isoformat()
                structured = {{
                    "query": {{"country": country, "province": province, "city": city, "lookupName": arguments.get("lookupName") or "{LOOKUP_NAME}"}},
                    "current": {{
                        "observedAt": observed_at,
                        "temperatureC": 20.0,
                        "humidityPercent": 60.0,
                        "windSpeedKmh": 8.0,
                        "weatherDescription": "压测晴",
                    }},
                    "source": {{"name": "workorch-loadtest-mcp", "runId": run_id}},
                }}
                text = f"{{province}}{{city}}治理压测结果：压测晴，气温 20.0°C，湿度 60.0%，风速 8.0 km/h。"
                return {{"content": [{{"type": "text", "text": text}}], "structuredContent": structured, "isError": False}}

            for line in sys.stdin:
                if not line.strip():
                    continue
                request = {{}}
                try:
                    request = json.loads(line)
                    method = request.get("method")
                    request_id = request.get("id")
                    if method == "initialize":
                        response(request_id, {{
                            "protocolVersion": request.get("params", {{}}).get("protocolVersion", "2025-11-25"),
                            "capabilities": {{"tools": {{}}}},
                            "serverInfo": {{"name": "workorch-loadtest-mcp", "version": "test"}},
                        }})
                    elif method == "notifications/initialized":
                        continue
                    elif method == "tools/list":
                        response(request_id, {{"tools": [{{
                            "name": "loadtest.governed_result",
                            "description": "Return deterministic governed load test evidence.",
                            "inputSchema": {{
                                "type": "object",
                                "properties": {{
                                    "country": {{"type": "string"}},
                                    "province": {{"type": "string"}},
                                    "city": {{"type": "string"}},
                                    "lookupName": {{"type": "string"}},
                                    "runId": {{"type": "string"}},
                                }},
                                "required": ["city"],
                            }},
                        }}]}})
                    elif method == "tools/call":
                        params = request.get("params", {{}})
                        if params.get("name") != "loadtest.governed_result":
                            raise RuntimeError(f"unsupported tool {{params.get('name')!r}}")
                        response(request_id, governed_result(params.get("arguments") or {{}}))
                    else:
                        response(request_id, error={{"code": -32601, "message": "method not found"}})
                except Exception as exc:
                    response(request.get("id"), error={{"code": -32000, "message": str(exc)}})
            """
        ),
        encoding="utf-8",
    )
    path.chmod(0o755)


def create_payloads(seq: int, template_id: str) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    workspace = {
        "title": f"workspace concurrency loadtest {BATCH_ID}-{seq:04d}",
        "description": "Workspace concurrency load test with activation-sealed MCP behavior evidence and final Result.",
        "templateId": template_id,
    }
    goal = {
        "text": "- MCP loadtest.governed_result evidence is collected"
    }
    dod = {
        "items": [
            "MCP loadtest.governed_result evidence is collected",
        ]
    }
    return workspace, goal, dod


def resolve_decision(workspace_id: str, ticket_id: str, payload: dict[str, Any]) -> dict[str, Any]:
    return run_curl(
        "POST",
        f"/api/v1/workorch/decisions/{url_quote(ticket_id)}/resolve?workspaceId={url_quote(workspace_id)}",
        {"resolutionType": "provide_missing_info", "resolutionPayload": payload},
    )


def is_repair_exhaustion_ticket(ticket_id: str) -> bool:
    normalized = ticket_id.strip().lower()
    return (
        "repair-exhausted" in normalized
        or "repair_exhausted" in normalized
        or "provider-retry-exhausted" in normalized
        or "provider_retry_exhausted" in normalized
    )


def approve_blocking_approval(workspace_id: str, approval_id: str) -> dict[str, Any]:
    return run_curl("POST", f"/api/v1/workorch/approvals/{url_quote(approval_id)}/approve?workspaceId={url_quote(workspace_id)}")


def real_decision_payload(decision_round: int) -> dict[str, Any]:
    return {
        "operator": "loadtest",
        "country": COUNTRY,
        "province": PROVINCE,
        "city": CITY,
        "lookupName": LOOKUP_NAME,
        "outputFormat": "structured_summary",
        "riskAccepted": True,
        "decisionRound": decision_round,
        "allRequiredInformationProvided": True,
        "allowMCPToolExecution": True,
        "allowRepositoryReadOnlyTools": True,
        "nextExpectedState": "continue",
    }


def drive_real_run_until_completed(result: WorkspaceResult, case_dir: Path) -> dict[str, Any]:
    if not result.workspace_id or not result.run_id:
        raise RuntimeError("workspace id and run id are required before real run drive")
    deadline = time.monotonic() + REAL_RUN_TIMEOUT_SECONDS
    seen_tickets: set[str] = set()
    seen_approvals: set[str] = set()
    gate_index = 0
    last_status: dict[str, Any] = {}
    while time.monotonic() <= deadline:
        status = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/status")
        last_status = extract_run_status(status)
        state = last_status.get("state")
        if state == "completed":
            write_json(case_dir / f"real-gate-{gate_index:03d}-completed-status.json", status)
            return status
        if state in {"failed", "canceled", "cancelled", "superseded"}:
            raise RuntimeError(f"real run reached terminal non-success state: {last_status}")
        if state == "blocked_on_decision":
            ticket_id = last_status.get("blockingTicketId", "")
            if not ticket_id:
                raise RuntimeError(f"real decision gate missing blockingTicketId: {last_status}")
            if is_repair_exhaustion_ticket(ticket_id):
                write_json(case_dir / f"real-repair-exhausted-{gate_index + 1:03d}-status.json", status)
                raise RuntimeError(f"real run exhausted runtime repair budget: {last_status}")
            if ticket_id not in seen_tickets:
                gate_index += 1
                seen_tickets.add(ticket_id)
                result.ticket_ids.append(ticket_id)
                write_json(case_dir / f"real-decision-{gate_index:03d}-status.json", status)
                with timed(result, "real_decision_resolve"):
                    decision = resolve_decision(result.workspace_id, ticket_id, real_decision_payload(len(result.ticket_ids)))
                write_json(case_dir / f"real-decision-{gate_index:03d}-resolved.json", decision)
            time.sleep(POLL_INTERVAL_SECONDS)
            continue
        if state == "blocked_on_approval":
            approval_id = last_status.get("blockingApprovalId", "")
            if not approval_id:
                raise RuntimeError(f"real approval gate missing blockingApprovalId: {last_status}")
            if approval_id not in seen_approvals:
                gate_index += 1
                seen_approvals.add(approval_id)
                if not result.approval_id:
                    result.approval_id = approval_id
                write_json(case_dir / f"real-approval-{gate_index:03d}-status.json", status)
                with timed(result, "real_approval_approve"):
                    approval = approve_blocking_approval(result.workspace_id, approval_id)
                write_json(case_dir / f"real-approval-{gate_index:03d}-approved.json", approval)
                if nested(approval, "approval.state", "") != "approved":
                    raise RuntimeError(f"real approval state={nested(approval, 'approval.state', None)!r}, want approved")
            time.sleep(POLL_INTERVAL_SECONDS)
            continue
        time.sleep(POLL_INTERVAL_SECONDS)
    raise RuntimeError(f"real run did not complete within {REAL_RUN_TIMEOUT_SECONDS}s, last status={last_status}")


def populate_result_state_summary(
    result: WorkspaceResult,
    snapshot: dict[str, Any],
    workspace_snapshot: dict[str, Any] | None = None,
    status: dict[str, Any] | None = None,
    observability: dict[str, Any] | None = None,
) -> dict[str, Any]:
    snap = snapshot.get("snapshot") or {}
    ws_snap = (workspace_snapshot or {}).get("snapshot") or snap
    run = snap.get("run") or {}
    workspace = ws_snap.get("workspace") or {}
    steps = snap.get("steps") or ([] if not snap.get("step") else [snap["step"]])
    tool_calls = snap.get("toolCalls") or []
    validations = snap.get("validationResults") or []
    checkpoints = snap.get("runtimeCheckpoints") or []
    results = snap.get("results") or []
    artifacts = snap.get("artifacts") or []
    acceptance = snap.get("acceptance") or {}
    audit = acceptance.get("audit") or {}
    task_progress = snap.get("taskProgress") or {}
    # The current Runtime projection exposes sealed operations and owner
    # evidence under acceptance.audit.  Keep the load-test assertions tied to
    # those authoritative fields while accepting older projection aliases when
    # they are present.
    if not steps:
        steps = []
        for operation in acceptance.get("operations") or []:
            parameters = operation.get("parameters") or []
            tool = next((item.get("string") for item in parameters if item.get("name") == "tool"), "")
            steps.append({
                "kind": operation.get("kind"),
                "state": operation.get("executionState") or operation.get("outcome"),
                "output": {"tool": tool} if tool else {},
            })
    if not tool_calls:
        tool_calls = [item for item in acceptance.get("operations") or [] if item.get("kind") == "mcp/tool_call" and item.get("outcome") == "accepted"]
    if not validations:
        validations = [item for item in audit.get("acceptedEvidence") or [] if item.get("kind") == "validation"]
    obs = (observability or {}).get("observability") or {}
    timeline = obs.get("timeline") or []
    progress = obs.get("progress") or {}
    if not progress and task_progress:
        progress = dict(task_progress)
        progress.setdefault("terminal", progress.get("state") in ("completed", "failed", "cancelled", "canceled"))
    run_status = (status or {}).get("status") or (status or {}).get("run") or run
    usage_traces = obs.get("llmUsageTraces") or (observability or {}).get("llmUsageTraces") or []
    usage_summary = obs.get("llmUsageSummary") or (observability or {}).get("llmUsageSummary") or snap.get("llmUsageSummary") or {}
    llm_steps = [item for item in steps if item.get("kind") == "llm/workspace_execution"]
    llm_terminal_steps = [item for item in llm_steps if item.get("state") in ("completed", "failed")]
    llm_non_terminal_steps = [item for item in llm_steps if item.get("state") not in ("completed", "failed")]
    missing_run_state = [item for item in timeline if item.get("source") in ("step", "step_stream") and not item.get("runState")]

    result.workspace_state = str(workspace.get("state") or "")
    result.run_state = str(run.get("state") or "")
    result.run_status_state = str(run_status.get("state") or "")
    result.progress_state = str(progress.get("state") or "")
    result.progress_terminal = progress.get("terminal") is True
    result.llm_step_count = len(llm_steps)
    result.llm_terminal_step_count = len(llm_terminal_steps)
    result.llm_non_terminal_step_count = len(llm_non_terminal_steps)
    result.llm_usage_trace_count = len(usage_traces)
    result.llm_usage_summary_entry_total = int(usage_summary.get("entryTotal") or 0)
    result.llm_usage_estimated_entries = int(usage_summary.get("estimatedEntries") or 0)
    result.llm_usage_provider_native_entries = int(usage_summary.get("providerNativeEntries") or 0)
    result.step_count = len(steps)
    result.tool_call_count = len(tool_calls)
    result.validation_count = len(validations)
    result.checkpoint_count = len(checkpoints)
    result.result_count = len(results)
    result.artifact_count = len(artifacts)
    result.timeline_missing_run_state = len(missing_run_state)

    return {
        "snap": snap,
        "ws_snap": ws_snap,
        "run": run,
        "workspace": workspace,
        "outcome": ws_snap.get("outcome") or snap.get("outcome") or {},
        "steps": steps,
        "decisions": snap.get("decisions") or [],
        "approvals": snap.get("approvals") or [],
        "tool_calls": tool_calls,
        "validations": validations,
        "checkpoints": checkpoints,
        "results": results,
        "artifacts": artifacts,
        "timeline": timeline,
        "progress": progress,
        "run_status": run_status,
        "usage_traces": usage_traces,
        "usage_summary": usage_summary,
        "llm_steps": llm_steps,
        "llm_terminal_steps": llm_terminal_steps,
        "llm_non_terminal_steps": llm_non_terminal_steps,
        "missing_run_state": missing_run_state,
    }


def validate_final_state(result: WorkspaceResult, snapshot: dict[str, Any], workspace_snapshot: dict[str, Any], status: dict[str, Any], observability: dict[str, Any]) -> None:
    state = populate_result_state_summary(result, snapshot, workspace_snapshot, status, observability)
    run = state["run"]
    workspace = state["workspace"]
    outcome = state["outcome"]
    tool_calls = state["tool_calls"]
    validations = state["validations"]
    checkpoints = state["checkpoints"]
    results = state["results"]
    artifacts = state["artifacts"]
    decisions = state["decisions"]
    approvals = state["approvals"]
    steps = state["steps"]
    progress = state["progress"]
    run_status = state["run_status"]
    timeline = state["timeline"]
    usage_traces = state["usage_traces"]
    usage_summary = state["usage_summary"]
    llm_steps = state["llm_steps"]
    llm_non_terminal_steps = state["llm_non_terminal_steps"]
    missing_run_state = state["missing_run_state"]

    errors: list[str] = []
    if workspace.get("state") not in ("active", "completed"):
        errors.append(f"workspace state={workspace.get('state')!r}, want active or completed")
    active_run_id = workspace.get("activeRunId")
    if active_run_id and active_run_id != result.run_id:
        errors.append(f"workspace activeRunId={workspace.get('activeRunId')!r}, want {result.run_id}")
    if outcome.get("runId") != result.run_id or outcome.get("accepted") is not True:
        errors.append(f"workspace outcome invalid: {outcome}, want accepted runId={result.run_id}")
    if run.get("state") != "completed":
        errors.append(f"run state={run.get('state')!r}, want completed: {run.get('failureReason', '')}")
    if run_status.get("state") != "completed":
        errors.append(f"run status state={run_status.get('state')!r}, want completed")
    if progress.get("state") != "completed" or progress.get("terminal") is not True:
        errors.append(f"observability progress invalid: {progress}")
    if llm_non_terminal_steps:
        errors.append(f"llm/workspace_execution non-terminal steps remain: {llm_non_terminal_steps}")
    if not usage_traces:
        errors.append("observability missing llmUsageTraces")
    else:
        estimated_entries = 0
        native_entries = 0
        for trace in usage_traces:
            estimated = bool(trace.get("estimated"))
            usage_source = trace.get("usageSource")
            if not usage_source:
                errors.append(f"llm usage trace missing usageSource: {trace}")
                continue
            if estimated:
                estimated_entries += 1
                if usage_source != "estimated":
                    errors.append(f"llm usage trace estimated/source mismatch: {trace}")
            else:
                native_entries += 1
                if usage_source != "provider_native":
                    errors.append(f"llm usage trace native/source mismatch: {trace}")
        if usage_summary.get("entryTotal") != len(usage_traces):
            errors.append(f"llm usage summary entryTotal mismatch: {usage_summary} vs traces={len(usage_traces)}")
        if usage_summary.get("estimatedEntries") != estimated_entries:
            errors.append(f"llm usage summary estimatedEntries mismatch: {usage_summary} vs {estimated_entries}")
        if usage_summary.get("providerNativeEntries") != native_entries:
            errors.append(f"llm usage summary providerNativeEntries mismatch: {usage_summary} vs {native_entries}")

    unresolved_decisions = [item for item in decisions if item.get("state") not in ("resolved", "closed")]
    if unresolved_decisions:
        errors.append(f"run has unresolved Decision Tickets: {unresolved_decisions}")
    unapproved_approvals = [item for item in approvals if item.get("state") not in ("approved", "rejected")]
    if unapproved_approvals:
        errors.append(f"run has unresolved Approvals: {unapproved_approvals}")
    has_expected_tool_step = any(
        (item.get("output") or {}).get("tool") == "loadtest.governed_result"
        for item in steps
        if item.get("kind") == "mcp/tool_call"
    )
    # The current Runtime projection intentionally keeps operation parameters
    # out of the acceptance view.  In that shape the owner-grounded tool proof
    # is the exact validation recipe/check attached to the accepted operation,
    # not a legacy step.output.tool field.
    if not has_expected_tool_step:
        for item in validations:
            if item.get("kind") != "validation":
                continue
            scalar_match = any(
                "loadtest.governed_result" in str(item.get(key) or "")
                for key in ("validationRecipeId", "requiredCheckRef", "targetRef")
            )
            list_match = any(
                "loadtest.governed_result" in str(check)
                for key in ("passedChecks", "executedChecks", "checks")
                for check in (item.get(key) or [])
            )
            if scalar_match or list_match:
                has_expected_tool_step = True
                break
    if not has_expected_tool_step:
        errors.append("mcp/tool_call step for loadtest.governed_result missing")
    if not tool_calls:
        errors.append("ToolCall evidence missing")
    if not validations:
        errors.append("ValidationResult evidence missing")
    accepted_results = [item for item in results if item.get("state") == "accepted"]
    if not accepted_results:
        errors.append(f"accepted Result missing: {results}")
    else:
        accepted = accepted_results[-1]
        if not str(accepted.get("summary") or "").strip():
            errors.append(f"accepted Result summary empty: {accepted}")
        summary_detail = accepted.get("summaryDetail") or {}
        if not summary_detail:
            errors.append(f"accepted Result summaryDetail missing: {accepted}")
        else:
            for key in ("goalOutcome",):
                if not str(summary_detail.get(key) or "").strip():
                    errors.append(f"accepted Result summaryDetail.{key} empty: {summary_detail}")
            if not summary_detail.get("evidenceRefs"):
                errors.append(f"accepted Result summaryDetail.evidenceRefs missing: {summary_detail}")

    missing_run_state = [item for item in timeline if item.get("source") in ("step", "step_stream") and not item.get("runState")]
    if missing_run_state:
        errors.append(f"timeline step rows missing runState: {missing_run_state[:3]}")

    if errors:
        result.llm_protocol_errors = errors
        raise RuntimeError("; ".join(errors))


def collect_failure_evidence(result: WorkspaceResult, case_dir: Path) -> None:
    if result.workspace_id and not result.run_id:
        try:
            snapshot = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/snapshot")
            write_json(case_dir / "failure-workspace-snapshot.json", snapshot)
            recovered_run_id = nested(snapshot, "snapshot.run.id", "") or nested(snapshot, "snapshot.workspace.activeRunId", "")
            if recovered_run_id:
                result.run_id = recovered_run_id
        except Exception as exc:
            write_text(case_dir / "failure-workspace-snapshot-error.txt", str(exc) + "\n")
    if not result.workspace_id or not result.run_id:
        return
    try:
        snapshot = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/snapshot")
        write_json(case_dir / "failure-snapshot.json", snapshot)
    except Exception as exc:
        write_text(case_dir / "failure-snapshot-error.txt", str(exc) + "\n")
        snapshot = {}
    try:
        observability = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/observability")
        write_json(case_dir / "failure-observability.json", observability)
    except Exception as exc:
        write_text(case_dir / "failure-observability-error.txt", str(exc) + "\n")
        observability = {}

    populate_result_state_summary(result, snapshot, None, None, observability)

    run = nested(snapshot, "snapshot.run", {})
    failure_reason = str(run.get("failureReason") or "")
    if failure_reason and failure_reason not in result.error:
        result.error = f"{result.error}; failureReason={failure_reason}"
    steps = nested(snapshot, "snapshot.steps", []) or []
    for step in steps:
        output = step.get("output") or {}
        stage = output.get("stage")
        parse_error = output.get("parseError") or output.get("repairError")
        if stage == "llm_schema_invalid" or parse_error:
            message = str(parse_error or output.get("error") or stage)
            if message and message not in result.llm_protocol_errors:
                result.llm_protocol_errors.append(message)


def execute_workspace(seq: int, template_id: str) -> WorkspaceResult:
    result = WorkspaceResult(index=seq, started_at=time.monotonic())
    case_dir = ARTIFACT_DIR / "workspaces" / f"{seq:04d}"
    try:
        workspace_payload, goal_payload, dod_payload = create_payloads(seq, template_id)
        with timed(result, "workspace_create"):
            workspace_response = run_curl("POST", "/api/v1/workorch/workspaces", workspace_payload)
        write_json(case_dir / "workspace.json", workspace_response)
        result.workspace_id = nested(workspace_response, "workspace.id", "")
        if not result.workspace_id:
            raise RuntimeError("workspace id missing")

        with timed(result, "goal_create"):
            goal_response = run_curl("POST", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/goals", goal_payload)
        write_json(case_dir / "goal.json", goal_response)
        goal_id = nested(goal_response, "goal.id", "")
        if not goal_id:
            raise RuntimeError("goal id missing")

        with timed(result, "dod_create"):
            dod_response = run_curl("POST", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/dods", dod_payload)
        write_json(case_dir / "dod.json", dod_response)
        dod_id = nested(dod_response, "dod.id", "")
        if not dod_id:
            raise RuntimeError("dod id missing")

        with timed(result, "activation_to_run"):
            activation = run_curl(
                "POST",
                f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/execution-activations",
                {
                    "intent": "restart",
                    "requestId": f"loadtest-restart-{BATCH_ID}-{seq:04d}",
                    "reason": "governed_workspace_concurrency_loadtest",
                    "source": "loadtest-governed",
                },
            )
        write_json(case_dir / "activation.json", activation)
        run = wait_activation_run(result.workspace_id, activation, case_dir)
        result.run_id = run.get("id", "")
        if not result.run_id:
            raise RuntimeError("run id missing")
        # Both mock and real providers use the same owner-driven wait. Any
        # genuine governance ticket is resolved through its typed owner API;
        # the fixture itself no longer fabricates legacy Decision/Approval
        # candidate fields or requires a fixed governance sequence.
        with timed(result, "run_to_completed"):
            completed = drive_real_run_until_completed(result, case_dir)
        write_json(case_dir / "run-completed-status.json", completed)
        run4 = extract_run_status(completed)
        if run4.get("state") != "completed":
            raise RuntimeError(f"final run state={run4.get('state')!r}, want completed")

        with timed(result, "snapshot_read"):
            snapshot = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/snapshot")
        with timed(result, "workspace_snapshot_read"):
            workspace_snapshot = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/snapshot")
        with timed(result, "status_read"):
            status = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/status")
        with timed(result, "observability_read"):
            observability = run_curl("GET", f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/runs/{url_quote(result.run_id)}/observability")
        write_json(case_dir / "snapshot.json", snapshot)
        write_json(case_dir / "workspace-snapshot.json", workspace_snapshot)
        write_json(case_dir / "status.json", status)
        write_json(case_dir / "observability.json", observability)

        validate_final_state(result, snapshot, workspace_snapshot, status, observability)
        result.success = True
    except Exception as exc:
        result.error = str(exc)
        if isinstance(exc, HTTPError):
            result.http_status = exc.http_status
            result.curl_code = exc.curl_code
            write_json(case_dir / "http-error.json", exc.to_dict())
        collect_failure_evidence(result, case_dir)
        result.failure_class = classify_failure(result)
        write_text(case_dir / "error.txt", result.error + "\n")
    finally:
        result.ended_at = time.monotonic()
        write_json(case_dir / "result.json", result_to_dict(result))
    return result


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    rank = (len(ordered) - 1) * pct
    lower = int(rank)
    upper = min(lower + 1, len(ordered) - 1)
    fraction = rank - lower
    return ordered[lower] * (1 - fraction) + ordered[upper] * fraction


def latency_stats(values: list[float]) -> dict[str, float]:
    return {
        "count": len(values),
        "avg_ms": statistics.mean(values) if values else 0,
        "p50_ms": percentile(values, 0.50),
        "p95_ms": percentile(values, 0.95),
        "p99_ms": percentile(values, 0.99),
        "max_ms": max(values) if values else 0,
    }


def result_to_dict(result: WorkspaceResult) -> dict[str, Any]:
    timing_by_stage: dict[str, float] = {}
    for item in result.timings:
        timing_by_stage[item.name] = timing_by_stage.get(item.name, 0) + item.duration_ms
    return {
        "index": result.index,
        "workspaceId": result.workspace_id,
        "runId": result.run_id,
        "success": result.success,
        "error": result.error,
        "e2eMs": result.e2e_ms,
        "ticketIds": result.ticket_ids,
        "approvalId": result.approval_id,
        "stepCount": result.step_count,
        "toolCallCount": result.tool_call_count,
        "validationCount": result.validation_count,
        "checkpointCount": result.checkpoint_count,
        "resultCount": result.result_count,
        "artifactCount": result.artifact_count,
        "timelineMissingRunState": result.timeline_missing_run_state,
        "llmStepCount": result.llm_step_count,
        "llmTerminalStepCount": result.llm_terminal_step_count,
        "llmNonTerminalStepCount": result.llm_non_terminal_step_count,
        "workspaceState": result.workspace_state,
        "runState": result.run_state,
        "runStatusState": result.run_status_state,
        "progressState": result.progress_state,
        "progressTerminal": result.progress_terminal,
        "llmUsageTraceCount": result.llm_usage_trace_count,
        "llmUsageSummaryEntryTotal": result.llm_usage_summary_entry_total,
        "llmUsageProviderNativeEntries": result.llm_usage_provider_native_entries,
        "llmUsageEstimatedEntries": result.llm_usage_estimated_entries,
        "llmProtocolErrors": result.llm_protocol_errors,
        "httpStatus": result.http_status,
        "curlCode": result.curl_code,
        "failureClass": result.failure_class,
        "timingByStage": timing_by_stage,
        "timings": [
            {
                "name": item.name,
                "startedAtMonotonicMs": item.started_at * 1000,
                "endedAtMonotonicMs": item.ended_at * 1000,
                "durationMs": item.duration_ms,
            }
            for item in result.timings
        ],
    }


def classify_failure(result: WorkspaceResult) -> str:
    if result.success:
        return ""
    if result.http_status:
        if 400 <= result.http_status < 500:
            return "http_4xx"
        if result.http_status >= 500:
            return "http_5xx"
    if result.curl_code:
        if result.curl_code == 28:
            return "http_timeout"
        if result.curl_code == 52:
            return "http_empty_reply"
        return "curl_error"
    if result.llm_protocol_errors:
        return "llm_protocol_error"
    error_text = result.error.lower()
    if "provider" in error_text or "eof" in error_text or "429" in error_text:
        return "provider_error"
    if "accepted result" in error_text or "artifact" in error_text or "summarydetail" in error_text:
        return "evidence_integrity_error"
    return "business_state_error"


def collect_metrics() -> dict[str, float]:
    metrics = {}
    try:
        text = run_curl_text("/metrics")
    except Exception:
        return metrics
    for line in text.splitlines():
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) != 2:
            continue
        key, value = parts
        if key in {
            "workorch_workspace_projection_active_run_heartbeats",
            "workorch_workspace_projection_pending_rebuilds",
            "workorch_workspace_projection_stalled_runs",
            "workorch_workspace_projection_ineffective_retry_runs",
            "workorch_convergence_active_slices",
            "workorch_convergence_open_focuses",
        }:
            try:
                metrics[key] = float(value)
            except ValueError:
                pass
    return metrics


class MetricsSampler:
    def __init__(self, interval_seconds: float):
        self.interval_seconds = interval_seconds
        self.samples: list[MetricsSample] = []
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._lock = threading.Lock()

    def start(self) -> None:
        if self._thread is not None:
            return
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def stop(self) -> list[MetricsSample]:
        self._stop.set()
        if self._thread is not None:
            self._thread.join(timeout=max(self.interval_seconds * 2, 1))
        with self._lock:
            return list(self.samples)

    def _run(self) -> None:
        while not self._stop.is_set():
            sampled_at = time.monotonic()
            values = collect_metrics()
            if values:
                with self._lock:
                    self.samples.append(MetricsSample(sampled_at=sampled_at, values=values))
            self._stop.wait(self.interval_seconds)


ACTIVE_RUN_STAGE_NAMES = {"activation_to_run", "run_to_completed"}


def max_overlap(windows: list[tuple[float, float]]) -> int:
    events: list[tuple[float, int]] = []
    for started, ended in windows:
        if started <= 0 or ended <= started:
            continue
        events.append((started, 1))
        events.append((ended, -1))
    active = 0
    ret = 0
    for _, delta in sorted(events, key=lambda item: (item[0], item[1])):
        active += delta
        if active > ret:
            ret = active
    return ret


def active_run_windows(results: list[WorkspaceResult]) -> list[tuple[float, float]]:
    windows: list[tuple[float, float]] = []
    for result in results:
        if not result.success:
            continue
        for timing in result.timings:
            if timing.name in ACTIVE_RUN_STAGE_NAMES:
                windows.append((timing.started_at, timing.ended_at))
    return windows


def build_summary(
    results: list[WorkspaceResult],
    started_at: float,
    ended_at: float,
    metrics_before: dict[str, float],
    metrics_samples: list[MetricsSample],
) -> dict[str, Any]:
    success = [item for item in results if item.success]
    failed = [item for item in results if not item.success]
    success_rate = len(success) / TOTAL if TOTAL else 0
    duration_s = max(ended_at - started_at, 0.001)
    stage_names = sorted({timing.name for item in results for timing in item.timings})
    stage_stats = {name: latency_stats([item.timing_ms(name) for item in results if item.timing_ms(name) > 0]) for name in stage_names}
    business_latency = {
        "activation": stage_stats.get("activation_to_run", latency_stats([])),
        "runToCompleted": stage_stats.get("run_to_completed", latency_stats([])),
        "snapshotRead": stage_stats.get("snapshot_read", latency_stats([])),
        "observabilityRead": stage_stats.get("observability_read", latency_stats([])),
    }
    result_items = [result_to_dict(item) for item in results]
    expected_overlap = min(CONCURRENCY, len(success))
    if expected_overlap < 2:
        expected_overlap = 0
    observed_stage_overlap = max_overlap(active_run_windows(results))
    active_overlap_pass = True
    if EXPECT_ACTIVE_OVERLAP and expected_overlap >= 2:
        active_overlap_pass = observed_stage_overlap >= expected_overlap
    diagnostic_stage_names = [
        "activation_to_run",
        "run_to_completed",
        "snapshot_read",
        "workspace_snapshot_read",
        "observability_read",
        "artifact_content_read",
    ]
    return {
        "batchId": BATCH_ID,
        "baseUrl": BASE_URL,
        "llmMode": LLM_MODE,
        "mcpSessionPolicy": MCP_SESSION_POLICY,
        "pollIntervalSeconds": POLL_INTERVAL_SECONDS,
        "statusWaitTimeoutSeconds": STATUS_WAIT_TIMEOUT_SECONDS,
        "total": TOTAL,
        "concurrency": CONCURRENCY,
        "expectActiveOverlap": EXPECT_ACTIVE_OVERLAP,
        "expectedActiveRunOverlap": expected_overlap,
        "maxObservedActiveStageOverlap": observed_stage_overlap,
        "activeRunOverlapPass": active_overlap_pass,
        "metricsSampleIntervalSeconds": METRICS_SAMPLE_INTERVAL_SECONDS,
        "metricsSampleTotal": len(metrics_samples),
        "workspaceSuccessTotal": len(success),
        "workspaceFailedTotal": len(failed),
        "successRate": success_rate,
        "minSuccessRate": MIN_SUCCESS_RATE,
        "pass": success_rate >= MIN_SUCCESS_RATE and active_overlap_pass,
        "durationSeconds": duration_s,
        "throughputWorkspacesPerMin": len(success) / duration_s * 60,
        "e2eLatency": latency_stats([item.e2e_ms for item in results]),
        "stageLatency": stage_stats,
        "businessLatency": business_latency,
        "llmCallCountPerWorkspace": latency_stats([float(item.llm_step_count) for item in results]),
        "llmProtocolErrorTotal": sum(len(item.llm_protocol_errors) for item in results),
        "http4xxTotal": len([item for item in failed if 400 <= item.http_status < 500]),
        "http5xxTotal": len([item for item in failed if item.http_status >= 500]),
        "curlErrorTotal": len([item for item in failed if item.curl_code and not item.http_status]),
        "failureClassTotal": count_by_key([item.failure_class or "unknown" for item in failed]),
        "timelineMissingRunStateTotal": sum(item.timeline_missing_run_state for item in results),
        "metricsBefore": metrics_before,
        "metricsAfter": collect_metrics(),
        "slowestWorkspaces": slowest_results(result_items),
        "stageSamples": stage_samples(result_items, diagnostic_stage_names),
        "results": result_items,
    }


def count_by_key(values: list[str]) -> dict[str, int]:
    ret: dict[str, int] = {}
    for value in values:
        ret[value] = ret.get(value, 0) + 1
    return ret


def slowest_results(results: list[dict[str, Any]], limit: int = 5) -> list[dict[str, Any]]:
    ret = []
    for item in sorted(results, key=lambda value: float(value.get("e2eMs") or 0), reverse=True)[:limit]:
        timings = item.get("timingByStage") or {}
        top_stages = sorted(timings.items(), key=lambda pair: float(pair[1] or 0), reverse=True)[:5]
        ret.append(
            {
                "index": item.get("index"),
                "workspaceId": item.get("workspaceId", ""),
                "runId": item.get("runId", ""),
                "success": item.get("success", False),
                "e2eMs": item.get("e2eMs", 0),
                "topStages": [{"name": name, "durationMs": duration} for name, duration in top_stages],
            }
        )
    return ret


def stage_samples(results: list[dict[str, Any]], stage_names: list[str], limit: int = 5) -> dict[str, list[dict[str, Any]]]:
    ret: dict[str, list[dict[str, Any]]] = {}
    for stage in stage_names:
        samples = []
        for item in results:
            timings = item.get("timingByStage") or {}
            if stage not in timings:
                continue
            samples.append(
                {
                    "index": item.get("index"),
                    "workspaceId": item.get("workspaceId", ""),
                    "runId": item.get("runId", ""),
                    "durationMs": timings[stage],
                }
            )
        ret[stage] = sorted(samples, key=lambda value: float(value.get("durationMs") or 0), reverse=True)[:limit]
    return ret


def write_markdown_report(summary: dict[str, Any]) -> None:
    lines = [
        "# workorch Workspace Concurrency Load Test Report",
        "",
        f"- batchId: `{summary['batchId']}`",
        f"- baseUrl: `{summary['baseUrl']}`",
        f"- llmMode: `{summary['llmMode']}`",
        f"- mcpSessionPolicy: `{summary['mcpSessionPolicy']}`",
        f"- pollIntervalSeconds: `{summary['pollIntervalSeconds']}`",
        f"- statusWaitTimeoutSeconds: `{summary['statusWaitTimeoutSeconds']}`",
        f"- total: `{summary['total']}`",
        f"- concurrency: `{summary['concurrency']}`",
        f"- expectActiveOverlap: `{summary['expectActiveOverlap']}`",
        f"- expectedActiveRunOverlap: `{summary['expectedActiveRunOverlap']}`",
        f"- maxObservedActiveStageOverlap: `{summary['maxObservedActiveStageOverlap']}`",
        f"- activeRunOverlapPass: `{summary['activeRunOverlapPass']}`",
        f"- metricsSampleTotal: `{summary['metricsSampleTotal']}`",
        f"- success: `{summary['workspaceSuccessTotal']}`",
        f"- failed: `{summary['workspaceFailedTotal']}`",
        f"- successRate: `{summary['successRate']:.4f}`",
        f"- minSuccessRate: `{summary['minSuccessRate']:.4f}`",
        f"- pass: `{summary['pass']}`",
        f"- throughputWorkspacesPerMin: `{summary['throughputWorkspacesPerMin']:.2f}`",
        f"- http4xxTotal: `{summary['http4xxTotal']}`",
        f"- http5xxTotal: `{summary['http5xxTotal']}`",
        f"- curlErrorTotal: `{summary['curlErrorTotal']}`",
        f"- llmProtocolErrorTotal: `{summary['llmProtocolErrorTotal']}`",
        f"- timelineMissingRunStateTotal: `{summary['timelineMissingRunStateTotal']}`",
        "",
        "## Failure Classes",
        "",
        "```json",
        json.dumps(summary["failureClassTotal"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## E2E Latency",
        "",
        "```json",
        json.dumps(summary["e2eLatency"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## Stage Latency",
        "",
        "```json",
        json.dumps(summary["stageLatency"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## Business Latency",
        "",
        "```json",
        json.dumps(summary["businessLatency"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## Slowest Workspaces",
        "",
        "```json",
        json.dumps(summary["slowestWorkspaces"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## Stage Samples",
        "",
        "```json",
        json.dumps(summary["stageSamples"], ensure_ascii=False, indent=2),
        "```",
        "",
        "## Failed Workspaces",
        "",
    ]
    failed = [item for item in summary["results"] if not item["success"]]
    if not failed:
        lines.append("No failed workspace.")
    else:
        for item in failed:
            lines.append(f"- `{item['workspaceId'] or item['index']}` / `{item['runId']}`: {item['error']}")
    lines.append("")
    write_text(ARTIFACT_DIR / "report.md", "\n".join(lines))


def main() -> int:
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    mcp_server_id = f"mcp-loadtest-{BATCH_ID}"
    workspace_template_id = f"loadtest-governed-{BATCH_ID}"
    provider_id = PROVIDER_ID or ("loadtest-mock" if LLM_MODE == "mock" else "")
    mock_server: MockLLMServer | None = None
    original_llm: dict[str, Any] | None = None
    started_at = time.monotonic()
    interrupted = False

    def handle_signal(signum: int, _frame: Any) -> None:
        nonlocal interrupted
        interrupted = True
        raise KeyboardInterrupt(f"received signal {signum}")

    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)

    try:
        ready = run_curl("GET", "/readyz")
        if ready.get("status") != "ready":
            raise RuntimeError(f"readyz status={ready.get('status')!r}, want ready")
        configuration = run_curl("GET", "/api/v1/workorch/configuration")
        write_json(ARTIFACT_DIR / "configuration-before.json", configuration)
        metrics_before = collect_metrics()
        write_json(ARTIFACT_DIR / "metrics-before.json", metrics_before)

        endpoint_dir = MCP_ENDPOINT_ROOT / mcp_server_id
        endpoint_dir.mkdir(parents=True, exist_ok=True)
        helper_path = endpoint_dir / "loadtest_mcp_helper.py"
        create_mcp_helper(helper_path)
        if LLM_MODE == "mock":
            mock_server = MockLLMServer(MOCK_ADDR, mcp_server_id)
            mock_server.start()
            original_llm = configure_mock_provider(provider_id)
        else:
            provider_id = select_real_provider()
            original_llm = configure_real_provider_timeout(provider_id)

        mcp_server_payload = {
            "id": mcp_server_id,
            "name": f"Governed Load Test MCP {BATCH_ID}",
            "transport": "stdio",
            "endpoint": str(helper_path),
            "enabled": True,
            "allowedTools": ["loadtest.governed_result"],
            "defaultApprovalPolicy": "read_only",
            "sessionPolicy": MCP_SESSION_POLICY,
            "timeoutMillis": 30000,
        }
        try:
            mcp_server = run_curl("POST", "/api/v1/workorch/mcp/servers", mcp_server_payload)
        except HTTPError as exc:
            if exc.http_status != 409:
                raise
            mcp_server = run_curl("PUT", f"/api/v1/workorch/mcp/servers/{url_quote(mcp_server_id)}", mcp_server_payload)
        write_json(ARTIFACT_DIR / "mcp-server.json", mcp_server)

        # Server creation materializes a conservative draft capability from
        # allowedTools (unknown effect).  The load-test CWI is a validation
        # proof, so seal the owner-declared read-only effect and bind this
        # exact capability through the template.  Binding only a server would
        # leave Runtime without an activation capability context and the
        # focus would correctly fail before candidate acquisition.
        mcp_capability_id = f"{mcp_server_id}:tool:loadtest.governed_result"
        mcp_capability_payload = {
            "id": mcp_capability_id,
            "serverId": mcp_server_id,
            "kind": "tool",
            "name": "loadtest.governed_result",
            "displayName": "Governed load-test result",
            "description": "Deterministic MCP behavior probe for governed concurrency validation.",
            "category": "loadtest",
            "tags": ["loadtest", "validation"],
            "usageHint": "Use only for the activation-sealed governed load-test CWI.",
            "inputSummary": "country, province, city, lookupName, runId",
            "outputSummary": "Deterministic structured governed result",
            "stageHints": ["execution", "validation"],
            "riskLevel": "read_only",
            "effectScope": "none",
            "idempotencyClass": "safe_to_replay",
            "enabled": True,
            "approvalPolicy": "read_only",
            "status": "active",
        }
        try:
            mcp_capability = run_curl("POST", "/api/v1/workorch/mcp/capabilities", mcp_capability_payload)
        except HTTPError as exc:
            if exc.http_status != 409:
                raise
            mcp_capability = run_curl("PUT", f"/api/v1/workorch/mcp/capabilities/{url_quote(mcp_capability_id)}", mcp_capability_payload)
        write_json(ARTIFACT_DIR / "mcp-capability.json", mcp_capability)

        # Workspace activation is intent-only.  Bind the load-test MCP server
        # through the workspace owner template so restart compilation reads a
        # sealed owner snapshot instead of accepting capability IDs from the
        # activation request body.
        templates = run_curl("GET", "/api/v1/workorch/workspace-templates")
        generic_template = next(
            (item for item in (templates.get("workspaceTemplates") or []) if item.get("id") == "generic-governed"),
            None,
        )
        if not generic_template:
            raise RuntimeError("generic-governed workspace template is unavailable")
        loadtest_template = json.loads(json.dumps(generic_template))
        loadtest_template["id"] = workspace_template_id
        loadtest_template["name"] = f"Governed Load Test {BATCH_ID}"
        loadtest_template["description"] = "Load-test owner binding for the deterministic MCP evidence server."
        context_hints = loadtest_template.setdefault("contextHints", {})
        context_hints.setdefault("loadtestMCP", {}).update({
            "defaultMCPServerIds": [mcp_server_id],
            "defaultCapabilityIds": [mcp_capability_id],
        })
        context_hints["acceptanceSlicePolicy"] = {
            "schemaVersion": "workorch.acceptance_slice_policy",
            "maxWorkItems": 1,
            "maxFocuses": 1,
            "maxCandidateContextItems": 4,
            "completionPolicy": "all_required_items_accepted",
            "resultPolicy": "slice_result_allowed",
            "groups": [{
                "id": "loadtest-mcp",
                "title": "Governed MCP load-test evidence",
                "scope": "loadtest_mcp",
                "priority": 10,
                "maxWorkItems": 1,
                "match": {"all": True},
            }],
        }
        loadtest_template.setdefault("evidencePolicies", []).append({
            "id": "loadtest-mcp-proof",
            "appliesTo": "mcp/tool_call",
            "coverageRequirements": [{
                "id": "*",
                "scope": "dod_item",
                "evidenceRole": "supplemental_evidence",
                "targetRole": "mcp_capability",
                "targetDiscoverySources": ["available_capabilities"],
                "targetSelectorRef": "mcp-capability-selector",
                "targetSelectorOwnerRef": "blocks/mcp",
                "targetSelectorSubjectRef": mcp_server_id + "/loadtest.governed_result",
                "requireTargetGrounding": False,
                "requiredEvidenceKinds": ["validation"],
                "requireDirectEvidence": True,
                "requireExecutableBehaviorEvidence": True,
                "acceptedStepKinds": ["mcp/tool_call"],
                "validationRecipeId": "mcp/tool_call/" + mcp_server_id + "/loadtest.governed_result",
                "requiredCheckRefs": ["mcp/tool_call/" + mcp_server_id + "/loadtest.governed_result"],
            }],
        })
        workspace_template = run_curl("POST", "/api/v1/workorch/workspace-templates", loadtest_template)
        write_json(ARTIFACT_DIR / "workspace-template.json", workspace_template)

        results: list[WorkspaceResult] = []
        metrics_sampler = MetricsSampler(METRICS_SAMPLE_INTERVAL_SECONDS)
        metrics_sampler.start()
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
                future_map = {executor.submit(execute_workspace, seq, workspace_template_id): seq for seq in range(1, TOTAL + 1)}
                for future in concurrent.futures.as_completed(future_map):
                    result = future.result()
                    results.append(result)
                    status = "ok" if result.success else "failed"
                    print(f"[{status}] index={result.index} workspace={result.workspace_id} run={result.run_id} e2eMs={result.e2e_ms:.1f}", flush=True)
        finally:
            metrics_samples = metrics_sampler.stop()
        results.sort(key=lambda item: item.index)
        ended_at = time.monotonic()
        summary = build_summary(results, started_at, ended_at, metrics_before, metrics_samples)
        write_json(ARTIFACT_DIR / "summary.json", summary)
        write_markdown_report(summary)
        print(json.dumps({k: summary[k] for k in ["batchId", "llmMode", "mcpSessionPolicy", "total", "concurrency", "expectActiveOverlap", "expectedActiveRunOverlap", "maxObservedActiveStageOverlap", "activeRunOverlapPass", "workspaceSuccessTotal", "workspaceFailedTotal", "successRate", "minSuccessRate", "pass", "throughputWorkspacesPerMin", "http4xxTotal", "http5xxTotal", "curlErrorTotal", "llmProtocolErrorTotal", "timelineMissingRunStateTotal"]}, ensure_ascii=False, indent=2))
        print(f"artifacts={ARTIFACT_DIR}")
        return 0 if summary["pass"] else 1
    except KeyboardInterrupt:
        print("load test interrupted", file=sys.stderr)
        return 130 if interrupted else 1
    except Exception as exc:
        write_text(ARTIFACT_DIR / "setup-error.txt", str(exc) + "\n")
        print(f"load test setup failed: {exc}", file=sys.stderr)
        print(f"artifacts={ARTIFACT_DIR}", file=sys.stderr)
        return 1
    finally:
        restore_llm_configuration(original_llm)
        if mock_server:
            mock_server.stop()
        if not KEEP_ARTIFACTS:
            shutil.rmtree(ARTIFACT_DIR, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())
