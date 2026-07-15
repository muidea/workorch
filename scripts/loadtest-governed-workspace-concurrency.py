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
EXPECTED_MAX_CONCURRENT_RUNS = int(os.environ.get("WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS", "0"))
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
            return {"completionState": "failed", "thoughtSummary": "probe"}

        if (
            step_input.get("graphFollowUp")
            or step_input.get("previousStepOutput")
            or "follow-up-graph" in step_id
            or "assessment" in step_id
            or "correction" in step_id
            or '"graphFollowUp": true' in raw
            or '"previousStepOutput"' in raw
            or '"assessmentOnly"' in raw
        ):
            previous_step_id = step_input.get("previousStepId") or extract_json_string(raw, "previousStepId") or f"{run_id}/block/loadtest-tool"
            decision_refs = [f"{run_id}-step-1-decision", f"{run_id}-llm-after-decision-decision"]
            approval_refs = [f"{run_id}-llm-after-decision-2-approval"]
            return {
                "thoughtSummary": "Governed load test evidence was collected and can be assessed.",
                "completionState": "completed",
                "dodAssessment": {
                    "satisfied": True,
                    "summary": f"{PROVINCE}{CITY} governed load test completed with two Decision Tickets, one Approval, and MCP tool evidence.",
                    "goalOutcome": f"{PROVINCE}{CITY} governed load test produced accepted MCP evidence.",
                    "implementationSummary": "The run completed the required governance sequence and executed the configured MCP tool.",
                    "verificationSummary": "The final DoD assessment references the MCP evidence step after two Decision Tickets and one Approval.",
                    "items": [
                        {"item": "two Decision tickets are resolved", "state": "satisfied", "evidenceRefs": decision_refs},
                        {"item": "one Approval is approved", "state": "satisfied", "evidenceRefs": approval_refs},
                        {"item": "MCP loadtest.governed_result evidence is collected", "state": "satisfied", "evidenceRefs": [previous_step_id]},
                    ],
                    "knownLimitations": [],
                    "residualRisks": [],
                    "nextActions": ["Review the load test report and retained workspace artifacts."],
                },
            }

        call = self.next_call(workspace_id, run_id)
        if "llm-after-approval" in step_id or call >= 4:
            return {
                "thoughtSummary": "Approval has been granted; execute the governed MCP tool.",
                "completionState": "continue",
                "executionGraph": {
                    "nodes": [
                        {
                            "id": "loadtest-tool",
                            "kind": "mcp/tool_call",
                            "input": {
                                "serverId": self.mcp_server_id,
                                "tool": "loadtest.governed_result",
                                "input": {
                                    "country": COUNTRY,
                                    "province": PROVINCE,
                                    "city": CITY,
                                    "lookupName": LOOKUP_NAME,
                                    "runId": run_id,
                                },
                                "validationEvidence": {
                                    "mode": "runtime_behavior",
                                    "observationType": "runtime_observation",
                                },
                            },
                        }
                    ]
                },
            }
        if call == 3:
            return {
                "thoughtSummary": "All missing information is available; operator approval is required before tool execution.",
                "completionState": "blocked_on_approval",
                "requiredApproval": {
                    "reason": f"Approve governed load test MCP execution for {COUNTRY}{PROVINCE}{CITY}.",
                    "action": "execute governed load test MCP tool",
                    "context": {"country": COUNTRY, "province": PROVINCE, "city": CITY, "tool": "loadtest.governed_result"},
                },
            }
        if call == 2:
            return {
                "thoughtSummary": "Target city is available; request the required output format and risk confirmation.",
                "completionState": "blocked_on_decision",
                "requiredDecision": {
                    "reason": "Output format and execution risk confirmation are required before approval.",
                    "decisionKind": "provide_missing_info",
                    "question": "Please provide output format and confirm the governed tool execution risk.",
                    "context": {"requiredFields": ["outputFormat", "riskAccepted"], "suggestedOutputFormat": "structured_summary"},
                },
            }
        return {
            "thoughtSummary": "Target city is missing; request operator input.",
            "completionState": "blocked_on_decision",
            "requiredDecision": {
                "reason": "The governed load test must be bound to a specific target city.",
                "decisionKind": "provide_missing_info",
                "question": "Which city should workorch use for this governed load test?",
                "context": {
                    "requiredFields": ["country", "province", "city", "lookupName"],
                    "suggestedCity": {"country": COUNTRY, "province": PROVINCE, "city": CITY, "lookupName": LOOKUP_NAME},
                },
            },
        }


def extract_json_string(raw: str, key: str) -> str:
    match = re.search(rf'"{re.escape(key)}"\s*:\s*"([^"]+)"', raw)
    return match.group(1) if match else ""


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
            "defaultMaxOutputTokens": int(budget.get("defaultMaxOutputTokens") or 4096),
            "maxRequestBytes": int(budget.get("maxRequestBytes") or 4194304),
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


def create_payloads(seq: int) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    decision_item = "two Decision tickets are resolved" if LLM_MODE == "mock" else "Decision governance is resolved"
    workspace = {
        "title": f"workspace concurrency loadtest {BATCH_ID}-{seq:04d}",
        "description": "Governed workspace concurrency load test with Decision governance, Approval, MCP evidence, and final Result.",
    }
    goal = {
        "text": "- MCP loadtest.governed_result evidence is collected"
    }
    dod = {
        "items": [
            decision_item,
            "one Approval is approved",
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
    obs = (observability or {}).get("observability") or {}
    timeline = obs.get("timeline") or []
    progress = obs.get("progress") or {}
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
        "outcome": ws_snap.get("outcome") or {},
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
    if workspace.get("state") != "completed":
        errors.append(f"workspace state={workspace.get('state')!r}, want completed")
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
    if not llm_steps:
        errors.append("llm/workspace_execution step missing")
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

    resolved_decisions = [item for item in decisions if item.get("state") == "resolved"]
    approved_approvals = [item for item in approvals if item.get("state") == "approved"]
    if LLM_MODE == "mock":
        if len(resolved_decisions) < 2:
            errors.append(f"resolved Decision Ticket count={len(resolved_decisions)}, want >=2: {decisions}")
        if not approved_approvals:
            errors.append(f"approved Approval missing: {approvals}")
        if not any((item.get("output") or {}).get("tool") == "loadtest.governed_result" for item in steps if item.get("kind") == "mcp/tool_call"):
            errors.append("mcp/tool_call step for loadtest.governed_result missing")
        if not tool_calls:
            errors.append("ToolCall evidence missing")
        if not validations:
            errors.append("ValidationResult evidence missing")
    else:
        unresolved_decisions = [item for item in decisions if item.get("state") not in ("resolved", "closed")]
        if unresolved_decisions:
            errors.append(f"real run has unresolved Decision Tickets: {unresolved_decisions}")
        unapproved_approvals = [item for item in approvals if item.get("state") not in ("approved", "rejected")]
        if unapproved_approvals:
            errors.append(f"real run has unresolved Approvals: {unapproved_approvals}")
        if not steps:
            errors.append("real run snapshot has no steps")
    if not checkpoints:
        errors.append("RuntimeCheckpoint evidence missing")
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
            for key in ("goalOutcome", "implementationSummary", "verificationSummary"):
                if not str(summary_detail.get(key) or "").strip():
                    errors.append(f"accepted Result summaryDetail.{key} empty: {summary_detail}")
            if not summary_detail.get("doDItems"):
                errors.append(f"accepted Result summaryDetail.doDItems missing: {summary_detail}")
            if not summary_detail.get("evidenceRefs"):
                errors.append(f"accepted Result summaryDetail.evidenceRefs missing: {summary_detail}")
        artifact_refs = set(accepted.get("artifactRefs") or [])
        linked = [item for item in artifacts if item.get("resultId") == accepted.get("id")]
        if not linked:
            errors.append(f"Artifact linked to Result {accepted.get('id')} missing")
        elif not artifact_refs.intersection({item.get("id") for item in linked}):
            errors.append(f"Result artifactRefs do not reference linked Artifact: result={accepted} artifacts={linked}")
        linked_categories = {item.get("category") for item in linked if item.get("category")}
        required_categories = {"final_summary", "runtime_evidence"}
        if not required_categories.issubset(linked_categories):
            errors.append(f"linked Artifact categories={sorted(linked_categories)}, want {sorted(required_categories)}")
        for artifact in linked:
            uri = str(artifact.get("uri") or "")
            if not uri.startswith("workorch://"):
                errors.append(f"artifact uri must use workorch:// only: {artifact}")
            if re.search(r"(^|\s)/(home|tmp|var|opt|usr)/", uri):
                errors.append(f"artifact uri leaks local absolute path: {uri}")
            if artifact.get("category") == "final_summary" and uri:
                try:
                    with timed(result, "artifact_content_read"):
                        content = run_curl_text(f"/api/v1/workorch/artifacts/content?uri={url_quote(uri)}")
                    if "Result Summary" not in content and "目标成果" not in content:
                        errors.append(f"final summary artifact content missing expected heading: {uri}")
                except Exception as exc:
                    errors.append(f"final summary artifact content read failed: {exc}")

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
    if LLM_MODE == "mock" and run.get("state") == "blocked_on_decision" and "want blocked_on_approval" in result.error and len(result.ticket_ids) >= 2:
        message = "llm workflow protocol violation: returned extra Decision after second resolution; expected Approval"
        if message not in result.llm_protocol_errors:
            result.llm_protocol_errors.append(message)
    if LLM_MODE == "mock" and run.get("state") == "blocked_on_approval" and "want blocked_on_decision" in result.error and len(result.ticket_ids) == 1:
        message = "llm workflow protocol violation: skipped second Decision; expected another Decision before Approval"
        if message not in result.llm_protocol_errors:
            result.llm_protocol_errors.append(message)
    steps = nested(snapshot, "snapshot.steps", []) or []
    for step in steps:
        output = step.get("output") or {}
        stage = output.get("stage")
        parse_error = output.get("parseError") or output.get("repairError")
        if stage == "llm_schema_invalid" or parse_error:
            message = str(parse_error or output.get("error") or stage)
            if message and message not in result.llm_protocol_errors:
                result.llm_protocol_errors.append(message)


def execute_workspace(seq: int, provider_id: str, mcp_server_id: str) -> WorkspaceResult:
    result = WorkspaceResult(index=seq, started_at=time.monotonic())
    case_dir = ARTIFACT_DIR / "workspaces" / f"{seq:04d}"
    try:
        workspace_payload, goal_payload, dod_payload = create_payloads(seq)
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

        with timed(result, "activation_to_decision1"):
            activation = run_curl(
                "POST",
                f"/api/v1/workorch/workspaces/{url_quote(result.workspace_id)}/execution-activations",
                {"goalId": goal_id, "doDId": dod_id, "llmTarget": {"providerId": provider_id, "modelId": LOADTEST_MODEL_ID}, "mcpServerIds": [mcp_server_id], "priority": seq % 10},
            )
        write_json(case_dir / "activation.json", activation)
        run = wait_activation_run(result.workspace_id, activation, case_dir)
        result.run_id = run.get("id", "")
        if not result.run_id:
            raise RuntimeError("run id missing")
        if LLM_MODE == "real":
            with timed(result, "real_activation_to_completed"):
                completed = drive_real_run_until_completed(result, case_dir)
            write_json(case_dir / "run-completed-status.json", completed)

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
            return result

        if run.get("state") == "blocked_on_decision" and run.get("blockingTicketId"):
            decision1_status = activation
            run1 = run
        else:
            decision1_status = wait_run_status(
                result.workspace_id,
                result.run_id,
                lambda status: status.get("state") == "blocked_on_decision" and bool(status.get("blockingTicketId")),
                "state blocked_on_decision with blockingTicketId after activation",
                wait_state="blocked_on_decision",
            )
            write_json(case_dir / "decision1-status.json", decision1_status)
            run1 = extract_run_status(decision1_status)
        if run1.get("state") != "blocked_on_decision":
            raise RuntimeError(f"first run state={run1.get('state')!r}, want blocked_on_decision")
        ticket1 = run1.get("blockingTicketId", "")
        if not ticket1:
            raise RuntimeError("first blockingTicketId missing")
        result.ticket_ids.append(ticket1)

        with timed(result, "decision1_resolve"):
            decision1 = resolve_decision(
                result.workspace_id,
                ticket1,
                {"operator": "loadtest", "country": COUNTRY, "province": PROVINCE, "city": CITY, "lookupName": LOOKUP_NAME},
            )
        write_json(case_dir / "decision1-resolved.json", decision1)

        with timed(result, "decision1_to_decision2"):
            next_gate_status = wait_run_status(
                result.workspace_id,
                result.run_id,
                lambda status: (
                    status.get("state") == "blocked_on_decision"
                    and bool(status.get("blockingTicketId"))
                    and status.get("blockingTicketId") != ticket1
                )
                or (status.get("state") == "blocked_on_approval" and bool(status.get("blockingApprovalId"))),
                f"state blocked_on_decision with a new blockingTicketId after {ticket1}, or blocked_on_approval",
            )
        run2 = extract_run_status(next_gate_status)
        if run2.get("state") == "blocked_on_decision":
            write_json(case_dir / "decision2-status.json", next_gate_status)
            ticket2 = run2.get("blockingTicketId", "")
            if not ticket2:
                raise RuntimeError("second blockingTicketId missing")
            result.ticket_ids.append(ticket2)

            with timed(result, "decision2_resolve"):
                decision2 = resolve_decision(
                    result.workspace_id,
                    ticket2,
                    {
                        "operator": "loadtest",
                        "outputFormat": "structured_summary",
                        "riskAccepted": True,
                        "decisionRound": 2,
                        "allRequiredInformationProvided": True,
                        "nextExpectedState": "blocked_on_approval",
                        "noMoreDecisionTickets": True,
                    },
                )
            write_json(case_dir / "decision2-resolved.json", decision2)

            with timed(result, "decision2_to_approval"):
                approval_status = wait_run_status(
                    result.workspace_id,
                    result.run_id,
                    lambda status: status.get("state") == "blocked_on_approval" and bool(status.get("blockingApprovalId")),
                    "state blocked_on_approval with blockingApprovalId",
                    wait_state="blocked_on_approval",
                )
        else:
            approval_status = next_gate_status
        write_json(case_dir / "approval-status.json", approval_status)
        run3 = extract_run_status(approval_status)
        if run3.get("state") != "blocked_on_approval":
            raise RuntimeError(f"third run state={run3.get('state')!r}, want blocked_on_approval")
        result.approval_id = run3.get("blockingApprovalId", "")
        if not result.approval_id:
            raise RuntimeError("blockingApprovalId missing")

        with timed(result, "approval_approve"):
            approval = approve_blocking_approval(result.workspace_id, result.approval_id)
        write_json(case_dir / "approval-approved.json", approval)
        if nested(approval, "approval.state", "") != "approved":
            raise RuntimeError(f"approval state={nested(approval, 'approval.state', None)!r}, want approved")

        with timed(result, "approval_to_completed"):
            completed: dict[str, Any] | None = None
            for gate_index in range(1, 5):
                gate_status = wait_run_status(
                    result.workspace_id,
                    result.run_id,
                    lambda status: status.get("state") == "completed"
                    or (status.get("state") == "blocked_on_approval" and bool(status.get("blockingApprovalId"))),
                    "state completed or blocked_on_approval with blockingApprovalId",
                )
                gate_run = extract_run_status(gate_status)
                if gate_run.get("state") == "completed":
                    completed = gate_status
                    break
                operator_approval_id = gate_run.get("blockingApprovalId", "")
                if not operator_approval_id:
                    raise RuntimeError(f"approval gate {gate_index} blockingApprovalId missing: {gate_run}")
                if operator_approval_id == result.approval_id:
                    raise RuntimeError(f"approval gate {gate_index} repeated already approved approval {operator_approval_id}")
                write_json(case_dir / f"approval-followup-{gate_index}-status.json", gate_status)
                operator_approval = approve_blocking_approval(result.workspace_id, operator_approval_id)
                write_json(case_dir / f"approval-followup-{gate_index}-approved.json", operator_approval)
                if nested(operator_approval, "approval.state", "") != "approved":
                    raise RuntimeError(f"follow-up approval state={nested(operator_approval, 'approval.state', None)!r}, want approved")
            if completed is None:
                raise RuntimeError("run did not complete after resolving follow-up approval gates")
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
        if key in {"workorch_run_queue_waiting", "workorch_run_queue_total", "workorch_run_requests_in_flight", "workorch_server_max_concurrent_runs"}:
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


ACTIVE_RUN_STAGE_NAMES = {
    "activation_to_decision1",
    "decision1_to_decision2",
    "decision2_to_approval",
    "approval_to_completed",
}


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


def max_metric_value(samples: list[MetricsSample], key: str) -> float:
    values = [sample.values.get(key, 0) for sample in samples]
    return max(values) if values else 0


def build_summary(
    results: list[WorkspaceResult],
    started_at: float,
    ended_at: float,
    metrics_before: dict[str, float],
    server_max_concurrent_runs: int,
    metrics_samples: list[MetricsSample],
) -> dict[str, Any]:
    success = [item for item in results if item.success]
    failed = [item for item in results if not item.success]
    success_rate = len(success) / TOTAL if TOTAL else 0
    duration_s = max(ended_at - started_at, 0.001)
    stage_names = sorted({timing.name for item in results for timing in item.timings})
    stage_stats = {name: latency_stats([item.timing_ms(name) for item in results if item.timing_ms(name) > 0]) for name in stage_names}
    business_latency = {
        "queueWaitProxy": stage_stats.get("activation_to_decision1", latency_stats([])),
        "decisionRound1": stage_stats.get("decision1_to_decision2", stage_stats.get("resume_to_decision2", latency_stats([]))),
        "decisionRound2": stage_stats.get("decision2_to_approval", stage_stats.get("resume_to_approval", latency_stats([]))),
        "approval": stage_stats.get("approval_approve", latency_stats([])),
        "toolExecutionAndResultMaterialization": stage_stats.get("approval_to_completed", stage_stats.get("resume_to_completed", latency_stats([]))),
        "snapshotRead": stage_stats.get("snapshot_read", latency_stats([])),
        "observabilityRead": stage_stats.get("observability_read", latency_stats([])),
    }
    result_items = [result_to_dict(item) for item in results]
    expected_overlap = min(CONCURRENCY, server_max_concurrent_runs or CONCURRENCY, len(success))
    if expected_overlap < 2:
        expected_overlap = 0
    observed_stage_overlap = max_overlap(active_run_windows(results))
    observed_metric_inflight = int(max_metric_value(metrics_samples, "workorch_run_requests_in_flight"))
    active_overlap_pass = True
    if EXPECT_ACTIVE_OVERLAP and expected_overlap >= 2:
        active_overlap_pass = max(observed_stage_overlap, observed_metric_inflight) >= expected_overlap
    diagnostic_stage_names = [
        "activation_to_decision1",
        "decision1_to_decision2",
        "decision2_to_approval",
        "approval_to_completed",
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
        "serverMaxConcurrentRuns": server_max_concurrent_runs,
        "expectActiveOverlap": EXPECT_ACTIVE_OVERLAP,
        "expectedActiveRunOverlap": expected_overlap,
        "maxObservedActiveStageOverlap": observed_stage_overlap,
        "maxObservedRunRequestsInFlight": observed_metric_inflight,
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
        f"- serverMaxConcurrentRuns: `{summary['serverMaxConcurrentRuns']}`",
        f"- expectActiveOverlap: `{summary['expectActiveOverlap']}`",
        f"- expectedActiveRunOverlap: `{summary['expectedActiveRunOverlap']}`",
        f"- maxObservedActiveStageOverlap: `{summary['maxObservedActiveStageOverlap']}`",
        f"- maxObservedRunRequestsInFlight: `{summary['maxObservedRunRequestsInFlight']}`",
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
        server_max_concurrent_runs = int(nested(configuration, "configuration.server.maxConcurrentRuns", 0) or 0)
        metrics_before = collect_metrics()
        write_json(ARTIFACT_DIR / "metrics-before.json", metrics_before)
        if EXPECTED_MAX_CONCURRENT_RUNS > 0 and server_max_concurrent_runs < EXPECTED_MAX_CONCURRENT_RUNS:
            raise RuntimeError(
                "deployed server maxConcurrentRuns="
                f"{server_max_concurrent_runs}, want >= {EXPECTED_MAX_CONCURRENT_RUNS}; "
                "increase [server].maxConcurrentRuns and redeploy before this capacity run"
            )

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
            "defaultApprovalPolicy": "operator",
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

        results: list[WorkspaceResult] = []
        metrics_sampler = MetricsSampler(METRICS_SAMPLE_INTERVAL_SECONDS)
        metrics_sampler.start()
        try:
            with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
                future_map = {executor.submit(execute_workspace, seq, provider_id, mcp_server_id): seq for seq in range(1, TOTAL + 1)}
                for future in concurrent.futures.as_completed(future_map):
                    result = future.result()
                    results.append(result)
                    status = "ok" if result.success else "failed"
                    print(f"[{status}] index={result.index} workspace={result.workspace_id} run={result.run_id} e2eMs={result.e2e_ms:.1f}", flush=True)
        finally:
            metrics_samples = metrics_sampler.stop()
        results.sort(key=lambda item: item.index)
        ended_at = time.monotonic()
        summary = build_summary(results, started_at, ended_at, metrics_before, server_max_concurrent_runs, metrics_samples)
        write_json(ARTIFACT_DIR / "summary.json", summary)
        write_markdown_report(summary)
        print(json.dumps({k: summary[k] for k in ["batchId", "llmMode", "mcpSessionPolicy", "total", "concurrency", "serverMaxConcurrentRuns", "expectActiveOverlap", "expectedActiveRunOverlap", "maxObservedActiveStageOverlap", "maxObservedRunRequestsInFlight", "activeRunOverlapPass", "workspaceSuccessTotal", "workspaceFailedTotal", "successRate", "minSuccessRate", "pass", "throughputWorkspacesPerMin", "http4xxTotal", "http5xxTotal", "curlErrorTotal", "llmProtocolErrorTotal", "timelineMissingRunStateTotal"]}, ensure_ascii=False, indent=2))
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
