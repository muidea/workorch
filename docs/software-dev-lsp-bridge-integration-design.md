# Software Dev LSP Bridge Integration Design

## 1. 目标

本文定义 WorkOrch 在 `software-dev` workspaceTemplate 中集成 `/home/rangh/aispace/lsp-bridge/docs/integration.md` 的方案。

目标是让软件开发类 Workspace 在规划、执行、修复和验证阶段可以使用 LSP 语义能力，包括：

- 初始化项目语言服务。
- 查询定义、引用、hover 信息。
- 修改后同步内存 buffer。
- 获取文件级诊断。
- 将诊断结果作为修复和验证证据的一部分。

该能力用于提升 LLM 对代码结构、符号关系、编译诊断和局部影响面的判断质量，避免只依赖全文读取、grep 或测试失败输出来定位问题。

## 2. 基础约束

WorkOrch core 必须保持业务中立。

因此本次集成必须遵守：

- `lsp-bridge` 不进入 core runtime 的硬编码流程。
- core 不内置 Go、Python、TypeScript、Rust、Shell 等语言规则。
- core 不根据 workspace 类型主动推断必须使用 LSP。
- `lsp-bridge` 只作为 MCP server / capability 被 workspaceTemplate、Soul、Skill 或用户选择绑定。
- 语言服务器命令、启用语言、初始化策略、诊断使用方式属于 profile/template 配置，不属于 core 逻辑。
- LSP 诊断是辅助证据，不能替代 build/test/lint/DoD evidence gate。

## 3. 集成定位

### 3.1 推荐形态

`lsp-bridge` 应作为 `software-dev` 的可选 MCP server 能力接入。

推荐抽象：

```text
workspaceTemplate(software-dev)
  -> mcpServer: lsp-bridge
  -> capabilities:
       lsp_initialize
       lsp_sync
       lsp_definition
       lsp_hover
       lsp_references
       lsp_diagnostics
  -> Soul guidance:
       planning/execution/repair/validation 按需使用
```

### 3.2 不推荐形态

以下方式不采用：

- 在 orchestrator 中固定调用 `lsp_initialize`。
- 在 patch/apply 后由 core 自动调用 `lsp_sync`。
- 将 LSP 诊断直接作为 DoD satisfied 的充分条件。
- 将 `gopls`、`pyright` 等具体语言服务器写入 core。
- 把 LSP 查询失败提升为通用 runtime failure。

## 4. MCP Server 配置

`lsp-bridge` 使用 stdio transport。

### 4.1 安装与运行路径

根据最新 `lsp-bridge/docs/integration.md`：

- 在线安装脚本未指定安装目录时，默认安装到 `$HOME/.local`。
- 二进制默认落在 `$HOME/.local/bin/lsp-bridge`。
- 如需固定安装目录，可通过 `LSP_BRIDGE_INSTALL_DIR` 指定，例如 `$HOME/lsp-bridge-install`。
- 临时环境可设置：

```bash
export LSP_BRIDGE_HOME="$HOME/.local"
export PATH="$LSP_BRIDGE_HOME/bin:$PATH"
```

WorkOrch 不负责在线安装 `lsp-bridge` 或语言服务器；部署环境必须提前准备可执行命令或绝对路径。

### 4.2 全局默认与 Workspace 私有配置

`lsp-bridge` 配置采用“全局默认 + workspace 私有有效配置”的混合模型。

全局配置适合承载：

- `lsp-bridge` 二进制路径或命令名。
- 常用语言服务器默认安装路径。
- 内置 `mcp-lsp-bridge` server / capability 默认定义。
- workspaceTemplate 默认语言配置片段。

workspace 私有配置适合承载：

- 当前 workspace 实际使用的 `mcp-config.json`。
- 当前 workspace 启用语言、语言服务器命令和环境变量。
- 与当前项目依赖、工作目录、索引状态相关的 LSP 配置。

推荐路径：

```text
.workorch-data/internal/workspaces/{workspaceId}/lsp-bridge/mcp-config.json
```

该路径属于 WorkOrch 内置管理目录，不属于 workspace 交付物目录，不应写入用户项目源码。

配置优先级：

1. workspace 私有 `mcp-config.json`。
2. workspaceTemplate 默认配置。
3. 全局默认配置。
4. 仍缺失时将 LSP capability 标记为 unavailable，不阻塞普通软件开发流程。

运行时应将 workspace 私有配置绑定到 MCP server env：

```json
{
  "env": {
    "LSP_BRIDGE_CONFIG": ".workorch-data/internal/workspaces/{workspaceId}/lsp-bridge/mcp-config.json"
  }
}
```

如果使用 long-lived stdio session，session key 不能只使用 `serverId`。至少应包含：

```text
workspaceId + serverId
```

否则不同 workspace 可能复用同一个 `lsp-bridge` 进程，导致 root、语言、依赖索引、诊断状态和内存 buffer 串扰。

### 4.3 Server 字段映射

`lsp-bridge` 原始 MCP Client 示例使用 `command` / `args` / `env`：

```json
{
  "command": "lsp-bridge",
  "args": [],
  "env": {
    "LSP_BRIDGE_CONFIG": "./mcp-config.json"
  }
}
```

WorkOrch 当前 `MCPServer` 模型使用：

```text
ID, Name, Transport, Endpoint, Enabled, AllowedTools, AllowedResources,
AllowedPrompts, DefaultApprovalPolicy, SessionPolicy, TimeoutMillis
```

当前阶段不考虑兼容处理，但仍要求保留 `endpoint`，因为它已经可以表达现有 HTTP endpoint 和部分 stdio command line。最终 stdio 解析规则如下：

- `transport=http/https`：必须使用 `endpoint`。
- `transport=stdio`：
  1. 如果指定了 `endpoint`，优先以 `endpoint` 为准，并按现有 stdio endpoint command line 解析。
  2. 如果 `endpoint` 为空，则使用 `command + args + env`，并通过 `exec.Command(command, args...)` 直接启动。
  3. 如果 `endpoint`、`command` 都为空，则返回明确配置错误：stdio MCP server requires endpoint or command。

推荐 server 定义：

```json
{
  "id": "mcp-lsp-bridge",
  "name": "LSP Bridge",
  "transport": "stdio",
  "command": "lsp-bridge",
  "args": [],
  "env": {
    "LSP_BRIDGE_CONFIG": "./mcp-config.json"
  },
  "enabled": true,
  "sessionPolicy": "long_lived",
  "timeoutMillis": 30000,
  "allowedTools": [
    "lsp_initialize",
    "lsp_sync",
    "lsp_definition",
    "lsp_hover",
    "lsp_references",
    "lsp_diagnostics"
  ]
}
```

如果需要继续使用 `endpoint`，仍然允许：

```json
{
  "endpoint": "$HOME/.local/bin/lsp-bridge"
}
```

实际落库时建议写入已展开的绝对路径，避免 runtime 对 `$HOME`、`~` 或 shell 语法产生隐式解析依赖。

但当 `endpoint` 与 `command/args/env` 同时存在时，runtime 必须优先使用 `endpoint`。这保证显式 endpoint 可以覆盖结构化 command 配置，便于部署侧使用 wrapper、绝对路径或已有配置入口。

WorkOrch 不负责在线安装语言服务器。部署环境可以按需准备：

| Language | Default LSP command |
| --- | --- |
| Go | `gopls serve` |
| Rust | `rust-analyzer` |
| Python | `pyright-langserver --stdio` |
| TypeScript/JavaScript | `typescript-language-server --stdio` |
| Shell | `bash-language-server start` |

## 5. Capability 模型

建议登记为 MCP tool capability：

| Capability ID | Tool | Purpose | Stage |
| --- | --- | --- | --- |
| `mcp-lsp-bridge:tool:lsp_initialize` | `lsp_initialize` | 初始化项目语言服务 | planning/execution |
| `mcp-lsp-bridge:tool:lsp_sync` | `lsp_sync` | 同步修改后的完整文件内容 | execution/repair |
| `mcp-lsp-bridge:tool:lsp_definition` | `lsp_definition` | 查询符号定义 | planning/repair |
| `mcp-lsp-bridge:tool:lsp_hover` | `lsp_hover` | 查询符号说明 | planning/repair |
| `mcp-lsp-bridge:tool:lsp_references` | `lsp_references` | 查询引用和影响面 | planning/repair |
| `mcp-lsp-bridge:tool:lsp_diagnostics` | `lsp_diagnostics` | 获取文件诊断 | execution/validation/repair |

## 6. 阶段使用合同

### 6.0 通用调用约束

根据最新 `llm-system-prompt.md`，所有阶段使用 LSP 工具时必须遵守：

- 不要在未 `lsp_initialize` 的情况下请求语义信息。
- `line` / `col` 均为 0-based。
- 对 workspace-scoped stdio MCP server，WorkOrch runtime 以 workspace 交付目录作为子进程工作目录；`lsp_initialize.root_path` 可以使用 `.`，后续 `path` / `file_path` 使用 workspace 相对路径。
- `lsp_definition`、`lsp_hover`、`lsp_references` 返回空不等于符号不存在，可能是索引未完成、位置不在符号上或语言服务器依赖缺失。
- LSP 子进程错误时，优先重新 `lsp_initialize` 或稍后有界重试。
- `lsp_references` 默认最多返回 10 条；`truncated=true` 时只能把返回项作为局部引用集合，不得声称已覆盖所有引用。
- `lsp_sync` 必须传入修改后的完整文件内容，并提供对应的 `path` 或 `file_path`；不能只传 `root_path`，也不能传 partial diff。

### 6.1 Planning

允许：

- 根据文件清单、`go.mod`、`package.json`、`pyproject.toml`、`Cargo.toml` 等判断候选语言。
- 调用 `lsp_initialize` 初始化相关语言。
- 调用 `lsp_definition` / `lsp_references` / `lsp_hover` 辅助定位入口、接口和影响面。
- 对多语言仓库按语言分别初始化。

禁止：

- 只因 LSP 不可用就阻塞整个 Run。
- 用 LSP 结果替代 Goal/DoD gap analysis。
- 把 LSP 空结果解释为“项目无该符号”或“无需修改”。

### 6.2 Execution

允许：

- 在生成 patch 前通过 definition/references 辅助定位目标。
- 当文件内容已通过 patch 落盘后，可以直接用文件路径进行后续 LSP 查询；当 runtime 或工具链存在未落盘内容时，必须使用 `lsp_sync` 同步完整文件内容。
- patch 后调用 `lsp_diagnostics` 快速发现语法、类型、未定义符号等问题。

禁止：

- 用 LSP 修改文件。文件状态变更仍必须通过 patch/apply 或其他明确 state-changing tool 完成。
- 因 LSP 无诊断就直接认为实现满足 DoD。
- 用 partial content 调用 `lsp_sync`。

### 6.3 Repair

允许：

- 针对 build/test/lint 失败中的符号、类型、接口问题，使用 LSP 查询 definition/references。
- 使用 diagnostics 将错误定位收敛到具体文件和行列。
- 将 LSP 诊断作为 repair input 的辅助上下文。
- 如果 LSP 子进程错误，允许一次有界重新初始化后重试当前查询。

禁止：

- 在 LSP 查询为空时扩散为无限探索。
- 把历史 diagnostics 覆盖当前 build/test 失败。
- 在 `lsp_references.truncated=true` 时声称已完成全量影响面分析。

### 6.4 Validation

允许：

- 在 build/test/lint 前后调用 diagnostics 作为辅助质量检查。
- 将 diagnostics 结果作为 validation evidence 的补充说明。

禁止：

- 仅凭 diagnostics clean 标记 DoD satisfied。
- 用 diagnostics 替代项目约定的 build/test/lint 命令。

## 7. Context 打包规则

LSP 输出进入 LLM 上下文时必须压缩为结构化摘要，避免扩大噪点。

推荐格式：

```json
{
  "lspContext": {
    "initializedLanguages": ["go"],
    "diagnostics": [
      {
        "path": "proxy/proxy.go",
        "line": 42,
        "col": 13,
        "severity": "error",
        "source": "compiler",
        "message": "undefined: Foo"
      }
    ],
    "symbolRefs": [
      {
        "query": "HandleStream",
        "kind": "references",
        "items": [
          {"path": "proxy/proxy.go", "line": 88, "col": 6}
        ],
        "total": 3,
        "truncated": false
      }
    ]
  }
}
```

上下文保留规则：

- 保留当前失败相关 diagnostics。
- 保留本轮 patch 影响文件的 diagnostics。
- 保留当前 repair target 的 definition/references 摘要。
- 保留 `lsp_references.total`、`limit`、`truncated`，防止 LLM 把局部引用误判为全量引用。
- 丢弃与当前 Goal/DoD、当前失败、当前 patch target 无关的 hover 长文本。
- 不把完整源码通过 LSP 输出重复注入上下文。

## 8. Evidence 规则

LSP evidence 的证据强度定义：

| Evidence | Strength | Can satisfy DoD alone |
| --- | --- | --- |
| `lsp_diagnostics` clean | auxiliary | No |
| `lsp_definition` / `lsp_references` | auxiliary | No |
| `lsp_sync` success | auxiliary | No |
| build/test/lint command success | direct | Yes, when mapped to DoD |
| patch/apply state change | direct state-change | Yes, with validation |

因此 DoD assessment 中：

- 可以引用 LSP tool call 作为辅助 evidenceRef。
- 高风险维度仍必须引用 shell/command、patch/apply、mcp/tool_call 等直接动作证据。
- LSP 诊断失败应进入 repair context，而不是直接生成 accepted result。

## 8.1 Post-action capability policy contract

`capabilityUsagePolicies` 不是只给 LLM 阅读的提示信息。它们是由 workspaceTemplate / profile 声明、由 runtime 以中立方式校验的能力使用合同。

Core runtime 只能理解以下通用字段：

- `id`
- `scope`
- `appliesToCategory`
- `usageMode`
- `triggerStages`
- `triggerSignals`
- `preferredTools`
- `requiredAfterKinds`
- `fallbackPolicy`
- `evidenceStrength`

Core runtime 不得识别 `lsp-bridge`、`gopls`、`software-dev` 或任何具体业务名称。具体 MCP server、tool 名称、语言策略和降级策略只能来自 template / soul / capability catalog。

当某个 policy 声明 `requiredAfterKinds`，并且当前 execution graph 中出现对应 step kind 时，runtime 必须执行以下中立门禁：

1. 若当前 run 存在匹配 `appliesToCategory` 且启用的 MCP capability，则该 graph 必须包含一个匹配 policy 的 `mcp/tool_call` follow-up node。
2. follow-up node 必须依赖触发 `requiredAfterKinds` 的 node，避免与 patch 并发执行造成顺序错误。
3. 若本轮确实不应或不能调用该能力，LLM 输出必须包含结构化 `capabilityPolicySkips`，至少声明 `policyId` 或 `category` 以及非空 `reason`。
4. 若没有匹配 category 的可用 capability，runtime 不强行阻塞，但后续可观测信息应能显示该能力未绑定或不可用。

推荐 `capabilityPolicySkips` 结构：

```json
{
  "capabilityPolicySkips": [
    {
      "policyId": "software-dev-code-semantics-guidance",
      "category": "code_semantics",
      "reason": "capability_unavailable",
      "fallbackPolicy": "fallback_to_file_read_and_build_output_without_blocking"
    }
  ]
}
```

对 `software-dev` 来说，`patch/apply` 后若 `code_semantics` 能力可用，应优先安排 `lsp_sync` 和/或 `lsp_diagnostics`。但这是 template/capability catalog 的策略，不是 core runtime 的业务逻辑。

## 9. Template / Soul 改动方向

### 9.1 software-dev workspaceTemplate

建议补充：

- 默认可选 MCP server：`mcp-lsp-bridge`。
- 默认可选 capabilities：`lsp_initialize`、`lsp_sync`、`lsp_definition`、`lsp_hover`、`lsp_references`、`lsp_diagnostics`。
- contextHints 中加入 `semanticCodeNavigation`，声明这是辅助能力。

默认语言识别策略放在 `software-dev` template / Soul，不进入 core。推荐候选规则：

| Signal | Candidate language |
| --- | --- |
| `go.mod` | `go` |
| `package.json` / `tsconfig.json` | `typescript` / `javascript` |
| `pyproject.toml` / `requirements.txt` | `python` |
| `Cargo.toml` | `rust` |
| `*.sh` / shellcheck 配置 | `shell` |

这些规则只用于生成候选语言初始化计划。识别失败不阻塞 run；多语言仓库可以初始化多个语言。

### 9.2 software-dev Planning Soul

补充：

- 当目标涉及已有代码修改、符号关系、接口实现或跨文件影响面时，优先考虑使用 LSP 查询辅助定位。
- 多语言项目需要按语言分别初始化。
- LSP 失败时回退到 file/read、rg、build/test 输出。

### 9.3 software-dev Execution Soul

补充：

- patch 后对已修改文件执行 `lsp_sync`。
- 对当前修改目标执行 `lsp_diagnostics`，发现语法/类型问题时优先修复。

### 9.4 software-dev Repair Soul

补充：

- 对 undefined、missing method、type mismatch、interface mismatch、unused import 等问题，优先使用 diagnostics + definition/references 定位根因。
- repair 必须以当前失败为准，LSP 仅辅助定位。

### 9.5 software-dev Validation Soul

补充：

- diagnostics clean 只能作为辅助质量信号。
- build/test/lint 仍是 DoD 验收主证据。

## 10. Failure Handling

LSP 类错误分级：

| Error | Runtime handling |
| --- | --- |
| server command missing | capability unavailable，降级为普通文件/命令流程 |
| language server missing | language capability unavailable，记录 tool_feedback |
| initialize timeout | retry bounded once，仍失败则跳过该辅助能力 |
| diagnostics timeout | 不阻塞 build/test/lint |
| query empty | 作为空结果，不进入 blocked_on_decision |
| sync failed | 记录 tool failure，并回退到磁盘文件状态 |
| workspace private config missing | 降级为 template/global default；仍不可用则 capability unavailable |
| long-lived session workspace mismatch | 必须关闭旧 session 并按 workspace 维度重建 |

只有当 Goal/DoD 明确要求使用 LSP 或语义索引能力时，LSP 不可用才可以成为 blocker。

## 11. Implementation Contract

### 11.1 Workspace 私有配置生成

配置生成采用：

```text
workspace 创建时生成默认私有配置
activation 时校验并按需补齐
```

规则：

- 创建 software-dev workspace 时，在 WorkOrch 内置管理目录创建 `lsp-bridge/mcp-config.json`。
- activation 时检查私有配置是否存在；缺失时按当前 template/global default 补齐。
- activation 不得无条件覆盖已有私有配置，避免误删用户或运行期写入的有效配置。
- 如果 template/default 增加了新语言配置，activation 可以只补缺失项，不覆盖已有项。
- 私有配置属于 runtime internal state，不进入 workspace 交付物目录，不作为 patch/apply 目标。

### 11.2 默认注册来源

`mcp-lsp-bridge` 的 server/capability 默认注册采用 bootstrap defaults。

推荐分层：

- bootstrap defaults 注册 `mcp-lsp-bridge` MCP server。
- bootstrap defaults 注册 LSP tool capabilities。
- `software-dev` template 通过 contextHints 暴露 `semanticCodeNavigation` 能力。
- `software-dev` Soul 指导 LLM 在需要语义定位时选择 LSP capability。

不建议把 LSP 作为某个 skill 的唯一 required MCP 入口。LSP 是软件开发通用辅助能力，应在 software-dev profile 中可见；skill 可以进一步声明更强的必需关系，但不能成为唯一暴露路径。

### 11.3 MCP Env 脱敏合同

`command/args/env` 是通用 MCP stdio 能力，不是 LSP 特例。`env` 可能包含 token、key、路径和本地敏感配置，因此必须按通用规则脱敏。

持久化规则：

- repository 可以保存 env 原值。
- env key 为空应拒绝。
- env value 允许为空字符串，表示显式设置为空。

展示和上下文规则：

- read-model / Web / TUI 默认展示脱敏值。
- LLM context 禁止注入 env 原值，只允许 key 名、是否存在值、是否 redacted。
- memory / artifact / result 不保存 env 原值。
- 运行日志和错误信息不输出 env 原值。

编辑语义：

- 未提交某个 env key：表示不修改。
- 提交 env key 且 value 为空字符串：表示设置为空值。
- 显式 delete env key：表示删除该 key。

建议按 key 名启发式脱敏：

```text
TOKEN, KEY, SECRET, PASSWORD, PASS, CREDENTIAL, AUTH
```

即使 key 不命中启发式规则，LLM context 仍不得包含 env 原值。

### 11.4 MCP stdio Session 生命周期

long-lived stdio session 必须按 workspace 隔离：

```text
sessionKey = workspaceId + serverId
```

若后续存在同一 workspace 内不同有效配置的并行使用，sessionKey 应进一步包含 server config digest。

workspace-scoped stdio session 启动时必须把子进程 cwd 设置为 workspace 交付目录。这样 LSP Bridge 这类工具收到 `root_path="."` 或 workspace 相对文件路径时，会解析到当前 workspace 交付物，而不是 WorkOrch 服务进程所在目录。该规则是通用 MCP stdio capability contract，不是 LSP 专用 core 逻辑。

关闭规则：

- WorkOrch shutdown 时关闭全部 stdio session。
- MCP server 配置变更后关闭旧 session。
- workspace 删除或清理时关闭该 workspace 对应 session。
- tool call 失败且判断 stdio 子进程不可用时关闭并驱逐该 session。

idle TTL 可以作为后续优化；如果第一轮不实现 TTL，必须至少实现 shutdown、配置变更和失败驱逐。

### 11.5 Web / TUI 配置交互

Web MCP server 配置页需要支持：

- `transport`
- `endpoint`
- `command`
- `args`
- `env`
- `sessionPolicy`
- `timeoutMillis`
- allowed tools/resources/prompts

交互规则：

- `transport=stdio` 时展示 `endpoint` 与 `command/args/env`，并说明优先级 `endpoint > command/args/env`。
- `args` 使用数组编辑，不使用 shell 字符串。
- `env` 使用 key/value 表格编辑。
- secret-like env value 默认脱敏展示。
- env 编辑必须支持“未修改 / 设置为空 / 删除”三态。

TUI 第一阶段可以只展示摘要：

- server id/name/transport/enabled。
- stdio 启动来源：endpoint 或 command。
- env keys 列表和 redacted 状态。

TUI 不要求第一阶段支持完整编辑。

### 11.6 LSP Evidence 与 DoD Gate

LSP tool call 可以作为辅助 evidenceRef，但不能单独支撑 high-risk DoD satisfied。

硬约束：

- `lsp_diagnostics` clean 不能替代 build/test/lint。
- `lsp_definition` / `lsp_references` 不能替代 state-changing evidence。
- `lsp_sync` success 不能证明 patch 已应用或行为已满足。
- 如果 DoD 涉及入口、协议、配置、副作用、并发、错误路径或用户可观察行为，仍必须引用直接 action evidence。

### 11.7 测试与验收用例

后续实现必须至少覆盖：

| Case | Expected |
| --- | --- |
| stdio server 同时配置 endpoint 和 command | runtime 使用 endpoint |
| stdio server endpoint 为空且 command 存在 | runtime 使用 command/args/env |
| stdio server endpoint 和 command 都为空 | 返回明确配置错误 |
| command/args/env 启动 stdio 进程 | env 正确注入子进程 |
| read-model 展示 MCP env | value 脱敏 |
| LLM context 注入 MCP server/capability | 不包含 env 原值 |
| env 编辑未提交某 key | 不修改原值 |
| env 编辑提交空字符串 | 设置为空值 |
| env 编辑显式 delete | 删除 key |
| long-lived stdio session 跨 workspace 调用同 serverId | 不复用同一 session |
| MCP server 配置变更 | 旧 session 被关闭 |
| LSP unavailable | software-dev 普通开发流程降级继续 |
| `lsp_references.truncated=true` | context 保留 truncated，不声称全量引用 |
| LSP clean 作为唯一证据 | DoD accepted 被拒绝 |

## 12. 实施清单

| ID | Item | Scope | Status |
| --- | --- | --- | --- |
| LSP-01 | 新增本文设计文档 | docs | Done |
| LSP-02 | 登记内置 `mcp-lsp-bridge` server 默认定义 | template/bootstrap | Done |
| LSP-03 | 登记 LSP MCP capabilities | template/bootstrap | Done |
| LSP-04 | software-dev template 增加 semanticCodeNavigation contextHints | template | Done |
| LSP-05 | Planning Soul 增加 LSP 使用约束 | template/soul | Done |
| LSP-06 | Execution Soul 增加 patch 后 sync/diagnostics 约束 | template/soul | Done |
| LSP-07 | Repair Soul 增加 diagnostics/definition/references 定位约束 | template/soul | Done |
| LSP-08 | Validation Soul 明确 LSP evidence 只能辅助 | template/soul | Done |
| LSP-09 | 扩展 MCP server generic `command/args/env`，并保留 `endpoint` 优先级：endpoint > command/args/env > explicit missing config error | mcp model/api/repository/web | Done |
| LSP-10 | 为 software-dev workspace 生成 workspace 私有 `lsp-bridge/mcp-config.json`，并放在 WorkOrch 内置管理目录 | workspace runtime/config | Done |
| LSP-11 | long-lived stdio MCP session key 增加 workspace 维度，避免跨 workspace 复用 | mcp runtime | Done |
| LSP-12 | LLM context package 支持 compact LSP evidence summary | orchestrator/read-model | Done：core 以通用 MCP tool evidence summary 压缩 `mcp/tool_call` 结果，保留 diagnostics/items/references/truncated 等结构化字段，不引入 LSP 专用逻辑 |
| LSP-13 | MCP tool failure 降级和 tool_feedback 记录 | orchestrator/mcp | Done：core 对通用 MCP server/tool 失败记录 bounded `tool_feedback`，并继续由 template/profile 决定是否降级、换能力或阻塞 |
| LSP-14 | 增加单测：software-dev 模板包含 LSP 可选能力且 core prompt 不含 LSP 专用逻辑 | test | Done：模板/default MCP/activation/runtime 已覆盖；`verify-boundary-contract.sh` 增加 core LSP profile-specific 字符串边界检查 |
| LSP-15 | 增加部署态验证：创建 software-dev workspace 并按需调用 lsp-bridge | e2e | Deferred：真实 `lsp-bridge` 与语言服务器依赖属于部署环境能力，不纳入 CI；部署态按本文和 `lsp-bridge/docs/integration.md` 单独验收 |
| LSP-16 | Web MCP server 配置支持 command/args/env 和 env 三态编辑/脱敏展示 | web | Done |
| LSP-17 | read-model / LLM context / memory / artifact 全链路 env 脱敏测试 | appservice/orchestrator/test | Done：activation/LLM context env 脱敏已覆盖；runtime memory 写入会拒绝敏感 env 文本，MCP evidence summary 不注入 env 原值 |
| LSP-18 | 补充 endpoint 优先、command fallback、missing config、session workspace isolation 的 MCP runtime 单测 | mcp runtime/test | Done |
| LSP-19 | 将 `capabilityUsagePolicies.requiredAfterKinds` 升级为 runtime 中立合同，触发后要求匹配 `mcp/tool_call` 或结构化 skip reason | orchestrator/runtime | Done |
| LSP-20 | software-dev code semantics policy 将 `lsp_sync` 纳入 preferredTools，确保 patch 后同步能力可被模板明确表达 | template | Done |
| LSP-21 | 增加 runtime 单测覆盖：触发 post-action capability policy 后缺少 MCP follow-up 被拒绝，声明 follow-up 或 skip reason 可通过 | orchestrator/test | Done |

## 13. 影响面核对

| Area | Current state | Required change |
| --- | --- | --- |
| MCP server repository/model | 已支持 `stdio`、`endpoint`、allowed tools、long-lived session | 需要新增 generic `command/args/env` 字段，并保留 `endpoint` |
| MCP runtime adapter | 已支持 stdio MCP initialize、tools/call、long-lived session | 需要调整 stdio 启动解析优先级：endpoint > command/args/env > missing config error；long-lived session key 增加 workspace 维度；无需 LSP 专用逻辑 |
| Workspace activation | 已支持 `mcpServerIds`、`capabilityIds`、skills required MCP auto-bind | 可复用；若要 software-dev 自动绑定，需要 template/skill 提供 required refs |
| Soul capability gate | 已支持 allowed MCP server/capability/tool kinds | 需要在 software-dev Soul 中允许对应 MCP tool call |
| LLM capability context | 已能注入 selected `mcpCapabilities`，并对 `mcp/tool_call` 过程证据生成通用 compact tool evidence summary | 已完成；LSP 结果通过通用 MCP evidence summary 进入上下文，不进入 core 专用逻辑 |
| Evidence gate | 已把 `mcp/tool_call` 作为直接动作证据类型之一 | 需在 Soul/Template 中声明 LSP evidence 只能辅助，不能单独验收 DoD |
| Web/TUI config | 已有 MCP server/capability 列表与选择入口 | 需要同步展示和编辑 command/args/env；env 展示默认脱敏 |
| Built-in defaults | 当前没有 `mcp-lsp-bridge` 默认 server/capability | 需要新增 bootstrap 默认注册或导入机制 |
| Workspace internal config | 当前未生成 LSP workspace 私有配置 | 需要在 WorkOrch 内置管理目录生成并绑定 `LSP_BRIDGE_CONFIG` |
| Tests | 已有 MCP block、activation、template 默认值、MCP evidence summary、tool_feedback、env 脱敏和 core neutrality 边界检查 | 已完成本阶段核心回归；真实 lsp-bridge 部署态验证另行执行 |

## 14. 验收标准

完成后应满足：

- software-dev workspace 能发现并绑定 `lsp-bridge` MCP 能力。
- LLM 上下文中可看到可用 LSP capability，但 core prompt 不包含 LSP 专用业务规则。
- 代码修改后可通过 `lsp_sync` 和 `lsp_diagnostics` 获取当前诊断。
- LSP diagnostics 能进入 repair context，帮助定位当前失败。
- LSP 不可用时不阻塞普通软件开发流程。
- DoD accepted 仍由 build/test/lint/patch/evidenceRefs 决定，不被 LSP clean 直接放行。
- 相关实现不破坏 `workorch-runtime-stage-contract-neutrality-design.md` 的 core neutrality 约束。
