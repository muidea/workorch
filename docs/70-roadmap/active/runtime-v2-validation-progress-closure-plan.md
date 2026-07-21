# Runtime V2 Validation Progress 收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-07-21

## Purpose

本文依据最新部署实例 `ws-fa686f29f54bc493/run-003` 的持久化事实，重新打开
Runtime V2 validation/progress 收敛收口项，并将问题拆分为可逐组件修改、独立回归、
跨状态机验收和 deployed 复核的工作单元。

本方案不修改 Runtime V2 的中立设计目标。权威约束仍是：

1. [runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)；
2. `internal/modules/kernel/orchestrator/readme.md`；
3. `internal/pkg/workorchruntimecontract` 中的 typed DTO 与合同；
4. 本文只记录上述合同在当前实现中的缺口及代码收口顺序。

历史 RVC/RPF/RCA completion record 仍可用于证明其当时覆盖的 typed transaction、
freshness、restart/crash 和 bounded terminal 能力；但不得再用历史 positive acceptance
结论证明当前 validation-only same-focus loop 已正确收敛。`run-003` 是新的反证。

## Fresh Deployed Fact Baseline

### Scope

| 项 | 值 |
| --- | --- |
| Workspace | `ws-fa686f29f54bc493` |
| Run | `run-003` |
| 部署日期 | 2026-07-21 |
| 最终状态 | `failed` |
| Terminal code | `no_progress_exhausted` |
| State revision | `167` |
| Evidence sequence | `25` |
| Completed dispatches | `25` |
| Candidate attempts | `25` |
| Repair budget | `10/10` |
| LLM calls | `25`，全部 completed |
| Input + output tokens | `513061` |

### 已确认正常的链路

| 合同面 | 事实 | 判断 |
| --- | --- | --- |
| Provider target | 系统默认、workspace 默认、Run sealed target 均为 `ai-proxy/DeepSeek-V4-Flash` | model/provider 不是本轮根因 |
| Provider transport | 25 次请求均完成，provider recovery 为 `0` | 无 transport retry loop |
| Cache prefix | `stablePrefixHash` 单一且稳定 | C0-C2 稳定前缀未漂移 |
| Projection | `sourceEventRevision=515`，`projectedSourceRevision=515` | eventstream/projection 已追平 |
| Terminal delivery | Result、Run terminal、Workspace activeRun 释放均完成 | SM-4 terminal delivery 主链正常 |
| Lifecycle | active 最终进入 failed，未误报 completed | SM-1 符合终态合同 |
| Metrics | `workorch_run_outcomes_total{status="failed"}=0`，`workorch_convergence_repair_attempts_total=0` | 与 persisted terminal/repair facts 不一致 |

### 未符合设计预期的收敛事实

Run 仅关闭 `cwi-001` 与 `cwi-026/source_change`，`cwi-002`、`cwi-003`、
`cwi-004` 及后续 focus 均未关闭。当前 focus 形成下列循环：

```text
validation command exits 0, but all relevant tests call t.Skip
  -> Shell marks evidence quality=valid
  -> Orchestrator admits receipt as accepted evidence
  -> current-focus accepted evidence calls NoteProgress
  -> ValidationOnlyRetries / SignatureCounts / ActiveGap are reset
  -> assessment recreates the same gap
  -> candidate emits another validation-only operation
  -> loop repeats until MaxRepairAttemptsPerFocus=10
```

这不是“有界失败即符合预期”。有界终止只证明 guard 最终生效；在 guard 之前，
有效进展判定和 validation-only escalation 已被错误重置，SM-2 没有按设计选择更强的
state change、grounded discovery、governance 或更早的 typed terminal。

## Root-Cause Evidence

| ID | 代码事实 | 缺口 |
| --- | --- | --- |
| RC-1 | `internal/modules/blocks/shell/biz/validation_observation.go:25` 对所有 exit code 0 的 `go test` 标记 `valid` | 未区分 mixed pass/skip 与 all-skipped；owner receipt 失真 |
| RC-2 | `internal/modules/kernel/orchestrator/biz/runtimev2/executor.go:457` 只拒绝 zero-work、inconclusive、invalid、skipped | RC-1 已错误标 valid 的 receipt 会进入 accepted |
| RC-3 | `internal/modules/kernel/orchestrator/biz/runtimev2/dispatch_commit.go:333` 对任意 applies-to-current-focus accepted evidence 调用 `NoteProgress` | accepted receipt 被错误等同于 effective convergence progress |
| RC-4 | `internal/pkg/workorchruntimecontract/runtime_repair_ledger.go:174` 的 `NoteProgress` 重置 validation-only retry、signature、active gap 等账本字段 | helper 本身是纯值转换；错误在 Orchestrator 的调用条件，不应把迁移决策下沉到 pkg |
| RC-5 | `internal/modules/kernel/orchestrator/biz/runtimev2/reducer.go:430` 已支持 validation-only cap 后升级 state change | counter 每轮被 RC-3 重置，既有 reducer 分支无法触发 |
| RC-6 | candidate admission 仅记录 active rejected operation shapes | accepted-but-no-progress validation 可通过轻微参数或理由变化重复 |
| RC-7 | `DiscoveryRequired` / `DiscoverySources` 目前只进入 bootstrap/hash/prompt | 缺少 owner-observed discovery snapshot 与 deterministic admission；LLM 可反复声称本机实际存在的工具缺失 |
| RC-8 | `workorchd` 在 activation 调用返回时执行 `observeRunOutcome` | Runtime V2 异步终态发生后没有 exactly-once 终态指标观察；deployed failed/repair metrics 仍为 0 |

现有 Shell 测试覆盖 zero-work 与 mixed pass/skip，但缺少 all-skipped；现有 reducer
测试已经覆盖 validation-only cap escalation，说明本轮应修复 evidence/progress 链路，
而不是重写 reducer 规则。

## Reopen Decision

本项以 `RUNTIME-VPC` 重新进入 active roadmap，关闭条件不是“Run 最终有界终止”，
而是同时满足：

1. owner receipt 如实表达执行质量；
2. accepted evidence 与 effective progress 被明确区分；
3. validation-only 计数不会被等价 evidence 清零；
4. 相同 gap 下的 accepted-but-no-progress operation family 不能无限变形重放；
5. `DiscoveryRequired` 具有确定性 owner observation；
6. async terminal 与 repair metrics 与 persisted terminal facts 一致；
7. fresh deployed workspace 在每个 bounded window 内产生实际 delta，或进入正确的治理/终态分支。

## Implementation Closure Record — 2026-07-21

VPC-1 至 VPC-5 的代码收口已完成，当前文档仍保持 `active`，原因是 VPC-6 的 fresh
deployed gate 尚待用新二进制和清洁数据执行；不得以本地测试替代 live 事实。

| Package | Landed closure | State-machine impact | Independent regression |
| --- | --- | --- | --- |
| VPC-1 | Shell 将“已执行但全部 skipped”的 observation 归为 `skipped`，保留 mixed pass/skip 和 zero-work 的既有语义。 | SM-3 input | `go test ./internal/modules/blocks/shell/biz -run 'Test.*Validation\|Test.*GoTest.*Observation' -count=1` |
| VPC-2 | Orchestrator 仅在当前 focus 的 durable `Changed=true` state change 时调用 `NoteProgress`；accepted validation 只使 assessment 失效，不清 repair window。 | SM-2, SM-3 | `go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -run 'TestAccepted.*ProgressWindow\|TestAcceptedValidation.*RepairLedger' -count=1` |
| VPC-3 | persisted repair ledger 保存 canonical accepted-no-progress operation shapes；同 focus 的重放在 candidate admission 前被拒绝，新的 durable progress 才清空。 | SM-2, SM-3 | `go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -run 'TestEvaluateCandidateAdmission' -count=1` |
| VPC-4 | validation policy 的 source union 为纯 contract helper；capability owner 的 typed inventory 附带 catalog observation；Orchestrator 以 manifest、workspace-tree、admissibility 和 capability observations 作 deterministic gate，缺失时不请求 provider。已有 cap reducer 分支继续负责 retry=2 后 state change/governance/terminal。 | SM-2, SM-3, SM-4 freshness reuse | `go test ./internal/modules/kernel/orchestrator/biz -run TestRuntimeV2ValidationDiscovery -count=1` |
| VPC-5 | Run state notification 携带单调 repair snapshot；workorchd 通过 EventStream notification 按 `workspace/run/state/runtimeRevision` 去重终态、按 run 最大 repair snapshot 累积 delta。activation 返回不再计 terminal outcome。 | SM-4 only | `go test ./internal/modules/application/workorchd/service/server -run TestServerMetricsObserveRunStateIsExactlyOnceAndMonotonic -count=1` |

Capability discovery 的“candidate claim”只接受 typed operation/feasibility 的可执行断言：候选
没有可被文本伪造的 `tool missing` authority 字段。operation 必须先匹配 fresh owner
admissibility inventory 并经 owner feasibility probe；与 ready inventory 冲突的 operation
在 seal 前被拒绝。自由文本不会成为环境事实或驱动 action。

本次本地验证已通过：

```text
go test -race ./internal/modules/kernel/orchestrator/... -count=1
make verify-boundary-contract
make verify-runtime-v2-positive-convergence-file
```

文件 verifier 已纳入 VPC Shell、effective progress、discovery 与 async metrics focused
tests，并使用 `/tmp/workorch-e2e`，不写入用户配置或 repository runtime data。

## Scope and Non-Goals

### In scope

- `internal/modules/blocks/shell/biz`：validation observation 与 receipt quality；
- `internal/modules/kernel/orchestrator/biz/runtimev2`：evidence admission、effective progress、
  same-focus repair ledger 调用、candidate no-progress admission、discovery prerequisite；
- `internal/pkg/workorchruntimecontract`：仅在需要时新增/补齐 DTO、枚举、normalize、
  clone、stable hash 和无状态比较 helper；
- `internal/modules/application/workorchd`：typed terminal/progress notification 的指标观察；
- 相应 owner `pkg/events`：具体 typed command/data/result；
- targeted、restart/idempotency、跨组件与 deployed verifier。

### Explicit non-goals

- 不修改 SM-1 的 Run lifecycle 语义；
- 不修改 workspace/provider/model 选择优先级；
- 不引入 V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch 或 auto replan；
- 不在 Orchestrator core 硬编码 Go、LSP、具体语言、测试框架或工具名称；
- 不让 LLM block、Shell block、Result、Run 或 workorchd 选择 next action；
- 不在 `pkg` 中调用 `NoteProgress`、选择 state change/validation、重置 repair ledger，
  或处理任何 aggregate 状态迁移；
- 当前阶段不增加兼容分支，不迁移历史 aggregate，不读取历史 V1 数据；
- 不通过增加 repair budget、放宽 evidence gate 或把 failed 显示成 active 掩盖循环。

## Four-State-Machine Impact Matrix

| 状态机 | 当前判断 | 本轮允许修改 | 禁止修改 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | 符合 | 仅补 terminal notification/metrics consumer 测试 | 不改变 active/blocked/terminal 迁移，不让 application 恢复 terminal Run |
| SM-2 authority/focus convergence | **不符合，主收口面** | effective progress 判定、repair ledger 调用条件、validation-only escalation 输入、grounded discovery | 不执行 capability operation，不把 action 选择下沉 owner/pkg |
| SM-3 candidate/sealed-dispatch | **部分符合** | weak receipt admission、accepted-no-progress context/admission、stable structural signatures | 不在 executor 选择 next action，不破坏 sealed transaction/fence/exactly-once |
| SM-4 drive/recovery/projection | 核心符合，observability 不完整 | restart/idempotency 复核、async terminal/progress metrics exactly-once | 不从 projection 重建 authority，不由 metrics observer 触发 Resume |

任何代码提交必须标明影响的 SM ID。VPC-2/VPC-3 同时修改 SM-2 与 SM-3 时，
必须在同一变更中更新 transition、duplicate/stale 和 restart tests，不能只修 happy path。

## Component Responsibilities and Interaction Contract

### Canonical flow

```text
Orchestrator sealed operation
  -> Shell owner executes bounded command
  -> Shell returns typed persisted receipt
       outcome + quality + zeroWork + validation observation + resource changes
  -> Orchestrator performs evidence admission (SM-3)
  -> Orchestrator evaluates effective progress against persisted facts (SM-2)
  -> Orchestrator updates repair ledger and reduces next action (SM-2)
  -> Run owner CAS-persists aggregate
  -> terminal delivery writes Result and projects Run/Workspace (SM-4)
  -> typed Run/progress notification updates workorchd metrics only
```

### Responsibility cards

| 组件 | 唯一职责 | 输出合同 | 明确禁止 |
| --- | --- | --- | --- |
| `blocks/shell/biz` | 执行 sealed shell/managed validation operation，形成真实 owner observation | exit/outcome、quality、zero-work、executed/passed/failed/skipped checks、bounded preview、receipt | accepted/rejected 裁决、coverage 关闭、repair counter、next action |
| `kernel/orchestrator/biz/runtimev2` | 唯一 evidence admission、effective progress、repair transition、candidate admission 与 reducer authority | persisted aggregate transition、sealed dispatch、accepted/rejected evidence、typed terminal | provider 特化、测试框架特化、跨 owner repository、application continuation |
| `workorchruntimecontract` | DTO、合同、normalize/clone/hash/纯比较 helper | 值对象和确定性无状态结果 | 读取 repository/EventHub、修改 aggregate、调用迁移、选择 action |
| `blocks/llm` | 依据 sealed request 产生 candidate | typed candidate/provider failure/usage | 裁决 evidence、声明未经 owner observation 支持的 authority fact、推进 Run |
| `kernel/run` | aggregate CAS 与 Run lifecycle persistence | typed Run state/progress notification、recovery inventory | reducer、operation execution、progress 判定 |
| `application/workorchd` | activation facade、read model、SSE/HTTP、指标 observer | typed bootstrap/Start/Resume；projection/metrics | direct Run/orchestrator repository、next action、terminal 后自动 Resume |
| `blocks/result` / `workspace` / `eventstream` | 保持已裁决结果、activeRun 投影和 audit | typed result/projection/audit | 重新裁决 progress 或 terminal |

### `pkg` boundary rule

允许留在 `internal/pkg/workorchruntimecontract`：

- `RuntimeOperationReceipt`、`RuntimeOperationEvidence`、`RuntimeValidationObservation`；
- repair ledger、operation shape、discovery snapshot 的 DTO；
- `Normalized`、`Clone`、`Validate`、stable hash、集合比较等纯函数；
- 返回新值、不访问外部状态的账本 helper。

必须留在 Orchestrator Biz：

- 当前 evidence 是否 admitted；
- admitted evidence 是否构成当前 focus 的 effective progress；
- 何时调用 `NoteProgress`、`NoteNoProgress`、`MarkValidationOnlyRetry`；
- 是否 invalidate assessment、清除 active gap、升级 state change；
- 是否把 operation shape 加入当前 cycle 的 disallowed/no-progress set；
- discovery observation 是否满足当前 authority transaction；
- terminal/governance/action 选择。

`NoteProgress` 可以继续作为 pkg 的纯值 helper；本轮不能把“何时 NoteProgress”写成
pkg 方法内部隐式规则，也不能让 receipt 自行修改 ledger。

## Closure Work Packages

### VPC-1 — Shell all-skipped evidence truthfulness

**Affected machines:** SM-3 input contract.

**Owner:** `blocks/shell`。

#### Required behavior

1. exit code 0 只表示进程成功，不自动等于 validation evidence `valid`；
2. observation 明确显示存在 executed checks、passed=0、failed=0，且所有 executed checks
   均 skipped 时，quality 必须为 `skipped` 或合同定义的等价 weak quality，并给出稳定、
   非业务化的 reason/tag；
3. mixed pass+skip 保持 `valid`，但 observation 必须保留 skipped 集合；
4. zero-work 继续由 evidence policy 标记 `inconclusive`，不能被 exit code 0 覆盖；
5. non-verbose 或无法形成 check observation 时，不得虚构 executed/passed；是否可视为 valid
   必须服从已有 evidence policy/marker，而不是依赖具体测试名；
6. parser 可以识别 Go test 输出，但判定逻辑必须基于通用 observation counts，不能写入
   LSP、workspace goal 或具体 `Test...` 名称。

#### Expected files and regression

- `internal/modules/blocks/shell/biz/validation_observation.go`；
- `internal/modules/blocks/shell/biz/validation_observation_test.go`；
- 如需通用质量归一，仅补充 Shell owner-private helper 或 contract enum/DTO tests。

```text
go test ./internal/modules/blocks/shell/biz -run 'TestRuntimeAdapter.*Validation|Test.*GoTest.*Observation' -count=1
go test ./internal/modules/blocks/shell/... -count=1
```

必须包含 all-skipped、mixed pass+skip、all-pass、zero-work、non-zero exit、non-verbose、
subtest-only output 和 policy 已显式标 inconclusive 不被覆盖。Shell receipt 不得出现 Run、
coverage、repair 或 action 字段。

### VPC-2 — Orchestrator effective progress transition

**Affected machines:** SM-2 + SM-3.

**Owner:** `kernel/orchestrator/biz/runtimev2`。

#### Required behavior

将三个概念分离：

```text
owner receipt accepted
  != evidence admitted into aggregate
  != effective convergence progress
```

最小规则：

1. weak validation 进入 rejected evidence，不能关闭 coverage、清 assessment 或调用
   `NoteProgress`；
2. valid validation 可以触发 assessment，但在 require-assessment focus 上，单纯增加一条
   同质量 receipt 不足以重置 repair ledger；
3. effective progress 必须对应 persisted delta：coverage 进入 satisfied、focus 原子推进、
   当前 focus durable state change 的 `Changed=true`，或 assessment gap 实质改善；
4. validation-only evidence 后 assessment 仍返回同 gap，应增加 validation-only/same-gap
   cycle，而不是清零；
5. assessment invalidation 与 `ActiveGap=nil` 必须由有效状态变化或明确的新 assessment
   transaction 驱动，不能由任意 accepted fact 无条件驱动；
6. stale、duplicate、idempotent replay 不得重复增加 evidence、repair、retry 或 progress；
7. `RepairAttempts` bounded guard 保持独立，不因同 focus 的微小 accepted evidence 重置。

#### Placement and regression

- 主要文件：`dispatch_commit.go`、`assessment_evidence_gate.go` 及 focused tests；
- `reducer.go` 仅在输入/分支需对齐时修改；
- pkg 只允许纯 delta helper，禁止在 ledger helper 内读取 facts 后选择迁移；
- transition 必须随 CAS 原子持久化，不能依赖进程内临时标记。

测试必须覆盖：weak evidence 不 NoteProgress、valid-but-same-gap 不清 counter、durable changed
state-change progress、no-op 非 progress、coverage delta、duplicate/stale、receipt/evidence/progress
commit 前后 crash/restart。

```text
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test -race ./internal/modules/kernel/orchestrator/... -count=1
```

### VPC-3 — Accepted-but-no-progress operation admission

**Affected machines:** SM-2 + SM-3.

**Owner:** Orchestrator；capability owner只提供 canonical receipt facts。

#### Required behavior

1. 保留现有 active rejected operation shapes gate；
2. 为当前 focus/repair cycle 增加 accepted-but-no-progress 历史，至少绑定
   `focus + coverage + kind/variant + normalized targets + effect scope + resulting gap + quality`；
3. cosmetic reason、boundedReason 或说明文本不能制造新 shape；
4. exact operation replay 直接拒绝；同 gap 下即使参数轻微变化，validation-only 次数仍
   共用同一个 bounded budget；
5. 真正的新 state-change、coverage target、discovery revision 或改善后的 gap 可以开启
   新 cycle，但必须由 persisted delta 证明；
6. disallowed shape/context 来自 aggregate，不得只存在 prompt、进程内 map 或 projection；
7. candidate attempt 0/1 与 repair/validation-only budget 保持独立。

pkg 如需新增内容，只保存 canonical no-progress record DTO、normalize/hash/clone/bounded list
和 candidate context typed facts；记录何时创建/失效、是否拒绝 candidate仍在 Orchestrator Biz。

回归覆盖 exact replay、cosmetic 变化、rejected 与 no-progress shapes 并存、state-change 后
失效边界、focus advance reset、restart 后仍拒绝、bounded context 不丢 active set，以及
volatile shape 只进入 C3-C5、`stablePrefixHash` 不漂移。

### VPC-4 — Validation-only escalation and grounded discovery

**Affected machines:** SM-2；新增 prerequisite transaction 时同时覆盖 SM-3/SM-4 recovery.

#### Validation-only escalation

封存策略 `MaxValidationOnlyRetriesPerFocus=2`、
`ValidationFailureImpliesStateChange=true`、
`PreferMinimalPatchBeforeRepeatedValidation=true` 必须成为可观察行为：

1. 同 focus validation-only retry 达到 2 后，下一 action 只能是 admissible state change、
   typed governance 或确定不可行的 typed terminal；
2. 不允许 counter 被同 gap validation evidence 清零后再次 validation；
3. 优先复用 reducer 现有 cap 分支，不通过把 cap 调大绕过问题。

#### Grounded discovery

`DiscoveryRequired` 不能只是 prompt 提示。目标合同：

```text
validation policy references neutral discovery source IDs
  -> Orchestrator requests typed prerequisite/discovery observation
  -> owning capability block observes current environment
  -> returns snapshot ID + revision + bounded neutral facts
  -> Orchestrator persists and binds snapshot to authority/candidate
  -> candidate claim conflicting with fresh owner observation is inadmissible
```

约束：

- source 定义属于 profile/capability contract，不属于 core business hard-code；
- 环境能力由对应 owner观察，Orchestrator只判断 freshness、binding 与 claim 一致性；
- pkg 可定义 observation DTO，不执行 probe；LLM只渲染 persisted observation；
- observation 包含 owner、subject、revision/hash、observedAt 和失效依赖；
- restart 后复用 committed fresh snapshot，依赖变化后 reacquire；
- 不硬编码 `gopls`、`rust-analyzer`、`pyright` 或其他具体语言工具。

回归覆盖 snapshot 缺失 acquisition、ready/absent observation、conflicting candidate claim、
commit 前后 crash、dependency stale/reacquire、duplicate observation，以及 cap=2 的完整
state-sequence matrix。

### VPC-5 — Async terminal and repair metrics exactly-once

**Affected machines:** SM-4 observability only.

**Owner:** `application/workorchd` metrics observer；事实源为 owner typed notification/read model。

#### Required behavior

1. activation 返回 active Run 时不计 terminal outcome；
2. Run 异步进入 completed/failed/canceled/superseded 时，按 typed terminal notification
   exactly once 计数；
3. duplicate event、projection rebuild、SSE reconnect 和同 revision replay 不重复计数；
4. repair attempts 从单调 persisted progress/terminal snapshot 观察 delta，不能在 activeRun
   释放后因当前视图为空而回到 0；
5. observer只更新指标，不触发 Resume、不写 owner、不读取 Orchestrator repository；
6. notification 包含 workspace/run/state revision/outcome 等 dedupe identity；
7. process restart 后按 Prometheus process-local counter 语义重新开始，不扫描 authority 历史。

主要修改 `workorchd/service/server` metrics observer/tests；必要时补 `kernel/run/pkg/events`
typed DTO。测试覆盖 active response=0、async failed=1、duplicate仍为1、四类 terminal 映射、
repair 0→1→3 delta、terminal 后不回退，以及 lifecycle/teardown 不泄漏订阅。

### VPC-6 — Cross-component and deployed acceptance

**Affected machines:** SM-2 + SM-3 + SM-4；SM-1 只做非回归。

#### Local integration gates

```text
go test ./internal/modules/blocks/shell/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test -race ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
make verify-boundary-contract
go test -p 1 ./... -count=1
go vet ./...
make verify-runtime-v2-positive-convergence-file
make release-check
```

若 full suite 触发真实 provider 或污染运行目录，必须修 fixture/config 隔离；禁止修改用户
`config/`、默认模型或 `.workorch-data` 让测试通过。file-backed fixture 使用 `t.TempDir()`
或 `/tmp/workorch-e2e`。

#### Required state-sequence tests

| Matrix | Sequence | Required assertion |
| --- | --- | --- |
| weak validation | sealed → process exit 0 → skipped quality → evidence rejected | 不 NoteProgress、不关 coverage、形成 typed gap |
| same-gap validation | valid validation → assessment same gap → retry 1/2 | counter 单调，progress revision 不变 |
| escalation | retry 2/2 → reduce | state change / governance / typed terminal 三选一，不再 validation |
| actual progress | durable changed receipt → assessment improves | NoteProgress 一次，旧 gap/shape 按合同失效 |
| duplicate | same receipt/event/revision replay | evidence、counter、progress、metrics exactly once |
| stale | dependency/authority revision changes | old candidate/receipt rejected或reacquired，不算 progress |
| restart | crash at receipt/evidence/progress/discovery/terminal boundaries | reload 等价，不重复 operation/evidence/metrics |
| terminal | no feasible path | typed terminal + Result/Run/Workspace 投影，不误报 completed |

#### Fresh deployed gate

使用最新二进制、清洁 runtime data 和显式 workspace 创建流程，不复用 `run-003` aggregate。
必须同时检查 API、persisted facts、EventStream、metrics 和工作区文件状态。

1. system default = workspace default = sealed Run target；
2. 同 Run `stablePrefixHash` 稳定，volatile repair/shape/discovery 位于 C3-C5；
3. all-skipped validation 不得成为 valid accepted progress；
4. 同 focus 同 gap validation-only 最多 2 次，之后必须升级；
5. 不允许 10 次同类 validation 直到总 repair budget 耗尽；
6. 只有 coverage、durable state、assessment gap 或 focus 实际 delta 才更新 progress；
7. owner discovery 与 candidate claim 一致；ready capability 不得被反复声称缺失；
8. 每个 bounded window 必须产生 delta、governance wait 或 typed terminal；
9. `sourceEventRevision == projectedSourceRevision`，health/ready 正常；
10. terminal outcome metrics 对该 Run 增加 1，repair metrics 与 persisted ledger 一致；
11. terminal delivery exactly once，Workspace activeRun 正确释放；
12. prerequisite 齐备时 canonical workspace 持续推进到真实 completion；确有外部缺失时，
    在 typed observation 支持下进入 governance/明确 terminal，不得伪进展。

## File-Level Closure Matrix

| Work package | Primary files | Allowed supporting files | Forbidden landing point |
| --- | --- | --- | --- |
| VPC-1 | `blocks/shell/biz/validation_observation*` | Shell evidence policy/tests；quality enum | Orchestrator reducer、application projection |
| VPC-2 | `orchestrator/biz/runtimev2/dispatch_commit*`、assessment gate | reducer 输入对齐；pkg 纯 delta helper | `pkg` 状态迁移、Shell/LLM action 选择 |
| VPC-3 | `orchestrator/biz/runtimev2/candidate_admission*` | contract shape DTO/hash/context | provider adapter 内保存 no-progress state |
| VPC-4 | Orchestrator prerequisite/reducer；capability observer | policy/discovery DTO、profile config | core 的 LSP/Go/tool-name hard-code |
| VPC-5 | `workorchd/service/server` metrics observer/tests | run typed notification/read model | direct Orchestrator repository、metrics Resume |
| VPC-6 | verifier/tests/docs | `/tmp/workorch-e2e` artifacts | 仓库 runtime data、用户 config 污染 |

## Commit and Review Slices

建议逐一提交：

1. `fix shell all-skipped validation evidence quality`（VPC-1）；
2. `fix runtime effective progress accounting`（VPC-2）；
3. `reject accepted no-progress operation replay`（VPC-3）；
4. `ground runtime validation discovery and escalation`（VPC-4）；
5. `observe asynchronous runtime terminal metrics`（VPC-5）；
6. `verify runtime validation progress convergence`（VPC-6 tests/verifier/docs）。

每个 review 必须回答：影响哪个 SM、哪个 owner写状态、typed I/O 是什么、哪个 evidence
被 admit、哪个 persisted delta 才算 progress、duplicate/stale/restart 怎么处理、组件如何独立
回归、是否保持 C0-C2 stable prefix、是否新增业务特化或跨 owner 直接依赖。

## Definition of Done

只有以下条件全部满足时，`RUNTIME-VPC` 才能关闭：

- VPC-1～VPC-5 代码与 focused tests 全部 landed；
- Shell、Orchestrator、workorchd 均可独立回归；
- SM-2 transition、SM-3 transaction、SM-4 restart/metrics matrix 全部通过；
- boundary contract、full test/vet、positive file verifier、release check 通过；
- fresh deployed gate 12 项全部有可复核事实；
- canonical workspace 不再复现 all-skipped → valid → NoteProgress → same gap 循环；
- active overview、current-state、unclosed matrix、todo index 与验证记录同步；
- completion record 迁入 `90-archive/completed/`，active 不保留假完成状态。

若只实现弱 evidence 拒绝，但没有 effective progress、validation cap、restart 和 deployed
验证，不得关闭；若只让 Run 更早 failed，也不得称为正向收敛完成。
