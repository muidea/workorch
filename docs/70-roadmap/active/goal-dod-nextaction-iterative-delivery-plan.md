# Goal/DoD NextAction Iterative Delivery Plan

Status: active

Type: roadmap

Last Updated: 2026-06-23

Design Source:

- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)

Related:

- [Run Resume And Gap Confirmation Design](../../30-module-architecture/run-resume-gap-confirmation-design.md)
- [Contract Violation Recovery Loop Design](../../30-module-architecture/contract-violation-recovery-loop-design.md)
- [EventHub Driven Workorchd State Loop Design](../../30-module-architecture/eventhub-driven-workorchd-state-loop-design.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)
- [LLM Cache Improvement Design](../../30-module-architecture/llm-cache-improvement-design.md)
- [S0.1 Command Chain Audit](./goal-dod-nextaction-s0-1-command-chain-audit.md)
- [S1.1-S1.2 Run Command Contract](./goal-dod-nextaction-s1-1-s1-2-run-command-contract.md)
- [S1.3 Run Command Projection](./goal-dod-nextaction-s1-3-run-command-projection.md)
- [S1.4 Replan Idempotency](./goal-dod-nextaction-s1-4-replan-idempotency.md)
- [S1.5 Restart Replacement Semantics](./goal-dod-nextaction-s1-5-restart-replacement.md)
- [S1.6 TUI/REST Command Path Parity](./goal-dod-nextaction-s1-6-command-path-parity.md)
- [S2.1 Workspace Definition Revision Model](./goal-dod-nextaction-s2-1-definition-revision-model.md)
- [S2.2 Current Definition Revision Binding](./goal-dod-nextaction-s2-2-current-definition-revision-binding.md)
- [S2.3 Initial Gap Baseline](./goal-dod-nextaction-s2-3-initial-gap-baseline.md)
- [S2.4 Resume Gap Baseline](./goal-dod-nextaction-s2-4-resume-gap-baseline.md)
- [S2.5 Replan Definition And Baseline Inheritance](./goal-dod-nextaction-s2-5-replan-definition-baseline.md)

## Purpose

本文把 `goal-dod-nextaction-delivery-state-machine-design.md` 拆分为可以逐一交付、逐一验收、逐一回归的功能点。本文不是编码规范，而是后续代码开发的迭代交付清单。

后续实现必须按本文编号推进。每个功能点完成时，需要在提交说明、PR 描述或对应收口记录中引用功能点 ID，并补充验收证据。未满足当前功能点验收条件时，不进入下一个依赖功能点。

当前阶段不考虑兼容性。若现有实现、端侧命令、测试脚本或文档与本文冲突，应按本文逐步收口。

## Delivery Principles

1. 每个功能点必须独立可验收，不能只提交“重构中间态”。
2. 每个功能点必须有明确 owner：core、workorchd appservice、TUI/Web/port、LLM context/provider、template/Soul/policy 或 runtime environment。
3. 每个功能点必须包含至少一种验证证据：单测、集成测试、边界搜索、projection 测试、cache/context 测试、e2e 或 live verification。
4. Core 修改必须保持业务中立。软件开发策略只能进入 workspaceTemplate/profile/Soul/policy/skill。
5. 每个功能点都必须能回答：它改善了哪个设计链路环节，以及失败时应回退到哪个审计点。
6. 不允许一次性大范围重写。先建立权威状态和可观测性，再逐步替换旧路径。

## Granularity Rules

`P0-P11` 是交付 epic，不是单次开发任务。实际开发必须继续拆成 `Sxx` 微交付项。每个 `Sxx` 是一次适合 LLM 上下文承载的最小功能切片。

单个 `Sxx` 必须满足：

- 只覆盖一个 owner，或一个清晰的 command -> owner -> projection 链路。
- 只修改一个主模块族，通常不超过 3 到 5 个业务文件；测试文件可另计。
- 只引入或迁移一个核心概念，例如只做 `EvidenceRecord`，不同时做 `CoverageLedger` 和 `StagePacket`。
- 只处理一个行为闭环，例如只处理 `resume` command，不同时处理 `replan`、`restart`、approval 和 provider retry。
- 必须有独立验收测试或可复核的边界搜索。
- 必须能在最终响应中用 10 行以内说明改了什么、命中了哪个设计条目、验证了什么。

单个 `Sxx` 禁止：

- 同时修改 core、TUI、Web、LLM prompt、template、watchdog 和 e2e。
- 把多个 unresolved design open item 一次性收口。
- 在没有模型/owner 验收的情况下直接改 prompt 或策略文案。
- 用 live workspace 的当前症状扩大为通用实现。
- 因为“顺手”改相邻模块、重命名大量文件或做无关清理。

如果某个任务需要读取超过以下信息，必须继续拆分：

- 超过 2 个设计文档。
- 超过 5 个相关源文件。
- 超过 1 条 EventHub 主路径。
- 超过 1 个状态 owner。
- 超过 1 类 LLM 上下文分区变更。

## Delivery Map

```text
P0 Audit Baseline
  -> P1 Command Main Path
  -> P2 Definition Revision And Gap Baseline
  -> P3 Evidence Ledger
  -> P4 Coverage Ledger
  -> P5 NextAction Interpreter
  -> P6 Stage Packet And LLM Context
  -> P7 Contract Violation Recovery
  -> P8 Projection And MVVP
  -> P9 IO/History/Watchdog Scope
  -> P10 Template/Profile Strategy
  -> P11 Full E2E And Live Verification
```

依赖规则：

- `P1` 是后续所有运行触发功能点的入口依赖。
- `P2-P5` 是 runtime 确定性推进的最小闭环。
- `P6-P7` 是 LLM 交互质量和违例恢复闭环。
- `P8` 是 TUI/Web 状态一致性的验收入口。
- `P9` 是长期运行稳定性入口。
- `P10` 是业务策略下沉入口。
- `P11` 是全链路验收，不应提前代替单点验收。

## Micro Delivery Backlog

本节是实际代码开发入口。每个 `Sxx` 都应单独完成 Contract Audit、代码修改、测试和边界搜索。

### P0 Slices: Audit Baseline

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S0.1 | 梳理 resume/replan/restart 当前入口和调用链 | workorchd/port | 输出 command -> owner -> projection 路径和缺口 |
| S0.2 | 梳理 Goal/DoD、run checkpoint、coverage 现有字段 | core | 输出字段映射，不改业务逻辑 |
| S0.3 | 梳理 evidence/result/artifact/memory 写入路径 | core | 标记 accepted/rejected 混用风险 |
| S0.4 | 梳理 LLM request 分区和 context budget 裁剪点 | llm block/core | 输出 C0-C5 当前映射 |
| S0.5 | 梳理 projection/ViewModel/TUI prompt 数据来源 | workorchd/TUI | 输出端侧是否存在本地状态机 |
| S0.6 | 执行 core/template 边界搜索并分类 | all | 只记录问题，不做清理 |

### P1 Slices: Scoped Command Main Path

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S1.1 | 定义 canonical command DTO 和 scope 字段 | port/client | resume/replan/restart/approval/decision DTO 有 workspace scope |
| S1.2 | 清理旧 `continue` 运行命令表述，统一推进命令为 `/run resume` | TUI/port | 端侧和 port 只暴露 canonical run intent |
| S1.3 | 实现 resume command accepted/rejected projection | workorchd | projection 可观察 command revision |
| S1.4 | 实现 replan command scope 与幂等 key | workorchd | 重复 replan 不创建重复调度 |
| S1.5 | 实现 restart replacement run command 语义 | workorchd | restart 与 resume/replan 明确分离 |
| S1.6 | 补充 TUI/REST 双入口同路径测试 | port/appservice | 两入口进入同一 owner command |

### P2 Slices: Definition Revision And Gap Baseline

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S2.1 | 定义或映射 WorkspaceDefinitionRevision 最小模型 | core | run 可引用 definition revision |
| S2.2 | 将 Goal/DoD 当前定义绑定到 revision | core/appservice | 首次 run 有 definition revision |
| S2.3 | 构建首次 GapBaseline 空证据路径 | core | 无历史 evidence 也能生成 open baseline |
| S2.4 | resume 前刷新或复用 GapBaseline | workorchd/core | resume 不绕过 gap gate |
| S2.5 | replan 继承 revision 与 baseline | workorchd/core | resolution 文本不覆盖 Goal/DoD |
| S2.6 | restart 基于最新 revision 重建 baseline | workorchd/core | replacement run 不复用旧 baseline 状态 |

### P3 Slices: Evidence Ledger

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S3.1 | 定义 EvidenceRecord 最小持久化/查询模型 | core/repository | evidence 有 scope、kind、status、sourceRef |
| S3.2 | 接入 state_change accepted evidence 写入 | core | admitted side effect 才能 accepted |
| S3.3 | 接入 validation accepted/rejected evidence 写入 | core | validation 结果可追溯 |
| S3.4 | 接入 decision/approval governance evidence | core/workorchd | resolution 可追溯 |
| S3.5 | 接入 diagnostic rejected evidence | core | LLM 违例只写 rejected diagnostic |
| S3.6 | 阻断 rejected evidence 写 artifact/result/memory | core | 测试证明无污染 |

### P4 Slices: Coverage Ledger

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S4.1 | 定义 CoverageItem 最小模型 | core | item 绑定 goal/dod 和 required evidence kinds |
| S4.2 | 从 definition revision 初始化 coverage ledger | core | 每个 DoD 有 open item |
| S4.3 | 根据 accepted evidence 更新 coverage | core | rejected 不影响 satisfied |
| S4.4 | 计算 missing evidence kinds | core | state_change/validation/assessment 不互相误满足 |
| S4.5 | completion gate 读取 coverage ledger | core | complete 有 evidence refs |
| S4.6 | projection 输出 coverage summary | workorchd | UI 可展示 open/satisfied/blocked |

### P5 Slices: NextAction

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S5.1 | 定义 NextAction 最小模型和 reason | core | 可从 ledger 重建 |
| S5.2 | 实现 pending approval -> request_approval | core | 不调度 state_change |
| S5.3 | 实现 open decision -> request_decision | core | 只在缺外部输入时触发 |
| S5.4 | 实现 missing gap -> assess_gap | core | 无 baseline 不执行 state_change |
| S5.5 | 实现 missing state_change -> state_change | core | 当前 focus 明确 |
| S5.6 | 实现 missing validation -> validate | core | state_change 后验证 |
| S5.7 | 实现 recoverable diagnostic -> repair/retry | core | 不直接 waiting_decision |
| S5.8 | 实现 all satisfied -> complete | core | completion 可解释 |

### P6 Slices: Stage Packet And LLM Context

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S6.1 | 定义 StagePacket 最小结构 | core | 包含 definition/currentFocus/requiredOutput |
| S6.2 | 将 accepted/rejected evidence refs 注入 StagePacket | core | LLM input 可追溯 |
| S6.3 | 将 target snapshot 完整性状态注入 StagePacket | core | truncated 不用于 exact file |
| S6.4 | 建立 C0/C1/C2/C3/C4/C5 渲染边界测试 | llm block | 动态字段不进 C0 |
| S6.5 | context budget 保护关键字段 | llm block/core | Goal/DoD/requiredOutput/evidence 不裁剪 |
| S6.6 | compact summary 只作为辅助引用 | core | ledger 仍是权威 |

### P7 Slices: Contract Violation Recovery

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S7.1 | 定义 ContractViolation 诊断模型 | core | 有 rule/signature/recoverability |
| S7.2 | parse/schema gate 输出 recoverable diagnostic | core/llm | 字段缺失可重试 |
| S7.3 | contract gate 阻断 side effect | core | 违例输出不执行 |
| S7.4 | retry 继承 stage/requiredOutput/definition | core | 同合同重试 |
| S7.5 | retry budget 和重复 signature 升级 | core | 避免无限循环 |
| S7.6 | projection 展示 contract violation recovery | workorchd | 用户可见正在自动修正 |

### P8 Slices: Projection And MVVP

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S8.1 | 定义 WorkspaceViewModel revision 必需字段 | workorchd | list/run/prompt/commands/coverage 同源 |
| S8.2 | prompt mode 只来自 projection | TUI | 切换 workspace 后 prompt 正确 |
| S8.3 | workspace list 和主视图同 revision 渲染 | TUI/Web | 无本地状态覆盖 |
| S8.4 | command pending/accepted 展示 | TUI/Web | 提交后等待 projection 更新 |
| S8.5 | SSE reconnect 和 last event id | Web/client | 不白屏、不永久停滞 |
| S8.6 | decision/approval 视图回归 | TUI/Web | replan/approve 后刷新 |

### P9 Slices: IO, History And Watchdog

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S9.1 | projection refresh 改为 readmodel 优先 | workorchd | 不全量扫描文件 |
| S9.2 | history 近期完整 + compact + refs | core | 不全量注入历史 |
| S9.3 | snapshot/payload 按需读取 | core/repository | 大对象不默认加载 |
| S9.4 | watchdog 仅覆盖外部调用超时 | core/workorchd | 不替代 command 主路径 |
| S9.5 | watchdog 补偿幂等 key | core | 重复触发无副作用 |
| S9.6 | service restart running -> recoverable | workorchd | 不显示假 running |

### P10 Slices: Template/Profile/Soul Strategy

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S10.1 | 定义 template stage policy schema | template/profile | core 只解释结构化字段 |
| S10.2 | 软件开发阶段策略绑定到 workspace | template/profile | 创建期可追溯 |
| S10.3 | evidence policy 绑定到 DoD/coverage | template/profile | requiredEvidenceKinds 来自策略 |
| S10.4 | 文件粒度/任务粒度 guidance 下沉到 Soul/skill | Soul/skill | core 无文件/语言特判 |
| S10.5 | MCP/LSP 能力作为 template capability | template/profile | 非强制 core 默认 |
| S10.6 | 非软件开发模板隔离测试 | template/profile | 不受软件策略影响 |

### P11 Slices: E2E And Live Verification

| Slice | Deliverable | Owner | Acceptance |
| --- | --- | --- | --- |
| S11.1 | 自动管理 workorch 实例的 e2e harness | e2e | 启停清理可重复 |
| S11.2 | 双 workspace 并发 resume/replan e2e | e2e | 不互相阻塞 |
| S11.3 | approval/decision e2e | e2e | resolution 后继续推进 |
| S11.4 | contract violation retry e2e | e2e | rejected diagnostic + bounded retry |
| S11.5 | service restart recoverable e2e | e2e | 重启后等待 resume |
| S11.6 | TUI/Web projection e2e | e2e | prompt/list/main/status 一致 |
| S11.7 | CPU/IO observation e2e | e2e | 无持续全量扫描高负载 |

## P0: Audit Baseline And Inventory

### Goal

建立代码现状、已有模型、旧路径和风险点清单，避免直接进入局部修改。

### Scope

- 梳理现有 Goal/DoD、coverage、evidence、run checkpoint、projection、LLM input、resume/replan/restart 相关模型和函数。
- 标记已有字段与目标设计之间的映射关系。
- 搜索 core 中已有业务化、模板化、语言化、命令化特判。
- 冻结当前 e2e/live blocker 作为后续回归样本，不把 live workspace 现象直接写入 core。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- `internal/modules/application/workorchd/biz/appservice/**`
- `internal/modules/blocks/llm/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/**`

### Acceptance

- 输出一份字段映射和缺口清单，明确哪些已有字段可复用，哪些需要新增。
- 输出旧路径审计：resume/replan/restart、decision resolve、approval approve、contract violation、provider retry。
- 完成 core 边界搜索，并记录需要清理的特判。
- 未进入业务逻辑修改。

### Verification

- `rg` 边界搜索记录。
- 现有相关测试基线记录。
- 至少列出 3 条当前 live/e2e 可复现样本作为后续验证入口。

## P1: Scoped Command Main Path

### Goal

统一 TUI/Web/CLI/REST/embedded backend 的命令入口，保证所有运行推进都通过 EventHub command 进入 workorchd owner。

### Scope

- Canonical command：`resume`、`replan`、`restart`、`approve`、`reject`、`decision resolve`。
- `continue` 仅允许入口归一化为 `resume`，不形成第二套语义。
- command 必须携带 workspace scope、run/ticket/approval scope、request id、idempotency key。
- owner 接收后记录 accepted/rejected command projection。

### Primary Owner

workorchd appservice + workorchport/workorchclient。

### Primary Code Areas

- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/services/workorch/**`
- `internal/modules/application/workorchd/biz/appservice/**`

### Dependencies

- P0。

### Acceptance

- `/run resume` 首次执行、恢复执行、decision/approval 释放后继续，都进入同一 command 主路径。
- `replan` 不创建 replacement run，不清空旧 evidence。
- `restart` 明确创建 replacement run，并 supersede/关闭旧 active run。
- command 重复提交不会创建重复 run/step。
- TUI/Web 不直接调用 kernel owner 或 repository。

### Verification

- 单测覆盖 command normalization 和 idempotency。
- 集成测试覆盖 TUI/REST 两种入口触发同一 owner command。
- Projection 中可观察 command accepted/rejected revision。

## P2: Workspace Definition Revision And Gap Baseline

### Goal

把 Goal/DoD/template/profile 的当前定义固化为可追溯 revision，并在首次执行、resume、replan、restart 前生成 GapBaseline。

### Scope

- 定义或映射 `WorkspaceDefinitionRevision`。
- Goal/DoD 变更形成新 revision。
- GapBaseline 绑定 definition revision。
- 首次执行无历史 evidence 时，也必须生成 baseline。
- Restart 使用最新 definition 重新生成 baseline。

### Primary Owner

core model + workorchd appservice。

### Primary Code Areas

- `internal/modules/kernel/**`
- `internal/modules/application/workorchd/biz/appservice/**`
- repository/readmodel 相关实现。

### Dependencies

- P1。

### Acceptance

- 每个 run 都能追溯当前 definition revision。
- 首次 resume 前有 GapBaseline。
- replan 后不会把 decision resolution 文本当作新 Goal/DoD 权威定义。
- Goal/DoD revision hash 可进入 LLM C1，但不改变 C0 stable prefix。

### Verification

- 单测覆盖首次运行、Goal/DoD 更新、replan、restart 的 definition revision 行为。
- LLM cache/context 测试覆盖 definition 变化只影响 C1。

## P3: Evidence Ledger Foundation

### Goal

建立 accepted/rejected evidence 的权威记录，禁止未通过合同的输出进入 accepted result/artifact/memory。

### Scope

- 定义或映射 `EvidenceRecord`。
- 支持 evidence kind：`state_change`、`validation`、`assessment`、`artifact`、`decision`、`approval`、`diagnostic`。
- 支持 status：`accepted`、`rejected`、`superseded`。
- LLM 违例输出只能写 `diagnostic/rejected`。
- 所有 evidence 必须有 workspace/run/step scope 和 sourceRef。

### Primary Owner

core orchestrator + repository/readmodel。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- `internal/pkg/repository/**`
- `internal/modules/application/workorchd/biz/appservice/**`

### Dependencies

- P2。

### Acceptance

- state-changing side effect 前必须通过合同准入。
- rejected diagnostic 不会满足 DoD，不会写 artifact/result，不会写高价值 memory。
- approval/decision 结果能作为 governance evidence 被追溯。
- 被截断、redacted、partial snapshot 不会被标记为 exact complete-file evidence。

### Verification

- 单测覆盖 accepted/rejected evidence 写入、查询和 projection。
- 失败样本测试覆盖 LLM 违例输出不会进入 accepted ledger。

## P4: Coverage Ledger Authority

### Goal

让 DoD/Goal coverage 只由 accepted evidence 满足，消除 state_change、validation、assessment 互相误满足的问题。

### Scope

- 定义或映射 `CoverageLedger` 和 `CoverageItem`。
- 每个 coverage item 绑定 goalItem/dodItem、requiredEvidenceKinds、accepted/rejected evidence refs。
- missing evidence 可由 ledger 确定性计算。
- large DoD 只影响切片策略，不直接阻塞 completion。

### Primary Owner

core orchestrator。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- coverage/convergence 相关实现。

### Dependencies

- P3。

### Acceptance

- `satisfied` 只能由 accepted evidence 触发。
- `state_change` 不能由 assessment 文本满足。
- `validation` 不能由泛化“已验证”文本满足。
- rejected evidence 只影响 retry/repair，不满足 coverage。
- completion gate 能解释每个未完成 coverage 的 missing evidence。

### Verification

- 单测覆盖 evidence kind 组合与 missing evidence 计算。
- 回归测试覆盖历史误判：assessment 不能替代 state_change，state_change 不能替代 validation。

## P5: Deterministic NextAction Interpreter

### Goal

从 coverage/evidence/治理状态确定性推导下一步动作，替代 LLM 自行选择状态转移。

### Scope

- 定义或映射 `NextAction`。
- 支持 action type：`assess_gap`、`plan`、`state_change`、`validate`、`assessment`、`repair`、`request_decision`、`request_approval`、`complete`、`fail`。
- resume/replan/restart/provider retry/contract retry/repair/follow-up 统一进入 NextAction。
- NextAction 具备 idempotency key。

### Primary Owner

core orchestrator + scheduler。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- `internal/modules/application/workorchd/biz/appservice/**`

### Dependencies

- P4。

### Acceptance

- 每次 scheduler 调度前都能投影当前 NextAction。
- 有 pending approval 时不再发起 state-changing step。
- 有可恢复 rejected diagnostic 时进入 repair/retry，而不是直接 decision。
- 缺 GapBaseline 时进入 `assess_gap`。
- 所有 required evidence 满足时进入 `complete`。

### Verification

- 单测覆盖设计文档中的 NextAction 推导顺序。
- 多 workspace 并发测试覆盖 action scope 和 idempotency。

## P6: Stage Packet And LLM Context Rendering

### Goal

每次 LLM 请求都由 Stage Packet 渲染，保证长会话阶段衔接不依赖旧历史自然语言或全量扫描。

### Scope

- 定义或映射 `StagePacket`。
- 渲染 C0/C1/C2/C3/C4/C5 分区。
- Stage Packet 包含 definition revision、active focus、requiredOutput、accepted/rejected evidence refs、current failure、target snapshots、process evidence timeline。
- target snapshot 必须标记 full/partial/truncated/redacted。
- compact summary 只作辅助，不替代 ledger。

### Primary Owner

core orchestrator + LLM block。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- `internal/modules/blocks/llm/**`

### Dependencies

- P5。

### Acceptance

- 每个 LLM step input 都能重建当前 Stage Packet。
- 动态 failure/progress/retry 不污染 C0。
- 被截断 snapshot 不允许要求 exact complete file。
- current requested action、requiredOutput、accepted/rejected evidence 不被 context budget 裁掉。

### Verification

- LLM request 渲染单测。
- cache hash 测试：动态 C4 变化不改变 C0 stable prefix。
- context budget 测试：关键字段不可裁剪。

## P7: Contract Violation Recovery Loop

### Goal

允许 LLM 输出违例被接收、诊断、反馈和 bounded retry，但绝不执行或接受违例输出。

### Scope

- 定义或映射 `ContractViolation`。
- parse/schema gate、contract gate、side-effect admission gate 分层。
- 可恢复违例生成 rejected diagnostic evidence。
- retry 继承原 stage、requiredOutput、repairObjective、definition revision 和 evidence timeline。
- 超预算或不可恢复违例进入 decision/failed。

### Primary Owner

core orchestrator + LLM block。

### Primary Code Areas

- `internal/modules/kernel/orchestrator/**`
- `internal/modules/blocks/llm/**`

### Dependencies

- P6。

### Acceptance

- 违例输出不执行、不 accepted、不写 artifact/result、不写长期 memory。
- 可恢复违例自动 retry，同一 signature 有预算。
- retry 只修改 C4，不修改 C0/C1/C2。
- 不可恢复违例携带诊断进入 governance。

### Verification

- 失败样本单测覆盖字段缺失、output kind 错误、误返回 blocked_on_decision、缺 evidenceRefs。
- 测试覆盖 rejected diagnostic 不污染 coverage。

## P8: Projection And MVVP Consistency

### Goal

workorchd 成为唯一 ViewModel owner，TUI/Web 只显示 projection，不维护另一套业务状态。

### Scope

- WorkspaceViewModel revision 包含 workspace list、active workspace、run state、decision/approval summary、current prompt mode、allowed commands、coverage summary、current focus、latest diagnostic。
- 主视图、workspace 列表、状态栏、prompt 来自同一 revision。
- command 提交后显示 accepted/pending，并等待 revision 更新。
- SSE 支持断线重连和 last event id。

### Primary Owner

workorchd appservice + TUI/Web/port。

### Primary Code Areas

- `internal/modules/application/workorchd/biz/appservice/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/services/workorch/**`
- web/TUI 相关目录。

### Dependencies

- P5 可先行；完整 prompt/coverage 展示依赖 P6/P7。

### Acceptance

- TUI 切换 workspace 后，prompt、主视图、workspace list 同步刷新。
- waiting_decision、waiting_approval、running、idle/recoverable 状态全部来自 ViewModel。
- Web 服务状态页和 SSE stream 不因单个 projection 未 ready 白屏。
- 端侧不根据本地旧状态推断可用命令。

### Verification

- projection 单测。
- TUI presenter/ViewModel 测试。
- SSE reconnect 集成测试。
- live verification：两个 workspace 切换、decision replan、approval approve。

## P9: IO, History And Watchdog Scope

### Goal

降低长期运行 CPU/IO 压力，并把 watchdog 限定为外部调用健康检查和幂等补偿。

### Scope

- projection 优先读 read model，不全量扫描 workspace 文件。
- history 使用近期完整事件 + 历史 compact + ledger refs。
- 大对象通过 payloadRef/snapshotRef 按需读取。
- watchdog 仅覆盖 provider/MCP/tool/shell/web/file 外部调用超时、command accepted 后 step 未进入 terminal、approval/decision resolution 未推进。
- 服务重启后，旧 running 但无执行上下文的 run 刷新为 idle/recoverable/pending_activation。

### Primary Owner

workorchd appservice + core scheduler/runtime environment。

### Primary Code Areas

- `internal/modules/application/workorchd/biz/appservice/**`
- `internal/modules/kernel/orchestrator/**`
- repository/readmodel/history/snapshot 相关实现。

### Dependencies

- P1、P5、P8。

### Acceptance

- view refresh 不触发全量文件扫描。
- 每个 run/step 更新只重算受影响 workspace projection。
- watchdog 补偿具备 scope 和 idempotency key。
- watchdog 不替代 EventHub command 主路径。
- 服务重启后旧 running 不继续显示为 active running。

### Verification

- IO 回归测试或 instrumentation 记录。
- watchdog 单测覆盖超时和幂等。
- service restart e2e。

## P10: Template/Profile/Soul Strategy Binding

### Goal

把软件开发等业务策略下沉到 template/profile/Soul/policy/skill，并确保创建 workspace 时绑定到 workspace，运行期由 core 结构化消费。

### Scope

- 软件开发阶段策略：gap_analysis、implementation_slice、validation_slice、repair_slice、acceptance。
- 文件粒度、任务粒度、验证偏好、MCP/LSP 能力说明。
- DoD evidence policy：每个 DoD 需要哪些 evidence kind。
- 策略从 template 创建期绑定到 workspace，并进入 run executionContext。

### Primary Owner

workspaceTemplate/profile/Soul/policy/skill。

### Primary Code Areas

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- skill 文件与 template/profile policy 绑定实现。

### Dependencies

- P2、P4、P6。

### Acceptance

- Core 中没有软件开发专用判断。
- 软件开发 workspace 能追溯策略来源：template -> workspace binding -> run context -> StagePacket/LLM input。
- 非软件开发 template 不受软件开发策略影响。
- LSP/MCP 是能力配置，不是 core 默认强制行为。

### Verification

- template binding 单测。
- 非目标模板不受影响测试。
- 边界搜索证明 core 无业务化特判。

## P11: Full E2E And Live Verification

### Goal

验证完整闭环能在真实运行中稳定推进，并且多 workspace、LLM 违例、decision/approval、服务重启、TUI/Web 展示都符合设计。

### Scope

- 双 workspace 并发执行。
- 首次 resume。
- decision replan。
- approval approve/reject。
- contract violation retry。
- provider timeout/retry。
- service restart recoverable。
- TUI/Web projection。
- CPU/IO/memory 观测。

### Primary Owner

全链路。

### Dependencies

- P1-P10。

### Acceptance

- 两个 workspace 不互相阻塞。
- replan 后能通过 EventHub 主路径触发后续 NextAction。
- LLM 违例时有 rejected diagnostic + bounded retry，而不是无限 waiting_decision。
- TUI/Web 展示与 workorchd projection 一致。
- workorch CPU/IO 不出现持续全量扫描型高负载。
- 完成时能列出 satisfied coverage item 与 accepted evidence refs。

### Verification

- `go test ./... -count=1` 或明确记录不能全量运行的原因。
- e2e 自动管理 `./bin/workorch` 实例。
- live verification 记录 workspaceId、runId、command time、projection revision、LLM request hash、accepted/rejected evidence refs、最终状态。

## Cross-Cutting Acceptance Gates

每个功能点提交前都必须通过以下门禁。

### Gate A: Contract Audit

必须记录：

- Scope
- Observed state
- Expected state
- Deviation
- Owner
- Event path
- Ledger impact
- Stage Packet impact
- Contract gap
- Proposed change
- Boundary check
- Verification

### Gate B: Core/Template Boundary

Core 修改必须通过边界搜索：

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "templateId|soulId|workspace title|provider|DeepSeek|OpenAI|Anthropic|LSP|MCP" internal/modules/kernel internal/modules/application/workorchd/biz/appservice -g "*.go"
```

发现具体业务词不一定必然错误，但必须解释为什么它只出现在测试、fixture、template 数据或中立字段名中。

### Gate C: State Ownership

需要证明：

- owner state 只由 owner 更新。
- command 通过 EventHub 进入 owner。
- projection 由 owner/readmodel 生成。
- TUI/Web 不复制业务状态机。

### Gate D: LLM Context Integrity

涉及 LLM input/cache/context 的功能点必须证明：

- C0 不包含 workspace/run/current failure/current action。
- C1 包含最新 Goal/DoD definition revision。
- C3/C4 包含 Stage Packet、requiredOutput、accepted/rejected evidence、current failure。
- context budget 不裁剪关键合同。

### Gate E: Evidence Integrity

需要证明：

- 只有 accepted evidence 满足 coverage。
- rejected diagnostic 不满足 coverage。
- 未通过合同的输出没有 side effect。
- memory/compact 不作为 completion authority。

## Iteration Exit Checklist

每个 P 点完成时，在收口记录中补充：

```markdown
## Iteration Exit

- Feature ID:
- Implemented scope:
- Deferred scope:
- Changed files:
- Owner:
- Event path:
- Ledger impact:
- StagePacket impact:
- Core/template boundary:
- Tests:
- Boundary search:
- E2E/live verification:
- Known residual risks:
```

如果 `Deferred scope` 影响下一个功能点依赖，不能进入下一个功能点。

## Suggested First Three Iterations

为了避免再次发散，建议先只推进前三个可验收切片：

1. `P0 Audit Baseline And Inventory`
   - 不改业务逻辑，只建立字段映射、旧路径审计、边界搜索和测试基线。
2. `P1 Scoped Command Main Path`
   - 只统一 resume/replan/restart/approval/decision command 主路径和幂等投影。
3. `P2 Workspace Definition Revision And Gap Baseline`
   - 只确保任何执行前都有 definition revision 和 GapBaseline。

在 `P0-P2` 未验收前，不应继续修改 prompt、repair 策略、template 文案或 LLM retry 行为。否则仍会回到“LLM 输出不稳定 -> 单点修 prompt -> live workspace 仍不收敛”的循环。

## Current Scope Boundary

本文只负责拆分可交付功能点。具体代码修改必须在进入对应 `Pxx` 前另行补充 Contract Audit，并按该功能点验收条件逐项完成。
