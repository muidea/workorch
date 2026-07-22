# Runtime V2 Validation Progress 收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-07-22

## Purpose

本文最初依据部署实例 `ws-fa686f29f54bc493/run-003` 的持久化事实重新打开
Runtime V2 validation/progress 收敛收口项；2026-07-21 最新 `run-006` 又证明旧的
validation-only 循环虽已被有界阻断，但 failure/repair target grounding 仍未闭环。因此本文将
剩余问题继续拆分为可逐组件修改、独立回归、跨状态机验收和 deployed 复核的工作单元。

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

## Post-deployment verification addendum — 2026-07-21

最新二进制在 `ws-fa686f29f54bc493/run-004` 中确认 VPC-2 至 VPC-5 的收敛行为已经生效：
连续两次 accepted-but-no-progress validation 后，reducer 按封存上限从
`produce_validation` 升级为 `produce_state_change`，未再重放 validation-only candidate。
Run 最终以 `candidate_retry_exhausted` 有界失败，且 terminal projection、workspace
active-run release 与 eventstream projection 均已完成。

同次验证重新发现两个必须纳入 VPC-6 fresh deployed gate 的输入/请求合同缺口：

1. `go test` 明确输出 `no tests to run`、但没有任何 parsed check 时，未配置 marker 的
   Shell receipt 曾错误标为 `valid`。Shell 现必须将该通用 tool-level zero-work signal
   固化为 `inconclusive + ZeroWork=true`；该规则只比较 owner observation 与工具输出，
   不解释 workspace、LSP 或测试名称。
2. `produce_state_change` 的两次 same-contract provider 请求均用尽 sealed
   `effectiveMaxOutputTokens=4096`，JSON 在边界处 EOF；Provider catalog 允许的输出上限为
   `65536`，且 stable prefix 未漂移。这不是 transport、model selection 或 Resume 缺陷，也
   **不能**通过把静态默认值改为 8192 或其他固定值收口：任何固定值都可能不足。Selected
   catalog/model 的 `maxOutputTokens` 是唯一 provider hard capability，不再叠加通用本地输出
   safety cap。`MaxInputTokens` 由 `contextWindowTokens - effectiveMaxOutputTokens` 与上下文利用率
   派生，sealed `MaxBytes` 再按 token/byte 估算自动换算；不再通过通用
   `llm.budget.maxRequestBytes` 对不同模型施加同一个固定字节截断。超过 provider hard limit 的
   合法候选必须进入
   Runtime V2 的 typed continuation/chunk protocol，而不是耗尽 same-contract candidate retry
   后 terminal。现有 artifact-continuation DTO、repository 与 read model 已存在；V2 主链
   已补入 bounded candidate-output continuation：初始 EOF fragment 与每个 suffix 均先由
   Orchestrator 持久化，再拼接、严格解码并回到原有 candidate admission/seal。该协议不把
   suffix 计为 attempt 1，也不允许 LLM 选择 sequence、transaction 或 chunk budget。

第一项影响 Shell 的 SM-3 receipt input。第二项需要扩展 SM-2/SM-3 的 candidate output
continuation 合同，并保持 SM-1 lifecycle 与 SM-4 recovery/projection 不变；`pkg` 只能承载
DTO、hash、normalize/clone 与纯比较，不能选择输出模式或推进 aggregate 状态迁移。

### VPC-7 — Provider-limited candidate output continuation

**Affected machines:** SM-3 candidate generation/seal; SM-4 restart recovery. SM-1 不变；
SM-2 仍只在完整 candidate admission 后选择下一 action。

**Contract:** activation 将 `maxChunkRetryAttempts` 与 `previousChunkTailBytes` 编译为
`RuntimeExecutionContract.CandidateOutputContinuation`。初始 structured response 在 output
token limit EOF 时返回 typed fragment；Orchestrator 建立 deterministic continuation ID，按
`1..N` 持久化 content hash、tail、transaction、attempt 与 stable prefix。suffix call 只能
携带 Orchestrator-sealed sequence/tail，采用 raw text provider mode；完整 payload 仅通过
`workorchruntimecontract` 的无状态严格 decoder 转为 DTO，随后仍由原 admission/seal
authority 裁决。任何 sequence/hash/tail/transaction 冲突、assembly decode 失败或封存 chunk
budget 耗尽都生成 typed terminal，不回退为无限 candidate retry。

**Independent regression:**

```text
go test ./internal/modules/blocks/llm/biz/runtimev2 -run 'TestRequestCandidate.*OutputLimit' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRequestAndSealRuntimeV2Candidate.*Output' -count=1
```

必测：首段+suffix 成功、restart 从已落盘首段继续、duplicate chunk/hash 冲突、final assembly
decode failure、chunk budget exhaustion、stable prefix/transaction 一致，以及 raw fragment 不进入
普通 read model。live 验证必须新建 Run；旧 Run 的 sealed 4096 ceiling 与既有 terminal 不迁移。

## Latest deployed convergence audit — `run-006`

### Fact baseline

| 项 | 值 |
| --- | --- |
| Workspace / Run | `ws-fa686f29f54bc493` / `run-006` |
| 最终状态 | `failed` / `no_progress_exhausted` |
| State / evidence revision | `134` / `16` |
| Completed dispatches / candidate attempts | `16` / `17` |
| Accepted / rejected evidence | `6` / `4` |
| Repair budget | `10/10` |
| Provider transport failure | `0` |
| Projection watermark | `sourceEventRevision=766`，`projectedSourceRevision=766` |

本轮已经排除 provider/model/budget、EventStream、terminal delivery 和 Run lifecycle：sealed model
为 `ai-proxy/DeepSeek-V4-Flash`，`contextWindowTokens=1000000`，
`effectiveMaxOutputTokens=65536`；所有请求均 `overBudget=false`、`truncated=false`，stable prefix
始终为 `candidate-prefix-6240a31f431108173e72`。Run 正确执行
`pending -> active -> failed`，Workspace `activeRunId` 已释放，projection 已完全追平。

因此 `run-006` 不是 SM-1 lifecycle、SM-3 sealed transaction 或 SM-4 delivery 卡死，而是
SM-2 在有界预算内没有把 validation failure 落到能够缩小当前 gap 的生产目标。

### New counterexample

```text
Focus.TargetRefs = nil
  -> repair prerequisite 只取得 lsp_test.go target snapshot
  -> state-changing candidates 持续修改验证入口
  -> production MCP/LSP adapter 未进入 repair target set
  -> callMCPTool placeholder 继续返回固定 "ok"
  -> definition/hover 产生伪通过，diagnostics/unsaved-edit 仍失败
  -> direct-validation coverage 不变
  -> repairAttempts 达到 10/10
  -> no_progress_exhausted
```

有界 terminal 本身符合 guard，但不能证明有效正向收敛。新的缺口是：

1. software-dev profile 没有把 validation entry、failure target、deliverable implementation
   target 的角色声明编译为 typed focus contract；
2. activation compiler 可以生成 `RequireDirectEvidence=true` 但没有 target refs 或 bounded
   target-discovery prerequisite 的 Focus；
3. Orchestrator 在 failure target 未落地时仍可请求 repair candidate；
4. Shell receipt 能表达执行目标，但 validation entry 与 related failure target 的关系不充分；
5. LLM 收到的 authority context 缺少 grounded implementation target，因而可持续提出
   test-only patch；
6. accepted durable file change 与“当前 coverage gap 实际缩小”仍需进一步区分。

## VPC-8 — Owner-grounded repair target convergence

**Affected machines:** SM-2 为主；SM-3 负责 target-bound candidate admission；SM-4 负责
committed target/failure snapshot 的 restart reuse。SM-1 lifecycle 不变。

### Canonical interaction contract

```text
software-dev profile declares neutral target/evidence roles
  -> workorchd activationcore compiles typed Focus/Bootstrap
  -> Orchestrator detects missing target grounding
  -> Orchestrator issues typed prerequisite command
  -> canonical owner observes manifest/target/failure facts
  -> Orchestrator CAS-persists snapshot and seals RepairDirective
  -> LLM renders sealed authority and proposes bounded operations
  -> capability owner executes and returns typed receipt
  -> Orchestrator admits evidence and compares persisted gap delta
  -> progress / retarget / governance / terminal
```

跨 owner 交互只能使用 EventHub concrete typed command/result。target discovery 必须由 canonical
owner 观察；LLM 文本、projection、路径启发式和进程内临时 map 都不能成为 authority。

### VPC-8A — Software-dev profile contract

**Owner:** `internal/pkg/workorchdefaults/defaults/souls/software-dev` 及 software-dev profile
默认合同。

需要声明 validation entry、failure target、deliverable implementation target 的中立角色，
direct-evidence focus 在目标未知时允许的 bounded discovery source/selector，placeholder/harness
不能替代 direct runtime evidence，以及 accepted/rejected operation kinds 与 required evidence
role。这些规则必须进入 typed profile contract，不能只存在于自然语言 prompt。

不得把 `_test.go`、Go、LSP、MCP 或具体工具名称写进 Runtime core。独立回归必须证明
software-dev 声明存在，而 neutral profile 不会被注入 software-dev target 语义。

### VPC-8B — Activation focus/bootstrap compilation

**Owner:** `internal/modules/application/workorchd/biz/appservice/activationcore`。

主要文件：`runtime_v2_focus_compiler.go`、`runtime_v2_bootstrap.go` 及 focused tests。

1. profile 的 target/evidence role、selector/discovery、direct-evidence 要求无损编译到
   `CoverageFocus`、`RuntimeRequirement` 和 `RuntimeBootstrap`；
2. compiler 保持 owner-neutral，只做 declarative contract compilation；
3. direct-evidence focus 若既没有确定 target，也没有 bounded discovery contract，activation
   必须 fail-fast；
4. resume/restart 使用当前定义构造的新 Run bootstrap，不复用旧终态缺失字段；
5. application 只构造 bootstrap 和发出 Start/Resume，不选择 repair action。

独立回归覆盖 exact/wildcard profile、neutral profile、missing-grounding fail-fast、resume
bootstrap、definition revision 与 deterministic focus ordering。

### VPC-8C — Runtime contract DTO boundary

**Owner:** `internal/pkg/workorchruntimecontract`，仅合同和纯无状态 helper。

按实际缺口补齐或强化 `CoverageFocus`、`RuntimeRepairDirective`、`FailureTarget`、
`RuntimeAssessmentGap`、`RuntimeContinuationFailure`、`ValidationDiagnostic`、`TargetSnapshot`，
以及 receipt 中 operation target 与 related failure target 的区分和 target/evidence role、
selector/discovery binding 的 typed DTO。

允许 `Normalize`、`Validate`、`Clone`、`Same`、stable hash 和集合比较；禁止选择 next action、
增减 repair counter、推进 Focus、执行 acquisition/dispatch/terminal，以及访问 repository、
EventHub 或修改 aggregate。

独立回归覆盖 normalize/hash 稳定、clone isolation、invalid role/binding 拒绝、round-trip 和
字段顺序不影响 canonical hash。

### VPC-8D — Orchestrator target grounding and effective progress

**Owner:** `internal/modules/kernel/orchestrator/biz/runtimev2`。这是 VPC-8 的主状态迁移面。

1. `RequireDirectEvidence=true` 且 target grounding 不完整时，只能选择
   `AcquirePrerequisite`，不能请求 state-changing LLM candidate；
2. assessment gap、operation failure、Shell diagnostic 与当前 Focus refs 必须合并为 persisted
   `FailureTarget` 和唯一 `RepairDirective`；
3. target/failure snapshot 必须绑定 workspace、run、focus、authority revision、owner、subject
   与 freshness dependencies；
4. candidate operation target 必须命中 sealed target/admissibility/selector contract；
5. accepted receipt 只有在 coverage、gap、required evidence、focus 或 profile 声明的 target role
   上产生持久化 delta 时才构成 effective progress；
6. 同 failure signature 下连续只修改非 required target 时，进入有界 retarget/reacquire，
   不得反复清空 no-progress window；
7. Core 依据 typed role 和 persisted delta 判断，不依据路径后缀判断 production/test；
8. retarget 仍不可行时进入 typed governance 或 terminal，不能提高 repair budget掩盖问题；
9. crash/restart 必须复用 committed FailureTarget、TargetSnapshot、RepairDirective、rejected/
   no-progress shapes 和 repair ledger；
10. duplicate/stale receipt、snapshot、candidate 不得重复扣减预算或制造 progress。

独立回归至少覆盖 missing target acquisition、failure target sealing、validation entry 与 failure
target 不同、非 required target durable change 不算 progress、required target gap delta、same-gap
retarget、focus advance、stale/duplicate，以及 acquisition/commit/dispatch/evidence/progress 各
crash boundary 的 restart 等价性。

### VPC-8E — Shell validation failure targeting

**Owner:** `internal/modules/blocks/shell/biz`。

Shell owner 只报告观察事实，稳定返回 operation `TargetRefs`、failed check ID、failure
class/code/signature、coverage targets、observed claims、executed/passed/failed/skipped 与 bounded
diagnostic preview。当前没有可绑定 owner/subject/role/revision 的 structured related-target 协议，
因此 Shell failure target observation 必须为 nil；验证入口不能被自动折叠为唯一 failure target。

Shell 不关闭 coverage、不更新 repair ledger、不选择 retarget/action。独立回归覆盖 validation
entry 与 diagnostic target 相同/不同、多个失败目标、无法解析目标、zero-work/all-skipped/
non-zero exit、bounded output 和 duplicate execution receipt。

### VPC-8F — LLM candidate authority rendering

**Owner:** `internal/modules/blocks/llm/biz` 与 `biz/runtimev2`。

LLM block 必须无损渲染 current focus、RepairDirective、current failure、FailureTarget、
TargetSnapshots、required evidence、rejected/no-progress shapes、admissibility 和 validation
diagnostics。candidate schema 继续严格拒绝 unknown field，并保持 attempt 0/1、transaction、
stable prefix 不变。

LLM block 不得自行发现或扩展 target、判断 productive progress、修改 repair budget、选择
reducer action，或把模型自由文本提升为 owner observation。

独立回归覆盖 context projection round-trip、target/failure role 不丢失、volatile grounding 只进入
C3-C5、C0-C2 stable prefix 不漂移、schema retry authority 不变和越界 target seal 前拒绝。

### VPC-8G — Projection and non-owner regression only

| 组件 | 允许修改 | 禁止修改 |
| --- | --- | --- |
| `blocks/file` | target/failure snapshot acquisition、freshness 与 restart tests | 决定 repair target/action |
| `kernel/run` | bootstrap/state CAS、terminal/restart tests | reducer、progress 判定 |
| `blocks/result` | rejected closeout 中展示 failure/coverage refs | 重新裁决 terminal |
| `kernel/eventstream`、Web/TUI | 展示 failure/repair target、no-progress、attempts | 重建 authority、触发 retarget/Resume |
| `blocks/git` | 前序 focus 关闭后的 add/commit sequence 回归 | 绕过当前 focus或选择下一 focus |

### VPC-8 implementation order

1. `define runtime repair target grounding contracts`（VPC-8A/C）；
2. `compile grounded runtime coverage focuses`（VPC-8B）；
3. `acquire and persist runtime failure targets`（VPC-8D prerequisite）；
4. `bind runtime repair progress to required targets`（VPC-8D progress/retarget）；
5. `report shell validation failure targets`（VPC-8E）；
6. `render sealed runtime repair authority`（VPC-8F）；
7. `verify grounded runtime repair convergence`（VPC-8G/integration/live）。

不能先在 LLM prompt 中增加“请修改生产代码”作为临时修复；在 typed contract、activation
compiler 和 Orchestrator authority 未落地前，prompt-only 修改不构成收口。

### VPC-8 cross-component acceptance matrix

| 场景 | 必须观察到的迁移 |
| --- | --- |
| direct evidence + target missing | SM-2 `AcquirePrerequisite`，provider call 不增加 |
| owner target snapshot committed | 同一 focus/revision 绑定 FailureTarget/RepairDirective |
| candidate 只命中 validation entry | admission 拒绝或记 no-progress，不清 ledger |
| required target durable change | evidence admitted，gap delta 后只 NoteProgress 一次 |
| MCP failed response 指向实现目标 | operation target 与 owner-observed failure target 均保留 |
| same failure repeated | bounded retarget/reacquire，不重放旧 shape |
| crash/restart | 复用 committed grounding，不重发旧 dispatch |
| target dependency stale | 旧 snapshot 失效并有界 reacquire |
| focus satisfied | 原子 advance，下一 focus 获得新 repair ledger |
| no feasible grounded target | typed governance/terminal，不伪进展 |

### VPC-8 live close gate

使用新 Run 和最新二进制验证，不迁移 `run-006` terminal aggregate：

1. Focus 或 prerequisite 明确提供 owner-grounded target；
2. direct-evidence target 缺失时 provider 不被提前调用；
3. 正向 MCP 场景的 candidate context 同时包含 validation entry 与 owner-observed failure target；
4. state change 命中需要修复的实现目标，而非持续只修改验证入口；
5. placeholder/harness 证据不能关闭 direct-validation coverage；
6. diagnostics、unsaved-edit 等失败能关联到实现目标；
7. 每个 repair window 产生 gap/coverage/target/focus delta，或进入 retarget/governance/terminal；
8. 不再以 10 次 test-only durable change 耗尽总 repair budget；
9. stable prefix、provider ceiling、sealed transaction 继续稳定；
10. projection high watermark 追平，terminal delivery exactly once；
11. restart 不重放旧 dispatch/receipt/snapshot；
12. canonical LSP-Bridge workspace 能进入后续 focus，并最终完成真实 validation 与 Git closeout。

### VPC-8 current closure baseline and remaining slices

下表是截至 2026-07-21 的代码核对结果，不是 completion record。它把已经具备的
合同基础与仍会导致 `Focus.TargetRefs=nil -> validation-entry-only patch ->
no_progress_exhausted` 的收口项分开，后续代码修改必须按此顺序完成，不能把已存在的
manifest/snapshot 读取误认为“owner 已经选定 implementation target”。

| Slice | 组件与当前基线 | 尚需收口的唯一职责 | SM / 独立回归 |
| --- | --- | --- | --- |
| 8A/C | `workorchdefaults`、`workorchruntimecontract` 已能声明/校验 `TargetRole`、`TargetRefs`、`TargetDiscoverySources`、`RequireTargetGrounding`；`EvidenceFact` 已保留 operation target lineage。 | 保持 discovery source 白名单和 canonical normalize/hash；如需新字段，只能表达 owner observation、selector binding 或 failure relation，不能把选靶、progress 或 action 迁入 `pkg`。 | SM-2 input / `go test ./internal/pkg/workorchruntimecontract ./internal/pkg/workorchdefaults -count=1` |
| 8B | `workorchd/.../activationcore` 已将 profile target 合同无损编译到 Focus，缺少 target/discovery 的 direct-evidence contract 会 fail-fast。 | 保持 application 仅编译 bootstrap；不得为 repair 选择路径、补造 target，或在 Resume 中修改已封存 Run authority。 | activation boundary / `go test ./internal/modules/application/workorchd/biz/appservice/activationcore ./internal/modules/application/workorchd/biz/appservice/activationports -count=1` |
| 8D-1 | `orchestrator/runtimev2` 已能把显式 refs 转为 File target snapshot；File → EventHub → Orchestrator 已具备 `target_selection` observation。File owner 只按有界 inventory 的**唯一性**返回 `selected`、`absent` 或 `ambiguous`，selected target 仍必须取得新鲜 snapshot；无 selected target 不再请求 provider。 | 为多文件 workspace 补 profile-bound owner selector / failing capability 的实际 related target observation。Orchestrator 只能保存、校验 freshness/binding 并将其封存为 `FailureTarget`/`RepairDirective`，不能从 manifest、文件名或后缀猜测 implementation target。 | SM-2 prerequisite + SM-4 reuse / target selection unique、missing、ambiguous、stale/reacquire、restart tests |
| 8D-2 | Orchestrator 已拒绝不命中 grounded target 的 durable candidate；非目标 durable change 不会重置 repair progress，RepairDirective 也不会因无关 evidence 切换为 validation。 | 完成“gap 实际缩小”的 persisted-delta 判定、same-signature retarget 与 no-feasible-target governance/terminal；在 receipt/evidence/progress/dispatch crash 边界复用 `FailureTarget`、snapshot、directive 与 rejected/no-progress shape，duplicate 不得扣预算或重置窗口。 | SM-2 + SM-3 + SM-4 / `go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1` 及 targeted restart matrix |
| 8E | MCP typed receipt 已区分 operation `TargetRefs` 与 owner-observed `RuntimeFailureTargetObservation`。MCP 仅从失败 response 的显式结构化 `failureTargets` 采纳 observation；Shell 没有该协议，失败 receipt 的 observation 和 failure target 均明确为 nil。 | 为 Shell 或其他 capability 补实际 structured observation 时，必须新增完整 owner/subject/role/revision contract；验证 parser/owner observation 必须在多失败 target、不可解析 target、zero-work/all-skipped/non-zero、重复 receipt 下保持该区分；owner 不得关闭 coverage 或决定 retarget。 | SM-3 owner input / `go test ./internal/modules/blocks/shell/biz ./internal/modules/blocks/mcp/biz -count=1` |
| 8F | LLM dynamic authority context 已投影 Focus target contract、FailureTarget、TargetSnapshots 与 RepairDirective，相关字段仅位于 C3-C5。 | 对 owner-selected target result 做 context round-trip 和 seal-before-provider gate；LLM 不得把自由文本、路径搜索或“建议修改某文件”提升为 target observation。 | SM-3 request rendering / `go test ./internal/modules/blocks/llm/biz -run 'TestRuntimeV2CandidateContext' -count=1` |
| 8G | File target snapshot command、Run CAS、Result/Eventstream projection 与 TUI/Web 现有展示链可复用。 | 增加跨 owner state-sequence 和 fresh deployed gate：缺 target 时零 provider call；MCP observation target 与 validation entry 同时可见；正确 target 的 durable delta 才推进；watermark/terminal/restart exactly-once。投影只能展示，不得重建 authority。 | SM-2/3/4 / focused integration + fresh clean-data Run |

#### 8D-1 owner-selected target contract

当前 `workspace_manifest`、`project_files` 等 source 只证明“可供发现的 owner inventory
已经取得”，并不证明其中任一文件就是实现目标。因此在 Focus 没有显式 `TargetRefs` 时，下一
步不是放宽 candidate admission，也不是把 validation entry 作为默认 repair target，而是增加
下列有界交互：

```text
sealed Focus target role + bounded discovery source
  -> Orchestrator requests typed owner observation (EventHub)
  -> File / failing capability owner returns selected target or typed absent/ambiguous result
  -> Orchestrator binds owner, subject, revision and freshness dependencies to RuntimeFacts
  -> RepairDirective admits only that committed target set
  -> candidate/receipt evidence is compared with the sealed set
```

结果必须是 concrete typed command/result；不能以 `map[string]any`、projection read model、LLM
JSON、临时缓存或 application callback 传递。若 owner 只能提供 inventory 而不能在声明的
selector 内唯一选择目标，必须返回 typed `absent`/`ambiguous` observation，随后由 SM-2 选择
有界 reacquire、governance 或 terminal。不得回退为 test-only patch，也不得通过提高 repair
budget 掩盖目标缺失。

#### Required implementation and review order

1. **已完成基础闭环：** File block 的 `target_selection` concrete EventHub command/result、纯
   DTO/normalize/freshness binding、Orchestrator acquire/CAS persistence，以及 unique target 的
   snapshot-before-candidate gate。多候选只形成 `ambiguous` owner fact，不能隐式选择第一项。
2. 为 profile-bound selector 和 failing capability 的 actual related target 补 concrete observation；
   在 Orchestrator prerequisite/commit/reducer 中完成其 stale invalidation、RepairDirective
   sealing 与 duplicate/restart reuse。
3. 在 MCP receipt 和 LLM dynamic context 中贯通 owner-observed failure target 与 selected target，增加
   target mismatch 的 seal-before-provider/admission 回归。
4. 增加覆盖 selection→candidate→receipt→gap delta→focus advance 的跨组件矩阵；随后用清洁
   runtime data 建立新 Run 执行 live close gate。旧 `run-006` 不迁移、不重跑，也不能作为
   positive evidence。

每个切片的提交说明必须列出影响的 SM ID、唯一 state owner、输入/输出 typed contract、持久化
delta、duplicate/stale/restart 行为与独立回归命令。任一切片没有这些内容时，不能以“单点修复”
形式合并。

#### 8D-02 / 8E-02 next closure — actual failure-target observation

**Status:** `open`。本切片收口“失败能力 owner 实际观察到的 implementation-related
target”，不把 sealed operation 的输入 target、诊断入口或 LLM 文本误提升为该事实。

当前 guard 已经阻止未 ground 的 validation entry 直接写为 `FailureTarget`；但这只是防止错误
升级，尚未使 owner 能够提供一个可验证的实际 related target。下一轮实现必须按下列合同分层：

| 组件 | 必须完成的唯一职责 | 输入/输出合同 | 不能做的事 |
| --- | --- | --- | --- |
| profile/default contract | 声明 capability observation 可满足的 target role、subject 与 bounded discovery source。 | typed Focus/requirement 的 role、selector、discovery binding。 | 依据文件名、后缀或工具名推断 implementation target。 |
| `blocks/file/biz` | 对 profile-bound selector 返回唯一 `selected`，或 `absent`/`ambiguous`。 | 已有 typed target-selection observation、workspace-tree revision。 | 以 inventory 的第一项替代 actual failure observation。 |
| `blocks/mcp/biz` adapter | 仅在 MCP **失败响应**含有显式、结构化的 `failureTargets` 字段时，归一化并报告该字段。operation input 的 `targetRef`/`path`/`uri` 只能保留为 diagnostic context。 | `MCPResponsePayload.JSON` → bounded/deduped owner diagnostics `FailureTargets`；缺字段即空 observation。 | 从 request input、自然语言错误、LSP/语言语义或固定 placeholder 猜目标；将 `result.TargetRefs` fallback 为 failure target。 |
| `blocks/shell/biz` adapter | 当前只报告 Shell 可观察的 validation/diagnostic 事实；没有结构化 related-target 协议时，failure target observation 必须为 nil。 | typed diagnostic/receipt 的 operation target lineage；未来新增 observation 必须单独绑定 owner、subject、role 与 revision。 | 从测试/LSP 输出的自由文本、命令入口或文件后缀推断实现目标。 |
| `orchestrator/biz/runtimev2` | 校验 role、owner、subject、authority/freshness binding；CAS 封存合格 observation 为 `FailureTarget`/`RepairDirective`，并选择 prerequisite、retarget、governance 或 terminal。 | concrete typed owner result、facts、freshness observation、sealed authority transaction。 | 调用 capability I/O、从 manifest/诊断文本猜测 target，或将 receipt 自行变成 progress。 |
| `blocks/llm/biz` | 将已封存的 selected/failure target 以 C3-C5 动态 authority 渲染。 | sealed candidate context。 | 增补/改写 owner observation、选择 target 或 action。 |
| Run/Result/Eventstream/Web/TUI | 复用事实展示、CAS/restart 与审计验证。 | typed aggregate/event/read-model DTO。 | 重建 authority、发送 Resume 或对 failure target 重新裁决。 |

规范化后的观察必须同时满足以下条件，才可被 Orchestrator 纳入 grounded repair target
precedence：

1. 来源是对应 capability owner 的 concrete typed result，且与 workspace/run/focus/subject
   scope 一致；
2. target role 与当前 Focus 的声明兼容，owner revision 和依赖 revision 可以进入 freshness
   observation；
3. target 值来自 owner 的显式结构化失败观察或 profile-bound selector，绝非 operation input
   的回填；
4. target collection 经过 bounded trim/dedupe/canonical normalize，空 collection 表示
   `unavailable`，不是以 validation entry 补位；
5. observation 与当前 authority transaction、target snapshot/admissibility 不一致时只能失效并
   进入 prerequisite，不能继续请求 repair candidate。

推荐的 MCP 端到端序列如下；该序列仅使用中立的结构化字段，不绑定 LSP、Go 或特定工具：

```text
sealed operation target = diagnostic-entry
  -> MCP owner executes operation
  -> failed response has explicit failureTargets = [implementation-target]
  -> MCP adapter emits owner diagnostic (entry context != observed failure target)
  -> Orchestrator validates role/freshness and CAS-persists target binding
  -> target snapshot + admissibility complete
  -> LLM receives only sealed binding and proposes a target-bound operation
  -> receipt/assessment proves gap delta, or SM-2 retargets/governs/terminates
```

若 response 没有 `failureTargets`，或值无法通过 role/freshness 校验，序列必须停在
`AcquirePrerequisite`、有界 retarget、governance 或 typed terminal；绝不能以
`diagnostic-entry`、`result.TargetRefs`、命令参数或 prompt 建议替代 implementation target。

**修改顺序与独立回归：**

1. 在 MCP adapter/owner 中移除从 operation input 派生 `FailureTargets` 的路径；添加 response
   JSON 顶层显式字段的纯本地解析、bound/normalize/dedupe 测试。至少覆盖：显式 target 与输入
   target 不同、无字段、多个 target、无效值、success response 不能产生 failure target、重复
   receipt。运行 `go test ./internal/modules/blocks/mcp/biz -count=1`。
2. 对 Shell 明确区分“已结构化观察”与“不可观察”，补充 target 不可得、多个 target、重复执行
   receipt 的 owner tests；不解析自由文本。运行
   `go test ./internal/modules/blocks/shell/biz -count=1`。
3. 在 Orchestrator 保持 guard，并增加 observation 的 role/freshness/authority binding、
   absent/ambiguous、target mismatch、pre-provider zero-call、stale invalidation、duplicate 与
   receipt/evidence/commit crash-restart matrix。运行
   `go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1`。
4. 在 LLM context 测试验证 observation 只有 CAS 封存后才出现在 C3-C5；C0-C2 与
   `stablePrefixHash` 不得因 target 变化而漂移。运行
   `go test ./internal/modules/blocks/llm/biz -run 'TestRuntimeV2CandidateContext' -count=1`。
5. 最后增加 selection → observation → snapshot → candidate → receipt → assessment 的跨组件
   state-sequence，以及清洁 runtime data 的新 Run live gate。每一 crash/restart 点都必须证明
   不重发旧 dispatch、不重复扣预算，且 projection 只显示已裁决事实。

本切片影响 SM-2（observation binding、retarget/progress）、SM-3（receipt/candidate admission）
和 SM-4（restart reuse）；SM-1 不变。不得为该切片引入兼容分支、历史 aggregate 迁移、V1
continuation 或 `pkg` 状态迁移。

#### 8D-02 implementation register — owner-observed failure target binding

本登记表将上一节的方案拆成可独立提交的收口单元。状态 `planned` 表示合同已经确定，
不表示实现或 live 验证已完成；每一单元只能在自身测试、相邻 EventHub 合同测试和指定的
状态序列测试均通过后更新为 `landed`。当前阶段不提供旧 receipt、旧 aggregate 或 V1 路径的
兼容分支。

| 顺序 / 状态 | 唯一实现 owner | 需要落地的 typed 合同与持久化事实 | 允许的状态机迁移 | 必须拒绝的输入 | 最小独立回归 |
| --- | --- | --- | --- | --- | --- |
| O1 / `landed baseline` | `internal/pkg/workorchruntimecontract` | 已新增纯 DTO `RuntimeFailureTargetObservation`：`OwnerModuleID`、`SubjectRef`、`TargetRole`、`ObservedRevision`、canonical `TargetRefs`。`FailureFact`、`FailureTarget`、receipt stable hash、repair directive equality/clone 已保留该值。 | 无；只提供 normalize/validate/clone/hash/equality。 | 缺 owner/subject/role/revision/target，或 observation targets 与 failure targets 不一致。 | contract normalize、clone isolation、invalid/mismatched observation、receipt hash 与 directive equality 因 observation 变化而变化。 |
| O2 / `landed baseline` | `kernel/orchestrator/pkg/events`、`kernel/orchestrator/biz/runtimev2` | sealed MCP operation 已传递 Focus 的声明性 `TargetRole`；owner result 携带 `OwnerRevision` 与可选 observation。receipt commit 校验 owner、role 和 exact target set。 | SM-3 receipt admission；SM-2 仅 CAS 保存已验证事实。 | 由 executor、application、LLM 或 operation input 伪造 observation；owner/module/role/target 不匹配。 | concrete EventHub MCP command 的 scope/role round-trip；wrong role/targets、missing owner revision。 |
| O3 / `landed baseline` | `blocks/mcp/biz` | 仅当**失败** response 顶层显式给出结构化 `failureTargets` 时，生成 MCP owner observation；`SubjectRef` 为 server identity，revision 为与 admissibility prerequisite 同源的 canonical MCP owner-catalog revision，role 来自 sealed Focus declaration。 | 无 aggregate 迁移；只形成 capability receipt。 | success response、request `path`/`uri`/`targetRef`、`result.TargetRefs`、自然语言错误或 placeholder 返回值。 | explicit target、no field、success response、input 与 observed target 不同、bound/dedupe、duplicate receipt、role/revision 缺失。 |
| O4 / `landed baseline` | `blocks/shell/biz` | 当前没有结构化 related-target source 时返回 nil observation。未绑定 `RelatedFailureTargets` output 已删除；未来协议必须新增独立 typed observation，并由 owner 一次性给出 role、subject、revision 和 target 后才可接入 O2。 | 无。 | 将 sealed command target、validation entry、自由文本、文件后缀、测试名称或 LLM candidate parameter 回填为 implementation target。 | operation contract 拒绝 candidate-claimed `relatedFailureTargets`；no-fallback、multiple/unavailable target、zero-work/all-skipped/non-zero、duplicate receipt；未来 structured protocol 要补 role/revision negative tests。 |
| O5 / `landed baseline / open restart matrix` | `kernel/orchestrator/biz/runtimev2` | 合格 observation 已 CAS 绑定到 `FailureFact`、`FailureTarget` 与 `RuntimeRepairDirective`，并进入 File owner target snapshot/admissibility gate。匹配的 owner-catalog revision stale 会原子清除 observation-derived failure target/directive，并 re-acquire prerequisite。 | SM-2 `AcquirePrerequisite` / retarget / governed wait / typed terminal；SM-3 只允许已封存 binding 进入 candidate context。 | validation entry 单独升级、target role 不同、observation targets 与 failure targets 不同、没有 File snapshot 即 provider request。 | compatible MCP observation -> `FailureTarget` -> target snapshot prerequisite，role/target mismatch、duplicate receipt、owner-catalog stale invalidation 已覆盖；commit 前后 crash/restart 仍 open。 |
| O6 / `landed baseline` | `blocks/file/biz`、Orchestrator prerequisite bridge | File owner 对 O5 需要的 refs 提供 current `TargetSnapshot` 和 freshness dependencies；File selection 与 capability failure observation 是两个事实来源，不能互相伪造。 | SM-2 prerequisite acquisition/reacquire；SM-4 复用已提交 snapshot。 | inventory 第一项、validation entry 或过期 snapshot 充当 implementation observation。 | persisted FailureTarget 触发 snapshot 已覆盖；absent/ambiguous、workspace-tree resource change stale、crash before/after prerequisite commit restart 继续属于 O8 matrix。 |
| O7 / `landed baseline` | `blocks/llm/biz` | 已渲染 CAS 封存的 observation / FailureTarget / directive 到 C3-C5，projection 可关联 owner、role、subject、revision 和 target refs。 | SM-3 provider request，不修改 authority。 | LLM 增补 target、把文本建议升级为 observation、将 volatile observation 放入 C0-C2 导致 stable prefix 漂移。 | context round-trip、C0-C2 hash 稳定、candidate target mismatch 在 seal 前拒绝；uncommitted observation 不可见由 O5 的 aggregate-only authority 保证。 |
| O8 / `partial` | `kernel/orchestrator/biz/runtimev2`、`kernel/run`（CAS/recovery） | MCP owner-catalog stale 后已由 RuntimeController CAS 持久化 invalidated candidate 与 pending re-acquire；Run-owner CAS repository matrix 同时覆盖 commit 前和 commit 后：写入失败时 persisted aggregate 保留原 observation/pending candidate，重试同一 stale 后才提交；重启 controller 只加载已提交 aggregate，不恢复 stale `FailureTarget`/directive 或旧 candidate，重复 stale 也不重复消耗 recovery ledger。另已覆盖 `validation_scratch`、File `target_selection` 和 File `target_snapshot` prerequisite，以及 candidate proposal 与 dispatch seal：CAS 写入失败后资源/selection/snapshot/proposal/dispatch 均不出现，pending prerequisite 或 pending attempt 保持；重启后仅能从成功 commit 的 aggregate 继续，重复 result 不变更 revision。owner-observed failure receipt 同样覆盖：CAS 失败后不出现 receipt、`FailureFact`、`FailureTarget` 或 rejected evidence；重启只从 claimed read-only operation 重放，成功后一次性提交完整 observation/evidence。assessment commit 同样覆盖：失败不出现 assessment、coverage/progress、dispatch close 或 terminal；重启只从 sealed assessment dispatch 提交一次。terminal delivery 的 commit-before/after crash point 仍须从 aggregate 重放。 | SM-2/SM-3 的 MCP stale、prerequisite、candidate proposal/seal、owner failure receipt、assessment progress restart 等价已覆盖；SM-4 only delivery retry。 | 使用 projection、process-local cache 或 Resume 重建 observation/authority。 | complete state-sequence matrix、其余 crash point restart、stale/duplicate、terminal delivery pending。 |

O1--O8 的因果顺序必须保持为：

```text
owner response observation
  -> Orchestrator validates and CAS-binds it
  -> File snapshot/admissibility becomes fresh
  -> only then render/request candidate
  -> sealed receipt/evidence
  -> assessment proves persisted gap delta
```

其中 O3/O4 可以并行开发，但 O5 不得在 O1--O4 的任一未验证路径上放宽 grounding gate。
O6 必须在 O5 首次允许 provider 前完成；O7 只能消费 O5 已提交事实；O8 是所有前置单元的
关闭条件，不能以单个 adapter 单测代替。

#### Remaining code-closure execution order

以下是基于 O1--O8 当前状态的**剩余**代码收口顺序。它不是要求所有 22 个组件修改代码：
已经标为 `landed baseline` 的组件先保持行为冻结，只补其指定的负例、重启或真实 EventHub
回归。任何步骤发现既有事实不满足合同，才在该事实的唯一 owner 内修复；不得把问题迁入
`pkg`、application projection 或另一个 capability owner。

| 顺序 | 状态 | 唯一代码收口组件 | 要完成的最小闭环 | 状态机与禁止扩散 | 独立完成证据 |
| --- | --- | --- | --- | --- | --- |
| R1 | `partial` | profile contract、`application/workorchd/.../activationcore`、`blocks/file/biz` | 已新增 opaque `TargetSelectorRef`，从 profile/default 编译到 Focus、target-selection prerequisite、typed EventHub command 与 File owner receipt。File 当前只声明并执行 `file/unique_workspace_entry`：唯一 regular file 才 `selected`，否则为 `absent`/`ambiguous`；未知 selector 拒绝，不会从 inventory 或 LLM 建议选第一项。Orchestrator 的 EventHub receive boundary 现要求 File selection 同时携带与 selection snapshot 精确一致的单一 File `workspace_tree` freshness observation；缺 observation、role、snapshot hash 或 tree revision 不匹配都不得写入 aggregate。仍需由真正拥有领域选择规则的 profile/capability owner 增加其它 selector。 | 不改变 SM-1/SM-4；SM-2 只消费 observation。application 不得在 Resume 补靶或选 repair action。 | profile/default、activation、File owner、concrete EventHub tests 覆盖 selector round-trip、unknown selector、unique/absent/ambiguous、missing/mismatched observation；仍需 semantic selector 的 definition revision drift 与 clean-data live。 |
| R2 | `open (protocol prerequisite)` | `blocks/shell/biz`，或真正观察失败关联的 capability owner | 仅在外部协议能提供可验证的 subject、role、owner revision 和结构化 related target 时，形成 `RuntimeFailureTargetObservation`；没有该协议时保持 nil。Shell sealed operation 已不接受 `relatedFailureTargets`，因此不能把 command entry、自由文本、后缀或测试名回填为 implementation target。 | 仅 SM-3 owner receipt input；禁止 coverage/progress/retarget/action。不得为本项新建 Shell→Orchestrator service/repository 直连。 | operation-contract negative test + owner Biz/concrete EventHub tests：structured/missing/multiple/role mismatch/revision stale/duplicate、zero-work/all-skipped/non-zero；无 structured source 时断言 nil observation。 |
| R3 | `open` | `kernel/orchestrator/biz/runtimev2`；`kernel/run` 仅提供 CAS/recovery fixture | 补全 O5/O8 的持久化 crash/restart 状态序列：acquisition/observation/snapshot/dispatch/receipt/evidence/assessment-progress/terminal 各边界重载 aggregate，确保 stale owner catalog 后不会复活旧 FailureTarget、directive 或 dispatch。candidate proposal、dispatch seal、owner-observed failure receipt 与 assessment/progress 的 Run-owner CAS commit-before/restart 已覆盖；终态 delivery 等其余边界继续按同一矩阵收口。 | SM-2/SM-3 是唯一 authority；SM-4 只重投已封存 terminal/projection。禁止通过 Resume、EventStream、process-local map 或 Run projection 重建 observation。 | file-backed controller matrix：每个边界 crash before/after commit、duplicate/stale receipt、CAS reload、terminal delivery pending；重启后 operation/evidence/repair budget exactly-once。 |
| R4 | `open (integration/live)` | `blocks/file`、`blocks/llm`、`blocks/mcp` 与 `blocks/shell` 的合同回归；`workorchd`、Result/Workspace/Eventstream/Web/TUI 仅做展示/投影回归 | 以真实 typed EventHub 串起 owner observation → Orchestrator binding → File snapshot/admissibility → C3-C5 candidate context → sealed receipt → assessment delta；再以清洁数据的新 Run 验证。终态 delivery 已补 Result → Workspace release → Run terminal 的实际 EventHub owner chain 和 duplicate convergence；MCP 已有 observation 只回归，不得改写为通用 target 推断。 | 覆盖 SM-2/SM-3/SM-4；投影不得发 Resume、重建 authority 或重新判断 evidence。 | focused cross-owner state-sequence、`make verify-boundary-contract`、full test/vet、`make verify-runtime-v2-positive-convergence-file`，以及 VPC-8 live close gate 的 12 项 persisted/API/eventstream/workspace 事实。 |

**R1 local compiler increment.** 一个 `CoverageFocus` 只能表达一组 target
role/selector binding。activationcore 现在在同一 acceptance slice 的 profile policy 声明不同
非空 `TargetRole` 或 `TargetSelectorRef` 时 fail-fast；不再静默保留第一个并丢失其余声明。
相同 binding 仍可安全聚合。该规则只保护 profile → Focus 的无损编译（SM-2 input），不让
application、File 或 Resume 解释 selector 语义或选择实现文件；真正的 semantic selector
仍须由其 profile/capability owner 提供。

**R1 direct-validation contract increment.** `software-dev` 的 `dodItem` 与
`goalCoverage` wildcard evidence requirement 现与 `source_change` 一样声明
`deliverable_implementation` role、`workspace_manifest` / `project_files` discovery source、
`file/unique_workspace_entry` selector 和 `RequireTargetGrounding=true`。activationcore 只无损
编译该声明；唯一 File entry 可继续走 snapshot-before-provider，零或多 entry 必须保持 owner
`absent` / `ambiguous` 事实并在 provider 前停下。该变更影响 SM-2 prerequisite input，不把
软件开发路径语义、按文件名选靶或 repair action 选择引入 application/File/core。默认数据、
`dodItem`/`goalCoverage` compiler 和 File-backed zero-provider ambiguity regression 均已覆盖；
真正的多资源 semantic selector 及 clean-data live 仍使 R1 保持 `partial`。

**R1 direct-validation pre-provider gate increment (2026-07-22).** 新二进制的
`ws-fa686f29f54bc493/run-008` 已封存上述 Focus 字段，但暴露旧的 reducer 只在
`produce_state_change` 检查 grounding：File owner 已持久化 `ambiguous` selection
(`go.mod`、`lsp_test.go`)，仍先请求了一次 `produce_validation` provider candidate 并执行了
无 target ref 的 validation operation；失败后才以测试输出创建泛化 governance ticket。这违反了
VPC-8 cross-component matrix 的“direct evidence + target missing：SM-2 prerequisite 或确定终态，
provider call 不增加”合同。

Orchestrator 现将该门禁收敛为 SM-2 的 `targetGroundingDecision`：所有
`RequireTargetGrounding=true` 的 executable direct-evidence action（state change 与 validation）
都必须先有 `GroundedRepairTargets`；否则无条件选择
`target_grounding_unresolved` terminal，不创建 candidate transaction 或 decision ticket。owner 已完成的
`absent`/`ambiguous` selection 是确定性 feasibility observation，不是外部权限、配置或人工信息：
generic `continue` 无法令候选 target 变为 admissible，创建治理票据只会在 resolution 后重入同一 gate。
SM-3 继续只约束 durable
state-change candidate 必须命中 sealed target；validation candidate 可以保留其独立的 validation
entry / external capability lineage，以便 capability owner 在一次已封存验证中返回结构化
failure-target observation。该变化没有让 File、LLM、Shell、application 或
`workorchruntimecontract` 选择 target；File 的 `ambiguous` 仍只是 owner observation。

唯一的后续 validation 例外是：同一 focus 已有 accepted、target-bound durable state-change
evidence 时，SM-2 可复用该已封存谱系执行无副作用 validation；它不能据此授权新的 durable
candidate。该例外保持 H5 的“target-bound change → fresh owner facts → validation → assessment”
正向链，同时不会让首次缺 target 的 validation（`run-008` 反例）调用 provider。

`TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider` 现包含
`state_change` 与 `direct_validation` 两个真实 File/Run/EventHub 子场景，均断言零 provider call、
零 governance-open、dispatch/receipt/evidence 和不创建 target snapshot；两个 reducer 回归分别验证
state change 与 direct validation 在 governance capability 存在时仍只形成该 terminal。R1 仍为
`partial`：多资源 semantic selector / structured owner failure target（R2）尚不存在，须在下一次 fresh
deployed Run 中验证新的零调用 terminal 事实，不能 resolve `run-008` 的旧泛化 ticket 作为补偿。

**R2 local no-fallback increment.** `TestShellOwnerDoesNotInferFailureTargetFromValidationCommand`
现覆盖相同 execution key/fence 的重复 Shell validation delivery：owner 只返回已提交 receipt，
不二次 commit，也不从 validation entry、diagnostic 或重放结果生成 failure target observation。
这只确认“无结构化 source 时为 nil”的既有 Shell owner 合同；外部 structured related-target
协议仍是 R2 的前置条件。

R1 与 R2 只要任一未具备真实 owner observation，R3 的正确行为仍是保持 target 未 ground：
进入有界 prerequisite 或 typed terminal，绝不能放宽 provider gate。R4 只能在 R1/R2
所需的 observation 已真正持久化后执行 positive convergence 验证；旧 `run-006` 不迁移，也不能
作为本轮正向完成证据。

#### R3/R4 implementation register — remaining crash, owner-chain and live closure

本登记册把“已经由真实 Run owner CAS 或生产 EventHub 覆盖”的边界，与下一步仍需修改或验证
的边界明确分开。它是 R3/R4 的提交顺序；**不授权**把新的状态迁移分散到 Run、Result、
Workspace、EventStream、Web、TUI 或 `workorchruntimecontract`。当前阶段不做历史 aggregate、
旧 receipt 或 V1 路径兼容。

| 序号 / 状态 | 唯一 owner 与允许改动 | 已经可依赖的事实 | 剩余收口动作与明确停止线 | 独立验收 |
| --- | --- | --- | --- | --- |
| R3-1 / `landed baseline` | `kernel/run` 提供 typed CAS repository；`kernel/orchestrator/biz/runtimev2` 通过该 repository 写 aggregate。 | Run-owner CAS 已覆盖 prerequisite、target selection、target snapshot、candidate proposal、dispatch seal、owner-observed failure receipt、assessment/coverage/progress 的 commit-before 失败与 restart。失败写入不会留下半个 snapshot、proposal、dispatch、receipt、evidence、assessment、coverage、repair progress 或 terminal。 | 不在 Run owner 增加 reducer、action、target 或 repair 规则。后续只在 Orchestrator 的同一 aggregate transition 增加尚未覆盖的 crash matrix；任何恢复都只 `Load` committed aggregate。 | `TestRuntimeV2StateRepository(PrerequisiteCommitCrashRestartIsAtomic|TargetSelectionCommitCrashRestartIsAtomic|TargetSnapshotCommitCrashRestartIsAtomic|CandidateProposalAndDispatchSealCrashRestartAreAtomic|OwnerFailureObservationReceiptCrashRestartIsAtomic|AssessmentCommitCrashRestartIsAtomic)`。 |
| R3-2 / `landed local` | `kernel/orchestrator/biz` 的 terminal delivery/recovery lane；Run、Result、Workspace 只消费既有 typed command。 | reducer 已先持久化 terminal authority；Result persistence 前后 interruption、重复 terminal delivery、Result → Workspace release → Run terminal 的顺序已有回归。真实 Run/Workspace/Result EventHub owner 与 file-backed repositories 覆盖“Workspace release 已落盘但未确认”和“Run terminal 已落盘但未确认”：重启只重投同一 terminal，收敛到单一 Result、已释放 active run 和 terminal Run。该矩阵还以已封存 MCP owner `FailureTarget` observation 验证，SM-4 acknowledgement-loss 重启前后 aggregate revision、terminal identity、FailureTarget owner/subject/role/revision/target refs 均不变；投影完成后迟到的 MCP owner-catalog stale 因无 pending candidate 被拒绝，不能重新进入 prerequisite acquisition。terminal-delivery ledger 达到上限时也只记录调度额度；aggregate terminal 不变，重启 inventory 返回 `terminal_delivery_pending`，而 `TestRuntimeV2StartupRecoveryDeliversPendingTerminalThroughOwnerChain` 进一步证明重启的 startup inventory 会实际调度同一 SM-4 owner chain 并清除该 inventory。 | 保持 Result 未写入、Result 已写入未确认、Workspace 已释放未确认、Run 已终态重复重放四个边界。恢复只能重投同一 committed terminal；不得把投影错误改写为新的 reducer terminal，也不得重新执行 operation 或增加 candidate/repair 预算。 | `TestDeliverRuntimeV2Terminal(CreatesResultAndProjectsOwnersIdempotently|RestartsAfterResultPersistenceBeforeOwnerProjection|RestartsAfterOwnerProjectionAcknowledgementLoss|UsesTypedOwnerEventContracts)`、`TestRuntimeV2TerminalDeliveryBudgetExhaustionRetainsStartupRecoveryInventory`、`TestRuntimeV2StartupRecoveryDeliversPendingTerminalThroughOwnerChain`；每项断言一个 Result、active-run 最终释放、Run terminal、owner-grounded FailureTarget、operation/evidence/repair ledger 均不变。 |
| R3-3 / `partial` | `kernel/orchestrator/biz/runtimev2`（SM-2/SM-3 transition）与其 recovery tests。 | stale MCP owner catalog 已能原子失效 observation-derived FailureTarget、directive 与 pending candidate；重启不复活旧 target 或重复扣 prerequisite refresh。另有 `TestRuntimeV2FileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries`：以真实 File owner 的 unique target-selection observation、snapshot、candidate proposal、state-change receipt/evidence、validation、assessment terminal 和 Result → Workspace → Run delivery组成同一 file-backed sequence；每个子用例在上述边界的 **CAS 前**及**CAS 后响应丢失**分别中断，新的 Orchestrator Biz 只从 Run owner aggregate 继续。未提交 transition 完全不可见；已提交 transition 维持原 selection/proposal/receipt/terminal identity，最终仅形成 2 个 receipt/evidence、唯一 assessment/Result/terminal 和已释放 workspace active run。只有 proposal CAS 前失败因没有 committed proposal 允许额外一次 provider request；不重放 durable operation。`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 还在同一 MCP observation → File patch 链中覆盖 resource-change 后重新获取的 File target snapshot 和 admissibility：CAS 前失败不泄漏，CAS 后调用方响应丢失时重启只加载 committed observation；相同 owner response 的重复提交不改变 revision 或 authority。 | 将 R3-1/R3-2 所有 crash point 扩展为完整 matrix：补 MCP owner-catalog stale、observation、admissibility、duplicate/stale result 与 CAS reload 的交叉组合，并覆盖其后完整 terminal delivery。禁止用 process-local cache、EventStream watermark 或 `/run resume` 恢复 authority。 | `TestRuntimeV2StateRepositoryRestartDoesNotReuseStaleMCPOwnerFailureTarget`、`TestRuntimeV2FileBackedGroundedTargetStateSequenceRestartsAtCommittedBoundaries`、`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 与后续 table-driven MCP/file-backed matrix；断言 dispatch ID、receipt/evidence ID、repair ledger、terminal identity 均 exactly-once。 |
| R4-1 / `landed baseline` | Result、Workspace、Run owner 的生产 EventHub adapters；Orchestrator 只发送 concrete typed commands。 | 已有生产链回归证明 `Result command → Workspace release command → Run terminal command`，并在重复投递时收敛到一个 rejected Result、释放的 workspace active run 与一个 failed Run。 | 保持命令边界；不得以测试便利改为跨 owner repository/service 调用。若补 delivery boundary，仍通过同一 typed EventHub 命令而非 direct fixture。 | `TestDeliverRuntimeV2TerminalUsesTypedOwnerEventContracts`；`make verify-boundary-contract`。 |
| R4-2 / `partial` | File、MCP/Shell、LLM、Orchestrator 的 concrete typed EventHub contracts。 | MCP 已以真实 owner Biz → typed receipt query/commit → RuntimeController CAS → File target snapshot EventHub 链路回归确认：失败 response 顶层显式 `failureTargets` 可在 role/revision 约束下成为 observation，且与 operation/validation entry target 分离；controller 只绑定 observation target，File owner 只为该 target 返回 snapshot；duplicate delivery 复用 receipt、不重发 MCP HTTP call。Shell 已删除未绑定 `RelatedFailureTargets` 输出；在没有 structured source 时，失败 validation receipt 的 failure target/observation 均为 nil。LLM C3-C5 sealed rendering 已具备单组件基线。 | 补全完整正向与负向 owner chain：`owner observation → File snapshot/admissibility → sealed C3-C5 request → state-change receipt → assessment delta → focus advance/terminal`。正向 path 只使用有结构化 source 的 MCP；Shell path 必须证明 nil observation 不会回退为 command target。LLM 只检查已提交 authority 的 rendering，不选择 target/action。 | focused cross-owner test：未 ground 时 provider zero-call；MCP target 与 validation entry 不同时仅 committed target 可进入 snapshot/candidate；role/revision mismatch、stale snapshot、duplicate receipt 均不改变 progress；assessment same-gap 不 advance。 |
| R4-3 / `open (live gate)` | deployed Runtime V2；Web/TUI/EventStream 仅为 read/projection consumer。 | 当前本地单元、CAS 和 terminal typed-chain 证明合同基础，不能替代部署事实。 | 用清洁 runtime data 创建全新 Run，逐项记录 persisted aggregate、API/read model、EventStream watermark、workspace active-run、provider call count 与实际 workspace evidence。验证 no-grounding 的 zero-call，grounded target-bound repair，assessment delta 才推进，以及 no-feasible path 的 governance/typed terminal。不得复用 `run-006`、修改默认模型、提高预算或让 projection 发送 Resume。 | VPC-8 live close gate 12 项；`make verify-runtime-v2-positive-convergence-file`、`make verify-boundary-contract`、affected-package race tests。全量 test/vet/release gate 仅在 fixture 不污染运行环境后执行。 |

**R3 transition invariant.** 同一条 Runtime V2 aggregate transition 只允许有两种可恢复状态：
“尚未 CAS 提交，重启后完全不可见”或“已 CAS 提交，重启后只可幂等消费”。任何第三种
“已执行但 aggregate 未记录、或 projection 反向补写 authority”的状态都是 defect。终态 delivery
是 SM-4 对既有 terminal 的投影，不是新的 SM-2 decision；因此它不可改变 focus、failure target、
repair ledger、candidate attempt 或 evidence revision。

**R4 owner-chain invariant.** EventHub command/result 必须保留 source owner、workspace/run/focus
scope、role、subject、revision 与 bounded refs。只有 Orchestrator 在 CAS 后才能把 observation
变为 authority；File/MCP/Shell/LLM、Result/Workspace/Run 和 projection 任一方都不得从另一方的
DTO、自由文本或 read model 反推 implementation target、next action 或 effective progress。

**提交与文档更新规则。** 每完成 R3-2、R3-3、R4-2 或 R4-3 中的一项，提交说明及本节对应
状态必须同时列出：影响的 SM ID、唯一状态 owner、具体 typed I/O、CAS commit point、
duplicate/stale/restart 结论、focused command 与仍未满足的 live 证据。仅增加测试而未覆盖实际
commit/delivery 边界，或仅让 TUI/Web 显示新状态，均不得把该项从 `open` 更新为 `landed`。

#### 22-component impact classification for VPC-8

VPC-8 不要求为 22 个组件同时新增运行逻辑。下表将修改范围与“只做边界回归”的范围明确
分离，避免以单点修复扩散 owner 权限。完整组件职责仍以
[22 组件合同矩阵](component-contract-and-regression-closure-matrix.md) 为准。

| 范围 | 组件 | 收口动作 |
| --- | --- | --- |
| 直接合同 / 行为修改 | `application/workorchd`、`kernel/orchestrator`、`blocks/file`、`blocks/llm`、`blocks/mcp`、`blocks/shell`；以及 `workorchruntimecontract`、software-dev defaults | 分别执行 O1--O8；只有 Orchestrator 变更 SM-2/SM-3，其他组件仅编译合同、观察事实、执行封存 operation 或渲染封存 authority。 |
| 相邻合同与恢复回归 | `kernel/run`、`blocks/workspace`、`blocks/result`、`kernel/eventstream`、`blocks/git`、Web/TUI projection | 验证 CAS/restart、terminal/result/workspace release、审计和展示能保留已裁决事实；不得添加 action、target 或 Resume side effect。 |
| 不在本切片修改 | `initiators/workorchruntime`、`kernel/governance`、`kernel/system`、`blocks/decision`、`blocks/approval`、`blocks/memory`、`blocks/skill`、`blocks/soul`、`blocks/mcpregistry`、`blocks/ssh` | 保持既有 typed EventHub、Base Biz、pkg 和 owner repository 边界；若测试暴露合同破坏，只修复该组件自身边界，不将 VPC-8 状态迁移迁入其中。 |

每一个直接修改组件必须同时满足其 22-component card 的独立命令和本表对应测试。每一个
“相邻合同与恢复回归”组件只验证已裁决事实的保存/投影，不可借 VPC-8 修改结果含义、repair
ledger、target precedence 或 Run lifecycle。

#### Component closure delivery board — current execution scope

下表是本轮代码收口的可执行范围清单。它是“哪些组件需要改、为什么改、改到哪里停止”的
唯一入口，供后续逐项提交和 live 验证使用。`open` 不表示可以在任意组件补偿：每一行的
**唯一 owner**才可写该行声明的事实；其余组件只能通过 concrete typed EventHub
command/result 消费它。未列为直接改造的 22-component card 保持边界回归，不因本问题新增
状态迁移。

| 顺序 / 状态 | 唯一 owner（组件） | 本轮必须收口的合同与功能职责 | 状态机归属与明确停止线 | 关闭证据 |
| --- | --- | --- | --- | --- |
| C1 / `partial` | `workorchdefaults` + `workorchruntimecontract` | profile/default 声明 target role、discovery source 与 opaque selector；contract 仅保存/校验/规范化 owner observation、failure relation、snapshot、directive 与纯集合 delta。 | 只提供 SM-2/SM-3 输入；`pkg` 禁止选择 target/action、修改 repair ledger、推进 focus 或访问 I/O。 | profile/default 与 contract 的 normalize/hash/clone/invalid-binding tests；语义 selector 的 role/revision drift 与 live evidence 仍待完成。 |
| C2 / `partial` | `application/workorchd/.../activationcore` | 将 profile 声明无损编译为 Focus/Bootstrap，并对缺 target 与缺 bounded discovery 的 direct-evidence focus fail-fast。 | 不变更 SM-1/SM-4；只构造 Start/Resume bootstrap，禁止补靶、选择 repair 或改写 sealed Run authority。 | exact/wildcard/neutral profile、selector round-trip、missing-grounding、definition revision、resume bootstrap tests。 |
| C3 / `partial` | `blocks/file/biz` | 以 profile-bound selector 对有界 workspace inventory 返回 `selected`/`absent`/`ambiguous`，并为 sealed ref 提供 target snapshot 与 freshness。 | 仅 owner observation/receipt；不得从 inventory 第一项、文件名或 LLM 建议猜 implementation target，不得裁决 progress。 | concrete EventHub selector、unique/absent/ambiguous、unknown selector、stale/reacquire、snapshot and restart tests；领域语义 selector 需由真正 owner 提供。 |
| C4 / `open` | 实际失败关联的 capability owner（当前为 `blocks/shell/biz`；MCP 已有受限基线） | 只在外部协议给出可验证的 owner、subject、role、revision 与结构化 related target 时生成 failure-target observation；不可观察时返回 nil。Shell sealed operation 已拒绝 candidate-claimed `relatedFailureTargets`；MCP 仅采纳失败 response 顶层 `failureTargets`。 | 仅 SM-3 receipt input；禁止从 command target、validation entry、自由文本、文件后缀、测试名或 `result.TargetRefs` 补造 observation，更不得关闭 coverage/retarget。 | Shell operation-contract negative + structured/missing/multiple/role/revision/duplicate tests；MCP explicit/no-field/success/mismatch tests；未具备结构化 source 时必须证明 nil observation。 |
| C5 / `open` | `kernel/orchestrator/biz/runtimev2` | 校验并 CAS 封存 owner selection/failure observation；以同一 sealed grounded target set 约束 prerequisite、candidate admission、directive、evidence、assessment gap delta、retarget 与 terminal。 | SM-2 为唯一 progress/action authority，SM-3 仅 sealed admission/dispatch，SM-4 仅重驱已提交事实；禁止 capability I/O、跨 owner repository、projection/Resume 重建 authority。 | pre-provider zero-call、role/target/freshness mismatch、non-target/no-op/same-gap、exact replay、retarget/governance/terminal、duplicate/stale、全 crash/restart state-sequence matrix。 |
| C6 / `partial` | `blocks/llm/biz` | 仅渲染 CAS 已封存的 Focus、selection、FailureTarget、snapshot、directive、admissibility 与 no-progress shapes 到 C3-C5；seal 前拒绝越界 target。 | 仅 SM-3 provider boundary；禁止发现/扩展 target、接受 candidate、修改 budget/ledger 或选择 action；C0-C2 不得引入 volatile grounding。 | authority context round-trip、uncommitted fact invisible、target/selector mismatch、schema rejection、stable-prefix tests。 |
| C7 / `open` | `kernel/run`（CAS/recovery fixture）与 Orchestrator lifecycle | Run 只持久化 aggregate/CAS/recovery inventory；`validation_scratch`、File target selection、target snapshot、candidate proposal、dispatch seal、owner-observed failure receipt 与 assessment/progress 已覆盖 CAS 失败不半提交和 restart 仅见 committed state。Orchestrator drive lifecycle 在 Stop 与后台 drive 并发时保持 owner dependency 有效，重启仅加载 committed aggregate。 | SM-1 不选 action；SM-4 不重建 SM-2 authority。禁止用 process-local cache、projection 或人工 Resume 恢复 stale target/directive/dispatch。 | target observation/dispatch/receipt/evidence/assessment/terminal 的 CAS commit 前后 matrix，lifecycle race、terminal delivery exactly-once。 |
| C8 / `regression + live` | `blocks/result`、`blocks/workspace`、`kernel/eventstream`、Web/TUI、`blocks/git` | 保存并展示已裁决的 failure/repair/terminal 事实，保证 Result/Run/Workspace/eventstream projection 与 Git closeout 不丢失。Result → Workspace release → Run terminal 的 production typed EventHub delivery 已有 duplicate convergence 回归。 | 只覆盖 SM-4 display/delivery 或前序 capability receipt；不得重判 evidence、修改 repair target、触发 Resume 或选择下一 action。 | typed projection/read-model tests、workspace active-run release、watermark 对齐、late/duplicate event、Git sequence，以及 clean-data deployed Run 的 VPC-8 live close gate。 |
| C9 / `boundary regression only` | `initiators/workorchruntime`、`kernel/governance`、`kernel/system`、`blocks/decision`、`blocks/approval`、`blocks/memory`、`blocks/skill`、`blocks/soul`、`blocks/mcpregistry`、`blocks/ssh` | 保持自身 EventHub/Base Biz/pkg/owner repository 边界；仅在本组件测试暴露既有合同违例时修复自身边界。 | 不承接 VPC-8 的 target precedence、effective progress、candidate admission 或 Runtime continuation 状态迁移。 | 对应 22-component card 的 focused regression 与 `make verify-boundary-contract`；没有新的 VPC-8 行为改造。 |

**C7 lifecycle local increment.** 已排队、但在 `Stop` 清理 owner authority 后才开始的
background drive 仍必须结束其 singleflight key；否则同一 workspace/run 会留下进程内假运行态。
`scheduleRuntimeV2Drive` 现在在 lifecycle readiness check 之前注册 key-release defer。并且
`TestScheduledRuntimeV2DriveReleasesSingleflightKeyWhenOwnerStopsBeforeTaskStarts` 与
`TestStopWaitsForRuntimeV2DriveLifecycleReadLockBeforeClearingDependencies` 分别验证晚启动任务
无副作用地退出、以及 Stop 在所有 drive read-lock 退出后才清空 owner dependency。该增量只补
SM-4 生命周期同步；C7 仍为 `open`，直到全 P1--P5 restart/terminal matrix 与 deployed gate 完成。

**执行依赖：** C1 → C2/C3 → C4 → C5 → C6/C7 → C8。C4 没有真实结构化观察时，
C5 的正确结果是保持 target 未 ground，并进入有界 prerequisite、governance 或 typed terminal；
它不是放宽 provider gate 或让 C6 用 prompt 猜测路径的授权。C9 不得作为 C1--C8 的替代
落点。每个提交都必须在本表对应行更新状态、链接 focused test，并写明 affected SM ID、CAS
事实、duplicate/stale/restart 处理和仍待部署验证的证据。

#### R4-2 code-closure handoff — ordered implementation docket

本清单把 R4-2 拆成可独立合入的最小提交。每个提交只可以修改本行的唯一 owner；前一行
未满足其验收条件时，后一行不得通过 prompt、fixture 直连或 projection 补偿前置事实。当前阶段
不增加旧 aggregate/receipt、V1 路径或历史运行数据的兼容分支。

| 提交顺序 | 唯一实现 owner | 要落地或强化的事实 | 绝对禁止 | 完成判据 |
| --- | --- | --- | --- | --- |
| H1 / `landed baseline` | `blocks/mcp/biz` | 已将**失败** MCP response 顶层的显式 `failureTargets` 以 concrete typed EventHub receipt 输出为 `RuntimeFailureTargetObservation`。sealed operation 的 `TargetRefs` 与 observation 的 `TargetRefs`、role、server subject、owner-catalog revision 分别保留。 | 从 request input、validation entry、tool 参数、自由文本或 `result.TargetRefs` 推断 failure target；success response 产生 observation。 | owner Biz → typed receipt query/commit 回归证明 observation target 与 operation target 不同仍完整保留；adapter 覆盖 no-field/success/invalid/bound/dedupe。 |
| H2 / `landed negative baseline` | `blocks/shell/biz` | 已删除未绑定 `RelatedFailureTargets` 输出。当前没有真实结构化 related-target 协议时，失败 validation receipt 只保留 command/validation entry lineage，failure target 与 observation 均为 nil；quality、zero-work 与 diagnostics 仍由 Shell owner 输出。未来接入协议必须一次性输出 owner、subject、role、revision 和 canonical targets。 | 以 command target、测试名、路径后缀、诊断文本或候选参数回填 implementation target。 | concrete owner receipt 覆盖失败 validation entry 不成为 FailureTarget；既有 adapter/owner tests 覆盖 all-skipped、zero-work、non-zero 与 duplicate。 |
| H3 / `landed receipt-binding baseline` | `kernel/orchestrator/biz/runtimev2` + `blocks/file/biz` | 已以真实 MCP owner EventHub receipt 覆盖：Controller claim 后将 Focus role、owner/subject/revision、FailureTarget CAS 绑定；operation target 保持 validation lineage，observation target 才成为 repair authority；实际 File owner 随后仅为该 observation target 返回 snapshot，并由 Controller CAS 保存；duplicate receipt 不重复执行 MCP。未 ground 时仍停在 prerequisite/governance/typed terminal，且 provider call 为零。 | 在 reducer、executor、application、LLM 或 projection 中补 target；从 read model 或进程内 cache 重建 binding。 | target/role/revision mismatch、absent/ambiguous、stale、duplicate receipt 均不能产生 candidate/progress；匹配 observation 必须先完成 File snapshot 后才可请求 candidate。完整 snapshot → C3-C5 → state-change → assessment/focus advance 仍属于 H4/H5。 |
| H4 / `landed local` | `blocks/llm/biz` | 真实 H3 aggregate 已经经 LLM Biz 的 concrete typed EventHub request 进入 candidate context：CAS 提交的 focus、MCP observation、FailureTarget、唯一 File snapshot、directive、admissibility 与 no-progress shapes 只渲染到 C3-C5；真实 target-bound File candidate 在 persisted owner feasibility 后才能 seal。C0-C2 继续稳定。 | LLM 扩展 target/role，或将模型文本提升为 observation；以渲染失败改变 action、repair 或 Run。 | committed-authority round-trip（`TestMCPOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot`）、uncommitted fact invisible、out-of-target candidate seal rejection、C0-C2/stablePrefixHash 不漂移。 |
| H5 / `partial` | `kernel/orchestrator/biz/runtimev2` + `kernel/run` fixture | 已形成真实 owner state sequence：MCP structured failure → File state change receipt → resource-change invalidation → manifest/admissibility（复用 `GroundedRepairTargets`）/snapshot reacquire → MCP accepted validation → sealed satisfied assessment → current focus complete。receipt/invalidation 与 assessment complete 后均重建 Controller 并只从 persisted repository load；`Changed=true` 与 accepted validation 均不会在 assessment 前清 progress。`make verify-runtime-v2-positive-convergence-file` 已通过。assessment 的 file-backed CAS/restart 子矩阵已覆盖 satisfied、same-gap 与 partial-gap：`TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic`、`TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow`、`TestRuntimeV2StateRepositoryPartialGapAssessmentCrashRestartPersistsSingleDelta` 均验证 CAS 前完全不可见、CAS 后 response-loss 只消费 committed aggregate，且重复 `CommitAssessment` 不会二次推进 focus/progress 或清 repair window。`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 已将真实 MCP HTTP owner、Run-owner file-backed CAS、File target snapshot 与三种 target-bound `patch/apply` durable 变体（`file_write`、`text_replace`、`unified_diff`）串为同一链：资源变更后，旧 snapshot/inventory 必须拒绝；fresh snapshot 和 inventory 的 CAS 前失败不会泄漏，Controller 重建后仅能提交同一 fresh owner result。MCP 不重复调用、File resource change 只失效相交 snapshot/admissibility，持久化 FailureTarget 不丢失。仍需把本 owner-grounded 链在每个 prerequisite、proposal/seal、receipt/evidence、terminal delivery 的 **CAS 前** 中断与 restart matrix 中逐项组合覆盖。 | 通过 `/run resume`、EventStream、TUI/Web 或 process-local map 恢复 authority；重发旧 durable operation 或重复扣 ledger。 | 每个断点重启只消费 committed aggregate；dispatch/receipt/evidence/repair/terminal identity exactly-once，且 terminal delivery 只重放既有 terminal。 |
| H6 / `pending deployed verification` | deployed Runtime V2；Web/TUI/EventStream 为消费者 | 使用清洁 runtime data 的新 Run 记录 grounded 与 ungrounded 两条事实链，并核对 API/read model、event watermark、workspace active-run、provider calls 与 workspace evidence。 | 复用 `run-006`、调整默认模型/预算让验证通过，或用 UI/投影触发 Resume 补偿。 | 通过 VPC-8 live close gate 的 12 项；未 ground 时 zero-call，grounded 后仅 target-bound repair 进入候选，assessment delta 才推进。 |

**H1--H6 shared invariants.** H1/H2 是 capability observation，不是 authority；H3 是唯一把
观察写入 Runtime aggregate 并选择下一状态的 owner；H4 是 provider adapter；H5/H6 只能验证
已定义的合同。所有跨 owner 调用必须是 concrete typed EventHub command/result。`pkg` 若因 H1--H5
需要新增字段，只可提供 DTO、validation、canonical normalize/hash/clone 或纯集合比较，不能拥有
transition、I/O、EventHub、retry、repair 或 terminal 逻辑。

#### H5-C local increment — invalidated prerequisite response fence

`RuntimePrerequisite.ID` 在 re-acquire 时保持其语义稳定，因而不能只凭 pending ID 接受 owner
响应。Orchestrator 现以 persisted `RuntimePrerequisiteInvalidationRecord` 的 owner freshness
dependency 作为提交栅栏：资源变更或 stale invalidation 后，仍携带被替换 revision 的旧 snapshot /
inventory result 必须以 conflict 拒绝，且不得恢复 facts、递增 revision 或消耗 recovery/repair ledger；
File owner 重新观察到新 revision 的同一 prerequisite 则可正常 CAS 提交。该规则属于 SM-2
prerequisite admission，未下沉到 `workorchruntimecontract` 或 capability owner。

`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 现覆盖真实 MCP failure-target
receipt → File durable patch resource-change invalidation → 旧 target snapshot **及旧 File
admissibility inventory** 结果拒绝 → 新 snapshot/inventory 的 CAS 前失败、CAS 后响应丢失重启与
重复响应不变性。File owner 对 inventory 中已经
授权且存在的 target 记录真实 content hash，不能再以 workspace-tree revision 冒充 target revision；
invalidation record 同时保存**实际与本次 change/stale 相交**的被失效 observation dependencies，
以便 Controller 即使 receipt 没有可用的 `BeforeRevision` 也能拒绝延迟响应，同时不错误拒绝同一
observation 内未变化的 owner-catalog dependency。`TestPrerequisiteStaleInvalidatesProposalAndReacquiresWithoutCandidateDebt`
同时覆盖 generic stale result 拒绝与 fresh owner result 重获。File 的 `file_write`、`text_replace`、
`unified_diff` 现均覆盖此 stale-generation 边界及 fresh snapshot/inventory CAS 前失败、CAS 后响应丢失后的
controller 重建；H5-C 仍为 `partial`，因为 P1--P5 的所有交叉断点尚未穷尽，H6 仍待 deployed verification。

File 资源变化也会使 Git repository-state prerequisite 失效。Git 不能把自己的
`ObservedRevision` 冒充 File 的 `workspace_tree` revision：它现在通过 concrete typed EventHub
向 File owner 读取 manifest revision，并将该值独立持久化为
`RuntimeCapabilityStateSnapshot.WorkspaceTreeRevision`。Git capability-state/admissibility observation
因而同时保存 Git repository 与 File workspace-tree 两个精确依赖；resource-change 后的新 Git
acquisition 不再被旧 tree revision 误拒绝。`TestDriveRuntimeV2FileBackedFreshFileShellGitAcceptance`
及 File-backed Git capability-state/restart matrix 覆盖该链路，`TestGitObservationsIncludeFileOwnedWorkspaceTreeDependency`
和 capability-bound admissibility owner test 覆盖 owner observation 的单组件合同。

File 的 manifest、target snapshot 与 target selection 三类 read-only prerequisite 现在均在
Orchestrator EventHub receive boundary 强制要求与主 snapshot/manifest/selection 精确绑定的
File freshness observation；缺失、内容 hash、target/tree revision 或 owner/kind 不匹配的结果
不得进入 aggregate。该规则保证 P1/P3 的后续 resource-change invalidation 始终有可验证的
依赖输入，而不由 Controller 根据路径或进程内状态补造。

**Recommended execution gates.** 每完成 H1--H4 先执行其组件 focused test；H3/H5 额外执行
`go test -race ./internal/modules/kernel/orchestrator/... -count=1`。H5 之后执行
`make verify-boundary-contract`、`make verify-runtime-v2-positive-convergence-file`、
`go test -p 1 ./... -count=1`、`go vet ./...` 与 `git diff --check`。H6 是唯一可将 R4-3 标记为
完成的依据；本地 green 不替代部署事实。

#### H4--H6 implementation contract — next code-closure batches

本节把剩余工作固定为可逐项提交的实施合同。它以 H3 已经 **CAS 提交** 的 aggregate 为
唯一输入；测试可使用真实 owner Biz、typed EventHub 和 file-backed Run fixture 组装链路，但不得
通过测试 helper 直接写入跨 owner 正式状态。当前阶段不做旧 aggregate、receipt、HTTP payload 或
V1 路径兼容。

| 批次 | 唯一可写组件 | 前置 committed facts | 唯一允许的输出 / 状态迁移 | 不可由本批次完成的职责 |
| --- | --- | --- | --- | --- |
| H4-A context projection | `blocks/llm/biz` | 同一 workspace/run/focus 的 authority transaction、`FailureTarget`（含 owner/subject/role/revision）、对应 File `TargetSnapshot`、`RepairDirective`、admissibility 与 no-progress shapes。 | 通过 `RequestRuntimeCandidateCommand/Result` 渲染 provider-neutral candidate context；上述易变事实只能位于 C3--C5，C0--C2 的 section/hash 不变。 | 发现/选择 target、写 aggregate、修改 candidate attempt、接受 candidate、变更 repair ledger 或调用 capability owner。 |
| H4-B seal boundary | `kernel/orchestrator/biz` candidate adapter / `runtimev2` admission | H4-A 的 typed provider result 与同一 persisted authority transaction。 | 仅在 candidate 的 durable targets、selector、effect scope、coverage 与已封存 snapshot/admissibility 一致时 CAS seal dispatch；拒绝结果只进入 attempt/diagnostic 既有合同。 | 依据模型文本补 observation，或因渲染/解码失败切换 SM-2 action。 |
| H5-A state-change receipt | `kernel/orchestrator/biz/runtimev2`（编排）与 File owner（执行） | 已 seal 的 target-bound dispatch，且每个 operation 取得 claim fence。 | File owner 通过 concrete typed receipt 报告真实 effect/resource change；Orchestrator 在同一 CAS 提交 evidence、失效相交 prerequisite，并保留 failure target/snapshot/directive 的 committed lineage。 | File owner 判断 coverage、gap delta、focus advance 或 next action；Orchestrator 直接访问 File repository。 |
| H5-B assessment and focus convergence | `kernel/orchestrator/biz/runtimev2` | 当前 focus 的 accepted evidence、current-run coverage、assessment candidate/receipt、persisted target set 与 repair ledger。 | 只有 assessment 证明 required coverage/gap 的中立集合严格缩小才记录 gap progress；satisfied assessment 原子 advance focus，不能缩小则保留同 focus ledger 并进入已定义的 repair/retarget/governance/terminal reducer 分支。 | LLM、File、MCP、Shell、Run、Web/TUI 自行关闭 coverage、清预算或推进 focus。 |
| H5-C restart/CAS matrix | `kernel/run` fixture + `kernel/orchestrator/biz/runtimev2` | 每个 prerequisite、proposal/seal、claim/owner receipt、evidence、assessment、terminal delivery 的 committed 或未提交边界。 | 重启只 load Run owner 已提交 aggregate；重复 delivery 复用 execution key/fence/receipt/terminal，不重复调用 owner、不重复扣 recovery/repair ledger。 | 通过 Resume、EventStream 或 process-local cache 重新选择 authority/target。 |
| H6 live consumer check | Web/TUI/EventStream、Result、Workspace 仅为消费者；Orchestrator drive 为生产写入者。 | H5 全部 local matrix 已通过，新的 clean-data Run 与最新二进制。 | 展示同一 Run 的 persisted progress/terminal、watermark 与 workspace active-run release；观察 grounded 与 ungrounded 两条链。 | UI 发起补偿性 Resume、投影改写 aggregate、修改模型或预算伪造收敛。 |

**H4 acceptance sequence.** 用 H3 的真实 MCP→Orchestrator CAS→File snapshot aggregate 触发 LLM
owner。捕获的 `RuntimeCandidateRequest` 必须同时包含 `implementation.go` 的
`FailureTarget`、MCP observation 的 owner/subject/role/revision、且只包含该 ref 的 target
snapshot；验证 entry 只能留在 operation lineage。随后以 matching target-bound candidate 走既有
admission/seal，以越界 target candidate 证明 seal-before-execute 拒绝。对同一 C0--C2 输入改变
observation/snapshot 等易变事实时，stable prefix 必须不漂移；未 CAS 提交的 observation 不得可见。

**H5 state-sequence.** 最小正向链必须是：

```text
MCP structured failure observation (H1)
  -> Orchestrator CAS FailureTarget/directive (H3)
  -> File snapshot prerequisite committed (H3)
  -> LLM C3-C5 request and target-bound candidate sealed (H4)
  -> File durable receipt/evidence committed (H5-A)
  -> assessment proves strict gap delta (H5-B)
  -> same-focus repair or atomic focus advance/terminal (H5-B)
  -> Run/Result/Workspace/EventStream projection replay only (H5-C/H6)
```

每一箭头都应各自覆盖「commit 前崩溃、commit 后响应丢失、duplicate、stale」；只有当前
focus 的 accepted evidence 可参与 assessment。target-bound durable operation 即使 `Changed=true`
也不能在 assessment 前清 repair window；same-gap、no-op、non-target 或弱 validation 均不得算
progress。所有测试 fixture 保持领域中立，`implementation.go` 仅是边界测试 ref，不是核心规则。

#### H5-C CAS/restart execution register — remaining local closure

H5 的正向 owner chain 已有本地证据，但它**不等价于**完整的恢复正确性。以下矩阵是下一批代码
收口的唯一断点清单。每一行都必须使用真实 Run owner 的 file-backed/CAS persistence fixture；禁止
使用浅复制的内存 aggregate fixture 声称原子性，因为 slice/pointer alias 可能在 `Save` 前污染
底层值。测试只可通过 concrete typed EventHub 调用 MCP、File、LLM、Result、Workspace 与 Run owner。

| 断点 / 状态 | 唯一写入 owner | 需要注入的中断位置 | 重启后的唯一允许行为 | 必须断言不发生 | 最小独立回归 |
| --- | --- | --- | --- | --- | --- |
| P1 `prerequisite` | Orchestrator aggregate，经 Run owner CAS | owner observation、File manifest/admissibility/snapshot 的 CAS **前**与成功提交后响应丢失。 | 仅加载已提交 prerequisite；未提交则重发同一 typed acquisition，已提交则复用 snapshot/receipt。 | 不把未提交 `FailureTarget`、selector、snapshot 或 acquisition generation 暴露给 candidate；不重复扣 freshness ledger。 | **landed local**：generic prerequisite、File target selection/snapshot、manifest/operation-admissibility 以及 MCP owner-observation stale invalidation 均以 file-backed Run owner 覆盖 CAS 前或 CAS 后 response-loss、restart 与 duplicate；stale 后旧 FailureTarget/directive/candidate 不会复活，新的 owner-catalog revision 只能经 fresh acquisition CAS 提交且重启仅保留该 revision。 |
| P2 `proposal/seal` | Orchestrator aggregate，经 Run owner CAS | candidate proposal 持久化前、proposal 后 seal 前、seal CAS 前、seal 成功后调用方未收到响应。 | 仅从已提交 proposal/sealed dispatch 恢复；已 seal 的 dispatch 使用原 ID、candidate hash、execution key 与 authority transaction。 | 不再请求 provider、不生成第二个 dispatch、不中断 C0--C2 stable prefix，也不增加 candidate/repair attempt。 | **landed**：`TestRuntimeV2StateRepositoryCandidateProposalAndDispatchSealCrashRestartAreAtomic` 使用 file-backed Run owner 覆盖 proposal/seal 的 CAS 前失败与 CAS 后 response-loss；recovery 仅消费已提交 proposal/dispatch，重复 seal 返回同一 dispatch/revision。`TestRuntimeV2MainlineRestartsFromPersistedCandidateProposalWithoutProviderReplay` 继续断言 persisted proposal seal 不回放 provider。 |
| P3 `claim/owner receipt` | capability owner receipt + Orchestrator evidence transition | claim 后 owner side effect 已完成但 Orchestrator receipt/evidence CAS 未提交；receipt 已提交而上层响应丢失。 | 用原 execution key/fence 查询或重放同一 owner receipt；再提交一次且仅一次 evidence。 | 不重做 MCP/File durable side effect、不生成新 fence、不重复 invalidation 或 prerequisite-receipt refresh。 | **partial local**：`TestRuntimeV2FileBackedMCPOwnerFailureTargetReceiptRestartMatrix` 覆盖真实 MCP HTTP failure-target receipt 的 CAS 前与 receipt 已提交/evidence 前边界，并在重建后以原 dispatch/key/fence 重投 MCP operation：owner 只返回已提交 receipt，不重发 HTTP、不重建 FailureTarget、不增加 aggregate revision；随后在同一已持久化 observation/snapshot authority 上覆盖 File `patch/apply` 的 `file_write`、`text_replace`、`unified_diff`：receipt-CAS 前中断与 replay-proof restart 后均保持原 execution key/fence，resource change 后旧 File snapshot/admissibility 被拒绝，fresh snapshot/inventory 的 CAS 失败不半提交且进程重建后仅提交同一 fresh result。`TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure` 还在重建后的同一链重新投递 owner operation，断言 File 只返回原 receipt，workspace、receipt/evidence 和 Run revision 均不二次变化；并继续驱动至 assessment/terminal，断言只保留初始 state-change provider request（另一次仅为 assessment），原 receipt/evidence key/fence/hash 不变。所有三种 patch variant 还覆盖 receipt-loss 后外部 target mutation：replay proof 只可确认封存 after-hash，不能重写外部内容，且统一返回 typed conflict 而非 variant-specific invalid-input。MCP call、File effect receipt/evidence 均 exactly-once，FailureTarget 保持。仍需把这些边界与完整 stale/terminal-delivery 的交叉组合覆盖。 |
| P4 `evidence/assessment` | Orchestrator aggregate，经 Run owner CAS | evidence admission、assessment gap delta、focus advance 各自 CAS 前与 CAS 后响应丢失。 | 未提交状态完全不可见；已提交 assessment 只消费其 focus/coverage/progress 结果。 | `Changed=true` 或 accepted validation 在 assessment 前清 repair window；重复关闭 coverage、重复 `NoteGapProgress` 或重开已完成 focus。 | **landed local**：`TestRuntimeV2StateRepositoryAssessmentCommitCrashRestartIsAtomic` 覆盖 satisfied assessment；`TestRuntimeV2StateRepositorySameGapAssessmentCrashRestartRetainsRepairWindow` 覆盖 same-gap；`TestRuntimeV2StateRepositoryPartialGapAssessmentCrashRestartPersistsSingleDelta` 覆盖 partial-gap。三者均使用 file-backed Run owner，验证 CAS 前失败、CAS 后 response-loss、进程重建与 duplicate commit rejection；partial-gap 只持久化一次 coverage/gap delta，未满足项保持 unsatisfied。 |
| P5 `terminal delivery` | Orchestrator delivery lane；Result → Workspace → Run owners | terminal authority 已提交后，Result 写入前后、Workspace release 前后、Run terminal 前后中断。 | 只重投同一 persisted terminal；按 Result → Workspace release → Run terminal 收敛。 | 重新 reducer、重新执行 operation、修改 failure target/repair ledger/candidate attempt，或生成第二个 Result。 | **landed**：file-backed `TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult`、`TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection` 与 `TestDeliverRuntimeV2TerminalRestartsAfterOwnerProjectionAcknowledgementLoss` 分别覆盖 Result 写入前、写入后、Workspace release 后和 Run terminal 后的 process reconstruction；另有 typed owner contract、duplicate convergence 与 startup recovery inventory/budget 回归。 |

**矩阵共同断言。** 每个 P1--P5 组合至少验证一次 duplicate result、stale dependency、CAS reload
与进程重建。重启后的 aggregate 必须只包含最后一次成功 CAS 的 facts；`DispatchID`、execution
key/fence、receipt/evidence ID、repair/freshness ledger 与 terminal identity 都必须保持 exactly-once。
“commit 后响应丢失”只能验证幂等消费，不能再次选择 action；“commit 前失败”只能验证该 transition
完全不可见。任何需要 TUI/Web `/run resume`、EventStream 投影或 process-local cache 才能恢复的
测试，都不能计入 H5-C 完成证据。

**P3 owner-side groundwork — not a completed runtime recovery.** `blocks/file` 已对带有 owner
feasibility 已验证 `baseHash` 的 `patch/apply:file_write` 补入 postcondition replay confirmation：
首次 durable write 已成功、但 receipt CAS 未提交时，若 owner 被以原 key/fence 再次调用，它只在
当前内容精确等于封存 content 且当前 hash 不同于封存 input revision 时生成同一逻辑 resource
delta，不再次写入，也不降为 no-op。feasibility 会拒绝不匹配的 supplied baseHash，避免 provider
伪造 prior revision。该实现和
`TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss`、
`TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure`、真实 H4/H5 candidate
path 一并回归。

当前 P3 已补入通用 typed `RuntimeOperationReplayProof`：File owner 在无副作用 feasibility 中以
实际 preview 产生 owner、operation、target、input/output revision、admissibility snapshot/binding；
Orchestrator 将其 CAS 持久化到 feasibility，再封存到 execution record 和 typed execution envelope。
只有带有该 proof 的 `at_most_once` claimed operation 才会消耗既有 recovery budget，并以原
execution key/fence 重新进入 owner；owner 先确认 exact postcondition，绝不重写 workspace。无
proof 的 durable claim 仍转为 `outcome_unknown`。`file_write`、`text_replace`、`unified_diff` 都使用
同一 proof 合同，不能仅把 kind 级 idempotency 放宽为 safe/owner-managed。

**P3 已 landed。** `TestRuntimeV2FileBackedPatchReplayProofRestartsAfterOwnerReceiptCASFailure`
以真实 file-backed Run owner 覆盖 `file_write`、`text_replace`、`unified_diff` 的“side effect
完成、receipt/evidence CAS 前失败 → 进程重建”边界；每个变体均断言原 execution key/fence、sealed
proof、receipt delta、recovery budget 与 evidence exactly-once，且 receipt 恢复后的资源失效只会进入
新 prerequisite。`TestRuntimeV2FileBackedPatchVariantRestartAfterOwnerReceipt` 覆盖同三变体
“receipt 已提交、上层响应丢失”的重启消费，复用既有 receipt 而不重新请求 candidate 或执行第二次
durable write。`TestRuntimeAdapterPatchApplyReplayProofCoversAllDurableVariants` 则保证 preview proof
和 File execution 对 byte-sensitive content、old/new text 与 unified diff 使用同一未截断 payload。

**Batch-level independent regression.**

1. H4：`go test ./internal/modules/blocks/llm/biz -run 'TestRuntimeV2CandidateContext' -count=1`，以及
   对真实 H3 aggregate 的 EventHub context round-trip integration test；
2. H5-A/B：`go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1`、
   `go test ./internal/modules/blocks/file/biz ./internal/modules/blocks/mcp/biz ./internal/modules/blocks/shell/biz -count=1`；
3. H5-C：`go test -race ./internal/modules/kernel/orchestrator/... -count=1`，并以 file-backed Run
   fixture 覆盖完整 crash/restart matrix；
4. H5 收口后：`make verify-boundary-contract`、
   `make verify-runtime-v2-positive-convergence-file`、`go test -p 1 ./... -count=1`、`go vet ./...`、
   `git diff --check`；
5. 只有上述本地证据全部通过，才可进入 H6 clean-data deployed gate。H6 的每项观察必须来自
   API/read model、provider audit 和 workspace evidence，不能以 TUI 文案作为 authority 证据。

### VPC-8 executable closure checklist — current code baseline

本节是 VPC-8 后续逐项代码收口的唯一执行清单。它以当前已经存在的
`target_selection`、failure-target receipt 和 LLM C3 投影为起点，明确哪些行为已经可以依赖、
哪些行为仍不得宣称完成。除非某项的合同、持久化行为和独立回归同时满足，否则该项保持
`open`；不以 prompt 调整、单个 live Run 的偶然结果或提高预算替代收口。

| ID | 状态 | 唯一 owner / 修改边界 | 输入与输出的 typed 合同 | 必须持久化的事实和状态机要求 | 独立回归与完成判据 |
| --- | --- | --- | --- | --- | --- |
| 8D-01 | landed baseline | File block 只观察有界 inventory；Orchestrator 只消费其 observation。 | `AcquireRuntimeTargetSelectionCommand/Result` 返回 `RuntimeTargetSelection`：`selected`、`absent` 或 `ambiguous`，包括 owner、role、revision、真实 `CandidateCount` 和有界候选 refs。 | SM-2 将 selection 与 workspace-tree freshness observation CAS 写入 facts；只有 `selected` 才可继续 target snapshot prerequisite。`ambiguous` 绝不可因 projection 截断而变成 `selected`。 | File 两目标且 `MaxCandidateContextItems=1` 仍返回 `ambiguous`；unique/absent、stale、restart、duplicate result 都不重复排队或扣预算。 |
| 8D-02 | landed MCP binding / open Shell and profile selector | Profile-bound selector 或实际 failing capability owner 负责给出 implementation-related target；不得由 File inventory 或 Orchestrator 猜测。 | MCP observation 已通过 concrete EventHub command/result 绑定 target role、canonical owner、subject、target refs 与同源 owner-catalog revision；Shell/MCP 只可报告自己观察到的 related failure target。 | SM-2 拒绝把 validation command entry 单独提升为 `FailureTarget`；MCP observation 必须匹配当前 Focus 与 persisted owner-catalog freshness，否则进入 prerequisite 而非请求 repair candidate。缺失、歧义或不匹配时不形成 repair authority。 | `TestRuntimeV2FileBackedAmbiguousTargetSelectionFailsWithoutProvider` 以真实 File/Run/EventHub 链验证双文件 `ambiguous` observation 直接进入 `target_grounding_unresolved` terminal、零 provider call、零 snapshot/dispatch/evidence；另有 MCP compatible/mismatched role/target、owner-catalog stale、snapshot gate 与 restart continuation。profile selector、Shell structured protocol、完整 commit crash/restart matrix 仍待完成。 |
| 8D-03 | landed baseline | Orchestrator `runtimev2` 是唯一 repair target precedence owner。 | `groundedRepairTargets(facts)` 的稳定优先级为 Focus refs、persisted FailureTarget、当前 failure refs、selected TargetSelection、RepairDirective refs。 | SM-2 的 required output、candidate admission、RepairDirective 和 LLM authority 都使用同一集合；候选 durable operation 不命中该集合不得 seal。 | 每个来源单独、来源冲突、selected target outside operation、stale target、duplicate candidate；不得存在第二份 fallback 逻辑。 |
| 8D-04 | landed baseline | Orchestrator 负责 effective-progress 判定；`workorchruntimecontract` 仅提供纯 delta/hash/clone helper。 | owner receipt、admitted evidence、coverage、assessment gap、focus、target set 和 repair ledger 都是输入；输出只能是 aggregate 的 CAS transition。 | SM-2：`RequireAssessment=true` 时，durable state change 只能使旧 assessment 失效，不能立即 `NoteProgress` 或清 repair window；后续 assessment 仅在 coverage/missing-evidence 的 neutral set **严格缩小**时调用纯 `NoteGapProgress`，保留剩余 `ActiveGap` 与同 focus 总预算；satisfied assessment 才可清 gap。无 assessment 要求时，只有 `Changed=true` 且命中 grounded target 的 durable change 才可能构成 progress。 | required target changed but assessment pending、same gap、partial gap delta、satisfied gap、non-target change、no-op、weak validation、restart 都有 focused 回归；duplicate/stale 与同签名 retarget 继续由 8D-05 收口。 |
| 8D-05 | landed baseline / open retarget | Orchestrator 负责 no-progress family、same-signature retarget 和预算；candidate/LLM/owner 都不得维护该状态。 | canonical operation shape 绑定 focus、coverage、kind/variant、normalized targets、effect scope、quality 与 resulting gap；reason/description 不得成为 identity。 | SM-2/SM-3：repairable assessment 重现或扩大 persisted gap 时，将 current-focus accepted state-change/validation operation 写入 `NoProgressOperationShapes`；candidate admission 拒绝 exact replay。真实 gap/coverage/focus/grounded-target delta 才能开启新 cycle。总 `RepairAttempts` 只在 focus advance 时重置。 | exact replay、文案变化、assessment same-gap shape persistence 已有 focused 回归；仍需完成 profile/capability owner 的 same-signature **retarget** observation、完整 crash boundary 与 fresh deployed sequence。达到上限后只可 state change、governance 或 typed terminal。 |
| 8E-01 | landed baseline | Shell 和 MCP block 仅形成 owner receipt。 | MCP 以 `RuntimeFailureTargetObservation` 传输失败 response 中的 actual failure targets；Shell 在没有结构化 source 时只传 operation `TargetRefs` lineage，failure target/observation 为 nil。两者均绝不回退 operation target。 | SM-3 仅把 receipt 作为 evidence-admission input；owner 不关闭 coverage、不修改 repair ledger、不选择 retarget/action。 | MCP target 与 operation target 相同/不同、多个/无可解析 target；Shell zero-work/all-skipped/non-zero/duplicate 与 validation entry 不升级为 failure target。 |
| 8E-02 | landed MCP observation baseline / open Shell and owner binding | 各 failing capability adapter 必须报告其实际可观察的 related implementation target 或明确不可得；不得伪造。MCP 只解析失败 response 的顶层显式 `failureTargets`；Shell 不再从 sealed request 或 command target 复制该字段。 | diagnostic target、failure signature、owner revision、subject 与 bounded evidence refs 由 capability owner 返回。缺结构化观察时 `FailureTargets=nil`。 | SM-2 只将与 Focus target role 相容的 receipt target 转成 `FailureTarget`；validation entry 本身不自动升级为 implementation target。当前 Shell 的 operation/diagnostic target 仍只是 receipt lineage，不能代替 future structured capability observation。 | MCP explicit/no-target/success-response/duplicate，Shell no-fallback、多个 target、stale owner revision、receipt replay；仍需 Shell structured protocol、role/freshness binding、crash/restart matrix。core 内不得新增文件名、后缀、Go/LSP/MCP 语义。 |
| 8F-01 | landed baseline | LLM block 只渲染 sealed authority；不拥有 Runtime facts。 | C3-C5 dynamic context 已含 Focus target contract、FailureTarget、TargetSnapshots、TargetSelection、RepairDirective、failure 和 rejected/no-progress shapes。 | SM-3 request 只能使用 Orchestrator 持久化的 authority snapshot；C0-C2 stable prefix 不包含这些易变事实。 | context round-trip、stable prefix 不漂移、selected/absent/ambiguous rendering、schema unknown field 拒绝。 |
| 8F-02 | landed FailureTarget snapshot gate / open owner-observation binding | Orchestrator 在 provider 前执行 target/admissibility/snapshot gate；LLM 不得扩展 target。 | candidate 的 target/selector/effect scope 必须匹配 sealed `RequiredOutputContract`、fresh owner snapshot 和 admissibility inventory。 | Focus refs、selected target **及已持久化 `FailureTarget`** 均必须取得 File owner 的当前 snapshot；缺 snapshot 时只能 `AcquirePrerequisite`，不得产生 provider request。越界 candidate 在 admission/seal 前拒绝且不形成 dispatch。 | persisted failure target → File snapshot prerequisite 的 focused 回归已具备；仍需 actual owner observation 的 role/revision/freshness binding、zero provider call、target mismatch、selector mismatch、stale snapshot、candidate retry/restart；自由文本“建议修改某文件”永远不是 observation。 |
| 8G-01 | open | Run/Result/File/Eventstream/Web/TUI 仅作 CAS owner、receipt owner 或 projection consumer；Orchestrator Biz 负责自身 drive lifecycle synchronization。 | 只消费 typed aggregate/event/read-model DTO；不添加 cross-owner repository/service access。 | SM-4 restart 复用已提交 target/failure/selection/directive，且 terminal/result/workspace projection exactly-once；后台 drive 在 authority、projection、recovery 分支持有 lifecycle read lock，`Stop` 必须先等待该分支退出再清 owner dependency；projection 不重建 authority 或发送 Resume。 | lifecycle race regression、state-sequence：selection → snapshot → candidate → receipt → assessment delta → focus advance；每个 prerequisite、receipt、evidence、progress、terminal crash boundary 和 duplicate/stale；fresh clean-data deployed Run。 |

#### Mandatory transition rules for 8D-04 and 8D-05

以下规则防止把“操作已执行”误记成“当前 gap 已缩小”，是本次有效进展收口的不可拆分部分：

```text
accepted owner receipt
  -> evidence admission (SM-3)
  -> if target/effect/quality is insufficient: retain no-progress lineage
  -> if Focus.RequireAssessment: invalidate prior assessment and reduce to assessment
  -> accepted assessment proves coverage/gap delta: NoteProgress once (SM-2)
  -> otherwise: retain ActiveGap and same-focus repair/no-progress window
```

1. `RequireAssessment` 是 Focus 合同，不是 UI 或 provider 提示。只要该标志为真，任何
   state-change receipt 都不能直接清 `ValidationOnlyRetries`、`SignatureCounts`、`ActiveGap` 或
   `LastProgressRevision`。
2. validation receipt 即使 accepted，也只能补足所需 validation evidence 或触发 assessment；
   除非 assessment 提供新的持久化 gap/coverage delta，不能视为 effective progress。
3. assessment 若重现同一 gap，必须保留 active gap 和 no-progress lineage；若缩小 gap 或关闭
   coverage，才可在同一 CAS transition 内记一次 progress。
4. duplicate、idempotent replay、stale authority/snapshot 或旧 dispatch 的回执，均只能进入
   diagnostic lineage，不能改变 evidence、repair、progress、metrics 或 candidate budget。
5. `pkg` 的 `NoteProgress` / hash / normalize helper 必须继续是输入到输出的纯值函数；
   “何时调用”只在 Orchestrator Biz 的 reducer/commit 主链决定。

#### Per-component closure boundary

| 组件 | 可以单独回归的功能边界 | 不可承接的状态迁移 |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | DTO validation、canonical normalize/hash、clone isolation、stable snapshot ID、pure set/delta comparison。 | action selection、repair counter mutation decision、focus advance、acquisition、dispatch、terminal。 |
| `blocks/file/biz` | inventory/selector observation、target snapshot、workspace-tree freshness、receipt idempotency。 | implementation target 的业务猜测、candidate admission、repair progress。 |
| `blocks/shell/biz` / `blocks/mcp/biz` | sealed operation execution、quality/diagnostic/failure-target owner observation。 | coverage/result acceptance、retarget、ledger 变更、next action。 |
| `blocks/llm/biz` | sealed context rendering、schema transport/decoding、provider capability boundary、C0-C2 cache stability。 | target discovery、authority rebuild、candidate acceptance、retry/repair transition。 |
| `workorchd` activationcore | profile → Focus/Bootstrap 的无损 declarative compilation 与 fail-fast contract validation。 | 路径选择、repair action、已封存 Run 的 authority 修改。 |
| `kernel/orchestrator/biz/runtimev2` | prerequisite binding、evidence/admission、progress/gap delta、repair/reducer、sealed dispatch/restart recovery。 | capability I/O 实现、Run repository direct access、projection-driven authority rebuild。 |
| `kernel/run` / `blocks/result` / eventstream / Web / TUI | aggregate CAS、terminal/result write、read model/event display。 | next action、evidence quality re-judgement、repair target/progress selection、Resume side effect。 |

#### Exit evidence for this checklist

每个 `open` 项完成时，提交必须更新本节状态并链接对应 focused test。VPC-8 只有在以下证据
齐备后才可转为 landed：

1. 代码审查可从 typed EventHub command/result 追踪 target/failure observation 的 owner；
2. `pkg` 扫描确认没有 aggregate 状态迁移或 EventHub/I/O；
3. 每个 owner 的 focused test 独立通过，Orchestrator state-sequence 与 restart matrix 通过；
4. `make verify-boundary-contract`、全量 test/vet 和 runtime verifier 通过；
5. 最新二进制在清洁 runtime data 的新 Run 上复核：无 target 时不请求 provider；有 owner
   grounded target 时 only target-bound repair 进入 candidate；assessment delta 才推进；无法选靶
   时 governed/terminal，且不因 test-only patch 耗尽总 repair budget。

## Scope and Non-Goals

### In scope

- `internal/pkg/workorchdefaults/defaults/souls/software-dev`：声明中立 target/evidence role、
  bounded discovery 与 direct-evidence profile 规则；
- `internal/modules/application/workorchd/biz/appservice/activationcore`：把 profile 声明无损
  编译为 typed Focus、Requirement 与 RuntimeBootstrap；
- `internal/modules/blocks/shell/biz`：validation observation 与 receipt quality；
- `internal/modules/kernel/orchestrator/biz/runtimev2`：evidence admission、effective progress、
  same-focus repair ledger 调用、candidate no-progress admission、failure/repair target grounding、
  discovery prerequisite 与 bounded retarget；
- `internal/modules/blocks/llm/biz`：无损渲染 sealed repair authority，保持 provider boundary
  与 stable prefix；
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
| SM-2 authority/focus convergence | **不符合，主收口面** | effective progress、failure/repair target grounding、repair ledger、bounded retarget 与 escalation | 不执行 capability operation，不把 action 选择下沉 owner/pkg |
| SM-3 candidate/sealed-dispatch | **部分符合** | target-bound candidate context/admission、weak/no-progress evidence、stable structural signatures | 不在 executor 选择 next action，不破坏 sealed transaction/fence/exactly-once |
| SM-4 drive/recovery/projection | 核心符合，补 grounding recovery | committed target/failure snapshot restart reuse、async terminal/progress metrics exactly-once | 不从 projection 重建 authority，不由 metrics observer 触发 Resume |

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
| VPC-7 | `orchestrator/biz/runtime_v2_candidate_continuation.go` | LLM typed boundary/provider raw-text mode、activation-sealed contract、artifact repository tests | `pkg` 状态迁移、Run/Result/LLM action authority |
| VPC-8A/C | software-dev defaults、`workorchruntimecontract` DTO | profile/default tests、纯 normalize/hash/validate | Runtime core 的 software-dev 路径/工具硬编码、pkg 状态迁移 |
| VPC-8B | `workorchd/.../activationcore/runtime_v2_focus_compiler.go`、`runtime_v2_bootstrap.go` | activation focused tests | application 选择 repair action |
| VPC-8D | `orchestrator/biz/runtimev2` prerequisite、directive、progress、admission | typed owner events、restart/state-sequence tests | capability owner/pkg/projection 选择 next action |
| VPC-8E | `blocks/shell/biz` observation/receipt | Shell parser与owner tests | Shell 关闭 coverage、retarget、修改 ledger |
| VPC-8F/G | `blocks/llm/biz/runtimev2` boundary；projection/regression | File/Run/Result/Eventstream/Web/TUI/Git tests | LLM/projection 扩展 authority 或驱动迁移 |

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
- VPC-7 candidate output continuation 与 VPC-8A～VPC-8G grounded repair target 全部 landed；
- Shell、Orchestrator、workorchd 均可独立回归；
- software-dev defaults、activationcore、workorchruntimecontract、LLM boundary 均可独立回归；
- SM-2 transition、SM-3 transaction、SM-4 restart/metrics matrix 全部通过；
- boundary contract、full test/vet、positive file verifier、release check 通过；
- fresh deployed gate 12 项全部有可复核事实；
- canonical workspace 不再复现 all-skipped → valid → NoteProgress → same gap 循环；
- canonical workspace 不再复现 `Focus.TargetRefs=nil` → test-only patch → same gap →
  `no_progress_exhausted` 循环；
- active overview、current-state、unclosed matrix、todo index 与验证记录同步；
- completion record 迁入 `90-archive/completed/`，active 不保留假完成状态。

若只实现弱 evidence 拒绝，但没有 effective progress、validation cap、restart 和 deployed
验证，不得关闭；若只让 Run 更早 failed，也不得称为正向收敛完成。
