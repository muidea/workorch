#!/usr/bin/env bash
# RVC-18: deployed dual-workspace positive-convergence live acceptance.
# Requires a running workorch instance at WORKORCH_BASE_URL and two real workspaces.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BASE_URL="${WORKORCH_BASE_URL:-http://127.0.0.1:8080}"
ARTIFACT_DIR="${WORKORCH_E2E_ARTIFACT_DIR:-.workorch-e2e/runtime-v2-positive-convergence-live/$(date -u +%Y%m%dT%H%M%SZ)}"
POLL_INTERVAL="${WORKORCH_E2E_POLL_INTERVAL_SECONDS:-2}"
TIMEOUT="${WORKORCH_E2E_TIMEOUT_SECONDS:-180}"
PROVIDER_ID="${WORKORCH_E2E_PROVIDER_ID:-}"
MODEL_ID="${WORKORCH_E2E_MODEL_ID:-}"
ACTIVATE="${WORKORCH_E2E_ACTIVATE:-0}"
mkdir -p "$ARTIFACT_DIR"

echo "runtime-v2 positive-convergence live verifier"
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
live dual-workspace fixture IDs not set.
Set WORKORCH_E2E_WORKSPACE_ID_A / WORKORCH_E2E_WORKSPACE_ID_B to the deployed
ai-proxy and LSP-Bridge workspaces, optionally with:
  WORKORCH_E2E_PROVIDER_ID / WORKORCH_E2E_MODEL_ID
  WORKORCH_E2E_ACTIVATE=1 to issue resume activations before polling
  WORKORCH_E2E_POLL_INTERVAL_SECONDS / WORKORCH_E2E_TIMEOUT_SECONDS
Then re-run. File-backed sequences are covered by:
  make verify-runtime-v2-positive-convergence-file
MSG
  if [[ "${WORKORCH_E2E_LIVE_ALLOW_SKIP:-}" == "1" ]]; then
    echo "WORKORCH_E2E_LIVE_ALLOW_SKIP=1: skipping dual-workspace live assertions"
    exit 0
  fi
  exit 3
fi

python3 - "$BASE_URL" "$ARTIFACT_DIR" "$WS_A" "$WS_B" "$POLL_INTERVAL" "$TIMEOUT" "$ACTIVATE" "$PROVIDER_ID" "$MODEL_ID" <<'PY'
import json, os, sys, time, urllib.request, urllib.error, uuid

base, artifact_dir, ws_a, ws_b, poll_s, timeout_s, activate, provider_id, model_id = sys.argv[1:10]
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
        "completedDispatches": rv2.get("completedDispatchCount"),
        "candidateAttempts": rv2.get("candidateAttemptCount"),
        "usageEntries": usage.get("entryTotal"),
        "usageTokens": usage.get("totalTokens"),
        "outcome": outcome.get("state"),
    }

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
while time.time() < deadline:
    terminal = True
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
    if terminal:
        results = dict(last)
        break
    time.sleep(poll)
else:
    results = dict(last)
    raise SystemExit("timeout waiting for dual-workspace terminal runs")

# Final snapshots + global readyz
readyz = get("/readyz")
with open(os.path.join(artifact_dir, "readyz-final.json"), "w") as f:
    json.dump(readyz, f, indent=2)

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
    with open(os.path.join(artifact_dir, f"final-{label}.json"), "w") as f:
        json.dump(body, f, indent=2)
    s = summarize(ws, body)
    results[ws] = s
    if s.get("runState") != "completed":
        errors.append(f"{label}: runState={s.get('runState')} fail={s.get('failureReason')}")
    if s.get("activeRunId"):
        errors.append(f"{label}: activeRunId still set ({s.get('activeRunId')})")
    if s.get("outcome") not in ("accepted", "completed", None) and s.get("disposition") not in ("accepted", "satisfied"):
        # accepted outcome is preferred; disposition accepted/satisfied also counts.
        if s.get("action") != "complete":
            errors.append(f"{label}: outcome={s.get('outcome')} disposition={s.get('disposition')} action={s.get('action')}")
    if not s.get("acceptedEvidence") or int(s.get("acceptedEvidence") or 0) < 1:
        errors.append(f"{label}: acceptedEvidence={s.get('acceptedEvidence')}")
    if not s.get("usageEntries") or int(s.get("usageEntries") or 0) < 1:
        errors.append(f"{label}: llmUsageSummary empty")
    if (s.get("projectionState") or "ready") not in ("ready", "ok", ""):
        errors.append(f"{label}: projectionState={s.get('projectionState')}")
    # Prefer complete action for positive convergence.
    if s.get("action") not in ("complete", "fail", None) and s.get("runState") == "completed":
        pass
    if s.get("runState") == "completed" and s.get("action") not in ("complete", None, ""):
        # allow empty action field on some projections
        if s.get("action") not in ("complete",):
            errors.append(f"{label}: expected action=complete got {s.get('action')}")

with open(os.path.join(artifact_dir, "summary.json"), "w") as f:
    json.dump({"results": results, "errors": errors, "projection": proj}, f, indent=2)

if errors:
    print("live verification failed:", "; ".join(errors), file=sys.stderr)
    sys.exit(1)

print("runtime-v2 positive-convergence live verification passed")
for ws, item in results.items():
    print(ws, json.dumps(item, ensure_ascii=False))
PY
