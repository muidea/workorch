#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"
source "$ROOT_DIR/scripts/lib/go-test-must-run.sh"

run_go_test ./internal/modules/kernel/orchestrator/biz/runtime \
  -run '^(TestAdvanceSatisfiedCoverageFocusDropsPriorFocusTargetFacts|TestDispatchCommitAdvancesToNextCoverageFocusDeterministically)$' \
  -count=1

run_go_test ./internal/modules/kernel/orchestrator/biz \
  -run '^TestRuntimeStartupRecoveryDeliversPendingTerminalThroughOwnerChain$' \
  -count=1

run_go_test ./internal/pkg/workorchdefaults \
  -run '^TestEmbeddedSoftwareDevTemplateDoDItemsCoverScopeEvidenceAndRisk$' \
  -count=1

echo "coverage focus settlement and terminal Result delivery verification passed"
