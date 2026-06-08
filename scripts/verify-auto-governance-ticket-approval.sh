#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "-> AG-E2E-01/02/03/04/05/06/07: auto governance ticket/approval/doD/budget policy"
go test ./internal/modules/kernel/governance/biz -count=1

echo "-> AG-E2E-08: Web read model includes auto governance summary"
go test ./internal/modules/application/workorchd/biz/appservice \
  -run 'TestReadDecisionWithBackendIncludesAutoGovernanceSummary|TestReadApprovalWithBackendIncludesAutoGovernanceSummary' \
  -count=1

echo "-> AG-E2E-09: TUI shows compact auto governance status"
go test ./internal/services/workorch/service/tui \
  -run 'TestModelGovernanceResolvedEventShowsCompactStatus|TestModelGovernanceEscalatedEventRefreshesBlockers' \
  -count=1

echo "-> AG-E2E-10: governance auto event payload/version contract"
go test ./internal/pkg/events -count=1

echo "-> integration baseline: llm workspace execution workflow"
"$ROOT_DIR/scripts/verify-llm-workspace-execution.sh"

echo "auto governance ticket/approval verification passed"
