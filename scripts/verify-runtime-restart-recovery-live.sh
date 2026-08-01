#!/usr/bin/env bash
# Validate deployed Runtime SM-4 restart recovery without issuing Resume or
# Activate. Fixture creation and the service restart are deliberately external:
# this verifier is read-only against the deployed instance.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ "${1:-}" == "--self-test" ]]; then
  exec python3 "$ROOT_DIR/scripts/verify_runtime_restart_contract.py" --self-test
fi

BASE_URL="${WORKORCH_BASE_URL:-http://127.0.0.1:8080}"
MANIFEST="${WORKORCH_E2E_RESTART_FIXTURE_MANIFEST:-}"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_E2E_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-restart-recovery-live/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_E2E_ARTIFACT_DIR="$(realpath -m "$ROOT_DIR/.workorch-e2e")"
POLL_INTERVAL="${WORKORCH_E2E_POLL_INTERVAL_SECONDS:-2}"
TIMEOUT="${WORKORCH_E2E_TIMEOUT_SECONDS:-180}"

if [[ -z "$MANIFEST" || ! -f "$MANIFEST" ]]; then
  echo "set WORKORCH_E2E_RESTART_FIXTURE_MANIFEST to a fresh fixture manifest" >&2
  exit 2
fi
if [[ "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR" || "$ARTIFACT_DIR" == "$REPOSITORY_E2E_ARTIFACT_DIR/"* ]]; then
  echo "WORKORCH_E2E_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi
mkdir -p "$ARTIFACT_DIR"
cp "$MANIFEST" "$ARTIFACT_DIR/fixture-manifest.json"

echo "runtime restart recovery live verifier"
echo "base_url=$BASE_URL manifest=$MANIFEST artifacts=$ARTIFACT_DIR"

curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/healthz" >"$ARTIFACT_DIR/healthz.json"
curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/readyz" >"$ARTIFACT_DIR/readyz.json"

python3 - "$BASE_URL" "$ARTIFACT_DIR" "$POLL_INTERVAL" "$TIMEOUT" <<'PY'
import json
import os
import sys
import time
import urllib.request

sys.path.insert(0, os.path.join(os.getcwd(), "scripts"))
from verify_runtime_restart_contract import (
    fixture_final_errors,
    fixture_manifest_errors,
)

base, artifact_dir, poll_s, timeout_s = sys.argv[1:5]
poll = float(poll_s)
timeout = float(timeout_s)
manifest = json.load(open(os.path.join(artifact_dir, "fixture-manifest.json"), encoding="utf-8"))
if manifest_errors := fixture_manifest_errors(manifest):
    raise SystemExit("invalid restart fixture manifest: " + "; ".join(manifest_errors))
fixtures = manifest["fixtures"]

def request(path):
    req = urllib.request.Request(base.rstrip("/") + path, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=45) as response:
        return json.load(response)

def snapshot(item):
    return request(f"/api/v1/workorch/workspaces/{item['workspaceId']}/snapshot")

def run_snapshot(item):
    body = snapshot(item)
    root = body.get("snapshot") or body
    run = root.get("run") or {}
    if run.get("id") != item["runId"]:
        raise RuntimeError(f"{item['class']}: snapshot run={run.get('id')!r}, want {item['runId']!r}")
    return body, root, run

def projection_errors(body):
    projection = body.get("projection") or {}
    state = (projection.get("state") or "").lower()
    errors = []
    if state and state not in ("ready", "ok", "healthy", "up"):
        errors.append(f"projection state={state}")
    source = projection.get("sourceEventRevision")
    projected = projection.get("projectedSourceRevision")
    if source is not None and projected is not None and source != projected:
        errors.append(f"projection revision source={source} projected={projected}")
    return errors

def runtime_observability(item, root):
    run = root.get("run") or {}
    run_id = run.get("id")
    if not run_id:
        raise RuntimeError("missing run id")
    try:
        body = request(f"/api/v1/workorch/workspaces/{item['workspaceId']}/runs/{run_id}/observability")
    except Exception as exc:
        raise RuntimeError(f"observability read failed: {exc}") from exc
    observed = body.get("observability") or {}
    return observed.get("acceptance") or {}

def receipt_errors(operations):
    seen = set()
    errors = []
    for operation in operations:
        key = (operation.get("executionKey"), operation.get("fence"))
        if key[0] and key in seen:
            errors.append(f"duplicate owner receipt executionKey/fence={key[0]}/{key[1]}")
        seen.add(key)
    return errors

baseline = {}
for item in fixtures:
    body, root, run = run_snapshot(item)
    baseline[item["class"]] = {
        "runState": run.get("state"),
        "activeRunId": (root.get("workspace") or {}).get("activeRunId") or (root.get("workspace") or {}).get("activeRunID"),
        "projectionErrors": projection_errors(body),
    }
with open(os.path.join(artifact_dir, "baseline.json"), "w", encoding="utf-8") as handle:
    json.dump(baseline, handle, indent=2)

deadline = time.monotonic() + timeout
last = {}
while True:
    all_converged = True
    errors = []
    for item in fixtures:
        cls = item["class"]
        try:
            body, root, run = run_snapshot(item)
            workspace = root.get("workspace") or {}
            state = (run.get("state") or "").lower()
            active_run = workspace.get("activeRunId") or workspace.get("activeRunID") or ""
            acceptance = runtime_observability(item, root)
            audit = acceptance.get("audit")
            operations = acceptance.get("operations") or []
            current = {
                "runState": state,
                "activeRunId": active_run,
                "projectionErrors": projection_errors(body),
                "receiptErrors": receipt_errors(operations),
                "transitionErrors": fixture_final_errors(item, state, active_run, audit, operations),
                "snapshot": body,
            }
            last[cls] = current
            if current["projectionErrors"] or current["receiptErrors"] or current["transitionErrors"]:
                errors.extend(f"{cls}: {error}" for error in current["projectionErrors"] + current["receiptErrors"] + current["transitionErrors"])
                all_converged = False
                continue
        except Exception as exc:
            errors.append(f"{cls}: {exc}")
            all_converged = False
    if all_converged:
        break
    if time.monotonic() >= deadline:
        with open(os.path.join(artifact_dir, "summary.json"), "w", encoding="utf-8") as handle:
            json.dump({"baseline": baseline, "last": last, "errors": errors}, handle, indent=2)
        raise SystemExit("restart recovery did not converge: " + "; ".join(errors or ["fixtures remained active"]))
    time.sleep(poll)

with open(os.path.join(artifact_dir, "summary.json"), "w", encoding="utf-8") as handle:
    json.dump({"baseline": baseline, "final": last, "status": "passed"}, handle, indent=2)
print("runtime deployed restart recovery verification passed")
PY
