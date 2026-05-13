# WorkOrch Runtime Stage Contract and Neutrality Design

本文定义 WorkOrch 面向长流程任务的阶段化运行合约。目标是解决反复单点修复后仍无法稳定推进的问题，同时明确核心框架必须保持功能中立：runtime 只能治理阶段、约束、证据、结果和恢复策略，不得内置软件开发、代码 patch、具体技术栈或具体业务领域规则。

## 权威合约声明

本文是后续 WorkOrch 功能开发、runtime 完善、能力扩展和缺陷收口的核心设计合约。任何涉及 WorkOrch runtime、orchestrator、LLM 执行、memory / recall、soul / skill / tool / mcp 能力选择、repair / replan、verification / acceptance、profile 映射的变更，都必须先核对本文。

后续实现必须满足：

- 不得突破本文定义的核心中立原则。
- 不得在 core runtime 中引入软件开发、代码 patch、测试框架、具体业务领域等特化逻辑。
- 新增或调整 runtime 流程时，必须能映射到本文定义的中立 stage、artifact、evidence、result、repair、replan 合同。
- 若旧文档、历史计划、局部收口方案与本文冲突，以本文为准；需要保留的合理内容应先抽象为中立规则，再并入本文或作为 profile 文档引用。
- 后续所有针对 WorkOrch 功能的开发完善，都必须在设计、实现和验证三个环节显式参照本文。

## 背景

近期多个 workspace 的执行核对显示，runtime 已经能够治理 provider retry、runtime repair、result policy、readable ID、state-changing repair 等底层问题，但在软件开发这类长流程任务中仍会出现持续循环：

- LLM 在尚未完成上下文理解时被过早要求输出 patch。
- 只读探索 graph 被统一视为实现阶段违规，导致 repair / replan 反复触发。
- runtime 对“应该开放判断”与“必须严格收敛”的阶段边界不够清晰。
- memory / recall / skill / tool / mcp / soul / prompt 已被接入，但缺少按阶段组织的输入合同，无法稳定为 LLM 提供决策所需信息。
- 修复容易滑向“为软件开发写死特殊逻辑”，破坏 WorkOrch 核心中立性。

因此后续不应继续围绕单个失败点打补丁，而应把 runtime 收口为“中立阶段状态机 + 阶段内 LLM 决策”的模式。

## 设计原则

### 核心中立

WorkOrch core runtime 只理解以下中立概念：

- workspace、run、step、stage、loop、attempt。
- Goal、DoD、input、artifact、evidence、result。
- stage contract、allowed output、forbidden output。
- state-changing、read-only、assessment-only、approval、decision。
- provider retry、runtime repair、replan、terminal result。
- memory recall / write 的价值等级与引用关系。

WorkOrch core runtime 不得理解以下领域概念：

- 什么是软件开发、代码实现、测试框架、语言栈、仓库结构。
- 什么文件必须创建或修改。
- 什么 patch 内容代表正确业务实现。
- 某类业务任务应使用何种技术方案。
- 某个领域的最佳实践或专用流程。

### 约束分层

WorkOrch 的阶段约束分三层：

| 层 | 职责 | 是否属于 core |
| --- | --- | --- |
| Runtime core | 阶段状态机、输出合约、恢复治理、证据/result、ID/lineage、memory 生命周期 | 是 |
| Capability / profile | 某类任务的 artifact 类型、tool/skill/mcp/soul/prompt 组合、阶段映射 | 否，属于可插拔能力层 |
| LLM decision | 在当前 stage contract 内选择方案、生成内容、解释缺口 | 否，由 LLM 执行 |

软件开发只能作为一个 profile 映射到中立阶段，不能成为 core runtime 的内置逻辑。

## 阶段模型

Runtime 统一使用中立阶段，不使用业务阶段名称作为核心状态。

| Stage | 目的 | 约束模式 | 允许输出 | 禁止输出 |
| --- | --- | --- | --- | --- |
| `exploration` | 收集上下文、识别问题、发现约束 | 开放判断 | read-only graph、tool read、context summary、candidate findings、blocker | 写 result 完成、无依据 state-changing、无限探索 |
| `planning` | 将 Goal / DoD 拆成可执行单元 | 半开放 | plan、implementation units、acceptance criteria、risk/blocker、stage transition proposal | 直接完成 run、绕过 DoD、空计划 |
| `execution` | 产生状态变更或明确无法变更 | 严格收敛 | state-changing artifact、tool action、patch-capable output、failed assessment、decision blocker | 只读 graph 循环、下钻 graph、placeholder output |
| `verification` | 用证据验证执行结果 | 严格证据 | verification tool result、evidence refs、failed/skipped/inconclusive assessment | 无证据 accepted、继续实现、弱证据通过 |
| `acceptance` | 对照 Goal / DoD 判定完成度 | 严格 result | accepted/rejected result、DoD coverage、evidence mapping | 新增实现、模糊完成、缺 result 终态 |
| `repair` | 针对失败证据做定向修复 | 继承原约束并收紧 | targeted correction、state-changing repair、failed assessment、blocker | 广泛重规划、降级为探索、重复同一失败 |
| `replan` | 原计划不成立时重新拆解 | 受控开放 | successor run proposal、new units、discard/keep rationale | 在旧失败路径继续循环、丢失 lineage |

## 阶段转换合同

### 标准路径

```text
created
  -> exploration
  -> planning
  -> execution
  -> verification
  -> acceptance
  -> completed | failed | blocked_on_decision
```

### 恢复路径

```text
execution | verification | acceptance
  -> repair
  -> execution | verification | acceptance

repair exhausted | plan invalid | repeated failure
  -> replan
  -> successor run
```

### 转换规则

| From | To | 必要条件 |
| --- | --- | --- |
| `exploration` | `planning` | 已形成足够上下文摘要，或达到探索预算 |
| `planning` | `execution` | 已产生至少一个可执行 unit，且 unit 绑定 Goal / DoD 覆盖点 |
| `execution` | `verification` | 已产生 state-changing artifact，或明确 failed/skipped 并可验证 |
| `verification` | `acceptance` | 已有可引用 evidence，且覆盖目标可判定 |
| `acceptance` | `completed` | accepted result，Goal / DoD coverage 完整 |
| `acceptance` | `failed` | rejected result，缺口不可自动修复或预算耗尽 |
| 任意阶段 | `blocked_on_decision` | 需要人工信息、审批、provider/config 修复或外部依赖 |
| `repair` | 原阶段 | 修复输出满足原阶段 contract |
| `repair` | `replan` | 同一 failure scope 重复失败或 repair budget exhausted |

### Failure Scope 与收敛预算

Runtime repair / duplicate / replan 治理必须同时识别两类中立 scope：

- 完整失败签名：用于精确区分同一目标下的不同失败形态。
- 目标级失败签名：从 `failureTargets`、`targetRef`、`conflictRef`、`serverId`、`tool`、`uri` 等中立键提取，用于识别同一失败目标在不同 step、graph node、loop、attempt 中反复出现。

当失败签名包含目标级 scope 时，node 名称、step ID、loop 编号或 graph 编号变化不得重置 repair budget。若同一 failure class + target scope 已达到 repair budget，runtime 必须进入 repair exhausted / duplicate / replan 治理，而不是继续派发普通 repair。

当失败签名不包含目标级 scope 时，runtime 不得臆造业务目标，也不得把无目标失败合并为同一预算；此时继续按完整失败签名、root step 或显式 lineage scope 收敛。

### Active Run 一致性

Workspace 的 `ActiveRunID` 只能指向同一 workspace 下的非终态 run。`completed`、`failed`、`cancelled`、`superseded` 均为终态，任何 workspace 都不得继续以这些 run 作为 active run。

Runtime 在以下边界必须执行 active-run 一致性核对：

- run 启动、恢复、完成、失败、取消、supersede 后。
- replan 创建 successor run 前后。
- 服务启动或运行恢复扫描时。
- runtime 验证脚本核对 workspace/run 执行过程时。

若发现 workspace 指向终态 run，runtime 必须清空 `ActiveRunID`，并按终态 run 状态收敛 workspace 状态：`completed` 映射为 workspace `completed`，`failed` / `cancelled` / `superseded` 映射为 workspace `failed`。若该 run 存在已解决 replan 决策，服务层恢复扫描可以在 workspace 回到无 active run 的终态后继续调度 successor，但 core runtime 不得通过业务领域逻辑推断 successor 内容。

## 阶段内约束模式

### 开放判断模式

适用于 `exploration` 和部分 `planning`。

Runtime 允许 LLM：

- 主动选择读取哪些上下文。
- 输出多个候选原因或候选方案。
- 暂不产生 state-changing artifact。
- 请求必要 tool / skill / mcp。
- 记录高价值 memory。

Runtime 必须限制：

- 探索预算。
- 重复读取同一信息。
- 无信息增量的循环。
- 在缺少 evidence 时写 accepted result。

### 严格收敛模式

适用于 `execution`、`verification`、`acceptance` 和大多数 `repair`。

Runtime 要求 LLM：

- 对当前 unit 给出可执行输出、失败 assessment 或 blocker。
- 输出必须能被 runtime 派发、验证或终态记录。
- state-changing 需求不得降级为只读 graph。
- verification 必须绑定 evidence。
- acceptance 必须绑定 Goal / DoD coverage。

Runtime 必须拒绝：

- shell-only / read-only graph 冒充实现进展。
- 下层 `executionGraph` 继续套娃。
- placeholder artifact。
- weak evidence 直接 accepted。
- assessment-only 阶段输出可执行字段。

当 run 级 `stateChangePolicy=must_produce_state_change` 时，`planning` 阶段可以先输出只读 graph 做上下文探索；但一旦 graph 中的 `llm/workspace_execution` 节点被调度到 `execution` 阶段，该 LLM 节点必须继承 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output` 与 state-changing guidance。普通 `file/read`、`file/list`、只读 tool 节点不因此被强制变更。

当 `execution/strict_convergence` 阶段的非只读步骤失败，且 run 级 contract 要求 `must_produce_state_change` 时，后续 runtime repair 必须继续携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。这包括 shell/test/build、patch/apply、file/edit、MCP tool 等执行或验证 state change 的步骤失败；普通 read observation 失败只有在缺失目标文件、DoD 缺口或其他明确 contract failure 时才提升为 state-changing repair。

若非 LLM 执行步骤失败原因是中立的瞬时执行超时（例如 runtime deadline / timeout，且尚未形成确定性失败证据），runtime repair 不应强制要求 state-changing 输出；应允许一次受预算约束的重试、缩小验证范围或返回 failed assessment / blocker。该规则只基于“执行超时”这一中立失败形态，不得写入特定命令、语言、测试框架或业务领域判断。

## Artifact 中立类型

Core runtime 不定义“代码 patch”这类领域概念，只定义中立 artifact 能力。

| Artifact type | 中立含义 | 示例 profile 映射 |
| --- | --- | --- |
| `state_change` | 对 workspace 状态产生可追踪变更 | 软件开发 profile 可映射为 patch/file write |
| `read_observation` | 读取或观察得到的上下文 | file read、web fetch、mcp query |
| `plan` | 可执行单元与验收点拆解 | implementation plan、migration plan |
| `evidence` | 验证结果或外部证明 | test result、build result、manual approval |
| `assessment` | 对 Goal / DoD 的判定 | accepted/rejected/inconclusive DoD assessment |
| `decision_request` | 需要人工或外部系统补充 | provider config、approval、missing requirement |
| `memory_write` | 可复用的高价值信息 | accepted input、necessary output、tool capability |

Capability / profile 可以把 `state_change` 绑定到具体 step kind，例如 `patch/apply`、`file/edit`、`mcp/update`，但 core 只按 state-changing contract 治理。

## Profile 映射规则

Profile 是 core 外层能力定义，用于把某类任务映射到中立 stage contract。

Profile 可以定义：

- 每个 stage 的推荐 prompt。
- 可用 skill / tool / mcp / soul。
- 允许 artifact type。
- state-changing artifact 的具体 step kind。
- evidence 的有效性标准。
- memory recall 的筛选优先级。

Profile 不可以改变：

- runtime 状态机。
- terminal result 要求。
- provider retry / repair / replan 的治理规则。
- ID / lineage / budget 规则。
- core 对 stage contract violation 的判定。

### 软件开发 profile 示例

该示例只作为 profile 映射，不得写入 core runtime 特判。

| 中立 stage | profile 行为 |
| --- | --- |
| `exploration` | 读取仓库结构、相关文件、已有测试和文档，形成问题摘要 |
| `planning` | 拆分实现 unit、DoD 覆盖点、验证命令 |
| `execution` | 生成 state-changing artifact，可映射为 patch/file edit |
| `verification` | 运行测试、检查 diff、收集 runtime evidence |
| `acceptance` | 对照 Goal / DoD 写 accepted/rejected result |
| `repair` | 针对失败 evidence 生成定向修复 |
| `replan` | 原实现路径不成立时创建 successor plan |

## Memory / Recall 合同

Memory 需要按阶段服务 LLM 决策，而不是堆叠 runtime 流水。

### 高价值 memory

高价值 memory 包括：

- 被 LLM 实际接纳的输入。
- 根据 Goal / DoD 产生的必要输出。
- 当前阶段的有效 plan / unit / acceptance criteria。
- 已验证 evidence 与其覆盖的 DoD。
- 已确认不可行的方案及原因。
- 当前可用 skill / tool / mcp / soul / prompt 的能力摘要。
- 人工 decision / approval 的结论。

### 低价值或噪音 memory

默认不应进入 recall 主上下文：

- provider retry 流水。
- 重复 step ID 列表。
- 已被 supersede 的旧 repair 过程。
- 只读扫描的原始日志。
- 没有被 LLM 接纳或没有影响后续决策的中间文本。
- 已过期的 blocker。

### 阶段化 recall

| Stage | recall 优先级 |
| --- | --- |
| `exploration` | Goal、DoD、workspace manifest、相关历史结论、可用能力 |
| `planning` | 探索摘要、约束、DoD、已有方案、不可行路径 |
| `execution` | 当前 unit、必要上下文、accepted plan、可用 state-changing capability |
| `verification` | 变更摘要、验证要求、evidence 标准、历史失败原因 |
| `acceptance` | Goal / DoD、evidence refs、coverage map、remaining gaps |
| `repair` | failure scope、失败 evidence、最近修复尝试、必须继承的 contract |
| `replan` | 原计划失败原因、可保留产物、需要重新拆解的目标 |

## Repair 合同

Repair 不是重新探索。Repair 必须继承来源阶段的约束，并根据失败类型收紧。

| 来源阶段 | repair 要求 |
| --- | --- |
| `exploration` | 只修复探索输出无效、上下文缺失或工具失败；不得直接进入 implementation 特判 |
| `planning` | 修复 plan 缺口，必要时要求明确 executable unit |
| `execution` | 必须回到 state-changing output、failed assessment 或 blocker |
| `verification` | 必须补 evidence、解释 weak evidence 或转 rejected |
| `acceptance` | 必须补 DoD coverage / result，不得新增实现 |

当 run 级 `stateChangePolicy=must_produce_state_change` 且失败输出包含 `missingDoDItems` 时，runtime repair 必须显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。后续 provider retry 也必须继承该约束，不能把 DoD 缺口修复降级为只读 exploration graph。

Repair exhausted 后必须二选一：

- 写 rejected result 并进入 failed / blocked_on_decision。
- 创建 replan successor run，并保留 predecessor lineage。

## Provider Retry 合同

Provider retry 始终只处理 provider transient failure，与 stage 语义无关。
Provider retry 预算必须来自 runtime 配置，默认 `maxProviderRetryAttempts=10`；不得在 core runtime 分支中写死固定业务场景次数。

允许：

- timeout。
- rate limit。
- context too large。
- provider stream incomplete。

禁止：

- 用 provider retry 修复 LLM contract violation。
- retry step 成为新的 retry root。
- retry 后丢失原 stage contract。
- retry 后把 state-changing 要求降级为 exploration。

### LLM Stream 完整性

Runtime LLM 的结构化输出必须以完整 provider 响应为准。

- HTTP SSE / compatible chat completion provider 只有收到 `[DONE]` 才能把 streamed content 视为完整输出。
- EOF 但未收到 `[DONE]` 必须归类为 `provider_stream_incomplete`，不得把已收到的 partial delta 交给 LLM decision parser。
- timeout 且已有 partial stream delta 时仍按 `provider_timeout` 治理，但 failure output 必须记录 `streamDiagnostics`，至少包含 `doneReceived`、`streamIncomplete`、`receivedDeltaCount` 和 `receivedBytes`。
- 已收到 done 但 JSON schema 无效时，归类为 LLM decision/schema 问题，不得误判为 provider stream incomplete。

## 相关文档整理与合并策略

有必要整理 `docs/` 中与 runtime 执行、memory、LLM context、repair/replan、soul/skill/mcp 能力选择相关的文档，但不应把所有文档机械合并到本文。本文应成为“阶段化 runtime 合约”的权威入口；历史收口记录、领域 profile、API/UI 设计和已完成任务流水应保留为引用或归档材料。

### 整理目标

- 让阶段合约、输出合约、恢复治理、memory recall、profile 边界在一个权威文档中闭合。
- 减少同一规则散落在多份文档中，导致实现时按旧计划或局部计划修补。
- 保持 core runtime 中立，不把软件开发 profile 的细节并入 core contract。
- 为后续逐项收口提供单一 checklist。

### 文档角色分层

| 层级 | 文档角色 | 处理方式 |
| --- | --- | --- |
| 权威合同 | 本文、`workorch-runtime-core-flow-contract.md` | 保留并同步引用；本文负责 stage/profile 中立合约，core flow 负责现有状态机和恢复优先级 |
| 专项合同 | lineage、readable ID、provider retry、memory recall、prompt context | 抽取通用规则并入本文；专项文档保留细节和历史落地记录 |
| Profile 文档 | software-dev、code-agent、DoD repair | 只抽取可泛化规则；领域行为留在 profile 层 |
| 能力文档 | soul、skill、mcp、tool、web、patch | 抽取“能力如何被 stage 使用”的通用边界；具体 API、UI、数据模型留在原文 |
| 历史计划/审计 | closure plan、audit、progress、status | 不合并正文；只作为追溯引用，完成后可归档 |

### 建议并入本文的内容

| 来源文档 | 应并入本文的内容 | 并入位置 |
| --- | --- | --- |
| `workorch-runtime-core-flow-contract.md` | 状态机、阶段输出合约、failure 分类、recovery 优先级、result 终态规则 | 阶段转换合同、阶段内约束模式、Repair 合同 |
| `workorch-runtime-lineage-and-assessment-contract.md` | lineage 必须结构化、assessment-only 边界、DoD assessment 与 evidence 归因 | Artifact 中立类型、验证脚本要求 |
| `workorch-runtime-readable-id-and-recovery-contract.md` | run-scoped ID、graph scope、provider retry root、memory 噪音治理 | 阶段转换合同、Memory / Recall 合同、验证脚本要求 |
| `runtime-recovery-governance-closure-design.md` | provider retry / runtime repair 预算、duplicate failure、manual escalation、auto-governance replan | Repair 合同、Provider Retry 合同、实施收口计划 |
| `graph-followup-dod-assessment-closure-plan.md` | graph follow-up 超限后进入 assessment-only、DoD 缺口驱动 repair/replan | 阶段内约束模式、Repair 合同、验证脚本要求 |
| `llm-decision-memory-improvement-plan.md` | accepted input memory、necessary output memory、skill/soul/prompt/tool/mcp memory 边界 | Memory / Recall 合同、Stage Input Bundle 合同 |
| `workspace-memory-recall-traceability-plan.md` | activation / step / recovery recall trace、可观测性字段 | Memory / Recall 合同、验证脚本要求 |
| `codex-like-llm-prompt-context-design.md` | Goal / DoD 不可压缩、上下文分层、budget manifest、prompt normalization | Stage Input Bundle 合同 |
| `workspace-soul-routing-plan.md` | stage-specific soul binding、选择优先级、run execution context | Profile 映射规则、Stage Input Bundle 合同 |
| `mcp-capability-management-design.md` | MCP capability 作为可选择能力目录，不进入 core 业务规则 | Profile 映射规则、Stage Input Bundle 合同 |

### 不应并入本文的内容

| 内容 | 原因 | 保留位置 |
| --- | --- | --- |
| 具体软件开发实现流程、测试命令、代码 patch 规则 | 属于 software-dev profile，不属于 core runtime | `software-dev-*` 文档或后续 profile 文档 |
| Web/TUI/API 交互细节 | 属于产品交互和服务接口，不是 runtime stage contract | 对应 Web/TUI/API 文档 |
| 数据库存储路径、file backend layout 细节 | 属于持久化实现，不是阶段合约 | `workspace-data-layout-plan.md` |
| 已完成任务流水和日期记录 | 只用于追溯，不应污染权威设计 | progress/status/audit 文档 |
| 某个具体 workspace 的排障样本 | 可作为验证 case，不应成为 core 规则 | 部署核对记录或 e2e fixture |

### Stage Input Bundle 合同

为避免 memory、soul、skill、tool、mcp、prompt 分散注入，后续 runtime 应为每个 stage 形成中立的 input bundle。Core 只负责 bundle 的结构与引用，profile 负责选择具体能力。

| Bundle section | Core 含义 | 来源 |
| --- | --- | --- |
| `contract` | Goal、DoD、runtimeStage、allowed artifact、forbidden output、result policy | run / goal / dod / stage contract |
| `continuity` | 当前 unit、上一步有效输出、必要 lineage、active blockers | orchestrator / result / ticket |
| `memory` | 被筛选后的 accepted input、necessary output、evidence、decision memory | memory recall |
| `capabilities` | 当前 stage 可用的 skill/tool/mcp/soul/prompt 引用与能力摘要 | profile / capability registry |
| `evidence` | 当前可用于 verification / acceptance 的证据引用 | step output / artifact / result |
| `budget` | stage、repair、provider retry、context budget | runtime governance |

Stage input bundle 必须遵守：

- Goal / DoD 不可被压缩、改写或用摘要替代。
- memory recall 只进入高价值决策信息，不堆叠 runtime 流水。
- skill/tool/mcp/soul/prompt 进入 bundle 时只提供能力摘要和约束，不注入领域硬编码。
- repair bundle 必须包含 failure scope、来源阶段、失败 evidence、已尝试修复和必须继承的 contract。
- profile-specific bundle 字段必须位于 profile namespace，不能提升为 core 字段。

### 后续文档收口顺序

| 顺序 | 动作 | 输出 |
| --- | --- | --- |
| 1 | 从 runtime core / lineage / readable ID / recovery 文档抽取通用 runtime 合同 | 更新本文对应章节，保留原文作为细节引用 |
| 2 | 从 memory / prompt context 文档抽取 stage input bundle 与 recall 规则 | 完善 Memory / Recall 合同和 Stage Input Bundle 合同 |
| 3 | 从 soul / skill / mcp 文档抽取 capability/profile 边界 | 完善 Profile 映射规则 |
| 4 | 从 software-dev 文档抽取可泛化规则，领域内容留给 profile | 新增或整理 software-dev profile 文档 |
| 5 | 标记历史 closure/progress/audit 文档的权威性状态 | 避免后续实现继续引用过期计划 |

## 验证脚本要求

后续部署核对脚本需要按 stage contract 验证，而不是只按固定软件开发路径验证。

必须覆盖：

- 每个 step 都有 runtime stage 或可从结构化 input 推导 stage。
- `exploration` / `planning` 允许 read-only output，但受预算限制。
- `execution` 的 state-changing requirement 不允许 read-only graph。
- `verification` accepted 必须有 evidence refs。
- `acceptance` terminal 必须有 result。
- `repair` 必须继承来源 stage contract。
- `replan` 必须创建 successor run 并保持 lineage。
- profile-specific 检查只能放在 profile verifier 中，不能写入 core verifier。

## 实施收口计划

| ID | 优先级 | 内容 | 验收 |
| --- | --- | --- | --- |
| STG-01 | P0 | 引入显式 `runtimeStage` / stage contract helper | step input 可稳定表达 stage，不再依赖业务文案或错误字符串 |
| STG-02 | P0 | 将现有 planning / execution / assessment-only contract 归一到 stage contract | core contract validator 只按 stage + artifact type 判断 |
| STG-03 | P0 | 区分开放判断与严格收敛模式 | exploration/planning 不被误判为实现违规；execution/verification/acceptance 不再无限只读循环 |
| STG-04 | P0 | Profile 层定义软件开发映射 | 软件开发能力不污染 core runtime |
| STG-05 | P1 | Memory recall 按 stage 组织 | recall 主上下文只保留高价值决策信息 |
| STG-06 | P1 | Repair 继承来源 stage contract | provider retry / repair / repair-of-repair 不丢失约束 |
| STG-07 | P1 | 更新验证脚本为 core + profile 两层 | core verifier 保持中立，profile verifier 检查软件开发映射 |
| STG-08 | P2 | 清理旧的字符串匹配与领域化提示 | contract violation 使用结构化字段表达 |

## 当前落地记录

- 2026-05-13：
  - 已完成 STG-01 第一批收口。
  - orchestrator 新增显式 `runtimeStage` 输入字段与统一 helper，作为 core runtime 的中立阶段标识。
  - 初始 LLM step、graph node、graph follow-up、completion follow-up、assessment-only follow-up、runtime repair、provider retry、decision resolution、approval resolution 已开始显式写入 `runtimeStage`。
  - `validateLLMExecutionOutputContract` 和 follow-up graph state-changing contract 已开始优先基于 `runtimeStage` 判断，不再只依赖 `soulStage`。
  - 当前仍保留 `soulStage` 作为兼容过渡信号；后续 STG-02 / STG-03 继续把 open/strict 模式、assessment-only、repair 继承和 profile 映射归一到 stage contract。
- 2026-05-13：
  - 已完成 STG-03 第一批收口。
  - core runtime 新增 `runtimeStageMode`，当前支持 `open_judgment`、`strict_convergence`、`structured_governance`。
  - `runtimeStageMode` 已随 `runtimeStage` 一起进入 step input、execution context package、memory trace 和 LLM provider input。
  - blocks/llm prompt 已显式接入 `runtimeStage` / `runtimeStageMode`，让 LLM 在 prompt 层知道当前是开放判断还是严格收敛。
  - 当前仍未完成 profile 层的独立阶段映射与专用 verifier；后续需要继续推进 STG-02 / STG-04 / STG-07，把 profile-specific 规则完全从 core runtime 中抽离。
- 2026-05-13：
  - 已完成 STG-02 / STG-06 / STG-08 追加收口。
  - core runtime 已移除通过 `graphNodeId` 文本关键词推断 state-changing graph node 的逻辑；graph node 是否必须产生 state-changing output 只读取显式 `stateChangingOutputRequired` contract。
  - execution graph node 的阶段判断已从 `soulStage=execution` 切换到中立 `runtimeStage=execution`。
  - planning / exploration 保持 `open_judgment` 语义，即使 run 级 `executionContract.stateChangePolicy=must_produce_state_change`，也不在 planning 阶段强制 patch；state-changing 要求在 execution / repair 等严格收敛阶段执行。
  - repair 新写入的 `repairObjective` 已改为 `produce_state_changing_output`；旧 `produce_state_changing_implementation` 仅作为历史输入兼容识别，不再作为新输出。
  - state-changing repair guidance 已改为中立的 state-changing output 要求，profile 层仍可把该 artifact 映射为 patch/file write 等具体能力。
- 2026-05-13：
  - 已完成 execution 失败 repair 追加收口。
  - `must_produce_state_change` run 中，`execution/strict_convergence` 阶段的非只读步骤失败会触发 state-changing repair contract，避免测试/构建/执行失败后的 repair 退回只读分析。
  - runtime repair ID 对普通 graph-scoped 失败使用失败 graph node 标签，例如 `repair-001-of-graph-003-003-run-go-test`；provider retry / runtime repair 等 recovery 外壳仍按归一 root 生成，避免再次出现 provider retry 壳嵌套。

## 非兼容性说明

当前阶段以彻底收口 WorkOrch 能力为目标，不考虑旧行为兼容。允许做以下破坏性调整：

- 替换旧的 `soulStage` 推断方式。
- 要求所有新 step 显式携带 `runtimeStage`。
- 重构 output contract validator。
- 将软件开发相关 prompt / tool 选择迁移到 profile 层。
- 清理旧 memory recall 中的低价值 runtime 流水。

但即使不考虑兼容，也不得破坏核心中立原则。

## 核对清单

后续每次收口前先核对：

1. 新逻辑是否只使用中立 stage / artifact / evidence / result 概念。
2. 是否把软件开发、patch、测试框架等领域语义写进 core runtime。
3. 当前阶段是开放判断还是严格收敛。
4. LLM 是否获得了该阶段必要的 Goal / DoD / memory / skill / tool / mcp / soul / prompt。
5. 输出是否满足当前 stage contract。
6. repair 是否继承来源阶段约束。
7. provider retry 是否只处理 provider transient failure。
8. terminal run 是否有符合状态的 result。
9. profile-specific 检查是否位于 profile 层。
10. 验证脚本是否能区分 core contract 与 profile contract。
