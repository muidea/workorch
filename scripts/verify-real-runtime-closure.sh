#!/usr/bin/env bash
# Verify a real-provider Runtime V2 positive run. This is the P6 real-provider
# gate: it observes only persisted Runtime V2 authority, audit and projection
# state, and deliberately has no retired continuation compatibility path.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_REAL_RUNTIME_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_REAL_RUNTIME_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-v2-real-provider/$(date -u +%Y%m%dT%H%M%SZ)}")"
PREFLIGHT_ONLY="${WORKORCH_REAL_RUNTIME_PREFLIGHT_ONLY:-0}"
WORKSPACE_A="${WORKORCH_REAL_RUNTIME_WORKSPACE_ID_A:-${WORKORCH_E2E_WORKSPACE_ID_A:-}}"
WORKSPACE_B="${WORKORCH_REAL_RUNTIME_WORKSPACE_ID_B:-${WORKORCH_E2E_WORKSPACE_ID_B:-}}"
PROVIDER_ID="${WORKORCH_REAL_RUNTIME_PROVIDER_ID:-${WORKORCH_E2E_PROVIDER_ID:-}}"
MODEL_ID="${WORKORCH_REAL_RUNTIME_MODEL_ID:-${WORKORCH_E2E_MODEL_ID:-}}"

if [[ "${1:-}" == "--self-test" ]]; then
  [[ -x "$ROOT_DIR/scripts/verify-runtime-v2-positive-convergence-live.sh" ]]
  [[ "$ARTIFACT_DIR" != "$ROOT_DIR/.workorch-e2e" && "$ARTIFACT_DIR" != "$ROOT_DIR/.workorch-e2e/"* ]]
  self_test_output="$(mktemp)"
  trap 'rm -f "$self_test_output"' EXIT
  set +e
  WORKORCH_REAL_RUNTIME_BASE_URL="http://127.0.0.1:1" "$ROOT_DIR/scripts/verify-real-runtime-closure.sh" >"$self_test_output" 2>&1
  self_test_status=$?
  set -e
  [[ $self_test_status -eq 2 ]]
  grep -q 'Set all real-provider fixture inputs' "$self_test_output"
  echo "runtime-v2 real-provider verifier self-test passed"
  exit 0
fi

if [[ "$PREFLIGHT_ONLY" != "1" && "$PREFLIGHT_ONLY" != "true" && "$PREFLIGHT_ONLY" != "yes" ]] &&
  [[ -z "$WORKSPACE_A" || -z "$WORKSPACE_B" || -z "$PROVIDER_ID" || -z "$MODEL_ID" ]]; then
  cat >&2 <<'MSG'
Set all real-provider fixture inputs before running this verifier:
  WORKORCH_REAL_RUNTIME_WORKSPACE_ID_A
  WORKORCH_REAL_RUNTIME_WORKSPACE_ID_B
  WORKORCH_REAL_RUNTIME_PROVIDER_ID
  WORKORCH_REAL_RUNTIME_MODEL_ID

The workspaces must be fresh Runtime V2 fixtures. This command explicitly
activates them and then verifies sealed provider ceiling, persisted authority,
owner receipts, evidence, assessment, terminal projection and high-watermark.
MSG
  exit 2
fi

mkdir -p "$ARTIFACT_DIR"
trap 'status=$?; if [[ $status -ne 0 ]]; then echo "runtime-v2 real-provider artifacts: $ARTIFACT_DIR" >&2; fi' EXIT

curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/healthz" >"$ARTIFACT_DIR/healthz.json"
curl -fsS --connect-timeout 5 --max-time 15 "$BASE_URL/readyz" >"$ARTIFACT_DIR/readyz.json"
python3 - "$ARTIFACT_DIR/readyz.json" <<'PY'
import json
import sys

readyz = json.load(open(sys.argv[1], encoding="utf-8"))
projection = readyz.get("projection") or {}
state = (projection.get("state") or readyz.get("status") or "").lower()
if state not in {"ready", "ok", "healthy", "up"}:
    raise SystemExit(f"service/projection is not ready: {state or 'unknown'}")
source = projection.get("sourceEventRevision")
projected = projection.get("projectedSourceRevision")
if source is not None and projected is not None and source != projected:
    raise SystemExit(f"projection revision lag: source={source} projected={projected}")
PY

if [[ "$PREFLIGHT_ONLY" == "1" || "$PREFLIGHT_ONLY" == "true" || "$PREFLIGHT_ONLY" == "yes" ]]; then
  echo "runtime-v2 real-provider preflight passed baseUrl=$BASE_URL artifacts=$ARTIFACT_DIR"
  exit 0
fi

export WORKORCH_BASE_URL="$BASE_URL"
export WORKORCH_E2E_MODE="positive"
export WORKORCH_E2E_ARTIFACT_DIR="$ARTIFACT_DIR/positive-live"
export WORKORCH_E2E_WORKSPACE_ID_A="$WORKSPACE_A"
export WORKORCH_E2E_WORKSPACE_ID_B="$WORKSPACE_B"
export WORKORCH_E2E_PROVIDER_ID="$PROVIDER_ID"
export WORKORCH_E2E_MODEL_ID="$MODEL_ID"
export WORKORCH_E2E_ACTIVATE="1"

exec "$ROOT_DIR/scripts/verify-runtime-v2-positive-convergence-live.sh"
