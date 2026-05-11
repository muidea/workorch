# LLM Decision Memory Improvement Plan

## 背景

`ws-759510dc7da435ca` 的最新运行暴露出 Memory 能力的两个层级问题：

- recall 链路层面：Run activation 会召回历史 Memory，但后续 LLM Step 主要复用 activation 时固定的 `ExecutionContext.memories`，运行中新写入的 Memory 没有进入后续 repair / retry / assessment 输入。
- 价值层面：当前 Memory 多为基础流程摘要，例如 DoD 未满足、assessment-only 合同问题、provider timeout、`[no test files]`。这些信息有运行诊断价值，但不足以让后续 LLM 做高质量工程决策。

本方案把 Memory 定义为 **LLM 可复用的任务状态压缩层**，而不是事件日志摘要。高价值 Memory 主要来自两类内容：

- 被 LLM 接纳并实际参与推理的输入。
- LLM 围绕 Goal / DoD 生成并被 runtime 接受的必要输出。

同时，Skill、Soul、Prompt、Tool、MCP 选择与约束也是 LLM 决策输入的一部分，必须作为 Memory 候选资产参与沉淀和召回。

## 设计目标

1. 让后续 LLM 能继承已经被接受的关键输入，而不是重复 scan/read 或重新解释 Goal / DoD。
2. 让 repair / retry / replan 能复用上一轮被接受的 gap analysis、repair plan、verification plan 和 tool strategy。
3. 让 Skill / Soul / Prompt / Tool / MCP 约束成为可追溯的决策上下文，避免后续步骤丢失能力边界。
4. 让 Memory recall trace 能说明本次 LLM Step 为什么选择这些 Memory、排除了哪些 Memory。
5. 保持 kernel 领域中立。软件开发只是 template / Soul / Prompt 的一种业务配置，不能把软件开发特判下沉到 Memory 基础设施。

## 当前实现核对

### 已有能力

- `MemoryItem` 已支持：
  - `decision`
  - `execution_lesson`
  - `project_context`
  - `dod_feedback`
  - `user_preference`
  - `skill_feedback`
  - `tool_feedback`
  - `run_summary`
- Run activation 已通过 `selectedMemoryItems()` 选择 workspace Memory，并写入：
  - `ExecutionContext.memoryRetrieval`
  - `ExecutionContext.memoryIds`
  - `ExecutionContext.memories`
- LLM Step 输入包已通过 `runExecutionContextPackage()` 生成 `selectedContext`。
- `recordLLMStepInputMemoryRecallTrace()` 已能为 step input 记录 `MemoryRecallTrace`。
- 当前 completion 成功路径已有部分 `skill_feedback`、`project_context`、`execution_lesson` 写入逻辑。

### 暴露缺口

- Step input 的 `selectedContext` 默认来自 Run activation 时的静态 `ExecutionContext`，没有动态 refresh 最新 Memory。
- 当前失败/assessment 中间态主要写 `run_summary`，缺少高价值 `project_context`、`dod_feedback`、`execution_lesson`、`tool_feedback`。
- `deterministic_workspace_list` 没有按“当前 run 优先、决策价值优先、类型配额”排序，容易被旧 run 的流程摘要占满。
- trace 只说明 retrieved IDs，不说明候选集、排除原因、类型配额和触发来源。
- Prompt / Soul / Skill / Tool / MCP 的有效选择结果没有稳定沉淀为 Memory，因此 provider retry 或 repair 可能重复推断能力边界。

## Memory 资产分类

### 1. Accepted Input Memory

定义：LLM Step 输入中已被 runtime 接受并进入推理上下文的关键输入，不记录全量 prompt，只记录可复用的压缩结论和 provenance。

建议映射到现有 Memory 类型：

| 内容 | MemoryType | 说明 |
| --- | --- | --- |
| Goal / DoD 有效解释 | `dod_feedback` 或 `project_context` | 记录 DoD-to-test 解释、验收边界、弱证据规则 |
| 项目结构、入口、配置结论 | `project_context` | 记录被 LLM 接纳的代码/配置事实 |
| Skill 选择与触发原因 | `skill_feedback` | 记录 selected skills、触发依据、可复用约束 |
| Soul / Prompt 阶段约束 | `project_context` 或 `execution_lesson` | 记录当前 stage 的行为边界和禁止项 |
| Tool / MCP 能力边界 | `tool_feedback` | 记录 allowed tool kinds、MCP server/capability 选择 |
| 用户已确认决策 | `decision` | 记录决策结论，不重复记录完整对话 |
| Evidence policy | `project_context` | 例如 zero-work test inconclusive |

当前 `ws-759510dc7da435ca` 中应沉淀的 accepted input 示例：

- 当前 Goal / DoD 要求的是轻量级 OpenAI-compatible 本地网关，核心验收覆盖非流式、流式、usage 记录、错误透传、并发、配置和单文件交付。
- 当前 workspace 是 Go 项目，入口为 `main.go`，模块名为 `gateway`。
- 当前 `config.yaml` 存在，但实现只读取环境变量，未解析 `config.yaml`。
- `[no test files]` 按 evidence policy 只能作为弱证据，不能支撑 DoD satisfied。
- software-dev validation Soul 要求逐项 DoD evidenceRefs，不能用总体“已完成”替代。

### 2. Necessary Output Memory

定义：LLM 围绕 Goal / DoD 输出并被 runtime 接受、后续步骤必须延续的结论。

建议映射到现有 Memory 类型：

| 内容 | MemoryType | 说明 |
| --- | --- | --- |
| Gap analysis | `dod_feedback` | 每个 DoD 的缺口、证据缺失、失败原因 |
| Repair plan | `execution_lesson` | 下一轮修复方向、禁止重复的无效路径 |
| Verification plan | `project_context` 或 `execution_lesson` | 验证命令、测试矩阵、DoD-to-test 映射 |
| Tool execution strategy | `tool_feedback` | shell/patch/MCP 使用顺序和约束 |
| Prompt/Soul 约束解释 | `execution_lesson` | 对当前 stage 有效的执行边界 |
| Replan reason | `decision` | 为什么需要 replan，下一轮应避免什么 |

当前 `ws-759510dc7da435ca` 中应沉淀的 necessary output 示例：

- 不应把 `usage.csv` 启动前不存在直接判为功能失败；应通过测试请求触发生成并验证 CSV 行。
- 应新增 `main_test.go`，用 `httptest` 模拟 upstream。
- 测试至少覆盖：
  - 非流式 usage 字段提取。
  - 流式响应转发与字符估算。
  - 401 / 429 上游状态码透传。
  - `usage.csv` 追加。
  - 并发请求下 CSV 写入不乱序或不崩溃。
  - 配置加载是否支持 `config.yaml` 和环境变量。
- 若继续只 scan/read 而不补 patch/test，不能满足 DoD。

## 写入策略

### Accepted Input 写入点

在 LLM Step 输出被 runtime 接受后，从该 Step 的 `executionContextPackage.selectedContext` 和 prompt 输入摘要中抽取 accepted input Memory。

候选来源：

- `selectedContext.skills`
- `selectedContext.memories`
- `selectedContext.memoryRetrieval`
- Soul routing 与当前 soul stage。
- MCP selection 与 allowed tool kinds。
- Prompt contract 中对当前 stage 生效的规则。
- Goal / DoD 原文及 evidence policy。
- 已读取并被 LLM 引用的 file/shell/tool evidence。

写入原则：

- 不写全量 prompt。
- 不写敏感字段。
- 只写“被接受且后续可复用”的压缩结论。
- 每条 Memory 必须带 source Step、GoalID、DoDID、stage、evidenceRefs。

### Necessary Output 写入点

在以下场景写入：

- assessment-only 返回合法 `dodAssessment` 后。
- graph follow-up 输出被接受后。
- runtime repair 输出被接受后。
- provider retry 失败或成功收口后。
- repair exhausted 进入 governance/replan 前。
- completion gate 接受 Result / Artifact 前。

写入原则：

- 将 `dodAssessment.items` 转为 `dod_feedback`，重点记录 failed/skipped 条目和 evidence gap。
- 将可执行修复计划转为 `execution_lesson`。
- 将工具、MCP、shell、patch 的有效/无效使用经验转为 `tool_feedback`。
- 将最终验证命令、测试矩阵、失败教训转为 `project_context` 或 `execution_lesson`。

## 召回策略

### 动态 refresh

LLM Step 输入构建时，不应只使用 activation 时固定的 `ExecutionContext.memories`。

新增 step-input refresh 触发范围：

- runtime repair step
- provider retry step
- graph expansion limit 后的 assessment step
- repair exhausted governance 前
- replan continuation activation 后的首个 LLM step

### 排序规则

在没有 embedding provider 时，使用规则排序：

1. 当前 run 的 Memory 优先于 superseded / failed run。
2. `dod_feedback`、`execution_lesson`、`project_context` 优先于普通 `run_summary`。
3. 与当前 GoalID / DoDID 匹配的 Memory 优先。
4. 与当前 stage 匹配的 Memory 优先。
5. 明确 superseded 的 Memory 降权。
6. 相似 summary 去重。

建议类型配额：

| 类型 | 数量 |
| --- | --- |
| `project_context` | 2 |
| `dod_feedback` | 2 |
| `execution_lesson` | 2 |
| `skill_feedback` / `tool_feedback` | 1-2 |
| `decision` / `run_summary` | 1 |

总数建议控制在 6-8 条。

### 语义召回

如果 embedding provider 可用：

- 先用 semantic recall 获取候选集。
- 再应用类型配额、当前 run 优先、过时降权。
- trace 需要同时记录 semantic score 和规则 score。

## Trace 可观测性

`MemoryRecallTrace` 需要从“retrieved IDs”升级为“选择解释”。

建议新增字段或 Data 扩展：

- `RefreshSource`: `activation | step_input | repair | provider_retry | assessment | replan`
- `CandidateMemoryIDs`
- `SelectedMemoryIDs`
- `ExcludedMemoryIDs`
- `ExcludeReasons`
- `SelectionPolicyVersion`
- `TypeQuota`
- `SelectionScores`

如果暂不扩模型，可以先在 trace `Reason` 或扩展 payload 中输出紧凑说明，但最终应落为结构化字段。

## Prompt / Soul / Skill / Tool / MCP Memory 边界

### Prompt

Prompt Memory 只记录当前任务中被接受的有效规则，不记录全量 prompt。

示例：

- validation stage 必须逐项 DoD assessment。
- zero-work test 不能作为 satisfied 证据。
- software-dev execution stage 可使用 shell/command、file/edit、patch/apply。

### Soul

Soul Memory 记录当前 stage 的行为边界：

- 当前 stage 的 SoulID。
- allowedToolKinds。
- 禁止项。
- 何时必须 decision / approval。

### Skill

Skill Memory 记录：

- selected skill IDs。
- 触发原因。
- 技术约束和可复用 workflow。
- 与当前 Goal / DoD 的映射关系。

### Tool / MCP

Tool / MCP Memory 记录：

- 可用工具能力。
- 选择原因。
- 成功/失败经验。
- 外部依赖约束。

## 实施任务

| ID | 任务 | 优先级 | 落点 | 验收 |
| --- | --- | --- | --- | --- |
| DMI-01 | 定义 decision memory 写入/召回契约 | P0 | docs | 已落地：文档明确 accepted input / necessary output / Skill-Soul-Prompt-Tool-MCP 边界 |
| DMI-02 | Step input 动态 refresh Memory | P0 | `execution_context_package.go` | 已落地：LLM step input 从 workspace 动态 refresh Memory，不再只复用 activation 静态 Memory |
| DMI-03 | 当前 run 优先 + 类型配额排序 | P0 | orchestrator memory selector | 已落地：当前 run 的 `dod_feedback`/`execution_lesson` 优先，旧 `run_summary` 严格受配额限制 |
| DMI-04 | assessment 写入 `dod_feedback` | P0 | `runtime_memory.go` | 已落地：DoD failed/skipped 条目自动形成可召回 Memory |
| DMI-05 | repair 输出写入 `execution_lesson` | P0 | orchestrator repair path / LLM `memoryWrites` | 已落地：runtime repair follow-up 的 accepted output 会自动沉淀 `execution_lesson`，同时允许 LLM 显式写入 |
| DMI-06 | Skill/Soul/Prompt/Tool/MCP 约束写入高价值 Memory | P1 | context package / memory writer | 已落地：LLM step accepted input 会自动沉淀 Skill、Tool/MCP、Soul/Prompt 约束 |
| DMI-07 | Trace 记录 candidate/selected/excluded | P1 | `MemoryRecallTrace` | 已落地：trace 结构化记录 refreshSource、policy version、candidate/selected/excluded、type quota 和 selection scores |
| DMI-08 | 相似 Memory 去重与 superseded 降权 | P1 | memory selector | 已落地：summary 去重、旧 `run_summary` 配额限制、superseded/failed/cancelled run 降权并写入 selection score reason |
| DMI-09 | e2e 覆盖动态 recall | P1 | e2e / Go tests / scripts | 已落地：Go 单元覆盖动态 recall，新增部署后核对脚本 `scripts/verify-workspace-memory-decision-recall.sh` |
| DMI-10 | assessment-only 合约收口 | P0 | orchestrator completion gate / LLM prompt | 已落地：assessment-only 响应若夹带 `executionGraph` 但已有 `dodAssessment`，runtime 忽略 graph、记录 contract violation 并继续 DoD gate；仅无 assessment 时才触发 contract correction |
| DMI-11 | 弱验证证据回流 execution repair | P0 | evidence quality / runtime repair / software-dev Soul | 已落地：`inconclusive` / `zeroWork` 证据会形成 `dod_evidence_quality_gap` 诊断，repair input 明确要求补真实测试/断言并重新验证 |

## 当前落地状态

本轮完成了 P0 的内核闭环：

- `run` source 允许写入 `dod_feedback`。
- `run` source 允许写入 `decision`、`dod_feedback`、`execution_lesson`、`skill_feedback`、`tool_feedback`。当前阶段不保留旧兼容限制，`project_context` 和 `user_preference` 也不再依赖关键词白名单，只保留敏感内容过滤。
- LLM 显式 `memoryWrites` 可以直接沉淀高价值 decision memory，包括 DoD gap、repair lesson、skill/tool feedback、项目上下文和偏好。
- `llm_dod_assessment` 输出中 failed/skipped DoD item 会自动生成 `dod_feedback`，内容聚焦缺口和 reason。
- LLM step 被 runtime 接受后，会从 step input 的 `executionContextPackage.selectedContext` 和 run `ExecutionContext` 中自动沉淀：
  - `skill_feedback`：被接受的 Skill、触发/选择依据、required tools 和约束。
  - `tool_feedback`：allowed tool kinds、MCP server/capability、自动绑定信息。
  - `project_context`：Soul routing、当前 stage 约束和 prompt 记录策略，且不记录全量 prompt。
- runtime repair follow-up 被接受后，会自动沉淀 `execution_lesson`，内容包含 repair attempt、failed step、failure class/signature、failure reason、missing DoD item 和 accepted repair output 摘要。
- provider retry follow-up 被接受后，会自动沉淀 `tool_feedback`，记录 provider retry attempt、失败上下文和 retry output 摘要。
- `runExecutionContextPackage()` 在每个 LLM step input 构建时从 workspace Memory 动态 refresh，并覆盖 `selectedContext.memories` / `selectedContext.memoryRetrieval`。
- 动态 selector 按当前 run 优先、高价值类型优先、类型配额和 summary 去重选择 Memory。旧 run 的 `run_summary` 不再能挤占当前 run 的 `dod_feedback` / `execution_lesson`。
- `MemoryRecallTrace` 已扩展结构化选择解释字段：`RefreshSource`、`SelectionPolicyVersion`、`CandidateMemoryIDs`、`SelectedMemoryIDs`、`ExcludedMemoryIDs`、`TypeQuota`、`SelectionScores`。read model 同步输出这些字段。
- selector 已接入同 workspace 的 run 状态：当前 run 最高优先级，completed run 轻微加权，superseded / failed / cancelled run 降权，降权原因进入 `SelectionScores.reason`。
- 新增部署核对脚本 `scripts/verify-workspace-memory-decision-recall.sh <workspace-id> [run-id]`，用于检查：
  - 当前 run 是否产生高价值 Memory。
  - 动态 step-input recall 是否选择了高价值 Memory。
  - trace 是否具备结构化 candidate / selected / quota / score 解释。
- assessment-only / providerRetryAssessmentOnly 合约已进一步收口：如果 LLM 在 assessment-only 响应中夹带 `executionGraph`，但同时提供了合法 `dodAssessment`，runtime 会删除并忽略 graph，保留 `assessmentOnlyContractViolation` / `ignoredExecutionGraph` 标记，然后继续 DoD evidence gate；不会执行该 graph，也不会因为夹带 graph 而丢弃可用 assessment。
- 弱证据不再只停留在 validation 诊断：当 satisfied DoD 条目引用 `inconclusive`、`zeroWork`、`[no test files]` 等证据时，runtime 会生成 `dod_evidence_quality_gap` 诊断并进入 execution repair。repair input 会携带 `evidenceRepairRequired`、`verificationEvidenceGap`、`missingDoDItems` 和 `repairGuidance`，要求补真实测试/断言后重新运行验证。
- software-dev 默认 Soul 已补充：
  - planning 阶段读取可选文件前先 `file/list`，不确定时 `file/read optional:true`。
  - execution 阶段遇到无测试、0 tests、无断言或 inconclusive evidence 时必须补有效测试/断言，不得重复运行弱验证命令。
  - validation / assessment-only 阶段禁止返回新的执行图；证据不足时输出 failed/skipped DoD assessment，由 runtime repair 回到执行阶段补齐。

本轮尚未完成的部分：

- 若后续需要进一步增强，可把 trace 的 `SelectionScores` 拆成更细的规则分项分数，当前实现先输出总分和原因标签。
- Provider timeout 仍需要部署态持续观察。如果本地模型在 validation/repair 阶段继续 60s 超时，应进一步按 stage 配置 LLM timeout 或压缩 provider retry 输入，但当前代码已减少 assessment-only graph 返工和弱证据 assessment 打转。

## 验收场景

### 场景 1：当前 run Memory 动态进入 repair

1. LLM assessment 产生 `dod_feedback`，指出测试缺口。
2. Runtime 写入 Memory。
3. 下一次 repair step 构建输入时 refresh Memory。
4. `MemoryRecallTrace` 显示该 `dod_feedback` 被 selected。
5. Repair 输出不再只重复 scan/read，而是沿 gap analysis 规划 patch/test。

### 场景 2：Skill/Soul/Prompt/Tool/MCP 约束可复用

1. Activation 选择 software-dev Soul 和相关能力。
2. LLM Step 接受该上下文。
3. Runtime 写入 `skill_feedback` / `tool_feedback` / `project_context`。
4. Provider retry 或 repair step 能召回这些约束。
5. 后续输出不违反 allowedToolKinds、不丢失 DoD 逐项评估规则。

### 场景 3：旧流程摘要不再挤占上下文

1. workspace 中存在多条旧 `run_summary`。
2. 当前 run 产生新的 `dod_feedback` 和 `execution_lesson`。
3. Step input recall 最终 selected memory 中当前 run 高价值 Memory 优先。
4. Trace 记录旧 `run_summary` 被降权或去重。

### 场景 4：assessment-only 夹带 executionGraph 不阻塞收口

1. graph expansion / provider retry 进入 assessment-only。
2. LLM 返回 `dodAssessment`，但错误夹带 `executionGraph`。
3. Runtime 删除并忽略 `executionGraph`，记录 contract violation trace。
4. Runtime 继续执行 DoD evidence gate；若 assessment 和证据有效则完成 run，若证据不足则进入 repair。
5. 被忽略 graph 中的节点不得被 dispatch。

### 场景 5：`[no test files]` 回流补测试

1. `go test ./...` 输出 `[no test files]`，evidence policy 标记 `inconclusive` / `zeroWork`。
2. LLM 若尝试用该 step 支撑 satisfied DoD，completion gate 拒绝该 evidence。
3. Runtime 生成 `dod_evidence_quality_gap`，repair input 携带缺口、证据引用和修复指导。
4. Repair step 必须补真实测试文件、断言和验证命令。
5. 只有新测试实际执行并通过后，才能支撑 satisfied DoD。

## 开放问题

- 是否需要新增更细的 MemoryType，例如 `accepted_input_context`、`goal_dod_interpretation`、`verification_plan`？当前建议先复用已有类型，避免扩模型过早。
- Skill/Soul/Prompt Memory 是否应由 orchestrator 自动抽取，还是由 LLM 输出 `memoryWrites` 显式声明？建议两者结合：关键上下文自动抽取，业务结论允许 LLM 显式写入。
- Prompt Memory 的粒度需要谨慎，避免把大段 prompt 变成 Memory 噪音；只记录当前任务生效的规则和边界。
