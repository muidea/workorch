# 软件需求开发编排能力完善计划

## 目标

本文档用于跟踪 `workorch` 在既有内置 `workspaceTemplate` 基础上，继续完善
“通过 Goal / DoD 自驱动完成软件需求开发”的能力。

本轮目标不是把 `workorch` 改造成软件开发专用 Code Agent。软件开发只是
`workorch` 支持的一类 Workspace 场景，核心仍然是通用的 Goal / DoD 自驱动、
自循环、可治理、可追溯编排能力。

目标形态：

- 用户在创建 Workspace 时显式选择合适的 `workspaceTemplate`，软件开发任务使用
  既有 `software-dev` 模板。
- 用户提交开发 `Goal` 和验收 `DoD` 后，系统自动完成需求理解、代码库发现、任务
  分解、实现、测试、修复、验收和交付物生成。
- 只有在缺少必要信息、重大技术路径分歧、风险操作授权等真实阻塞时，才通过
  Decision Ticket 或 Approval 请求用户介入。
- 普通开发步骤不需要用户 step by step 引导或逐步确认。
- 所有执行过程、Memory 召回、TaskDecomposition、Patch、Shell、ToolCall、DoD
  Assessment、Result 和 Artifact 都应可追溯。

## 范围边界

### 模板层增强

模板层增强只影响特定 `workspaceTemplate` 的默认配置、Soul 行为约束、默认 DoD、
推荐能力和验收策略。

允许调整：

- `software-dev` 默认 Goal / DoD。
- `software-dev` 六阶段 Soul 的行为约束。
- `code-review`、`doc-generator` 与 `software-dev` 的协作边界说明。
- 面向软件开发 Workspace 的默认验证策略和 Artifact 要求。

禁止调整：

- 禁止把软件开发专用规则写入通用 kernel / appservice / governance 核心逻辑。
- 禁止在 core 里硬编码具体业务、具体城市、具体代码仓或具体测试场景。
- 禁止让 Workspace 在 Run 阶段重新绑定 template；template 只在创建 Workspace 时
  初始化。

### 通用平台层增强

平台层增强必须保持领域中立，可被任意模板复用。

允许增强：

- Goal / DoD 质量校验和补全建议。
- TaskDecomposition 从只读投影演进为可确认、可追溯的计划事实。
- 运行期计划进度、证据链、失败修复循环和结果质量门控。
- 验证命令、项目约定、历史决策等 Memory 的召回、写入和证据关联。
- Result / Artifact 与 DoD 条目的严格映射。

禁止增强：

- 禁止平台层解析软件开发特定业务语义。
- 禁止在平台层根据 Goal 文本做场景硬编码。
- 禁止引入和 Run / Goal / DoD 并行的第二套任务状态机。

## 当前基线

### 内置模板

当前已内置以下 `workspaceTemplate`：

| 模板 ID | 场景 |
| --- | --- |
| `generic-governed` | 通用治理任务 |
| `weather-query` | 天气查询治理 |
| `software-dev` | 软件开发 |
| `code-review` | 代码审查 |
| `doc-generator` | 文档生成 |
| `data-analysis` | 数据分析 |

### `software-dev` 模板现状

当前 `software-dev` 已具备：

- 默认 Goal：编码实现、运行测试、修复缺陷、产出交付物。
- 默认 DoD：
  - 目标（Goal）对应的需求能力已实现，并与验收范围保持一致。
  - 代码和配置变更保持最小且与目标直接相关，并提供 `unified_diff` 证据。
  - 相关 build/test 验证已执行并通过；无法执行时记录跳过原因和影响评估。
  - lint / 静态检查按项目约定执行；缺失命令时记录替代检查或跳过原因。
  - 最终结果包含改动摘要、验证命令、验证结果、失败/跳过说明、残余风险和后续建议。
  - DoD 验收评估逐项给出 `evidenceRefs`，并在交付物索引中可追溯。
- 六阶段 Soul：
  - `soul-software-dev-planning`
  - `soul-software-dev-execution`
  - `soul-software-dev-decision`
  - `soul-software-dev-approval`
  - `soul-software-dev-validation`
  - `soul-software-dev-artifact`

`software-dev` 与 companion template 关系（`SRD-08` 收口）：

- `software-dev` 是“实现与验证”主流程模板，不隐式触发或切换到其他模板。
- `code-review` 是独立只读审查模板，可在开发完成后单独创建 Workspace 执行复核。
- `doc-generator` 是独立文档产出模板，可在开发完成后单独创建 Workspace 生成文档交付。
- 第一版不做模板自动串联；如需组合，必须由用户显式创建对应 Workspace 并提交 Goal / DoD。

### 已完成能力

`docs/software-dev-code-agent-closure-plan.md` 已记录上一阶段 Code Agent 能力收口，
当前 DEV-01 至 DEV-15 均为 `verified`。

已具备的平台能力：

- Run / Goal / DoD 编排模型。
- Workspace 创建期 template 初始化。
- 阶段 Soul 绑定。
- LLM `executionGraph`、`patchRequests`、`shellCommands`、`mcpToolCalls`、
  `memoryWrites`、`dodAssessment`。
- Decision Ticket / Approval 治理。
- Memory 召回、写入和追溯。
- File / Diff / Patch 能力。
- Web Search 能力。
- Result / Artifact / Patch / ToolCall / ValidationResult 证据。
- Runtime repair loop。
- TaskProgress、ContextBundle、CompressedContext、TaskDecomposition 只读投影。
- TUI / Web 对运行状态、结果、交付物和 Memory trace 的展示基础。

### 当前已确认约束

- 全新部署首次启动默认 Workspace 使用 `generic-governed`，不隐式选择
  `software-dev`。
- 软件开发任务需要在创建 Workspace 时显式选择 `software-dev` template。
- TUI 不提供普通 workspaceTemplate 重新绑定能力。
- TUI 本地模式继续通过 EventHub 与内置后端交互，不使用本地 HTTP client。
- 状态刷新和交互应基于 EventHub 异步回调，不采用定时轮询。

## 差距分析

### 模板层差距

| ID | 差距 | 说明 |
| --- | --- | --- |
| SDT-01 | `software-dev` 默认 DoD 偏粗 | 当前 DoD 覆盖编译、测试、lint、diff、报告，但缺少需求对齐、最小变更、回归范围、残余风险和证据映射要求 |
| SDT-02 | 软件需求澄清策略还可更明确 | Soul 已要求避免 step by step，但需要更明确区分“必须 Decision”和“可自行从代码/Memory 推断”的边界 |
| SDT-03 | 代码库发现和验证命令发现需要模板化 | 需要要求先识别技术栈、项目脚本、测试入口和风险文件，再进入修改 |
| SDT-04 | 交付物质量要求需要细化 | 最终 Result / Artifact 应包含需求达成、变更文件、diff、验证命令、结果、跳过原因、风险和后续动作 |
| SDT-05 | `code-review` / `doc-generator` 与软件开发协作关系不清晰 | 它们可以作为独立模板，也可作为 software-dev 的后续验收或交付补充，但不能被隐式混入核心流程 |

### 通用平台层差距

| ID | 差距 | 说明 |
| --- | --- | --- |
| GDP-01 | Goal / DoD 质量缺少统一预检 | 需要领域中立地判断 Goal 是否可执行、DoD 是否可验证、是否缺少关键约束 |
| GDP-02 | TaskDecomposition 当前主要是只读投影 | 需要评估是否引入 accepted plan / plan revision，使计划、执行和验收能完整追溯 |
| GDP-03 | DoD 条目和证据映射仍可加强 | 最终验收应能逐项看到 evidenceRefs、未满足原因、跳过原因和风险说明 |
| GDP-04 | 验证命令和项目约定复用不足 | 可通过 Memory 记录已验证命令、失败修复经验、项目规范，并在后续 Run 召回 |
| GDP-05 | 结果质量门控需要平台化 | 防止最终 Result 只有“已完成”这类弱摘要；该能力应检查结构和证据完整性，而不是检查软件开发语义 |
| GDP-06 | repair loop 可见性和控制粒度不足 | 需要让用户和前端知道当前修复轮次、失败原因、继续条件和升级 Decision 的原因 |
| GDP-07 | 多端展示模型需要统一 | Web / TUI 对 Goal、DoD、TaskDecomposition、Memory Recall、Artifact、Approval、Decision 的展示应严格匹配业务模型 |

## 任务清单

状态取值：

- `pending`：已确认需求，尚未开始。
- `in_progress`：正在开发或验证。
- `implemented`：代码已实现，尚未完成完整验收。
- `verified`：代码、文档、e2e 验证均完成。
- `blocked`：存在外部依赖或设计问题需要先确认。
- `deferred`：确认延期。

### Phase 1：基线和设计对齐

| ID | 任务 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| SRD-01 | 固化本轮目标和边界 | 文档 | P0 | verified | 文档明确软件开发是模板场景，平台能力保持领域中立 |
| SRD-02 | 核对现有 `software-dev` 模板和 Soul | 文档/模板 | P0 | verified | 明确当前可复用项、需要增强项和不应改动项 |
| SRD-03 | 核对旧文档潜在冲突 | 文档 | P1 | verified | 清理或标注旧文档中与当前约束冲突的描述，例如默认模板策略 |

### Phase 2：模板和 Soul 增强

| ID | 任务 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| SRD-04 | 完善 `software-dev` 默认 DoD | 模板 | P0 | verified | 默认 DoD 覆盖需求达成、最小相关变更、测试/验证、diff、交付物、风险说明 |
| SRD-05 | 强化规划 Soul 的代码库发现要求 | Soul | P0 | verified | Planning 阶段要求先识别技术栈、入口、验证命令、相关文件和风险点 |
| SRD-06 | 强化执行 Soul 的自治开发闭环 | Soul | P0 | verified | Execution 阶段明确实现、验证、失败修复、证据产出和阻塞升级边界 |
| SRD-07 | 强化 Validation / Artifact Soul | Soul | P0 | verified | DoD 逐项证据映射、跳过原因、残余风险和交付物索引要求明确 |
| SRD-08 | 明确 companion template 关系 | 文档/模板 | P1 | verified | `code-review`、`doc-generator` 与 `software-dev` 的边界和组合方式有记录 |

### Phase 3：通用 Goal / DoD 编排增强

| ID | 任务 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| SRD-09 | 增加 Goal / DoD 质量预检模型 | 后端/通用 | P0 | verified | 领域中立地输出可执行性、可验证性、缺失约束和建议补充项 |
| SRD-10 | 设计 accepted plan / plan revision | 后端/通用 | P0 | verified | TaskDecomposition 可作为确认后的计划事实追溯，且不替代现有 Run 状态机 |
| SRD-11 | 强化 DoD evidence mapping | 后端/通用 | P0 | verified | Run Snapshot / Observability 可按 DoD 条目展示证据、状态、失败原因 |
| SRD-12 | 强化验证命令 Memory 策略 | Memory/通用 | P1 | verified | 可记录并召回项目验证命令、约定和失败修复经验，且带来源证据 |
| SRD-13 | 引入 Result 质量门控 | 后端/通用 | P0 | verified | 缺少目标成果、验证摘要、证据引用或风险说明时不能输出弱完成结果 |
| SRD-14 | 暴露 repair loop 运行态 | 后端/Web/TUI | P1 | verified | 前端可看到修复轮次、最近失败、下一步、升级 Decision 原因 |

### Phase 4：展示和验收固化

| ID | 任务 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| SRD-15 | 统一 Web / TUI 业务区展示 | Web/TUI | P1 | verified | Goal、DoD、TaskDecomposition、Memory Recall、Artifact、Decision、Approval 展示字段与业务模型一致 |
| SRD-16 | 增加真实软件需求开发 e2e | e2e | P0 | verified | 沙箱项目提交 Goal + DoD 后，自动修改代码、运行验证、修复失败、输出 diff 和逐项 DoD 证据 |
| SRD-17 | 增加需求歧义 e2e | e2e | P0 | verified | 缺少关键业务含义时产生 Decision，而不是猜测实现 |
| SRD-18 | 增加风险操作 e2e | e2e | P1 | verified | 依赖升级、破坏性接口变更、迁移或部署类操作触发 Approval |
| SRD-19 | 增加修复失败升级 e2e | e2e | P1 | verified | 多轮 repair 仍失败时，系统输出失败证据并升级 Decision |

## 建议实施顺序

1. 先完成 `SRD-01` 至 `SRD-03`，确保本轮目标和历史文档没有冲突。
2. 再完成 `SRD-04` 至 `SRD-08`，低风险增强 `software-dev` 模板和 Soul。
3. 之后推进 `SRD-09`、`SRD-11`、`SRD-13`，优先补齐通用 Goal / DoD 质量闭环。
4. 最后推进 `SRD-10`、`SRD-12`、`SRD-14`、`SRD-15` 至 `SRD-19`，完善计划事实、
   Memory 复用、修复可见性和 e2e 验收。

## 验收场景

### E2E-SRD-01：明确需求自动开发

给定一个小型测试项目和明确 Goal / DoD，创建 `software-dev` Workspace 后提交任务。

期望：

- 系统识别技术栈和验证命令。
- 生成任务拆分和执行计划。
- 自动修改代码。
- 运行相关测试。
- 失败时自动修复。
- 输出 unified diff。
- Result 包含目标成果、实现摘要、验证摘要、证据引用、风险和后续动作。
- DoD 每一项都有 evidenceRefs。

### E2E-SRD-02：需求歧义升级 Decision

给定存在多种合理业务语义的 Goal，且无法从代码、配置或 Memory 推断。

期望：

- 系统不猜测实现。
- 产生 Decision Ticket。
- Ticket 问题具体，包含已知上下文和需要用户补充的信息。

### E2E-SRD-03：风险操作升级 Approval

给定任务需要依赖升级、破坏性接口变更、数据迁移或部署操作。

期望：

- 系统在执行前产生 Approval。
- Approval 包含 action、context、影响范围、替代方案和回滚说明。
- 拒绝后不执行风险操作。
- 批准后继续执行，并保留审批证据。

### E2E-SRD-04：修复失败后可追溯阻塞

给定一个无法在限定修复轮次内完成的任务。

期望：

- 系统展示 repair loop 尝试次数。
- 保留每轮失败证据。
- 达到上限后升级 Decision 或输出明确失败 Result。
- 不输出“已完成”弱摘要。

## 待确认问题

| ID | 问题 | 默认建议 |
| --- | --- | --- |
| Q1 | 是否需要将 accepted plan 作为新持久化模型？ | 建议先设计为通用模型，但不替代现有 Run / Step / executionGraph |
| Q2 | Goal / DoD 预检是否允许自动补写 DoD？ | 第一版只给建议或生成待确认草案，避免擅自扩大验收范围 |
| Q3 | 验证命令 Memory 是否按 Workspace 还是项目路径复用？ | 建议以 Workspace 为主，记录项目路径、命令来源和成功证据 |
| Q4 | companion template 是否需要自动串联？ | 第一版不自动串联，只在文档和 Soul 中明确可选关系 |
| Q5 | Result 质量门控失败时如何处理？ | 建议触发 artifact/validation repair，仍失败再升级 Decision |

## 跟踪记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-06 | 初始创建。基于当前内置 templates、`software-dev` Soul、已 verified 的 Code Agent 收口计划和用户校准后的目标边界，形成后续需求开发编排增强计划。 |
| 2026-05-06 | 完成 Phase 1（SRD-01/02/03）：已核对内置模板和 `software-dev` Soul 基线，已修正文档冲突项（默认 Workspace 模板策略以 `generic-governed` 为准，`software-dev` 需显式选择）。 |
| 2026-05-06 | 完成 Phase 2 核心项（SRD-04/05/06/07）：已更新 `software-dev` 默认 DoD 和 planning/execution/validation/artifact Soul 约束，并增加 `bootstrap/defaults_test.go` 以防止默认模板和 Soul 语义回退。 |
| 2026-05-06 | 完成 SRD-08：已明确 `software-dev` 与 `code-review`、`doc-generator` 的协作边界和组合规则；第一版保持不自动串联，仅支持显式组合。 |
| 2026-05-06 | 完成 SRD-09：新增 Goal / DoD 质量预检模型（可执行性、可验证性、缺失约束、补充建议），并通过 appservice 与 HTTP 路由 `/api/v1/workorch/workspaces/{workspaceId}/goal-dod-quality-check` 对外提供。 |
| 2026-05-06 | 完成 SRD-11：在 `RunSnapshot` / `RunObservability` 新增 `doDEvidence` 结构，按 DoD 条目输出证据状态（`linked/partial/missing`）、失败原因和缺失证据引用，并补充 read model 回归测试。 |
| 2026-05-06 | 完成 SRD-13（implemented）：在 `RunSnapshot` / `RunObservability` 新增 `resultQuality` 结构化质量评估，按 `goalOutcome`、`verificationSummary`、`evidenceRefs`、`knownLimitations/residualRisks`、`nextActions` 与 DoD 条目覆盖输出 `passed/warning/failed` 及问题明细；当前先以可观测门控落地，避免破坏既有运行闭环。 |
| 2026-05-06 | 完成 SRD-10（implemented）：新增 `taskPlans` 计划事实投影（anchor + revision + state + stage + evidence + occurredAt），在 `RunSnapshot` / `RunObservability` 暴露计划版本演进，不替代既有 Run 状态机；已通过 appservice/server/orchestrator 单测回归。 |
| 2026-05-06 | 完成 SRD-12（implemented）：在完成态证据收口时新增验证命令记忆与失败修复经验记忆写入（复用 `MemoryItem`，不新增存储模型），携带命令、退出码、证据引用和错误信号，并补充 orchestrator 单测验证。 |
| 2026-05-06 | 完成 SRD-14（implemented）：在 `RunSnapshot` / `RunObservability` / `TaskProgress` 新增 `repairLoop` 投影，汇总修复轮次、最近失败（failureClass/failureSignature/step/checkpoint）、下一步动作以及修复耗尽升级 Decision 的原因与票据引用，并补充 read model 单测覆盖“自动修复中”和“修复升级决策”场景。 |
| 2026-05-06 | 推进 SRD-15（进行中）：`workorch` 侧 client/backend snapshot 类型补齐 `taskProgress`（含 `repairLoop`），TUI 进度区追加修复轮次摘要展示（`repair #N | failureClass | nextAction`），并补充 TUI 单测验证显示链路。 |
| 2026-05-06 | 推进 SRD-15（进行中）：TUI 上下文拉取链路补齐运行态 DoD 读取；Workspace Snapshot 展示模型补齐 `taskDecompositions`；主视图新增 `Task Decomposition`、`Memory Recall`、`Governance` 业务区并按模型字段渲染，避免仅依赖结果文本拼接。 |
| 2026-05-06 | 推进 SRD-15（进行中）：继续补齐 TUI 的 Goal/DoD/TaskDecomposition/Memory Recall/Governance 展示回归测试，确认主视图可直接从 snapshot/taskProgress 读取业务字段。 |
| 2026-05-06 | 完成 SRD-15（implemented）：Web 控制台补齐 `taskProgress.repairLoop`、`runSnapshot.repairLoop`、`runObservability.repairLoop` 类型映射与展示链路；“任务进度与恢复”按业务模型新增修复状态、轮次、最近失败、下一步、升级信息与证据字段；并完成 `workorch-console` 构建及 appservice/TUI/backend 回归测试。 |
| 2026-05-06 | 回归验证：执行 `./scripts/verify-software-dev-code-agent.sh` 全量通过（包含 orchestrator 修复与交付证据、approval 合约、CLI 架构守卫、TUI 接受测试与关键 e2e）。 |
| 2026-05-06 | 新增 `scripts/verify-software-requirement-development-orchestration.sh` 与 `make verify-software-requirement-orchestration`：统一映射 SRD-16~19 验收链路。当前已完成本地确定性回归（SRD-16 合同测试、SRD-19 repair exhaustion + read-model escalation 投影）；SRD-17/18 复用 `verify-llm-workspace-execution.sh` 的 deployed e2e（通过 `WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1` 启用）。 |
| 2026-05-06 | 完成 SRD-16~19 验证闭环：新增 `scripts/verify-software-dev-goal-dod-e2e.sh`，在 deployed 服务上验证软件开发 Goal/DoD 自动改码修复闭环与 repair 耗尽升级 Decision；并通过 `WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 make verify-software-requirement-orchestration` 全量通过。 |
| 2026-05-06 | 完成 SRD-10/12/13/14/15 验证闭环：扩展 `scripts/verify-software-requirement-development-orchestration.sh` 为 SRD-10~19 统一验收，新增对 deployed `scenario1` snapshot/observability 的 `taskPlans`、`resultQuality`、`doDEvidence`、`repairLoop`、`memoryRecallTraces` 断言，并执行 `WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh` 全量通过。 |
| 2026-05-06 | 补齐 SRD-15 前端验收闭环：在统一验收脚本中纳入 `make console-build`，并再次执行 `WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh`，确认 Web 控制台资源构建与 Web/TUI read-model 展示链路均通过。 |
