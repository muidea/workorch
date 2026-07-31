#!/usr/bin/env python3
"""Create and verify the deployed SMO-C7 restart/crash matrix.

This is a test-only process harness.  It starts a fresh file-backed WorkOrch
binary and a deterministic OpenAI-compatible provider, then creates three
ordinary workspace activations in the same data root:

* ``active_no_dispatch`` is stopped after its provider submission is durable
  but before a candidate document is received;
* ``active_sealed_dispatch`` is stopped while a shell-owner operation from an
  already sealed dispatch is claimed and deliberately waiting;
* ``terminal_delivery_pending`` is stopped after a terminal is sealed while
  the Result owner's directory is temporarily non-writable.

The harness neither writes runtime records nor adds a production test hook.
It only drives public HTTP APIs, controls the disposable provider/process and
temporarily changes permissions in its own fresh data root.  After the crash,
the normal startup-recovery path must converge the three persisted aggregates.
"""

from __future__ import annotations

import argparse
import copy
import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from verify_runtime_v2_restart_contract import fixture_manifest_errors  # noqa: E402


def free_addr() -> tuple[str, int]:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as value:
        value.bind(("127.0.0.1", 0))
        return value.getsockname()[0], value.getsockname()[1]


def json_value(value: Any, *names: str) -> Any:
    if not isinstance(value, dict):
        return None
    for name in names:
        if name in value:
            return value[name]
    return None


def string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item.strip() for item in value if isinstance(item, str) and item.strip()]


def prompt_sections(messages: Any) -> dict[str, str]:
    content = "\n".join(
        str(item.get("content", "")) for item in messages or [] if isinstance(item, dict)
    )
    result: dict[str, str] = {}
    marker = "### "
    parts = content.split(marker)
    for part in parts[1:]:
        title, sep, body = part.partition("\n")
        if sep and title.strip():
            result[title.strip()] = body.strip()
    return result


def contract_target(contract: dict[str, Any]) -> str:
    target = json_value(contract, "target", "Target") or {}
    for key in (
        "allowedTargetRefs",
        "AllowedTargetRefs",
        "exactTargetRefs",
        "ExactTargetRefs",
        "ownerScopeTargetRefs",
        "OwnerScopeTargetRefs",
    ):
        refs = string_list(json_value(target, key))
        if refs:
            return refs[0]
    raise ValueError("sealed operation contract has no concrete target")


def select_contract(contracts: list[Any], *kinds: str) -> dict[str, Any]:
    for kind in kinds:
        for item in contracts:
            if isinstance(item, dict) and json_value(item, "kind", "Kind") == kind:
                return item
    raise ValueError("sealed operation contract lacks one of: " + ", ".join(kinds))


def focus_ordinal(focus_id: str) -> int:
    digits = "".join(ch for ch in reversed(focus_id) if ch.isdigit())
    return max(1, int(digits[::-1])) if digits else 1


def fixture_content(focus_id: str) -> str:
    ordinal = focus_ordinal(focus_id)
    return "Runtime V2 restart recovery fixture input.\n" + "".join(
        f"verified by restart harness focus-{number:03d}\n" for number in range(1, ordinal + 1)
    )


class MockState:
    def __init__(self, artifact_dir: Path, shell_release: Path) -> None:
        self._lock = threading.Lock()
        self._control: dict[str, str] = {}
        self._events: list[dict[str, Any]] = []
        self.artifact_dir = artifact_dir
        self.events_path = artifact_dir / "provider-events.jsonl"
        self.shell_release = shell_release

    def set_control(self, values: dict[str, str]) -> None:
        with self._lock:
            self._control = dict(values)
        (self.artifact_dir / "provider-control.json").write_text(
            json.dumps(values, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )

    def mode_for(self, workspace_id: str) -> str:
        with self._lock:
            return self._control.get(workspace_id, "normal")

    def record(self, event: dict[str, Any]) -> None:
        event = dict(event)
        event["at"] = time.time()
        with self._lock:
            event["sequence"] = len(self._events) + 1
            self._events.append(event)
            with self.events_path.open("a", encoding="utf-8") as handle:
                handle.write(json.dumps(event, ensure_ascii=False, sort_keys=True) + "\n")

    def count(self, workspace_id: str) -> int:
        with self._lock:
            return sum(1 for item in self._events if item.get("workspaceId") == workspace_id)

    def snapshot(self) -> list[dict[str, Any]]:
        with self._lock:
            return copy.deepcopy(self._events)


def candidate_payload(state: MockState, payload: dict[str, Any]) -> tuple[dict[str, Any], str]:
    sections = prompt_sections(payload.get("messages"))
    authority_raw = sections.get("C3.runtime-authority")
    if not authority_raw:
        # Provider readiness probes must remain independent of Runtime V2
        # authority. They are never submitted as candidates.
        return {
            "operations": [{
                "id": "probe-read",
                "kind": "file/read",
                "coverageWorkItemIds": ["cwi-probe"],
                "targetRefs": ["runtime-validation-source.txt"],
                "parameters": [{"name": "optional", "boolean": True}, {"name": "maxBytes", "integer": 64}],
            }]
        }, "probe"
    authority = json.loads(authority_raw)
    context = json_value(authority, "context", "Context") or {}
    output = json_value(authority, "outputContract", "OutputContract") or {}
    transaction = json_value(authority, "transaction", "Transaction") or {}
    workspace_id = str(json_value(context, "workspaceId", "WorkspaceID") or "").strip()
    run_id = str(json_value(context, "runId", "RunID") or "").strip()
    required_state = str(json_value(context, "requiredState", "RequiredState") or "").strip()
    focus = json_value(context, "focus", "Focus") or {}
    focus_id = str(json_value(focus, "id", "ID") or "").strip()
    coverage = string_list(json_value(focus, "coverageWorkItemIds", "CoverageWorkItemIDs"))
    contracts = json_value(output, "operationContracts", "OperationContracts") or []
    if not workspace_id or not run_id or not focus_id or not coverage:
        raise ValueError("C3.runtime-authority is missing scoped focus")
    mode = state.mode_for(workspace_id)
    state.record({
        "workspaceId": workspace_id,
        "runId": run_id,
        "authoritySnapshotId": json_value(transaction, "authoritySnapshotId", "AuthoritySnapshotID"),
        "requiredState": required_state,
        "mode": mode,
        "stablePrefixPresent": "C0.runtime-contract" in sections,
    })
    if mode == "hold" and required_state == "need_state_change":
        while state.mode_for(workspace_id) == "hold":
            time.sleep(0.05)
    if required_state == "need_state_change":
        if mode == "shell_wait":
            contract = select_contract(contracts, "shell/command")
            target_ref = contract_target(contract)
            return {
                "operations": [{
                    "id": "restart-shell-wait-" + focus_id,
                    "kind": "shell/command",
                    "coverageWorkItemIds": coverage[:1],
                    "targetRefs": [target_ref],
                    "parameters": [
                        {"name": "command", "string": "sh"},
                        {"name": "args", "stringList": ["-c", f'while [ ! -f "{state.shell_release}" ]; do sleep 1; done']},
                        {"name": "timeoutMillis", "integer": 600000},
                        {"name": "stateChanging", "boolean": True},
                    ],
                }]
            }, mode
        contract = select_contract(contracts, "file/edit", "patch/apply", "file/write")
        kind = str(json_value(contract, "kind", "Kind"))
        target_ref = contract_target(contract)
        previous = fixture_content(f"focus-{focus_ordinal(focus_id)-1:03d}") if focus_ordinal(focus_id) > 1 else "Runtime V2 restart recovery fixture input.\n"
        current = fixture_content(focus_id)
        if kind == "file/edit":
            parameters = [{"name": "oldText", "string": previous}, {"name": "newText", "string": current}]
        elif kind == "patch/apply":
            parameters = [
                {"name": "patchType", "string": "file_write"},
                {"name": "content", "string": current},
                {"name": "summary", "string": "restart recovery fixture state change"},
            ]
        else:
            parameters = [{"name": "content", "string": current}]
        return {
            "operations": [{
                "id": "restart-state-" + focus_id,
                "kind": kind,
                "coverageWorkItemIds": coverage[:1],
                "targetRefs": [target_ref],
                "parameters": parameters,
            }]
        }, mode
    if required_state == "need_validation":
        contract = select_contract(contracts, "file/read", "validation/managed_session", "shell/command")
        kind = str(json_value(contract, "kind", "Kind"))
        target_ref = contract_target(contract)
        if kind == "file/read":
            parameters = [{"name": "optional", "boolean": False}, {"name": "maxBytes", "integer": 16384}]
        else:
            parameters = [
                {"name": "command", "string": "sh"},
                {"name": "args", "stringList": ["-c", "test -f runtime-validation-source.txt"]},
                {"name": "timeoutMillis", "integer": 30000},
            ]
        return {
            "operations": [{
                "id": "restart-validation-" + focus_id,
                "kind": kind,
                "coverageWorkItemIds": coverage[:1],
                "targetRefs": [target_ref],
                "parameters": parameters,
            }]
        }, mode
    if required_state == "need_assessment":
        evidence: list[str] = []
        execution_tail = sections.get("C4.execution-tail", "")
        if execution_tail:
            try:
                tail = json.loads(execution_tail)
                for item in tail.get("latestAcceptedEvidence") or []:
                    if set(string_list(item.get("coverageWorkItemIds"))) & set(coverage):
                        evidence.extend(string_list(item.get("evidenceRefs")))
            except json.JSONDecodeError:
                pass
        if not evidence:
            raise ValueError("assessment C4 tail has no current-focus evidence")
        evidence = list(dict.fromkeys(evidence))
        return {
            "focusId": focus_id,
            "coverageWorkItemIds": coverage,
            "disposition": "satisfied",
            "items": [{"coverageWorkItemId": item, "satisfied": True, "evidenceRefs": evidence} for item in coverage],
            "summary": "Restart recovery fixture assessment accepted.",
            "evidenceRefs": evidence,
        }, mode
    raise ValueError(f"unsupported C3 requiredState {required_state!r}")


def start_mock_server(state: MockState, host: str, port: int) -> ThreadingHTTPServer:
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, _format: str, *_args: Any) -> None:
            return

        def _json(self, status: int, value: dict[str, Any]) -> None:
            raw = json.dumps(value, ensure_ascii=False).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self) -> None:  # noqa: N802
            if self.path.split("?", 1)[0] not in ("/v1/models", "/models"):
                self._json(404, {"error": "not found"})
                return
            self._json(200, {"object": "list", "data": [{
                "id": "verify-model", "object": "model", "owned_by": "verify",
                "operations": ["chat_completions"], "contextWindowTokens": 128000, "maxOutputTokens": 8192,
            }]})

        def do_POST(self) -> None:  # noqa: N802
            if self.path.split("?", 1)[0] not in ("/v1/chat/completions", "/chat/completions"):
                self._json(404, {"error": "not found"})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                request = json.loads(self.rfile.read(length))
                candidate, _mode = candidate_payload(state, request)
                self._json(200, {"choices": [{"message": {"content": json.dumps(candidate, ensure_ascii=False)}}]})
            except (ValueError, json.JSONDecodeError) as exc:
                # Keep the mock's own contract-parser failure in the managed
                # artifact.  The Runtime only sees a normal provider HTTP
                # response; recording this locally prevents a fixture defect
                # from being misdiagnosed as a Runtime provider failure.
                state.record({"mode": "provider_error", "error": str(exc)})
                self._json(400, {"error": {"message": str(exc)}})

    server = ThreadingHTTPServer((host, port), Handler)
    threading.Thread(target=server.serve_forever, name="runtime-v2-c7-mock", daemon=True).start()
    return server


class Harness:
    def __init__(self, artifact_dir: Path, binary: Path) -> None:
        self.artifact_dir = artifact_dir
        self.binary = binary
        self.config_dir = artifact_dir / "config"
        self.data_dir = artifact_dir / ".workorch-data"
        self.deliverables_dir = artifact_dir / "deliverables"
        self.shell_release = artifact_dir / "shell-release"
        self.server_log = artifact_dir / "workorch-server.log"
        self.host, self.port = free_addr()
        self.mock_host, self.mock_port = free_addr()
        self.base_url = f"http://{self.host}:{self.port}"
        self.mock_url = f"http://{self.mock_host}:{self.mock_port}"
        self.server: subprocess.Popen[bytes] | None = None
        self.mock_state = MockState(artifact_dir, self.shell_release)
        self.mock = start_mock_server(self.mock_state, self.mock_host, self.mock_port)

    def close(self) -> None:
        if not self.shell_release.exists():
            self.shell_release.touch()
        self.stop_server(force=True)
        self.mock.shutdown()
        self.mock.server_close()

    def write_config(self) -> None:
        self.config_dir.mkdir(parents=True, exist_ok=True)
        self.data_dir.mkdir(parents=True, exist_ok=True)
        (self.config_dir / "application.toml").write_text(
            f'''endpointName = "workorch"\n\n[server]\naddr = "{self.host}:{self.port}"\nreadHeaderTimeout = "5s"\nreadTimeout = "15s"\nwriteTimeout = "180s"\nidleTimeout = "60s"\nshutdownTimeout = "5s"\nmaxConcurrentRuns = 3\nstaleDispatchedStepMinAge = "10m"\n\n[repository]\nbackend = "file"\ndataDir = "{self.data_dir}"\n\n[runtime.git]\ndefaultTimeout = "30s"\n\n[llm.aiProxy]\nbaseURL = "{self.mock_url}"\napiKey = "restart-harness-secret"\ntimeout = "60s"\ncatalogRequestTimeout = "5s"\ncatalogTTL = "5m"\ncatalogMaxStaleAge = "24h"\nmaxCatalogResponseBytes = 1048576\nmaxCatalogModels = 1000\n\n[llm.defaultChat]\nproviderID = "verify"\nmodelID = "verify-model"\n\n[llm.customProviders.verify]\nprovider = "compatible"\napiSurface = "openai_compatible_http"\nbaseURL = "{self.mock_url}"\napiKey = "restart-harness-secret"\ntimeout = "60s"\n\n[llm.customProviders.verify.models.verify-model]\ncontextWindowTokens = 128000\nmaxOutputTokens = 8192\noperations = ["chat_completions"]\n''',
            encoding="utf-8",
        )

    def start_server(self) -> None:
        if self.server is not None:
            raise RuntimeError("server already running")
        self.write_config()
        log = self.server_log.open("ab")
        self.server = subprocess.Popen([str(self.binary), "--server", "-config-dir", str(self.config_dir)], stdout=log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            if self.server.poll() is not None:
                raise RuntimeError("workorch server exited before readiness")
            try:
                self.get("/healthz")
                self.get("/readyz")
                return
            except Exception:
                time.sleep(0.1)
        raise RuntimeError("timed out waiting for workorch server")

    def stop_server(self, force: bool = False) -> None:
        if self.server is None:
            return
        if self.server.poll() is None:
            self.server.send_signal(signal.SIGKILL if force else signal.SIGINT)
            try:
                self.server.wait(timeout=15)
            except subprocess.TimeoutExpired:
                self.server.kill()
                self.server.wait(timeout=5)
        self.server = None

    def request(self, method: str, path: str, body: Any | None = None) -> dict[str, Any]:
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(self.base_url + path, data=data, headers=headers, method=method)
        with urllib.request.urlopen(request, timeout=45) as response:
            return json.load(response)

    def get(self, path: str) -> dict[str, Any]:
        return self.request("GET", path)

    @staticmethod
    def _slice_policy() -> dict[str, Any]:
        return {
            "schemaVersion": "workorch.acceptance_slice_policy.v2", "maxWorkItems": 1, "maxFocuses": 1,
            "maxCandidateContextItems": 4, "completionPolicy": "all_required_items_accepted", "resultPolicy": "slice_result_allowed",
            "groups": [{"id": "restart-all", "title": "Restart recovery", "scope": "restart_recovery", "priority": 10, "maxWorkItems": 1, "match": {"all": True}}],
        }

    def create_fixture(self, label: str, shell_state_change: bool = False) -> dict[str, str]:
        template_id = f"runtime-v2-c7-restart-{label}"
        hints: dict[str, Any] = {"stateChangePolicy": "must_produce_state_change", "acceptanceSlicePolicy": self._slice_policy()}
        if shell_state_change:
            hints["profileContracts"] = {
                "restart-recovery": {
                    "evidenceRequirements": [{
                        # DoD work units are generated as dod-001, dod-002,
                        # etc.  A literal profile requirement ID would never
                        # match them; use the documented scope wildcard so the
                        # sealed focus carries this fixture's shell contract.
                        "id": "*", "scope": "dodItem", "evidenceRole": "primary_state_change",
                        "requiredEvidenceKinds": ["state_change"], "acceptedStepKinds": ["shell/command"],
                        "requireDirectEvidence": True, "requireTargetGrounding": True,
                        "targetDiscoverySources": ["workspace_manifest"], "targetSelectorRef": "file/workspace_deliverable",
                        "targetRole": "restart_recovery_target",
                    }]
                }
            }
        template = {
            "id": template_id, "name": "Runtime V2 restart recovery " + label,
            "description": "Fresh SMO-C7 crash/restart fixture.",
            "goalText": "Produce and validate one bounded restart-recovery result.",
            "doDItems": ["One bounded state change is produced and validated."],
            "contextHints": hints,
            "evidencePolicies": [{
                "id": "restart-evidence", "appliesTo": "shell/command" if shell_state_change else "file/edit",
                "coverageRequirements": [{"id": "*", "scope": "dod_item", "requiredEvidenceKinds": ["state_change", "validation"], "requireDirectEvidence": True}],
            }],
        }
        created = self.request("POST", "/api/v1/workorch/workspace-templates", template)
        if (created.get("workspaceTemplate") or {}).get("id") != template_id:
            raise RuntimeError(f"template creation failed: {created}")
        deliverable = self.deliverables_dir / label
        deliverable.mkdir(parents=True, exist_ok=False)
        (deliverable / "runtime-validation-source.txt").write_text("Runtime V2 restart recovery fixture input.\n", encoding="utf-8")
        workspace = self.request("POST", "/api/v1/workorch/workspaces", {
            "title": "Runtime V2 restart recovery " + label,
            "description": "Fresh SMO-C7 crash/restart fixture.",
            "deliverableDirectory": str(deliverable),
            "templateId": template_id,
        }).get("workspace") or {}
        workspace_id = str(workspace.get("id") or "")
        if not workspace_id:
            raise RuntimeError("workspace creation returned no ID")
        goal = self.request("POST", f"/api/v1/workorch/workspaces/{workspace_id}/goals", {"text": "Produce and validate one bounded restart-recovery result."}).get("goal") or {}
        dod = self.request("POST", f"/api/v1/workorch/workspaces/{workspace_id}/dods", {"items": ["One bounded state change is produced and validated."]}).get("dod") or {}
        goal_id = str(goal.get("id") or "")
        dod_id = str(dod.get("id") or "")
        if not goal_id or not dod_id:
            raise RuntimeError("fixture definitions returned no goal/DoD ID")
        return {"label": label, "workspaceId": workspace_id, "goalId": goal_id, "doDId": dod_id, "templateId": template_id}

    def activate(self, fixture: dict[str, str]) -> None:
        body = {
            # A fresh fixture has no active Run.  Restart is the explicit
            # full-workspace activation; goal/DoD/model are owner-sourced from
            # the Workspace and configured default target rather than legacy
            # activation payload fields.
            "intent": "restart", "requestId": "c7-restart-" + fixture["label"], "reason": "c7_restart_crash_fixture", "source": "verification",
        }
        response = self.request("POST", f"/api/v1/workorch/workspaces/{fixture['workspaceId']}/execution-activations", body)
        (self.artifact_dir / f"activate-{fixture['label']}.json").write_text(json.dumps(response, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    def snapshot(self, workspace_id: str) -> dict[str, Any]:
        return self.get(f"/api/v1/workorch/workspaces/{workspace_id}/snapshot")

    def details(self, fixture: dict[str, str]) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
        snapshot = self.snapshot(fixture["workspaceId"])
        root = snapshot.get("snapshot") or snapshot
        run = root.get("run") or {}
        run_id = str(run.get("id") or "")
        if not run_id:
            raise RuntimeError(f"{fixture['label']}: no run in workspace snapshot")
        observed = self.get(f"/api/v1/workorch/workspaces/{fixture['workspaceId']}/runs/{run_id}/observability")
        return snapshot, root, observed

    def wait_for(self, description: str, predicate: Any, timeout: float = 90) -> Any:
        deadline = time.monotonic() + timeout
        last: Any = None
        while time.monotonic() < deadline:
            try:
                last = predicate()
                if last:
                    return last
            except Exception as exc:  # transient projection/read races are expected while driving.
                last = str(exc)
            time.sleep(0.1)
        raise RuntimeError(f"timed out waiting for {description}: {last}")


def acceptance(observed: dict[str, Any]) -> dict[str, Any]:
    return ((observed.get("observability") or {}).get("acceptance") or {})


def audit_of(observed: dict[str, Any]) -> dict[str, Any]:
    return acceptance(observed).get("audit") or {}


def save_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def candidate_count(audit: dict[str, Any]) -> int:
    return len(audit.get("candidateAttempts") or [])


def dispatch_count(audit: dict[str, Any]) -> int:
    return len(audit.get("dispatches") or [])


def stable_prefixes(audit: dict[str, Any]) -> list[str]:
    values: list[str] = []
    for item in audit.get("candidateAttempts") or []:
        value = str((item or {}).get("stablePrefixHash") or "").strip()
        if value:
            values.append(value)
    return sorted(set(values))


def wait_event_count(harness: Harness, workspace_id: str, minimum: int = 1) -> bool:
    return harness.mock_state.count(workspace_id) >= minimum


def run_recovery_settled(harness: Harness, fixture: dict[str, str]) -> bool:
    """Return whether a restarted Run reached an owner-visible safe boundary.

    The harness deliberately leaves the pre-crash activation alone after the
    process is killed.  Before beginning the next matrix case, wait for its
    normal startup recovery to reach a terminal or governed wait.  This keeps
    each persisted boundary independent without manufacturing contention in
    the process BackgroundRoutine or EventHub lanes.
    """

    _snapshot, root, _observed = harness.details(fixture)
    state = str((root.get("run") or {}).get("state") or "").strip().lower()
    return state in {"completed", "failed", "cancelled", "superseded", "blocked_on_decision", "blocked_on_approval"}


def save_recovery_snapshot(harness: Harness, fixture: dict[str, str], fixture_class: str) -> None:
    snapshot, _root, observed = harness.details(fixture)
    save_json(
        harness.artifact_dir / f"post-recovery-{fixture_class}.json",
        {"snapshot": snapshot, "observability": observed},
    )


def pre_restart_fixture(harness: Harness, fixture: dict[str, str], fixture_class: str) -> dict[str, Any]:
    snapshot, root, observed = harness.details(fixture)
    audit = audit_of(observed)
    run = root.get("run") or {}
    workspace = root.get("workspace") or {}
    data = {
        "class": fixture_class,
        "workspaceId": fixture["workspaceId"],
        "runId": run.get("id"),
        "preRestart": {
            "stateRevision": int(audit.get("stateRevision") or 0),
            "candidateAttemptCount": candidate_count(audit),
            "dispatchCount": dispatch_count(audit),
            "authoritySnapshotId": ((audit.get("authority") or {}).get("snapshotId") or ""),
            "providerRequestCount": harness.mock_state.count(fixture["workspaceId"]),
            "stablePrefixHashes": stable_prefixes(audit),
            "runState": run.get("state"),
            "activeRunId": workspace.get("activeRunId") or workspace.get("activeRunID"),
        },
    }
    save_json(harness.artifact_dir / f"pre-restart-{fixture_class}.json", {"snapshot": snapshot, "observability": observed, "fixture": data})
    return data


def run_harness(artifact_dir: Path, binary: Path) -> None:
    artifact_dir.mkdir(parents=True, exist_ok=True)
    harness = Harness(artifact_dir, binary)
    try:
        harness.start_server()
        save_json(
            artifact_dir / "configuration-bootstrap.json",
            {"providerId": "verify", "modelId": "verify-model", "source": "fresh_test_config"},
        )
        no_dispatch = harness.create_fixture("active-no-dispatch")
        sealed = harness.create_fixture("active-sealed-dispatch", shell_state_change=True)
        terminal = harness.create_fixture("terminal-delivery-pending")
        save_json(artifact_dir / "fixtures.json", {"fixtures": [no_dispatch, sealed, terminal]})
        # A synchronous owner/provider call intentionally holds one EventHub
        # lane. Do not try to stack three artificial waits in one process: that
        # would test hub saturation rather than the three persisted recovery
        # boundaries. The one fresh-data verification round therefore performs
        # one crash/restart cycle per workspace and never resumes a crashed Run.
        def crash_and_restart(label: str) -> None:
            harness.stop_server(force=True)
            save_json(artifact_dir / f"crash-{label}.json", {"class": label, "signal": "SIGKILL", "at": time.time()})
            harness.start_server()

        harness.mock_state.set_control({no_dispatch["workspaceId"]: "hold"})
        harness.activate(no_dispatch)
        harness.wait_for("provider submission without dispatch", lambda: wait_event_count(harness, no_dispatch["workspaceId"]))
        no_item = pre_restart_fixture(harness, no_dispatch, "active_no_dispatch")
        if no_item["preRestart"]["dispatchCount"] != 0:
            raise RuntimeError("active_no_dispatch unexpectedly sealed a dispatch")
        crash_and_restart("active_no_dispatch")
        harness.wait_for("active_no_dispatch startup recovery", lambda: run_recovery_settled(harness, no_dispatch))
        save_recovery_snapshot(harness, no_dispatch, "active_no_dispatch")

        harness.mock_state.set_control({sealed["workspaceId"]: "shell_wait"})
        harness.activate(sealed)
        harness.wait_for("provider request for sealed dispatch", lambda: wait_event_count(harness, sealed["workspaceId"]))

        def sealed_claimed() -> bool:
            _snapshot, _root, observed = harness.details(sealed)
            return any((item.get("executionState") in {"sealed", "claimed"}) for item in acceptance(observed).get("operations") or [])

        harness.wait_for("active sealed dispatch", sealed_claimed)
        sealed_item = pre_restart_fixture(harness, sealed, "active_sealed_dispatch")
        dispatches = (audit_of(harness.details(sealed)[2]).get("dispatches") or [])
        if not dispatches:
            raise RuntimeError("sealed fixture has no dispatch")
        sealed_item["preRestart"]["sealedDispatchId"] = dispatches[-1].get("id")
        sealed_item["preRestart"]["operationExecutions"] = [
            {
                "executionKey": item.get("executionKey"),
                "fence": item.get("fence"),
                "executionState": item.get("executionState"),
            }
            for item in acceptance(harness.details(sealed)[2]).get("operations") or []
            if item.get("executionKey")
        ]
        crash_and_restart("active_sealed_dispatch")
        harness.wait_for("active_sealed_dispatch startup recovery", lambda: run_recovery_settled(harness, sealed))
        save_recovery_snapshot(harness, sealed, "active_sealed_dispatch")

        # Result files are scoped beneath the Run. Pre-create only this fresh
        # fixture's result directory before activation and make it read-only;
        # terminal aggregate/run persistence remains writable in its parent.
        result_dir = harness.data_dir / "internal" / "workspaces" / terminal["workspaceId"] / "runs" / "run-001" / "results"
        result_dir.mkdir(parents=True, exist_ok=True)
        result_dir.chmod(0o555)
        save_json(artifact_dir / "terminal-result-directory.json", {"path": str(result_dir), "mode": "0555"})
        harness.mock_state.set_control({})
        harness.activate(terminal)

        def terminal_pending() -> bool:
            _snapshot, root, observed = harness.details(terminal)
            audit = audit_of(observed)
            return bool((audit.get("terminal") or {}).get("code")) and (root.get("run") or {}).get("state") == "active"

        harness.wait_for("terminal delivery pending", terminal_pending)
        terminal_item = pre_restart_fixture(harness, terminal, "terminal_delivery_pending")
        terminal_audit = audit_of(harness.details(terminal)[2])
        terminal_item["preRestart"]["terminalCode"] = (terminal_audit.get("terminal") or {}).get("code")
        manifest = {"fixtures": [no_item, sealed_item, terminal_item]}
        errors = fixture_manifest_errors(manifest)
        if errors:
            raise RuntimeError("invalid produced restart manifest: " + "; ".join(errors))
        save_json(artifact_dir / "fixture-manifest.json", manifest)
        save_json(artifact_dir / "provider-events-pre-restart.json", harness.mock_state.snapshot())

        # Remove the external Result I/O fault while the service is down. The
        # final startup owns all terminal delivery recovery; no Resume or
        # activation is issued for this crashed Run.
        harness.stop_server(force=True)
        save_json(artifact_dir / "crash-terminal_delivery_pending.json", {"class": "terminal_delivery_pending", "signal": "SIGKILL", "at": time.time()})
        result_dir.chmod(0o755)
        harness.shell_release.touch()
        harness.mock_state.set_control({})
        harness.start_server()
        env = os.environ.copy()
        env.update({
            "WORKORCH_BASE_URL": harness.base_url,
            "WORKORCH_E2E_RESTART_FIXTURE_MANIFEST": str(artifact_dir / "fixture-manifest.json"),
            "WORKORCH_E2E_ARTIFACT_DIR": str(artifact_dir / "restart-verification"),
            "WORKORCH_E2E_TIMEOUT_SECONDS": "180",
        })
        subprocess.run([str(ROOT / "scripts" / "verify-runtime-v2-restart-recovery-live.sh")], cwd=ROOT, env=env, check=True)
        post_events = harness.mock_state.snapshot()
        save_json(artifact_dir / "provider-events-post-restart.json", post_events)
        request_errors = []
        for item in manifest["fixtures"]:
            workspace_id = item["workspaceId"]
            before = int(item["preRestart"]["providerRequestCount"])
            after = sum(1 for event in post_events if event.get("workspaceId") == workspace_id)
            if after != before:
                request_errors.append(f"{item['class']}: provider requests before/after restart={before}/{after}, want unchanged")
        if request_errors:
            raise RuntimeError("; ".join(request_errors))
        save_json(artifact_dir / "summary.json", {
            "status": "passed", "baseUrl": harness.base_url, "manifest": manifest,
            "providerRequests": {item["class"]: item["preRestart"]["providerRequestCount"] for item in manifest["fixtures"]},
            "guarantees": ["no manual runtime-data mutation", "three controlled crash/restart cycles", "no post-crash activation or resume", "provider requests did not replay"],
        })
        print(f"runtime-v2 C7 deployed restart harness passed artifacts={artifact_dir}")
    finally:
        harness.close()


def self_test() -> None:
    state = MockState(Path("/tmp"), Path("/tmp/release"))
    payload = {"messages": [{"content": """### C3.runtime-authority
{"transaction":{"authoritySnapshotId":"authority-1"},"outputContract":{"operationContracts":[{"kind":"file/edit","target":{"allowedTargetRefs":["target.txt"]}}]},"context":{"workspaceId":"ws-1","runId":"run-1","requiredState":"need_state_change","focus":{"id":"focus-001","coverageWorkItemIds":["cwi-1"]}}}

### C0.runtime-contract
stable
"""}]}
    candidate, mode = candidate_payload(state, payload)
    operation = (candidate.get("operations") or [{}])[0]
    if mode != "normal" or operation.get("kind") != "file/edit" or operation.get("targetRefs") != ["target.txt"]:
        raise AssertionError(f"C3 mock parser produced {candidate!r} mode={mode!r}")
    print("runtime-v2 C7 restart harness self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifact-dir", type=Path)
    parser.add_argument("--binary", type=Path, default=ROOT / "bin" / "workorch")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    if args.artifact_dir is None:
        parser.error("--artifact-dir is required unless --self-test is used")
    if not args.binary.is_file() or not os.access(args.binary, os.X_OK):
        raise SystemExit(f"workorch binary is not executable: {args.binary}")
    if (ROOT / ".workorch-e2e") in [args.artifact_dir, *args.artifact_dir.parents]:
        raise SystemExit("artifact directory must not be inside repository .workorch-e2e")
    run_harness(args.artifact_dir.resolve(), args.binary.resolve())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
