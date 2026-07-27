# Built-in Git Tool Capability

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接内置 git 工具能力的设计说明。

## Stable Rules

### Positioning

- Git 应作为受治理的 runtime capability 暴露，而不是继续通过自由 `shell/command` 间接执行任意 Git 子命令
- Git 能力主要服务于软件开发和仓库治理场景，但不应污染 core runtime
- Git block 不替代 `patch/apply`；文件内容修改仍优先通过 patch 落盘，Git 负责仓库状态观察和提交闭环

### Minimum Capability Surface

第一版只开放高频、低歧义操作：

- `git/read`
  - `status`
  - `diff`
  - `log`
  - `show`
  - `branch`
- `git/write`
  - `add`
  - `commit`

不开放：

- `push`
- `pull`
- `fetch`
- `merge`
- `rebase`
- `reset`
- `clean`
- `checkout`
- `switch`
- `cherry-pick`

### Safety Boundary

- 默认仅允许本地仓库只读和最小写入
- 默认只作用于 Workspace `deliverableDirectory` 对应仓库
- 默认禁止 remote 变更、历史改写和工作区破坏性清理
- 写操作应显式声明目标路径、目标对象或提交信息

### Evidence Contract

- 每次 Git 操作都应产生结构化 `ToolCall` / `RuntimeCheckpoint` 证据
- Result / Artifact 应可引用 branch、commit hash、changed files、diff 摘要等 Git 证据
- UI 只消费读模型摘要，不重复解析原始 stdout

### Architecture Boundary

- Git block 只负责参数解析、仓库校验、策略拦截、命令执行和结构化输出
- orchestrator 只按 step kind 分发，不内置 Git repo 语义
- appservice/read model 负责把 Git 证据投影到 Run / Workspace 视图

## Step Kind / Adapter Mapping

当前稳定 step kind -> adapter 方向：

| Step Kind | Adapter Block | 主要产出 |
| --- | --- | --- |
| `git/read` | `blocks/git` | branch、status、diff、log、show 摘要 |
| `git/write` | `blocks/git` | add / commit 动作证据、head commit 变化 |

与文件能力的分工：

- 文件正文修改仍优先走 `patch/*`
- `git/*` 负责仓库视角的状态、差异和提交闭环

## Evidence / Artifact / Read-Model Mapping

Git 能力应优先沉淀为：

- `ToolCall`
  - 结构化 Git 动作记录
- `RuntimeCheckpoint`
  - 风险拦截、策略命中或关键仓库状态摘要
- `Artifact`
  - 必要时承接 diff summary、commit summary 或变更清单

产品层应主要从以下读模型查看：

- `tool-calls`
  - 查看 Git 动作明细
- `run snapshot`
  - 查看交付物和结果摘要中的 Git 证据引用
- `run observability`
  - 查看变更链、治理链、closure trace

## Verification Mapping

当前 Git capability 主要由以下入口间接或直接承接：

- `verify-workspace-file-diff-patch.sh`
- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-requirement-development-orchestration.sh`

其中：

- file-diff-patch verifier 主要校验文件/patch 主链与审批边界
- software-dev e2e 负责校验 Git 证据是否能进入交付闭环

## Approval Matrix

| 能力动作 | 默认审批方向 | 说明 |
| --- | --- | --- |
| `git/read.status` / `diff` / `log` / `show` / `branch` | 不需要 | 只读仓库观察 |
| `git/write.add` | 视 profile/policy 决定 | 通常与已有 patch/apply 共同组成提交流程 |
| `git/write.commit` | 视 profile/policy 决定 | meaningful changes 场景可自动执行，敏感仓库可要求审批 |
| remote / history rewrite / destructive git operations | 禁止 | 不进入第一版能力面 |

## Product Examples

典型产品展示方向：

- ToolCall detail
  - `operation`、`branch`、`headCommit`、`changedFiles`
- run snapshot / result summary
  - `commitHash`、`commitSubject`、版本控制快照说明
- artifact/export
  - 必要时导出 diff summary / changed files index

更具体的跨产品示例：

- Console ToolCall detail
  - `operation/branch/headCommit/changedFiles` 为主
- Result summary
  - 用 `commitHash/commitSubject` 表达版本控制快照
- Approval/Checkpoint
  - 当 Git 写入需要治理时，优先从 approval/checkpoint 查看理由，而不是只看 stdout

## Read-Model / UI Field Matrix

| 产品面 | 关键字段 | 当前方向 |
| --- | --- | --- |
| tool-calls detail | `operation` / `branch` / `headCommit` / `changedFiles` / `summary` | Git 动作主明细 |
| runtime checkpoints | `runStateSnapshot` / `blocking*` / repo state summary | 风险与恢复快照 |
| run snapshot | `results` / `artifacts` / Git 证据引用 | 交付闭环聚合 |
| run observability | `gitOperations` / `timeline` | Git 证据时间线 |

## Cross-Layer Mapping

- block 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- 运行结果与证据：
  `../../30-module-architecture/blocks-workspace-run-result.md`
- API/detail page：
  `../../50-product-interface/service-api.md`

## Input / Output Direction

- Git block 不接受任意 shell script、`workingDir` 或自由命令透传
- 仓库根目录只能由 `WorkspaceDeliverableDirectory` 推导
- 输出应包含 `operation`、`branch`、`headCommit`、`changedFiles`、`summary`、`stdout/stderr`、`duration`、`truncated` 等结构化字段
- 大结果必须有大小上限和截断标记

## Source Documents

- `file-diff-patch.md`
