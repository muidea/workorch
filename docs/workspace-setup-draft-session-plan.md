# Workspace Setup Draft Session Plan

## 目标

在 Web 新建 workspace 向导中支持多轮对话，由 LLM 辅助生成标准格式的 Goal 和 DoD 草案。用户确认前不创建 workspace，也不写入 Goal/DoD；确认后复用现有 workspace 创建、Goal 定义、DoD 定义和 Soul 绑定流程。

新建向导在“工作空间基础信息”步骤内提供配置方式开关：

- 手动填写：保留原有步骤流，用户依次填写 Goal、DoD、Soul 绑定并确认提交。
- 智能生成：用户先完成基础信息，再进入独立的“智能向导”步骤，通过多轮对话生成并应用 Goal/DoD 草案；应用后跳过 Goal/DoD 手动步骤，进入 Soul 绑定和确认提交。

配置方式不是独立步骤；智能向导是智能生成模式下的独立步骤。切换上一步/下一步或切换配置方式时，已填写的工作空间、Goal、DoD 和 Soul 绑定表单值必须保留，避免用户已输入信息丢失。

## 边界约束

- 该能力是通用的“创建前需求澄清与草案生成”，不属于 run、ticket 或 approval。
- WorkOrch 基础流程只处理对话、模板上下文、结构化草案、校验和归一化。
- 具体业务偏好来自 workspaceTemplate、Soul、默认 DoD 和用户输入，不在 appservice/kernel 中写具体模板、语言、命令或业务场景特判。
- LLM 输出必须是结构化 JSON；不能把 Markdown 原文直接落库。
- DoD 入库前必须复用 `doditems.Normalize`，避免标题、父项或空泛结构项成为独立验收项。

## 业务流程

1. 用户打开 Web 新建 workspace 向导，在基础信息步骤选择 workspaceTemplate 和配置方式。
2. 手动填写模式沿用原步骤：工作空间基础信息 -> Goal -> DoD -> Soul -> 确认提交。
3. 智能生成模式使用独立步骤流：工作空间基础信息 -> 智能向导 -> Soul -> 确认提交。
4. 智能向导步骤内输入第一轮自然语言需求描述。
5. Web 创建 draft session，并追加用户消息。
6. 后端组合以下上下文提交给 LLM：
   - workspace 标题、描述、工作目录。
   - 当前选中的 workspaceTemplate 名称、描述、默认 Goal、默认 DoD。
   - 当前多轮对话历史。
   - 当前草案状态。
   - 通用 Goal/DoD 输出规范。
7. LLM 返回结构化草案：
   - `goalText`
   - `doDItems`
   - `assumptions`
   - `clarifyingQuestions`
   - `warnings`
   - `confidence`
8. 用户可继续补充信息，后端基于完整 session 修订草案。
9. 用户点击“应用到表单”，草案写入现有 Goal/DoD 表单状态。
10. 智能生成模式校验 Goal/DoD 已应用后进入 Soul 绑定；手动填写模式继续通过 Goal/DoD 步骤编辑。
11. 确认创建时，仍调用现有：
   - `createWorkspace`
   - `defineGoal`
   - `defineDoD`
   - `updateWorkspaceSoulBindings`

## 数据模型

```json
{
  "id": "wsdraft-xxx",
  "templateId": "generic-governed",
  "workspace": {
    "title": "xxx",
    "description": "xxx",
    "workingDirectory": "xxx"
  },
  "messages": [
    { "role": "user", "content": "..." },
    { "role": "assistant", "content": "..." }
  ],
  "draft": {
    "goalText": "...",
    "doDItems": ["..."],
    "assumptions": [],
    "clarifyingQuestions": [],
    "warnings": [],
    "confidence": "medium"
  },
  "state": "clarifying | draft_ready | confirmed | expired"
}
```

## 接口

### 创建会话

`POST /api/v1/workorch/workspace-setup-draft-sessions`

请求：

```json
{
  "templateId": "generic-governed",
  "title": "新工作空间",
  "description": "背景描述",
  "workingDirectory": "/path/to/workspace"
}
```

响应：

```json
{
  "draftSession": {}
}
```

### 追加消息并生成草案

`POST /api/v1/workorch/workspace-setup-draft-sessions/{id}/messages`

请求：

```json
{
  "content": "用户补充的需求信息",
  "draft": {
    "goalText": "用户手工调整后的 Goal",
    "doDItems": ["用户手工调整后的 DoD"]
  }
}
```

响应：

```json
{
  "draftSession": {}
}
```

### 重新生成草案

`POST /api/v1/workorch/workspace-setup-draft-sessions/{id}/regenerate`

基于已有消息、模板上下文和当前草案重新生成。

### 确认会话

`POST /api/v1/workorch/workspace-setup-draft-sessions/{id}/confirm`

当前阶段 Web 仍可直接使用现有创建链路；该接口预留为后续“一键确认创建”。

## 草案生成规则

- Goal 必须是一段明确、可执行的目标，不写成步骤清单。
- DoD 每条必须是可验证的验收事实。
- DoD 不应包含纯标题、章节号、父级标签或空泛描述。
- 智能向导是 workspace 创建期草案生成，不是运行期执行；当自然语言意图足够形成 Goal/DoD 时，应先生成可创建的草案。
- 模板或 Soul 中描述的运行期决策、审批、参数补齐，应沉淀为 Goal/DoD 行为约束，不应作为创建期草案生成的阻塞条件。
- 不要求用户在向导阶段手工翻译或补齐内部工具参数、ID、枚举值或实现字段名；必要时记录为 assumption/warning，由运行期 Decision/Approval 继续治理。
- 如果用户输入存在歧义但 workspace 意图仍清晰，应记录最佳理解为 `assumptions` 或 `warnings`，不要反复生成澄清问题。
- 只有当现有对话和模板上下文无法形成 workspace Goal 或 DoD 时，才生成 `clarifyingQuestions`；不要编造验收项。
- 如果用户手动编辑了草案，用户编辑内容优先级高于 LLM 默认补全。
- 每次生成都应保留上一轮已确认的信息，并基于新增消息修订，而不是无条件从零覆盖。

## 实施任务

| ID | 任务 | 状态 | 验收 |
| --- | --- | --- | --- |
| WSD-01 | 新增设计文档 | Completed | 文档明确边界、流程、接口和验收规则 |
| WSD-02 | 后端新增 draft session 类型和内存会话管理 | Completed | 能创建、追加消息、重新生成；session 保留多轮消息和草案状态 |
| WSD-03 | 后端新增 LLM 草案生成器 | Completed | 使用当前 LLM 配置，要求输出结构化草案 |
| WSD-04 | 后端 DoD 草案统一走 `doditems.Normalize` | Completed | 标题/父项不会进入草案 DoD |
| WSD-05 | HTTP API 接入 draft session | Completed | Web 可调用创建、追加消息、重新生成 |
| WSD-06 | Web 新建向导接入多轮对话 | Completed | 智能生成入口位于独立智能向导步骤内，展示对话、草案预览，并可应用到 Goal/DoD 表单 |
| WSD-07 | 测试覆盖 | Completed | 后端单测、HTTP 测试、Web build 通过 |
| WSD-08 | 边界核对 | Completed | 新增能力只处理通用 draft session、结构化草案和 DoD 归一化；边界搜索未发现本次新增模板/业务/语言/命令特判 |
| WSD-09 | Web 向导配置方式与步骤状态管理 | Completed | 手动模式保留原步骤；智能模式从基础信息进入智能向导、Soul、确认；上一步/下一步和模式切换不清空已填写表单 |
| WSD-10 | 智能向导应用状态校验 | Completed | 智能模式必须生成并应用草案后才能离开智能向导步骤，模板预填值不能绕过智能向导 |
| WSD-11 | 智能向导显示优化 | Completed | 智能向导减少边框留白；Goal 使用正文块展示；DoD 使用有序列表完整展示，避免长文本被 Tag 压缩 |
| WSD-12 | 智能向导多轮修订与按钮合并 | Completed | 生成/重新生成合并为“生成/修订草案”；多轮输入基于 session 当前草案和完整消息历史动态修订，生成后立即更新预览 |
| WSD-13 | 智能向导生成反馈与预览稳定性 | Completed | 首次生成使用刚创建 session 的模板草案作为基线；状态、置信度、生成摘要、告警使用可读展示；未形成完整 Goal/DoD 前不能应用到表单 |
| WSD-14 | 智能向导 LLM 草案解析兼容与空草案兜底 | Completed | 兼容 LLM 常见字段别名；空草案自动生成中立澄清问题；待澄清问题前置展示，避免只显示空 Goal/DoD |
| WSD-15 | 智能向导草案合并保护 | Completed | LLM 返回部分字段或空草案时保留 currentDraft 里的 Goal/DoD，只叠加新的澄清、告警和摘要，避免模板草案被空响应覆盖 |
| WSD-16 | 智能向导消息请求上下文同步 | Completed | 消息/重新生成请求携带当前 template/workspace 基础信息；后端可用请求上下文修复缺失 templateId 的旧 session，并恢复模板草案基线 |
| WSD-17 | 智能向导最终存储合并保护 | Completed | 生成器返回澄清-only 或部分草案时，最终写回 session 前仍按字段级合并，避免测试替身、真实 LLM 或后续 provider 绕过 appservice 合并规则后清空已有 Goal/DoD |
| WSD-18 | LLM provider schema 分流与澄清生命周期 | Completed | `workspace/setup_draft` 使用结构化 JSON 草案提示词，不再套用执行规划 `LLMExecutionDecision` 合约；用户追加消息时旧澄清问题只保留在对话历史，不再作为 currentDraft 字段反复回写 |
| WSD-19 | 创建期与运行期职责分离 | Completed | 智能向导不再把运行期工具参数补全当作草案生成阻塞条件；自然语言意图足够时生成可创建草案，歧义进入 assumptions/warnings，运行期参数补全继续由 Decision/Approval 治理 |

## 验收标准

- 用户可以在新建向导里多轮补充需求。
- 草案能随补充信息更新 Goal 和 DoD。
- 草案 DoD 不出现独立 Markdown 标题、父项或空项。
- 用户确认前不会创建 workspace 或写入 Goal/DoD。
- 用户可以手动编辑最终 Goal/DoD，并以手工编辑结果为准。
- 配置方式开关位于工作空间基础信息步骤，不作为单独向导步骤。
- 智能生成模式下，智能向导作为单独步骤展示和校验。
- 智能生成模式下，必须应用草案后才能进入 Soul 绑定步骤。
- 智能向导内的 Goal/DoD 草案按业务模型展示，Goal 为目标正文，DoD 为可读验收项列表。
- 智能向导支持多轮对话修订；新一轮输入必须基于上一轮草案动态更新 Goal/DoD，并在响应后显示最新预览。
- “继续生成草案”和“重新生成”不作为两个独立按钮暴露，统一由“生成/修订草案”承载。
- 智能向导生成后必须显示可读状态、置信度、生成摘要或待澄清/告警信息，避免只显示内部状态值或空预览。
- LLM 返回字段名不完全匹配 schema 时，系统应尽量按通用别名解析；确实无法形成 Goal/DoD 时必须显示可读澄清问题。
- LLM 返回部分草案时不能清空已有 currentDraft 的 Goal/DoD；草案修订必须做字段级合并。
- 消息追加和重新生成请求必须同步当前 template/workspace 基础信息，避免旧 session 或异常 session 丢失模板上下文后持续产生澄清-only 草案。
- 生成器返回值写回 session 前仍必须执行字段级合并；任何澄清-only 响应都只能追加澄清状态，不能覆盖已有完整 Goal/DoD。
- 非执行规划类 LLM 请求必须按 `schema` 生成对应结构化 JSON，不得套用 `workorch.llmExecutionDecision.v1` 提示词。
- 用户追加新消息时，上一轮澄清问题应由消息历史承载；生成前的 currentDraft 基线应清理旧澄清问题，避免 LLM 重复输出已过期问题。
- 智能向导不得把运行期工具参数、内部字段或实现细节作为创建期必填项；当 Goal/DoD 可形成时，仍应允许应用草案。
- 手动填写和智能生成两种模式的步骤流清晰且互不清空已填写内容。
- 无 LLM 配置或 LLM 调用失败时，Web 提示失败，不影响手工创建流程。
