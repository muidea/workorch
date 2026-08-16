#!/usr/bin/env bash

# Compatibility entry for the former provider-driven fixture. Acceptance-slice
# settlement is now verified at the Runtime owner boundary.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"$ROOT_DIR/scripts/verify-acceptance-slice-result-e2e.sh"

cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/go-test-must-run.sh"
run_go_test ./internal/modules/kernel/orchestrator/biz \
  -run '^(TestRuntimeFileBackedEvidenceCoverageFocusAdvanceCrashRestart|TestDriveRuntimeFileBackedTwoFocusStateValidationAssessmentAndRestart)$' \
  -count=1

echo "acceptance-slice file-backed Runtime verification passed"
