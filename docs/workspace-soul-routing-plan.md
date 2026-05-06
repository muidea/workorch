# Workspace Soul Routing Plan

本文定义 workorch 支持 Workspace 按任务环节定制 Soul 的完整方案。目标是让同一
Workspace 在规划、执行、决策、审批、验证和交付物整理等不同环节使用不同的
Soul，同时保持现有单一 `soulId` 启动方式可继续作为本次 Run 的默认 Soul。

## Goals

- 每个 Workspace 可以配置默认 Soul。
- 每个 Workspace 可以按任务环节配置阶段 Soul。
- 启动 Run 时可以传入本次 Run 默认 Soul。
- 启动 Run 时可以传入本次 Run 的阶段 Soul 覆盖项。
- Run execution context 必须记录最终 Soul 路由结果和来源，便于审计、调试和恢复。
- LLM 相关 Step 能根据当前任务环节选择合适的 Soul 作为上下文。
- Soul 能力校验继续约束 Skill、MCP 和 Tool，不允许绕过已有 capability 边界。
  Run 激活时选择的 Skill/MCP/Capability 属于执行能力集合，统一由最终
  `execution` 阶段 Soul 校验；`planning`、`decision`、`approval`、
  `validation`、`artifact` 等非工具阶段只通过自身 `allowedToolKinds` 和
  `soulMd` 限制当前阶段行为，不因执行阶段选择了 MCP/Skill 而阻断 Run 启动。

## Non-Goals

- 不把 Soul 定义复制到 Workspace 目录；Workspace 只保存绑定关系和使用策略。
- 不把 Run 级覆盖回写到 Workspace 配置。
- 当前阶段不要求兼容旧的无 Soul routing 数据；旧 Run 可按缺省 `unbound` 读模型展示。

## Implementation Constraints

- `application/workorch` 是 Soul Routing 的唯一策略解析层，负责在 Run 创建或恢复前解析 Workspace/Run/System 的 Soul 配置并写入 `executionContext.soulRouting`。
- `kernel/orchestrator` 只消费和透传 execution context，不允许根据 Goal、DoD 或其他业务目标文本选择 Soul。
- `blocks/llm` 只根据 Step input 和 `executionContext.soulRouting` 选择当前阶段的 Soul 上下文，不允许读取 Workspace/Soul 仓储，也不允许修改 Soul 绑定。
- `soulMd` 是提示词定义内容，不是授权源；Skill、MCP 和 Tool 权限只能来自结构化字段 `allowedSkillRefs`、`allowedMCPServerRefs`、`allowedCapabilityRefs`、`allowedCapabilityCategories`、`allowedToolKinds`。
- `allowedSkillRefs`、`allowedMCPServerRefs`、`allowedCapabilityRefs` 和
  `allowedCapabilityCategories` 支持 `"*"`。ID 通配用于允许运行期动态注册的
  Skill/MCP/Capability；Capability Category 是业务分类，可由专用 Soul 收窄，
  内置通用模板默认接纳动态分类，并主要通过 `allowedToolKinds` 约束工具边界。
- 阶段级 Soul 选择不违反以上约束；只要选择结果由 `application/workorch` 解析并固化到 Run context，运行期 block 只消费该结果即可。

## Task Stages

正式阶段枚举如下：

| Stage | 用途 | 典型场景 |
| --- | --- | --- |
| `planning` | 目标理解、任务拆分、执行图规划 | 生成 executionGraph、拆分步骤、选择工具 |
| `execution` | 工具执行上下文策略 | shell、MCP、patch、外部工具调用 |
| `decision` | 决策工单生成与补充信息处理 | `provide_missing_info`、`replan`、`reject_current_path` |
| `approval` | 审批说明与风险解释 | 高风险动作、人工批准前说明 |
| `validation` | DoD 验证与证据核对 | 检查验收项、判断 satisfied/failed |
| `artifact` | 正式交付物整理 | 结果摘要、交付报告、发布稿、patch/report |

阶段枚举属于 workorch 业务执行语义，由 `application/workorch` 负责管理和解释。

## Directory Layout

Soul 定制配置纳入 Workspace 数据目录，不放入 `working` 或 `artifacts`。

推荐目录：

```text
.workorch-data/
  system/
    souls/
      soul-general/
        metadata.json
        soul.md
    skill.json
    mcp_server.json
    runtime_profile.json
    workspace_index.json
  workspaces/
    ws-xxx/
      workspace.json
      config/
        soul_bindings.json
        runtime_profile.json
      working/
      artifacts/
      runs/
```

目录职责：

- `system/souls/{soulId}/soul.md`: 全局 Soul 定义内容，保存可复用 Soul 的 Markdown 人格、策略和输出契约。
- `system/souls/{soulId}/metadata.json`: Soul 结构化元数据和能力边界。
- `system/runtime_profile.json`: 系统默认运行配置，可包含系统默认 Soul。
- `workspaces/{workspaceId}/config/soul_bindings.json`: 当前 Workspace 的 Soul 使用策略。
- `working/`: 源码、博文、素材、临时执行文件等可持续编辑内容。
- `artifacts/`: 被 Result/DoD 接受后的正式交付物快照。
- `runs/`: Run 运行证据、状态、治理项和结果元数据。

## Workspace Configuration Model

`config/soul_bindings.json` 建议结构：

```json
{
  "defaultSoulId": "soul-general",
  "bindings": {
    "planning": "soul-planner",
    "execution": "soul-engineer",
    "decision": "soul-analyst",
    "approval": "soul-reviewer",
    "validation": "soul-validator",
    "artifact": "soul-writer"
  },
  "priorityPolicy": [
    "run_stage_override",
    "workspace_stage_binding",
    "run_default",
    "workspace_default",
    "system_default",
    "unbound"
  ]
}
```

说明：

- `defaultSoulId`: Workspace 默认 Soul。
- `bindings`: Workspace 阶段 Soul 绑定。
- `priorityPolicy`: 启用优先级，作为显式配置和审计说明；系统实现必须按该顺序执行。

## Run Activation Model

启动 Workspace 执行时，保留现有 `soulId`，并增加可选 `soulBindings`：

```json
{
  "goalId": "goal-xxx",
  "doDId": "dod-xxx",
  "llmProviderId": "provider-main",
  "soulId": "soul-general",
  "soulBindings": {
    "planning": "soul-planner-for-this-run",
    "validation": "soul-validator-for-this-run"
  }
}
```

语义：

- `soulId` 是本次 Run 默认 Soul。
- `soulBindings` 是本次 Run 阶段级覆盖，只影响当前 Run。
- 未传 `soulBindings` 时使用 Workspace 绑定。
- 未传 `soulId` 时使用 Workspace 默认 Soul或系统默认 Soul。

## Enable Priority

Soul 启用优先级从高到低：

1. Run 请求的阶段级覆盖：`activation.soulBindings[stage]`
2. Workspace 配置的阶段级绑定：`workspace.config.soulBindings[stage]`
3. Run 请求的默认 Soul：`activation.soulId`
4. Workspace 配置的默认 Soul：`workspace.config.defaultSoulId`
5. 系统默认 Soul：`system.runtimeProfile.defaultSoulId`
6. 无 Soul：`unbound`

规则：

- 阶段级配置永远高于默认配置。
- Run 级配置只影响当前 Run，不回写 Workspace。
- Workspace 配置影响后续 Run。
- 系统默认只作为兜底，不覆盖 Run 或 Workspace 显式配置。
- 如果解析到的 Soul 不存在，Run 启动前必须失败。
- 如果解析到的 Soul 存在但不允许当前 Skill/MCP/Tool 选择，Run 启动前必须失败。
- 如果所有层级均未绑定 Soul，允许继续执行，但必须记录 `soulRouting.status = "unbound"`。

## Run Execution Context

Run 创建时应将最终解析结果写入 `executionContext.soulRouting`：

```json
{
  "soulId": "soul-general",
  "soul": {
    "id": "soul-general",
    "name": "General Soul"
  },
  "soulRouting": {
    "status": "resolved",
    "priorityPolicy": [
      "run_stage_override",
      "workspace_stage_binding",
      "run_default",
      "workspace_default",
      "system_default",
      "unbound"
    ],
    "effectiveDefaultSoulId": "soul-general",
    "bindings": {
      "planning": {
        "soulId": "soul-planner",
        "source": "workspace_stage_binding"
      },
      "execution": {
        "soulId": "soul-general",
        "source": "run_default"
      },
      "decision": {
        "soulId": "soul-analyst",
        "source": "workspace_stage_binding"
      },
      "approval": {
        "soulId": "soul-reviewer",
        "source": "workspace_stage_binding"
      },
      "validation": {
        "soulId": "soul-validator",
        "source": "workspace_stage_binding"
      },
      "artifact": {
        "soulId": "soul-writer",
        "source": "workspace_stage_binding"
      }
    }
  }
}
```

当无任何 Soul 时：

```json
{
  "soulRouting": {
    "status": "unbound",
    "reason": "no run, workspace, or system soul configured",
    "bindings": {}
  }
}
```

## Runtime Selection Rules

LLM Step 应根据当前任务意图选择阶段：

- 初始 workspace execution Step: `planning`
- executionGraph 后续工具节点调度上下文: `execution`
- requiredDecision 生成或 Decision resolution 后续 LLM Step: `decision`
- requiredApproval 生成或 Approval 后续 LLM Step: `approval`
- DoD assessment Step: `validation`
- Result / Artifact summary generation: `artifact`

实现边界：

- `application/workorch` 在 Run 创建前解析并持久化完整 `soulRouting`。
- `kernel/orchestrator` 只透传 execution context，不理解 Soul 业务策略。
- `blocks/llm` 根据 Step input 中的阶段标记和 `soulRouting` 选择当前有效 Soul 上下文，不读取仓储、不修改绑定。
- 如果无法识别阶段，使用 `effectiveDefaultSoulId`；若仍不存在则按 `unbound` 执行。

当前实现状态：

- 已落地 Workspace 默认 Soul、Workspace 阶段 Soul、Run 默认 Soul、Run 阶段覆盖的解析和持久化。
- 已落地 `soul.md` 定义方式：Soul 人格、阶段策略和输出契约统一通过 `soulMd` 管理；file backend 写入 `system/souls/{soulId}/soul.md`，结构化能力边界写入 `metadata.json`。
- 已落地 `GET/PUT /api/v1/workorch/workspaces/{workspaceId}/soul-bindings`。
- 已落地 Run `executionContext.soulRouting`，并在 Web Console 暴露 Workspace 级配置和启动执行时的阶段覆盖。
- 已落地 LLM provider request 的 `currentSoulStage`、`currentSoulBinding`、`currentSoul` 注入；当前可识别初始规划、graph 节点执行、Decision resolution、Approval resolution、graph/completion follow-up 验收上下文，并支持 Step input 通过 `soulStage` 显式指定 `planning/execution/decision/approval/validation/artifact`。
- 已落地 Soul capability 校验诊断：Run 启动前会使用最终 `execution`
  阶段 Soul 明确返回不被允许的 Skill、MCP server、Capability、Capability
  Category 或 Tool kind，便于在 API/Web 验收时定位绑定问题。
- `execution` 阶段已由 execution graph 节点调度上下文消费；`artifact` 阶段已支持显式 LLM Step 输入消费，默认 Result Summary Artifact 仍由 DoD assessment 确定性生成。
- `system_default` 已接入 `[runtime].defaultSoulId` 配置源；服务配置响应和 Web Console 会展示当前系统默认 Soul。后续目录规划落地后，可将该值迁移或同步到 `system/runtime_profile.json`。

## API Design

Workspace 级配置：

```http
GET /api/v1/workorch/workspaces/{workspaceId}/soul-bindings
PUT /api/v1/workorch/workspaces/{workspaceId}/soul-bindings
GET /api/v1/workorch/souls
POST /api/v1/workorch/souls
GET /api/v1/workorch/souls/{soulId}
PUT /api/v1/workorch/souls/{soulId}
```

PUT 请求：

```json
{
  "defaultSoulId": "soul-general",
  "bindings": {
    "planning": "soul-planner",
    "execution": "soul-engineer",
    "decision": "soul-analyst",
    "approval": "soul-reviewer",
    "validation": "soul-validator",
    "artifact": "soul-writer"
  }
}
```

响应：

```json
{
  "service": "workorch",
  "workspaceId": "ws-xxx",
  "soulBindings": {
    "defaultSoulId": "soul-general",
    "bindings": {
      "planning": "soul-planner",
      "execution": "soul-engineer",
      "decision": "soul-analyst",
      "approval": "soul-reviewer",
      "validation": "soul-validator",
      "artifact": "soul-writer"
    },
    "priorityPolicy": [
      "run_stage_override",
      "workspace_stage_binding",
      "run_default",
      "workspace_default",
      "system_default",
      "unbound"
    ]
  }
}
```

Run activation 增加：

```json
{
  "soulBindings": {
    "planning": "soul-planner-for-this-run"
  }
}
```

## Web Design

Workspace Detail 增加“执行人格配置”区域：

- 默认 Soul: Select。
- 阶段 Soul 绑定: 表格或表单，列为“任务环节 / 当前 Soul / 来源 / 策略摘要”。
- 保存按钮调用 Workspace `PUT soul-bindings`。
- 启动执行弹窗保留 `Soul` 字段，命名为“本次执行默认 Soul”。
- 启动执行弹窗增加可折叠“阶段 Soul 覆盖”，默认收起。
- Run Detail 展示 `soulRouting`，用于确认各阶段实际使用的 Soul 和来源。

## Validation Rules

保存 Workspace Soul binding 时：

- `workspaceId` 必须存在。
- `defaultSoulId` 可为空；非空时 Soul 必须存在。
- `bindings` 的 key 必须是正式阶段枚举。
- `bindings` 的 value 非空时 Soul 必须存在。
- 同一个阶段只能绑定一个 Soul。

启动 Run 时：

- Run 级 `soulId` 非空时 Soul 必须存在。
- Run 级 `soulBindings` key 必须是正式阶段枚举。
- Run 级 `soulBindings` value 非空时 Soul 必须存在。
- 最终参与 capability 校验的所有有效 Soul 必须允许本次选择的 Skill/MCP/Tool。
- 任一校验失败时，在 Run 创建前返回业务错误。

## Persistence Boundary

当前 file backend 规划：

- Workspace Soul binding 已作为 Workspace-scoped 配置模型保存。
- 系统默认 Soul 当前来自 `application.toml` 的 `[runtime].defaultSoulId`。
- file backend 文件落点为：
  `workspaces/{workspaceId}/config/soul_bindings.json`。
- `workspaces/{workspaceId}/workspace.json` 只保存 Workspace 元数据；读取 Workspace
  时由 application bootstrap repository 合并配置模型，kernel/workspace 不感知目录策略。

## E2E Acceptance

最小验收场景：

1. 创建 6 个 Soul：planner、engineer、analyst、reviewer、validator、writer。
2. 创建 Workspace。
3. 配置 Workspace `defaultSoulId` 和阶段 bindings。
4. 启动 Run，不传 Run 级阶段覆盖。
5. 验证 Run snapshot 中 `executionContext.soulRouting` 包含所有阶段绑定与来源。
6. 验证初始 LLM Step 使用 `planning` Soul。
7. 验证 Decision 场景使用 `decision` Soul。
8. 验证 Approval 场景使用 `approval` Soul。
9. 验证 DoD assessment 使用 `validation` Soul。
10. 验证 Result/Artifact 生成使用 `artifact` Soul。

覆盖优先级场景：

1. Workspace 配置 `planning = soul-planner`。
2. 启动 Run 时传 `soulBindings.planning = soul-run-planner`。
3. 验证本次 Run 的 `planning` 来源为 `run_stage_override`。
4. 再启动一个不传覆盖的 Run，验证 `planning` 来源恢复为 `workspace_stage_binding`。

失败场景：

- 配置不存在的 Soul，保存失败。
- Run 覆盖不存在的 Soul，启动失败。
- Soul 不允许本次 Skill/MCP/Tool 选择，启动失败。
- 未配置任何 Soul 时，Run 可执行，但 `soulRouting.status = "unbound"`。

## Implementation Plan

1. 模型与服务
   - 定义 `SoulBindingStage` 枚举和 `WorkspaceSoulBindings` DTO。
   - 在 Workspace 维度增加 Soul binding 读写能力。
   - 增加 Soul routing resolver，输出带来源的最终路由。

2. API
   - 增加 Workspace soul-bindings GET/PUT。
   - 扩展 activation request 支持 Run 级 `soulBindings`。

3. Execution Context
   - activation 时解析并写入 `executionContext.soulRouting`。
   - 保留 `executionContext.soulId` 作为 effective default。
   - 保留 `executionContext.soul` 作为 effective default Soul 摘要。

4. LLM Runtime
   - 在 LLM request context 中加入当前阶段、选中的 Soul 和 Soul 来源。
   - 不在 LLM runtime 查询 repository。

5. Web
   - Workspace Detail 增加执行人格配置。
   - Activation 弹窗增加本次执行默认 Soul 和阶段覆盖。
   - Run Detail 展示 Soul routing。

6. Verification
   - 单元测试覆盖 resolver 优先级。
   - HTTP 测试覆盖 GET/PUT 和 activation request。
   - e2e 覆盖多阶段 Soul 参与真实 Run。

## Acceptance Criteria

- Workspace 可以保存、读取默认 Soul 和阶段 Soul。
- Run activation 能解析并持久化 `soulRouting`。
- 启用优先级严格符合本文定义。
- 不存在或不允许的 Soul 在 Run 创建前失败。
- LLM Step 证据中能看到当前阶段实际 Soul。
- Web 能配置 Workspace Soul bindings 并查看 Run 实际 Soul routing。
- 新能力通过 mock e2e 和真实 LLM e2e 验收。
