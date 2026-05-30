# Acceptance Record: Verification Layer

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对
`verification-layer-code-alignment-checklist.md`
的当前正式验收结果。

判定口径：

- `pass`
  - 当前脚本、测试和验收入口与验证层设计一致，验证层只承接上游 contract
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明脚本/测试已经完全覆盖
- `impl-drift`
  - 当前脚本/测试/验收口径与验证层设计冲突

## Evidence Base

本轮主要检查以下实现与验证证据：

- `Makefile`
- `scripts/verify-workorch-http.sh`
- `scripts/verify-workorch-container.sh`
- `scripts/verify-workorch-runtime-core-flow.sh`
- `scripts/verify-boundary-contract.sh`
- `scripts/verify-incremental-artifact-e2e.sh`
- `scripts/verify-workorch-runtime-profile-software-dev.sh`
- `scripts/verify-software-dev-goal-dod-e2e.sh`
- `scripts/verify-software-dev-code-agent.sh`
- `scripts/verify-software-requirement-development-orchestration.sh`
- `scripts/loadtest-governed-workspace-concurrency.sh`
- `scripts/loadtest-governed-workspace-concurrency.py`
- `scripts/verify-tui-agent.sh`
- `scripts/verify-tui-long-output-e2e.sh`
- `internal/modules/application/workorchd/service/server/http_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_verify_script_test.go`
- `internal/modules/application/workorch/service/tui/*_test.go`

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| service acceptance | `Makefile`, `verify-workorch-http.sh`, `verify-workorch-container.sh`, `http_test.go` | pass | deployment/acceptance 边界成立；deployed script、handler fixture、container fixture 三层证据与文档一致 |
| core runtime verifier | `verify-workorch-runtime-core-flow.sh`, `verify-boundary-contract.sh`, `verify-incremental-artifact-e2e.sh`, `runtime_verify_script_test.go`, incremental artifact 单测 | pass | core contract、boundary guard、runtime fixture 校验均有正式脚本和单测承接；incremental artifact 由独立 e2e 与单测承接 |
| profile software-dev verifier | profile verifier scripts + scenario e2e + orchestration verifier | pass | `software-dev` 专项规则、scenario1~6、code-agent 边界和 read-model projection 均有独立验证入口 |
| loadtest baseline | `loadtest-governed-workspace-concurrency.sh/.py`, `Makefile` | pass | mock/real 共用单一 harness；输出 summary/report；验收口径不仅看 HTTP 成功率，还看 accepted Result / Artifact / timeline integrity |
| TUI acceptance / display closure | `verify-tui-agent.sh`, `verify-tui-long-output-e2e.sh`, TUI go tests | pass | static guard、防 polling/HTTP、workspace switch、decision/approval、result-first display、long output 行为均有证据 |
| audits boundary | `docs/60-verification/audits/overview.md` + 主验证入口无依赖 | pass | audit 目录作为历史核查材料入口存在，当前未被脚本/Makefile 当作 active verifier contract |
| cross-verification consistency | `overview.md`, `service-acceptance.md`, `core-runtime-verifier.md`, `profile-software-dev-verifier.md`, `loadtest-baseline.md` | pass | service/core/profile/loadtest/TUI 的入口职责分层清楚，不互相替代 |

## Requirement Notes

### 1. Service acceptance

证据：

- `Makefile` 中 `verify-service`, `verify-service-file`
- `verify-workorch-http.sh`
- `verify-workorch-container.sh`
- `http_test.go`

结论：

- 已证明 service acceptance 作为行为验证入口成立
- 已证明 handler fixture direct 覆盖文档列出的主要 endpoint 家族
- 已证明 deployment/startup 与 acceptance 没有在实现上混层

### 2. Core runtime verifier

证据：

- `verify-workorch-runtime-core-flow.sh`
- `verify-boundary-contract.sh`
- `verify-incremental-artifact-e2e.sh`
- `runtime_verify_script_test.go`

结论：

- 已证明 core verifier 真实承接 stage contract、context gate、repair lineage、result closeout 等中立断言
- 已证明 boundary contract 不是空壳，且由单测调用脚本验证
- 已证明 `incrementalArtifact` 由独立 e2e、orchestrator/repository/read-model 单测承接，不会只停留在文档，也不会误要求 `verify-workorch-runtime-core-flow.sh` 直接承接全部 continuation 生命周期

### 3. Profile software-dev verifier

证据：

- `verify-workorch-runtime-profile-software-dev.sh`
- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-dev-code-agent.sh`
- `verify-software-requirement-development-orchestration.sh`

结论：

- 已证明 profile verifier 独立承接 software-dev 规则
- 已证明 scenario1~6 在脚本和产物中有稳定证据路径
- 已证明 code-agent、approval contract、read-model projection 等不依赖 core verifier 替代

### 4. Loadtest baseline

证据：

- `Makefile` 的 `loadtest-governed` / `loadtest-governed-real`
- `loadtest-governed-workspace-concurrency.sh`
- `loadtest-governed-workspace-concurrency.py`

结论：

- 已证明 mock/real 通过单一 harness + 环境变量分流
- 已证明脚本输出 `summary.json` / `report.md`
- 已证明 baseline 验证完整治理型闭环，不是普通 HTTP QPS

### 5. TUI acceptance / display closure

证据：

- `verify-tui-agent.sh`
- `verify-tui-long-output-e2e.sh`
- `service/tui/*_test.go`
- `biz/backend/eventhub_test.go`

结论：

- 已证明 TUI acceptance 不依赖本地 HTTP/polling
- 已证明 result-first、workspace switch、decision/approval inline、monitor refresh、long output 边界均有测试证据
- 已证明 display closure 没有退回 event-stream-first

### 6. Audits boundary

证据：

- `audits/overview.md`
- `Makefile` 和 `scripts/` 中未见 audit 文档被当作 active verifier 入口

结论：

- 已证明 audits 作为材料目录存在
- 当前没有脚本或入口把 audit 反向当成新的 canonical contract

## Summary

本轮结论：

- 验证层文档、脚本、测试和入口整体对齐良好。
- 当前未发现明显 `impl-drift`。
- 也未发现必须保留为关键 `doc-ahead` 的核心缺口。

总体判定：

- 总体结果：`pass`
