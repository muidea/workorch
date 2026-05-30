# Verification Layer Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `60-verification/` 当前验证层设计是否已经被代码和脚本正确承接。

它不重新定义 verifier / acceptance / audit / loadtest 规则，只把当前需要逐项核对的实现对齐项拆成可执行清单，便于后续按验证入口逐项处理。

## Canonical Sources

核对时以以下文档为准：

- `../60-verification/overview.md`
- `../60-verification/service-acceptance.md`
- `../60-verification/core-runtime-verifier.md`
- `../60-verification/profile-software-dev-verifier.md`
- `../60-verification/loadtest-baseline.md`
- `../60-verification/tui/tui-agent-acceptance.md`
- `../60-verification/tui/display-closure-tracking.md`
- `../60-verification/audits/overview.md`
- `../50-product-interface/service-api.md`
- `../50-product-interface/tui/requirements.md`
- `../20-runtime-flow/overview.md`

## Review Goal

这一轮只回答三个问题：

1. 验证层设计是否已经在脚本、测试和验收入口中有明确落点。
2. verifier / acceptance / loadtest / TUI acceptance 是否只验证上游 contract，而没有自行发明另一套业务规则。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下实现面：

- `scripts/`
- `Makefile`
- `internal/modules/application/workorchd/service/server/http_test.go`
- `internal/modules/kernel/orchestrator/...`
- `internal/modules/application/workorch/service/tui/...`
- 相关 e2e / verifier 测试入口

## Review Order

建议顺序：

1. service acceptance
2. core runtime verifier
3. profile software-dev verifier
4. loadtest baseline
5. TUI acceptance / display closure
6. audits boundary

## 1. Service Acceptance

重点核对：

- `deployment.md` 与 `service-acceptance.md` 的责任边界是否在实现里保持分离。
- `make verify-service` / `make verify-service-file` 是否只连接现有服务实例，不隐式 build/start/restart。
- `verify-workorch-http.sh`、`verify-workorch-container.sh` 等脚本是否真的覆盖文档列出的最小 service / business / file-backend 行为。
- `http_test.go` 是否覆盖：
  - `run snapshot`
  - `run status`
  - `run observability`
  - `workspace snapshot`
  - `decisions`
  - `approvals`
  - `runtime/payloads`
  - `artifacts/content`
  - `events`
  - `events/stream`
- service acceptance 是否验证的是行为成立，而不是重新定义业务状态机。

常见偏差：

- verify 脚本隐式启动服务，模糊 deployment/acceptance 边界
- handler fixture 没有覆盖文档宣称 direct 的端点
- service acceptance 在脚本中重演业务规则而不是验证返回结果

## 2. Core Runtime Verifier

重点核对：

- `verify-workorch-runtime-core-flow.sh` 是否真实承接 core contract 断言。
- `verify-boundary-contract.sh` 是否真实阻止 profile/domain 语义回流到 core/runtime reusable code。
- 是否覆盖：
  - stage contract
  - context gate
  - repair/replan lineage
  - accepted/rejected result closeout
  - protected fields / request audit
  - incremental artifact neutrality
- orchestrator / blocks / repository 测试是否与文档中的 representative mapping 一致。

常见偏差：

- 文档声称有 direct verifier，但脚本没有对应断言
- boundary contract 只查少量字符串，无法覆盖设计要求
- incremental artifact 只在 e2e 里出现，core verifier 没有承接

## 3. Profile Software-Dev Verifier

重点核对：

- `verify-workorch-runtime-profile-software-dev.sh` 是否真实断言：
  - `acceptanceDimensions`
  - `deliverableCoverage`
  - `evidenceRequirements`
  - direct evidence step-kind gate
- `verify-software-dev-goal-dod-e2e.sh`
  是否真实覆盖文档列出的 scenario1~6 或等价场景。
- `verify-software-dev-code-agent.sh`
  是否真实覆盖 code-agent 边界、approval contract、TUI/CLI 集成。
- `verify-software-requirement-development-orchestration.sh`
  是否真实承接 read-model projection 断言。
- profile verifier 是否只验证 software-dev 规则，而没有反向改写 core runtime 规则。

常见偏差：

- profile verifier 只看 happy-path accepted result
- 文档列出 scenario，但脚本并未断言关键规则
- profile 验证里夹带 core 通用规则，层级混乱

## 4. Loadtest Baseline

重点核对：

- `make loadtest-governed` / `make loadtest-governed-real` 是否对应单一 active harness。
- `loadtest-governed-workspace-concurrency.py` 是否真实输出文档声明的：
  - `summary.json`
  - `report.md`
  - success rate / throughput / latency / failure class / timeline integrity
- loadtest 是否验证“治理型完整业务闭环并发”，而不是普通 HTTP QPS。
- `workspace-concurrency-loadtest-record.md` 与 `loadtest-baseline.md` 是否在实现/脚本层没有混用为同一套基线。

常见偏差：

- mock/real 其实是两套脚本分叉，不是单一 harness
- 输出字段与文档口径不一致
- 压测结论只看 HTTP 成功率，不看 accepted Result / Artifact / timeline integrity

## 5. TUI Acceptance / Display Closure

重点核对：

- `make verify-tui-agent` / `make cli-test` / TUI go tests 是否真实存在并覆盖文档列出的 acceptance layers。
- static architecture guards 是否真实防止：
  - `biz/client` 直连
  - fixed polling
  - 本机 HTTP listener 依赖
- TUI model/integration tests 是否覆盖：
  - workspace switch
  - compact progress
  - inline decision/approval
  - terminal result / failure
  - monitor refresh
- display closure 是否真实保持 result-first，而不是实现里仍有默认 event-stream-first 面板。
- TUI acceptance 是否只验证产品交互 contract，而不另起一套后端协议。

常见偏差：

- TUI verifier 只测启动，不测 blocker / workspace switch / terminal result
- display closure 文档说 result-first，但默认 UI 仍输出 raw event history
- 测试依赖 polling 或 HTTP，而非共享 backend/EventHub

## 6. Audits Boundary

重点核对：

- `audits/` 是否作为审计材料目录，而不是活跃设计主入口。
- audit 文档中的历史路径、旧权威入口是否已指向当前正式文档。
- 审计脚本/说明是否没有反向成为新的 canonical contract。

常见偏差：

- audit 文档仍引用旧核心文档名并被实现继续当作权威
- 审计材料中的历史建议被误当作当前正式设计

## 7. Cross-Verification Consistency Checks

重点核对以下跨验证入口一致性：

- service acceptance、core verifier、profile verifier 是否共享同一套 endpoint / projection 语义。
- TUI acceptance 是否与 `50-product-interface/tui/*` 的产品边界一致。
- loadtest baseline 是否不与 service acceptance / profile acceptance 混成同一类门禁。
- script direct、handler fixture direct、container fixture direct 三层证据是否与文档口径一致。

常见偏差：

- 不同 verify 脚本对同一字段采用不同语义
- TUI 验收断言和产品文档边界不一致
- loadtest 被错误当成日常功能门禁替代 service/profile acceptance

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与脚本/测试/验收入口一致，验证层只承接上游 contract
- `doc-ahead`
  - 文档定义已明确，但脚本/测试尚未正式覆盖
- `impl-drift`
  - 脚本/测试已有实现，但验证口径、入口责任或断言范围与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. 文档宣称 direct 的 endpoint / verifier 覆盖，在脚本或 `http_test.go` 中是否真实存在
2. `verify-boundary-contract.sh` 是否真的阻止 profile/domain 语义回流到 core
3. software-dev scenario 断言是否真实覆盖关键规则，而不是只保存快照文件
4. TUI acceptance 是否真的防止 polling / HTTP 依赖 / event-stream-first 回退
5. loadtest 是否真的验证 accepted Result / Artifact / timeline integrity

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code/Script Path | Result | Notes |
| --- | --- | --- | --- |
| service acceptance | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| core runtime verifier | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| profile software-dev verifier | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| loadtest baseline | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| TUI acceptance | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| audits boundary | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |
| cross-verification consistency | `path/to/code-or-script` | pass / doc-ahead / impl-drift | 简述问题 |

## 2026-05-30 Code Alignment Result

| Area | Code/Script Path | Result | Notes |
| --- | --- | --- | --- |
| service acceptance | `Makefile`, `scripts/verify-workorch-http.sh`, `scripts/verify-workorch-container.sh`, `internal/modules/application/workorchd/service/server/http_test.go` | impl-drift fixed | `verify-service` / `verify-service-file` 当前均为 deployed-instance check；file backend 启动/重启证据由 `docker-verify-file` / container fixture 与 `http_test.go` 承接。已同步修正 `service-acceptance.md` 中将 `verify-service-file` 描述为 file backend 启动基线的口径漂移。 |
| core runtime verifier | `scripts/verify-workorch-runtime-core-flow.sh`, `scripts/verify-boundary-contract.sh`, `scripts/verify-incremental-artifact-e2e.sh`, `internal/modules/kernel/orchestrator/biz/runtime_verify_script_test.go`, incremental artifact orchestrator/repository/read-model tests | pass | core flow script 包含 stage/context gate/state-changing/result closeout/activeRun 等 JSON fixture 断言；boundary script 阻止 weather/software-dev/LSP 等 profile 语义回流到 reusable runtime/Web 代码；incremental artifact 由独立 e2e 和单测承接，不误压到 core-flow 脚本。 |
| profile software-dev verifier | `scripts/verify-workorch-runtime-profile-software-dev.sh`, `scripts/verify-software-dev-goal-dod-e2e.sh`, `scripts/verify-software-dev-code-agent.sh`, `scripts/verify-software-requirement-development-orchestration.sh` | pass | profile verifier 校验 `profileContracts.software-dev`、acceptance dimensions、deliverable coverage、evidence requirements 与 direct evidence step-kind gate；e2e 场景不替代 core verifier。 |
| loadtest baseline | `Makefile`, `scripts/loadtest-governed-workspace-concurrency.sh`, `scripts/loadtest-governed-workspace-concurrency.py` | pass | mock/real 共用单一 harness，通过环境变量切换；脚本输出 `summary.json` / `report.md`，并断言 success rate、accepted Result、linked Artifact、Decision/Approval/ToolCall/ValidationResult 与 timeline integrity。 |
| TUI acceptance | `scripts/verify-tui-agent.sh`, `scripts/verify-cli-architecture.sh`, `internal/modules/application/workorch/service/tui/*_test.go`, `scripts/verify-tui-long-output-e2e.sh` | pass | static guard 阻止 `biz/client`、HTTP、fixed polling；TUI model tests 覆盖 workspace switch、decision/approval、terminal result、result-first display、monitor refresh 与长输出滚动。 |
| audits boundary | `docs/60-verification/audits/overview.md` | pass | audit 目录作为历史核查材料入口保留，当前没有被 Makefile/script 当作 active verifier contract。 |
| cross-verification consistency | `docs/60-verification/service-acceptance.md`, `docs/60-verification/core-runtime-verifier.md`, `docs/60-verification/profile-software-dev-verifier.md`, `docs/60-verification/loadtest-baseline.md` | pass | service/core/profile/loadtest/TUI 的入口职责已分层：deployed-instance、container/file-backend、core neutrality、profile rules、capacity baseline 不互相替代。 |

### Follow-Up

- TUI 文档中的场景编号比 `verify-tui-agent.sh` 的脚本化编号更完整；当前由 Go model tests + scripted e2e 共同覆盖。后续若需要严格“一编号一脚本输出”，应只补脚本报告索引，不改变 TUI 产品合同。
- file backend reload 的 detail page “真正重启后再次翻页”仍主要由 handler/file fixture 覆盖；如需 deployed shell 级样本，应在 `service-acceptance.md` 的 follow-up 范围内补充。
