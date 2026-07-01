# WorkOrch Product Architecture

Status: active

Type: product-architecture

Last Updated: 2026-07-01

## 1. Architecture Purpose

本文从产品视角整理 WorkOrch 的架构层级，说明一个 Goal / DoD 如何经过产品入口、应用编排、kernel owner state、runtime block、shared contract、read model 和验证体系，最终形成可审计交付。

本文不新增 runtime core 规则；runtime 规则以 `../00-core/runtime-stage-contract.md` 为准。

## 2. Architecture Principles

| Principle | Meaning |
| --- | --- |
| Kernel owns business state | Workspace / Run / Step / Result / Decision / Approval 的正式状态只由 kernel owner 维护 |
| Blocks own side effects | LLM、MCP、shell、file、git、web、ssh 等外部动作由 blocks 执行 |
| Application owns composition | HTTP/TUI/server/bootstrap/appservice 只做请求校验、use-case 组合、read-model 装配 |
| Shared packages own contracts | common types、runtime contract、owner contract、repository、event envelope、port/client DTO 位于 `internal/pkg` |
| UI consumes projection | Web/TUI 展示服务端 projection，不复制业务状态机 |
| Profile owns specialization | software-dev、web-search、workspace setup、MCP 策略等属于 template/Soul/profile/capability |

## 3. Architecture Levels

### 3.1 Product Experience Level

职责：

- Web Console：多 workspace 管理、详情、governance、configuration、MCP/template/Soul 管理。
- TUI：本地 agent-first 执行、workspace 切换、decision/approval 输入、进度观察。
- HTTP API：集成入口、Web Console backend、remote backend。
- CLI/agent：本地命令入口和自动化入口。

不负责：

- 不直接修改正式业务状态。
- 不自行判断 Goal / DoD 完成。
- 不从 raw payload 重建业务状态机。

### 3.2 Application / Service Level

代码落点：

- `application/workorch`
- `internal/initiators/workorchruntime`
- `internal/modules/application/workorchd`
- `internal/services/workorch`

职责：

- process startup、mode selection、framework lifecycle。
- repository、event hub、runtime policy、provider client、projection store wiring。
- HTTP route、request validation、response envelope。
- appservice use-case 组合。
- observed / presenter / read-model projection。
- TUI embedded backend 和 remote backend 的端口适配。

不负责：

- 不拥有 Workspace / Run / Result / Decision / Approval 的正式状态。
- 不绕过 EventHub/owner service 执行业务状态转换。
- 不复制 orchestrator 收敛判断。

### 3.3 Kernel Business Authority Level

代码落点：

- `internal/modules/kernel/workspace`
- `internal/modules/kernel/run`
- `internal/modules/kernel/result`
- `internal/modules/kernel/orchestrator`
- `internal/modules/kernel/decision`
- `internal/modules/kernel/approval`
- `internal/modules/kernel/governance`
- `internal/modules/kernel/soul`
- `internal/modules/kernel/skill`
- `internal/modules/kernel/mcp`
- `internal/modules/kernel/memory`
- `internal/modules/kernel/eventstream`

职责：

- Workspace / Goal / DoD formal definition。
- Run / Step lifecycle。
- ToolCall / Patch / ValidationResult / RuntimeCheckpoint 过程证据。
- Result / Artifact 正式 outcome。
- Decision / Approval / Resolution 治理对象。
- Soul / Skill / MCP / Memory 上下文与能力对象。
- EventRecord 持久化与 replay 输入。
- Orchestrator 收敛、dispatch、repair、replan、completion gate。

不负责：

- 不直接实现 provider 协议。
- 不直接执行 shell/git/file/web/ssh 外部动作。
- 不定义 Web/TUI 页面交互。
- 不内置 profile-specific 业务策略。

### 3.4 Runtime Block / Adapter Level

代码落点：

- `internal/modules/blocks/llm`
- `internal/modules/blocks/mcp`
- `internal/modules/blocks/shell`
- `internal/modules/blocks/file`
- `internal/modules/blocks/git`
- `internal/modules/blocks/web`
- `internal/modules/blocks/ssh`

职责：

- 执行 provider call 或外部 side effect。
- 将输出标准化为 runtime 可消费结果。
- 提供 bounded summary、structured metadata、raw ref。
- 对外部动作做 adapter-local 安全限制和错误分类。

不负责：

- 不决定 Run 是否 completed。
- 不创建 accepted Result。
- 不替代 DoD assessment。
- 不持有 workspace business state。

### 3.5 Shared Contract / Infrastructure Level

代码落点：

- `internal/pkg/common`
- `internal/pkg/workorchruntimecontract`
- `internal/pkg/workorchownercontract`
- `internal/pkg/workorchport`
- `internal/pkg/workorchclient`
- `internal/pkg/events`
- `internal/pkg/runtimeevent`
- `internal/pkg/repository`
- `internal/pkg/workorchrepo`
- `internal/pkg/workorchconfig`
- `internal/pkg/runtimepath`
- `internal/pkg/workorchdefaults`
- `internal/pkg/workorchruntimepolicy`

职责：

- common IDs and states。
- runtime module / block kind / StagePacket / CoverageWorkItem / ContractViolation。
- owner contract DTO。
- CLI/TUI/agent port DTO。
- REST client DTO and path/error mapping。
- event envelope、hub、lane policy、topic contract。
- repository interfaces and file/memory backend。
- config/defaults/runtime policy。
- path normalization and safe segment。

边界：

- shared package 只能表达中立 contract / infrastructure。
- 若存在聚合型 package 需要引用 owner event/model，必须保持“聚合 contract”语义，不得加入业务裁决。
- REST/client/port DTO 必须复用 runtime vocabulary，避免多端各自维护常量。

### 3.6 Storage / Read Model Level

职责：

- `.workorch-data/internal/workspaces/<workspace-id>/` 保存 WorkOrch 内部状态。
- `.workorch-workspaces/<workspace-id>/` 保存用户交付物。
- itemized run storage 保存 step、tool call、patch、validation、checkpoint、llm call、artifact continuation。
- append-only event storage 支持审计和 replay。
- projection store 支撑 workspace list/detail、run snapshot/status/observability。
- raw payload ref 支持按需打开。

边界：

- read model 是投影，不是 owner state。
- event stream 是审计/回放输入，不是唯一业务状态源。
- raw payload 不默认进入列表和主视图。

### 3.7 Profile / Capability Level

文档落点：

- `../40-profiles-capabilities/`

职责：

- Workspace template。
- Soul routing and stage policy。
- Evidence policy。
- MCP capability management。
- software-dev code-agent / DoD / LSP / file granularity guidance。
- runtime capability profile。

边界：

- profile 可定义“某类任务应该如何使用能力”。
- core runtime 只消费结构化字段，不识别模板名、语言、文件、命令或业务场景。

### 3.8 Verification / Governance Level

文档落点：

- `../60-verification/`

职责：

- acceptance records。
- code alignment checklist。
- runtime verifier。
- profile verifier。
- service acceptance。
- TUI/Web acceptance。
- audit reports。

边界：

- verification 文档证明实现符合合同，不新增 runtime 规则。
- e2e fixture 可以具体，但断言应证明通用能力消费结构化字段。

## 4. End-to-End Architecture Flow

```text
User
  -> TUI / Web / API
  -> application/workorchd appservice
  -> EventHub command or repository/read-model query
  -> kernel owner state
  -> orchestrator builds ConvergenceContract / StagePacket / requiredOutput
  -> blocks execute LLM / MCP / shell / file / git / web / ssh
  -> kernel records evidence / updates ledger / decides next action
  -> result/artifact/governance/eventstream persist formal outcome
  -> projection store updates Workspace / Run view
  -> TUI / Web / API display projection
```

## 5. State Authority Architecture

| State / View | Authority | Consumers |
| --- | --- | --- |
| WorkspaceState | kernel workspace + read-model projection | Web/TUI/API |
| RunState | kernel run | orchestrator, appservice, projection |
| StepState | kernel run/step | orchestrator, observability |
| ConvergenceContract | kernel orchestrator | LLM input, next action, diagnostics |
| CoverageWorkItem | kernel run coverage ledger | completion gate, projection |
| ResultState | kernel result | workspace outcome, run outcome |
| Decision/Approval State | kernel governance/decision/approval | Web/TUI governance views |
| Projection revision/health | workorchd projection | Web/TUI/API |

## 6. Interaction Architecture

### 6.1 Web Console

Allowed write surfaces:

- workspace create/delete/start/restart/activation。
- Goal / DoD add and update。
- Soul binding and catalog management。
- WorkspaceTemplate management。
- Workspace setup draft。
- Decision resolve and Approval approve/reject。
- configuration。
- MCP server/capability management。

Read-only requirements:

- Web Console must not own business state.
- Web Console must not infer completion or next action.
- Web Console must consume projection and submit scoped commands.

### 6.2 TUI

Allowed write surfaces:

- workspace selection and execution command。
- resume/restart/replan through unified scoped runtime command。
- decision/approval input。

Read-only requirements:

- TUI prompt mode comes from projection/port DTO。
- TUI must not duplicate backend state machine.
- TUI must not resolve unscoped run/ticket/approval when multiple workspace objects conflict.

### 6.3 HTTP API / Client

Requirements:

- workspace-scoped run operations use `/workspaces/:workspaceId/runs/:runId/...`。
- unscoped list endpoints may exist only as read/list helpers and must not create ambiguous state-changing operations。
- client DTO and server response families must remain aligned。
- raw content endpoints must validate logical URI/ref scope.

## 7. Deployment Architecture

Runtime can run as:

- local TUI process。
- server/workorchd process。
- containerized service。
- remote backend used by client/TUI。

Deployment requirements:

- config separates server/runtime/LLM/web-search sections。
- provider-specific config belongs to adapter/config, not core state machine。
- metrics/health/ready support operational checks。
- service restart recovery must refresh inactive running projections and allow manual/explicit continuation.

## 8. Architecture Invariants

- `application/workorch` remains thin main.
- all module/block/initiator registration flows through the application entry/wiring.
- kernel production code must not import blocks implementation.
- blocks production code must not import kernel repository/service implementation.
- Web/TUI must not directly call kernel owner repositories.
- shared runtime vocabulary must come from `workorchruntimecontract` or equivalent central contract.
- profile-specific logic must not appear in kernel/orchestrator/appservice core paths.
- read-model projection must not become business state owner.

## 9. Source Mapping

| Product Architecture Section | Authority Source |
| --- | --- |
| runtime neutrality and stage authority | `../00-core/runtime-stage-contract.md` |
| domain object ownership | `../10-domain-model/` |
| activation/execution/repair/verification flow | `../20-runtime-flow/` |
| module paths and dependencies | `../30-module-architecture/` |
| profiles and capability policies | `../40-profiles-capabilities/` |
| API/TUI/Web/deployment | `../50-product-interface/` |
| acceptance and verification | `../60-verification/` |

