#!/usr/bin/env bash
# Runtime V2 deployed verification. Positive acceptance requires completed Runs;
# bounded terminal mode additionally constrains the fixture shape, while liveness
# observes real workspaces without claiming accepted convergence.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ "${1:-}" == "--self-test" ]]; then
  exec python3 "$ROOT_DIR/scripts/verify_runtime_v2_live_contract.py" --self-test
fi

BASE_URL="${WORKORCH_BASE_URL:-http://127.0.0.1:8080}"
MODE="${WORKORCH_E2E_MODE:-positive}"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_E2E_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-v2-positive-convergence-live/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_E2E_ARTIFACT_DIR="$(realpath -m "$ROOT_DIR/.workorch-e2e")"
if [[ "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR" || "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR/"* ]]; then
  echo "WORKORCH_E2E_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi
POLL_INTERVAL="${WORKORCH_E2E_POLL_INTERVAL_SECONDS:-2}"
TIMEOUT="${WORKORCH_E2E_TIMEOUT_SECONDS:-180}"
PROVIDER_ID="${WORKORCH_E2E_PROVIDER_ID:-}"
MODEL_ID="${WORKORCH_E2E_MODEL_ID:-}"
ACTIVATE="${WORKORCH_E2E_ACTIVATE:-0}"
case "$MODE" in
  positive|bounded_terminal|liveness) ;;
  *)
    echo "WORKORCH_E2E_MODE must be positive, bounded_terminal, or liveness, got $MODE" >&2
    exit 2
    ;;
esac
if [[ "$MODE" == "liveness" && "$ACTIVATE" != "0" ]]; then
  echo "liveness mode observes existing Runs and does not activate workspaces" >&2
  exit 2
fi
if [[ "$MODE" != "liveness" && "$ACTIVATE" != "1" ]]; then
  echo "positive and bounded_terminal modes require WORKORCH_E2E_ACTIVATE=1 so the verifier can prove a fresh Run" >&2
  exit 2
fi
mkdir -p "$ARTIFACT_DIR"

echo "runtime-v2 live verifier mode=$MODE"
echo "base_url=$BASE_URL artifacts=$ARTIFACT_DIR poll=${POLL_INTERVAL}s timeout=${TIMEOUT}s"

if ! curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/healthz" >"$ARTIFACT_DIR/healthz.json"; then
  echo "deployed instance not reachable at $BASE_URL/healthz" >&2
  echo "start workorch with --server and re-run this script" >&2
  exit 2
fi

if ! curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/readyz" >"$ARTIFACT_DIR/readyz.json"; then
  echo "deployed instance readyz not reachable" >&2
  exit 2
fi

python3 - "$ARTIFACT_DIR/readyz.json" <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
proj = data.get("projection") or {}
state = (proj.get("state") or data.get("state") or data.get("status") or "").lower()
print(f"readyz projection.state={state or 'unknown'} source={proj.get('sourceEventRevision')} projected={proj.get('projectedSourceRevision')}")
if state and state not in ("ready", "ok", "healthy", "up"):
    print(f"projection is not ready: {state}", file=sys.stderr)
    sys.exit(1)
if proj.get("sourceEventRevision") is not None and proj.get("projectedSourceRevision") is not None:
    if proj.get("sourceEventRevision") != proj.get("projectedSourceRevision"):
        print(
            f"projection revision lag: source={proj.get('sourceEventRevision')} projected={proj.get('projectedSourceRevision')}",
            file=sys.stderr,
        )
        sys.exit(1)
PY

WS_A="${WORKORCH_E2E_WORKSPACE_ID_A:-}"
WS_B="${WORKORCH_E2E_WORKSPACE_ID_B:-}"
if [[ -z "$WS_A" || -z "$WS_B" ]]; then
  cat <<MSG
Runtime V2 workspace IDs not set.
Set WORKORCH_E2E_WORKSPACE_ID_A / WORKORCH_E2E_WORKSPACE_ID_B to the deployed
fresh fixtures, with:
  WORKORCH_E2E_PROVIDER_ID / WORKORCH_E2E_MODEL_ID
  WORKORCH_E2E_ACTIVATE=1 to issue required explicit fixture activations before polling
  WORKORCH_E2E_POLL_INTERVAL_SECONDS / WORKORCH_E2E_TIMEOUT_SECONDS
Then re-run. File-backed sequences are covered by:
  make verify-runtime-v2-positive-convergence-file
MSG
  if [[ "${WORKORCH_E2E_LIVE_ALLOW_SKIP:-}" == "1" ]]; then
    echo "WORKORCH_E2E_LIVE_ALLOW_SKIP=1: skipping live verification"
    exit 0
  fi
  exit 3
fi

python3 - "$BASE_URL" "$ARTIFACT_DIR" "$WS_A" "$WS_B" "$POLL_INTERVAL" "$TIMEOUT" "$ACTIVATE" "$PROVIDER_ID" "$MODEL_ID" "$MODE" <<'PY'
import json, os, sys, time, urllib.request, urllib.error, uuid

sys.path.insert(0, os.path.join(os.getcwd(), "scripts"))
from verify_runtime_v2_live_contract import bounded_git_sequence_errors, external_provider_unavailable_reason, provider_ceiling_trace_errors, runtime_v2_audit_errors, runtime_v2_completion_sequence_errors

base, artifact_dir, ws_a, ws_b, poll_s, timeout_s, activate, provider_id, model_id, mode = sys.argv[1:11]
poll = float(poll_s)
timeout = float(timeout_s)
do_activate = activate in ("1", "true", "TRUE", "yes", "YES")

def request(method, path, body=None):
    url = base.rstrip("/") + path
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=45) as resp:
        return resp.status, json.load(resp)

def get(path):
    _, body = request("GET", path)
    return body

def snapshot(ws):
    return get(f"/api/v1/workorch/workspaces/{ws}/snapshot")

def summarize(ws, body):
    snap = body.get("snapshot") or body
    proj = body.get("projection") or {}
    w = snap.get("workspace") or {}
    run = snap.get("run") or {}
    conv = snap.get("convergenceStatus") or {}
    usage = snap.get("llmUsageSummary") or {}
    tp = snap.get("taskProgress") or {}
    rv2 = tp.get("runtimeV2") or {}
    outcome = snap.get("outcome") or {}
    return {
        "workspaceId": ws,
        "title": w.get("title"),
        "activeRunId": w.get("activeRunId") or w.get("activeRunID"),
        "projectionState": (proj.get("state") or "").lower(),
        "runId": run.get("id"),
        "runState": run.get("state"),
        "failureReason": run.get("failureReason") or "",
        "focusId": conv.get("currentFocusId") or rv2.get("currentFocusId"),
        "action": conv.get("nextActionKind") or rv2.get("currentAction"),
        "disposition": conv.get("assessmentDisposition"),
        "repairAttempts": conv.get("repairAttempts"),
        "remaining": conv.get("remainingCoverageItemCount"),
        "closed": conv.get("closedCoverageItemCount"),
        "gapSummary": conv.get("gapSummary") or "",
        "stateRevision": rv2.get("stateRevision"),
        "acceptedEvidence": rv2.get("acceptedEvidenceCount"),
        "rejectedEvidence": rv2.get("rejectedEvidenceCount"),
        "completedDispatches": rv2.get("completedDispatchCount"),
        "candidateAttempts": rv2.get("candidateAttemptCount"),
        "usageEntries": usage.get("entryTotal"),
        "usageTokens": usage.get("totalTokens"),
        "outcome": outcome.get("state"),
    }

def write_json(name, value):
    with open(os.path.join(artifact_dir, name), "w") as f:
        json.dump(value, f, indent=2)

def acceptance_bounds(body):
    snap = body.get("snapshot") or body
    acceptance = snap.get("acceptance") or {}
    slices = acceptance.get("slices") or []
    focus_sizes = [len(item.get("coverageWorkItemIds") or item.get("coverageWorkItemIDs") or []) for item in slices]
    return {
        "focusCount": len(slices),
        "maxCoverageItemsPerFocus": max(focus_sizes, default=0),
        "coverageItemsPerFocus": focus_sizes,
    }

def detail_snapshot(ws, label, body):
    snap = body.get("snapshot") or body
    run = snap.get("run") or {}
    detail = {
        "workspaceId": ws,
        "snapshot": body,
        "acceptanceBounds": acceptance_bounds(body),
        "governance": {
            "decisions": snap.get("decisions") or [],
            "approvals": snap.get("approvals") or [],
        },
    }
    run_id = run.get("id")
    if run_id:
        try:
            observability = get(f"/api/v1/workorch/workspaces/{ws}/runs/{run_id}/observability")
            detail["observability"] = observability
            observed = observability.get("observability") or {}
            detail["operationReceipts"] = (observed.get("acceptance") or {}).get("operations") or []
            detail["usageAudit"] = observed.get("llmUsageTraces") or []
        except Exception as exc:
            detail["observabilityError"] = str(exc)
    write_json(f"final-detail-{label}.json", detail)
    return detail

def acceptance_from_detail(detail):
    observed = detail.get("observability") or {}
    root = observed.get("observability") or {}
    acceptance = root.get("acceptance") or {}
    if acceptance:
        return acceptance
    snap = detail.get("snapshot") or {}
    return snap.get("acceptance") or {}

def operation_receipt_errors(detail):
    operations = acceptance_from_detail(detail).get("operations") or []
    seen = set()
    errors = []
    for operation in operations:
        key = (operation.get("executionKey"), operation.get("fence"))
        if key[0] and key in seen:
            errors.append(f"duplicate operation receipt executionKey/fence={key[0]}/{key[1]}")
        seen.add(key)
        if operation.get("executionState") == "outcome_unknown":
            errors.append(f"operation {operation.get('operationId')} has outcome_unknown")
    return errors

def runtime_audit_errors(detail, workspace_id, run_id, require_terminal):
    acceptance = acceptance_from_detail(detail)
    return runtime_v2_audit_errors(acceptance.get("audit"), workspace_id, run_id, require_terminal)

def provider_ceiling_audit_errors(detail):
    acceptance = acceptance_from_detail(detail)
    return provider_ceiling_trace_errors(acceptance.get("audit"), detail.get("usageAudit") or [])

def completion_sequence_errors(detail, require_focus_advance):
    return runtime_v2_completion_sequence_errors(acceptance_from_detail(detail).get("audit"), require_focus_advance)

def positive_acceptance_errors(detail, bounded):
    acceptance = acceptance_from_detail(detail)
    summary = acceptance.get("summary") or {}
    operations = acceptance.get("operations") or []
    errors = []
    accepted_slices = int(summary.get("acceptedSlices") or 0)
    if accepted_slices < 1:
        errors.append(f"acceptedSlices={summary.get('acceptedSlices')}, want at least 1")
    if bounded and accepted_slices != 2:
        errors.append(f"acceptedSlices={summary.get('acceptedSlices')}, want 2 for bounded terminal acceptance")
    durable = any(
        item.get("outcome") == "accepted" and item.get("observedEffectScope") in ("workspace_durable", "remote_durable")
        for item in operations
    )
    if not durable:
        errors.append("no accepted durable owner receipt")
    return errors

def governance_binding_errors(detail):
    snap = detail.get("snapshot") or {}
    run = snap.get("run") or {}
    task = snap.get("taskProgress") or {}
    ticket_id = run.get("blockingTicketId") or task.get("blockingTicketId")
    if not ticket_id:
        return []
    governance = acceptance_from_detail(detail).get("governance") or []
    binding = next((item for item in governance if item.get("ticketId") == ticket_id), None)
    if binding is None:
        return [f"blocking ticket {ticket_id} has no Runtime V2 aggregate governance binding"]
    decisions = (detail.get("governance") or {}).get("decisions") or []
    decision = next((item for item in decisions if item.get("id") == ticket_id), None)
    if decision is None:
        return [f"blocking ticket {ticket_id} is absent from workspace governance projection"]
    state = (decision.get("state") or "").lower()
    if state in ("resolved", "completed"):
        if not binding.get("resolutionId"):
            return [f"ticket {ticket_id} is resolved but aggregate has no matching Runtime V2 resolution"]
        decision_resolution = decision.get("resolutionId")
        if decision_resolution and decision_resolution != binding.get("resolutionId"):
            return [f"ticket {ticket_id} resolution mismatch decision={decision_resolution} aggregate={binding.get('resolutionId')}"]
    return []

initial_bodies = {}
baseline = {}
capability_state_observations = {ws_a: [], ws_b: []}

def observe_capability_states(ws, body):
    snap = body.get("snapshot") or body
    acceptance = snap.get("acceptance") or {}
    states = acceptance.get("capabilityStates") or []
    for item in states:
        value = {
            "snapshotId": item.get("snapshotId"),
            "requirementId": item.get("requirementId"),
            "disposition": item.get("disposition"),
            "requiredOperationSequence": item.get("requiredOperationSequence") or [],
        }
        if value not in capability_state_observations[ws]:
            capability_state_observations[ws].append(value)

for ws, label in ((ws_a, "a"), (ws_b, "b")):
    body = snapshot(ws)
    initial_bodies[ws] = body
    baseline[ws] = summarize(ws, body)
    observe_capability_states(ws, body)
    write_json(f"fixture-preflight-{label}.json", {
        "summary": baseline[ws],
        "acceptanceBounds": acceptance_bounds(body),
    })

if mode == "bounded_terminal":
    fixture_errors = []
    for ws, label in ((ws_a, "a"), (ws_b, "b")):
        bounds = acceptance_bounds(initial_bodies[ws])
        if not do_activate and bounds["focusCount"] != 2:
            fixture_errors.append(f"{label}: focusCount={bounds['focusCount']} must equal 2 for the bounded dual-focus terminal gate")
        if bounds["focusCount"] > 2:
            fixture_errors.append(f"{label}: focusCount={bounds['focusCount']} exceeds terminal fixture limit 2")
        if bounds["maxCoverageItemsPerFocus"] > 4:
            fixture_errors.append(f"{label}: maxCoverageItemsPerFocus={bounds['maxCoverageItemsPerFocus']} exceeds terminal fixture limit 4")
    if fixture_errors:
        write_json("summary.json", {
            "mode": mode,
            "baseline": baseline,
            "errors": fixture_errors,
            "reason": "bounded_terminal mode requires bounded fixtures; use positive or liveness for other workspaces",
        })
        raise SystemExit("terminal fixture preflight failed: " + "; ".join(fixture_errors))

def activate_workspace(ws, label):
    body_snap = snapshot(ws)
    snap = body_snap.get("snapshot") or body_snap
    goal = (snap.get("goal") or {}).get("id") or (snap.get("workspace") or {}).get("goalId")
    dod = (snap.get("dod") or snap.get("doD") or {}).get("id") or (snap.get("workspace") or {}).get("doDId")
    payload = {
        "intent": "resume",
        "requestId": f"rvc18-live-{label}-{uuid.uuid4().hex[:12]}",
        "reason": "rvc18_positive_convergence_live_script",
        "source": "rvc18-live",
    }
    if goal:
        payload["goalId"] = goal
    if dod:
        payload["doDId"] = dod
    if provider_id and model_id:
        payload["llmTarget"] = {"providerId": provider_id, "modelId": model_id}
    status, resp = request("POST", f"/api/v1/workorch/workspaces/{ws}/execution-activations", payload)
    with open(os.path.join(artifact_dir, f"activate-{label}.json"), "w") as f:
        json.dump({"status": status, "request": payload, "response": resp}, f, indent=2)
    if status not in (200, 202) or not (resp.get("accepted") is True or resp.get("action") == "execution-activation"):
        raise RuntimeError(f"activation failed for {ws}: status={status} body={resp}")
    print(f"activated {label} requestId={payload['requestId']} status={status}")

if do_activate:
    activate_workspace(ws_a, "a")
    activate_workspace(ws_b, "b")

# Positive acceptance is evidence for this invocation only when both explicit
# activations actually create a new Run.  A prior completed Run could otherwise
# make an observation-only verifier pass without exercising the deployed binary
# or this activation's model/definition binding.
expected_run_ids = {}
if do_activate:
    fresh_run_deadline = time.time() + min(30.0, max(5.0, timeout))
    while time.time() < fresh_run_deadline and len(expected_run_ids) < 2:
        for ws, label in ((ws_a, "a"), (ws_b, "b")):
            if ws in expected_run_ids:
                continue
            body = snapshot(ws)
            observe_capability_states(ws, body)
            current = summarize(ws, body).get("runId")
            previous = baseline[ws].get("runId")
            if current and current != previous:
                expected_run_ids[ws] = current
                write_json(f"fresh-run-{label}.json", {
                    "previousRunId": previous,
                    "activatedRunId": current,
                    "summary": summarize(ws, body),
                })
        if len(expected_run_ids) < 2:
            time.sleep(min(poll, 1.0))
    if len(expected_run_ids) != 2:
        missing = [label for ws, label in ((ws_a, "a"), (ws_b, "b")) if ws not in expected_run_ids]
        write_json("summary.json", {
            "mode": mode,
            "baseline": baseline,
            "freshRunIDs": expected_run_ids,
            "errors": [f"activation did not produce a fresh Run for fixtures {missing}"],
        })
        raise SystemExit("fresh Run preflight failed: activation did not replace the baseline Run for " + ", ".join(missing))

# A workspace has no authoritative focus slices until its fresh Runtime V2 Run
# is activated. Recheck the bound after activation so bounded terminal mode cannot
# accidentally accept an uncompiled or oversized fixture merely because its
# pre-activation snapshot had no slices yet.
if mode == "bounded_terminal" and do_activate:
    activated_fixture_errors = []
    activated_bodies = {}
    for ws, label in ((ws_a, "a"), (ws_b, "b")):
        # Activation schedules the Run asynchronously.  Wait only for the
        # authoritative acceptance slices to appear; this is a fixture
        # compilation preflight, not part of the 180-second terminal SLA.
        preflight_deadline = time.time() + min(30.0, max(5.0, timeout))
        body = snapshot(ws)
        while acceptance_bounds(body)["focusCount"] == 0 and time.time() < preflight_deadline:
            time.sleep(min(poll, 1.0))
            body = snapshot(ws)
        observe_capability_states(ws, body)
        activated_bodies[ws] = body
        bounds = acceptance_bounds(body)
        write_json(f"fixture-preflight-activated-{label}.json", {
            "summary": summarize(ws, body),
            "acceptanceBounds": bounds,
        })
        if bounds["focusCount"] != 2:
            activated_fixture_errors.append(f"{label}: focusCount={bounds['focusCount']} after activation, want exactly 2 for the bounded dual-focus gate")
        if bounds["maxCoverageItemsPerFocus"] > 4:
            activated_fixture_errors.append(f"{label}: maxCoverageItemsPerFocus={bounds['maxCoverageItemsPerFocus']} exceeds terminal fixture limit 4 after activation")
    if activated_fixture_errors:
        write_json("summary.json", {
            "mode": mode,
            "baseline": baseline,
            "errors": activated_fixture_errors,
            "reason": "activated bounded terminal fixtures must expose exactly two bounded focus slices",
        })
        raise SystemExit("terminal fixture activated preflight failed: " + "; ".join(activated_fixture_errors))

deadline = time.time() + timeout
results = {}
last = {}
timed_out = False
while time.time() < deadline:
    terminal = mode != "liveness"
    for ws, label in ((ws_a, "a"), (ws_b, "b")):
        body = snapshot(ws)
        observe_capability_states(ws, body)
        s = summarize(ws, body)
        last[ws] = s
        with open(os.path.join(artifact_dir, f"progress-{label}.json"), "w") as f:
            json.dump(s, f, indent=2)
        if s.get("runState") not in ("completed", "failed", "cancelled", "superseded") or s.get("activeRunId"):
            terminal = False
    print(
        time.strftime("%H:%M:%S"),
        " | ".join(
            f"{(last[ws].get('title') or ws)}: {last[ws].get('runId')}/{last[ws].get('runState')} "
            f"act={last[ws].get('action')} rev={last[ws].get('stateRevision')} "
            f"ev={last[ws].get('acceptedEvidence')} outcome={last[ws].get('outcome')}"
            for ws in (ws_a, ws_b)
        ),
        flush=True,
    )
    if mode != "liveness" and terminal:
        results = dict(last)
        break
    time.sleep(poll)
else:
    timed_out = True
    results = dict(last)

# Final snapshots, owner receipts, governance and usage audit are retained for both
# terminal failures and liveness observations.
readyz = get("/readyz")
write_json("readyz-final.json", readyz)

errors = []
external_provider_failures = []
proj = readyz.get("projection") or {}
if (proj.get("state") or "").lower() != "ready":
    errors.append(f"global projection state={proj.get('state')}")
if proj.get("sourceEventRevision") != proj.get("projectedSourceRevision"):
    errors.append(
        f"projection revision mismatch source={proj.get('sourceEventRevision')} projected={proj.get('projectedSourceRevision')}"
    )

for ws, label in ((ws_a, "a"), (ws_b, "b")):
    body = snapshot(ws)
    write_json(f"final-{label}.json", body)
    detail = detail_snapshot(ws, label, body)
    s = summarize(ws, body)
    results[ws] = s
    expected_run_id = expected_run_ids.get(ws)
    if expected_run_id and s.get("runId") != expected_run_id:
        errors.append(f"{label}: final runId={s.get('runId')} differs from activated fresh Run {expected_run_id}")
    errors.extend(f"{label}: {value}" for value in operation_receipt_errors(detail))
    errors.extend(f"{label}: {value}" for value in governance_binding_errors(detail))
    if (s.get("projectionState") or "ready") not in ("ready", "ok", ""):
        errors.append(f"{label}: projectionState={s.get('projectionState')}")
    if mode != "liveness":
        provider_unavailable = external_provider_unavailable_reason(s.get("failureReason"))
        if provider_unavailable:
            external_provider_failures.append({"workspaceId": ws, "runId": s.get("runId"), "reason": provider_unavailable})
            errors.append(f"{label}: external_provider_unavailable: {provider_unavailable}")
            if s.get("runState") != "failed":
                errors.append(f"{label}: provider transport terminal has runState={s.get('runState')}, want failed")
            if s.get("activeRunId"):
                errors.append(f"{label}: provider transport terminal retained activeRunId ({s.get('activeRunId')})")
            # Transport exhaustion occurs before a complete candidate document;
            # all acceptance/audit requirements below are intentionally not
            # applicable to this non-success P6 invocation.
            continue
        errors.extend(f"{label}: {value}" for value in runtime_audit_errors(detail, ws, s.get("runId") or "", True))
        errors.extend(f"{label}: {value}" for value in provider_ceiling_audit_errors(detail))
        errors.extend(f"{label}: {value}" for value in completion_sequence_errors(detail, mode == "bounded_terminal"))
        if s.get("runState") != "completed":
            errors.append(f"{label}: runState={s.get('runState')} fail={s.get('failureReason')}")
        if s.get("activeRunId"):
            errors.append(f"{label}: activeRunId still set ({s.get('activeRunId')})")
        if s.get("outcome") not in ("accepted", "completed", None) and s.get("disposition") not in ("accepted", "satisfied"):
            if s.get("action") != "complete":
                errors.append(f"{label}: outcome={s.get('outcome')} disposition={s.get('disposition')} action={s.get('action')}")
        if not s.get("acceptedEvidence") or int(s.get("acceptedEvidence") or 0) < 1:
            errors.append(f"{label}: acceptedEvidence={s.get('acceptedEvidence')}")
        if not s.get("usageEntries") or int(s.get("usageEntries") or 0) < 1:
            errors.append(f"{label}: llmUsageSummary empty")
        if s.get("runState") == "completed" and s.get("action") not in ("complete", None, ""):
            errors.append(f"{label}: expected action=complete got {s.get('action')}")
        errors.extend(f"{label}: {value}" for value in positive_acceptance_errors(detail, mode == "bounded_terminal"))
        if mode == "bounded_terminal" and label == "a":
            errors.extend(
                f"{label}: {value}"
                for value in bounded_git_sequence_errors(acceptance_from_detail(detail).get("operations") or [])
            )
    else:
        before = baseline[ws]
        terminal_states = ("completed", "failed", "cancelled", "superseded")
        if before.get("runState") in terminal_states:
            if before.get("activeRunId") or s.get("activeRunId"):
                errors.append(f"{label}: terminal Run retained an activeRunId")
            continue
        legal_wait_states = ("blocked_on_decision", "blocked_on_approval")
        effective_progress = (
            int(s.get("acceptedEvidence") or 0) > int(before.get("acceptedEvidence") or 0) or
            s.get("focusId") != before.get("focusId") or
            (s.get("runState") != before.get("runState") and s.get("runState") in terminal_states + legal_wait_states)
        )
        if not effective_progress:
            errors.append(
                f"{label}: no effective progress (accepted evidence, focus advance, or legal lifecycle transition); "
                f"revision {before.get('stateRevision')}->{s.get('stateRevision')} "
                f"dispatches {before.get('completedDispatches')}->{s.get('completedDispatches')} "
                f"rejectedEvidence {before.get('rejectedEvidence')}->{s.get('rejectedEvidence')}"
            )
        if s.get("runState") in terminal_states and s.get("activeRunId"):
            errors.append(f"{label}: terminal Run retained an activeRunId")

if timed_out and mode != "liveness":
    errors.append(f"timeout waiting {timeout}s for dual-workspace terminal runs")

classification = ""
if mode == "liveness":
    terminal_states = ("completed", "failed", "cancelled", "superseded")
    if any((item.get("runState") in terminal_states and item.get("runState") != "completed") for item in results.values()):
        classification = "safe_terminal_rejected"
    else:
        classification = "lifecycle_liveness"
elif mode == "bounded_terminal":
    classification = "bounded_terminal_acceptance"
elif external_provider_failures:
    classification = "external_provider_unavailable"
else:
    classification = "positive_acceptance"

write_json("summary.json", {
    "mode": mode,
    "classification": classification,
    "timedOut": timed_out and mode != "liveness",
    "observationWindowElapsed": timed_out and mode == "liveness",
    "baseline": baseline,
    "freshRunIDs": expected_run_ids,
    "results": results,
    "errors": errors,
    "externalProviderFailures": external_provider_failures,
    "capabilityStateObservations": capability_state_observations,
    "projection": proj,
})

if errors:
    print(f"{mode} live verification failed:", "; ".join(errors), file=sys.stderr)
    sys.exit(1)

print(f"runtime-v2 {classification} verification passed")
for ws, item in results.items():
    print(ws, json.dumps(item, ensure_ascii=False))
PY
