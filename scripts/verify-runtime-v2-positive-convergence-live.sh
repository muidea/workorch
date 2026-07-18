#!/usr/bin/env bash
# Runtime V2 deployed verification. Terminal mode accepts only bounded fixtures;
# liveness mode observes real workspaces without imposing a terminal SLA.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BASE_URL="${WORKORCH_BASE_URL:-http://127.0.0.1:8080}"
MODE="${WORKORCH_E2E_MODE:-terminal}"
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
  terminal|liveness) ;;
  *)
    echo "WORKORCH_E2E_MODE must be terminal or liveness, got $MODE" >&2
    exit 2
    ;;
esac
if [[ "$MODE" == "liveness" && "$ACTIVATE" != "0" ]]; then
  echo "liveness mode observes existing Runs and does not activate workspaces" >&2
  exit 2
fi
mkdir -p "$ARTIFACT_DIR"

echo "runtime-v2 positive-convergence live verifier mode=$MODE"
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
bounded dual-workspace fixture IDs not set.
Set WORKORCH_E2E_WORKSPACE_ID_A / WORKORCH_E2E_WORKSPACE_ID_B to the deployed
bounded positive fixtures (each <=2 focuses and <=4 coverage items/focus), optionally with:
  WORKORCH_E2E_PROVIDER_ID / WORKORCH_E2E_MODEL_ID
  WORKORCH_E2E_ACTIVATE=1 to issue explicit fixture activations before polling
  WORKORCH_E2E_POLL_INTERVAL_SECONDS / WORKORCH_E2E_TIMEOUT_SECONDS
Then re-run. File-backed sequences are covered by:
  make verify-runtime-v2-positive-convergence-file
MSG
  if [[ "${WORKORCH_E2E_LIVE_ALLOW_SKIP:-}" == "1" ]]; then
    echo "WORKORCH_E2E_LIVE_ALLOW_SKIP=1: skipping bounded dual-workspace terminal assertions"
    exit 0
  fi
  exit 3
fi

python3 - "$BASE_URL" "$ARTIFACT_DIR" "$WS_A" "$WS_B" "$POLL_INTERVAL" "$TIMEOUT" "$ACTIVATE" "$PROVIDER_ID" "$MODEL_ID" "$MODE" <<'PY'
import json, os, sys, time, urllib.request, urllib.error, uuid

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

def operation_receipt_errors(detail):
    observed = detail.get("observability") or {}
    operations = (observed.get("observability") or {}).get("acceptance", {}).get("operations") or []
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

initial_bodies = {}
baseline = {}
for ws, label in ((ws_a, "a"), (ws_b, "b")):
    body = snapshot(ws)
    initial_bodies[ws] = body
    baseline[ws] = summarize(ws, body)
    write_json(f"fixture-preflight-{label}.json", {
        "summary": baseline[ws],
        "acceptanceBounds": acceptance_bounds(body),
    })

if mode == "terminal":
    fixture_errors = []
    for ws, label in ((ws_a, "a"), (ws_b, "b")):
        bounds = acceptance_bounds(initial_bodies[ws])
        if bounds["focusCount"] > 2:
            fixture_errors.append(f"{label}: focusCount={bounds['focusCount']} exceeds terminal fixture limit 2")
        if bounds["maxCoverageItemsPerFocus"] > 4:
            fixture_errors.append(f"{label}: maxCoverageItemsPerFocus={bounds['maxCoverageItemsPerFocus']} exceeds terminal fixture limit 4")
    if fixture_errors:
        write_json("summary.json", {
            "mode": mode,
            "baseline": baseline,
            "errors": fixture_errors,
            "reason": "terminal mode requires bounded fixtures; use WORKORCH_E2E_MODE=liveness for real workspaces",
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

deadline = time.time() + timeout
results = {}
last = {}
timed_out = False
while time.time() < deadline:
    terminal = mode == "terminal"
    for ws, label in ((ws_a, "a"), (ws_b, "b")):
        body = snapshot(ws)
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
    if mode == "terminal" and terminal:
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
    errors.extend(f"{label}: {value}" for value in operation_receipt_errors(detail))
    if (s.get("projectionState") or "ready") not in ("ready", "ok", ""):
        errors.append(f"{label}: projectionState={s.get('projectionState')}")
    if mode == "terminal":
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
    else:
        before = baseline[ws]
        terminal_states = ("completed", "failed", "cancelled", "superseded")
        if before.get("runState") in terminal_states:
            if before.get("activeRunId") or s.get("activeRunId"):
                errors.append(f"{label}: terminal Run retained an activeRunId")
            continue
        legal_wait_states = ("blocked_on_decision", "blocked_on_approval")
        effective_progress = (
            s.get("acceptedEvidence", 0) > before.get("acceptedEvidence", 0) or
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

if timed_out and mode == "terminal":
    errors.append(f"timeout waiting {timeout}s for dual-workspace terminal runs")

write_json("summary.json", {
    "mode": mode,
    "timedOut": timed_out and mode == "terminal",
    "observationWindowElapsed": timed_out and mode == "liveness",
    "baseline": baseline,
    "results": results,
    "errors": errors,
    "projection": proj,
})

if errors:
    print(f"{mode} live verification failed:", "; ".join(errors), file=sys.stderr)
    sys.exit(1)

print(f"runtime-v2 positive-convergence {mode} verification passed")
for ws, item in results.items():
    print(ws, json.dumps(item, ensure_ascii=False))
PY
