#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE

echo "=== software-dev code agent verification ==="

echo "-> orchestrator software-dev repair and delivery evidence"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff|TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeApprovalApproved|TestRuntimeOrchestrator_ResumeRunRejectsRuntimeApprovalWithoutDispatchingNextStep' \
  -count=1

echo "-> approval action/context contract"
go test ./internal/modules/blocks/llm/biz ./internal/modules/kernel/approval/biz ./internal/pkg/events ./internal/pkg/repository \
  -run 'TestNormalizeDecisionOutputPreservesRequiredApprovalActionContext|TestParseLLMExecutionDecisionAllowsNestedGovernanceExtraFields|TestApprovalRepositoryContract|TestDecodeWorkorchEventNormalizesApprovalPayload|TestServiceEnsureApproval' \
  -count=1

echo "-> TUI/CLI architecture and TUI acceptance"
./scripts/verify-cli-architecture.sh
./scripts/verify-tui-agent.sh

if [[ "${WORKORCH_SOFTWARE_DEV_VERIFY_LLM_E2E:-0}" == "1" ]]; then
  if [[ -z "${WORKORCH_LLM_VERIFY_BASE_URL:-}" ]]; then
    echo "WORKORCH_LLM_VERIFY_BASE_URL is required when WORKORCH_SOFTWARE_DEV_VERIFY_LLM_E2E=1" >&2
    exit 2
  fi
  echo "-> deployed LLM governance e2e at ${WORKORCH_LLM_VERIFY_BASE_URL}"
  WORKORCH_LLM_VERIFY_MOCK_ADDR="${WORKORCH_LLM_VERIFY_MOCK_ADDR:-127.0.0.1:18181}" \
    ./scripts/verify-llm-workspace-execution.sh
fi

echo "software-dev code agent verification passed"
