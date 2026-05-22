# Workspace Builtin Git Tool Capability Plan

## 背景

本文档用于定义 workorch 面向软件开发与仓库治理场景补齐内置 Git Tool 能力的
设计边界、任务范围、交付顺序和验收标准。

目标不是继续通过 `shell/command` 间接执行任意 `git` 命令，而是把 Git 作为
受治理的 runtime block 纳入现有
`Run / Goal / DoD / ToolCall / RuntimeCheckpoint / Approval / Artifact / Memory`
闭环：

- Git 能力通过显式 step kind 暴露，而不是隐藏在自由 shell 中。
- 只读和写操作分离治理，默认最小开放。
- 每次 Git 操作都生成结构化证据，便于 DoD、Artifact 和 Timeline 引用。
- 默认拒绝高风险仓库改写、历史改写和 remote 变更。
- 配置保持最小，优先固化安全默认值，避免用户侧复杂装配。

## 目标

### 用户目标

用户只需要在合适的 Soul / WorkspaceTemplate 中启用 Git 工具能力，workorch 即可：

- 查询当前仓库状态、分支、最近提交和差异。
- 在明确路径范围内暂存变更。
- 以显式提交信息创建本地提交。
- 在最终 Result / Artifact 中引用 Git 证据。
- 在需要时通过 Approval 控制状态变更类操作。

### 系统目标

- Git 能力必须作为内置 runtime block 接入 orchestrator。
- LLM 看到的是结构化 Git 工具，而不是开放式原始命令执行。
- Git 工具默认只作用于 Workspace `deliverableDirectory` 对应仓库。
- 当前阶段只提供高价值、低歧义操作，不暴露完整 Git 命令面。
- 输出必须兼容现有 `ToolCall`、`RuntimeCheckpoint`、Result / Artifact 证据链。
- 配置项保持最少，默认行为尽可能覆盖常见开发流。

## 非目标

- 不在第一版支持任意原始 Git 子命令透传。
- 不在第一版支持 `push`、`pull`、`fetch`、`merge`、`rebase`、`reset`、
  `clean`、`checkout`、`switch`、`cherry-pick`。
- 不在第一版支持多仓聚合编排或跨仓事务。
- 不在第一版提供完整交互式 Git 工作流。
- 不依赖外部 MCP Git Server 作为主路径。
- 不把 Git block 设计成 shell policy 的薄包装。

## 当前基线

已存在或可复用能力：

- orchestrator 已通过 runtime adapter registry 按 step kind 注册执行器。
- `file/*`、`web/*`、`mcp/*`、`shell/command`、`patch/*` 已采用显式 runtime
  block 模型。
- `ToolCall`、`RuntimeCheckpoint`、`Result`、`Artifact`、`Memory` 已形成基础
  证据链。
- Soul 可通过 `allowedToolKinds` 向 LLM 暴露允许的工具种类。
- shell block 当前仅对极少数危险 Git shell 行为做兜底拦截，无法表达 Git
  的仓库语义和状态边界。

当前缺口：

- 缺少原生 Git runtime block。
- 缺少 Git 结构化输入输出契约。
- 缺少 Git 仓库边界、路径范围和高风险操作的专门策略。
- 缺少 Git 操作在 Result / Artifact / UI 中的统一证据模型。

## 设计原则

### 一等能力，不走 shell 包装

Git 必须作为独立 block 接入，不继续沿用 `shell/command` 执行 `git`：

- shell 只能表达进程级风险，难以表达 Git 的索引区、工作区、HEAD、branch、
  remote、副作用范围。
- Git 需要结构化参数和结构化证据，便于编排、审计和 DoD 引用。
- Git 的可开放面应精确受 `allowedToolKinds` 控制，而不是通过 prompt 暗示。

### 最小能力面

第一版只开放少量高频操作：

- `git/read`
  - `status`
  - `diff`
  - `log`
  - `show`
  - `branch`
- `git/write`
  - `add`
  - `commit`

### 默认安全

- 默认仅允许本地仓库只读和最小写入。
- 默认禁止 remote 变更。
- 默认禁止历史改写和工作区破坏性清理。
- 默认要求写操作显式声明目标路径或目标对象。

### 配置最小化

用户侧只保留必要配置；其余策略内建为安全默认值。

## 架构边界

### 内置 block 边界

建议新增：

- `internal/modules/blocks/git`
  - `git/read`
  - `git/write`

要求：

- block 只负责 Git 参数解析、仓库校验、策略拦截、命令执行和结构化输出。
- block 不负责业务目标判断，不直接完成 DoD 结论。
- block 不直接写 Memory 或 Artifact。
- block 不绕过 orchestrator 的 ToolCall / RuntimeCheckpoint 记录链路。

### Runtime Helper / Registry 边界

需要沿用现有 runtime block 接线方式：

- `workorchruntime.Helper` 新增 Git adapter getter / setter。
- `blocks/git/plugin.go` 在模块启动时向 runtime 注入 Git adapter。
- `kernel/orchestrator/plugin.go` 将 `git/read`、`git/write` 注册到 runtime registry。

Git 不应复用 shell adapter 的 step kind。

### Orchestrator 边界

- orchestrator 继续按 step kind 分发执行。
- orchestrator 不内置 Git 业务规则或 repo 语义。
- Git block 结果回流给后续 LLM step，再决定是否继续读取、提交、验证或结束。
- Git block 不替代 `patch/apply`。文件内容变更仍优先由 patch 落盘，Git 负责
  仓库状态观察和提交闭环。

### AppService / Read Model 边界

- appservice 负责把 Git ToolCall 证据投影到 Run / Workspace 读模型。
- Web/TUI/CLI 只消费投影结果，不重复解析 Git stdout。
- UI 默认展示摘要、影响范围、commit hash、branch、changed files 等关键信息。

## 能力模型

### 通用输入字段

所有 Git step 输入都遵循以下通用字段：

- `operation`: 必填，Git block 内部支持的操作名。
- `pathspecs`: 可选或按 operation 必填，必须是 workspace 相对路径列表。
- `reason`: 可选，说明本次 Git 操作服务的 Goal、DoD 或证据目标。
- `timeoutMillis`: 可选，覆盖默认超时，但不得超过 runtime 内置最大值。
- `approvalId`: 预留字段；第一版 Git block 不主动触发 Approval。

禁止字段：

- `command`
- `args`
- `workingDir`
- `repoRoot`
- 任意 shell script 字段

Git block 不接受任意命令透传。仓库位置只能由
`StepExecutionRequest.WorkspaceDeliverableDirectory` 推导。

### 通用输出字段

所有 Git step 输出都遵循以下通用字段：

- `status`: `completed` / `failed` / `blocked` / `rejected` / `timeout`
- `operation`
- `stage`: `git_read` / `git_write`
- `repoRoot`
- `workingDir`
- `branch`
- `headCommit`
- `pathspecs`
- `summary`
- `stdout`
- `stderr`
- `exitCode`
- `durationMillis`
- `timeoutMillis`
- `changedFiles`
- `truncated`
- `zeroWork`
- `errorCode`
- `error`

输出限制：

- `stdout` 和 `stderr` 单字段默认最多保留 64 KiB。
- `diff` / `show` 正文默认最多保留 128 KiB。
- `changedFiles` 默认最多返回 500 项。
- 被截断时必须设置 `truncated = true`，并在 `summary` 中说明。

`changedFiles` 建议结构：

```json
{
  "path": "docs/example.md",
  "oldPath": "",
  "status": "modified",
  "staged": false,
  "unstaged": true,
  "untracked": false,
  "additions": 12,
  "deletions": 3
}
```

`status` 取值建议：

- `added`
- `modified`
- `deleted`
- `renamed`
- `copied`
- `untracked`
- `conflicted`

### git/read

用途：只读仓库状态查询。

建议输入：

- `operation`: 必填，支持 `status` / `diff` / `log` / `show` / `branch`
- `pathspecs`: 可选，限制路径范围
- `ref`: 可选，用于 `show` / `diff` / `log`
- `baseRef`: 可选，用于 `diff`
- `limit`: 可选，用于限制日志或差异返回规模
- `reason`: 可选，用于说明本次查询服务哪个 Goal / DoD

建议输出：

- `status`
- `operation`
- `repoRoot`
- `workingDir`
- `branch`
- `headCommit`
- `pathspecs`
- `summary`
- `stdout`
- `stderr`
- `exitCode`
- `durationMillis`
- `changedFiles`
- `aheadBehind`
- `truncated`

规则：

- 不允许修改索引区、工作区或 refs。
- 返回内容必须有大小上限。
- `diff`、`log`、`show` 应支持截断并给出 `truncated` 标记。
- 输出中优先补充结构化摘要，避免后续链路只能依赖原始 stdout。

### git/read operation 契约

#### status

输入：

```json
{
  "operation": "status",
  "pathspecs": ["docs"]
}
```

规则：

- `pathspecs` 可选。
- 返回当前 branch、HEAD、ahead / behind、staged / unstaged / untracked 文件。
- 必须使用稳定机器格式解析，不依赖人类可读输出。

输出重点：

- `branch`
- `headCommit`
- `changedFiles`
- `aheadBehind`
- `summary`

#### diff

输入：

```json
{
  "operation": "diff",
  "baseRef": "HEAD",
  "ref": "",
  "pathspecs": ["docs/workspace-builtin-git-tool-capability-plan.md"],
  "limit": 200
}
```

规则：

- `baseRef` 可选，默认比较工作区与索引 / HEAD。
- `ref` 可选，用于比较两个 ref。
- `pathspecs` 可选，但必须通过路径策略。
- 返回 diff 摘要和受限正文。

输出重点：

- `changedFiles`
- `stdout`
- `truncated`
- `summary`

#### log

输入：

```json
{
  "operation": "log",
  "ref": "HEAD",
  "limit": 10,
  "pathspecs": ["docs"]
}
```

规则：

- `limit` 默认 20，最大 100；超过最大值时自动 clamp 到 100，并在 `summary` 中标注。
- `ref` 默认 `HEAD`。
- 输出结构化 commits，不只输出 raw log。

输出重点：

- `commits`
- `summary`

`commits` 建议结构：

```json
{
  "hash": "full-commit-hash",
  "shortHash": "abc1234",
  "author": "Name <mail@example.com>",
  "authorTime": "2026-05-22T10:00:00Z",
  "subject": "docs: update design"
}
```

#### show

输入：

```json
{
  "operation": "show",
  "ref": "HEAD",
  "pathspecs": ["docs"]
}
```

规则：

- `ref` 必填或默认 `HEAD`。
- 输出 commit metadata、文件摘要和受限 diff 正文。
- 不允许使用 `show` 读取任意 blob 到绕过 file block 的无限内容。

#### branch

输入：

```json
{
  "operation": "branch"
}
```

规则：

- 只读当前 branch 和本地 branch 摘要。
- 第一版不执行切换、创建或删除分支。

输出重点：

- `branch`
- `detachedHead`
- `branches`
- `headCommit`

### git/write

用途：受控本地状态变更。

定义输入：

- `operation`: 必填，支持 `add` / `commit`
- `pathspecs`: `add` 必填；`commit` 不接受
- `message`: `commit` 必填
- `amend`: 第一版不支持
- `reason`: 可选，说明本次变更与 Goal / DoD 的对应关系
- `approvalId`: 预留字段；第一版 `add` / `commit` 不使用。

定义输出：

- `status`
- `operation`
- `repoRoot`
- `workingDir`
- `branch`
- `headCommitBefore`
- `headCommitAfter`
- `message`
- `pathspecs`
- `summary`
- `stdout`
- `stderr`
- `exitCode`
- `durationMillis`
- `changedFiles`
- `commit`
  - `hash`
  - `subject`

规则：

- `add` 必须显式给出 `pathspecs`，第一版不接受隐式全仓暂存。
- `commit` 必须显式给出非空 `message`。
- 若无 staged changes，`commit` 返回结构化零工作结果，不应伪成功。
- 第一版不支持自动补全 commit identity；若环境缺失用户名或邮箱，返回明确错误。

### git/write operation 契约

#### add

输入：

```json
{
  "operation": "add",
  "pathspecs": ["docs/workspace-builtin-git-tool-capability-plan.md"],
  "reason": "stage the design document for the builtin git tool feature"
}
```

规则：

- `pathspecs` 必填且不能为空。
- 不接受 `"."`、`"*"`、`":/"`、空字符串或只包含目录上跳的路径。
- 所有 pathspec 必须通过 workspace 边界校验。
- 执行前后都读取 `status`，输出 staged delta。
- 若没有任何文件被 staged，返回 `completed + zeroWork`。

输出重点：

- `changedFiles`
- `stagedFiles`
- `zeroWork`
- `summary`

#### commit

输入：

```json
{
  "operation": "commit",
  "message": "docs: design builtin git tool",
  "expectedHead": "full-commit-hash",
  "reason": "record the design baseline for implementation"
}
```

规则：

- `message` 必填，去除首尾空白后必须非空。
- `message` 第一行建议不超过 72 字符，但第一版只做提示，不强制失败。
- `commit` 不接受 `pathspecs`。它只提交当前 index 中已经 staged 的变更。
- `expectedHead` 可选；提供时，提交前 HEAD 必须匹配，否则返回 `failed`。
- 默认不允许空提交。
- 默认使用 `--no-verify`，不执行仓库内 Git hooks，避免 commit 隐式执行任意代码。
- 不支持 `--amend`。
- 不支持 GPG signing 交互；若本地配置要求签名导致失败，返回明确错误。

输出重点：

- `headCommitBefore`
- `headCommitAfter`
- `commit.hash`
- `commit.subject`
- `stagedFiles`
- `committedFiles`
- `summary`

## 底层命令映射

Git block 内部可以通过系统 `git` CLI 实现，但对外不暴露原始命令。

通用执行要求：

- 所有命令使用 `git -C <repoRoot>` 或设置 `cmd.Dir = repoRoot` 执行。
- 所有 pathspec 前必须插入 `--`。
- 所有 pathspec 在进入命令前完成 workspace 相对路径校验。
- 设置 `GIT_TERMINAL_PROMPT=0`，禁止 Git 阻塞等待凭据输入。
- 使用受限环境变量集合，避免泄露不必要环境。
- 禁止通过输入覆盖 `GIT_DIR`、`GIT_WORK_TREE`、`SSH_ASKPASS` 等变量。

命令映射：

| Operation | 命令映射 | 说明 |
| --- | --- | --- |
| `status` | `git status --porcelain=v2 --branch --untracked-files=all -- <pathspecs>` | 稳定解析 staged / unstaged / untracked |
| `diff` | `git diff --numstat --name-status -- <pathspecs>` + 受限 `git diff -- <pathspecs>` | 先结构化摘要，再读取受限正文 |
| `log` | `git log --format=<machine-format> -n <limit> <ref> -- <pathspecs>` | 输出结构化 commits |
| `show` | `git show --format=<machine-format> --stat <ref>` + 受限 `git show --format= -- <ref> -- <pathspecs>` | metadata 与 diff 分离 |
| `branch` | `git branch --format=<machine-format>` + `git rev-parse --abbrev-ref HEAD` | 检测 detached HEAD |
| `add` | `git add -- <pathspecs>` | 只接受显式 pathspec |
| `commit` | `git -c core.hooksPath=/dev/null commit --no-verify -m <message>` | 只提交当前 staged changes；不执行 hooks，不 amend，不空提交 |

命令失败时：

- 返回原始 `exitCode`。
- `stderr` 截断保存。
- 将常见错误归类为 `errorCode`，例如 `not_git_repository`、
  `path_rejected`、`identity_missing`、`no_staged_changes`、
  `head_mismatch`、`operation_rejected`。

## 策略模型

### 仓库边界

默认仓库边界：

- Git tool 只能作用于 `WorkspaceDeliverableDirectory` 所属仓库。
- 若目录不在 Git 仓库中，Git block 返回明确错误。
- 不允许通过输入指定任意工作目录跳出 Workspace 边界。
- 不支持 bare repository。
- 第一版不递归管理 submodule；进入 submodule 内部执行时视为独立仓库，必须仍位于
  Workspace 边界内。
- symlink 解析后的真实路径不得逃逸 Workspace 边界。

### Pathspec 策略

所有 pathspec 必须：

- 是相对路径。
- 清理后不为空。
- 不包含 `..` 路径上跳。
- 不以 `-` 开头，避免被 Git 解释为参数。
- 不等于 `.git`、`.git/` 或 `.git` 内部路径。
- 不等于 `.`、`*`、`:/` 等全仓或魔法 pathspec。

第一版不支持 Git magic pathspec，例如：

- `:(glob)`
- `:(exclude)`
- `:(top)`

原因是 magic pathspec 会扩大 LLM 可表达的选择语义，增加策略误判风险。

### Branch / HEAD 策略

第一版规则：

- `git/read` 可在 detached HEAD 下执行。
- `git/write:add` 可在 detached HEAD 下执行，但输出必须标注 `detachedHead = true`。
- `git/write:commit` 默认拒绝 detached HEAD。
- 不内建 protected branch 配置，避免新增复杂配置项。
- 如后续需要保护 `master` / `main`，优先通过 WorkspaceTemplate 或 Approval
  策略扩展，而不是第一版配置项。

### 操作分级

按风险分层：

- 低风险只读：`status`、`branch`、`log`、`show`、有限 `diff`
- 中风险本地写：`add`、`commit`
- 高风险禁用：改写历史、切换分支、清理工作区、remote 变更

第一版处理原则：

- 低风险默认允许。
- 中风险只在 `writeEnabled=true` 且 Soul / Template 显式开放时允许。
- 第一版 `add` / `commit` 不引入额外 Approval，避免增加配置和运行时复杂度。
- 高风险默认不可用，不进入 tool surface。

### Hook / Credential 策略

Git commit 默认可能执行仓库内 hook。第一版必须避免 Git block 隐式执行未声明代码：

- `commit` 默认使用 `--no-verify`。
- 不执行 pre-commit、commit-msg、post-commit 等 hooks。
- 若用户需要执行 hooks，应通过显式 `shell/command` 验证步骤完成，并生成独立证据。

凭据策略：

- Git block 不主动读取或注入 remote 凭据。
- 设置 `GIT_TERMINAL_PROMPT=0`，避免阻塞等待用户名、密码或 token。
- 第一版不执行 remote 操作，因此不应触发网络凭据交互。

### 零工作与异常分类

应统一区分：

- `completed`
- `failed`
- `blocked`
- `rejected`
- `zeroWork`

典型场景：

- `git commit` 无 staged changes：`completed + zeroWork`
- 仓库不存在：`failed`
- 操作不在允许面内：`rejected`
- 命中需要 Approval 的策略但未带 `approvalId`：`blocked`

### 并发与漂移策略

同一 Workspace 可能被多个 Run 或外部用户同时修改。第一版 Git block 必须保证：

- `commit` 前读取 `HEAD`，并在输出记录 `headCommitBefore`。
- 输入带 `expectedHead` 时，提交前 HEAD 不匹配必须失败。
- `add` 后重新读取 status，输出本次实际 staged 文件。
- `commit` 前重新读取 staged status；无 staged changes 返回 `zeroWork`。
- `commit` 成功后重新读取 HEAD，输出 `headCommitAfter` 和 commit hash。
- 不尝试自动解决并发冲突。

如后续需要更强隔离，再引入 Workspace 级 runtime lock；第一版先以
`expectedHead` 和前后状态证据收口。

## 配置设计

原则：只保留必需配置项。

建议配置：

```toml
[runtime.git]
enabled = true
writeEnabled = true
defaultTimeout = "30s"
```

说明：

- `enabled`
  - 是否启用 Git runtime block。
- `writeEnabled`
  - 是否开放 `git/write`。
- `defaultTimeout`
  - Git 命令默认超时；仍应有内部最大上限。

第一版不建议暴露的配置：

- 任意命令 allowlist / denylist
- 任意 repo allowlist
- remote 细粒度开关
- branch 正则策略
- commit identity 配置注入

这些策略先以内建默认值处理，避免用户配置复杂度膨胀。

配置校验：

- `defaultTimeout` 必须是合法 duration。
- `defaultTimeout` 最小 1s，最大 10m。
- `enabled = false` 时不注册 Git adapter。
- `writeEnabled = false` 时不注册或不接受 `git/write`。

`application.toml` 示例：

```toml
[runtime.git]
enabled = true
writeEnabled = true
defaultTimeout = "30s"
```

默认值建议：

- `enabled = true`
- `writeEnabled = true`
- `defaultTimeout = "30s"`

说明：默认启用 `git/read` 和 `git/write`，但写能力仍只支持本地 `add` /
`commit`，不支持 remote 操作、历史改写、分支切换或 hooks。

## 与 Soul / Template 的关系

建议在软件开发执行阶段显式开放：

- `git/read`
- `git/write`

而不是继续只开放 `shell/command`。

典型关系：

- 规划阶段通常不需要 Git 写能力。
- 执行阶段可开放 `git/read` 与 `git/write`。
- 验证阶段通常只需要 `git/read`。
- 若 WorkspaceTemplate 不涉及仓库提交，则可只开放 `git/read`。

### runtimeCapabilities 注入

LLM runtimeCapabilities 中应描述实际可用能力，不暗示完整 Git CLI：

```json
{
  "kind": "git/read",
  "operations": ["status", "diff", "log", "show", "branch"],
  "readOnly": true,
  "pathScoped": true
}
```

```json
{
  "kind": "git/write",
  "operations": ["add", "commit"],
  "readOnly": false,
  "pathScoped": true,
  "requiresExplicitPathspecs": true,
  "remoteOperations": false,
  "historyRewrite": false
}
```

当 `runtime.git.writeEnabled = false` 时，不应向 LLM 注入 `git/write`。

### Prompt 约束

software-dev execution Soul 应补充：

- 使用 `patch/apply` 修改文件。
- 使用 `git/read` 查看仓库状态和差异。
- 只有用户目标明确要求提交时才使用 `git/write`。
- 不要通过 `shell/command` 调用 `git` 完成已由 Git block 支持的操作。

software-dev validation Soul 应补充：

- 使用 `git/read` 核对最终仓库状态。
- 不使用 `git/write`。

## 与 patch / shell 的关系

### 与 patch/apply

- 文件内容修改仍优先通过 `patch/apply` 进行，确保 diff 可审计。
- Git 不负责生成或应用内容 patch。
- Git 负责承接 patch 落地后的仓库状态观察、暂存和提交闭环。

### 与 shell/command

- shell 仍保留给构建、测试、格式化、验证等非 Git 任务。
- 不建议继续在软件开发主流程里把 Git 作为 shell 默认路径。
- shell 中已有的危险 Git 兜底拦截继续保留，作为最后一道防线，而不是主治理层。
- 第一版不强行禁止 shell 执行全部 git 命令，避免破坏已有流程。
- Soul / runtimeCapabilities 应引导 LLM 优先使用 Git block。
- 后续可增加 shell evidence policy：当 shell 调用 Git 且 Git block 已支持该操作时，
  标记为低质量证据或要求改用 Git block。

## 证据模型

每次 Git ToolCall 至少输出以下结构化字段：

- `operation`
- `repoRoot`
- `workingDir`
- `branch`
- `headCommit`
  或 `headCommitBefore` / `headCommitAfter`
- `changedFiles`
- `summary`
- `stdout`
- `stderr`
- `exitCode`
- `durationMillis`

`commit` 成功时必须补充：

- `commit.hash`
- `commit.subject`

`status` / `diff` / `log` / `show` 建议补充：

- `truncated`
- `aheadBehind`
- `pathspecs`

Artifact / Result 可基于这些字段引用：

- 本次提交 hash
- 本次提交消息
- 暂存/提交涉及文件
- 提交前后仓库状态摘要

### Event / Timeline 展示

Run observability timeline 中建议展示：

- Git operation
- repo root 简写
- branch
- changed file count
- commit hash
- zeroWork / truncated 标记
- errorCode

Web/TUI/CLI 不直接展示完整 diff 正文，默认展示摘要和证据引用。

## LLM 使用约束

为减少歧义，Git 工具面应尽量结构化：

- LLM 不应生成任意原始 Git 子命令字符串。
- LLM 应选择 `git/read` 或 `git/write`，并明确 `operation`。
- 写操作必须显式声明目标范围和提交信息。
- 若任务目标只是核对当前变更状态，不应调用 `git/write`。

这有助于让 runtime submitted context 中的工具能力与实际支持面最大限度对齐，
避免 LLM 误以为可使用完整 Git CLI。

## 实现落点

建议新增或调整文件：

- `internal/modules/blocks/git/module.go`
- `internal/modules/blocks/git/plugin.go`
- `internal/modules/blocks/git/biz/git_runtime_adapter.go`
- `internal/modules/blocks/git/biz/git_runtime_adapter_test.go`
- `internal/modules/blocks/git/pkg/common/common.go`
- `internal/initiators/workorchruntime/pkg/common/common.go`
- `internal/initiators/workorchruntime/module.go`
- `internal/modules/kernel/orchestrator/plugin.go`
- `internal/pkg/workorchconfig/config.go`
- `docs/builtin-templates-and-souls-design.md`

实现分层：

- `pkg/common`: step kind、operation、状态常量。
- `biz`: adapter、输入解析、策略校验、Git command runner、输出归一化。
- `module.go`: runtime adapter 生命周期封装。
- `plugin.go`: 注入 workorch runtime helper。

Git command runner 应设计为接口，便于单元测试模拟命令输出；集成测试再使用真实临时
Git 仓库。

## 测试矩阵

### 单元测试

| 场景 | 期望 |
| --- | --- |
| nil input | 返回 invalid argument |
| 未知 step kind | 返回 rejected |
| 未知 operation | 返回 rejected |
| 非 Git repo | 返回 `failed/not_git_repository` |
| bare repo | 返回 rejected |
| pathspec 为空执行 add | 返回 rejected |
| pathspec 为 `.` / `*` / `:/` | 返回 rejected |
| pathspec 包含 `..` | 返回 rejected |
| pathspec 以 `-` 开头 | 返回 rejected |
| pathspec 指向 `.git` | 返回 rejected |
| status 正常解析 porcelain v2 | 返回 changedFiles |
| diff 大输出 | 返回 truncated |
| log limit 超过最大值 | clamp 到最大值并在 summary 标注 |
| commit message 为空 | 返回 rejected |
| commit 无 staged changes | 返回 `completed + zeroWork` |
| commit 缺 identity | 返回 `failed/identity_missing` |
| commit detached HEAD | 返回 rejected |
| commit expectedHead 不匹配 | 返回 `failed/head_mismatch` |
| commit 成功 | 返回 full hash 和 subject |

### 集成测试

| 场景 | 期望 |
| --- | --- |
| 临时仓库 status | 能返回 branch、HEAD、changedFiles |
| patch 后 git diff | 能返回目标文件差异 |
| add 指定文件 | 只 staged 指定文件 |
| commit 指定 staged 文件 | 创建新 commit |
| commit hook 存在 | 因 `--no-verify` 不执行 hook |
| `writeEnabled=false` | `git/write` 不可执行 |
| Git adapter 未注册 | orchestrator 返回 unsupported runtime step |

### 回归测试

- 现有 `shell/command` 行为不因新增 Git block 破坏。
- 现有 `patch/apply` 证据链不改变。
- software-dev execution 的 `allowedToolKinds` 注入后，LLM 上下文中只出现实际可用
  Git 能力。

## 交付顺序

### G1. 设计与契约

- 明确 step kind、operation surface、输入输出结构、策略边界。
- 更新相关 Soul / Template 文档中的 Git 能力声明。
- 收口当前文档中的策略结论。

### G2. Runtime 接线

- 新增 `blocks/git`
- 新增 Git runtime adapter
- runtime helper 与 orchestrator registry 完成接线
- 增加 `runtime.git` 配置解析和默认值

### G3. 证据与读模型

- 标准化 Git ToolCall 输出字段
- Run / Workspace 读模型投影 Git 摘要信息
- runtimeCapabilities 注入 Git 能力

### G4. 验证与验收

- 只读成功路径测试
- 写路径成功测试
- 非仓库目录拒绝测试
- 禁止操作拒绝测试
- 零工作结果测试
- hook、detached HEAD、identity missing、expectedHead mismatch 测试

## 收口清单

| 编号 | 收口项 | 结论 |
| --- | --- | --- |
| GT1 | step kind | `git/read`、`git/write` |
| GT2 | 第一版只读操作 | `status`、`diff`、`log`、`show`、`branch` |
| GT3 | 第一版写操作 | `add`、`commit` |
| GT4 | remote 操作 | 第一版全部禁止 |
| GT5 | 历史改写 | 第一版全部禁止 |
| GT6 | 分支切换 | 第一版禁止 |
| GT7 | pathspec | 必须显式、相对、不可越界、不可 magic |
| GT7a | `add` pathspec | 必填 |
| GT7b | `commit` pathspec | 不支持；只提交当前 staged changes |
| GT8 | commit hooks | 默认 `--no-verify`，不执行 |
| GT9 | detached HEAD commit | 默认拒绝 |
| GT10 | commit identity | 不注入，缺失则失败 |
| GT11 | 默认配置 | `enabled=true`、`writeEnabled=true`、`defaultTimeout=30s` |
| GT12 | shell git | 暂不硬禁，通过 Soul 引导迁移 |
| GT13 | patch 关系 | 文件内容修改仍由 `patch/apply` 承担 |
| GT14 | 并发漂移 | 用 `expectedHead` 与前后状态证据收口 |
| GT15 | 证据输出 | ToolCall 输出结构化 Git 字段 |
| GT16 | 配置样例 | `config/file`、`config/container-file` 和本机默认配置显式声明 `[runtime.git]` |
| GT17 | LLM 能力注入 | runtimeCapabilities 按 `runtime.git.enabled/writeEnabled` 注入 `git/read`、`git/write` |

## 验收标准

- `git/read` 能在 Workspace 仓库中成功返回结构化状态证据。
- `git/write:add` 只能在显式 `pathspecs` 下执行。
- `git/write:commit` 必须要求提交信息，并返回 commit hash。
- 非仓库目录、越界目录和禁用操作能稳定拒绝。
- software-dev 类 Soul 能通过 `allowedToolKinds` 精确暴露 Git 能力。
- 最终 Result / Artifact 能引用 Git 证据，而不是解析原始 shell 文本。

## 当前建议结论

第一版应采用独立内置 Git block，能力面限制为：

- `git/read`: `status`、`diff`、`log`、`show`、`branch`
- `git/write`: `add`、`commit`

并保持最小配置：

- `enabled`
- `writeEnabled`
- `defaultTimeout`

远程操作、历史改写和分支切换暂不进入当前交付范围。
