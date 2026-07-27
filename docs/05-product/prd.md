# WorkOrch PRD

Status: active

Type: product-requirements

Last Updated: 2026-07-14

## 1. Product Positioning

WorkOrch 是面向长流程任务的个人 AI 工作编排网关。

用户在一个 workspace 中定义 Goal 和 DoD，WorkOrch 根据当前上下文、Soul、Skill、MCP、Memory、运行证据和治理策略，自动组织 LLM 与工具执行，持续推进直到形成可验收结果、进入明确治理等待，或以可解释的失败终止。

WorkOrch 的核心价值不是“让 LLM 一次答对”，而是提供一条可审计、可恢复、可治理、可验证的长期执行链路。

## 2. Product Goals

### G1. Goal / DoD Driven Delivery

用户只需要定义目标和完成定义，系统必须围绕当前有效 Goal / DoD 推进。

要求：

- Goal / DoD 是正式定义，不是普通提示词片段。
- Goal / DoD 变更必须形成 definition revision。
- 每次 run / repair / replan（新 activation）都必须能追溯到当前有效 definition。
- 完成判断必须基于 accepted evidence 和 DoD coverage，而不是 LLM 自述。

### G2. Long-Running Workspace Execution

系统必须支持多轮、长上下文、跨工具、跨阶段的任务推进。

要求：

- 支持 planning、execution、verification、acceptance、repair 语义，以及 replan（新 activation / 新 Run）。
- 支持 state-changing output、validation evidence、governance blocker、artifact continuation。
- 支持服务重启后从 V2 aggregate 幂等再 drive 的可恢复运行态。
- 支持多 workspace 并发，workspace/run/ticket/approval 全程携带 scope。

### G3. Deterministic Runtime Authority

Runtime 必须是确定性状态转移的权威，LLM 只负责候选内容生成。

要求：

- 生产主链为 Runtime V2 only（`DriveRuntimeV2` + reducer + sealed dispatch）。
- LLM 合同违例只能记录为 rejected diagnostic evidence。
- 未通过合同的输出不能 seal、不能执行、不能接受、不能写 result/artifact/memory。
- Runtime 根据 facts、focus、requiredOutput、evidence ledger 和 governance gate 选择唯一 reducer action。
- 当 runtime 可机械推进时，不应直接退化为人工 decision。
- 不得依赖 V1 `StagePacket` / `NextAction` / graph dispatch / `ExecutionContextPatch`。

### G4. Evidence-Based Acceptance

系统必须用证据证明交付进展和完成度。

要求：

- state change、validation、approval、decision、artifact 都必须可追踪。
- accepted Result 必须引用 evidence refs；Result 不裁决，承接 Orchestrator 裁决。
- CoverageFocus / CoverageWorkItem 负责把 Goal / DoD / deliverable requirement 映射到 accepted/rejected evidence。
- 弱证据、截断快照、无效验证和合同违例不能作为 accepted evidence。

### G5. Governed Human Interaction

当系统缺少外部信息、权限、审批或策略选择时，必须进入结构化治理，而不是静默失败或继续乱跑。

要求：

- Decision Ticket 表达“需要用户做结构化选择”（`request_governance` / governed_wait）。
- Approval 表达“需要用户批准高风险或策略要求动作”。
- Resolution 必须能指向 continue（回 reducer）、replan（新 activation）或 terminate。
- Decision resolve 后 application 不得再强制 Resume；Web/TUI 只是提交治理动作和展示投影，不拥有业务状态机。

### G6. Context And Capability Governance

系统必须显式管理能力、上下文和记忆，避免让 LLM 从噪声中猜测。

要求：

- Soul 定义阶段约束、能力边界和风险策略。
- Skill 表达可复用指导。
- MCP Server / Capability 表达外部能力目录。
- Memory 只沉淀高价值可复用信息，不搬运事件日志。
- Memory recall 必须有 trace，且只把当前相关高价值内容注入上下文。

### G7. Product Surfaces Consistency

HTTP、Web Console、TUI、CLI/agent 必须看到一致的 workspace/run 状态。

要求：

- 服务端 projection / read model 是展示权威。
- Web/TUI 不自行重建业务状态机。
- TUI 和 Web 的操作入口只能提交 scoped command 或调用正式 API。
- 大 payload 通过 raw ref/detail 打开，不进入默认列表和主视图。

## 3. User Roles

| Role | Need | Primary Surface |
| --- | --- | --- |
| Operator | 创建 workspace、定义 Goal/DoD、启动/恢复/重规划、处理 decision/approval | TUI / Web Console |
| Builder | 让系统在项目目录中推进交付物，查看证据和结果 | TUI / Web Console |
| Reviewer | 核对 Result、Artifact、Validation、Event、Memory 和 runtime trace | Web Console / API |
| Maintainer | 配置 provider、MCP、Soul、template、运行参数和部署 | Web Console / API / config |
| Integrator | 通过 API 或 client 集成 WorkOrch 能力 | HTTP API / workorchclient |

## 4. Product Scope

### 4.1 Workspace Management

功能：

- 创建、删除、查看 workspace。
- 维护 deliverable directory 和 internal directory。
- 绑定 workspace template、默认 Soul、阶段 Soul、Skill、MCP、Memory。
- 展示 workspace projection、active run、task progress、outcome、governance state。

验收：

- workspace detail 可展示当前有效 Goal / DoD、active run、最新 outcome、当前 blocker。
- 多 workspace 下相同 run/ticket/approval ID 不得串 scope。
- 用户交付目录和 WorkOrch 内部状态目录严格分离。

### 4.2 Goal / DoD Definition

功能：

- 新增、修改、查看 Goal。
- 新增、修改、查看 DoD。
- 每次有效定义变化形成 definition revision 和 lineage。
- 支持基于当前 Goal / DoD 的 gap baseline。

验收：

- run activation 必须绑定明确的 GoalID、DoDID、definitionRevision。
- replan / resume / repair 必须继承或显式更新当前有效 definition。
- 不允许以历史摘要替代当前 Goal / DoD 原文。

### 4.3 Run Execution

功能：

- 创建 run、启动 run、恢复 run、workspace-level activation/restart。
- 通过 **Runtime V2** 主链推进：typed bootstrap → `DriveRuntimeV2` → reducer → prerequisite / candidate → sealed dispatch → evidence → terminal。
- 支持 LLM candidate 请求、sealed runtime operation（file/git/shell/MCP/web/ssh）、managed validation、artifact continuation。
- 支持 provider retry、contract violation retry（candidate attempt 0/1）、same-focus repair、governance wait。
- replan 语义仅通过 **新的** typed activation / 新 V2 Run 表达，不是同 Run 自动 successor。

验收：

- Start/Resume 只向 Run owner提交 typed intent；缺少 typed bootstrap 必须经 Result/Workspace/Run owner-local terminal delivery 收敛。
- 任何 candidate 输出在 seal / 执行前必须通过 requiredOutput / authority / execution contract gate。
- sealed operation 与 evidence commit 必须先完成合同/治理/evidence 判定，再推进 reducer。
- run 不得出现 `active` 且无进展、无 blocker、无合法 drive disposition 的撕裂状态（无 V1 `NextAction` / graph dispatch 兜底）。
- 不得将 `StagePacket`、`NextAction`、graph dispatch、`ExecutionContextPatch`、自动 replan successor 作为生产路径。

### 4.4 Evidence / Result / Artifact

功能：

- 记录 ToolCall、Patch、ValidationResult、RuntimeCheckpoint 等过程证据（观测/过程面）。
- 记录 accepted/rejected evidence（EvidenceLedger）。
- 维护 CoverageFocus / CoverageWorkItem。
- 由 Orchestrator 裁决后生成 Result 和 Artifact。
- 支持 raw payload/detail 按需打开。

验收：

- accepted Result 必须绑定 current Goal / DoD 和 accepted evidence refs；Result owner 不裁决。
- Artifact 必须有 workspace/run/result/过程证据归属之一，不得暴露本地绝对路径。
- 大 payload 默认只进入 bounded summary + raw ref。
- Step / checkpoint 不得被叙述为 V2 调度权威。

### 4.5 Governance

功能：

- 创建和展示 Decision Ticket。
- 提交 Resolution：provide_missing_info、reject_current_path、replan（新 activation 意图）。
- 创建和展示 Approval。
- 提交 approve/reject。
- 支持 batch resolve。

验收：

- 只有真实治理缺口才能产生 decision/approval。
- decision/approval resolved 后必须进入 scoped continuation：governance resolution commit 回 reducer，或 operator 发起新 activation / terminate。
- application 不得因 Decision resolve 再发 Resume。
- replan 必须明确为 **新** typed activation / 新 V2 Run（含 lineage），不是同 Run 自动 successor。

### 4.6 Context Registry

功能：

- 管理 Soul、WorkspaceTemplate、Skill、MCP Server、MCP Capability、Memory。
- 支持 workspace setup draft。
- 支持 stage policy、evidence policy、capability allowlist、Soul routing。

验收：

- template/Soul/profile 承载具体业务策略，core runtime 只消费结构化字段。
- Skill/MCP/Memory 选择必须能被追踪到 RuntimeBootstrap Activation 或 MemoryRecallTrace。
- 非软件开发模板不得受 software-dev profile 规则污染。

### 4.7 Product Interface

功能：

- HTTP API。
- Web Console。
- TUI。
- CLI/agent backend。
- SSE / event refresh。
- Deployment / configuration / metrics / health。

验收：

- Web/TUI 主视图消费 projection，而不是反复执行 full snapshot heavy 聚合。
- 写操作只能通过正式 API / command/event 入口。
- raw/detail 入口必须有 scope 校验。
- 接口不得把 Step/graph/RecoveryPlan 类字段叙述为 V2 调度权威。

## 5. Non-Functional Requirements

### Reliability

- EventHub command/result 必须有 owner scope、lane policy 和错误回传。
- 服务重启后 `active` 但无真实执行中的状态必须可从 V2 aggregate 刷新为可激活/可恢复状态。
- external interaction timeout 允许幂等补偿，非预期 event 丢失必须定位并修复主路径。

### Observability

- EventRecord、RunObservability、TaskProgress、ContextBundle、MemoryRecallTrace 必须覆盖主要链路。
- projection 必须带 revision/health。
- live verification 必须记录 workspaceId、runId、focus/decision/approval/replan/resume 时间和最终状态。

### Performance

- 列表和主视图不得默认加载无界 raw payload。
- TUI/Web refresh 不得持续触发 full detail + JSON map 重建。
- 历史上下文使用“近期完整数据 + 历史摘要/引用”的 compact 策略。

### Security / Safety

- public API 不暴露内部绝对路径。
- 高风险 state-changing action 走 Approval 或 policy gate。
- Provider-specific 字段不溢出 core runtime。
- credential、token、secret、private key 不进入 Memory 或默认事件投影。

### Neutrality

- core 不内置软件开发、代码语言、测试框架、业务场景、模板 ID、Soul ID 的特判。
- 具体策略通过 workspaceTemplate / Soul / profile / capability / skill 表达。

## 6. Out Of Scope

- WorkOrch core 直接判断某个业务领域的正确方案。
- Web/TUI 在前端重建业务状态机。
- LLM 合同违例输出直接执行。
- 把事件日志、provider retry 流水或低价值诊断写成长记忆。
- 把 software-dev 的文件、命令、测试框架规则写入 kernel/orchestrator。
- 将 V1 StagePacket / NextAction / graph dispatch / ExecutionContextPatch / 自动 replan successor 作为生产路径。

## 7. Acceptance Criteria

- 用户可从 Web/TUI 创建 workspace、定义 Goal/DoD、启动执行、观察进展、处理 decision/approval、查看结果和证据。
- 任意 run 的当前状态可从 owner state、V2 aggregate、projection、event timeline 中互相解释。
- 任意 accepted result 可追溯到 current Goal / DoD、coverage items 和 accepted evidence refs。
- 任意 same-focus repair / resume 都继承当前有效 focus/requiredOutput 合同；replan 通过新 activation 保留 definition lineage 与显式 carry-over 规则，不丢失 accepted/rejected evidence 审计。
- 任意 UI 展示状态不得和 backend projection 权威状态冲突。
- 任意 profile-specific 策略都能定位到 template/Soul/profile/capability，而不是 core runtime 特判。
- 生产路径可证明为 Runtime V2 only。
