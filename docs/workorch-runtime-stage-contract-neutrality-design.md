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

当一个非 repair-follow-up 的当前失败 step 同时携带历史 `repairOfFailureSignature` 和自身 `failureSignature` 时，scope 判定必须优先使用当前失败 step 输出中的 `failureSignature` / `failureClass`。历史 repair lineage 只能作为上下文与来源说明，不得覆盖当前失败证据。只有真正的 repair-follow-up step 才可以继续按其 `repairOfFailureSignature` 归入原始 repair scope。该规则适用于所有中立失败类型，尤其用于避免“上一轮失败签名”串线到新的执行超时、验证失败或能力 contract violation。

若当前失败签名包含短目标引用，runtime 可以用当前 workspace manifest 做唯一后缀解析，以便把失败证据定位到实际资源；若存在多个候选，必须保留原始短引用并标记为未确定目标，不得猜测具体资源。该解析只基于 workspace 中已知资源路径这一中立事实，不得依赖文件类型、语言、测试框架或业务约定。

### Replan Successor Lineage

当 repair exhausted、duplicate failure 或人工/自动治理决策要求 `replan` 时，successor run 必须继承 predecessor 的 Goal / DoD / execution contract，并在 `replanContinuation` 中记录中立 lineage：

- predecessor run ID。
- 触发 replan 的 ticket / resolution 引用。
- ticket reason 与 resolution payload。
- 触发 replan 的 failure context，包括 failure class、failure signature、failure target scope、failed step id/kind、failed step output、最近 repair attempts 或可用摘要。

Successor 的首个 LLM step 不应只知道“需要重规划”。它必须获得足够的失败证据来判断哪些产物可以保留、哪些路径已失败、哪些 contract 仍然必须继承。若 successor 被设为 state-changing repair continuation，则其初始输入必须继续携带 `stateChangingOutputRequired`、`repairObjective` 与对应 guidance；同时不得把历史 provider/model 等运行期动态绑定写入 successor 的恢复上下文。

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
- 下层 `executionGraph` 继续依赖 `llm/workspace_execution` 决策节点套娃。
- placeholder artifact。
- weak evidence 直接 accepted。
- assessment-only 阶段输出可执行字段。

### Goal / DoD Coverage Assessment 合同

当 acceptance / verification 需要对 Goal 与 DoD 做完成度判断时，runtime 必须把覆盖判断建模为中立 coverage 合同：

- Goal 中被拆出的 leaf requirement 必须获得确定性、可复现的 `goalRequirementId`，ID 来源只能是 requirement 顺序与规范化文本，不得使用随机值或领域语义。
- DoD 与 Goal coverage 必须分别表达；DoD item 通过不代表 Goal leaf 自动覆盖，除非 `dodAssessment.goalCoverage.items[]` 明确给出对应 Goal requirement、状态和 evidence refs。
- `missingGoalCoverage` 中的每个条目都必须在 assessment-only correction 中成为独立的 `dodAssessment.goalCoverage.items[]` 条目；不得把多个缺口合并成一个宽泛描述后宣称满足。
- runtime 可以为 `missingGoalCoverage` 附加中立 `evidenceCandidates`，候选来源只能是当前 run 已完成或失败 step 的 step id、kind、state、output 文本与 requirement 文本的通用匹配。候选证据不是自动通过依据，LLM 必须判断其 output 是否真实支持该 Goal leaf，并在满足时引用具体 `evidenceRefs`。
- assessment-only correction 不得返回 executable work；只能返回 completed / failed `dodAssessment` 或真实治理 blocker。
- 若 assessment-only correction 给出 failed assessment，但同一缺口仍存在候选证据或满足项矛盾，runtime 应在 repair budget 内再次派发 assessment-only correction，而不是立即写 rejected result。预算耗尽后才进入 rejected / blocked / replan 治理。
- rejected result 的 `SourceRef` 应指向最终产生 rejection 的终态 assessment / repair step；root step、failed step、候选证据 step 应保留在 `EvidenceRefs` 中，确保终态来源与 lineage 都可追踪。

若 `execution/strict_convergence` 阶段的 LLM graph 节点返回的 `executionGraph` 已经收敛为具体 runtime capability，且图中至少包含一个有效 state-changing 节点、没有继续包含 `llm/workspace_execution` 决策节点、state-changing 节点不依赖新的 LLM 决策祖先，则 runtime 可以把该图作为新的 graph scope 派发执行。该判断只基于 capability kind、依赖关系和 capability input contract，不得引入业务领域语义。

当 run 级 `stateChangePolicy=must_produce_state_change` 时，`planning` 阶段可以先输出只读 graph 做上下文探索；但一旦 graph 中的 `llm/workspace_execution` 节点被调度到 `execution` 阶段，该 LLM 节点必须继承 `stateChangingOutputRequired=true`、`repairObjective=produce_state_changing_output` 与 state-changing guidance。普通 `file/read`、`file/list`、只读 tool 节点不因此被强制变更。

当 `execution/strict_convergence` 阶段的非只读步骤失败，且 run 级 contract 要求 `must_produce_state_change` 时，后续 runtime repair 必须继续携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。这包括 shell/test/build、patch/apply、file/edit、MCP tool 等执行或验证 state change 的步骤失败；普通 read observation 失败只有在缺失目标文件、DoD 缺口或其他明确 contract failure 时才提升为 state-changing repair。

若非 LLM 执行步骤失败原因是中立的瞬时执行超时（例如 runtime deadline / timeout，且尚未形成确定性失败证据），runtime repair 不应强制要求 state-changing 输出；应允许一次受预算约束的重试、缩小验证范围或返回 failed assessment / blocker。该规则只基于“执行超时”这一中立失败形态，不得写入特定命令、语言、测试框架或业务领域判断。

当 provider/config 决策解除后需要重试原失败 LLM step 时，decision resolution 只能补充“已授权重试、已提供信息、恢复动作”等恢复上下文，不得覆盖原 step 已存在的 `runtimeStage`、`runtimeStageMode`、`soulStage`、`stateChangingOutputRequired`、`repairObjective` 等阶段合同。换言之，`decision` 是恢复入口，不是被重试任务的执行阶段；provider retry 必须继续继承失败 step 的原阶段语义。

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

当 run 级 `stateChangePolicy=must_produce_state_change` 且失败输出包含 `missingDoDItems` 时，runtime repair 必须先区分中立缺口类型：

- `implementation_gap`：已有 evidence 表明实现缺失、实现错误、artifact 无法应用或非只读执行失败，repair 必须显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。后续 provider retry 也必须继承该约束，不能把实现缺口修复降级为只读 exploration graph。
- `evidence_gap`：verification 阶段只证明“缺少 concrete evidence / 未执行验证 / evidence 不足”，但尚未形成实现错误诊断时，repair 应携带 `repairObjective=produce_concrete_validation_evidence`，允许受限 tool / shell / mcp 验证图生成证据；若该验证图后续产生确定性失败，再由新的失败证据触发 state-changing repair。

当 patch / preview / apply 因 context mismatch、base mismatch 或 stale artifact 失败时，默认 repair 仍要求基于当前 target snapshot 重新生成可应用 state-changing artifact。但完整 `file_write` 只允许建立在完整、未脱敏、未截断且标记为 `safeForFileWrite` 的当前快照上；若 repair context 中的内容包含 `[REDACTED]` / `[SECRET]` / `[MASKED]`、省略片段或截断标记，LLM 不得把该内容复制进 patch content / diff，而应先读取目标、使用 exact oldText 的最小替换，或基于可见上下文生成最小 `unified_diff`。

Runtime 可以从失败 diff 与当前 target snapshot 中抽取中立的 `patchRepairSuggestions`，例如“某个 oldText 在当前目标中唯一匹配，可替换为 newText”。该建议只来源于失败 artifact 与当前资源内容的机械对齐，不包含业务语义；它用于帮助 LLM 生成更小、更精确、可审计的 repair artifact，避免重复输出行号漂移、上下文过期或依赖脱敏快照的 patch。

如果 repair 基于当前 `patchRepairContext` 判断目标状态已经满足、失败 patch 已不再需要，runtime 可以接受一个受限 concrete validation evidence graph，用于验证当前状态；该 graph 不得只包含 read-only inspection，也不得包含新的 LLM 嵌套或 patch 节点。若验证失败，新的验证失败 evidence 再驱动后续 state-changing repair。

若 `dodAssessment.satisfied=true` 但 runtime 校验发现 DoD / Goal coverage 字段缺失或不完整，该失败属于 assessment contract correction，而不是新的 state-changing implementation 缺口。runtime 必须以 `assessmentOnly` / `verification` 模式要求 LLM 补齐 `dodAssessment.items`、`dodAssessment.goalCoverage` 与 evidenceRefs；不得在没有新的实现缺口证据时强制重新生成 patch、shell 或其他可执行工作。

如果 assessment-only coverage correction 仍返回 `dodAssessment.satisfied=true` 但继续缺失 DoD / Goal coverage，runtime 不得立即把 run 终结为 rejected result；应在 runtime repair budget 内继续派发 assessment-only correction，并把最新 missingDoDItems / missingGoalCoverage 作为强约束输入。只有 coverage correction 预算耗尽、LLM 返回明确 unsatisfied assessment、或出现不可恢复 contract violation 时，才进入 rejected / decision / fail 路径。

Goal coverage 校验只应把可判定的叶子需求作为覆盖对象。Markdown 标题、分组项、以冒号结尾的父级目标项只用于组织上下文，不得作为必须单独满足的叶子需求。若 Goal 叶子需求与 DoD 项文字不完全一致，LLM 必须通过 `dodAssessment.goalCoverage` 显式建立 Goal requirement -> evidenceRefs 的映射；runtime 不应依赖领域语义硬编码进行推断。

Repair 输入必须尽量包含足以定向修复的当前证据：

- `failedStepOutput`：当前失败的精简输出，包括 failure class / signature、error、reason、status、关键诊断和必要的 stdout/stderr 摘要。
- `failureTargetRepairContext`：当失败证据指向具体 workspace 资源时，提供当前目标资源快照、存在性、content hash 和来源说明。
- `workspaceFileManifest`：当失败目标需要与其他已知资源核对、DoD 缺口涉及资源证据、或 repair 需要避免误替换无关资源时，提供当前 workspace 资源清单。
- `patchRepairContext` 或等价 capability repair context：当失败来自 state-changing artifact 应用失败时，提供当前目标快照，并要求 LLM 基于当前快照重新生成可应用 artifact，而不是重复失败 artifact。
- `patchRepairSuggestions` 或等价 capability repair suggestion：当 runtime 能从失败 artifact 与当前目标快照中机械抽取唯一 oldText / newText 替换时，提供可直接复用的最小修复证据；该 suggestion 不代表业务判断，只代表当前资源内容与失败 artifact 的中立匹配事实。
- 近期 repair attempt 摘要：用于避免重复同一失败输出。

如果 repair-of-repair 是因为 LLM 输出违反 contract，例如缺少必需 artifact 字段、返回只读 graph、返回能力不支持的操作，后续 repair 必须继承上一轮可用的 target context、workspace manifest、state-changing contract 和 rejection evidence。不得因为 contract violation 发生在 LLM repair step 外壳上，就丢失原始 failure target 或降级为泛化探索。

当 verification / strict convergence follow-up 已经识别出需要继续收敛，却再次返回只读 execution graph 且没有 state-changing output、concrete validation evidence 或 rejected assessment 时，该失败属于 contract violation。后续 repair 必须携带对应的 rejection evidence；若 failure evidence 表明缺口已经是实现/状态变更缺口，则必须提升为 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`，不能退回普通只读探索。

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
  - activation 会基于仍匹配 workspace soul routing 的当前 template `ContextHints` 刷新 run 级 `executionContract`，并保留 workspace 扩展字段；该逻辑只更新中立 contract，不重新绑定 workspace template、soul、skill、mcp。
  - `executionContextPackage` 不再压缩 `executionContract`，确保 LLM 能看到完整 `profileContracts`、required deliverables 与 evidence requirements。
  - verification follow-up 返回只读 execution graph 且缺少有效收敛输出时，后续 repair 会继承 contract violation evidence；需要状态变更时会显式携带 `stateChangingOutputRequired=true` 与 `repairObjective=produce_state_changing_output`。
  - assessment-only coverage correction 若仍缺 DoD / Goal coverage，不再首轮 repair 后直接终止；runtime 会在 repair budget 内继续派发 assessment-only correction，并携带最新缺失 coverage 项。
  - 外部 capability 失败的 target scope 已支持 `serverId+tool` 组合并使用 run 级目标预算；同一外部能力反复确定性失败时会进入 duplicate / repair exhausted 治理，rejected result 会携带 terminal failed step 与 repair step 证据，run failure reason 优先使用 unsatisfied DoD assessment summary。

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
