#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GOCACHE="${GOCACHE:-/tmp/workorch-go-build}"
export GOCACHE
SRD_E2E_DIR=""

echo "=== software requirement development orchestration verification (SRD-10~34) ==="

echo "-> SRD-10/SRD-13/SRD-14: 计划事实、结果质量门控、repair loop 投影（read model）"
go test ./internal/modules/application/workorchd/biz/appservice \
  -run 'Test(ReadRunTaskDecompositionsWithBackendReturnsExecutionGraphProjection|ReadRunSnapshotWithBackendReturnsExecutionEvidence|MapResultQualityFlagsWeakResult|MapTaskProgressProjectsRepairLoop(Escalation|State))$' \
  -count=1

echo "-> SRD-12: 验证命令与修复经验 Memory 策略（orchestrator）"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeOrchestrator_(StartRunPersistsRuntimeMemoryWrites|RecordsValidationCommandMemoriesFromDoDAssessment)$' \
  -count=1

echo "-> SRD-15: Web/TUI 业务模型展示映射回归"
make console-build
go test ./internal/modules/application/workorch/service/tui -count=1
go test ./internal/modules/application/workorch/biz/backend -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1

echo "-> SRD-16: software-dev Goal/DoD 自动开发闭环（改码/验证/修复/diff/DoD 证据）"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'Test(RuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff|RuntimeOrchestrator_ResultArtifactsIncludePatchDiff|RuntimeOrchestrator_ResultManifestListsOnlyPatchTargets)$' \
  -count=1
if [[ "${WORKORCH_SRD_VERIFY_DEPLOYED_E2E:-0}" == "1" ]]; then
  e2e_dir="$(mktemp -d)"
  SRD_E2E_DIR="$e2e_dir"
  WORKORCH_SOFTWARE_DEV_E2E_ARTIFACT_DIR="$e2e_dir" "${ROOT_DIR}/scripts/verify-software-dev-goal-dod-e2e.sh"
  python3 - "$e2e_dir/scenario1-snapshot.json" "$e2e_dir/scenario1-observability.json" <<'PY'
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8")).get("snapshot") or {}
observability = json.load(open(sys.argv[2], encoding="utf-8")).get("observability") or {}
errors = []

task_plans = snapshot.get("taskPlans") or []
if not task_plans:
    errors.append("snapshot missing taskPlans")
else:
    accepted = [item for item in task_plans if item.get("state") == "accepted"]
    if not accepted:
        errors.append(f"taskPlans missing accepted revision: {task_plans!r}")
    if not any((item.get("revision") or 0) >= 1 for item in task_plans):
        errors.append(f"taskPlans missing revision>=1: {task_plans!r}")

result_quality = snapshot.get("resultQuality") or []
if not result_quality:
    errors.append("snapshot missing resultQuality")
elif result_quality[0].get("state") not in {"passed", "warning", "failed"}:
    errors.append(f"resultQuality invalid state: {result_quality[0]!r}")

dod_evidence = snapshot.get("doDEvidence") or []
if not dod_evidence:
    errors.append("snapshot missing doDEvidence")
elif not (dod_evidence[0].get("items") or []):
    errors.append(f"doDEvidence missing item-level evidence: {dod_evidence[0]!r}")

repair_loop = ((snapshot.get("taskProgress") or {}).get("repairLoop") or {})
if int(repair_loop.get("attemptCount") or 0) < 1:
    errors.append(f"repairLoop.attemptCount={repair_loop.get('attemptCount')!r}, want >=1")

memory_recalls = snapshot.get("memoryRecallTraces") or []
if not memory_recalls:
    errors.append("snapshot missing memoryRecallTraces")
elif not any(item.get("status") in {"selected", "degraded", "empty"} for item in memory_recalls):
    errors.append(f"memoryRecallTraces missing expected status projection: {memory_recalls!r}")

obs_task_plans = observability.get("taskPlans") or []
if not obs_task_plans:
    errors.append("observability missing taskPlans")
if not (observability.get("resultQuality") or []):
    errors.append("observability missing resultQuality")
if not (observability.get("doDEvidence") or []):
    errors.append("observability missing doDEvidence")

if errors:
    for line in errors:
        print(line, file=sys.stderr)
    raise SystemExit(1)
PY
fi

echo "-> SRD-17/SRD-18: 需求歧义 Decision 与风险操作 Approval（deployed e2e）"
if [[ "${WORKORCH_SRD_VERIFY_DEPLOYED_E2E:-0}" == "1" ]]; then
  "${ROOT_DIR}/scripts/verify-llm-workspace-execution.sh"
else
  echo "   skip deployed e2e (set WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 to run verify-llm-workspace-execution.sh)"
fi

echo "-> SRD-19: repair loop 耗尽后升级 Decision 与可追溯投影"
go test ./internal/modules/kernel/orchestrator/biz \
  -run 'TestRuntimeOrchestrator_RuntimeRepairExhaustionFailsRun' \
  -count=1
go test ./internal/modules/application/workorchd/biz/appservice \
  -run 'TestMapTaskProgressProjectsRepairLoop(Escalation|State)$' \
  -count=1

echo "-> SRD-21~32: Phase 5 默认值、上下文、repair/provider、脱敏、DoD 门控与失败交付回归"
go test \
  ./internal/pkg/common \
  ./internal/modules/application/workorchd/biz/bootstrap \
  ./internal/modules/application/workorchd/biz/appservice \
  ./internal/modules/kernel/decision/biz \
  ./internal/modules/kernel/orchestrator/biz \
  ./internal/modules/blocks/llm/biz \
  -count=1

echo "-> SRD-33: 复杂软件开发闭环 deployed e2e（空 workspace、累积 Goal/DoD、secret 脱敏、弱证据拒绝）"
if [[ "${WORKORCH_SRD_VERIFY_DEPLOYED_E2E:-0}" == "1" ]]; then
  if [[ -z "$SRD_E2E_DIR" || ! -f "$SRD_E2E_DIR/scenario3-snapshot.json" || ! -f "$SRD_E2E_DIR/scenario4-snapshot.json" ]]; then
    echo "SRD-33 artifacts missing from software-dev deployed e2e" >&2
    exit 1
  fi
  echo "   SRD-33 covered by verify-software-dev-goal-dod-e2e artifacts in $SRD_E2E_DIR"
else
  echo "   skip deployed e2e (set WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 to run SRD-33 coverage)"
fi

echo "software requirement development orchestration verification passed"
