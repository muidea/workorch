#!/usr/bin/env bash
# Run the fresh-data, deterministic-provider portion of SMO-C7 against a
# managed deployed instance. The managed harness owns its temporary binary,
# config, data root and workspace roots; this wrapper never reuses an operator
# workspace or alters a running service.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_C7_SCRIPTED_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-v2-c7-scripted/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_RUNTIME_ROOT="$(realpath -m "$ROOT_DIR/.workorch-e2e")"

if [[ "$ARTIFACT_DIR" == "$REPOSITORY_RUNTIME_ROOT" || "$ARTIFACT_DIR" == "$REPOSITORY_RUNTIME_ROOT/"* ]]; then
  echo "WORKORCH_C7_SCRIPTED_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi

export WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR="$ARTIFACT_DIR"
export WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1
export WORKORCH_MANAGED_VERIFY_TARGET=verify-runtime-v2-c7-fixture
export WORKORCH_MANAGED_SKIP_PREFLIGHT="${WORKORCH_MANAGED_SKIP_PREFLIGHT:-0}"
export WORKORCH_MANAGED_GO_TEST_PACKAGES=./...
export WORKORCH_MANAGED_GO_TEST_FLAGS=-count=1
# C7 proves two independently activated workspace scopes can run at once. Do
# not inherit the managed harness's single-run default, which only serializes
# the test and leaves the concurrency exit criterion unexercised.
export WORKORCH_MANAGED_MAX_CONCURRENT_RUNS="${WORKORCH_MANAGED_MAX_CONCURRENT_RUNS:-2}"
export WORKORCH_VERIFY_KEEP_TMP=1
# Keep all C7 request, receipt, terminal and safe-diagnostic artifacts beneath
# the managed harness root.  A fresh run never reuses an operator workspace or
# leaves its evidence in an unrelated mktemp directory.
export WORKORCH_LLM_VERIFY_ARTIFACT_DIR="$ARTIFACT_DIR/llm-verification"

"$ROOT_DIR/scripts/verify-workorch-managed-full.sh"

# The managed fresh-data proof above covers concurrent positive convergence.
# Run the three crash boundaries in a second, equally isolated data root: this
# avoids mixing its intentionally interrupted Runs into the terminal fixture
# while retaining both evidence sets below the one C7 artifact directory.
python3 "$ROOT_DIR/scripts/verify_runtime_v2_c7_restart_harness.py" \
  --artifact-dir "$ARTIFACT_DIR/restart-recovery" \
  --binary "$ROOT_DIR/bin/workorch"
