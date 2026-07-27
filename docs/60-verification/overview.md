# WorkOrch Verification Overview

Status: active

Type: verification

Last Updated: 2026-07-27

## Purpose

集中承接服务验收、core/profile verifier、TUI acceptance 与 loadtest baseline。  
历史 alignment checklist / acceptance-record / dated live 记录 / V1 live 复盘已迁入 `90-archive/records/`，**不再**作为当前实现约束。
`artifacts/` live 运行产物已从仓库移除（git 历史可找回）；结论保留在 records markdown。

生产 runtime 主链为 **Runtime V2 only**。验收不得把 `StagePacket` / `NextAction` / graph dispatch / `ExecutionContextPatch` / 自动 replan successor 重新声明为通过标准。

## Production Acceptance Baseline（Runtime V2）

验收必须以 **Runtime V2 only** 为准：

| 维度 | 必须覆盖 |
| --- | --- |
| Four-state-machine conformance | SM-1 Run lifecycle、SM-2 authority/focus、SM-3 candidate/dispatch、SM-4 drive/recovery/projection；合法/禁止迁移与协调协议全部有证据 |
| Activation / Bootstrap | typed `RuntimeBootstrap`（Definition + Activation + Focuses + Requirements）；缺 bootstrap 失败经 owner-local terminal delivery 收敛 |
| Start | Run owner接受 activation intent 并内部 `pending→active`，`RunStateChanged` 后进入 `DriveRuntimeV2` |
| Drive | 有界循环；非等待非终态 transition 递增 StateRevision；无进展 / 超 guard fail-fast |
| Prerequisite | 缺失资源走 `AcquirePrerequisite` + commit + reduce，不伪装 state-change |
| Candidate | attempt 0/1、same transaction + stable prefix；漂移不 seal；Accept 后 seal |
| Dispatch / Operation | `SealedDispatch` + typed `ExecuteRuntimeOperation`；operation evidence 与 snapshot/dispatch 一致；dependency graph 仅作 sealed 操作序，非 V1 graph dispatcher |
| Focus / EvidenceLedger | required evidence（及可选 assessment）满足后切换 focus；stale→diagnostic，duplicate→幂等 |
| Assessment / Result | Orchestrator 裁决；Result owner 只持久化 outcome；不裁决 |
| Governance wait | `request_governance` → ticket → immutable wait fact → Run owner-local blocked；resolution commit 回 reducer/fact；application **不因** resolve 再 Resume |
| Repair | 同 focus 再收敛；预算耗尽升治理；非自动 replan |
| Replan semantic | 仅新 activation / 新 V2 Run；无自动 successor lifecycle |
| Resume / 重启 | 只 load aggregate 幂等再 drive；无 ExecutionContextPatch；不重放 V1 checkpoint 驱动 |
| Candidate context / cache | `RuntimeCandidateContext` + C0–C5；P0–P4 integrity |
| 中立性 | core 无 software-dev / profile ID 硬编码；profile 字段仅作 activation / C2 输入 |

## Current Entry Documents

| 文档 | 职责 |
| --- | --- |
| [service-acceptance.md](service-acceptance.md) | 服务级验收入口与门禁指针 |
| [core-runtime-verifier.md](core-runtime-verifier.md) | core runtime verifier 说明 |
| [profile-software-dev-verifier.md](profile-software-dev-verifier.md) | software-dev profile verifier |
| [loadtest-baseline.md](loadtest-baseline.md) | 压测 baseline |
| [tui/tui-agent-acceptance.md](tui/tui-agent-acceptance.md) | TUI agent 验收入口 |

## Common Commands（make verify-*）

以仓库根 `Makefile` 为准。Runtime V2 / authority / resume / governance / live gate 相关示例：

```bash
make test
make vet
make verify-boundary-contract
make verify-ci-mvp-file
make verify-service
make verify-llm
make verify-contract-violation-recovery-e2e
make verify-service-restart-recoverable-e2e
make verify-run-resume-gap-confirmation-e2e
make verify-dual-workspace-resume-replan-e2e   # replan 语义 = 新 activation/run，非自动 successor
make verify-approval-decision-resolution-e2e  # resolution 后无 application auto-Resume 依赖
make verify-auto-governance-ticket-approval
make verify-convergence-authority-dispatch-live
make verify-authority-live-e2e-closure
make observe-runtime-closure
make verify-incremental-artifact-e2e
make verify-runtime-profile-software-dev
# 更大套件：verify-e2e-full / verify-deployed-full / verify-managed-full
# 以及 scripts/ 下 e2e：verify-software-dev-goal-dod-e2e.sh 等
```

具体脚本与断言矩阵以 `core-runtime-verifier.md` / `service-acceptance.md` 与 Makefile help 为准。新增验收不得引入 V1 StagePacket/NextAction 通过条件。

## What Was Archived

- `acceptance-record-*`、`*-code-alignment-checklist`、`framework-eventhub-module-boundary-checklist`
- `convergence-authority-dispatch-live-verification.md`、`workorch-runtime-closure-modification-spec.md`
- 2026-07-16：`authority-live-e2e-closure-verification.md`、`ai-proxy-*-live-verification-*.md`、`tui-workspace-runtime-refresh-history-closure-verification-*.md`、`tui-display-closure-tracking.md` → `90-archive/records/`
- 2026-07-16：`tui-data-stability-command-optimization-plan.md` → `90-archive/completed/`
- 2026-07-16：`60-verification/artifacts/**` 整目录出库（live JSON/log；结论在 records markdown）

检索：[../90-archive/records/overview.md](../90-archive/records/overview.md)、[../90-archive/completed/overview.md](../90-archive/completed/overview.md)

## Related

- [../01-overview/current-state.md](../01-overview/current-state.md)
- [../00-core/runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- [../00-core/runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md)
- [../20-runtime-flow/overview.md](../20-runtime-flow/overview.md)
- `internal/modules/kernel/orchestrator/readme.md`
