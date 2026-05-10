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
- TUI 普通对话输入不是默认新 Goal：非终态 Run 中作为 operator input
  resume；终态 Run 后作为 operator input 直接重启 Workspace execution；
  只有 `/goal <text>` 或无既有任务上下文时才创建新 Goal。

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

### 最新运行回归差距

基于 `ws-e1e310fd02e4375e` 的运行核对，当前 SRD-01 至 SRD-19 虽已完成原定
验收，但复杂软件开发 Goal / DoD 闭环仍暴露出以下新增差距。这些差距需要进入
下一轮逐项收口，避免继续把问题归因到单一 provider timeout 或单一提示词偏差。

| ID | 差距 | 说明 |
| --- | --- | --- |
| RGD-01 | 内置模板和 Soul 默认值未稳定刷新 | 运行期仍使用缺少“空 workspace 不等于缺源码”约束的旧 `software-dev` Soul，说明嵌入默认值和系统数据之间存在漂移 |
| RGD-02 | 空 workspace 新项目场景仍可能误触发源码缺失 Decision | 软件开发新项目应能从空目录创建最小可验证项目，而不是默认请求源码来源 |
| RGD-03 | Stage Soul 绑定与默认 Soul 上下文存在不一致 | Workspace 阶段绑定为 `software-dev`，但 run context 中出现 `soul-code-review-approval` 作为默认 Soul 信息，容易污染 LLM 上下文 |
| RGD-04 | 补充 Goal / DoD 后的重新规划不充分 | 后续执行需要基于累计 Goal / DoD 形成新的计划修订，而不是只沿用旧 graph 或局部 evidence |
| RGD-05 | DoD 评估证据包不完整 | 最终评估只看到部分 build/test/patch 证据，缺少覆盖全部 DoD 的完整验证证据链 |
| RGD-06 | repair / provider retry 边界不清晰 | provider timeout、DoD 不满足、repair exhausted 被嵌套处理，导致恢复链膨胀和重复 Ticket |
| RGD-07 | LLM context 持续增长 | governanceHistory、graphEvidence、previousStepOutput 等反复进入上下文，导致请求体变大并放大 timeout 风险 |
| RGD-08 | Decision resolution 和 ToolCall 中存在敏感信息持久化风险 | 目标项目测试 provider/API Key 信息会进入 resolution、step input、tool_call 和后续 LLM 上下文 |
| RGD-09 | 目标项目外部依赖配置与 WorkOrch 自身 provider 配置边界需要更强约束 | 用户补充的 DeepSeek 信息是目标项目测试输入，不应被解释为替换 WorkOrch runtime LLM provider |
| RGD-10 | 测试通过但无实际执行仍可能被弱化处理 | `go test -run ...` 输出 `no tests to run` 或 `go test ./...` 输出 `[no test files]` 不能证明 DoD 中流式、Token、异常透传、并发等能力满足 |
| RGD-11 | 软件开发执行策略可吸收成熟 Code Agent 行为 | `ai-proxy/interactions` 显示 Codex 在 PRD 驱动新项目、路径覆盖、根因分析、测试补齐和交付摘要方面有可复用策略，但不能照搬完整历史 prompt 堆叠方式 |
| RGD-12 | LLM prompt 可追溯应保持摘要化和脱敏 | Codex 请求会把完整多轮上下文、工具调用和工具输出堆入请求体，WorkOrch 应继续使用 contextBudget/sourceRefs，默认不持久化完整明文 prompt |
| RGD-13 | DoD completion gate 仍可能接受 DoD 子集 | `ws-e1e310fd02e4375e` 显示最终 accepted assessment 只覆盖了原始 DoD 的一部分，平台不能只信任 LLM 返回的 assessment 子集 |
| RGD-14 | Workspace outcome 可能指向较早 accepted run | 同一 workspace 多次运行后，workspace outcome 可能被 stale taskProgress/旧结果影响，最终展示与最新有效终态 run 不一致 |

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

### Phase 5：复杂软件开发闭环回归收口

| ID | 改进项名称 | 类型 | 优先级 | 状态 | 原因 | 改进方案 | 验收标准 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SRD-20 | 文档状态刷新与新增回归项 | 文档 | P0 | verified | SRD-01 至 SRD-19 的原验收结论不能覆盖 `ws-e1e310fd02e4375e` 暴露的新问题 | 将本轮新增问题作为 Phase 5 独立跟踪，不回写篡改历史验收结论 | 文档明确新增差距、任务、验收场景和跟踪记录 |
| SRD-21 | 内置 template / Soul 默认刷新机制 | 后端/模板 | P0 | verified | 已部署数据可能继续使用旧内置 Soul，导致修复后的模板约束无法生效 | 当前阶段不考虑老数据兼容，内置默认 Soul/template 按内置 ID 做确定性 upsert；用户自定义能力需通过非内置 ID 表达 | 重启或重新部署后，系统内置 `software-dev` Soul/template 与嵌入默认值一致，并有单测覆盖 |
| SRD-22 | 空 workspace 新项目初始化闭环 | Soul/e2e | P0 | verified | 空目录软件开发任务被错误升级为“需要源码来源” Decision | 在 `software-dev` Soul/template 中明确新项目初始化路径；平台只提供通用 template 刷新和阶段上下文，不在 kernel 写软件开发语义 | 空 workspace + `software-dev` + 新项目 Goal 不产生源码缺失 Ticket，可规划创建最小项目骨架和验证命令 |
| SRD-23 | Workspace 默认 Soul 绑定一致性 | 后端/通用 | P0 | verified | run context 中出现无关 `code-review` Soul，可能干扰软件开发规划和审批判断 | 核对 workspace 创建、template 应用和 soul routing，确保阶段绑定、默认 Soul、run context 一致；无默认 Soul 时不注入无关 Soul | `software-dev` workspace 的 run context 不包含无关 template Soul，阶段路由仍按绑定正确解析 |
| SRD-24 | Goal / DoD 累积上下文进入重新规划 | 后端/通用 | P0 | verified | 补充 Goal/DoD 后执行计划未稳定基于累计需求修正 | resume / repair / replan 统一读取当前 workspace 累计 Goal/DoD，并在 task plan revision 标明来源 Goal/DoD | 提交新 Goal/DoD 后，下一轮 plan revision 明确引用最新累计 Goal/DoD，并修正后续执行 graph |
| SRD-25 | DoD 评估证据范围完整性 | 后端/通用 | P0 | verified | 最终 DoD 评估只基于局部 graph evidence，无法判断复杂 DoD 是否满足 | 为 graph/completion follow-up 输入生成当前 run 最新完整 evidence summary，覆盖 patch、shell、validation、tool_call、governance、artifact refs | DoD follow-up 能看到完整最新证据包；复杂 DoD 未满足时列出具体缺口，满足时每项均有 evidenceRefs |
| SRD-26 | repair loop 继续策略 | 后端/通用 | P0 | verified | repair exhausted 后用户回复“继续”会形成多层嵌套 repair，恢复语义不清 | repair exhausted 后要求明确恢复策略：重试同一步、重新规划、补充信息或终止；禁止无边界嵌套 repair attempt | repair attempt 不再无限膨胀；恢复路径和下一步动作在 ticket/context/read model 中可追溯 |
| SRD-27 | provider timeout 持续推进能力 | 后端/通用 | P1 | verified | provider timeout 与业务 DoD 失败混在同一 repair 链路中，导致错误恢复 | 将 provider timeout/failover/context shrink 与 DoD 不满足的 implementation repair 分流处理，分别记录 recovery policy | provider timeout 不直接污染业务 repair；timeout 耗尽后 Ticket 明确要求修复 runtime provider 或稍后重试 |
| SRD-28 | LLM context 大小严控 | LLM/通用 | P0 | verified | governance/evidence 反复进入上下文，context 体积持续增长并放大 timeout 风险 | 按 provider/model context profile 和阶段预算裁剪输入；重复证据只传摘要和 refs；保留可追溯 sourceRefs | 后续 LLM 请求 `contextBudget` 在配置预算内，sourceRefs 可追溯，被裁剪字段可在 read model 中查看 |
| SRD-29 | Decision payload 敏感信息治理 | 安全/通用 | P0 | verified | 目标项目测试 API Key 可能明文持久化到 resolution、step input、tool_call 并进入 LLM 上下文 | 引入领域中立 secret redaction / secretRef；持久化和 LLM 上下文只保留脱敏值，执行工具时按受控 env 注入 | resolution、tool_call、step、event、read model 中不出现 `sk-`、password、token 等明文敏感值 |
| SRD-30 | Ticket 语义边界 | Governance/通用 | P0 | verified | 目标项目测试 provider 信息可能被误读为 WorkOrch 自身 LLM provider 配置 | Ticket context 强制区分 `workorch_runtime`、`target_workspace`、`external_dependency`；校验和 prompt 按 scope 处理 | target workspace 外部依赖配置不会触发 WorkOrch runtime provider 替换语义，也不会要求用户配置 WorkOrch 自身 provider |
| SRD-31 | 测试质量门控 | 后端/Soul/e2e | P0 | verified | `go test ./...` 通过但无测试文件，不能证明复杂 DoD 功能满足 | `software-dev` Soul 要求根据 DoD 生成有效验收测试；平台 resultQuality 识别“无有效断言/无测试文件”的弱证据 | 复杂软件开发 e2e 中必须包含功能测试、stream 测试、usage/异常/并发验证或明确跳过原因 |
| SRD-32 | 最终交付物质量 | Artifact/通用 | P0 | verified | DoD 未满足时不应输出“已完成”或弱交付 | DoD 未满足时生成失败 Result，列出已完成项、未满足项、缺失证据、下一步计划和需要用户补充的信息 | 未满足 DoD 的 run 以明确失败交付结束；满足 DoD 的 run 输出完整 Result/Artifact/DoD evidence |
| SRD-33 | 复杂软件开发 e2e 覆盖 | e2e | P0 | verified | 现有 e2e 未覆盖空 workspace、复杂 DoD、外部测试配置、secret 脱敏和 repair 恢复组合场景 | 新增 deployed e2e：创建空 `software-dev` workspace，提交复杂 Goal/DoD，补充目标测试 provider 信息，验证执行/脱敏/恢复/交付；弱验证证据必须先记录 rejected Result，再进入 repair，repair 耗尽后升级 Decision | e2e 全量通过，并能防止 `ws-e1e310fd02e4375e` 同类回归 |
| SRD-34 | 当前未完成草稿清理 | 工程卫生 | P0 | verified | 工作区存在未完成 untracked redaction 草稿，可能影响测试和提交 | 开发前先删除草稿或补全为正式通用脱敏实现；不得留下半成品文件 | `git status` 无未确认草稿；`go test ./...` 不受半成品影响 |
| SRD-35 | 吸收 Code Agent 工程执行策略 | 模板/Soul | P0 | verified | `ai-proxy/interactions` 显示 PRD 驱动新项目、DoD-to-test、路径覆盖、根因分析和交付摘要对复杂开发任务有效 | 增强 `software-dev` 默认 DoD 和 planning/execution/validation/artifact Soul：要求提取 PRD/需求文档、生成 DoD-to-test 映射、覆盖主路径/错误路径/关键分支、DoD 不满足时补实现/补测试、交付摘要包含变更文件和行为变化 | 内置默认值测试覆盖关键语义，重启后内置 `software-dev` template/Soul 可确定性刷新 |
| SRD-36 | 摘要式 LLM context trace | LLM/通用 | P0 | verified | 需要追溯 LLM 请求上下文组织情况，但不能默认保存完整明文 prompt | 在 contextBudget trace 中显式输出 `tracePolicy=summary_refs_only` 和 `promptCaptured=false`，继续只记录预算、裁剪规则和 sourceRefs | Run Snapshot / Observability 可看到预算、sourceRefs 和 trace policy；无完整 prompt 明文落入默认 read model |
| SRD-37 | DoD 全量覆盖门控 | Orchestrator/通用 | P0 | verified | LLM 返回 `satisfied=true` 时可能遗漏当前 effective DoD 的未完成条目 | completion gate 从 Run executionContext 的 effective DoD 提取非标题条目，对 LLM `dodAssessment.items` 做规范化覆盖对账；遗漏任一条目时记录 rejected Result 并进入 runtime repair | 单测覆盖 markdown/checklist 规范化、遗漏 DoD 子集拒绝和完整覆盖通过 |
| SRD-38 | Workspace outcome 最新终态选择 | Read model/通用 | P0 | verified | Workspace 级 outcome 可能优先使用 stale taskProgress，导致展示较早 accepted run | terminal workspace 的 outcome 优先按 run 事件选择最新终态 run，无事件时回落到最新 accepted result，再回落到 run 列表；非终态仍保留 active/taskProgress 语义 | 单测覆盖 stale taskProgress 指向旧 run 时 outcome 仍选择最新 accepted terminal run |
| SRD-39 | EvidenceQuality 证据质量门控 | Orchestrator/Template | P0 | verified | 命令退出码为 0 但没有实际执行测试时仍可能被 LLM 当作 satisfied 证据引用 | 基础框架新增领域中立 `EvidenceQuality`（valid/invalid/inconclusive/skipped）并在 DoD completion gate 拒绝 zero-work / inconclusive / invalid 证据；具体命令/输出识别规则只作为 `workspaceTemplate.evidencePolicies` 数据配置，创建 workspace 时绑定到 workspace，激活 run 时不按 Soul 名称或业务场景推断策略 | 单测覆盖 workspace-bound evidence policy 注入、runtime 输出 metadata 识别、DoD satisfied 引用 inconclusive step 被拒绝 |
| SRD-40 | Completion gate 持续收口能力 | Orchestrator/通用 | P0 | implemented | `ws-c4b6c80ed677c443` 类场景中，after-graph 评估可能继续扩展 graph；provider timeout retry 又携带完整上下文，导致重复扩展、重复 repair 或无法交付 | 对 graph follow-up 扩展设置通用边界；completion gate provider retry 进入 assessment-only 紧凑模式；终态 Step 禁止同 ID 覆盖；可选文件读取用 `exists:false` 完成输出避免无效 repair | 本地单测覆盖扩展边界、assessment-only retry、终态 Step 幂等和 optional read；待部署后复验同类 workspace |
| SRD-41 | TUI 对话输入重启 Workspace 执行 | TUI/后端通用 | P0 | implemented | 用户在 TUI 输入“继续处理”或补充信息时，期望推进现有 Workspace，而不是被误当成新 Goal 或只能提示使用 `/goal` | activation 支持 `executionContextPatch`；TUI 在非终态 Run 中 resume 并注入 operator input，在终态 Run 后或 `/restart` 时复用累计 Goal/DoD 激活新 Run | TUI 单测覆盖终态 Run 后普通输入重启、新 `/restart` 命令、普通补充信息不创建 Goal；appservice/server 测试覆盖 activation patch 透传 |

## 建议实施顺序

1. 先完成 `SRD-01` 至 `SRD-03`，确保本轮目标和历史文档没有冲突。
2. 再完成 `SRD-04` 至 `SRD-08`，低风险增强 `software-dev` 模板和 Soul。
3. 之后推进 `SRD-09`、`SRD-11`、`SRD-13`，优先补齐通用 Goal / DoD 质量闭环。
4. 最后推进 `SRD-10`、`SRD-12`、`SRD-14`、`SRD-15` 至 `SRD-19`，完善计划事实、
   Memory 复用、修复可见性和 e2e 验收。
5. 基于 `ws-e1e310fd02e4375e` 回归结果，先处理 `SRD-34`、`SRD-21`、`SRD-23`，
   消除工程卫生、默认值漂移和 Soul 上下文污染。
6. 再处理 `SRD-22`、`SRD-24`、`SRD-25`、`SRD-26`、`SRD-28`、`SRD-29`、
   `SRD-30`、`SRD-31`、`SRD-32`，最后用 `SRD-33` 做 deployed e2e 闭环。

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

### E2E-SRD-05：复杂软件开发 Goal / DoD 闭环

给定一个空 `software-dev` Workspace，提交复杂软件开发 Goal 和细粒度 DoD，过程中
通过 Ticket 补充目标项目测试所需的外部依赖配置。

期望：

- 空 workspace 被识别为新项目初始化场景，不产生“源码来源缺失” Ticket。
- 系统根据累计 Goal / DoD 生成 task plan revision，并自动创建源码、配置和测试。
- 目标项目外部测试配置按 `target_workspace` 或 `external_dependency` scope 处理，
  不影响 WorkOrch 自身 runtime LLM provider。
- resolution、step、tool_call、event、read model 中不持久化 API Key、token、
  password 等明文敏感信息。
- 执行证据覆盖 patch、build、test、stream、usage、异常透传、并发或明确跳过原因。
- DoD 未满足时输出明确失败交付和下一步；DoD 满足时输出完整 Result / Artifact /
  evidenceRefs。

## 待确认问题

| ID | 问题 | 默认建议 |
| --- | --- | --- |
| Q1 | 是否需要将 accepted plan 作为新持久化模型？ | 建议先设计为通用模型，但不替代现有 Run / Step / executionGraph |
| Q2 | Goal / DoD 预检是否允许自动补写 DoD？ | 第一版只给建议或生成待确认草案，避免擅自扩大验收范围 |
| Q3 | 验证命令 Memory 是否按 Workspace 还是项目路径复用？ | 建议以 Workspace 为主，记录项目路径、命令来源和成功证据 |
| Q4 | companion template 是否需要自动串联？ | 第一版不自动串联，只在文档和 Soul 中明确可选关系 |
| Q5 | Result 质量门控失败时如何处理？ | 建议触发 artifact/validation repair，仍失败再升级 Decision |
| Q6 | 内置默认 upsert 是否会覆盖用户改过的同 ID Soul/template？ | 当前开发阶段不考虑老数据兼容；建议约定内置 ID 归系统管理，用户自定义使用新 ID |
| Q7 | 目标项目测试 secret 是否允许进入 shell env？ | 允许以受控 secretRef/env 注入执行，不允许明文进入持久化记录、LLM 上下文或前端展示 |

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
| 2026-05-07 | 新增 Phase 5（SRD-20 至 SRD-34）：基于 `ws-e1e310fd02e4375e` 复杂软件开发运行核对，记录内置默认值漂移、空 workspace 误判、Soul 上下文污染、Goal/DoD 重新规划、DoD 完整证据包、repair/provider retry 分流、context 预算、敏感信息治理、Ticket scope、测试质量门控和复杂 e2e 覆盖等待收口项。 |
| 2026-05-08 | 收口 SRD-21/SRD-23：内置 Soul/template 改为确定性 upsert；阶段 Soul 绑定存在时不再向 run context 注入无关系统默认 Soul；已补充 bootstrap/appservice 定向单测。 |
| 2026-05-08 | 收口 SRD-24/SRD-25：blocked run 恢复时刷新当前 workspace 累计 Goal/DoD，并将最新定义写入 run context 与 run GoalID/DoDID；graph/completion follow-up 新增通用 `runEvidenceSummary`，provider context budget 纳入裁剪和 sourceRefs。 |
| 2026-05-08 | 收口 SRD-26/SRD-27/SRD-30：repair exhausted Ticket 增加明确 recovery policy 和可选恢复动作；含糊“继续”类回答不再直接解决 repair exhausted Ticket；provider retry 与业务 repair 分流继续保留 `workorch_runtime` scope 与系统配置恢复约束。 |
| 2026-05-08 | 收口 SRD-28/SRD-29：补充通用脱敏工具，Decision/Ticket/Approval 持久化和事件链路统一脱敏；target workspace 外部依赖配置仍允许提交，但 API Key/token/password 不再明文进入 resolution payload。 |
| 2026-05-08 | 收口 SRD-31/SRD-32：`software-dev` Validation Soul 和 LLM runtime 指令明确 no test files/no assertions 为弱证据；平台 DoD completion gate 拒绝用弱测试证据标记 satisfied；DoD 未满足时记录 `rejected` Result，避免输出“已完成”弱交付。 |
| 2026-05-08 | 收口 SRD-33：扩展 `verify-software-dev-goal-dod-e2e.sh`，新增空 `software-dev` Workspace、累计 Goal/DoD、目标项目外部依赖 secret 脱敏、弱验证证据 rejected Result 场景；修复 e2e 状态等待只匹配 `status.state`。 |
| 2026-05-08 | 回归发现并修复 SRD-32 补漏：DoD assessment 不完整（缺失 `dodAssessment` 或 `summary`）仍作为 LLM 输出契约错误直接失败；完整 DoD assessment 但未满足或弱证据时记录 rejected Result，并进入通用 runtime repair 继续补实现/补测试，repair 耗尽后升级 Decision。 |
| 2026-05-08 | 验证结果：`WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh` 已在当前代码临时服务 `http://127.0.0.1:18089` 全量通过（SRD-10~34，含 SRD-33）；`go test ./... -count=1` 全量通过。 |
| 2026-05-08 | 重新部署后回归：`WORKORCH_BASE_URL=http://127.0.0.1:8080 WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1 ./scripts/verify-software-requirement-development-orchestration.sh` 通过；`WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-e2e-full` 通过，覆盖模板/Soul、LLM、MCP、SkillHub、自动治理、software-dev、SRD-10~34、文件补丁、Web Search、天气治理、TUI 与长输出。 |
| 2026-05-08 | 基于 `/home/rangh/aispace/ai-proxy/interactions` 的 Codex 请求核对，收口 SRD-35/SRD-36：吸收 PRD 驱动新项目、DoD-to-test、路径覆盖、根因分析和交付摘要策略；LLM context trace 明确为 `summary_refs_only` 且 `promptCaptured=false`，避免默认保存完整 prompt 明文。 |
| 2026-05-08 | 回归同步 SRD-33：`verify-software-dev-goal-dod-e2e.sh` 的弱验证证据场景改为验证 rejected Result + runtime repair + repair 耗尽 Decision 升级，避免继续使用“直接失败”旧预期。 |
| 2026-05-08 | 基于 `ws-e1e310fd02e4375e` 核对收口 SRD-37/SRD-38：DoD completion gate 增加当前 effective DoD 全量覆盖对账；workspace outcome 在终态下优先选择最新终态 run，避免旧 accepted result 覆盖最新结果。 |
| 2026-05-09 | 收口 SRD-39：基础框架新增 `EvidenceQuality` 通用证据质量字段和 DoD completion gate 质量检查；`workspaceTemplate.evidencePolicies` 在创建 workspace 时绑定到 workspace，激活 run 时只注入 workspace 已绑定策略；runtime 基于策略输出 `inconclusive/zero_work` 等结构化 metadata，不能支撑 satisfied DoD。已通过 orchestrator、appservice、repository 定向单测。 |
| 2026-05-10 | 收口 SRD-40 本地实现：completion gate 的 graph follow-up 不允许无边界继续扩展；provider timeout retry 对 completion follow-up 使用 assessment-only 紧凑上下文；Step dispatch 不再覆盖终态 Step；`file/read optional:true` 支持可选文件探测。已通过本地 `go test ./... -count=1`，部署后需继续用 `ws-c4b6c80ed677c443` 类场景验证。 |
| 2026-05-10 | 收口 SRD-41 本地实现：TUI 普通对话输入在非终态 Run 中继续作为 operator input resume；终态 Run 后会复用 Workspace 累计 Goal/DoD 激活新 Run；新增 `/restart [message]` 显式重启命令；activation HTTP/EventHub/local/remote 路径均透传 `executionContextPatch`。已通过本地 `go test ./... -count=1`。 |
| 2026-05-10 | 收口 SRD-42 本地实现：修复自动治理 `replan` 后只将旧 Run 置为 `superseded`、但没有可靠创建 continuation Run 的问题；workorchd 现在绑定共享 RuntimeSession 实际 EventHub，监听通用 `superseded + resolved replan` 事件触发下一轮 Workspace activation，并在 Validate 阶段补偿扫描历史遗留状态。已补充 server 定向单测。 |
