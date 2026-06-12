# Software-Dev Goal Coverage And DoD

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 `software-dev` profile 在 Goal coverage、DoD-to-test mapping、
evidence policy、deliverable expectation 上的领域特化约束。

## Stable Rules

### Goal Decomposition

`software-dev` profile 应把软件开发 Goal 拆成可验证维度，而不是只保留一条抽象目标。常见维度包括：

- `functional_behavior`
- `external_contract`
- `runtime_entrypoint`
- `configuration`
- `persistence_or_side_effect`
- `error_behavior`
- `streaming_or_long_running`
- `concurrency`
- `deliverable`

这些维度不是每次都必须全部出现，但只要 Goal / DoD 涉及，就必须进入 plan、execution、validation 和 acceptance coverage。

### Goal Coverage And DoD Mapping

- Goal leaf requirement 必须进入 acceptance coverage，而不是只依赖 LLM 自述。
- `satisfied` 的 Goal / DoD 项必须引用有效 `evidenceRefs`。
- `failed` / `skipped` 项必须给出 reason、impact 和 next actions。
- 如果验证命令只覆盖部分目标，未覆盖部分不得标记为 satisfied。

### Acceptance Dimensions

`software-dev` profile 可以声明领域维度，例如：

- Goal decomposition checklist
- acceptance dimensions
- evidence quality policy
- deliverable expectations
- runtime entrypoint integrity checks
- external contract / error path / streaming / persistence / concurrency 等验证维度

这些维度的业务解释由 profile/Soul 提供，core 只校验它们是否被覆盖。

### Evidence Policy

- build/test/lint command success 可以作为直接验证证据，但必须映射到具体 Goal / DoD 项。
- `patch/apply`、`mcp/tool_call`、`shell/command` 等动作证据可以支撑副作用、流式、错误路径和并发声明。
- `deliverableExpectations.evidenceRole` 决定交付物覆盖闭环：
  `primary_state_change` 必须由真实 workspace 状态变更或等价主证据支撑；
  `supplemental_evidence` 可以由 runtime 结果 Artifact、验证证据、版本控制证据或覆盖映射补齐。
  core runtime 只消费 profile 声明的元数据，不内置 software-dev 专属交付物语义。
- `file/read`、`file/list`、`mcp/resource_read` 这类只读证据只能作为辅助上下文，不能单独支撑动作类 satisfied 声明。
- weak / partial evidence 不得直接支撑 accepted result。

### Runtime Entrypoint Integrity

- 若 Goal 声明单二进制、CLI、服务或 API 交付，必须验证对应入口可用。
- 若项目存在显式入口目录、脚本、构建目标或启动命令，验证必须覆盖目标入口而不是只跑局部测试。
- 若存在多个候选入口，必须说明目标入口选择依据或未覆盖原因。

### External Contract And Branch Coverage

- 外部接口必须覆盖成功路径和失败路径。
- 若 Goal 涉及协议兼容、格式转换、流式、持久化或并发，验证必须覆盖相应关键分支。
- 命令通过但缺少入口完整性、错误路径、配置路径或并发路径覆盖时，应返回 failed / skipped assessment，而不是 accepted。

### Completion To Repair

- completion assessment 发现 coverage 缺口时，应优先转成可执行 repair graph，而不是停留在抽象描述。
- 如果缺口可以自动推进，应优先补代码、补测试、补验证命令，并绑定 `coverageTargets`。
- 只有当前 workspace、当前 graph evidence 和当前 contract 无法机械推进时，才应进入 failed assessment、repair、replan 或 governance。

### Deliverable Expectations

- 最终结果必须引用 source diff、validation evidence 和 coverage map。
- 默认不交付构建产物二进制，除非 Goal 明确要求且 artifact policy 允许。
- 若生成了二进制、日志或临时文件，应说明是否属于正式交付范围。

## Runtime Capability Mapping

`software-dev` profile 常见覆盖维度与 step kind 的稳定映射方向：

| 覆盖维度 | 优先 step kind / 证据 |
| --- | --- |
| `functional_behavior` | `shell/command`、`validation`、必要时 `mcp/tool_call` |
| `external_contract` | `shell/command`、`web/fetch`、`mcp/tool_call` |
| `runtime_entrypoint` | `shell/command`、必要时 `git/read` |
| `configuration` | `file/read`、`patch/preview`、`patch/apply` |
| `persistence_or_side_effect` | `patch/apply`、`shell/command`、`mcp/tool_call` |
| `error_behavior` | `shell/command`、`validation`、`web/fetch` |
| `streaming_or_long_running` | `shell/command`、运行时 event / observability |
| `concurrency` | `shell/command`、`validation`、必要时 runtime metrics |
| `deliverable` | `patch/apply`、`git/read`、`artifacts`、`results` |

约束：

- 这些映射属于 `software-dev` profile 对中立 step kind 的解释，不是 core runtime 固定规则。
- 若某个维度只存在只读证据，必须在 assessment 中明确其为辅助上下文，而不是直接完成证明。

## Evidence / Artifact / Read-Model Mapping

software-dev profile 的稳定证据面应优先复用：

- `Patch`
  - 表达源码、配置和文本交付面的状态变更
- `ValidationResult`
  - 表达 build/test/lint/contract checks 的正式验证结果
- `ToolCall`
  - 表达 shell、MCP、git、web 等外部动作证据
- `Artifact`
  - 表达最终交付清单、生成文件、验证报告、diff 摘要或附加物
- `Result`
  - 表达 Goal / DoD 级正式 closure

产品与读模型层的主要承接面：

- `run snapshot`
  - 查看 `taskProgress`、`results`、`artifacts`、`doDEvidence`
- `run observability`
  - 查看验证链、repair 链和 usage summary
- run detail pages
  - `results` / `patches` / `validation-results` / `artifacts`

## Verification Mapping

`software-dev` profile 当前主要由以下验证入口承接：

- `scripts/verify-workorch-runtime-profile-software-dev.sh`
  - 校验 coverage / evidence / result 结构
- `scripts/verify-software-dev-goal-dod-e2e.sh`
  - 校验 patch / validation / artifact / result 闭环
- `scripts/verify-software-dev-code-agent.sh`
  - 校验 code-agent 边界、TUI/CLI 集成与治理约束
- `scripts/verify-software-requirement-development-orchestration.sh`
  - 校验复杂 software-dev orchestration 闭环

因此本 profile 文档中的任何规则，如果无法映射到上述 verifier / e2e 之一，
就不应被视为稳定约束。

## Cross-Layer Mapping

- runtime flow 收口点：
  `../../20-runtime-flow/verification-acceptance-flow.md`
- 过程证据与正式结果分层：
  `../../20-runtime-flow/evidence-result-flow.md`
- block / step kind 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- 运行证据与结果落点：
  `../../30-module-architecture/kernel-workspace-run-result.md`
- profile verifier：
  `../../60-verification/profile-software-dev-verifier.md`

## Boundary

这些规则只属于 `software-dev` profile：

- 不进入 core runtime contract
- 不进入 generic prompt
- 不进入通用 module/service 边界

## Source Documents

- `code-agent.md`
- `../../70-roadmap/active/software-requirement-development-orchestration-plan.md`
- `../../90-archive/completed/software-dev-code-agent-closure-plan.md`
