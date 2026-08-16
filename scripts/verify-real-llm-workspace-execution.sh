#!/usr/bin/env bash

# Compatibility entry for callers that used the old single-workspace real-LLM
# verifier. The current gate requires two fresh workspace fixtures and inspects
# only persisted Runtime authority, owner receipts, evidence and delivery.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export WORKORCH_REAL_RUNTIME_BASE_URL="${WORKORCH_REAL_RUNTIME_BASE_URL:-${WORKORCH_REAL_LLM_BASE_URL:-${WORKORCH_BASE_URL:-}}}"
export WORKORCH_REAL_RUNTIME_PROVIDER_ID="${WORKORCH_REAL_RUNTIME_PROVIDER_ID:-${WORKORCH_REAL_LLM_PROVIDER_ID:-}}"
export WORKORCH_REAL_RUNTIME_MODEL_ID="${WORKORCH_REAL_RUNTIME_MODEL_ID:-${WORKORCH_REAL_LLM_MODEL_ID:-}}"
export WORKORCH_REAL_RUNTIME_ARTIFACT_DIR="${WORKORCH_REAL_RUNTIME_ARTIFACT_DIR:-${WORKORCH_REAL_LLM_ARTIFACT_DIR:-}}"

exec "$ROOT_DIR/scripts/verify-real-runtime-closure.sh" "$@"
