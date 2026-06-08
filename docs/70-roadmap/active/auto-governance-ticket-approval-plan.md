# Auto Governance Ticket / Approval Plan

Status: active

Type: roadmap

Last Updated: 2026-05-30

## Stable Definition Entry

以下稳定定义已迁入正式结构，不再由本 roadmap 文档重复维护正文：

- `../../10-domain-model/governance-decision-approval.md`
- `../../20-runtime-flow/repair-replan-flow.md`
- `../../30-module-architecture/kernel-orchestrator-governance.md`
- `../../30-module-architecture/storage-readmodel.md`
- `../../40-profiles-capabilities/mcp/capability-management.md`
- `../../50-product-interface/service-api.md`
- `../../50-product-interface/tui/interaction-contract.md`
- `../../50-product-interface/web-console/overview.md`
- `../../60-verification/core-runtime-verifier.md`

## Purpose

本文件只保留 auto governance 的后续跟踪项：自动化策略、预算、风险分级、
展示补强和验收补全。

它不再承接以下稳定正文：

- Decision / Approval 的领域对象边界
- 恢复链与治理恢复动作的正式 flow 定义
- governance kernel / read-model / API 主体设计
- TUI / Web 的治理交互基础契约
- 已完成能力的 dated closure 记录

## 背景

本文档用于跟踪 workorch 通过 LLM 辅助自动处理 Ticket 和 Approval 的设计、边界、任务状态与验收标准。

目标不是让 LLM 无约束地自动回答和自动批准，而是在 `Run / Goal / DoD / Ticket / Approval / Memory / Event / Artifact` 的治理闭环内，尽量减少用户被中断的次数。

核心原则：

- Ticket 尽量自动补全可推导信息。
- Approval 只在 deterministic policy 允许时自动批准。
- 高风险、越权、超出 DoD 或上下文不足的事项必须升级人工处理。
- 每一次自动处理都必须可追溯、可审计、可在 Web/TUI 中解释。

上述原则的正式长期定义以后续查看 stable definition entry 为准；本 roadmap
只跟踪还未完全收口的自动化部分。

## 目标

### 用户目标

用户提交 Goal 和 DoD 后，workorch 能够自动完成大部分常规澄清和低风险授权：

- 从 Goal、DoD、Workspace 配置、Memory、历史 Run 中推导 Ticket 答案。
- 对只读操作、低风险 workspace 内写入等 Approval 自动批准。
- 对风险较高或语义不明确的 Ticket / Approval 保留人工确认。
- 在 Web/TUI 中清楚看到哪些事项被自动处理、为什么自动处理、引用了哪些证据。

### 系统目标

- 新增 Auto Governance Agent，订阅 Ticket / Approval / Run blocked 相关事件。
- Ticket 自动处理采用 `LLM-first, policy-guarded`。
- Approval 自动处理采用 `policy-first, LLM-explained`。
- 所有自动处理必须检查当前 Goal / DoD 约束。
- 自动处理结果必须写入 eventstream、trace、read model，并可被 e2e 验证。
- 不引入绕过现有 Ticket / Approval repository 和 appservice 的旁路。

## 非目标

- 不允许 LLM 自动批准所有 Approval。
- 不允许 LLM 绕过 policy 直接调用 repository 修改审批状态。
- 不把自动治理做成 TUI/Web 私有逻辑。
- 不把自动处理过程只记录在日志中。
- 不在第一版处理生产发布、凭据访问、大规模删除等高风险自动批准。

## 当前稳定基线

已存在或可复用能力：

- `Run / Goal / DoD` 编排模型。
- Ticket / Decision 与 Approval 业务模型。
- EventHub 和 EventStream。
- Memory recall traceability。
- Runtime block 和 step evidence。
- Web/TUI/CLI 对 Ticket、Approval、Run、Result、Artifact 的基础展示。
- workspaceTemplate、Soul、MCP 能力约束。

当前仍待补齐：

- Auto Governance Agent。
- Ticket / Approval 自动处理策略和配置。
- 风险分级模型。
- 自动化预算。
- 自动处理 trace。
- Web/TUI 展示自动处理依据。
- e2e 验收脚本。

## 本轮核对结果

### 已稳定并已迁入正式结构

- Ticket / Decision 与 Approval 的对象边界、resolution / lineage / recovery mapping
  已迁入 `../../10-domain-model/governance-decision-approval.md`
- repair / governance / blocker / resume / replan 的恢复链边界已迁入
  `../../20-runtime-flow/repair-replan-flow.md`
- governance kernel / orchestrator 分工已迁入
  `../../30-module-architecture/kernel-orchestrator-governance.md`
- API、Web、TUI 的基础治理读写边界已迁入
  `../../50-product-interface/service-api.md`、
  `../../50-product-interface/tui/interaction-contract.md`、
  `../../50-product-interface/web-console/overview.md`

### 当前仍未完全收口

- Auto Governance Agent 的独立运行单元和订阅链仍待补齐
- Ticket 自动回答与 Approval 自动批准的风险分级/预算策略仍需继续固化
- 自动治理 trace、read-model 投影和 Web/TUI 说明仍需补强
- e2e / verifier 对自动治理场景的专门覆盖仍可继续补齐

## 剩余迁移关注点

### Auto Governance Agent

当前只跟踪仍需补齐的职责：

- 订阅 Ticket / Approval / Run blocked 相关 EventHub 事件。
- 拉取当前 Workspace、Run、Goal、DoD、Memory recall、Tool evidence、Artifact、历史处理记录。
- 调用 LLM 生成结构化建议。
- 调用 deterministic policy 判断是否允许自动处理。
- 通过现有 appservice/repository 路径提交 Ticket answer 或 Approval resolution。
- 记录 Auto Governance Trace。

要求：

- 不直接修改底层 JSON 文件。
- 不绕过现有业务服务。
- 不在 TUI/Web 中复制判断逻辑。
- 不引入定时轮询。

### Ticket 边界

Ticket 的正式对象边界已迁入 stable definition entry。这里仅保留自动处理 follow-up：

可自动处理：

- 答案能从 Goal / DoD / Workspace 配置中明确推导。
- 答案能从 Memory recall 或历史同类 Run 中明确推导。
- 属于格式补全，例如默认城市、默认分支、默认语言、默认路径、默认测试命令。
- 可通过只读工具补齐，例如文件读取、配置查询、Web Search。

必须升级人工：

- 存在多个合理答案且会影响执行方向。
- 会改变 Goal 或 DoD 范围。
- 涉及凭据、生产环境、远程写操作、外部发布、成本消耗。
- LLM 置信度低于阈值。

### Approval 边界

Approval 的正式对象边界已迁入 stable definition entry。这里仅保留自动批准 follow-up：

Approval 不能只根据 LLM 文本自动批准，必须由 deterministic policy 决定是否允许自动处理。

可自动批准：

- 只读操作。
- workspace 管理目录内低风险写入。
- 已由 DoD 明确授权的测试命令。
- 小范围 patch，且路径在 workspace 内，不触碰敏感文件。
- workspaceTemplate / Soul / MCP 明确绑定并允许的低风险工具调用。

必须人工审批：

- SSH 到远程主机执行写操作。
- 删除、覆盖、移动大量文件。
- 修改 `.ssh`、密钥、凭据、CI/CD、部署配置。
- 生产环境操作。
- 外部发布、付费 API、大批量调用。
- LLM 置信度不足或 policy 无法判断。

## 风险分级

| 级别 | 类型 | 默认处理 |
| --- | --- | --- |
| L0 | 纯信息展示，不产生副作用 | 不需要 Approval |
| L1 | 只读操作，例如文件读取、配置查询、Web Search | 可自动批准 |
| L2 | workspace 内低风险写入，例如生成 artifact、写受控 patch | 满足 policy 时可自动批准 |
| L3 | 外部系统或有副作用操作，例如 MCP 写操作、远程命令、依赖安装 | 默认人工，后续可按模板策略逐步放开 |
| L4 | 凭据、生产、删除、发布、高成本操作 | 必须人工 |

## Policy 配置模型

建议复用 workorch 整体配置框架，不新增独立配置文件。

示例：

```yaml
governance:
  autoTicket:
    enabled: true
    minConfidence: 0.82
    maxPerRun: 5
  autoApproval:
    enabled: true
    minConfidence: 0.9
    maxPerRun: 3
    allowReadOnly: true
    allowWorkspaceWrite: true
    requireHumanFor:
      - ssh_write
      - secret_access
      - external_publish
      - destructive_file_change
      - production_operation
  budget:
    maxConsecutiveFailures: 2
    maxRepeatedSamePrompt: 1
```

配置层级建议：

- system default。
- workspaceTemplate override。
- workspace override。
- run activation override。

优先级从低到高，run activation 只允许收紧策略，不允许扩大系统禁用项。

## LLM 上下文组织

Auto Governance Agent 调用 LLM 时必须使用结构化上下文。

建议输入：

- governance item：
  - ticketId / approvalId
  - type
  - prompt
  - requestedAction
  - requestedContext
- run context：
  - workspaceId
  - runId
  - goal
  - DoD
  - current stage
  - blocked reason
- evidence：
  - memory recall refs
  - step evidence refs
  - artifact refs
  - file diff summary
  - tool call summary
- policy context：
  - risk level candidate
  - allowed actions
  - forbidden actions
  - automation budget remaining

建议输出：

```json
{
  "canAutoResolve": true,
  "action": "answer|approve|reject|escalate",
  "answer": "string",
  "riskLevel": "L1",
  "confidence": 0.91,
  "reason": "inferred from workspace memory and DoD",
  "evidenceRefs": ["memory-recall-xxx", "step-yyy"],
  "policySignals": ["read_only", "within_dod"],
  "requiresHumanReason": ""
}
```

Ticket 自动补全不从自然语言 Goal/DoD 中猜测业务字段。LLM 或模板侧需要在 `requiredDecision.context` 中提供通用结构化证据，core 只消费这些中立字段：

```json
{
  "knownValues": {"source": "workspace"},
  "missingFields": ["target"],
  "candidateValues": {"target": "artifact"},
  "confidenceByField": {"target": 0.92},
  "requiresHumanFields": []
}
```

- `knownValues` 表示已确认信息。
- `missingFields` 表示当前 ticket 请求补齐的字段。
- `candidateValues` 表示可由 LLM/模板上下文给出的候选值。
- `confidenceByField` 或顶层 `confidence` 达到系统阈值时，core 才允许自动回答。
- `requiresHumanFields`、`humanRequired`、`manualRequired` 出现时，core 必须升级人工处理。

## DoD 约束

自动处理前必须判断当前 Ticket / Approval 是否仍服务于当前 Goal / DoD。

规则：

- 与 Goal / DoD 无关的事项不能自动处理。
- 会扩大 Goal / DoD 范围的事项不能自动处理。
- DoD 已明确授权的测试、验证、只读查询，可降低风险级别。
- DoD 未覆盖但属于必要前置条件时，可以生成 Ticket 询问用户，不应直接自动扩大范围。

## 自动化预算

为避免自动循环失控，每个 Run 需要预算限制：

- Ticket 自动回答次数上限。
- Approval 自动批准次数上限。
- 连续失败次数上限。
- 同一 prompt 重复出现次数上限。
- 同一风险 action 自动处理次数上限。

预算耗尽时：

- 停止自动处理。
- 生成人工可读阻塞原因。
- 在 Web/TUI 中提示需要人工介入。

## Trace / Event / Memory

### Auto Governance Trace

新增一等过程证据模型，建议字段：

- `ID`
- `WorkspaceID`
- `RunID`
- `ItemType`: `ticket` / `approval`
- `ItemID`
- `Action`: `answer` / `approve` / `reject` / `escalate`
- `RiskLevel`
- `Confidence`
- `PolicyMatched`
- `PolicyBlocked`
- `BudgetBefore`
- `BudgetAfter`
- `Reason`
- `EvidenceRefs`
- `LLMProviderID`
- `LLMModel`
- `LLMDecisionPreview`
- `Status`
- `OccurredAt`

### EventHub / EventStream

新增事件建议：

- `/governance/auto/evaluated`
- `/governance/auto/resolved`
- `/governance/auto/escalated`
- `/governance/auto/blocked`

事件 payload 至少包含：

- trace id
- workspace id
- run id
- item type
- item id
- action
- risk level
- status
- reason

### Memory 写入

不应把每一次自动处理都写入 Memory。

可写入 Memory：

- 用户明确确认的长期偏好。
- 多次重复出现并被人工确认的默认决策。
- 代码任务中的项目约定、默认测试命令、默认部署限制。

不可写入 Memory：

- 单次审批文本。
- 原始敏感上下文。
- 失败的 LLM 推理全文。

## Web/TUI 展示

### Web

Web 需要展示：

- Ticket / Approval 是否由 Auto Governance 处理。
- 自动处理动作。
- 风险等级。
- policy 匹配或拦截原因。
- 置信度。
- evidence refs。
- 是否消耗预算。

展示必须来自 read model，不解析日志。

### TUI

TUI 默认保持收敛展示：

- 进度栏提示 `Auto-resolved ticket` 或 `Auto-approved read-only action`。
- Result 区最终展示自动治理摘要。
- `/governance` 或 `/status` 可查看 compact trace。
- 不展示详细 LLM 推理全文。

TUI 本地模式继续通过 EventHub 获取刷新信号，不能使用 HTTP client 或定时轮询。

## E2E 验收项

| ID | 验收项 | 预期 |
| --- | --- | --- |
| AG-E2E-01 | Ticket 可从 Memory 自动回答 | Run 不阻塞人工输入，trace 记录 memory evidence |
| AG-E2E-02 | Ticket 语义不明确时升级人工 | 不自动回答，Web/TUI 显示升级原因 |
| AG-E2E-03 | 只读 Approval 自动批准 | Approval resolved，risk=L1，policy=read_only |
| AG-E2E-04 | workspace 内低风险 patch 自动批准 | 小范围 patch 被批准，diff evidence 可追溯 |
| AG-E2E-05 | 高风险 SSH 写操作必须人工 | 不自动 approve，risk=L3/L4 |
| AG-E2E-06 | 超出 DoD 的请求不自动处理 | 自动治理 blocked，reason 指向 DoD mismatch |
| AG-E2E-07 | 自动化预算耗尽后停止自动处理 | Run blocked 并提示预算耗尽 |
| AG-E2E-08 | Web 展示 Auto Governance Trace | Ticket/Approval 详情可见 action/risk/evidence |
| AG-E2E-09 | TUI 展示 compact 自动治理状态 | 不展示事件流，但可看到自动处理摘要 |
| AG-E2E-10 | EventStream 可查询自动治理事件 | `/governance/auto/*` 事件可追溯 |

## 任务状态规则

状态取值：

- `pending`：已确认需求，尚未开始。
- `in_progress`：正在开发或验证。
- `implemented`：代码已实现，尚未完成完整验收。
- `verified`：代码、文档、e2e 验证均完成。
- `blocked`：存在外部依赖或设计问题需要先确认。
- `deferred`：确认延期。

## 需求项状态

| ID | 需求项 | 类型 | 优先级 | 状态 | 验收标准 |
| --- | --- | --- | --- | --- | --- |
| AG-01 | 固化自动治理设计和验收范围 | 文档 | P0 | verified | 本文档确认目标、非目标、风险分级、policy、trace、e2e |
| AG-02 | 扩展配置模型支持 governance policy | 配置 | P0 | verified | 配置可表达 autoTicket、autoApproval、预算和 requireHumanFor |
| AG-03 | 新增 Auto Governance Agent 事件订阅 | 后端 | P0 | verified | 能基于 EventHub 感知 Ticket/Approval created 和 Run blocked |
| AG-04 | 实现 Ticket 自动回答 | 后端/LLM | P0 | verified | 可从 Goal/DoD/Memory/配置推导答案并通过现有路径提交 |
| AG-05 | 实现 Approval 风险分级和 policy 裁决 | 后端 | P0 | verified | L0-L4 分类可测，L3/L4 默认不能自动批准 |
| AG-06 | 实现只读 Approval 自动批准 | 后端 | P0 | verified | 文件读取、配置查询、Web Search 等只读操作可按 policy 自动 approve |
| AG-07 | 实现 workspace 内低风险写入自动批准 | 后端 | P1 | verified | 小范围 workspace patch/artifact 写入可按 policy 自动 approve |
| AG-08 | 新增 Auto Governance Trace 持久化 | 后端/Repository | P0 | verified | trace 可按 workspace/run/item 查询，具备 file/memory backend |
| AG-09 | 新增 Auto Governance EventStream | 事件 | P0 | verified | evaluated/resolved/escalated/blocked 事件可持久化查询 |
| AG-10 | Web 展示自动治理摘要和详情 | Web | P1 | verified | Ticket/Approval 详情显示 action、risk、policy、evidence |
| AG-11 | TUI 展示自动治理 compact 状态 | TUI | P1 | verified | TUI 进度栏/状态视图可见自动处理摘要，不展示详细事件流 |
| AG-12 | 自动化预算与重复问题保护 | 后端 | P0 | verified | 预算耗尽、连续失败、重复 prompt 会停止自动处理 |
| AG-13 | Memory 写入策略 | Memory | P1 | verified | 运行期与通用 Memory 写入按 source+type 白名单约束；拒绝敏感内容和单次审批原文 |
| AG-14 | e2e 验收脚本 | e2e | P0 | verified | 新增 `scripts/verify-auto-governance-ticket-approval.sh`，覆盖 Ticket、Approval、DoD、预算、Web/TUI/EventStream 契约并通过本地最新服务验证 |

## 实施顺序建议

1. `AG-01`、`AG-02`：先固化文档和配置模型。
2. `AG-03`、`AG-08`、`AG-09`：先打通 agent、trace、event。
3. `AG-04`：优先实现 Ticket 自动回答，收益高且风险低。
4. `AG-05`、`AG-06`：实现风险分级和只读 Approval 自动批准。
5. `AG-07`：再支持 workspace 内低风险写入。
6. `AG-10`、`AG-11`：补齐 Web/TUI 展示。
7. `AG-12`、`AG-13`、`AG-14`：完成预算、Memory 策略和 e2e 收口。

## 已确认问题

| ID | 问题 | 当前结论 |
| --- | --- | --- |
| Q1 | 是否允许 LLM 自动批准所有 Approval？ | 否。Approval 必须由 deterministic policy 最终裁决 |
| Q2 | Ticket 和 Approval 的自动处理策略是否相同？ | 否。Ticket 是 `LLM-first, policy-guarded`，Approval 是 `policy-first, LLM-explained` |
| Q3 | 高风险操作是否可以后续自动批准？ | 第一版不支持。后续只能通过 workspaceTemplate/soul 明确策略逐步放开 |
| Q4 | TUI 是否展示详细事件流？ | 否。TUI 只展示 compact 状态和最终摘要，详细 trace 留给 Web/API |
| Q5 | 自动处理是否必须受 DoD 约束？ | 是。超出 Goal/DoD 的事项不能自动处理 |

## 跟踪记录

| 日期 | 记录 |
| --- | --- |
| 2026-05-06 | 初始创建。基于 ticket 自动补全、approval 策略化自动批准、风险分级、DoD 约束、自动化预算和 traceability 的讨论结论形成独立跟踪文档。 |
| 2026-05-06 | 第一批代码收口：新增 governance 配置模型；新增 `AutoGovernanceTrace` 模型与 `AutoGovernanceTraceRepository`（memory/file）；新增 `/governance/auto/*` 事件 topic 和 payload 编解码；并完成契约测试与相关模块依赖透传。 |
| 2026-05-06 | 第二批代码收口：新增 Auto Governance Agent 事件订阅、Ticket 自动回答与恢复运行链路；governance 逻辑保持能力中立，仅使用结构化上下文和通用文本推断；已完成单测与全量 `go test ./...` 验证。 |
| 2026-05-06 | 第三批代码收口：新增 Approval 自动治理链路（事件订阅、风险分级、policy 裁决、自动批准与恢复运行）；新增自动化预算保护（连续失败与重复 prompt 指纹）；并扩展 AutoGovernance trace/event payload 记录 `promptFingerprint`。当前 Web/TUI 展示与 e2e 收口仍在进行中。 |
| 2026-05-06 | 第四批代码收口：完成 AG-10/AG-11。Decision/Approval API 新增 `autoGovernance` 摘要并在 Web 决策/审批视图展示；TUI 接入 `/governance/auto/*` 事件并输出 compact 状态；补齐 runtime initiator 导出 `AutoGovernanceTraces`，并通过 `go test ./...` 与 console 前端构建验证。 |
| 2026-05-06 | 第五批代码收口：完成 AG-13/AG-14。新增 Memory source+type 写入白名单与敏感文本拦截；运行期 `memoryWrites` 仅保留长期偏好与确认后的项目约定；补齐预算（重复 prompt）单测与 `scripts/verify-auto-governance-ticket-approval.sh` 验收脚本，并已在本地最新 `workorch --server`（127.0.0.1:18080）完成通过验证。 |
| 2026-05-06 | 本轮收口复验：执行 `go test ./internal/pkg/workorchconfig ./internal/modules/kernel/governance/biz ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/service/tui ./internal/pkg/events ./internal/pkg/repository/... -count=1`，并在本地服务（127.0.0.1:18080）运行 `WORKORCH_LLM_VERIFY_BASE_URL=http://127.0.0.1:18080 ./scripts/verify-auto-governance-ticket-approval.sh` 通过；将 AG-02/05/06/07/08/09/10/11/12 状态升级为 `verified`。 |
| 2026-05-09 | 针对 `ws-9da22d2b5f3aec24` ticket 无法自动补全问题完成收口：默认 file/container 配置显式启用 autoTicket/autoApproval；LLM decision contract 要求 `requiredDecision.context` 携带中立结构化候选证据；governance core 新增 `knownValues/missingFields/candidateValues/confidenceByField/requiresHumanFields` 通用消费逻辑，并保留自然语言不猜测的人工升级路径。已通过 `go test ./... -count=1` 与 `scripts/verify-boundary-contract.sh`。 |
