# Software-Dev Code Agent Profile

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 `software-dev` profile 中已经稳定的 Code Agent 行为约束，只保留仍应指导当前架构和实现的 profile 规则。

## Stable Contract

### Goal / DoD 驱动

- `software-dev` 任务始终围绕显式 `Goal` 和 `DoD` 收敛，不得引入并行的开发任务状态机。
- agent 需要覆盖任务拆解、状态变更、验证、修复、验收和交付证据生成的完整闭环。
- TUI 只负责 Goal/DoD 输入与结果呈现，不承载后端业务分叉。

### Template And Run Boundary

- `software-dev` 能力通过 workspace template 在 Workspace 创建期初始化，不在 Run activation 时重新绑定模板、SoulBindings、Skill 或 MCP。
- Run activation 只允许在不改变 Workspace 持久化绑定的前提下刷新中立 `executionContract` 基线，用于继承最新 `profileContracts`、`deliverableExpectations` 和 `evidenceRequirements`。
- 当前目录默认自动创建 Workspace 时应使用 `generic-governed`；需要软件开发能力时必须显式选择 `software-dev` template。

### Governance Boundary

- Ticket 只用于缺失信息或真实决策阻塞，不得替代可机械推进的 repair。
- Approval 只用于高风险操作授权，不得退化成普通步骤确认机制。
- 本地 TUI/CLI 必须继续通过 EventHub 与后端交互，不引入本机 HTTP client 分叉或定时轮询。

### Delivery And Evidence

- 最终 Result/Artifact 必须提供真实交付证据，包括变更摘要、diff 或可追溯 diff artifact、验证命令与结果、剩余风险和交付物索引。
- Memory 只应沉淀项目约定、验证命令、历史决策和失败修复经验，不写入低价值过程噪音。
- 自动修复循环必须基于失败证据推进，直到 DoD 满足、出现真实 blocker，或进入治理边界。
- 当构建、测试或工具执行失败暴露依赖解析、锁文件、校验和、工具链或 registry/network prerequisite 问题时，`software-dev` template 通过 `contextHints.runtimeRepair.prerequisiteManifestTargetPatterns` 声明可作为根因候选的生态文件，例如 module manifest、lock、checksum、requirements、package metadata 和 build entry metadata。Core runtime 只消费这些 pattern 与当前 manifest，不在核心层理解具体语言或包管理器。

## Capability And Module Mapping

`software-dev` code-agent 的稳定能力面主要依赖：

- `file/*` / `patch/*`
  - 代码和配置状态变更
- `shell/command`
  - build / test / lint / entrypoint verification
- `git/*`
  - 仓库状态观察、diff 摘要和提交闭环
- `mcp/tool_call`
  - 仅在 profile 明确允许时引入外部能力
- `web/search` / `web/fetch`
  - 仅在 profile 或 Goal 明确要求引用外部事实时启用

对应 block 适配层：

- `blocks/file`
- `blocks/shell`
- `blocks/git`
- `blocks/mcp`
- `blocks/web`

对应 kernel / application 约束层：

- `kernel/soul`
- `kernel/skill`
- `kernel/mcp`
- `kernel/memory`
- `application/workorchd`

## Product / Read-Model Mapping

code-agent 的典型观察面不应自建并行状态，而应复用：

- `workspace snapshot`
  - 当前 workspace outcome、runs、taskProgress
- `run snapshot`
  - 当前 run 的 results、artifacts、DoD closure、deliverable coverage
- `run observability`
  - execution / repair / governance 时间线
- detail pages
  - `patches`、`validation-results`、`artifacts`、`tool-calls`
- TUI requirements
  - Goal / DoD 输入、治理交互、result-first 输出

## Verification Mapping

code-agent profile 的稳定验证入口包括：

- `verify-software-dev-code-agent.sh`
- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-requirement-development-orchestration.sh`
- `verify-tui-agent.sh`

这些验证共同回答：

- 是否仍围绕 Goal / DoD 收敛
- 是否继续通过 patch / validation / artifact / result 闭环交付
- 是否保持治理边界和 EventHub/TUI 集成边界
- 是否没有退化回本地轮询、自由 shell 分叉或并行状态机

## Cross-Layer Mapping

- profile coverage 细则：
  `goal-coverage-and-dod.md`
- block 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- 运行结果与证据：
  `../../30-module-architecture/kernel-workspace-run-result.md`
- TUI 产品约束：
  `../../50-product-interface/tui/requirements.md`
- profile verifier：
  `../../60-verification/profile-software-dev-verifier.md`

## Verification Focus

- 主链路验证见 `../../60-verification/profile-software-dev-verifier.md`
- TUI 侧产品与交互约束见 `../../50-product-interface/tui/requirements.md`
- Goal / DoD 和 coverage 规则见 `goal-coverage-and-dod.md`

## Source Documents

- `../../90-archive/completed/software-dev-code-agent-closure-plan.md`
- `goal-coverage-and-dod.md`
- `lsp-bridge-integration.md`
