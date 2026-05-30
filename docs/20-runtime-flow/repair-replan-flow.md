# Repair And Replan Flow

Status: active

Type: runtime-flow

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 在失败、contract violation、provider retry、repair exhausted、
decision resolution、replan successor 等情况下的恢复链路。

## Flow

```text
failure detected
-> classify failure
-> convergence contract / recovery decision
-> retry / repair / governance / replan
-> successor run or resumed run
-> continue with inherited contract and evidence
```

## Main Stages

### 1. Failure Classification

失败首先需要被结构化分类，而不是只保留自然语言错误文本。

恢复链依赖：

- failure class
- failure signature
- failure target scope
- current stage
- current required output

### 2. Repair

repair 的职责是针对当前明确失败证据做定向修复。

repair 不应退化为：

- 广泛重新规划
- 无界继续探索
- 重复执行同一无效图

### 3. Replan

replan 的职责是：

- supersede 当前失效路径
- 基于当前 Goal / DoD 和 predecessor evidence 创建 successor scope
- 保留 lineage，但不让旧 continuation 直接主导新 run

### 4. Governance-Mediated Recovery

当 repair exhausted、duplicate failure、manual gate、provider/config blocker
出现时，恢复可能经过：

- Decision
- Approval
- auto-governance

但治理对象的作用是选择恢复路径，不是替代恢复路径本身。

恢复治理补充要求：

- runtime recovery 决策必须优先基于结构化 ticket context，而不是依赖自由文本问题描述
- `allowedRecoveryActions`、`recoveryPolicy`、`failureClass`、`failureSignature`、`scope`、`dependency` 等字段应作为自动恢复的主要输入
- 对 `runtime_repair_exhausted_requires_strategy` 这类结构化恢复 ticket，若 policy 允许且不存在 manual/provider/config gate，可由 auto-governance 自动选择 `replan`
- `retry_same_step`、`replan`、`provide_information`、`terminate` 应保持为中立恢复动作，而不是某个 profile 的私有规则

#### Recovery Action Matrix

当前治理恢复动作至少应按以下矩阵稳定分流：

| Recovery Action | 典型触发面 | 恢复结果 |
| --- | --- | --- |
| `retry_same_step` | provider transient、schema wrapper、可重试同一失败面 | 继续既有 run，保留原 failure surface |
| `provide_information` | runtime 缺少无法自动取得的信息 | 继续既有 run，并把结构化补充信息回灌到 context / repair package |
| `approve` / `reject` | 高风险 action 需要人工批准或拒绝 | 继续既有 run 或进入明确终态失败 |
| `replan` | repair exhausted、duplicate failure、当前路径已不可继续 | 创建 successor run，旧路径进入 superseded / historical continuation |
| `terminate` | 已确认不能继续、或策略要求显式终止 | 显式终态失败或治理终止 |

约束：

- 动作只选择恢复路径，不替代 Run / Result / Decision / Approval 本体
- `replan` 与 `retry_same_step` 互斥：前者创建 successor，后者继续原 run
- `provide_information` 只补上下文，不直接表达 accepted closeout

#### Decision / Approval / Auto-Governance Routing

| 治理对象 | 当前主要作用 | 典型输出 |
| --- | --- | --- |
| `Decision` | 选择恢复路径或补充缺失信息 | `provide_information` / `replan` / `terminate` |
| `Approval` | 对高风险 action 做显式人工确认 | `approve` / `reject` |
| `auto-governance` | 在 policy 允许时自动选择建议恢复动作 | 多为 `replan` / `retry_same_step` / 等价中立动作 |

稳定规则：

- `Decision` 优先解决“信息缺口 / 策略选择”
- `Approval` 优先解决“高风险动作允许性”
- `auto-governance` 只在结构化 policy 足够明确时替代人工选择
- auto-governance 产出的建议动作仍应在读模型中可追踪，而不是静默发生

## Concrete Failure Evidence Acquisition

当 repair / replan continuation 缺少 concrete repair target evidence 时，不得直接维持
`patch/apply` 或其他 state-changing contract，而应先进入中立取证阶段。

稳定 flow：

```text
repair target evidence missing
-> acquire_concrete_failure_evidence
-> bounded read / list / command / resource fetch
-> rebuild failure target scope and snapshots
-> resume state-changing repair or verification evidence repair
```

该阶段允许的能力只应包括：

- `file/list`
- `file/read`
- `shell/command`
- `mcp/tool_call`
- profile 明确允许的等价只读/取证能力

该阶段不允许：

- 直接要求 `patch/apply`
- 无界 exploration
- 用 run snapshot / raw provider dump 代替 target evidence
- 因缺少 target evidence 而伪造 state-changing contract

## Wrapper Recovery Inheritance

provider retry、schema invalid、runtime contract correction、repair-of-repair、
assessment-only correction 等 wrapper recovery 不得清空原始失败面。

稳定继承要求：

- 保留原 `failureClass` / `failureSignature` / `failureTargetScope`
- 保留 `repairActionPackage`
- 保留 target snapshots / manifest / coverage target context
- 保留 recent rejected attempts / latest actionable diagnostics
- wrapper failure 只作为最新 rejected output 或 contract diagnostics，不得覆盖原 failure surface
- `currentFailure`、`structuredFailureSignals` 和 `primaryDiagnosticRepair` 必须继续来自 primary actionable failure；schema/mode/contract wrapper 的 `error`、`reason`、字段名和 target-like token 只能作为 rejected attempt 摘要
- 若上一轮 output mode 是 `incrementalArtifact`，且 wrapper failure 只表示 artifact 输出合同不合法，下一轮继续 artifact contract recovery，不得退回单次 `executionGraph`

若 wrapper failure 后仍没有 concrete target evidence：

- 进入 `acquire_concrete_failure_evidence`

若 wrapper failure 后已有 concrete target evidence：

- 恢复到原 repair lineage
- 继续执行 state-changing repair 或 concrete validation evidence repair

## Recovery Path Selection Rules

当前恢复链至少应满足以下中立分流：

| 当前状态 | 默认恢复方向 |
| --- | --- |
| provider transient / stream incomplete / schema-invalid wrapper | provider retry 或 schema recovery；不得覆盖原 failure surface |
| state-changing repair 缺 target snapshot | `read_required` / `acquire_concrete_failure_evidence` |
| assessment / claim-evidence 缺口但已有候选 evidence | assessment-only correction 或 concrete validation evidence |
| repair exhausted / duplicate failure / policy gate | decision / approval / auto-governance |
| predecessor path 已不可继续 | `replan` successor |

该表只表达恢复链分流，不表达任何领域特化修复策略。

当 `replan` successor 同时继承 rejected result continuation 时，分流顺序必须保持：

1. 如果 predecessor progress digest 或 process evidence 仍有未关闭 concrete actionable failure，优先恢复该 failure。
2. 如果该 failure 只有 target refs / diagnostics、没有当前 snapshot，进入 `acquire_concrete_failure_evidence`。
3. 只有 concrete failure 已关闭或不存在时，才让 rejected result 的 coverage / deliverable gap 驱动后续修复。

因此 rejected result continuation 是验收收口背景，不是自动覆盖 replan primary failure 的更高优先级来源。

## Governance Queue And Resume Semantics

治理队列与 resume 恢复链在 flow 上至少应保持以下稳定语义：

- Ticket / Approval 必须保留 `createdAt` / `updatedAt`
- 默认列表按 `createdAt` 倒序分页
- `cursorAfterId` 必须基于排序后的队列继续翻页
- file backend 对历史无时间字段对象，可用 mtime 作为排序兜底，但只表达生成顺序

resume 相关补充约束：

- blocked run 在 ticket/approval 提交后若短暂恢复又再次阻塞，必须通过 checkpoint /
  latest ticket / latest failure context 定位新的失败面
- 不得仅凭“已提交/已处理”推断 run 已经真正恢复成功
- replan continuation 若缺少显式 `processEvidenceTimeline/currentProgressDigest`，
  必须先合成最小过程证据，再继续 strict convergence / repair
- continuation 继承的 step output 只能保留诊断摘要、stage/failure/status/preview，
  不得把完整 `executionGraph`、`llmDecision`、`patches[].content` 继续嵌入下一轮 prompt

## Context-Gate Recovery Routing

当失败发生在 provider 提交之前，恢复链必须先按 context gate 语义分流，
不得把它伪装成普通业务 repair。

### `llm_context_integrity_failed` Boundary

- `llm_context_integrity_failed` 属于 runtime 在 provider 前的准入失败
- 它不能进入 provider retry
- 它不能进入普通 runtime repair 或 state-changing repair
- `stateChangingOutputRequired=true` 不得覆盖该边界

### Routing Matrix

| Gate Action | 恢复路径 | 说明 |
| --- | --- | --- |
| `pass` | 提交 provider | 进入正常 LLM 调用链 |
| `read_required` | 派发 `read/resource fetch`；若无法解析资源则创建 `runtime_context_required` | 只补上下文，不做 patch/apply |
| `fail_fast` | 终止本轮 provider 提交，并记录结构化原因 | 必须保留 `contextGateAction`、`contextGateReason`、受影响 protected field |
| `blocker_required` | 创建 `blocked_on_decision` | 说明缺什么、为何 runtime 无法自动获取 |
| `replan_required` | 创建 successor lineage | 继承 predecessor failure context 与仍有效 Goal/DoD |

### Decision Resolution Resume Rule

`runtime_context_required` 决策被 resolved 后，恢复动作只表示 runtime 获得继续构造上下文的
授权或补充信息，不表示可以直接重放已失败 step。

恢复时必须：

- 创建新的 context retry step
- 从 ticket context、failed step input、replan continuation、`repairActionPackage`、
  current snapshots、process evidence 中重新 hydrate `currentFailure`
- 重建 `currentExecutionProgress`、`requiredOutput`、target evidence

若这些 P0/P1 字段仍无法恢复：

- 进入明确的 `replan` / terminal / blocker
- 不得生成相同原因的重复 context ticket

### Cross-Module Recovery Invariants

- budget / request assembly 模块只负责计算 `contextGateAction/contextGateReason`
- orchestrator 必须按 context gate 语义执行恢复，不能假设后续 patch 会补好上下文
- context gate failure 的恢复链必须留下新的 step / ticket / lineage 证据，不能静默覆盖原失败

## Key Flow Guarantees

- repair 必须继承来源 stage contract
- failure surface 不能被 wrapper/provider 外壳错误覆盖
- replan successor 必须能看到 predecessor failure context
- auto-governance 只能基于结构化恢复动作，而不是自由文本语义
- no-actionable continuation 只能有限次 follow-up，不能形成无界 continuation 链
- provider/config/human-required recovery 必须明确升级为人工治理，而不是被自动 replan 覆盖
- decision / approval / auto-governance 必须在恢复动作上分工明确，不能互相混写为同一类 blocker

## Output

repair / replan 链的正式输出可以是：

- resumed run
- successor run
- blocked governance state
- explicit terminal failure

其中：

- `resumed run` 表示在既有 run lineage 上继续推进
- `successor run` 表示由治理结论创建的新 run，对旧路径形成 supersede
- governance output 只能决定流向，不能替代 Run / Result 本体

## Source Documents

- `../90-archive/completed/runtime-recovery-governance-closure-design.md`
- `../60-verification/audits/full-chain-assessment-closure-plan.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
- `../00-core/runtime-stage-contract.md`
