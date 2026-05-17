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

当失败同时包含 `serverId` 与 `tool` 时，目标级 scope 应组合为同一外部能力目标，例如 `serverId=<id>|tool=<name>`；只有缺少其中一项时才退化为单独的 `serverId` 或 `tool`。这仍然是能力目录层的中立引用，不得解释为某个具体业务服务。

当失败签名包含目标级 scope 时，node 名称、step ID、loop 编号或 graph 编号变化不得重置 repair budget。若同一 failure class + target scope 已达到 repair budget，runtime 必须进入 repair exhausted / duplicate / replan 治理，而不是继续派发普通 repair。

目标级 scope 不得跨越当前有效 artifact / graph root 窗口无限合并。若新的 state-changing graph、repair graph 或 successor root 已经重新写入同一 target，后续 build/test/verification 失败应优先按当前 graph root / recovery root 计算 repair budget；历史同 target 失败只能作为上下文证据和风险信号，不得直接耗尽新 artifact 版本的修复机会。

外部能力依赖失败是例外中的中立子类：当 `mcp/tool_call` 或等价外部 capability 对同一 `failure class + target scope` 反复返回不可用、未找到、权限拒绝、配置缺失等确定性失败时，新的 graph root 或 loop 不应把该外部依赖预算重置为 0。runtime 应收敛到 rejected result、blocked_on_decision 或 replan，而不是继续要求 LLM 生成同一能力调用。是否换用其他能力、补充配置或继续重试，必须通过 capability/profile 或治理决策表达，core 不得内置具体服务名、工具名或业务 fallback。

当失败签名不包含目标级 scope 时，runtime 不得臆造业务目标，也不得把无目标失败合并为同一预算；此时继续按完整失败签名、root step 或显式 lineage scope 收敛。

当当前失败 step 同时携带历史 `repairOfFailureSignature` 和自身 `failureSignature` 时，scope 判定必须优先使用当前失败 step 输出中的 `failureSignature` / `failureClass`。历史 repair lineage 只能作为上下文与来源说明，不得覆盖当前失败证据。真正的 provider/runtime 调度外壳若只记录了 provider retry 状态且没有新的有效失败签名，可以继续按原始 repair scope 计数；除此以外，repair-follow-up 自身产生的 runtime contract violation、patch contract violation、验证失败或能力失败都必须作为当前失败证据进入下一轮治理。该规则适用于所有中立失败类型，尤其用于避免“上一轮失败签名”串线到新的执行超时、验证失败或能力 contract violation。

若当前失败签名包含短目标引用，runtime 可以用当前 workspace manifest 做唯一后缀解析，以便把失败证据定位到实际资源；若存在多个候选，必须保留原始短引用并标记为未确定目标，不得猜测具体资源。该解析只基于 workspace 中已知资源路径这一中立事实，不得依赖文件类型、语言、测试框架或业务约定。

### Replan Successor Lineage

当 repair exhausted、duplicate failure 或人工/自动治理决策要求 `replan` 时，successor run 必须继承 predecessor 的 Goal / DoD / execution contract，并在 `replanContinuation` 中记录中立 lineage：

- predecessor run ID。
- 触发 replan 的 ticket / resolution 引用。
- ticket reason 与 resolution payload。
- 触发 replan 的 failure context，包括 failure class、failure signature、failure target scope、failed step id/kind、failed step output、最近 repair attempts 或可用摘要。

Successor 的首个 LLM step 不应只知道“需要重规划”。它必须获得足够的失败证据来判断哪些产物可以保留、哪些路径已失败、哪些 contract 仍然必须继承。若 successor 被设为 state-changing repair continuation，则其初始输入必须继续携带 `stateChangingOutputRequired`、`repairObjective` 与对应 guidance；同时不得把历史 provider/model 等运行期动态绑定写入 successor 的恢复上下文。

`governanceHistory`、`replanContinuation.failureContext`、`replanContinuation.ticketContext` 必须是 summary/ref-only lineage。允许保留 failure class、signature、target scope、failed step id/kind、attempts、recovery policy、failed step output summary、target snapshot metadata、ticket/resolution refs 与人工决议摘要；禁止携带 `failedStepInput`、完整 `executionContext`、完整 `executionGraph.nodes[].input.diff/content`、raw LLM stream、raw preview、provider/model 动态绑定、target snapshot `content`。若需要完整证据，必须通过 step/tool/LLM raw ref 按需读取。该规则是核心中立约束：它只约束运行态数据形态，不得引入具体应用领域判断。

### Active Run 一致性

Workspace 的 `ActiveRunID` 只能指向同一 workspace 下的非终态 run。`completed`、`failed`、`cancelled`、`superseded` 均为终态，任何 workspace 都不得继续以这些 run 作为 active run。

Runtime 在以下边界必须执行 active-run 一致性核对：

- run 启动、恢复、完成、失败、取消、supersede 后。
- replan 创建 successor run 前后。
- 服务启动或运行恢复扫描时。
- runtime 验证脚本核对 workspace/run 执行过程时。

若发现 workspace 指向终态 run，runtime 必须清空 `ActiveRunID`，并按终态 run 状态收敛 workspace 状态：`completed` 映射为 workspace `completed`，`failed` / `cancelled` / `superseded` 映射为 workspace `failed`。若该 run 存在已解决 replan 决策，服务层恢复扫描可以在 workspace 回到无 active run 的终态后继续调度 successor，但 core runtime 不得通过业务领域逻辑推断 successor 内容。

Workspace outcome 与 taskProgress 投影必须优先反映最新终态证据：当最新终态 result 的时间不早于最新 run 事件时，outcome / taskProgress 应共同指向该 result 对应 run；当存在更新的终态 run 事件但该 run 尚无 result 时，outcome / taskProgress 应指向该最新 run 及其失败/终态原因。历史 accepted / rejected result 必须继续保留在 results 列表中，但不得覆盖更新 run 的终态状态。若 snapshot / read model 中的 event records 因分页、保留窗口或恢复重放而滞后，更新的 terminal result 必须可以覆盖旧 run event，避免 workspace 已由最新 run 失败/完成但 outcome 或 taskProgress 仍投影到历史 run。

Result 事件与持久化 marker 必须表达 result 的真实状态。`accepted` result 才能生成 `event/accepted` 标识；`rejected` result 必须生成 `event/rejected` 标识，即使当前事件 topic 仍用于通知 result materialized / refresh，也不得让 rejected result 在文件系统或 read model 中留下 accepted marker。

Result summary / artifact summary 必须以 runtime 已记录事实为准进行最终校正。若同一 run 已存在已应用 patch、tool call、validation、artifact 等 state-changing 或 evidence 记录，accepted result、summaryDetail、result-summary artifact、deliverable-manifest 不得保留与这些事实矛盾的 LLM 自述，例如“仅 assessment-only”“无新增可执行操作”“不包含新的执行性修复”。Core 只允许基于中立 runtime evidence 类型做事实校正，不得推断该变更属于代码、测试、配置或任何具体业务领域；领域解释继续由 profile / template / soul 提供。

Run observability / snapshot 的 outcome 也必须遵守同一语义：`superseded` run 的 outcome 首先表达该 run 已被后续 lineage 取代，而不是继续被其历史 `rejected` result 投影为 `failed`。换言之，`rejected -> superseded` 只保留为历史证据，不再是当前 run observability 的有效终态结论。

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
- 下层 `executionGraph` 继续依赖 `llm/workspace_execution` 决策节点套娃。
- placeholder artifact。
- weak evidence 直接 accepted。
- assessment-only 阶段输出可执行字段。

### Goal / DoD Coverage Assessment 合同

当 acceptance / verification 需要对 Goal 与 DoD 做完成度判断时，runtime 必须把覆盖判断建模为中立 coverage 合同：

- Goal 中被拆出的 leaf requirement 必须获得确定性、可复现的 `goalRequirementId`，ID 来源只能是 requirement 顺序与规范化文本，不得使用随机值或领域语义。
- DoD 与 Goal coverage 必须分别表达；DoD item 通过不代表 Goal leaf 自动覆盖，除非 `dodAssessment.goalCoverage.items[]` 明确给出对应 Goal requirement、状态和 evidence refs。
- profile 声明的 required `acceptanceDimensions` 与 required `deliverableExpectations` 必须分别通过 `dodAssessment.acceptanceDimensions.items[]` 与 `dodAssessment.deliverableCoverage.items[]` 表达；core 只校验声明的 `id`、状态和 evidenceRefs / artifactRefs，不得根据 id 名称推断业务含义。
- coverage item 的中立标识允许来自稳定字段别名集合，而不能强绑定单一字段名：Goal coverage 可使用 `goal` / `goalItem` / `item` / `requirement`；acceptance dimension 可使用 `id` / `dimension` / `key` / `name`；deliverable coverage 可使用 `id` / `item` / `deliverable` / `name` / `path`。runtime 只能把这些字段视为“coverage 条目标识”的结构别名，不得据此推断业务语义。
- `missingGoalCoverage` 中的每个条目都必须在 assessment-only correction 中成为独立的 `dodAssessment.goalCoverage.items[]` 条目；不得把多个缺口合并成一个宽泛描述后宣称满足。
- `missingDeliverableCoverage` 中的每个条目都必须在 assessment-only correction 中成为独立的 `dodAssessment.deliverableCoverage.items[]` 条目；满足项必须复制 required deliverable id，并引用具体 `evidenceRefs` 或 `artifactRefs`。
- runtime 可以为 `missingGoalCoverage` 附加中立 `evidenceCandidates`，候选来源只能是当前 run 已完成或失败 step 的 step id、kind、state、output 文本与 requirement 文本的通用匹配。候选证据不是自动通过依据，LLM 必须判断其 output 是否真实支持该 Goal leaf，并在满足时引用具体 `evidenceRefs`。
- assessment-only correction 输出缺少可由 `missingGoalCoverage` 唯一确定的 requirement 文本或 `goalRequirementId` 时，runtime 可以在 completion gate 前进行中立 schema normalization：按条目顺序或稳定 ID / 文本匹配补齐 requirement 元数据，并剔除非可追踪 `evidenceRefs`；若满足项仅给出不可追踪引用，可回填该缺口的中立候选 step id。该 normalization 只修复合同结构和证据引用形态，不代表 runtime 自动接受候选证据的业务含义。
- completion gate 在校验 `dodAssessment` 前必须基于 runtime evidence catalog 规范化 `evidenceRefs`：已完成 step、通过的 validation result、已应用 patch、tool call 等 runtime 已知证据可以作为 canonical evidence；LLM 输出的 graph node 别名、`runId/nodeId`、旧 `block/nodeId/patch/N`、`runId/<assessment-follow-up-step>-<node-id>` 等结构别名必须映射到 canonical step / validation / patch ref；别名匹配应支持 node id 的 `_` / `-` 等价与 graph step 顺序前缀剥离，但只能映射到同 run 内可追踪且可用的 runtime evidence。已知失败、无效、不可追踪或无法映射的引用必须从满足项中剔除。若剔除后某个 satisfied item 不再有有效 evidence，completion gate 必须拒绝该 assessment，而不能让历史失败 evidence 或 LLM 自造 ref 支撑 accepted result。
- 若 LLM 将 `dodAssessment.goalCoverage`、`dodAssessment.acceptanceDimensions` 或 `dodAssessment.deliverableCoverage` 误输出为 bare array，runtime 可以在解析前中立规范化为 `{ "satisfied": <由 items 状态推断>, "items": [...] }`。该容错只修复 schema 形态，不得改变 item 文本、证据引用、artifact 引用或业务判断。
- completion gate 因 `goalCoverage`、`acceptanceDimensions` 或 `deliverableCoverage` 缺失、未满足或结构不完整失败时，runtime 必须保留对应 coverage 诊断，并在后续 repair 中进入 assessment-only coverage correction；不得把纯 assessment 合同缺口误升级为普通 execution repair。
- assessment-only correction 不得返回 executable work；只能返回 completed / failed `dodAssessment` 或真实治理 blocker。
- assessment-only contract correction 的输入必须是 narrow assessment package：不得携带 `executionGraph`、patch/shell/tool/memory 可执行字段，也不得携带 `governanceHistory`、`replanContinuation`、历史 `convertedDecision/llmDecision`、state-changing repair action package、patch repair context 或 provider retry lineage。correction 只用于修正 assessment schema / coverage / evidence refs，不得被旧 replan 或 decision 语义诱导成新的执行规划。
- 若 assessment-only correction 给出 failed assessment，但同一缺口仍存在候选证据或满足项矛盾，runtime 应在 repair budget 内再次派发 assessment-only correction，而不是立即写 rejected result。预算耗尽后才进入 rejected / blocked / replan 治理。
- rejected result 的 `SourceRef` 应指向最终产生 rejection 的终态 assessment / repair step；root step、failed step、候选证据 step 应保留在 `EvidenceRefs` 中，确保终态来源与 lineage 都可追踪。

若 `execution/strict_convergence` 阶段的 LLM graph 节点返回的 `executionGraph` 已经收敛为具体 runtime capability，且图中至少包含一个有效 state-changing 节点、没有继续包含 `llm/workspace_execution` 决策节点、state-changing 节点不依赖新的 LLM 决策祖先，则 runtime 可以把该图作为新的 graph scope 派发执行。该判断只基于 capability kind、依赖关系和 capability input contract，不得引入业务领域语义。

当 run 级 `stateChangePolicy=must_produce_state_change` 时，`planning` 阶段可以先输出只读 graph 做上下文探索；但一旦 graph 中的 `llm/workspace_execution` 节点被调度到 `execution` 阶段，该 LLM 节点必须继承 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output` 与 state-changing guidance。普通 `file/read`、`file/list`、只读 tool 节点不因此被强制变更。

LLM 输出 `executionGraph` 时，runtime 必须按当前 Soul 绑定阶段解析有效 Soul 绑定，并用中立 capability contract 校验 graph 节点：若 `allowedToolKinds` 非空且不含 `*`，所有节点 kind 必须在允许列表内；若 `allowedMCPServerRefs` 非空且不含 `*`，MCP 节点引用的 server 必须在允许列表内。该校验只依据 Soul 的能力白名单，不得写入具体业务领域或特定工具 fallback 规则。

当 `execution/strict_convergence` 阶段的非只读步骤失败，且 run 级 contract 要求 `must_produce_state_change` 时，后续 runtime repair 必须继续携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。这包括 shell/test/build、patch/apply、file/edit、MCP tool 等执行或验证 state change 的步骤失败；普通 read observation 失败只有在缺失目标文件、DoD 缺口或其他明确 contract failure 时才提升为 state-changing repair。

若非 LLM 执行步骤失败原因是中立的瞬时执行超时（例如 runtime deadline / timeout，且尚未形成确定性失败证据），runtime repair 不应强制要求 state-changing 输出；应允许一次受预算约束的重试、缩小验证范围或返回 failed assessment / blocker。该规则只基于“执行超时”这一中立失败形态，不得写入特定命令、语言、测试框架或业务领域判断。

当 provider/config 决策解除后需要重试原失败 LLM step 时，decision resolution 只能补充“已授权重试、已提供信息、恢复动作”等恢复上下文，不得覆盖原 step 已存在的 `runtimeStage`、`runtimeStageMode`、Soul 绑定阶段、`stateChangingOutputRequired`、`repairObjective` 等阶段合同。换言之，`decision` 是恢复入口，不是被重试任务的执行阶段；provider retry 必须继续继承失败 step 的原阶段语义。

当 execution graph 以并行 batch 方式调度时，batch 完成后的续跑必须以本批次所属 graph root / lineage 为锚点计算 dependency-satisfied 节点，不得用 run 内全局“最新 completed graph step”推断当前 graph。全局最新节点可能属于其他 repair、replan 或历史 graph scope；runtime 必须按当前批次 root 继续推进，直到该 graph scope 内没有可执行节点，再进入 follow-up / verification / acceptance。

当 state-changing 输出依赖具体 runtime capability 时，prompt / tool schema / repair guidance 必须明确该 capability 的中立能力边界。若某个 state-changing capability 只支持有限操作，LLM 不得用未声明语义伪造操作。例如某能力只支持“写入完整内容”和“应用 diff”时，不得把“空内容写入”解释为删除。Runtime 必须拒绝不满足 capability contract 的输出，并把拒绝原因作为下一轮 repair evidence；prompt 层也必须把该约束暴露给 LLM，减少重复输出必然失败的 artifact。

能力边界属于 runtime capability contract，不属于业务领域规则。Core 可以声明“某 capability 不支持 delete、content 不能为空、diff 必须完整”等中立能力事实；但不能据此推断某个业务文件是否应该删除、保留或重命名。

### 已知可读资源不得升级为人工缺失信息

在任意阶段中，如果 LLM 返回 `blocked_on_decision/provide_missing_info`，但其 `missingFields` / `requiresHumanFields` 指向的内容已经存在于 runtime 可确认的 workspace manifest、known resource list、read-only evidence 或当前工作目录中，则该 decision 不属于真实人工阻塞。

Runtime 必须按中立规则处理：

- 不得要求用户粘贴已知可读资源的内容。
- 不得把“已知资源内容未进入当前 prompt”误判为人工缺失信息。
- 应将该 decision 转换为 runtime repair / continuation，并把对应资源的当前快照或可读取引用放入 repair 输入。
- `workspaceFiles`、`existingFiles`、`readableResources`、`knownResources`、`workspaceFileManifest` 等字段都只能作为“资源是否已知”的中立证据；若这些字段不完整，runtime 仍应以当前 workspace 工作目录中真实可读资源为准做兜底核对。
- 若资源读取失败，应按普通 read observation failure 或 workspace path failure 收敛，而不是继续追问用户。

该规则只识别“资源是否已知且可读”这一中立事实，不得写入 `main_test.go`、测试文件、代码文件、业务配置文件等领域特判。Profile 可以决定哪些资源更重要，但 core 只负责避免把可自动读取的信息升级为人工决策。

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
- evidenceRequirements：按 profile 命名空间声明某类 coverage / dimension / deliverable 需要或拒绝的 step kind。
- memory recall 的筛选优先级。

Profile 不可以改变：

- runtime 状态机。
- terminal result 要求。
- provider retry / repair / replan 的治理规则。
- ID / lineage / budget 规则。
- core 对 stage contract violation 的判定。

Core 可以按中立结构执行 profile 声明的 evidenceRequirements，但只能做机械校验：

- 读取 `profileContracts.<profile>.evidenceRequirements[]`。
- 按 `scope` 选择 `acceptanceDimensions`、`deliverableCoverage`、`goalCoverage` 或 `dodAssessment.items`。
- 当对应 item 为 `satisfied` 时，检查其 `evidenceRefs` 指向的 step kind 是否命中 `acceptedStepKinds` 或 `rejectedStepKinds`。
- 当 `acceptedStepKinds` 已声明时，`rejectedStepKinds` 表示“不能单独支撑该声明”的证据类型；若同一 item 同时引用 accepted 证据，只读/辅助证据可以保留为上下文。
- 失败时生成中立 `dod_evidence_quality_gap` diagnostic，驱动后续 repair / rejected assessment。

Core 不得根据 `id` 名称推断业务含义。例如 `persistence_or_side_effect`、`concurrency` 只是 profile 声明的字符串；core 不知道它们对应软件开发、数据写入、测试或任何具体领域。

Run activation 必须把当前有效的 profile/template contract 组装进 `executionContract`。若 workspace 是由某个 template 初始化，且其 default soul / stage soul bindings 仍与该 template 匹配，activation 可以使用当前 template 的 `ContextHints` 作为 contract 基线，再保留 workspace 中 template 未声明的扩展字段；template 已声明的同名 contract 字段以当前 template 为准，避免旧 workspace 长期携带过期的 `profileContracts`、`deliverableExpectations` 或 `evidenceRequirements`。该刷新只作用于中立 execution contract，不得在 activation 阶段重新写入或覆盖 workspace 的 soul bindings、skill、mcp 或其他业务绑定。

`executionContextPackage` 交给 LLM 的 `executionContract` 必须保留完整结构，尤其是 profile 下的 `acceptanceDimensions`、`deliverableExpectations`、`evidenceRequirements` 及其 `required`、`acceptedStepKinds`、`rejectedStepKinds` 等字段。Goal / DoD / 历史上下文可以按预算压缩，但 contract 不能被压缩成数组数量或摘要，否则 LLM 无法按 profile 约束做决策，runtime 也会在后续 verification / repair 阶段反复纠偏。

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
| `planning` | 探索摘要、约束、DoD、当前 workspace manifest / known resources、已有方案、不可行路径 |
| `execution` | 当前 unit、必要上下文、accepted plan、可用 state-changing capability |
| `verification` | 变更摘要、验证要求、evidence 标准、历史失败原因 |
| `acceptance` | Goal / DoD、evidence refs、coverage map、remaining gaps |
| `repair` | failure scope、失败 evidence、最近修复尝试、必须继承的 contract |
| `replan` | 原计划失败原因、当前 workspace manifest / known resources、可保留产物、需要重新拆解的目标 |

## Repair 合同

Repair 不是重新探索。Repair 必须继承来源阶段的约束，并根据失败类型收紧。

| 来源阶段 | repair 要求 |
| --- | --- |
| `exploration` | 只修复探索输出无效、上下文缺失或工具失败；不得直接进入 implementation 特判 |
| `planning` | 修复 plan 缺口，必要时要求明确 executable unit |
| `execution` | 必须回到 state-changing output、failed assessment 或 blocker |
| `verification` | 必须补 evidence、解释 weak evidence 或转 rejected |
| `acceptance` | 必须补 DoD coverage / result，不得新增实现 |

Runtime repair 预算必须来自 runtime 配置，默认 `maxRuntimeRepairAttempts=10`；不得在 core runtime 分支中写死固定次数。repair 输入、repair exhausted ticket、auto-resume / auto-replan 判断、duplicate failure 判断和 `repairActionPackage.maxRuntimeRepairAttempts` 必须使用同一份归一化预算。Provider retry 预算与 runtime repair 预算互相独立，provider retry 不得消费 runtime repair 次数。

State-changing repair 必须满足目标相关性约束。若 repair 输入已经包含 `failureTargetScope`、`failureTargetRepairContext`、`patchRepairContext`、`repairActionPackage.targetSnapshots` 或等价的显式 `targetRef`，则 `patch/apply` 只有在目标属于这些失败目标、当前目标快照或明确 repair 目标时，才算有效 state-changing artifact。写入无关 marker、空转文件、报告文件或与失败目标无关的 no-op patch，不得满足 state-changing repair 合同；runtime 必须将其作为 contract violation 重新进入 repair / blocker / rejected 治理。

该约束属于 core runtime 的中立合同，只比较结构化目标引用、target snapshot 和 capability input，不解释文件类型、测试框架、编程语言或业务正确性。Core 可以判断“patch target 与失败目标不相关”，但不能判断“某个业务文件应如何修改”。当没有任何显式目标引用时，runtime 不得臆造目标；此时只能依赖 capability contract、LLM 输出和后续 evidence 判断。

若失败证据本身明确指向依赖、manifest、metadata、checksum、lock 等配套资源缺失，state-changing repair 可以写入与失败目标同目录的中立配套元数据目标。该放宽只基于失败文本中的通用前置资源信号、目标路径关系和目标文件名的通用 metadata/manifest/lock/checksum 形态，不得硬编码语言、框架、包管理器或具体业务文件。若配套目标不在同一目录、失败证据没有前置资源信号，或目标只是无关 marker/report/no-op 文件，仍必须按无关 patch 拒绝。

Shell / tool 调用不得被用来隐藏实际目标变更，再通过一个无关 `patch/apply` marker 来满足 state-changing 合同。若某个 capability 本身是声明过的 state-changing artifact，必须由该 capability 的输入、输出和 evidence 直接表达目标关系；否则修复应使用目标相关的 `patch/apply`、返回 failed assessment，或进入 blocker。

当 run 级 `stateChangePolicy=must_produce_state_change` 且失败输出包含 `missingDoDItems` 时，runtime repair 必须先区分中立缺口类型：

- `implementation_gap`：已有 evidence 表明实现缺失、实现错误、artifact 无法应用或非只读执行失败，repair 必须显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。后续 provider retry 也必须继承该约束，不能把实现缺口修复降级为只读 exploration graph。
- `evidence_gap`：verification 阶段只证明“缺少 concrete evidence / 未执行验证 / evidence 不足”，但尚未形成实现错误诊断时，repair 应携带 `repairObjective=produce_concrete_validation_evidence`，允许受限 tool / shell / mcp 验证图生成证据；若该验证图后续产生确定性失败，再由新的失败证据触发 state-changing repair。

当 state-changing repair 输入已经携带 `missingDoDItems` 或 `missingGoalCoverage` 时，返回的 `patch/apply` 至少必须有一个节点声明中立 `coverageTargets`，并且目标必须匹配对应缺口的原始 DoD item 文本、`goalRequirementId` 或 Goal coverage item 文本。该字段只表达“这个 state-changing artifact 试图关闭哪个 coverage gap”，不表示 runtime 认可其业务正确性；最终是否满足仍由后续 validation / acceptance evidence 判断。若 patch 不能绑定任何具体缺口，应返回 failed `dodAssessment` 或 blocker，而不是提交泛化骨架、marker、报告或只具备形式变更的 patch。

当 patch / preview / apply 因 context mismatch、base mismatch 或 stale artifact 失败，或 `file/edit` 因 `oldText` 不存在、上下文冲突失败时，默认 repair 仍要求基于当前 target snapshot 重新生成可应用 state-changing artifact。runtime 必须把失败目标的当前快照放入 `patchRepairContext` / `repairActionPackage.targetSnapshots`，并明确失败 patch / edit 片段已经陈旧，不能重复使用同一 hunk、oldText 或未解析模板。完整 `file_write` 只允许建立在完整、未脱敏、未截断且标记为 `safeForFileWrite` 的当前快照上，且 patch 请求必须携带与当前快照 `contentHash` 完全一致的字面 `baseHash`；否则必须使用 `unified_diff`、`file/edit` 或先重新读取目标。若 repair context 中的内容包含 runtime redaction marker、被省略片段或截断标记，LLM 不得把该内容复制进 patch content / diff，而应先读取目标、使用 exact oldText 的最小替换，或基于可见上下文生成最小 `unified_diff`。`baseHash` 只能省略或使用当前 evidence 中的字面 `contentHash` 值；不得输出 `${...}`、`{{...}}`、`<<...>>` 或其他未解析占位符。`unified_diff` 的 hunk header 必须是可解析的真实行号范围，`@@ ... @@` 等伪 hunk 属于无效 artifact。

当前 repair target snapshot 是定向修复的高优先级输入。Provider context budget、prompt normalization 和 outbound redaction 不得把满足以下条件的 `failureTargetRepairContext[].content`、`patchRepairContext[].content` 或 `repairActionPackage.targetSnapshots[].content` 压缩、摘要化或替换为 `[REDACTED]`：`contentComplete=true`、`safeForFileWrite=true`、`redacted=false`、`truncated=false`、`isBinary=false` 或未声明、且存在字面 `contentHash`。该豁免只适用于当前 repair target snapshot 的 `content` 字段，且只表达“当前资源快照可作为修复基线”的中立事实；其他敏感字段、历史输出、失败正文、stream partial、provider 配置和不满足安全标记的 snapshot 仍必须按通用 redaction / budget 规则处理。若在极端预算下连安全当前快照也无法完整提供，runtime 必须显式标记快照不可用于完整 `file_write`，要求 LLM 重新读取目标、使用最小 diff / exact edit，或返回 blocker / failed assessment；不得向 LLM 发送已被 `[REDACTED]` 替换的“伪完整快照”。

当失败 evidence 已经包含中立的编译/类型/API 形状诊断时，repair 需要遵守“首要阻塞优先”合同。这里的中立诊断包括但不限于：line-ref compile/build/test 错误、`undefined` / unresolved identifier、`unknown field`、`cannot use` / type mismatch、missing method、constructor/schema/interface mismatch，以及其他明确指向当前目标资源无法通过编译或基本验证的结构性错误。runtime core 只能表达以下中立约束，不得写入具体语言框架、业务模型或供应商细节：
- repairGuidance / provider prompt 必须要求 LLM 先修复这些已报告的首要阻塞，再处理断言增强、格式整理、文案修饰或其他次要改进。
- 若 `failureTargetRepairContext`、`patchRepairContext` 或 `repairActionPackage.targetSnapshots` 已提供当前目标快照，LLM 必须优先把目标与当前可见资源/API 形状对齐，而不是凭空发明新的名字、字段、入口或脚手架。
- 若具体任务属于软件开发等领域，workspaceTemplate / soul 可以在领域层进一步细化“对齐现有类型、构造函数、配置结构、入口函数和测试入口”的要求；这些领域规则不得下沉到 core runtime 里。
- 该规则只定义 repair 的排序与 grounding，不判断某个业务符号该叫什么、某个框架入口该怎么写，也不允许 core runtime 根据命令名、语言名或业务关键字生成特定修复方案。

失败产物上下文必须遵守“不可复用失败正文”约束。对于被拒绝的 LLM decision、schema-invalid 输出、失败 patch / edit 正文、stream partial output 或任何包含 runtime redaction marker 的片段，runtime 不应把原文再次放入 repair input；只能暴露中立诊断摘要，例如失败 stage、failureClass、error/reason、streamDiagnostics、llmUsage、rejectedExecutionGraph 摘要，以及当前 target snapshot / workspace manifest / patchRepairSuggestions。若 stream partial output 或 rejected graph 摘要中包含 redaction marker，runtime 必须记录 omitted 标记与不可复用原因，而不是保留可复制片段。该约束用于防止 LLM 复制 malformed JSON、过期 diff、脱敏片段或重复 rejected artifact；它不判断具体业务内容，只限制 runtime repair 的输入形态。

Runtime repair grounding 的详细合同见 `workorch-runtime-neutral-repair-grounding-design.md`。其中 `structuredFailureSignals`、`repairActionPackage`、失败产物隔离、identifier grounding 与重复失败收敛都属于核心中立运行态约束；任何语言、测试框架、协议、供应商或业务语义解释必须留在 workspaceTemplate / profile / soul / skill 中。

Runtime 可以从失败 diff 与当前 target snapshot 中抽取中立的 `patchRepairSuggestions`，例如“某个 oldText 在当前目标中唯一匹配，可替换为 newText”。该建议只来源于失败 artifact 与当前资源内容的机械对齐，不包含业务语义；它用于帮助 LLM 生成更小、更精确、可审计的 repair artifact，避免重复输出行号漂移、上下文过期或依赖脱敏快照的 patch。

如果 repair 基于当前 `patchRepairContext` 判断目标状态已经满足、失败 patch 已不再需要，runtime 可以接受一个受限 concrete validation evidence graph，用于验证当前状态；该 graph 不得只包含 read-only inspection，也不得包含新的 LLM 嵌套或 patch 节点。若验证失败，新的验证失败 evidence 再驱动后续 state-changing repair。

若 `dodAssessment.satisfied=true` 但 runtime 校验发现 DoD / Goal / acceptance dimension / deliverable coverage 字段缺失或不完整，该失败属于 assessment contract correction，而不是新的 state-changing implementation 缺口。runtime 必须以 `assessmentOnly` / `verification` 模式要求 LLM 补齐 `dodAssessment.items`、`dodAssessment.goalCoverage`、`dodAssessment.acceptanceDimensions`、`dodAssessment.deliverableCoverage` 与 evidenceRefs / artifactRefs；不得在没有新的实现缺口证据时强制重新生成 patch、shell 或其他可执行工作。

如果 assessment-only coverage correction 仍返回 `dodAssessment.satisfied=true` 但继续缺失 DoD / Goal / acceptance dimension / deliverable coverage，runtime 不得立即把 run 终结为 rejected result；应在 runtime repair budget 内继续派发 assessment-only correction，并把最新 missingDoDItems / missingGoalCoverage / missingDeliverableCoverage 作为强约束输入。只有 coverage correction 预算耗尽、LLM 返回明确 unsatisfied assessment、或出现不可恢复 contract violation 时，才进入 rejected / decision / fail 路径。

Completion / assessment 必须遵守“最新非 superseded 证据优先”规则。对于同一中立 scope（例如同一 graph node label、同一 targetRef、同一 failure target scope 或同一可追踪步骤标签），如果较早失败 evidence 之后出现了成功完成的验证、patch 或执行证据，较早失败只能作为 lineage / 历史诊断保留，不能继续主导 accepted / rejected 判断。runtime 在 completion gate 中应把可机械匹配的旧失败 evidenceRef 归一化为后续成功 evidenceRef；在 assessment-only coverage correction 输入中应携带 `latestClosureEvidence` 与 `supersededFailureEvidence`，其中 `latestClosureEvidence` 至少应覆盖最新成功 step evidence、已应用 patch evidence 与已通过 validation evidence。LLM 必须对比最新成功证据后再判断；若最新成功证据仍不足，必须说明具体缺失的中立验收维度，而不是笼统复述旧失败。

Goal coverage 校验只应把可判定的叶子需求作为覆盖对象。Markdown 标题、分组项、以冒号结尾的父级目标项只用于组织上下文，不得作为必须单独满足的叶子需求。若 Goal 叶子需求与 DoD 项文字不完全一致，LLM 必须通过 `dodAssessment.goalCoverage` 显式建立 Goal requirement -> evidenceRefs 的映射；runtime 不应依赖领域语义硬编码进行推断。

Repair 输入必须尽量包含足以定向修复的当前证据：

- `failedStepOutput`：当前失败的精简输出，包括 failure class / signature、error、reason、status、关键诊断和必要的 stdout/stderr 摘要。
- `failureTargetRepairContext`：当失败证据指向具体 workspace 资源时，提供当前目标资源快照、存在性、content hash 和来源说明。
- `workspaceFileManifest`：当失败目标需要与其他已知资源核对、DoD 缺口涉及资源证据、或 repair 需要避免误替换无关资源时，提供当前 workspace 资源清单。
- `patchRepairContext` 或等价 capability repair context：当失败来自 state-changing artifact 应用失败时，提供当前目标快照，并要求 LLM 基于当前快照重新生成可应用 artifact，而不是重复失败 artifact。
- `patchRepairSuggestions` 或等价 capability repair suggestion：当 runtime 能从失败 artifact 与当前目标快照中机械抽取唯一 oldText / newText 替换时，提供可直接复用的最小修复证据；该 suggestion 不代表业务判断，只代表当前资源内容与失败 artifact 的中立匹配事实。
- 近期 repair attempt 摘要：用于避免重复同一失败输出。

如果 repair-of-repair 是因为 LLM 输出违反 contract，例如缺少必需 artifact 字段、返回只读 graph、返回能力不支持的操作，后续 repair 必须继承上一轮可用的 target context、workspace manifest、state-changing contract 和 rejection evidence。不得因为 contract violation 发生在 LLM repair step 外壳上，就丢失原始 failure target 或降级为泛化探索。

当 verification / strict convergence follow-up 已经识别出需要继续收敛时，可以返回受限 concrete validation evidence graph，例如明确的 shell/tool/mcp 验证节点；该 graph 不需要包含 patch，但不得只包含 read-only inspection、自检、run snapshot、context rehydration 或新的 LLM 嵌套。若再次返回只读 inspection graph 且没有 state-changing output、concrete validation evidence 或 rejected assessment，该失败属于 contract violation。后续 repair 必须携带对应的 rejection evidence；只有 failure evidence 已表明缺口是实现/状态变更缺口时，才提升为 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`，不能把纯验证证据缺口误升级为写文件 repair。

Runtime 可以把失败证据中的目标引用解析为当前 workspace 中的已知资源快照，但只能使用中立路径匹配和 manifest 事实；不能把某个 profile 的文件命名、测试约定或技术栈规则写入 core repair。

Repair exhausted 后必须二选一：

- 写 rejected result 并进入 failed / blocked_on_decision。
- 创建 replan successor run，并保留 predecessor lineage。

为支持 blocked_on_decision 后的继续处理，因 repair exhausted、duplicate failure 或其他可恢复治理点写入的 rejected result 只能代表当时失败窗口的临时结论。若后续已经出现针对该 rejected result source step 的 provider retry、runtime repair 或 decision-authorized retry 进展，该 rejected result 必须转为 `superseded`，不得继续阻断当前 run 的恢复、repair 或最终 acceptance。

## Runtime Capability Time Budget

非 LLM runtime capability 的默认执行预算必须来自中立配置或 capability 自身声明，不得在 core runtime 中按业务领域、语言、框架或命令名称写死。`shell/command` 这类通用执行能力应提供可配置默认超时，并允许单个 step 通过结构化字段声明更长但有上限的预算。

默认预算用于避免普通长流程任务被过短固定值误杀；单步覆盖用于让 LLM 在严格收敛阶段为明确长耗时操作申请有限执行窗口。无论默认值还是单步覆盖，都必须是有界的，并在 step output 中记录实际 `timeoutMillis`，作为后续 repair / verification 的中立证据。

当非 LLM 执行因为 timeout 失败时，runtime repair 只能基于“执行超时”这一中立失败形态选择重试、缩小动作、调整有界预算、返回 failed assessment 或 blocker，不得依赖具体命令名推断业务策略。

## Provider Retry 合同

Provider retry 始终只处理 provider transient failure，与 stage 语义无关。
Provider retry 预算必须来自 runtime 配置，默认 `maxProviderRetryAttempts=10`；不得在 core runtime 分支中写死固定业务场景次数。

Provider retry / config blocker 的边界必须基于中立失败形态：

- `timeout`、`rate limit`、`context too large`、`provider stream incomplete`、已完成 stream 但内容为空的 `provider stream error`、未带明确配置错误标记的 `provider unavailable / request failed / provider error` 等瞬时 provider 执行失败，应先消耗 provider retry 预算。
- `auth or config`、`schema drift`、明确不支持的 provider operation、明确 client/config unavailable、4xx、缺失 provider / model / credential 等配置型失败，可以直接进入 provider/config blocker。
- provider/config blocker 的 `attempts` 必须记录真实已失败 provider retry 次数，`maxAttempts` 才记录预算上限；不得在未实际重试时把 `attempts` 写成预算上限。
- provider retry 不得消费或重置业务 repair/replan 预算；它只表示 runtime LLM provider 调用基础设施暂时未产生可用 LLM 输出。

### LLM 运行期动态绑定

LLM provider、model、base URL、context profile、failover 目标等信息属于运行期动态绑定，不属于 workspace/run/step/ticket 的可恢复业务状态。runtime 持久化记录可以保留中立观测指标，例如 token 数、请求/响应字节、耗时、failure class、stream diagnostics，但不得把 provider/model/base URL 作为后续恢复、retry、repair、memory 或 decision 的依据。

因此：

- run activation、CLI、HTTP、backend/eventhub 等入口不得把 `llmProviderId` 写入 run execution context。
- step input、provider retry input、decision ticket context 和 LLM context compaction 必须剥离 provider/model/base URL 等绑定字段。
- provider/config blocker 被解决并收到“继续处理 / retry same step”后，runtime 必须重新按当前系统配置动态解析 provider；不得因为历史 step 记录中的旧 provider 继续原地 reblock。
- provider unavailable / auth or config / schema drift 这类配置型失败可以先进入 config blocker；但 blocker 解决后的 retry 是一次新的运行期绑定，不继承旧 provider 选择。
- 显式 provider 选择如后续重新支持，必须设计为短生命周期调度参数，而不是持久化恢复状态。

允许：

- timeout。
- rate limit。
- context too large。
- provider stream incomplete。
- provider stream error with empty content。

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
- 收到 done 但 `receivedBytes=0` / `receivedDeltaCount=0` 时，不得当成有效 LLM 输出；应归类为 provider stream error 并进入 provider retry 预算，预算耗尽后再进入 provider/config blocker。
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
- exploration / planning / replan continuation 的 LLM input 必须包含当前 workspace 的有界 manifest / known resources。该清单只表达“当前已知资源事实”，不得编码语言、框架、文件类型或业务领域规则；其目的在于避免 LLM 在已有工作区中误生成初始化路径。
- repair bundle 必须包含 failure scope、来源阶段、失败 evidence、已尝试修复、必要目标快照、workspace manifest 和必须继承的 contract。
- replan bundle 必须包含 predecessor lineage、replan ticket / resolution、原失败 evidence、已失败 repair path、可保留产物线索和仍然有效的 Goal / DoD / execution contract。
- capability 摘要必须包含当前能力支持和不支持的中立操作，例如必需字段、禁止字段、是否支持删除、是否要求完整内容或完整 diff。
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
- replan successor 的初始输入必须能看到触发 replan 的 failure context，而不是只看到 ticket / resolution id。
- repair-of-repair 必须保留原始 failure target context、workspace manifest 和 state-changing contract。
- 当前失败 step 的 output failure signature 必须优先于继承的历史 repair signature，避免新失败被计入旧预算。
- capability contract violation 必须被拒绝并形成 repair evidence；LLM prompt 必须包含该 capability 的中立限制。
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
  - `validateLLMExecutionOutputContract` 和 follow-up graph state-changing contract 已开始优先基于 `runtimeStage` 判断，不再依赖 Soul 绑定阶段。
  - 2026-05-17 已完成破坏性收口：runtime contract 不再从 `soulStage` 反推 `runtimeStage`；Soul 绑定阶段只用于选择 Soul / capability allowlist，不再作为 runtime stage fallback。
- 2026-05-13：
  - 已完成 STG-03 第一批收口。
  - core runtime 新增 `runtimeStageMode`，当前支持 `open_judgment`、`strict_convergence`、`structured_governance`。
  - `runtimeStageMode` 已随 `runtimeStage` 一起进入 step input、execution context package、memory trace 和 LLM provider input。
  - blocks/llm prompt 已显式接入 `runtimeStage` / `runtimeStageMode`，让 LLM 在 prompt 层知道当前是开放判断还是严格收敛。
  - 当时仍未完成 profile 层的独立阶段映射与专用 verifier；后续 STG-04 / STG-07 落地记录见 2026-05-17 条目。
- 2026-05-13：
  - 已完成 STG-02 / STG-06 / STG-08 追加收口。
  - core runtime 已移除通过 `graphNodeId` 文本关键词推断 state-changing graph node 的逻辑；graph node 是否必须产生 state-changing output 只读取显式 `stateChangingOutputRequired` contract。
  - execution graph node 的阶段判断已从 `soulStage=execution` 切换到中立 `runtimeStage=execution`。
  - planning / exploration 保持 `open_judgment` 语义，即使 run 级 `executionContract.stateChangePolicy=must_produce_state_change`，也不在 planning 阶段强制 patch；state-changing 要求在 execution / repair 等严格收敛阶段执行。
  - repair 新写入的 `repairObjective` 已改为 `produce_state_changing_output`；`produce_state_changing_implementation` 已从 runtime state-change 判定中移除。
  - state-changing repair guidance 已改为中立的 state-changing output 要求，profile 层仍可把该 artifact 映射为 patch/file write 等具体能力。
- 2026-05-13：
  - 已完成 execution 失败 repair 追加收口。
  - `must_produce_state_change` run 中，`execution/strict_convergence` 阶段的非只读步骤失败会触发 state-changing repair contract，避免测试/构建/执行失败后的 repair 退回只读分析。
  - runtime repair ID 对普通 graph-scoped 失败使用失败 graph node 标签，例如 `repair-001-of-graph-003-003-run-go-test`；provider retry / runtime repair 等 recovery 外壳仍按归一 root 生成，避免再次出现 provider retry 壳嵌套。
- 2026-05-14：
  - 已完成 repair failure scope 与上下文继承追加收口。
  - 非 repair-follow-up 当前失败优先使用自身 output failure signature；历史 `repairOfFailureSignature` 不再覆盖新的无 target scope 失败，避免新执行超时或验证失败串入旧 repair budget。
  - failure target repair 已补充当前目标快照、唯一后缀路径解析、workspace manifest 与 repair-of-repair 上下文继承；重复 contract violation 不再丢失原始 failure target。
  - state-changing repair guidance 与 LLM provider prompt 已补充 runtime capability 边界：能力不支持的操作不得用伪语义表达，例如不得用空 `file_write` 表示删除。
  - replan successor 的 `replanContinuation` 已要求携带 ticket failure context，使 successor 能基于 predecessor failure evidence 继续收敛，而不是只收到 replan 元信息。
  - planning / exploration / replan continuation 的 execution context package 已要求携带当前 workspace 的有界 manifest；state-changing repair follow-up 也必须在未显式继承 manifest 时从当前 workspace 补齐，避免已有资源不可见导致重复初始化或错误覆盖。
  - patch repair context 已补充 `contentComplete` / `safeForFileWrite` 标记与 redaction/truncation 禁用规则；LLM provider prompt 不再无条件鼓励基于当前快照输出完整 `file_write`。
  - patch repair 已增加中立 `patchRepairSuggestions`：从失败 diff 与当前目标快照机械抽取唯一 oldText/newText 替换，帮助生成最小 diff，避免反复输出 stale diff 或复制脱敏内容。
  - `unified_diff` 应用能力已支持在行号漂移但 old/context lines 于当前目标中唯一匹配时按唯一匹配位置应用，降低 LLM 生成 diff 行号不准导致的无效 repair 循环。
- 2026-05-15：
  - 已完成 execution contract 下发与 template contract 刷新收口。
  - patch 输入校验已覆盖 `baseHash` 未解析占位符与 `@@ ... @@` 伪 hunk，LLM provider prompt / repair guidance 已要求使用字面 `contentHash` 或省略 `baseHash`，并在修复前基于当前 target snapshot 避免重复添加等价内容。
  - activation 会基于仍匹配 workspace soul routing 的当前 template `ContextHints` 刷新 run 级 `executionContract`，并保留 workspace 扩展字段；该逻辑只更新中立 contract，不重新绑定 workspace template、soul、skill、mcp。
  - `executionContextPackage` 不再压缩 `executionContract`，确保 LLM 能看到完整 `profileContracts`、required deliverables 与 evidence requirements。
  - verification follow-up 返回只读 execution graph 且缺少有效收敛输出时，后续 repair 会继承 contract violation evidence；需要状态变更时会显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。
  - assessment-only coverage correction 若仍缺 DoD / Goal coverage，不再首轮 repair 后直接终止；runtime 会在 repair budget 内继续派发 assessment-only correction，并携带最新缺失 coverage 项。
  - 外部 capability 失败的 target scope 已支持 `serverId+tool` 组合并使用 run 级目标预算；同一外部能力反复确定性失败时会进入 duplicate / repair exhausted 治理，rejected result 会携带 terminal failed step 与 repair step 证据，run failure reason 优先使用 unsatisfied DoD assessment summary。
- 2026-05-17：
  - 已完成运行态上下文放大收口。
  - verification follow-up 已允许受限 concrete validation evidence graph；只有纯 read-only inspection / 自检 / context rehydration 且无 evidence、无 rejected assessment 时才视为 contract violation，避免把验证阶段测试/检查图误升级为必须 patch 的 repair。
  - state-changing repair target relevance 已补充中立配套元数据目标规则：当失败证据明确指向 dependency / manifest / metadata / checksum / lock 前置资源缺失时，可修复同目录配套目标，但不得引入语言、框架或业务文件硬编码。
  - repair scope 已要求当前 repair-follow-up 自身产生的 runtime contract violation 优先使用当前 `failureClass/failureSignature`，provider retry 等无新签名的运行外壳才继续继承原始 scope。
  - patch repair 中已有目标 `file_write` 已改为安全条件放行：必须命中当前 repair target snapshot，且 snapshot 完整、未脱敏、可 file write，并且请求 `baseHash` 与 `contentHash` 完全一致；否则继续拒绝并要求使用最小 diff/edit 或重新读取目标。
  - rejected / partial LLM 输出中的 redacted marker 不再通过 `partialOutputPreview` 回流到 repair input，只保留 omitted 标记和不可复用原因。
  - `governanceHistory` 与 `replanContinuation` 的失败 lineage 改为 summary/ref-only，禁止把旧的 raw failure input/output、target content、LLM raw preview、stream events 或 provider/model 动态绑定复制进 successor run、step input 或 execution context package。
  - assessment-only contract correction 输入已进一步隔离旧治理/重规划/决策/执行修复上下文，只保留评估补正必要字段和已清洗的 invalid assessment 摘要。
  - workspace 终态 read model 已要求 `taskProgress` 与 `outcome` 使用同一最新终态 result / run 选择规则，避免 outcome 指向当前 rejected run 但 taskProgress 回退到历史 run。
  - completion gate evidence ref 归一已补充 graph follow-up / assessment follow-up 派生别名映射：`run/<follow-up-step>-<node-id>`、`verify_full_test` / `verify-full-test`、`003-verify-full-test` 等中立结构别名可映射到同 run 内真实 graph step；不可追踪或失败证据继续剔除。
  - accepted result / result-summary / deliverable-manifest 已增加 runtime 事实校正：同一 run 存在已应用 workspace 变更时，不再保留“仅 assessment-only / 无新增可执行操作”等与事实矛盾的 LLM 自述。
  - Step state event payload 改为 raw ref + compact summary，不再复制完整 step input/output；ToolCall runtime evidence 改为记录 step input/output refs 与摘要，不再把完整 step input 重复写入 `tool-calls/*/arguments.json`。
  - 该变更用于避免长流程 workspace 在 repair/replan 中反复 JSON clone/marshal/read/write，导致 CPU、内存和 payload 文件成倍膨胀。
- 2026-05-17：
  - 已完成 DoD / Goal coverage 缺口提示中立化收口。
  - core runtime 不再根据 DoD 文本中的测试、配置、并发、语言栈或其他业务关键词生成领域化 evidence hint。
  - `missingDoDItems` / Goal coverage diagnostics 只提示按当前 `runtimeStage` 与 profile contract 提供具体 evidence refs；软件开发所需的 build/test/diff/config/concurrency 等证据要求继续由 `software-dev` workspaceTemplate / Soul / profile contract 承载。
  - Result manifest 与 patch evidence artifact 命名同步从 `Code Change` / `code-change-diff` 收敛为中立 `Workspace Change` / `workspace-change-diff`，避免 core 把通用 workspace 状态变更解释为代码变更。
  - workspace 交付文件在 core manifest 中统一标记为 `deliverable`，不再按文件名或扩展名推断 source / configuration / test_report；若某个 profile 需要更细的产物分类，必须由 profile verifier 或模板输出结构化声明。
  - blocks/llm 全局 provider prompt 已移除软件开发默认路径、测试命令示例、项目初始化和领域化 DoD repair 话术；全局 prompt 只表达 stage / capability / workspace target / evidence 的中立约束，软件开发流程继续由 `software-dev` Soul 和 profile contract 下发。
  - 已新增 `scripts/verify-workorch-runtime-profile-software-dev.sh` 与 `make verify-runtime-profile-software-dev`，把 software-dev 的 `goalCoverage`、`acceptanceDimensions`、`deliverableCoverage` 等 profile-specific 验证从 core verifier 拆出；`verify-workorch-runtime-core-flow.sh` 继续只检查中立 runtime flow。
  - `scripts/verify-boundary-contract.sh` 已补充软件开发偏置守卫，禁止 reusable runtime / 全局 LLM prompt 重新出现 `Code Change`、固定测试命令、`implementation path`、`affected files`、profile deliverable id 等专属表达。
  - 已完成不兼容收口：`runtimeStageFromStepInput` 不再从 `soulStage` 反推 runtime stage；blocks/llm provider Soul 推断也不再把旧 `soulStage` 当作 fallback。后续所有 runtime 合同判断必须依赖显式 `runtimeStage` 或结构化 follow-up flags。
  - run-summary memory 上下文压缩已移除旧 `run_summary` 兼容 trust marker；core 只保留中立 provenance 与当前版本标签，不再为历史数据生成特殊降权字段。
  - run-summary memory 的 core 文本已把 `Verification commands` / `Patch targets` / `code_agent` 收敛为中立 `Evidence executions` / `State-change targets` / `runtime_result`，软件开发具体命令和 patch 语义只作为 profile evidence 内容出现，不作为 core 标签或固定标题。
  - continuable decision -> runtime repair 的判断已移除 `new project`、`project path`、`工作目录` 等文本关键词启发式；core 只接受中立结构化标记 `runtimeCanContinue` / `continuableByRuntime` / `workspaceActionable` 或已知可读 workspace resource 证据。

## 非兼容性说明

当前阶段以彻底收口 WorkOrch 能力为目标，不考虑旧行为兼容。允许做以下破坏性调整：

- 替换旧的 `soulStage` 推断方式。
- 要求所有新 step 显式携带 `runtimeStage`。
- 重构 output contract validator。
- 将软件开发相关 prompt / tool 选择迁移到 profile 层。
- 清理旧 memory recall 中的低价值 runtime 流水。
- 移除旧 memory trust marker、旧 run-summary 降权分支和历史数据特殊解释。
- 移除基于自然语言关键词的历史 decision 兼容推断；需要继续执行时必须通过结构化 runtime context 表达。
- 替换旧的 runtime file-backend 文件命名与 raw payload ref 形态。

但即使不考虑兼容，也不得破坏核心中立原则。

## Runtime Storage Naming Contract

运行态存储命名属于 core runtime 可观测性合同。WorkOrch 的长流程执行需要人、LLM、read-model 和恢复逻辑都能从目录结构中快速定位 workspace / run / stage / loop / step / artifact，因此 file backend 不得把逻辑 ID 的完整字符串默认编码成不可读文件名。

### 主命名规则

- runtime 逻辑 ID 必须优先保持结构化、可读、稳定，例如 `run-xxx/graph-002/007-run-go-test`。
- file backend 必须把逻辑 ID 按 `/` 展开为可读层级路径，而不是把整个 ID 做 base64、hash 或随机 token。
- 每个路径 segment 只允许保留文件系统安全字符，例如 `[A-Za-z0-9._-]`；其他字符应做中立清洗，例如替换为 `-`。
- 空 segment、`.`、`..`、路径穿越和绝对路径必须被拒绝或清洗为安全 segment。
- 结构化 lineage 仍然是因果关系和恢复预算的权威来源；文件路径只用于定位、审计和排查，不得反向推断业务语义。

推荐落盘形态：

```text
runs/<run-id>/
  steps/<run-id>/graph-002/007-run-go-test/metadata.json
  tool-calls/<run-id>/graph-002/007-run-go-test/runtime-execute/output.json
  llm-calls/<run-id>/repair-001/request.json
  checkpoints/<run-id>/checkpoint/0007.json
  results/<run-id>/result/dod-assessment.json
```

### 禁止项

以下形态不得作为默认 runtime 文件名或 raw payload ref：

```text
steps/cnVuLTMyNDk1MjE0OTUyNGQ2NDgvZ3JhcGgtMDAy/metadata.json
checkpoints/cnVuLTMyNDk1MjE0OTUyNGQ2NDgvY2hlY2twb2ludC8x.json
llm-calls/3f8a9c1b/request.json
```

base64/hash 只允许作为极端 fallback，例如单个 segment 超过文件系统限制、清洗后发生冲突且无法通过结构化顺序消解。即便 fallback，也必须带可读前缀和短 hash 后缀，例如：

```text
graph-002-007-run-go-test--h-abc123
```

不得使用纯 base64 或纯 hash 作为主路径。

### Raw Payload Ref

`workorch://` raw payload ref 必须使用同一可读层级规则：

```text
workorch://workspaces/<workspace-id>/runs/<run-id>/llm-calls/<run-id>/repair-001/response.json
workorch://workspaces/<workspace-id>/runs/<run-id>/steps/<run-id>/graph-002/007-run-go-test/output.json
```

raw ref 是稳定的逻辑定位，不是本地绝对路径。API 实现必须继续校验 workspace、run、collection、item path 和文件名白名单，防止路径穿越。

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
11. runtime 文件名和 raw payload ref 是否保持可读层级路径，且没有默认 base64/hash 主命名。
