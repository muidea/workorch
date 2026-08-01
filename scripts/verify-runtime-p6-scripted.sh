#!/usr/bin/env bash
# Run the isolated, mock-provider half of the Runtime P6 gate.  The managed
# harness owns the temporary binary, config, data root and workspace roots.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="$(realpath -m "${WORKORCH_P6_SCRIPTED_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-e2e/runtime-p6-scripted/$(date -u +%Y%m%dT%H%M%SZ)}")"
REPOSITORY_RUNTIME_ROOT="$(realpath -m "$ROOT_DIR/.workorch-e2e")"

if [[ "${1:-}" == "--self-test" ]]; then
  [[ -x "$ROOT_DIR/scripts/verify-workorch-managed-full.sh" ]]
  [[ "$ARTIFACT_DIR" != "$REPOSITORY_RUNTIME_ROOT" && "$ARTIFACT_DIR" != "$REPOSITORY_RUNTIME_ROOT/"* ]]
  python3 "$ROOT_DIR/scripts/verify_runtime_p6_scripted_contract.py" --self-test
  echo "runtime P6 scripted verifier self-test passed"
  exit 0
fi

if [[ "$ARTIFACT_DIR" == "$REPOSITORY_RUNTIME_ROOT" || "$ARTIFACT_DIR" == "$REPOSITORY_RUNTIME_ROOT/"* ]]; then
  echo "WORKORCH_P6_SCRIPTED_ARTIFACT_DIR must not point to the repository .workorch-e2e directory" >&2
  exit 2
fi

export WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR="$ARTIFACT_DIR"
export WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1
export WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file
# P6 scripted evidence is only useful when the artifact was produced from the
# complete current tree, not a caller-selected partial preflight.
export WORKORCH_MANAGED_SKIP_PREFLIGHT=0
export WORKORCH_MANAGED_GO_TEST_PACKAGES=./...
export WORKORCH_MANAGED_GO_TEST_FLAGS=-count=1

"$ROOT_DIR/scripts/verify-workorch-managed-full.sh"
python3 "$ROOT_DIR/scripts/verify_runtime_p6_scripted_contract.py" "$ARTIFACT_DIR" --write-summary

echo "runtime P6 scripted verification passed artifacts=$ARTIFACT_DIR"
