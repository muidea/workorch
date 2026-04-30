# MCP Capability 管理设计

## 背景

当前 workorch 已支持 MCP Server 管理，并能通过 `allowedTools`、
`allowedResources`、`allowedPrompts` 暴露 tool / resource / prompt 白名单。
该实现可以满足运行时策略校验，但在 Web 管理和业务使用时仍存在识别成本：

- `repo.summary` 这类 tool 技术标识对用户不够直观。
- `workspace://acceptance` 这类 resource URI 能定位资源，但不能说明业务用途。
- `workorch.acceptance` 这类 prompt 名称能被系统引用，但用户难以判断适用阶段。
- 当前 Web 主要按 MCP Server 展示三类字符串列表，缺少统一能力目录和场景化筛选。

本设计目标是在不改变 workorch 业务边界的前提下，把 MCP tool / resource /
prompt 从“技术白名单字符串”推进为可识别、可管理、可绑定、可观测的正式能力项。

## 跟踪状态

| 项目 | 内容 |
| --- | --- |
| 需求主题 | MCP `tool` / `resource` / `prompt` 统一识别、管理和使用 |
| 当前状态 | 当前范围已完成实现 |
| 适用阶段 | MCP 能力管理和 Workspace 能力绑定 |
| 主要入口 | 运行控制、Workspace 启动执行、运行证据 |
| 主要业务模型 | `MCPServer`、`MCPCapability`、`Skill`、`Soul`、`Run` |
| 验收方式 | 单元测试、HTTP API 测试、部署实例 e2e、Web 交互验收 |

当前开发进度：

- completed: `MCP-CAP-01` 正式能力模型。
- completed: `MCP-CAP-02` memory/file capability repository 与仓储契约测试。
- completed: `MCP-CAP-03` MCP Server 创建或更新后派生默认 capability。
- completed: `MCP-CAP-04` capability appservice 与 HTTP API。
- completed: `MCP-CAP-05` Web MCP 能力目录支持统一列表、筛选和详情编辑。
- completed: `MCP-CAP-06` Workspace 启动执行支持 capability 选择，并写入 Run execution context。
- completed: `MCP-CAP-07` Skill required capability refs 已进入正式模型和启动校验。
- completed: `MCP-CAP-08` Soul allowed capability refs/categories 已进入正式模型和启动校验。
- completed: `MCP-CAP-09` Run snapshot / observability 已暴露 selected / used capability refs。
- completed: `MCP-CAP-10` 部署实例 e2e 已覆盖 capability 元数据更新、启动选择和运行证据。

## 目标

- 用户能通过业务名称、说明、场景、阶段、风险等级识别 capability。
- 仍保留技术 ID / URI / name，满足运行时调用、审计和排障。
- MCP Server 继续负责连接、会话和默认策略；Capability 负责表达具体可用能力。
- Workspace、Soul、Skill、Run 启动和运行证据都可以引用具体 capability。
- Web 端提供统一能力目录，而不是仅在某个 Server 下查看三段字符串。

## 非目标

- 不让 kernel/orchestrator 根据具体业务目标硬编码选择某个 tool。
- 不让 LLM 直接绕过策略调用 MCP tool / resource / prompt。
- 不把 prompt 文本或 soulMd 文本作为能力授权来源。
- 不在本阶段实现外部 marketplace 或远程能力安装治理。
- 不在本阶段实现外部 MCP discovery 协议接入。

## 核心概念

### MCPServer

`MCPServer` 表示外部能力提供方和连接策略。

字段保持偏技术和运行时：

- `id`
- `name`
- `transport`
- `endpoint`
- `enabled`
- `sessionPolicy`
- `defaultApprovalPolicy`
- `timeoutMillis`

### MCPCapability

`MCPCapability` 表示 MCP Server 暴露出来的单个可用能力。

当前实现模型：

```ts
type MCPCapability = {
  id: string;
  serverId: string;
  kind: 'tool' | 'resource' | 'prompt';

  name: string;
  displayName: string;
  description: string;
  category?: string;
  tags?: string[];

  usageHint?: string;
  inputSummary?: string;
  outputSummary?: string;
  stageHints?: string[];
  riskLevel?: 'read_only' | 'write' | 'external';

  schema?: Record<string, unknown>;
  uri?: string;
  enabled: boolean;
  approvalPolicy?: string;
  status?: string;
};
```

字段语义：

- `id`: workorch 内部稳定引用，例如 `mcp-local-context/tool/repo.summary`。
- `name`: MCP 原始技术标识，例如 `repo.summary`。
- `displayName`: 面向用户的业务名称，例如 `仓库摘要`。
- `description`: 说明能力做什么，以及何时使用。
- `category`: 面向业务的分类，例如 `代码仓库`、`验收`、`文档`。
- `tags`: 搜索和自动匹配辅助标签。
- `usageHint`: 操作使用建议。
- `inputSummary`: 用户可理解的输入要求摘要。
- `outputSummary`: 用户可理解的输出内容摘要。
- `stageHints`: 适用阶段，例如 `planning`、`execution`、`validation`、`acceptance`。
- `riskLevel`: 能力风险级别，用于展示和审批默认值。
- `schema`: tool input、prompt args 或 resource metadata 的结构化 schema。
- `uri`: resource 的 URI；tool / prompt 可为空。

## 示例映射

| kind | 技术标识 | 用户显示名 | 说明 |
| --- | --- | --- | --- |
| `tool` | `repo.summary` | 仓库摘要 | 获取代码仓库结构、关键文件和当前变更概览 |
| `resource` | `workspace://acceptance` | 工作空间验收资料 | 读取当前 Workspace 的验收标准、结果和证据 |
| `prompt` | `workorch.acceptance` | 验收提示词 | 用于生成或复核 DoD 验收结论 |

对应 capability 示例：

```json
{
  "id": "mcp-local-context:tool:repo.summary",
  "serverId": "mcp-local-context",
  "kind": "tool",
  "name": "repo.summary",
  "displayName": "仓库摘要",
  "description": "获取代码仓库结构、关键文件和当前变更概览。",
  "category": "代码仓库",
  "tags": ["repo", "summary", "context"],
  "usageHint": "适用于规划或排查前快速了解代码上下文。",
  "inputSummary": "可选仓库路径、分支或文件范围。",
  "outputSummary": "返回仓库结构、关键文件、最近变更和风险摘要。",
  "stageHints": ["planning", "execution"],
  "riskLevel": "read_only",
  "enabled": true
}
```

```json
{
  "id": "mcp-local-context:resource:workspace:%2F%2Facceptance",
  "serverId": "mcp-local-context",
  "kind": "resource",
  "name": "workspace://acceptance",
  "displayName": "工作空间验收资料",
  "description": "读取当前 Workspace 的验收标准、正式结果、Artifact 和运行证据摘要。",
  "category": "验收",
  "tags": ["workspace", "acceptance", "dod", "artifact"],
  "usageHint": "适用于验收、复核和交付物确认阶段。",
  "outputSummary": "返回 DoD、Result、Artifact 和证据引用。",
  "stageHints": ["validation", "acceptance"],
  "riskLevel": "read_only",
  "enabled": true
}
```

```json
{
  "id": "mcp-local-context:prompt:workorch.acceptance",
  "serverId": "mcp-local-context",
  "kind": "prompt",
  "name": "workorch.acceptance",
  "displayName": "验收提示词",
  "description": "将 Goal、DoD、运行证据和交付物整理为验收判断提示。",
  "category": "验收",
  "tags": ["prompt", "acceptance", "dod"],
  "usageHint": "适用于生成或复核 DoD 验收结论。",
  "inputSummary": "需要 Goal、DoD、Result、Artifact 和 Evidence refs。",
  "outputSummary": "生成结构化验收判断、风险和补充建议。",
  "stageHints": ["validation", "acceptance"],
  "riskLevel": "read_only",
  "enabled": true
}
```

## Web 交互设计

### MCP 服务管理

保留 MCP Server 管理页，但将职责聚焦为连接和运行策略：

- 服务 ID、名称、传输方式、端点。
- 启用状态。
- 会话策略。
- 默认审批策略。
- 超时配置。
- 能力数量摘要。

不建议继续把全部 tool / resource / prompt 当作三段 TextArea 的核心管理方式。
TextArea 可以作为早期快速录入或调试入口，但正式管理应进入 capability 目录。

### Capability 目录

新增统一目录视图，建议字段：

- 显示名称。
- 类型：Tool / Resource / Prompt。
- 技术标识。
- 所属 MCP Server。
- 分类。
- 标签。
- 适用阶段。
- 风险等级。
- 启用状态。

筛选能力：

- 按 `kind` 筛选。
- 按 MCP Server 筛选。
- 按业务分类筛选。
- 按阶段筛选。
- 按风险等级筛选。
- 搜索匹配 `displayName`、`name`、`description`、`tags`。

### Capability 详情

详情页或侧边抽屉展示：

- 用户显示名和描述。
- 技术标识和 server ID。
- 输入摘要、输出摘要、使用建议。
- schema 或 URI。
- 审批策略。
- 关联 Skill / Soul 策略引用。

### 业务选择入口

用户不应优先看到 `repo.summary` 这类裸 ID，而应看到：

```text
仓库摘要
repo.summary · Tool · mcp-local-context
获取代码仓库结构、关键文件和当前变更概览
```

选择入口包括：

- Workspace 启动执行：可按 Server 粗粒度选择，也可展开选择具体 capability。
- Soul 管理：可限制允许使用的 capability 或 capability 分类。
- Skill 管理：可声明需要的 capability。
- 运行证据：展示实际使用的 capability displayName，同时保留技术标识。

## 运行时展示

运行过程和证据不应只显示 `mcp/tool_call`、`repo.summary`。
建议展示：

```text
工具调用：仓库摘要
技术标识：repo.summary
所属服务：mcp-local-context
用途：为本轮任务收集仓库结构和变更上下文
结果：已读取 12 个关键文件，发现 3 个相关模块
```

resource 示例：

```text
资源读取：工作空间验收资料
资源 URI：workspace://acceptance
用途：读取 DoD、Result、Artifact 和证据引用
结果：读取 4 个验收项、1 个正式结果、2 个交付物
```

prompt 示例：

```text
提示词：验收提示词
技术标识：workorch.acceptance
用途：生成 DoD 验收判断
结果：生成结构化验收结论和待补充信息
```

## 业务边界

| 模块 | 负责 | 不负责 |
| --- | --- | --- |
| `kernel/mcp` | MCP Server 和 MCPCapability 元数据、能力启用状态、策略字段 | 执行 MCP 协议调用 |
| `kernel/skill` | Skill 声明 required capabilities | 直接执行 tool/resource/prompt |
| `kernel/soul` | Soul 限制 allowed capability / category / server | 从 soulMd 隐式授权 |
| `kernel/orchestrator` | 根据 Run context 和策略形成候选能力集合，调度 block | 根据具体业务文本硬编码选择工具 |
| `blocks/mcp` | 执行已批准的 tool call、resource read、prompt get，并归一化输出 | 绕过策略选择能力 |
| `application/workorchd` | API/Web 管理、执行激活、读模型聚合 | 直接执行 MCP 协议 |
| `observability/result` | 记录实际能力使用证据和结果引用 | 把原始 tool 输出直接当最终业务结果 |

边界约束：

- `kernel/orchestrator` 只能基于 Run context、Skill/Soul/Workspace 策略和能力元数据形成候选集合，不允许根据某个具体业务目标硬编码工具。
- `blocks/mcp` 只能执行已经被策略允许并完成审批的 capability，不负责选择业务能力。
- `Skill` 可以声明所需 capability，但不能绕过 MCP Server 启用状态和审批策略。
- `Soul` 可以约束允许的 capability、category 或 server，但 `soul.md` 文本本身不能隐式授予权限。
- `Run Snapshot` 和 observability 只展示 selected / used capability，不反向修改 MCP 能力配置。

## 数据关系

```text
MCPServer
  -> MCPCapability(tool/resource/prompt)

Skill
  -> requiredCapabilityRefs

Soul
  -> allowedCapabilityRefs / allowedCapabilityCategories / allowedMCPServerRefs

Workspace / Run activation
  -> selectedCapabilityRefs

Run Snapshot / Observability
  -> usedCapabilityRefs
  -> ToolCall / RuntimeCheckpoint / EventRecord evidence
```

## 实施拆解

| 编号 | 工作项 | 交付物 | 验收标准 |
| --- | --- | --- | --- |
| MCP-CAP-01 | 新增正式能力模型 | `MCPCapability` 模型、字段校验、ID 生成规则 | 能表达 tool/resource/prompt，能保留技术标识和用户显示信息 |
| MCP-CAP-02 | 新增能力仓储 | memory/file repository 和测试 | file backend 重启后 capability 元数据不丢失 |
| MCP-CAP-03 | MCP Server 到能力派生 | 从 `allowedTools` / `allowedResources` / `allowedPrompts` 生成默认 capability | 新建或更新 server 后能看到默认能力项 |
| MCP-CAP-04 | 应用服务与 HTTP API | capability CRUD、列表筛选、详情读取 | API 支持 kind/server/category/stage/risk/enabled/q 查询 |
| MCP-CAP-05 | Web 能力目录 | 统一能力目录、筛选、详情编辑 | 用户能用中文名称识别 `repo.summary` 等能力 |
| MCP-CAP-06 | Workspace 使用入口 | 启动执行支持选择 capability | Run context 持久化 selected capability refs |
| MCP-CAP-07 | Skill / Soul 绑定 | Skill required capability refs，Soul allowed refs/categories | 启动时能自动校验、绑定或拒绝不允许能力 |
| MCP-CAP-08 | Soul 能力边界 | Soul allowed refs/categories | 启动时拒绝未授权能力 |
| MCP-CAP-09 | 运行证据增强 | Run snapshot / observability 暴露 selected / used capability | 证据同时显示业务名称和技术标识 |
| MCP-CAP-10 | 验证覆盖 | 单元测试、API 测试、部署实例 e2e | 覆盖 tool/resource/prompt 的管理、选择、执行和证据链 |

## API

当前在现有 MCP Server API 之外提供统一能力读写 API：

```text
GET  /api/v1/workorch/mcp/capabilities
GET  /api/v1/workorch/mcp/capabilities/{capabilityId}
POST /api/v1/workorch/mcp/capabilities
PUT  /api/v1/workorch/mcp/capabilities/{capabilityId}
```

查询参数：

- `serverId`
- `kind`
- `category`
- `stage`
- `riskLevel`
- `enabled`
- `q`

与现有 API 的关系：

- `GET /mcp/servers/{serverId}/tools` 继续作为 Server 维度轻量读模型。
- 新 Web 页面优先使用 `/mcp/capabilities`。
- Server 创建时仍可填写 `allowedTools`、`allowedResources`、`allowedPrompts`，
  服务端会生成默认 capability，用户后续补 displayName / description。

## 已落地路径

当前实现按以下路径收口：

1. 引入 `MCPCapability` 模型、仓储和 appservice。
2. 从 MCP Server 的 `allowedTools` / `allowedResources` / `allowedPrompts`
   自动派生默认 capability。
3. Web 增加 Capability 目录和详情编辑。
4. Workspace 启动执行支持 `capabilityIds`，同时保留 `mcpServerIds` 粗粒度选择。
5. Skill 增加 required capability refs。
6. Soul 增加 allowed capability refs / categories。
7. Run context 和 Snapshot 暴露 selected / used capability refs。
8. 运行证据中按 capability displayName 展示工具、资源和提示词。

## 验收核对表

### Web 验收

- [x] 用户能在 Web 上看到统一的 MCP Capability 目录。
- [x] `repo.summary` 显示为 `仓库摘要`，并展示技术标识、说明、输入输出摘要和风险等级。
- [x] `workspace://acceptance` 默认显示为 `工作空间验收资料`。
- [x] `workorch.acceptance` 默认显示为 `验收提示词`。
- [x] MCP Server 页面只承担连接和默认策略管理，能力明细进入统一目录查看和编辑。
- [x] Workspace 启动执行时，可以选择 MCP Server，也可以选择具体 capability。

### 运行时验收

- [x] Skill 可以声明 required capability，Workspace 启动时自动校验或绑定。
- [x] Soul 可以限制 allowed capability，拒绝未授权能力。
- [x] Run Snapshot 能展示 selected capabilities 和实际 used capabilities。
- [x] 运行过程与证据显示用户可理解名称，同时保留技术 ID。
- [x] Tool 执行证据仍遵守现有审批、策略和边界约束。
- [x] 未授权 capability 有明确失败原因。

### 数据验收

- [x] memory backend 和 file backend 的 capability 行为一致。
- [x] file backend 重启后 MCP Server、Capability、Workspace 绑定关系不丢失。
- [x] 能力目录搜索同时覆盖 `displayName`、`name`、`description`、`tags`。

## 交付清单

| 编号 | 状态 | 说明 |
| --- | --- | --- |
| MCP-CAP-01 | completed | 新增 `MCPCapability` 正式业务模型 |
| MCP-CAP-02 | completed | 新增 capability repository memory/file 实现 |
| MCP-CAP-03 | completed | MCP Server 创建或更新后自动生成默认 capability |
| MCP-CAP-04 | completed | 新增 capability appservice 和 HTTP API |
| MCP-CAP-05 | completed | Web Runtime Control 增加 Capability 目录、筛选和详情编辑 |
| MCP-CAP-06 | completed | Workspace 启动执行增加 capability 选择 |
| MCP-CAP-07 | completed | Skill required MCP 能力从 server refs 扩展到 capability refs |
| MCP-CAP-08 | completed | Soul capability 策略从 server/tool kind 扩展到 capability/category |
| MCP-CAP-09 | completed | Run context、snapshot、observability 暴露 selected / used capability refs |
| MCP-CAP-10 | completed | e2e 覆盖 capability 派生、元数据编辑、选择执行和证据链 |

## 验证命令建议

开发完成后至少执行：

```bash
make fmt-check
make vet
make test
make console-build
make build
make verify-deployed-full
```

说明：

- `make verify-deployed-full` 仍应验证当前部署实例，不由脚本自行拉起内部服务。
- 真实 LLM 相关用例只在本地部署验收中执行，CI 应继续使用 mock LLM，避免外部网络或额度导致误报。
- Web 验收需要覆盖能力目录、MCP Server 页面、Workspace 启动执行、运行证据四个入口。

## 后续候选

- Capability 删除策略：是否允许物理删除，还是仅允许禁用并保留历史引用。
- Workspace 模板 capability 预设：是否支持必选 / 推荐 / 可选三种级别。
- SkillHub 管理的 Skill capability 声明：使用 workorch 本地映射，还是扩展 skill 元数据。
- Capability 最近使用记录：直接从 event stream / observability 推导，还是引入独立读模型缓存。
- Resource / Prompt 执行证据 e2e：在已有派生和展示基础上补充实际运行验证。
- 未启用 capability 与审批拒绝 capability 的错误诊断验收。
