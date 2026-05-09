# WorkOrch 全链路评估与收口计划

## 目标

本文档用于跟踪本轮对 WorkOrch 处理链路的全量评估和后续收口工作，覆盖：

- WorkOrch 基础流程框架：Workspace、Goal、DoD、Run、Step、Decision、Approval、Result、Artifact、Memory、EventHub、Read Model。
- LLM 执行链路：prompt 组织、context budget、provider retry/failover、streaming progress、usage/cost、DoD completion gate。
- 软件开发 `workspaceTemplate`：`software-dev` 默认 DoD、六阶段 Soul、复杂软件开发 Goal / DoD 闭环、e2e 验收。
- TUI/Web/CLI 展示与交互链路：状态刷新、业务模型展示、EventHub/SSE、避免功能分叉。

本轮目标不是继续针对单个 Workspace 或单个错误做临时补丁，而是建立跨层契约，确保复杂软件开发任务可以围绕当前有效 Goal / DoD 自动规划、执行、修复、验证和交付。

## 设计约束

- WorkOrch 核心保持领域中立，不在 kernel、governance、orchestrator、LLM 基础契约中硬编码具体业务场景。
- 软件开发只是一个 `workspaceTemplate`，软件开发策略应沉淀到 `software-dev` template、Soul、默认 DoD、e2e 和通用质量门控中。
- Goal / DoD 是强契约，禁止压缩、替换、静默丢弃或只使用摘要视图。
- Decision / Approval 只在真实阻塞、风险操作或治理需要时触发，不能替代普通自动执行。
- TUI 本地模式继续使用 EventHub 异步交互；Web/远端可使用 HTTP/SSE，但展示模型必须复用同一 read model。
- 不采用临时兼容性方案；当前开发阶段不要求老数据兼容。
- 所有新增能力必须有单元测试或 e2e 验证，禁止主动跳过测试项。

## 当前核对结论

### 1. 执行图终止语义存在链路缺口

`ws-e1e310fd02e4375e` 暴露出当前 orchestrator 对 `completionState=continue` 但没有可执行后续节点的处理不完整。

当前表现：

- Run 终态为 `failed`。
- 失败原因为 `invalid argument: completionState continue has no actionable continuation`。
- 失败 step 为 `zz-llm-after-graph-...-run_go_vet`。
- LLM 返回了重复/已执行过的 `executionGraph`，但没有新的 actionable continuation。

当前代码中该行为被定义为硬失败，并有单测锁定：

- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator_test.go`

问题本质：

- LLM 决策契约、执行图可执行性判断、DoD completion gate、repair/replan 之间缺少统一收口策略。
- 对复杂长流程来说，“continue 但图耗尽”不应直接变成普通失败，应进入明确的 DoD 评估、缺口修复、重新规划或治理升级路径。

### 2. LLM 基础 prompt 混入模板/场景语义

`internal/modules/blocks/llm/biz/http_provider.go` 中全局 `decisionContractPrompt()` 当前包含大量具体场景规则，例如：

- 空 workspace / 源码缺失判断。
- 软件开发测试弱证据。
- SSH 凭据处理。
- Web 查询策略。
- code-agent final result 要求。

这些内容虽然解决了近期软件开发问题，但会让所有 template 共享 software-dev/code-agent 偏置，不符合 WorkOrch 基础能力中立约束。

需要拆分为：

- Core LLM Contract：只描述 WorkOrch 通用输出契约、可用 block、治理规则、Goal/DoD 强约束。
- Capability Contract：由 runtimeCapabilities 和 block adapters 提供各能力自己的使用约束。
- Template/Soul Guidance：由当前 Workspace 的 template/Soul 注入具体业务策略。

### 3. LLM context 出口仍存在敏感信息泄漏风险

Decision resolution 写入时已有 redaction，但实际 `ai-proxy/interactions` 中仍看到旧治理历史或上下文中出现 `apiKey=sk-...`。

风险来源：

- 旧数据或历史 governanceHistory 可能包含明文。
- execution context package、runtime repair、graph follow-up 可能直接复制 resolution payload。
- provider request 出口没有统一做最终 redaction 兜底。

需要在 LLM provider request 构建出口统一执行领域中立的 sensitive redaction，并保留可追溯标记。

### 4. e2e 验证链路存在覆盖盲区

`scripts/verify-software-requirement-development-orchestration.sh` 当前对 SRD-17/18 和 SRD-33 的 deployed e2e 仍由 `WORKORCH_SRD_VERIFY_DEPLOYED_E2E=1` 控制。

现有 e2e 还缺少以下回归：

- LLM 返回重复 graph。
- `continue` 但没有新增 actionable node。
- graph 完成后必须进入 DoD 评估或缺口修复。
- 出站 LLM context 不泄漏旧治理历史 secret。
- 全量验证脚本默认不跳过关键 e2e。

### 5. TUI 本地交互架构基本符合既定约束

当前核对结果：

- TUI 本地模式使用 EventHub 后端。
- TUI 订阅 backend events，不依赖本地 HTTP client。
- 未发现 TUI 主路径定时轮询状态。
- Web 侧仍存在 `setInterval`，如果要求 workspace 列表和运行状态全端实时一致，需要继续收敛到 SSE/read model 事件投影。

## 分层评估模型

| 层级 | 范围 | 当前风险 | 收口方向 |
| --- | --- | --- | --- |
| L0 契约模型 | Goal / DoD / Run / Step / Decision / Approval / Result / Artifact / Memory | Goal/DoD 强约束已有基础，但 continuation、repair、replan 契约边界不够完整 | 定义通用 continuation contract 和 terminal gate |
| L1 Orchestrator | executionGraph、follow-up、completion gate、repair loop | `continue` 无 actionable continuation 直接失败 | 引入 graph exhausted -> DoD assessment / repair / replan 策略 |
| L2 LLM Context | prompt messages、context budget、redaction、usage | 基础 prompt 混入场景语义；出口 redaction 不完整 | prompt 分层、出站统一脱敏、P0 超预算硬阻断 |
| L3 Template/Soul | `software-dev` template 和六阶段 Soul | 软件开发策略已增强，但部分策略被下沉到通用 prompt | 将场景规则回收到 template/Soul |
| L4 Read Model/UI | Web/TUI/CLI 展示与状态刷新 | TUI 基本符合，Web 仍有轮询；展示依赖同一 read model 需继续核对 | Web 事件化刷新、展示字段严格匹配业务模型 |
| L5 E2E | deployed e2e、脚本、mock provider | 关键链路仍可被默认 skip，缺少 graph exhaustion 场景 | 禁止关键测试默认 skip，补复杂回归 |

## 任务清单

状态取值：

- `pending`：已确认，未开始。
- `in_progress`：正在处理。
- `implemented`：代码已实现，待完整验证。
- `verified`：代码、文档、e2e 均通过。
- `blocked`：依赖外部信息或设计决策。

| ID | 任务 | 类型 | 优先级 | 状态 | 原因 | 收口方案 | 验收标准 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| FCA-01 | 定义 executionGraph continuation 终止契约 | Orchestrator/设计 | P0 | verified | `continue` 无 actionable continuation 当前直接失败 | 已定义：无 DoD gate 仍非法失败；有 DoD gate 进入 no-actionable validation follow-up | `TestRuntimeOrchestrator_FailsContinueWithoutActionableContinuationWithoutDoDGate` 覆盖 |
| FCA-02 | 实现 graph exhausted 后的通用收口策略 | Orchestrator | P0 | verified | 图耗尽后缺少 DoD 评估/repair/replan 路径 | 已实现 no-actionable continuation follow-up；重复无行动项进入既有 repair 路径 | Orchestrator 单测通过；deployed e2e 场景已补充待重部署后执行 |
| FCA-03 | 调整现有硬失败单测 | Test | P0 | verified | 当前单测锁定旧行为 | 已拆分非法 continue 与 DoD-gated validation/repair 场景 | Orchestrator 单测能区分三类路径 |
| FCA-04 | prompt 契约分层 | LLM/设计 | P0 | verified | 全局 prompt 混入 software-dev / SSH / Web 等场景语义 | 已拆分 core decision contract 与 runtime capability contract | `blocks/llm` 测试确认 core prompt 不含 software-dev 专用短语 |
| FCA-05 | 将软件开发规则回收到 `software-dev` Soul | Template/Soul | P0 | verified | 软件开发策略不应下沉到全局 LLM contract | 已确认空 workspace、新项目初始化、弱测试证据、外部依赖 provider 作用域规则位于 `software-dev` Soul；全局 prompt 已移除对应措辞 | HTTP provider 测试确认 generic prompt 不再携带这些规则 |
| FCA-06 | capability 规则由 runtimeCapabilities 注入 | Blocks/LLM | P1 | verified | SSH/Web/Patch/Shell 规则混在全局 prompt 中 | 已按 runtimeCapabilities 中实际声明的 block kind 注入 Shell/File/Patch/Web/SSH 能力规则 | `TestHTTPProviderCompleteUsesChatCompletionsAndExtractsDecision` 覆盖 |
| FCA-07 | LLM 出站 context 统一脱敏 | Security/LLM | P0 | verified | 旧治理历史或复制链路仍可能泄漏 secret | 已在 provider request 出口统一 redaction，覆盖 input、promptContext turns/sections、traceRefs | `TestRuntimeAdapterRedactsOutboundProviderRequest` 覆盖 |
| FCA-08 | 旧 governanceHistory secret 兜底处理 | Appservice/Orchestrator | P0 | verified | 旧 resolution payload 可能进入 context | activation context、executionContextPackage、repair input 读取治理历史时统一 redaction | appservice/orchestrator/LLM 测试通过 |
| FCA-09 | provider request redaction 观测 | Read Model/Test | P1 | verified | 脱敏后需要可排查 | `contextBudget` 增加 `redactionApplied`、`redactedPaths`，只记录路径不记录原文 | LLM 单测覆盖，read model 已沿用 contextBudget 投影 |
| FCA-10 | 全量 e2e 禁止默认跳过关键项 | E2E | P0 | verified | SRD-17/18/33 deployed e2e 仍可默认 skip | `verify-software-requirement-development-orchestration.sh` 已取消默认 skip，关键 deployed e2e 缺服务时 fail fast | 全量 SRD 脚本已执行并通过，输出不含关键 e2e skip |
| FCA-11 | 增加 repeated graph / no actionable continuation e2e | E2E | P0 | verified | 当前问题未被 e2e 覆盖 | `verify-software-dev-goal-dod-e2e.sh` 增加 Scenario-5，验证 no-actionable continuation validation follow-up | deployed e2e 已通过，Scenario-5 run 完成 |
| FCA-12 | 增加出站 LLM context secret e2e | E2E/Security | P0 | verified | 已发现实际交互日志有泄漏风险 | 既有 SRD-33 secret 场景继续校验 snapshot/observability/event/LLM request；provider 出口增加单测兜底 | 单测与 deployed e2e 均已通过 |
| FCA-13 | Web 轮询刷新核对与替换方案 | Web | P1 | verified | Web 侧存在 `setInterval` | Web 工作空间详情页已改为订阅 `/api/v1/workorch/events/stream`，事件触发 workspace list/context 刷新 | `make console-build` 和 server/backend/TUI 测试通过；源码不再存在该轮询 |
| FCA-14 | Read model 展示字段一致性复核 | Web/TUI | P1 | verified | 业务区展示需严格匹配模型 | 本轮改动继续复用 snapshot/contextBudget/read model 投影，未新增 UI 自造业务字段 | appservice、backend、TUI 测试通过 |
| FCA-15 | `ws-e1e310fd02e4375e` 回归验收 | Runtime/E2E | P0 | verified | 该 workspace 是当前复杂链路样本 | 同类根因已通过 continuation contract、prompt 分层、redaction、e2e 场景收口 | 已用同类复杂软件开发 deployed e2e 全链路回归验证 |
| FCA-16 | EvidenceQuality 质量门控 | Orchestrator/Template | P0 | verified | 只按 exitCode 判断证据有效会让 zero-work 命令支撑 accepted Result | Core 增加领域中立证据质量状态并在 DoD completion gate 拒绝 invalid/inconclusive/skipped/zero-work 证据；具体命令/输出识别规则只作为 `workspaceTemplate.evidencePolicies` 数据配置，并在创建 workspace 时绑定到 workspace | `TestRuntimeStepEvidenceQualityUsesRuntimeOutputMetadata`、`TestDoDAssessmentEvidenceQualityRejectsInconclusiveStepEvidence`、`TestActivateWorkspaceExecutionWithBackendAddsWorkspaceBoundEvidencePolicy` 覆盖 |

## 目标行为定义

### Graph exhausted handling

当 LLM 返回 `completionState=continue`，但当前 executionGraph 没有新的可执行节点时：

1. 如果 Run 不需要 DoD completion gate，视为非法 continuation，失败并记录诊断。
2. 如果 Run 需要 DoD completion gate，且已有足够 evidence summary，则调度 validation follow-up，要求 LLM 在当前证据上输出：
   - `completed + dodAssessment`，或
   - `continue + missing verification/implementation graph`，或
   - `blocked_on_decision` / `blocked_on_approval`，或
   - `failed` 并说明不可恢复原因。
3. 如果 LLM 再次返回重复 graph 且无 delta，进入 repair/replan 策略，不允许无限循环。
4. 所有路径都必须在 run snapshot 和 read model 中暴露原因、证据和下一步。

### Prompt neutrality

全局 LLM contract 只允许包含：

- 输出 JSON schema 和字段约束。
- completionState 语义。
- Goal / DoD 是强约束。
- Decision / Approval 治理规则。
- blockKinds 必须来自 runtimeCapabilities。
- sensitive data 禁止明文输出和传播。
- WorkOrch-managed artifact/report 文件不能由 patch 写入。

模板或场景策略必须来自：

- `executionContext.currentSoul`
- `executionContext.soulRouting`
- `workspaceTemplate`
- selected skill / MCP / capability descriptions

### Secret redaction

所有进入以下位置的数据必须经过统一脱敏：

- LLM provider request input。
- prompt messages。
- promptContext sections。
- runtime repair input。
- execution context package。
- read model context bundle。
- eventstream payload。
- Web/TUI snapshot display。

脱敏策略保持领域中立，按 key 和 value pattern 识别：

- `apiKey`
- `token`
- `secret`
- `password`
- `authorization`
- `privateKey`
- `sk-*`
- bearer token
- 常见 connection string secret 段

## 验证计划

### 单元测试

- Orchestrator：
  - `continue` 无 graph / 无 DoD gate -> failed。
  - `continue` 重复 graph / 有 DoD gate -> validation follow-up。
  - graph exhausted follow-up 返回 DoD 缺口 -> repair。
  - graph exhausted follow-up 返回完整 DoD -> complete。

- LLM：
  - prompt core contract 不包含 software-dev 专用短语。
  - template/Soul guidance 能进入 prompt。
  - provider request 出口统一 redaction。
  - Goal/DoD raw section 不被压缩。

- Appservice：
  - governanceHistory 读取时脱敏。
  - context bundle 不泄漏旧 resolution secret。
  - read model 暴露 redaction summary。

### E2E

- `FCA-E2E-01`：复杂软件开发 graph exhausted 回归。
- `FCA-E2E-02`：重复 graph 无新增节点回归。
- `FCA-E2E-03`：旧治理历史 secret 不进入 LLM request。
- `FCA-E2E-04`：全量验证脚本不跳过 SRD-17/18/33。
- `FCA-E2E-05`：Web/TUI 展示同一 read model 下的 repair、DoD、usage、artifact。

## 与既有文档关系

- `docs/software-requirement-development-orchestration-plan.md`：继续作为软件开发 Goal / DoD 闭环主计划。
- `docs/software-dev-dod-repair-closure-plan.md`：继续作为 DoD repair 缺口专项计划。
- `docs/codex-like-llm-prompt-context-design.md`：继续作为多轮 prompt/context 组织设计。
- 本文档负责跨上述文档的全链路缺口跟踪，避免问题散落到单点修复。

## 执行顺序建议

1. 先完成 `FCA-01` 至 `FCA-03`，修正 executionGraph 终止语义。
2. 再完成 `FCA-07`、`FCA-08`、`FCA-12`，先堵住敏感信息出口风险。
3. 然后完成 `FCA-04` 至 `FCA-06`，把 prompt 契约做中立化分层。
4. 最后完成 `FCA-10`、`FCA-11`、`FCA-13`、`FCA-14`、`FCA-15`，补齐全量回归和展示一致性。

## 核对记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-09 | 初始创建。基于 `ws-e1e310fd02e4375e`、orchestrator/LLM/appservice/TUI/Web/e2e 核对结果，形成全链路评估和收口任务清单。 |
| 2026-05-09 | 完成第一轮代码收口：执行图 no-actionable continuation 进入 validation/repair；LLM prompt 拆分 core/capability/template 边界；provider request 出口统一脱敏并投影 redaction summary；Web workspace 刷新由轮询切换为 SSE；SRD 全量脚本取消关键 e2e 默认 skip；software-dev e2e 增加 Scenario-5 覆盖 no-actionable continuation。已通过 `go test ./... -count=1` 与 `make console-build`，deployed e2e 待最新服务重部署后执行。 |
| 2026-05-09 | 最新服务重部署后执行收口验证：修正 deployed e2e mock 对多轮 prompt 的 `runId/stepId` 解析；隔离 Scenario-5 workspace；修复 MagicHub unsubscribe 阻塞 RuntimeSession teardown 的风险。`scripts/verify-software-requirement-development-orchestration.sh`、`scripts/verify-llm-workspace-execution.sh`、`scripts/verify-software-dev-goal-dod-e2e.sh` 与 `go test ./... -count=1` 均通过。 |
| 2026-05-09 | 重新部署最新服务后再次复验：`scripts/verify-software-requirement-development-orchestration.sh` 全量通过，覆盖 SRD-16、SRD-17/18、SRD-33 deployed e2e 且无关键 e2e skip；随后执行 `go test ./... -count=1` 全量通过。 |
| 2026-05-09 | 基于 `ws-e1e310fd02e4375e` 复核补漏：新增 `EvidenceQuality` 通用证据质量模型；`workspaceTemplate.evidencePolicies` 在创建 workspace 时绑定到 workspace，激活上下文只注入 workspace 已绑定策略；运行时依据策略输出 `inconclusive/zero_work` 等结构化证据质量，DoD completion gate 不允许其支撑 satisfied 条目。 |
