# Built-in Templates And Souls

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 内置模板与配套 Soul 的稳定角色。

## Built-in Templates

当前内置模板至少包括：

- `generic-governed`
- `weather-query`
- `software-dev`
- `code-review`
- `doc-generator`
- `data-analysis`

### Template Roles

- `generic-governed`: 通用治理型任务
- `weather-query`: 外部查询与工具调用
- `software-dev`: 软件开发与代码变更闭环
- `code-review`: 代码审查与只读分析
- `doc-generator`: 文档生成与交付整理
- `data-analysis`: 数据分析、脚本与长耗时计算

## Stage Model In Profile Layer

模板通常围绕以下阶段配置 Soul：

- planning
- execution
- decision
- approval
- validation
- artifact

这些阶段在 profile 层的职责是：

- 注入阶段行为约束
- 限定 allowed tool kinds / capability categories
- 提供 template-specific guidance

### Stage Responsibilities

- `planning`: 理解 Goal、拆解任务、形成执行计划
- `execution`: 执行工具调用、文件变更和状态变更
- `decision`: 信息缺失或路径分歧时请求人工决策
- `approval`: 高风险操作前请求人工审批
- `validation`: 基于证据评估 DoD 和交付完整性
- `artifact`: 生成最终 summary、manifest 或交付整理结果

## Stable Template Rules

### Templates And Souls Can Define

- template 默认 Goal / DoD 建议
- stage-specific Soul bindings
- capability allowlist / category boundary
- evidence policy
- deliverable expectation
- domain-specific acceptance guidance

### Templates And Souls Cannot Define

- 与具体 Workspace 无关的业务答案
- 对 core runtime contract 的削弱或绕过
- 取代正式 Goal / DoD 的固定实现结论
- 通过 `soul.md` 文本隐式授予结构化 capability 权限

### Workspace Boundary

- template 只在 Workspace 创建期初始化默认配置
- Run activation 可以刷新中立 `executionContract`，但不应重新绑定 template、Soul、Skill、MCP
- 具体业务目标仍然来自 Goal、DoD、Memory 和用户输入，而不是模板硬编码答案

## Template / Stage / Capability Mapping

当前稳定映射方向：

| 模板 | 主要能力侧重点 | 典型阶段特化 |
| --- | --- | --- |
| `generic-governed` | 中立治理、基础 LLM / shell / file 能力 | planning / decision / approval / validation |
| `weather-query` | MCP / web / 外部查询能力 | execution / validation |
| `software-dev` | file / patch / shell / git / verifier | execution / validation / artifact |
| `code-review` | 只读 file / git / shell 分析 | planning / validation |
| `doc-generator` | file / patch / artifact | planning / execution / artifact |
| `data-analysis` | shell / file / 长耗时运行 | execution / validation / artifact |

约束：

- 模板描述的是能力组合和阶段 guidance，不是业务答案模板。
- 同一模板的阶段 Soul 仍需通过 workspace soul routing 落到结构化绑定。

## First-Cut Template Field Matrix

当前内置模板的稳定字段面主要围绕：

- `id`
- `name`
- `description`
- `goalText`
- `doDItems`
- `defaultSoulId`
- `soulBindings`
- `contextHints`
- `evidencePolicies`

### `generic-governed`

- 角色
  - 中立治理型默认模板
- 默认能力重点
  - 基础 LLM / shell / file / decision / approval
- 字段方向
  - `defaultSoulId`
    - 应指向 generic-governed 默认 Soul
  - `soulBindings`
    - 应覆盖 planning / execution / decision / approval / validation / artifact
  - `contextHints`
    - 以中立 execution contract、治理边界、context assembly 提示为主
  - `evidencePolicies`
    - 以通用 evidence quality / result gate 为主
- 主要验证入口
  - `verify-workspace-template-soul.sh`
  - `verify-llm-workspace-execution.sh`

### `weather-query`

- 角色
  - 外部查询、MCP 工具调用与来源证据模板
- 默认能力重点
  - MCP / web / validation
- 字段方向
  - `defaultSoulId`
    - 应指向 weather-query 默认 Soul
  - `soulBindings`
    - execution / validation 阶段更强调来源、工具和证据
  - `contextHints`
    - 应表达来源追溯、当前事实窗口、查询安全边界
  - `evidencePolicies`
    - 应强调来源、时间、引用链
- 主要验证入口
  - `verify-real-weather-query.sh`
  - `verify-weather-ticket-approval-query.sh`

### `software-dev`

- 角色
  - 软件开发与代码变更闭环模板
- 默认能力重点
  - file / patch / shell / git / MCP(LSP) / verifier
- 字段方向
  - `goalText`
    - 应提供可继续累积的开发目标基线，而不是固定答案
  - `doDItems`
    - 应覆盖 Goal / PRD、unified diff、git snapshot、build/test、真实交付入口、残余风险、evidenceRefs
  - `defaultSoulId`
    - 应指向 `soul-software-dev-*` 体系
  - `soulBindings`
    - 应覆盖六阶段 software-dev Souls
  - `contextHints`
    - 应至少包含 `stateChangePolicy`、`runtimeRepair.prerequisiteManifestTargetPatterns`、`versionControl`、`semanticCodeNavigation`、`profileContracts`、`capabilityUsagePolicies`
  - `evidencePolicies`
    - 应包含 `workEvidenceMarkers`
- 验证策略
  - 服务、代理、流式或长运行入口必须通过有界、前台、可自动退出的验证动作证明，例如项目测试 harness、本地 deterministic fake/stub server、临时端口、受 timeout 控制且自然退出的前台命令或等价 MCP/tool 证据
  - `software-dev` Soul / template 可以要求上述验证习惯，但 WorkOrch core 只校验中立 shell 安全形态；不要把 Go、HTTP、LSP、网关等具体业务判断写入 core
- 主要验证入口
  - `defaults_test.go`
  - `verify-software-dev-goal-dod-e2e.sh`
  - `verify-workorch-runtime-profile-software-dev.sh`

### `code-review`

- 角色
  - 只读审查与风险识别模板
- 默认能力重点
  - file/read / git/read / shell 分析 / validation
- 字段方向
  - `doDItems`
    - 应强调 findings、风险、证据与未验证项，而不是状态变更
  - `soulBindings`
    - execution Soul 应保持只读分析约束
  - `contextHints`
    - 应强调 review 范围、证据、残余风险输出
- 主要验证入口
  - `verify-workspace-template-soul.sh`
  - TUI / code-review profile 后续专项验证

### `doc-generator`

- 角色
  - 文档生成与交付整理模板
- 默认能力重点
  - file/read / patch/apply / artifact
- 字段方向
  - `goalText`
    - 应聚焦文档交付目标
  - `doDItems`
    - 应强调结构完整、交付文件、引用/摘要、可读性
  - `soulBindings`
    - artifact 阶段应偏 manifest / summary 输出
- 主要验证入口
  - `verify-workspace-template-soul.sh`
  - 后续 doc-generator 专项 e2e

### `data-analysis`

- 角色
  - 数据分析、脚本运行和长耗时计算模板
- 默认能力重点
  - shell / file / artifact / validation
- 字段方向
  - `doDItems`
    - 应强调分析结果、方法说明、输入来源、输出交付
  - `contextHints`
    - 应强调长耗时、结果摘要、artifact 导出
  - `soulBindings`
    - validation / artifact 阶段应强调结果解释和交付整理
- 主要验证入口
  - `verify-workspace-template-soul.sh`
  - 后续 data-analysis 专项 e2e

## Embedded Defaults Evidence

当前内置模板 / Soul 默认资源的主要代码证据包括：

- `internal/pkg/workorchdefaults/defaults/souls.json`
- `internal/pkg/workorchdefaults/defaults/souls/<template>/<stage>.md`
- `internal/pkg/workorchdefaults/defaults_test.go`

这些测试当前已直接覆盖：

- `software-dev` template 的 `doDItems`
- `software-dev` 的 `contextHints.versionControl`
- `software-dev` 的 `contextHints.semanticCodeNavigation`
- `software-dev` 的 `profileContracts`
- `software-dev` 的 `evidencePolicies`
- 多个内置 Soul 的关键 `SoulMD` 约束

## Setup-Draft / Activation Field Matrix

各内置模板在创建前草案与 activation 阶段的稳定字段重心至少应保持如下分层：

| 模板 | setup-draft 重点字段 | activation 重点字段 |
| --- | --- | --- |
| `generic-governed` | `title`、`description`、`goalText`、`doDItems`、`defaultSoulId` | `goalId`、`doDId`、`soulBindings`、基础 capability / memory 选择 |
| `weather-query` | `goalText`、`doDItems`、`templateId`、来源与交付目录提示 | `selectedCapabilities`、来源相关 Soul、query/fetch 相关 memory/context |
| `software-dev` | `goalText`、`doDItems`、`defaultSoulId`、阶段 Soul、`contextHints.versionControl`、`semanticCodeNavigation` | `goalId`、`doDId`、`soulBindings`、`skillIds`、`mcpServerIds`、`capabilityIds`、`memoryIds` |
| `code-review` | `goalText`、只读审查 DoD、`defaultSoulId`、只读 contextHints | `goalId`、`doDId`、只读 capability 选择、review Soul routing |
| `doc-generator` | `goalText`、文档交付 DoD、artifact 导向 Soul | `goalId`、`doDId`、artifact / file capability、summary-oriented Soul |
| `data-analysis` | `goalText`、分析目标、输入来源、输出交付 DoD | `goalId`、`doDId`、shell/file capability、长耗时 / artifact Soul |

稳定约束：

- setup-draft 只负责草案与建议，不创建 Run
- activation 只消费已确认的正式 Workspace / Goal / DoD / bindings
- 模板字段可以影响 activation 输入基线，但不能替代 activation 的正式对象引用

## Formal Split And Cross-Reference

本文件当前承担“模板总入口”角色；更细正文应继续由子目录承接：

| 模板 / 主题 | 正式承接文档 |
| --- | --- |
| workspace 侧 Soul routing / setup-draft / default Soul resource | `workspace/soul-routing.md` / `workspace/setup-draft-session.md` / `workspace/soul-md-default-resource.md` |
| `software-dev` Goal/DoD / code-agent / LSP | `software-dev/goal-coverage-and-dod.md` / `software-dev/code-agent.md` / `software-dev/lsp-bridge-integration.md` |
| MCP capability | `mcp/capability-management.md` |
| runtime capability 细分 | `runtime-capabilities/*` |

总入口与子正文的稳定关系：

- 本文件负责：
  - 列模板族、阶段 Soul 方向、逐模板字段重心、跨模板能力边界
- 子文档负责：
  - 单模板 / 单能力的细节规则、验证映射、产品示例

## Product / Read-Model Mapping

模板与 Soul 的主要产品面包括：

- `GET /workspace-templates`
- `GET /souls`
- `GET /workspaces/:workspaceId/soul-bindings`
- `workspace setup draft session`
- `execution activation`

运行态观察面主要复用：

- `context bundle`
- `run snapshot`
- `run observability`

## Verification Mapping

当前与内置模板 / Soul 直接相关的验证入口：

- `make verify-workspace-template-soul`
- `verify-llm-workspace-execution.sh`
- `verify-software-dev-goal-dod-e2e.sh`
- `verify-workspace-web-search-e2e.sh`

这些入口分别验证：

- 模板和 Soul 的 CRUD、绑定与 workspace 初始化
- activation 之后模板约束是否能进入执行链
- 不同模板是否仍保持各自能力边界

## Cross-Layer Mapping

- workspace routing：
  `workspace/soul-routing.md`
- default Soul 资源：
  `workspace/soul-md-default-resource.md`
- API 契约：
  `../50-product-interface/service-api.md`
- 服务验收：
  `../60-verification/service-acceptance.md`

## Boundary

模板和 Soul 可以定义：

- capability allowlist
- evidence policy
- stage guidance
- domain-specific acceptance expectations

模板和 Soul 不可以定义：

- 与具体 workspace 无关的业务答案
- 对 core runtime contract 的削弱
- 取代正式 Goal / DoD

## Source Documents

- `workspace/soul-routing.md`
- `../00-core/runtime-stage-contract.md`

## Open Migration Notes

- `generic-governed`、`weather-query`、`code-review`、`doc-generator`、`data-analysis`
  的逐模板 Soul 细节仍在旧文档中，后续需继续按模板拆分。
