# WorkOrch Product Architecture

Status: active

Type: product-architecture

Last Updated: 2026-07-14

## 1. Architecture Purpose

本文从产品视角整理 WorkOrch 的架构层级，说明一个 Goal / DoD 如何经过产品入口、应用编排、kernel owner state、runtime block、shared contract、read model 和验证体系，最终形成可审计交付。

本文不新增 runtime core 规则；runtime 规则与 **Runtime V2 生产主链** 以 `../00-core/runtime-stage-contract.md` 为准。产品叙述采用双层语言：中立 stage 语义 / profile 意图 + V2 authority / focus / sealed dispatch 实现。

## 2. Architecture Principles

| Principle | Meaning |
| --- | --- |
| Kernel owns business state | Workspace / Run / Result / Decision / Approval 的正式状态只由 kernel / stateful block owner 维护；Runtime V2 aggregate（facts / authority / evidence ledger）由 Orchestrator 维护 |
| Blocks own side effects | LLM、MCP、shell、file、git、web、ssh 等外部动作由 blocks 执行；正式入口为 sealed dispatch 后的 typed operation |
| Application owns composition | HTTP/TUI/server/bootstrap/appservice 只做请求校验、use-case 组合、read-model 装配；Start/Resume 进入 V2 drive，不持有 execution-context patch 或 continuation queue |
| Shared packages own contracts | common types、runtime contract、owner contract、repository、event envelope、port/client DTO 位于 `internal/pkg` |
| UI consumes projection | Web/TUI 展示服务端 projection，不复制业务状态机，不推断 next action |
| Profile owns specialization | software-dev、web-search、workspace setup、MCP 策略等属于 template/Soul/profile/capability；kernel 不理解 profile ID |

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
- 不推断 RuntimeDecision / next action。

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
- 构造 typed activation / `RuntimeBootstrap` 输入；发出 Start / Resume / governance resolution 命令。

不负责：

- 不拥有 Workspace / Run / Result / Decision / Approval 的正式状态。
- 不绕过 EventHub/owner service 执行业务状态转换。
- 不复制 orchestrator 收敛判断。
- 不因 Decision resolve 再 Resume；不持有 application Start/Resume continuation queue。
- 不通过 `ExecutionContextPatch` 或 V1 lifecycle facade 推进 run。

### 3.3 Business Authority Module / Stateful Block Level

代码落点：

- `internal/modules/blocks/workspace`
- `internal/modules/kernel/run`
- `internal/modules/blocks/result`
- `internal/modules/kernel/orchestrator`
- `internal/modules/blocks/decision`
- `internal/modules/blocks/approval`
- `internal/modules/kernel/governance`
- `internal/modules/blocks/soul`
- `internal/modules/blocks/skill`
- `internal/modules/blocks/mcpregistry`
- `internal/modules/blocks/memory`
- `internal/modules/kernel/eventstream`

其中 `internal/modules/blocks/workspace`、`result`、`decision`、`approval`、`memory`、`skill`、`soul` 与 `mcpregistry` 分别承载单一资源聚合 stateful block；`mcpregistry` 与 invocation 技术 block `blocks/mcp` 明确分离。其余目录按各自职责保留为 module。职责：

- Workspace / Goal / DoD formal definition。
- Run lifecycle 与 Run owner CAS（含 `RuntimeStateRecord` / V2 aggregate 持久化面）。
- ToolCall / Patch / ValidationResult 等过程证据；V2 operation evidence 与 sealed dispatch 绑定。
- Result / Artifact 正式 outcome（Result 不裁决，只承接 Orchestrator 裁决后的 accepted/rejected）。
- Decision / Approval / Resolution 治理对象；governance ticket 由 `request_governance` 打开，resolution commit 后回 reducer。
- Soul / Skill / MCP / Memory 上下文与能力对象（作为 activation / candidate 输入，不作为 kernel 状态机）。
- EventRecord 持久化与 replay 输入。
- Orchestrator：`DriveRuntimeV2`、reducer、authority transaction、candidate、sealed dispatch、focus/evidence ledger、repair 同 focus 再收敛、terminal projection。

不负责：

- 不直接实现 provider 协议。
- 不直接执行 shell/git/file/web/ssh 外部动作（经 sealed operation 委托 blocks）。
- 不定义 Web/TUI 页面交互。
- 不内置 profile-specific 业务策略。
- 不生产 `StagePacket` / `NextAction` / graph dispatch / 自动 replan successor。

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

- 执行 provider call 或外部 side effect（candidate 请求或 sealed operation）。
- 将输出标准化为 runtime 可消费结果 / evidence 载荷。
- 提供 bounded summary、structured metadata、raw ref。
- 对外部动作做 adapter-local 安全限制和错误分类。

不负责：

- 不决定 Run 是否 completed。
- 不创建 accepted Result。
- 不替代 DoD assessment。
- 不持有 workspace business state。
- 不充当 V1 step graph dispatcher。

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
- Runtime V2 typed contract：bootstrap / facts / focus / authority / requiredOutput / candidate / sealed dispatch / evidence / state record；中立 stage 语义与 coverage 相关词汇。
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
- **不得** 将已废止的 V1 `StagePacket` / `NextAction` / graph dispatch 合同重新定义为生产 vocabulary。

### 3.6 Storage / Read Model Level

职责：

- `.workorch-data/internal/workspaces/<workspace-id>/` 保存 WorkOrch 内部状态。
- `.workorch-workspaces/<workspace-id>/` 保存用户交付物。
- run storage 保存 evidence、operation 过程对象、checkpoint（诊断/观测面）、artifact continuation 等。
- append-only event storage 支持审计和 replay。
- projection store 支撑 workspace list/detail、run snapshot/status/observability。
- raw payload ref 支持按需打开。
- V2 aggregate / `RuntimeStateRecord` 由 Run owner CAS 持久化；恢复 = load aggregate，不重放 V1 progress/terminal checkpoint 作为驱动。

边界：

- read model 是投影，不是 owner state。
- event stream 是审计/回放输入，不是唯一业务状态源。
- raw payload 不默认进入列表和主视图。
- Step / graph / RecoveryPlan 类字段若出现在 projection，仅为观测/诊断，不是生产调度权威。

### 3.7 Profile / Capability Level

文档落点：

- `../40-profiles-capabilities/`

职责：

- Workspace template。
- Soul routing and stage **policy**（语义层输入）。
- Evidence policy。
- MCP capability management。
- software-dev code-agent / DoD / LSP / file granularity guidance。
- runtime capability profile。
- stage policy / focus 作为 C2 / activation 输入进入 candidate 上下文。

边界：

- profile 可定义“某类任务应该如何使用能力”。
- core runtime 只消费结构化字段，不识别模板名、语言、文件、命令或业务场景，**不理解 profile ID**。

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
- V2 drive / live gate 验收维度。
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
  -> seal typed RuntimeBootstrap (Definition + Activation + Focuses + Requirements)
  -> Run created; Start: AcceptRunStart (pending→active) then DriveRuntimeV2
     Resume: workspace/run scope only; active or resolved blocker; same drive
  -> reducer(RuntimeFacts) → RuntimeDecision + AuthorityTransaction
  -> acquire_prerequisite | produce_* | request_governance | complete | fail
  -> candidate (attempt 0/1) → SealDispatch → ExecuteRuntimeOperation (blocks)
  -> evidence / assessment / governance commit → reducer
  -> governed_wait projects Run blocked (no application auto-Resume)
  -> Result accepted/rejected via Result owner; Run / Workspace terminal projection
  -> projection store updates Workspace / Run view
  -> TUI / Web / API display projection
```

## 5. State Authority Architecture

| State / View | Authority | Consumers |
| --- | --- | --- |
| WorkspaceState | blocks/workspace + read-model projection | Web/TUI/API |
| RunState | kernel/run | orchestrator, appservice, projection |
| RuntimeFacts / AuthoritySnapshot / EvidenceLedger / Focus queue | kernel orchestrator aggregate | DriveRuntimeV2, diagnostics |
| RuntimeDecision / RequiredOutputContract / SealedDispatch | orchestrator reducer / seal | candidate, operation execution, evidence gate |
| RuntimeStateRecord | Run owner CAS | recovery, projection, audit |
| Step / process evidence projection | run storage + read-model（观测/legacy 过程面） | observability；**非** V2 调度权威 |
| ResultState | blocks/result（承接 Orchestrator 裁决） | workspace outcome, run outcome |
| Decision/Approval State | decision/approval + governance | Web/TUI governance views |
| Projection revision/health | workorchd projection | Web/TUI/API |

Canonical state sets:

- `WorkspaceState`: `active` / `blocked_on_approval` / `superseded`。Decision blockers are expressed through run/ticket/projection fields rather than workspace owner state.
- `RunState`: `pending` / `active` / `blocked_on_decision` / `blocked_on_approval` / `completed` / `failed` / `cancelled` / `superseded`。
- Step 过程状态若仍出现在 storage/projection：`pending` / `executing` / `succeeded` / `failed` / `skipped` / `cancelled`——仅作过程证据/观测，不作为 V2 reducer 生产状态枚举。

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
- Web Console must not infer completion or RuntimeDecision / next action.
- Web Console must consume projection and submit scoped commands.

### 6.2 TUI

Allowed write surfaces:

- workspace selection and execution command。
- resume / restart / **replan 语义**（新 activation 或 replacement run）through unified scoped runtime command。
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
- Start / Resume 返回 accepted command 语义；后续状态以 projection / run snapshot 为准。
- Decision resolve **不得**要求 application 再发 Resume 才能回到 drive；governance resolution commit 后由 runtime 路径回 reducer。

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
- service restart recovery must load persisted V2 aggregate / refresh inactive `active` run projections；continuation is explicit Start/Resume drive or governance resolution path, not V1 checkpoint replay.

## 8. Architecture Invariants

- `application/workorch` remains thin main.
- all module/block/initiator registration flows through the application entry/wiring.
- kernel production code must not import blocks implementation.
- blocks production code must not import kernel repository/service implementation.
- Web/TUI must not directly call kernel owner repositories.
- shared runtime vocabulary must come from `workorchruntimecontract` or equivalent central contract.
- profile-specific logic must not appear in kernel/orchestrator/appservice core paths.
- read-model projection must not become business state owner.
- production drive is Runtime V2 only; V1 StagePacket / NextAction / graph dispatch / ExecutionContextPatch / auto replan successor are not production paths.
- Start: AcceptRunStart then DriveRuntimeV2; Resume: scoped, no execution-context patch; governance wait does not auto-Resume from application.

## 9. Source Mapping

| Product Architecture Section | Authority Source |
| --- | --- |
| runtime neutrality, stage semantic, Runtime V2 mainline | `../00-core/runtime-stage-contract.md` |
| domain object ownership | `../10-domain-model/` |
| activation / drive / repair / verification flow | `../20-runtime-flow/` |
| module paths and dependencies | `../30-module-architecture/` |
| profiles and capability policies | `../40-profiles-capabilities/` |
| API/TUI/Web/deployment | `../50-product-interface/` |
| acceptance and verification | `../60-verification/` |
| orchestrator implementation note | `../../internal/modules/kernel/orchestrator/readme.md` |
