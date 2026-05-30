# Workspace Setup Draft Session

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 workspace setup draft session 的产品能力与 profile 约束。

## Stable Rules

### Positioning

- setup draft session 属于“创建前需求澄清与草案生成”，不属于 run、ticket 或 approval
- 用户确认前不创建 workspace，也不写入 Goal/DoD
- 确认后仍复用现有 `createWorkspace`、`defineGoal`、`defineDoD`、`updateWorkspaceSoulBindings` 主链

### Step Flow

手动模式：

- 工作空间基础信息
- Goal
- DoD
- Soul
- 确认提交

智能生成模式：

- 工作空间基础信息
- 智能向导
- Soul
- 确认提交

配置方式只是基础信息步骤内的开关，不应成为独立步骤；模式切换和上一步/下一步都不应清空已填写内容。

### Draft Contract

LLM 返回必须是结构化草案，而不是直接落库 Markdown。稳定草案字段至少包括：

- `goalText`
- `doDItems`
- `assumptions`
- `clarifyingQuestions`
- `warnings`
- `confidence`

DoD 入库前必须统一走 `doditems.Normalize`，避免标题、父项或空泛结构项进入正式验收项。

### Generation Rules

- Goal 必须是一段明确、可执行的目标，不写成步骤清单
- DoD 每条必须是可验证的验收事实
- 用户手工编辑内容优先级高于 LLM 默认补全
- 每次生成都应保留上一轮已确认信息，基于新增消息修订，而不是无条件从零覆盖
- 若自然语言意图足够形成 Goal/DoD，应优先生成可创建草案，而不是反复追问运行期参数

### Boundary

- 具体业务偏好来自 workspaceTemplate、Soul、默认 DoD 和用户输入
- appservice/kernel 不应写入具体语言、命令、模板或业务场景特判
- 无 LLM 配置或 LLM 调用失败时，不应影响手工创建流程

## API / Product Mapping

setup draft session 的稳定产品面包括：

- `POST /api/v1/workorch/workspace-setup-draft-sessions`
- `POST /api/v1/workorch/workspace-setup-draft-sessions/:sessionId/messages`
- `POST /api/v1/workorch/workspace-setup-draft-sessions/:sessionId/regenerate`
- `workspace template` 选择入口
- 确认后回落到：
  - `POST /workspaces`
  - `POST /workspaces/:workspaceId/goals`
  - `POST /workspaces/:workspaceId/dods`
  - `PUT /workspaces/:workspaceId/soul-bindings`

字段方向：

- `templateId`
  - 决定默认模板约束和提示上下文
- `title` / `description` / `deliverableDirectory`
  - 表达 workspace 基础信息草案
- `content`
  - 表达当前对话轮次新增输入
- `draft`
  - 表达当前待确认的结构化草案
- `dialogLanguage`
  - 表达向导交互语言，而不是 runtime 执行语言

## Module Mapping

该能力的稳定落点应限定在 application / product 组织层：

- `application/workorchd`
  - draft session 创建、修订、确认主链
- setup draft session in-memory/session state
  - 只服务创建前草案过程
- workspace / goal / DoD / soul binding appservice commands
  - 负责确认后的正式落库

它不应直接落到：

- `kernel/orchestrator`
- `run`
- `decision`
- `approval`
- runtime stage contract

## Template / Soul Mapping

setup draft session 与模板/Soul 的稳定关系：

- `templateId` 决定草案阶段可用的默认 Goal / DoD / Soul guidance
- setup draft 期间允许预选 Soul，但不应提前创建 Run
- 确认提交后，template 和 Soul 绑定必须通过正式 workspace create + soul binding 写接口落地
- `software-dev`、`weather-query` 等模板只能影响草案建议，不得在 draft 阶段直接写入业务结果

## Deliverable Boundary Profile View

对 workspace profile 而言，`deliverableDirectory` 是用户可见、可交付、可修改的工作目录，
而不是 WorkOrch 自己的内部状态目录。

创建前表单阶段需要保持以下稳定认知：

- `deliverableDirectory`
  - 表达用户项目根目录或目标交付目录
- runtime internal state
  - 不应在 setup draft 表单中暴露为可编辑字段
- template / draft / Soul guidance
  - 只能建议交付目录的用途，不得把 internal state 路径混入草案

创建期产品约束：

- 用户只能显式填写 `deliverableDirectory`，不能填写 internal runtime state 路径
- setup draft / template / Soul 不得把 `.workorch-data/internal/...` 之类 internal 目录当成交付目标
- 表单确认后，workspace create 需要同时建立 deliverable boundary 和 internal boundary，但只有 deliverable boundary 属于 profile-facing 输入

典型用户语义：

- `software-dev`
  - 交付目录通常是项目仓库根目录
- `doc-generator`
  - 交付目录通常是文档输出目录
- `data-analysis`
  - 交付目录通常是分析脚本与产物目录

## Memory / Context UX

setup draft 阶段的 memory/context UX 应保持极简，不让创建前草案退化成运行时上下文面板。

当前稳定方向：

- draft 阶段可以展示：
  - template 说明
  - Goal / DoD 草案
  - assumptions / clarifyingQuestions / warnings
- draft 阶段不应展示：
  - runtime `selectedContext`
  - `memoryRecallTraces`
  - repair / retry / provider 历史
  - raw executionContext JSON

原因：

- setup draft 只服务“创建前澄清”
- memory recall / selected context 属于 run 已创建后的运行态证据
- 若把两者混到一起，用户会误以为草案阶段已经进入执行

## Form Field Matrix

| 表单字段 | 阶段 | 当前方向 |
| --- | --- | --- |
| `setupMode` | 基础信息 | 只切换“手动填写 / 智能生成”，不应清空现有输入 |
| `templateId` | 基础信息 | 决定默认 Goal / DoD / Soul guidance |
| `title` | 基础信息 | workspace 标题草案，创建期主字段 |
| `description` | 基础信息 | workspace 描述草案 |
| `deliverableDirectory` | 基础信息 | workspace 交付目录草案 |
| `dialogLanguage` | 智能生成 | 向导交互语言，而不是 runtime 执行语言 |
| `content` | 智能生成 | 当前对话轮次新增输入 |
| `draft.goalText` | 草案预览 | Goal 草案，需用户确认后进入正式 Goal |
| `draft.doDItems` | 草案预览 | DoD 草案，确认后进入正式 DoD |
| `draft.assumptions` | 草案预览 | 被模型采用的假设说明 |
| `draft.clarifyingQuestions` | 草案预览 | 仍待澄清的问题 |
| `draft.warnings` | 草案预览 | 风险或注意事项 |
| `defaultSoulId` / `bindings.*` | Soul 步骤 | 正式 workspace 默认 Soul 与阶段绑定 |

## Read-Model / UI Field Direction

当前产品面应优先展示以下字段：

- setup draft summary
  - `draftSession.state`
  - `draft.confidence`
  - `draft.generationSummary`
- draft preview
  - `draft.goalText`
  - `draft.doDItems`
  - `draft.assumptions`
  - `draft.clarifyingQuestions`
  - `draft.warnings`
- workspace template selection
  - `templateId`
  - template `name` / `description`
- final binding
  - `defaultSoulId`
  - `bindings.execution` / `bindings.validation` 等阶段绑定
- deliverable boundary
  - `deliverableDirectory`
  - template `description`
  - 与创建后 workspace 标题/目录的一致性提示

## Activation Field Mapping

setup draft session 虽然不直接创建 activation，但其确认结果会影响后续
`workspaceExecutionActivation` 的以下字段：

| Activation 字段 | 创建期来源 | 当前方向 |
| --- | --- | --- |
| `activation.workspace` | `title` / `description` / `deliverableDirectory` / `templateId` | workspace 创建后的正式基线 |
| `activation.run.goalId` / `doDId` | 确认后的 Goal / DoD | activation 选择的正式目标定义 |
| `activation.executionContext.soulRouting` | `defaultSoulId` / `bindings.*` | 当前 run 的 Soul routing 解析结果 |
| `activation.executionContext.executionContract` | template / workspace context hints | 当前 run 的中立执行合同 |
| `activation.soulBindings` | Soul 步骤确认结果 | activation 返回的绑定基线 |
| `activation.taskProgress` / `outcome` | activation 轻量投影 | 用于创建后立即返回的主视图摘要 |

约束：

- setup draft 只决定创建期输入，不直接写 `executionContext`
- `executionContext` 只能在 workspace 创建完成后，由 activation 路径正式生成

## Product Examples

典型产品展示方向：

- 创建向导基础信息页
  - 展示 `templateId`、`title`、`description`、`deliverableDirectory`
- 智能草案页
  - 展示 `draft.goalText`、`draft.doDItems`、`draft.assumptions`、`draft.warnings`
- Soul 绑定页
  - 展示 `defaultSoulId` 与 `bindings.execution` / `bindings.validation`
- 确认提交前摘要
  - 展示“将创建的 workspace 标题/目录/模板/Soul 绑定”，但不展示 runtime executionContext

## Verification Mapping

当前与 setup draft session 直接相关的验证入口包括：

- `make verify-workspace-template-soul`
- `../../50-product-interface/service-api.md`
- `../../50-product-interface/tui/requirements.md`

当前主要验证方向：

- template 选择与 Soul 绑定是否能贯通到 workspace 创建
- 智能生成模式是否仍然输出 Goal / DoD 草案，而不是运行期参数
- draft session 失败时是否仍能安全回退到手工创建

## Cross-Layer Mapping

- 模板与 Soul：
  `../builtin-templates-and-souls.md`
- Soul routing：
  `soul-routing.md`
- API 契约：
  `../../50-product-interface/service-api.md`
- TUI 产品约束：
  `../../50-product-interface/tui/requirements.md`

## Verification Focus

- Web 向导应支持多轮对话修订和“应用到表单”
- 智能模式必须应用草案后才能进入 Soul 绑定步骤
- 智能模式不得把运行期工具参数或内部字段当作创建期必填项

## Source Documents

- `../../50-product-interface/service-api.md`
