# Runtime V2 Candidate Admission Transaction Closure

Status: active

Type: remediation plan

Last Updated: 2026-07-20

## Implementation Checkpoint

2026-07-20 current worktree has completed CA-1 through CA-7 and the
deterministic portion of CA-8:

- production admission has only typed `AdmitCandidateProposal` and
  `AdmitAndSealCandidateAttempt` transitions; the former direct proposal/accept
  entry points no longer exist in production code;
- full and partial replay both persist `pending -> failed` in the same CAS;
  attempt 1 exhaustion re-reduces in that CAS, while duplicate delivery returns
  the persisted typed result without an additional write;
- a file-backed Run restart after the partial-replay rejection CAS is covered:
  it starts exactly one attempt 1 with the same transaction/stable prefix and
  seals the corrected candidate;
- current schema rejects historical terminal-plus-pending and unreduced
  attempt-1-failed aggregates. There is intentionally no compatibility,
  migration, or automatic repair path;
- a subsequent fresh deployed `run-001` exposed a coordinated SM-2/SM-3/SM-4
  ordering gap: both provider responses reached the sealed 4096-token output
  ceiling and ended with `unexpected EOF`; attempt 1 was correctly persisted as
  failed, but prerequisite freshness reacquisition won before the determined
  `candidate_retry_exhausted` failure, leaving an unreduced failed attempt for
  SM-4 to classify as `runtime_internal_contract_misalignment`;
- the owner-local reducer now gives determined failure/governance dispositions
  priority over mutable prerequisite and freshness bookkeeping; attempt 1
  exhaustion and terminal drive failures close any pending candidate attempt in
  the same controller CAS; the LLM boundary returns stable typed
  `candidate_response_truncated` schema failure when a response reaches the
  sealed output ceiling before candidate JSON completes;
- deterministic regressions now cover freshness debt plus candidate exhaustion,
  terminal drive failure plus a pending candidate transaction, the same-contract
  two-attempt drive sequence, and output-limit truncation classification.

The remaining CA-8 item is the explicitly external fresh deployed acceptance
below. It must use a new Runtime V2 Run, never the pre-closure `run-009` or the
pre-priority-fix `run-001` aggregate. The deterministic verifier includes typed
admission, partial-replay/restart, freshness-priority, atomic terminal closure,
and output-limit truncation coverage.

## Purpose

本方案用于收口 Runtime V2 candidate admission 与 attempt transaction 未闭合问题，
确保 provider 返回的 candidate 无论被接受、拒绝、判定 stale，还是耗尽 attempt
预算，都只能通过 Orchestrator 的 typed 状态转换落入 persisted aggregate。普通校验
错误不得越过 SM-3 进入 SM-4 drive failure，也不得留下 `pending` attempt 后直接将
Run 投影为 terminal。

本方案是实现执行文档，不创建第二份 Runtime V2 核心语义。状态机语义仍以
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md) 为准，实现 SoT
仍为 `internal/modules/kernel/orchestrator/readme.md`。LLM request 的 stable prefix
与 C3/C4 volatile candidate context 仍遵循
[runtime-request-cache-note.md](../../00-core/runtime-request-cache-note.md)。

## Live Audit Baseline

2026-07-20 对重新部署版本 `69a2486f97db` 的 live 核对确认：

- 服务、projection 与 AI provider 健康；问题不是服务未 ready 或 provider 不可用。
- workspace 修改后的模型已正确进入 activation，实际 candidate 请求使用
  `DeepSeek-V4-Flash`；问题不是 TUI model 持久化或 provider/model 解析漂移。
- 最新 `run-009` 未正向完成，terminal failure 为：

```text
runtime_contract_defect:
runtime candidate includes 1 operation(s) that replay active rejected shape(s)
```

- aggregate 终态仍保留当前 authority contract 的
  `produce_validation / attempt 0 / pending` record，没有形成 failed attempt 0、attempt 1
  retry 或 candidate exhaustion。
- 当前阶段不要求兼容该历史异常 aggregate。收口后允许 current-schema load/validate
  对这类数据 fail-fast；部署验证使用 fresh Run，必要时在部署前清理旧 fixture 数据，
  不增加 schema migration、in-place repair 或兼容分支。

### Follow-up Fresh Run Finding

同日重新部署后创建的 fresh `run-001` 证明 model activation 已生效，实际 provider
请求使用 `ai-proxy / DeepSeek-V4-Flash`。该 Run 的两个 same-contract candidate attempt
都在输出达到 sealed 4096-token ceiling 后以 `unexpected EOF` 结束。真正的缺口不是
provider 失败本身，而是 persisted facts 同时包含 freshness reacquisition debt 时，SM-2
先选择了 `produce_state_change`，压过 attempt 1 已确定的
`candidate_retry_exhausted`：

```text
SM-3 attempt 1 -> failed(schema, candidate_response_truncated)
  + Facts.FreshnessReacquireCoverage != empty
  -> reducer incorrectly selects freshness-driven produce_state_change
  -> aggregate invariant detects failed attempt 1 without reducer transition
  -> SM-4 records runtime_internal_contract_misalignment
  -> Run remains active instead of reaching a typed terminal
```

收口后的统一顺序为：已经由 persisted failure class、budget 或 resolution 确定的
terminal/governance disposition 先于 mutable prerequisite/freshness bookkeeping；只有仍可
恢复或仍需业务 repair 的 failure 才继续参与普通 prerequisite/focus 选择。该规则属于
Orchestrator owner reducer，不下沉到 `internal/pkg`。terminal 决策与 pending attempt
closure 在同一 CAS 内完成，重启不能再次发送旧 provider transaction。

本阶段仍不处理 `run-001` 的兼容、迁移或自动修复。部署验收必须再次创建新的 fresh
Run。

### Observed Failure Chain

```text
SM-3 candidate attempt 0 = pending
  -> candidate contains a partial replay of active rejected operation shapes
  -> ValidateCandidateAgainstRejectedShapes returns ordinary error text
  -> isRuntimeCandidateReplayError does not match the partial-replay wording
  -> pending attempt is not changed to failed
  -> error escapes requestAndSealRuntimeV2Candidate
  -> SM-4 classifyRuntimeV2DriveFailure
  -> runtime_internal_contract_misalignment / runtime_contract_defect
  -> terminal projection
  -> persisted aggregate still contains pending attempt 0
```

当前 string classifier 只匹配：

```text
replays active rejected operation shape
replays rejected operation shape
replay rejected shape
only validation evidence is missing
```

而 partial replay 返回：

```text
includes 1 operation(s) that replay active rejected shape(s)
```

这不是需要增加一个字符串分支的局部缺陷，而是 candidate rejection 仍以普通
`error` 穿过 owner 状态机边界的结构性问题。

## Root Cause

### 1. Contract package 承担了 authority admission

`internal/pkg/workorchruntimecontract/runtime_candidate_anti_replay.go` 当前同时包含：

- rejected operation shape DTO 与 normalization；
- operation shape 稳定哈希；
- 从 persisted facts、completed dispatch 与 candidate attempt 派生 active rejected
  shape；
- 判断 candidate 是否允许进入当前 authority transaction；
- 将拒绝结果编码为普通 error 文本。

前两项属于合同或无状态结构处理，可以留在 `internal/pkg`。后两项读取当前
aggregate lineage 并决定 candidate 是否可进入 SM-3，是 Orchestrator owner authority，
不得继续位于共享 contract package。

### 2. Mainline 通过错误文本选择状态转换

`internal/modules/kernel/orchestrator/biz/runtime_v2_mainline.go` 的
`isRuntimeCandidateReplayError` 通过 `strings.Contains` 将部分普通 error 转为
`FailCandidateAttempt(contract)`。任何错误文案变化、复数形式或新增 admission gate
都可能绕过 attempt failure transition，错误进入 SM-4。

### 3. Admission 与 attempt closure 不是同一个 CAS transition

当前流程为：

```text
BeginCandidateAttempt(pending)
  -> validate / RecordCandidateProposal
  -> mainline inspect error
  -> optional FailCandidateAttempt
```

candidate 被 admission 拒绝与 `pending -> failed` 分成两次 owner 调用。只要第一步
返回未分类 error、进程中断或调用方遗漏第二步，persisted transaction 就保持打开。

### 4. Terminal invariant 不完整

现有 validation 能拒绝：

- attempt 1 不是同 contract retry；
- accepted attempt 没有 sealed dispatch；
- accepted attempt 后仍存在 retry；
- attempt 超过 0/1。

但没有拒绝：

```text
Decision.Action in {complete, fail} && current authority contract has pending attempt
```

因此 SM-4 drive failure 可以终态化 aggregate，而 SM-3 transaction 仍未闭合。

## State-Machine Impact

| 状态机 | 影响 | 必须保持 |
| --- | --- | --- |
| SM-1 Run lifecycle | 不新增状态；只禁止从未闭合 candidate transaction 投影 terminal | terminal immutable；继续工作只能新建 Run |
| SM-2 authority/focus | attempt 1 exhaustion 形成 typed failure fact；已经确定的 terminal/governance failure 优先于 freshness/prerequisite bookkeeping | reducer 是 next action 唯一权威；candidate rejection 不直接选择业务 repair；`internal/pkg` 不选择迁移 |
| SM-3 candidate/dispatch | 本方案主要修改：typed admission、attempt 0/1 原子闭合、terminal closure invariant | same transaction、same stable prefix、无 attempt 2、accepted 必须同时 seal dispatch；terminal CAS 不得遗留 pending attempt |
| SM-4 drive/recovery/projection | 不再把普通 candidate rejection 或已确定 exhaustion 误分类为 drive failure；terminal drive failure 同 CAS 关闭开放 attempt | 仅真正的 CAS、owner transient、internal invariant 或 delivery failure进入 SM-4 recovery/classification；重启不重放已关闭 provider transaction |

### Required Positive Flow

```text
LLM typed candidate
  -> Orchestrator runtimev2 CandidateAdmission.Evaluate
       -> accepted
            -> persist proposal / owner feasibility
            -> accept attempt + seal dispatch in one CAS
       -> rejected_contract
            -> pending attempt 0 -> failed(contract) in one CAS
            -> attempt 1 pending with same transaction/stablePrefix
       -> rejected_contract on attempt 1
            -> attempt 1 failed + candidate_exhausted fact + reducer in one CAS
            -> governed wait or typed terminal
       -> prerequisite_stale
            -> attempt invalidated + prerequisite transition in one CAS
```

### Forbidden Flow

```text
contract package error text
  -> mainline strings.Contains classifier
  -> SM-4 drive failure
  -> terminal with pending attempt
```

同时禁止：

- `internal/pkg` 读取 candidate attempt ledger 后选择 retry、fail、reduce 或 terminal；
- LLM block 根据 error 文本决定 attempt 0/1；
- application、Run owner、Result owner或 projection 补关 candidate transaction；
- 通过增加 `isRuntimeCandidateReplayError` 文本分支宣称完成；
- 为既有异常 aggregate 增加兼容 schema、隐式迁移或启动时 authority rebuild。

## Target Ownership And Package Boundary

| 落点 | 允许职责 | 禁止职责 |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | DTO、enum、stable code、normalization、stable structural hash、单 DTO/record 结构校验 | 读取 aggregate lineage 选择 active rejected shape；决定 retry/reduce/terminal；执行 CAS |
| `kernel/orchestrator/biz/runtimev2` | 从 persisted aggregate 派生 admission facts；candidate admission authority；attempt CAS transition；seal 与 invariant | provider rendering；跨 owner 直接 service/repository；业务 profile 特化 |
| `kernel/orchestrator/biz/runtime_v2_mainline.go` | 调用 typed controller result并按 disposition 顺序推进 drive | 字符串错误分类；修改 attempt record；重建 authority |
| `blocks/llm` | 消费 typed candidate command/context；返回 candidate 或 typed provider/schema failure | 读取 Run aggregate；判断 active rejected lineage；选择 attempt/next action |
| `kernel/run` | CAS 持久化完整 `RuntimeStateRecord` | candidate admission、reducer、failure classification |
| `blocks/result` | 保存 Orchestrator 已裁决 terminal outcome | 根据 pending/failed attempt 自行裁决结果 |
| `application/workorchd` | 只读投影 candidate/terminal 状态 | 用 projection 修复 aggregate 或触发第二条 retry |

### Contract Package Retention Matrix

`runtime_candidate_anti_replay.go` 收口后应按下表拆分：

| 符号/逻辑 | 目标位置 | 原因 |
| --- | --- | --- |
| `RuntimeRejectedOperationShape` | 保留 `workorchruntimecontract` | typed DTO |
| `NormalizeRuntimeRejectedOperationShapes` | 保留 `workorchruntimecontract` | 无状态 normalization |
| stable operation shape hash | 保留或导出为 contract pure helper | operation shape identity 是持久化/请求合同，不选择状态 |
| `ActiveRejectedOperationShapes` | 移至 `orchestrator/biz/runtimev2` | 读取 facts/dispatch/attempt lineage 并决定 active authority context |
| `activeRejectedOperationShapeSet` | 移至 `orchestrator/biz/runtimev2` | admission owner 内部派生集合 |
| `latestAcceptedEvidenceRevision` | 移至 `orchestrator/biz/runtimev2` | 解释 current-focus evidence lineage |
| `ValidateCandidateAgainstRejectedShapes` | 替换为 runtimev2 typed admission | 结果影响 pending/failed/attempt 1 |
| `ValidateCandidateAgainstMissingValidationGap` | 移至 runtimev2 admission | 读取 current facts/decision 并拒绝 candidate，属于 SM-3 authority gate |
| `RuntimeCandidateContextForDecision` 中 rejected-shape 派生 | 移至 Orchestrator；contract builder只接收显式 DTO | context package不能从 ledger 重建 authority |

## Typed Admission Contract

candidate admission 是 Orchestrator owner 内部合同，不需要新增跨 owner EventHub topic。
建议在 `biz/runtimev2` 定义等价于下列语义的 typed result；具体 Go 名称可按现有
controller 命名统一，但字段与 disposition 不得退回字符串判断：

```go
type CandidateAdmissionDisposition string

const (
    CandidateAdmissionAccepted         CandidateAdmissionDisposition = "accepted"
    CandidateAdmissionRejectedContract CandidateAdmissionDisposition = "rejected_contract"
    CandidateAdmissionRejectedInfeasible CandidateAdmissionDisposition = "rejected_infeasible"
    CandidateAdmissionPrerequisiteStale CandidateAdmissionDisposition = "prerequisite_stale"
)

type CandidateAdmissionResult struct {
    Disposition CandidateAdmissionDisposition
    Failure     *runtimecontract.RuntimeCandidateFailure
    Stale       *runtimecontract.RuntimePrerequisiteStale
}
```

约束：

1. `error` 只表示调用参数、CAS conflict、repository/owner transport 或 aggregate
   invariant defect；可预期的 candidate rejection 必须是 typed disposition。
2. `rejected_contract` 必须携带 `RuntimeCandidateFailureClassContract`、稳定 `Code`
   和 `Signature`；人类可读 `Reason` 不参与控制流。
3. full replay 与 partial replay 都进入同一 typed contract rejection 路径；可使用
   不同稳定 code 便于诊断，但不得依赖 message。
4. attempt 0/1 使用同一 admission evaluator、同一 rejected-shape set、同一
   authority transaction 和同一 stable prefix。
5. rejected shapes 位于 candidate context 的 C3/C4 volatile tail；迁移派生逻辑
   不得改变 C0/C1/C2，`stablePrefixHash` 必须保持不变。

建议稳定 failure code 至少覆盖：

| Code | Class | 触发条件 |
| --- | --- | --- |
| `candidate_replays_rejected_shape` | `contract` | candidate 全量重放 active rejected shapes |
| `candidate_partially_replays_rejected_shape` | `contract` | candidate 仅部分 operation 重放 active rejected shapes |
| `candidate_action_evidence_mismatch` | `contract` | 只有 validation gap 时仍提交不相容 state-change candidate 等 action/evidence 偏差 |
| `candidate_admission_invariant` | `internal` | persisted transaction、proposal 或 admission ledger 自相矛盾；此项才允许进入 internal failure 路径 |

## Atomic Transition Contract

### Attempt 0 Contract Rejection

同一 CAS 必须完成：

```text
attempt 0 pending
  -> persist rejected proposal/hash when available
  -> attempt 0 failed(class=contract, code, signature, reason)
  -> StateRevision + 1
```

该 transition 不调用 reducer、不写 `Facts.Failure`，mainline 下一循环只能开始
same-contract attempt 1，并将 attempt 0 typed failure 作为 `PreviousFailure`。

### Attempt 1 Contract Rejection

同一 CAS 必须完成：

```text
attempt 1 pending
  -> attempt 1 failed(class=contract, ...)
  -> Facts.Failure = candidate_exhausted
  -> reducer(Facts, prior authority snapshot)
  -> governed wait or terminal decision
  -> StateRevision + 1
```

不得先保存 failed attempt、再由 mainline 另一次调用补 `Facts.Failure`。进程在任意
CAS 边界退出后，重启只能看到完整的 retryable 或 exhausted transaction。

### Accepted Candidate

同一 CAS 必须继续保持：

```text
pending attempt
  -> accepted attempt
  -> clear unsealed proposal/feasibility scratch
  -> ActiveDispatch = sealed candidate + transaction + candidate hash
  -> StateRevision + 1
```

accepted attempt 与 sealed dispatch 不得分离。

### Prerequisite Stale

stale 不是 candidate failure。现有 `pending -> invalidated` 语义保留，并必须在同一
CAS 清除 proposal/feasibility/failure，记录 typed stale 后回到 SM-2 prerequisite
reacquire；不得消费 attempt 1 或 candidate exhaustion。

## Aggregate Invariants

在 `runtimev2.RuntimeAggregate.Validate` 与 persisted
`RuntimeStateRecord.Validate` 的 current schema 中补齐：

1. `Decision.Action == complete || Decision.Action == fail` 时，不得存在 current
   authority contract 的 `pending` candidate attempt。
2. `Decision.Terminal != nil` 时不得存在任何仍可继续的 pending candidate attempt；
   historical contract records只能是 `failed`、`accepted` 或 `invalidated`。
3. `ActiveDispatch != nil` 时，其 transaction 必须恰好对应一个 accepted attempt；
   不能同时存在同 contract pending attempt。
4. attempt 0 failed 后若 attempt 1 不存在，aggregate 是合法 retryable 状态；drive
   必须选择 attempt 1，而不是 terminal。
5. attempt 1 failed 后必须已有 `candidate_exhausted` fact 或 reducer 已形成相应
   governed/terminal decision；不能停留在原 `produce_*` decision。
6. candidate admission rejection 不得改变 evidence revision、repair budget、provider
   transport recovery budget或 prerequisite refresh ledger。
7. current schema 不兼容既有 terminal+pending 异常记录；不增加迁移或兼容分支。

## Work Packages

### CA-1: Contract Package Neutralization

**Files**

- `internal/pkg/workorchruntimecontract/runtime_candidate_anti_replay.go`
- `internal/pkg/workorchruntimecontract/runtime_candidate.go`
- 对应 `*_test.go`

**Change**

- 保留 rejected shape DTO、normalization、stable hash 与结构级 validation。
- 移除读取 facts + dispatches + attempts 后选择 active shape 的 authority logic。
- 将 candidate context builder 改为接收 Orchestrator 已派生的
  `RejectedOperationShapes`，不自行读取 dispatch/attempt ledger。
- 从 contract package 移除任何决定 candidate rejection disposition 的函数。

**SM**: SM-3；不改变 SM-2 reducer。

**Direct regression**

- shape normalization/dedup/sort；
- semantic parameter hash 变化、display-only metadata 不改变 hash；
- `RuntimeCandidateContext.Validate` 仍只校验 scope/revision/focus/activation 结构；
- boundary scan 确认 contract package 不再出现 attempt retry/terminal 选择。

**Done when**

- `internal/pkg` 单独测试不需要构造完整 aggregate、controller 或 repository；
- package 测试只证明 DTO/纯函数合同，不证明状态迁移。

### CA-2: Orchestrator Candidate Admission Authority

**Files**

- 新增或收口 `internal/modules/kernel/orchestrator/biz/runtimev2/candidate_admission.go`
- `candidate_admission_test.go`

**Input**

- current `RuntimeAggregate`；
- current pending attempt；
- provider candidate；
- 已持久化 owner feasibility records。

**Output**

- typed `CandidateAdmissionResult`；
- active rejected shape DTO，用于 candidate context；
- 普通 error 仅用于 invariant/internal failure。

**Change**

- 迁入 active rejected evidence/feasibility lineage 派生；
- 迁入 full/partial replay、missing-validation/action mismatch admission；
- 为每类可预期拒绝生成稳定 failure class/code/signature；
- 保持领域中立，不识别文件类型、Git、语言、测试框架或 profile ID。

**SM**: SM-2 facts read + SM-3 admission；不得选择下一 RuntimeAction。

**Direct regression**

- full replay rejected；
- partial replay rejected；
- candidate 全部为新 shape accepted；
- later accepted current-focus evidence使旧 shape失活；
- unbound/older evidence 不错误清除 active shape；
- infeasible proposal shape 在相同 evidence revision 下继续 active；
- repair directive action 不被 anti-replay 改写。

**Done when**

- 测试断言 typed disposition/code，不使用 `strings.Contains(err.Error())`；
- evaluator 不持久化、不调用 reducer、无 EventHub 依赖，可作为 owner-local pure
  decision unit 独立回归。

### CA-3: Atomic Proposal Admission

**Files**

- `internal/modules/kernel/orchestrator/biz/runtimev2/controller.go`
- `controller_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`

**Change**

- 将 `RecordCandidateProposal` 收口为 typed proposal admission transition；
- accepted proposal与 pending proposal持久化保持幂等；
- rejected proposal在同一 CAS 将 pending attempt改为 failed；
- duplicate delivery必须返回同一 persisted result，不新增 attempt、不重复 Save；
- attempt 0 rejection不得调用 reducer；attempt 1 rejection必须原子形成 exhaustion
  fact并 reduce。

**SM**: SM-3，attempt 1 exhaustion 与 SM-2 reducer有原子交接。

**Direct regression**

- partial replay：attempt 0 pending -> failed，revision仅 +1；
- duplicate partial replay：不新增 Save；
- attempt 0 failed后仅允许 same-contract/same-prefix attempt 1；
- attempt 1 rejection产生 `candidate_retry_exhausted`；
- CAS conflict重载后不会出现两个 attempt 1；
- failure transition不改变 evidence/repair/recovery ledgers。

**Done when**

- controller 调用返回时不存在“candidate 已拒绝但 attempt 仍 pending”的可观察状态。

### CA-4: Atomic Final Admission And Seal

**Files**

- `internal/modules/kernel/orchestrator/biz/runtimev2/controller.go`
- `internal/modules/kernel/orchestrator/biz/runtimev2/dispatch_commit.go`
- 对应 controller/dispatch tests

**Change**

- final admission 重新核对 current authority、proposal hash、rejected shapes 与 persisted
  feasibility；
- accepted 与 `ActiveDispatch` seal 继续同一 CAS；
- 可预期 final rejection使用 typed result并同 CAS关闭 attempt；
- transaction drift、proposal mutation、缺失 owner feasibility 属于 conflict/internal
  invariant，不伪装为 candidate contract retry。

**SM**: SM-3。

**Direct regression**

- accepted attempt必有 active dispatch；
- proposal hash漂移拒绝且不 seal；
- final partial replay关闭 attempt而不泄漏 error；
- feasibility snapshot漂移不执行 operation；
- crash-after-proposal-before-seal重启不重复 provider call；
- crash-after-seal重启不重复 seal或 durable effect。

**Done when**

- accepted、failed、invalidated 是 pending attempt唯一出口；
- 每个出口均由 controller CAS 持久化。

### CA-5: Mainline Typed Sequencing

**Files**

- `internal/modules/kernel/orchestrator/biz/runtime_v2_mainline.go`
- `runtime_v2_state_repository_test.go`
- `runtime_v2_drive_test.go`

**Change**

- 删除 `isRuntimeCandidateReplayError`；
- mainline 只根据 typed admission disposition决定继续 attempt 1、返回 prerequisite
  reacquire、使用 sealed dispatch或进入 reducer terminal；
- admission ordinary error只能作为真正 invariant/internal error返回；
- provider/schema/infeasible/replay/stale 分类保持互斥，不能共用 message classifier。

**SM**: SM-3 sequencing；防止错误越界进入 SM-4。

**Direct regression**

- partial replay完成 attempt 0 -> attempt 1，不进入
  `classifyRuntimeV2DriveFailure`；
- attempt 1 exhaustion返回 typed terminal；
- provider transient只消费 provider transport budget；
- provider authentication/configuration进入治理事实；
- prerequisite stale不消费 candidate retry；
- internal transaction mismatch仍进入 internal contract failure。

**Done when**

- production mainline不存在基于 candidate error message的状态转换；
- `rg -n "isRuntimeCandidateReplayError|replay.*strings.Contains"` 无生产命中。

### CA-6: Terminal Transaction Closure Invariant

**Files**

- `internal/modules/kernel/orchestrator/biz/runtimev2/dispatch_commit.go`
- `internal/pkg/workorchruntimecontract/runtime_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_v2_state_repository_test.go`
- 对应 canonical/aggregate validation tests

**Change**

- current schema 同时在 DTO record validation 与 aggregate authority validation中拒绝
  terminal+pending；
- 拒绝 attempt 1 failed但 decision仍停留原 `produce_*` 的半闭合 aggregate；
- 保持 accepted attempt -> sealed dispatch 与 attempt 0/1 identity invariants；
- 不增加旧数据兼容、schema migration或 load-time repair。

**SM**: SM-3 closure + SM-4 terminal delivery entrance guard。

**Direct regression**

- `fail + pending` invalid；
- `complete + pending` invalid；
- terminal outcome + pending historical contract invalid；
- attempt 0 failed且尚无 attempt 1合法；
- attempt 1 failed + exhaustion/reducer terminal合法；
- accepted + active/completed dispatch合法；accepted无 dispatch invalid。

**Done when**

- 任何 terminal aggregate 都能证明 SM-3 transaction 已闭合；
- 旧 `run-009` 形态在 current-schema fixture 中明确失败。

### CA-7: LLM Context And Stable-Prefix Regression

**Files**

- `internal/modules/kernel/orchestrator/biz/runtime_v2_mainline.go`
- `internal/modules/kernel/orchestrator/pkg/events` candidate command tests
- `internal/modules/blocks/llm/biz` candidate/context tests
- request rendering/cache verifier tests

**Change**

- Orchestrator 在发出 candidate command前派生精确 active rejected shapes；
- LLM block只消费 `RuntimeCandidateContext.RejectedOperationShapes`；
- attempt 1 必须收到 attempt 0 typed `PreviousFailure` 与相同 rejected shape set；
- rejected shapes 保持 C3/C4 volatile，不进入 stable prefix。

**SM**: SM-3 request contract；不赋予 LLM authority。

**Direct regression**

- attempt 0/1 `StablePrefixHash` 相同；
- partial replay failure加入 volatile tail；
- rejected shape变化只改变 volatile/focus bundle hash，不改变 C0/C1/C2 prefix；
- LLM candidate command无法缺省 workspace/run/transaction/attempt；
- LLM block无 Run repository、Orchestrator service 或 aggregate读取依赖。

**Done when**

- owner派生事实与 provider rendering职责完全分离；
- request audit能定位 rejected shape hash与 failure lineage。

### CA-8: Cross-Component, Restart And Live Acceptance

**Components**

- `kernel/orchestrator`：主责任；
- `kernel/run`：CAS persistence/restart fixture；
- `blocks/llm`：typed request/response fake；
- `blocks/result`：terminal outcome storage；
- `application/workorchd`：read-only projection/TUI display。

**Cross-component regression**

1. Orchestrator fake LLM responder返回 full replay candidate；attempt 0 failed、attempt 1
   可继续。
2. fake responder返回 mixed candidate（1 replay + 1 new）；partial replay走同一 typed
   contract rejection。
3. attempt 1仍 replay；形成 `candidate_retry_exhausted`，Result只保存 reducer裁决。
4. 在下列持久化边界分别停止并重启：
   - attempt pending、provider response前；
   - rejected proposal CAS后、attempt 1 begin前；
   - proposal persisted后、owner feasibility前；
   - feasibility persisted后、seal前；
   - seal CAS后、operation execution前。
5. duplicate candidate response、duplicate drive与CAS reload不增加 attempt/provider call/
   dispatch。
6. terminal projection失败只重试 Result/Run/Workspace delivery，不重新 admission。

**Fresh deployed acceptance**

- 使用最新 build新建 fresh Runtime V2 Run，不复用 `run-009` 或 `run-001`；
- workspace activation中记录的 provider/model必须与真实 LLM request一致；
- 构造至少一次 partial replay attempt 0，确认 attempt 1收到 typed failure并继续；
- 若 provider response达到 sealed output token ceiling并发生 JSON truncation，必须记录
  typed `candidate_response_truncated`，attempt 1耗尽后直接形成
  `candidate_retry_exhausted`，不得被 freshness debt改写；
- 最终允许两种正确结果：
  - attempt 1提交修正 candidate并继续到 sealed dispatch/正向 evidence；
  - attempt 1再次拒绝并形成 `candidate_retry_exhausted` typed terminal；
- 两种结果均必须满足：无 pending attempt、无
  `runtime_internal_contract_misalignment`误分类、无人工 Resume补偿。

**Done when**

- owner-local、file-backed restart/crash、cross-component与fresh deployed evidence全部具备；
- read model展示的是 persisted attempt/terminal事实，不参与恢复。

## Component-By-Component Regression Commands

优先执行直接组件回归，再执行合同边与全库门禁：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'Test.*Candidate|Test.*RuntimeV2.*Replay|Test.*RuntimeV2.*Restart' -count=1
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

Orchestrator/Run 变更后追加：

```bash
go test -race ./internal/modules/kernel/orchestrator/... -count=1
go test -race ./internal/modules/kernel/run/... -count=1
make verify-boundary-contract
git diff --check
```

完成全部工作包后执行：

```bash
go test -p 1 ./... -count=1
go vet ./...
make verify-runtime-v2-positive-convergence-file
make release-check
```

如果 CI 最小环境没有 `rg`，`verify-boundary-contract` 必须继续使用已经约定的
等价 `grep -R` fallback；不能因工具缺失跳过边界验证。

## Required Test Matrix

| Case | Attempt transition | Expected authority result | Negative assertion |
| --- | --- | --- | --- |
| no active rejected shape | pending 0 -> accepted | sealed dispatch | 不创建 failure |
| full replay attempt 0 | pending 0 -> failed | attempt 1 available | 不进入 SM-4 failure |
| partial replay attempt 0 | pending 0 -> failed | attempt 1 available | 不残留 pending 0 |
| corrected attempt 1 | pending 1 -> accepted | sealed dispatch | stable prefix不漂移 |
| replay attempt 1 | pending 1 -> failed | candidate exhausted -> reducer | 不存在 attempt 2 |
| owner infeasible | pending -> failed(infeasible) | validation policy决定reduce或retry | 不伪装 replay |
| prerequisite stale | pending -> invalidated | acquire prerequisite | 不消费 candidate attempt 1 |
| provider transient | pending保持可恢复或typed budget exhaustion | provider recovery ledger | 不消费 candidate contract budget |
| provider auth/config | pending closed with typed failure | governance fact/action | 不转 internal misalignment |
| proposal crash restart | pending + proposal复用 | 不再次请求 provider | 不新增 attempt |
| final seal crash restart | accepted + sealed dispatch复用 | exactly-once execution | 不重复 durable effect |
| terminal aggregate | no pending attempt | Result/Run/Workspace delivery | projection失败不重开 admission |
| duplicate drive | state revision不变或合法推进 | idempotent | 不重复 provider/probe/dispatch |

## Implementation Order And Merge Gates

必须按以下顺序实施，避免 mainline先依赖尚未存在的 typed controller contract：

1. **CA-1 + CA-2**：先确定 package边界、typed admission与owner-local test。
2. **CA-3 + CA-4**：将所有 pending出口收进 controller CAS。
3. **CA-5**：mainline改用 typed result并删除 string classifier。
4. **CA-6**：在新 transition已完整后启用严格 terminal closure invariant。
5. **CA-7**：确认 LLM context/stable prefix没有回归。
6. **CA-8**：cross-component、file-backed crash/restart与fresh deployed验收。

每一波提交必须满足：

- 说明影响 SM ID；
- 列出 owner input/output contract；
- 至少包含一个正向和一个负向直接测试；
- 不允许以端到端测试替代 controller/contract package独立回归；
- 不允许在 CA-5 完成前删除旧 classifier却留下普通 rejection error；
- 不允许在 CA-6 启用严格 invariant后添加历史数据兼容分支。

## Non-Goals

- 不改变 File、Git、Shell、MCP、SSH、Web owner 的 operation execution裁决。
- 不改变 operation admissibility inventory或owner feasibility语义。
- 不改变 Result owner acceptance职责。
- 不增加 candidate attempt 2或动态 attempt预算。
- 不把 candidate contract retry并入 per-focus repair budget或 provider recovery budget。
- 不恢复 V1 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、自动
  replan successor或application continuation queue。
- 不在 LLM block、Run owner、application或projection中重建 authority。
- 不为 `run-009`、`run-001` 或同类历史异常 aggregate增加兼容读取、迁移或自动修复。

## Closure Criteria

本方案只有同时满足以下条件才可归档：

1. `internal/pkg/workorchruntimecontract` 不再拥有读取 persisted lineage后决定
   candidate admission的逻辑。
2. production mainline不存在 candidate rejection string matching。
3. full/partial replay在 attempt 0 都原子形成 failed record并进入 attempt 1。
4. attempt 1 rejection原子形成 candidate exhaustion并经 reducer收敛。
5. current-schema validation拒绝任何 terminal+pending aggregate。
6. restart/crash tests覆盖 pending、proposal、feasibility、seal五个边界。
7. LLM request的 C0/C1/C2 stable prefix不因 rejected shape迁移而漂移。
8. fresh deployed Run在不人工 Resume的情况下正向执行或以正确 typed exhaustion
   收敛，且不存在 `runtime_internal_contract_misalignment`误分类。
9. active overview、current state、unclosed matrix与todo index同步标记完成后，再将
   本文迁入 `docs/90-archive/completed/`。
