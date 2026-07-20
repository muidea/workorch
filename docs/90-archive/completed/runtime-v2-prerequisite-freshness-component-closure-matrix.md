# Runtime V2 Prerequisite Freshness Component Closure Matrix

Status: completed

Type: implementation runbook

Parent plan: [runtime-v2-prerequisite-freshness-closure.md](runtime-v2-prerequisite-freshness-closure.md)

End-to-end supplement: [runtime-v2-system-component-closure-supplement.md](runtime-v2-system-component-closure-supplement.md)

Last Updated: 2026-07-20

## Purpose And Use

本文件把 RPF-1～RPF-5 拆成可单独实现、回归、验收和合并的组件工作包。它回答
“哪个组件改什么、只能和谁交互、交付何种合同、怎样证明已经收口”。实施顺序必须遵守
本文件；某个组件的单测通过不代表跨组件或 deployed 验收已完成。

它是父方案的执行矩阵，不是第五份状态机，也不替代以下权威来源：

- 中立状态机和禁止迁移：`docs/00-core/runtime-stage-contract.md`；
- Runtime V2 实现合同：`internal/modules/kernel/orchestrator/readme.md` 与
  `internal/pkg/workorchruntimecontract`；
- 当前未关闭项：`docs/01-overview/current-unclosed-matrix.md`。

每个工作包合并前都必须写明影响的 SM 编号，并附上本文件要求的直接回归命令和结果。

**当前交接点：**managed M2 的 MCP capability-owner 部分已经本地收口：显式
metadata 经 registry 和 activation 保持不变，选中的只读 tool 可作为 validation
执行，并已提交 `focus-002` receipt。当前第一个断言是 mock assessment 写入了当前
slice 以外的历史 coverage ID。它是[单一实施规格中的 M2-H1](runtime-v2-code-closure-execution-spec.md#m2-h1--managed-assessment-current-slice-isolation)，属于 verifier fixture，不能成为放宽 owner、reducer、candidate DTO、Decision 或 Resume
合同的理由。

## Fixed Architecture And Interaction Contract

```text
Application/TUI/Web
  Start | Resume (typed bootstrap only)
        |
        v
Run owner <----CAS aggregate---- Orchestrator (SM-2/SM-3/SM-4 authority)
                                      |  ^
             typed acquire/probe/execute EventHub commands/results
                                      v  |
          File Shell Git MCP SSH Web LLM (single-resource/capability owners)
                                      |
              typed receipt: effect + ResourceChanges | PrerequisiteStale
                                      v
          Result / Eventstream / workorchd read model / Web / TUI
                       (terminal storage or display only; no authority)
```

| From | To | Permitted contract | Prohibited responsibility |
| --- | --- | --- | --- |
| Application / TUI / Web | Orchestrator | typed `Start` / unified `Resume` bootstrap and read requests | choosing action, patching active run provider/model, direct owner access |
| Orchestrator | prerequisite/capability owner | EventHub `Send` with concrete acquire, probe, execute command/result | repository/service injection, `map[string]any`, generic envelope |
| owner | Orchestrator | typed fact + `RuntimePrerequisiteObservation`, or durable receipt + `RuntimeResourceChange`, or `RuntimePrerequisiteStale` | selecting candidate, retry policy, result terminalization |
| Orchestrator | Run / Result / Workspace owner | typed CAS/projection command | letting an owner choose the next runtime action |
| Eventstream / workorchd / Web / TUI | readers | derived audit/read model | rebuilding or overwriting aggregate, resolving a ticket, turning stale into a retry |

All EventHub-using Module/Block changes must retain the existing
`module.go -> biz.(basebiz.Base) -> service/adapter` direction. `basebiz.Base`
remains an owner-neutral Hub/observer/background-task base only: it must not
gain freshness policy, topic aliases, DTOs, a repository, or a cross-owner
facade. An Initiator must not embed it.

## Non-Negotiable Per-Change Invariants

1. A mutable satisfied prerequisite has one persisted observation. Its resource
   identity is exactly `OwnerModuleID + Domain + SubjectRef`; matching never
   uses path-prefix or string-contains guessing.
2. A durable accepted effect reports owner-observed resource changes. A stale
   pre-effect receipt reports `RuntimePrerequisiteStale`, has no effect and
   cannot also report a change.
3. Only Orchestrator intersects changes/stale with observations, returns the
   requirement to pending, consumes the refresh budget, and asks the reducer
   for the next action.
4. A stale proposal or dispatch becomes `invalidated`, never candidate failure,
   repair debt, provider retry, operation-recovery retry, or rejected evidence.
5. All facts, receipt/evidence close, invalidation ledger and affected pending
   prerequisites commit in one aggregate CAS. Replays, late replies and CAS
   reloads are deduplicated by persisted identity.
6. Refresh reuses the sealed Run definition and `ProviderModelTarget`. It may
   create a new authority transaction but starts candidate attempt at 0.
7. C0-C2 LLM cache prefix is invariant across refresh. Freshness metadata is
   volatile context only (C3-C5).
8. A projection may show `prerequisite_refreshing` or exhausted, but cannot
   alter authority. The only user-facing run continuation command is `resume`.

## Implementation Order And Merge Gates

Work packages are intentionally sequential where one persists/consumes the
previous package's contract. Parallel edits are permitted only inside a single
package when they do not alter the same aggregate schema or event DTO.

| Order | Work package | Main SM | Prerequisite | Exit gate |
| --- | --- | --- | --- | --- |
| P0 | Contract/schema characterization | SM-2/3/4 | none | typed contract validation and legacy decode tests |
| P1 | Orchestrator acquisition/invalidation authority | SM-2/3/4 | P0 | fake-owner stale/change/restart/CAS matrix |
| P2 | File, Shell, Git owner adoption | SM-2/3/4 | P1 | each owner direct test plus File→Shell→Git integration |
| P3 | MCP, SSH, Web, LLM, Result adoption | SM-2/3/4 | P1 | owner/read-path direct tests and cache/result invariants |
| P4 | Run/Workspace delivery and presentation | SM-1/4 | P1, P3 | projection is descriptive and TUI exposes only resume |
| P5 | file-backed crash matrix and deployed acceptance | SM-1/2/3/4 | P2-P4 | six restart boundaries plus fresh accepted terminal |

No P5 test failure may be repaired by adding a component-specific retry in an
owner, Application, Result, Eventstream or UI. The repair belongs in the
earliest failed package in this order.

## Atomic Component Handoff Cards

本节是后续逐项代码收口的唯一实施顺序。每张卡只允许修改表中列出的
owner 和它已拥有的 typed contract；若实现发现需要另一个 owner 的状态、
repository 或策略，应停止扩张该卡并回到 P0/P1 定义新的 EventHub 合同。
这样可以让每个组件在不启动完整服务、不调用真实 provider、也不直接依赖
其他 owner repository 的前提下完成局部功能回归。

| Card | Bounded owner / code boundary | Allowed input -> required output | SM impact | Local completion gate | Integration exit / must not do |
| --- | --- | --- | --- | --- | --- |
| C0 | `workorchruntimecontract` + orchestrator event DTO | typed observation/change/stale/remediation -> normalized, validated persisted value and concrete command/result | SM-2/3/4 schema | contract + event package invalid/duplicate/legacy-decode tests | P1 may consume it; no owner I/O, action selection or HTTP presentation |
| C1 | Orchestrator reducer/controller/executor/drive | owner fact, stale or receipt -> one aggregate CAS containing invalidation, budget state and next reducer-visible facts | SM-2/3/4 | reducer legal/forbidden, duplicate/late, CAS reload, restart and budget tests | P2/P3 owners may emit the contract; no filesystem/network access or provider fallback |
| C2 | File owner | acquire/probe/execute command -> exact File observation, typed stale, or receipt with observed target/tree change | SM-3 input; SM-2 only through C1 | File package external-mutation, no-op and same-fence replay tests | File->Shell->Git fixture; no refresh/candidate/recovery budget decision |
| C3 | Shell owner | policy/target observation and validation command -> exact policy/target stale or bounded receipt | SM-3 input; SM-2 only through C1 | Shell policy-vs-target and replay tests, including no root-mtime false stale | File->Shell->Git fixture; no File repository access or retry loop |
| C4 | Git owner | one acquired repository snapshot plus feasibility/execute command -> same-snapshot sequence, stale or fenced receipt | SM-3 input; SM-2 only through C1 | Git post-init/post-add sequence, snapshot-identity, replay and stale tests | File-backed Git restart fixture; no reuse of a pre-effect sequence or direct Run mutation |
| C5 | MCP, SSH, Web and LLM owners | sealed request plus their observable catalog/config/remote fact -> exact stale **or** ordinary typed failure/receipt | SM-2/3 inputs | one direct package regression per owner; LLM stable-prefix/no-fallback/deadline tests | P4 read mapping; no UI retry, target substitution or freshness classification from a timeout |
| C6 | Decision/Approval, Workspace and activation builder | ticket/user intent or workspace preference -> durable permitted resolution, or new sealed activation target | SM-1 admission; SM-2 governance fact; SM-4 continuation | owner/activation precedence, conflict, duplicate-resolution and restart tests | P4 presentation; no direct drive, active-Run target patch or successor creation |
| C7 | Result, Run/Workspace delivery, Eventstream, workorchd, remote/TUI/Console | terminal/projection/read DTO or user `resume` -> idempotent owner projection or descriptive display | SM-1/4 | Result duplicate delivery, projection replay, remote/TUI read-only and unified-resume tests | P5 process recreation; no authority reconstruction, ticket resolution or owner operation |
| C8 | boundary verifier and file-backed fixture | repository source / persisted aggregate -> portable structural result / one replay-safe accepted trace | SM-1/2/3/4 | verifier without `rg`; P5 six-boundary restart tests | fresh deployed acceptance; no blanket verifier exception or process-local fake completion |

### Card Completion Protocol

A card can be marked `done` only when all five statements below are true.
The card's direct gate is deliberately narrower than the next card's
integration exit; a green local test must never be promoted into a deployed
completion claim.

1. **Contract proof:** every accepted and rejected input has a concrete typed
   result; no `map`/`any`, generic envelope or cross-owner facade was added.
2. **State-machine proof:** the change names its affected SM IDs and contains
   at least one legal and one forbidden-transition assertion.
3. **Replay proof:** duplicate identity/fence, late result and restart/CAS
   reload produce no second side effect or budget charge.
4. **Boundary proof:** the component-local test uses only its own owner state
   and fake local dependencies. Cross-owner assertions are placed in C1 or
   P5 fixtures, never hidden in an owner unit test.
5. **Handoff proof:** the parent plan receives the exact test names, commands,
   status and the next unresolved card. A failure at the integration exit
   reopens the earliest card that owns the violated contract.

The fixed handoff direction is `C0 -> C1 -> C2/C3/C4/C5/C6 -> C7 -> C8`.
Cards C2--C6 can have independent local regressions after C1 stabilizes, but
their changes must be merged behind the same C1 schema and are integrated only
through C7/C8. This is the only permitted parallelism; it prevents concurrent
owners from silently creating incompatible observations or retry semantics.

## Code-Closure Handoff Board

This board turns the cards into small, reviewable code changes.  It is the
implementation order for the current residual work; it does **not** change the
Runtime V2 design contract.  A row is `locally closed` only after its stated
direct gate and the five card-completion proofs pass.  It is `system closed`
only after its named downstream exit also passes.  In particular, no row below
authorizes an Application, TUI, Console, projection, Result owner, or
capability owner to add a retry loop.

| Handoff | Status | Bounded code change and owning component | Typed interaction contract to implement | Affected SM / forbidden shortcut | Local regression required before handoff | Downstream exit |
| --- | --- | --- | --- | --- | --- |
| H1 | locally closed | **Git owner + C0/C1 contract seam**: bind capability-state, admissibility and feasibility to one owner-authored repository acquisition | `RuntimeCapabilityStateSnapshot.AcquisitionKey` is accepted by Git as the opaque persisted `AcquisitionBinding`, copied into `RuntimeOperationAdmissibility` and feasibility command/result, and included in snapshot/inventory identities and the persisted feasibility record.  A bound inventory is derived from that exact receipt; a mismatch returns typed `RuntimePrerequisiteStale`. | SM-2/3/4. Orchestrator compares opaque bindings and invalidates/reacquires; it does not derive a binding from revisions, understand Git sequence semantics, or record mismatch as candidate failure. | `TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale`; stable-identity and prerequisite-commit binding tests; post-`add`, replay and Git package gates. | Targeted file-backed Git stale/restart fixtures pass; it remains part of the shared P5 fresh deployed exit, which is not implied by this local status.
| H2 | in progress | **Remaining capability/terminal owners**: MCP, SSH, Web, LLM and Result complete their own observation/failure/delivery edges | Each acquire/probe returns a bounded observation; confirmed revision drift returns only generic stale; transport/auth/timeout returns only ordinary typed failure. LLM `provider_ceiling` now emits a persisted `provider_config` observation and candidate-time configuration drift returns that exact stale through the typed candidate contract. Result accepts only an already-judged terminal and returns an idempotent delivery receipt. | SM-2/3 for owners; SM-4 for terminal delivery. A timeout must not become stale; Result must not terminalize a recoverable refresh. | Per-owner drift/no-side-effect/replay tests; LLM deadline, C0--C2 stable-prefix, sealed-target/no-fallback tests; Result duplicate and recoverable-stale tests. | C7 projection contract and P5 terminal/restart boundaries.
| H3 | in progress | **Governance, Workspace and activation**: make the one user continuation rule durable | Decision loads the persisted ticket binding/allow-list, normalizes permitted replan-style ingress and emits one bound continuation. Approval persists its own evidence only and never supplies a synthetic Runtime V2 ticket/Resume. Workspace persists a next-run default; activation copies explicit -> workspace -> system target into a new immutable Run. | SM-1 admission, SM-2 governance fact, SM-4 drive. No direct `ResumeRun` after a resolved Decision, synthetic Approval continuation, V1 successor, active-Run model patch, or provider fallback. | Ticket allow-list/remediation, duplicate idempotency/restart, exactly-one Decision continuation, zero Approval scheduler continuations, precedence, sealed-active-run and Resume-target tests. | C7 API/TUI mapping: one visible `resume`; a replan-style intent is auditable but never a Runtime V2 command.
| H4 | open | **Run, Eventstream, workorchd, remote backend, TUI and Console**: make all read paths descriptive | Persisted aggregate/projection DTO -> display of lifecycle, blocker/allowed action, sealed and next-run targets, freshness owner/resource/revision/budget, provider failure phase, terminal-delivery phase.  User input -> typed Start, unified Resume, workspace update, or ticket-resolution intent only. | SM-1/4. A projection replay cannot drive, resolve, create Result, patch provider/model, or reconstruct authority. | Projection replay/read-only test; remote mapping test; TUI/Console tests for `resume` only, provider failure vs stale, and current vs next-run model. | P5 process recreation and fresh deployed trace.
| H5 | locally closed | **Boundary verifier and file gate**: make architecture verification portable and ensure the file-backed verifier runs its real package | Repository source -> deterministic pass/fail without requiring `rg`; scan each EventHub Module/Block for local `biz` embedding `basebiz.Base`, and reject Hub traffic outside Biz or unbounded EventHub DTOs.  The positive-convergence script targets `kernel/orchestrator/biz`, rather than the no-test parent package, and includes the Git binding regression. | Structural protection for all SMs. Do not add a component exemption, put lifecycle/business handling in a module root/service, or accept a no-test package as a file-backed gate. | `make verify-boundary-contract`; `make verify-runtime-v2-positive-convergence-file` proves the package path executes the named real fixtures. | Release gate and C8/fresh deployed trace.
| H6 | local P5 gate passed; deployed evidence open | **File-backed/deployed acceptance**: verify the persisted end-to-end chain rather than a process-local happy path | Reloaded Run aggregate + durable owner receipts -> same effects/budget/result exactly once across all six prescribed boundaries; each executable capability must have a sealed owner-admissibility prerequisite and, after an intersecting effect, each required inventory must be reacquired and committed before candidate seal; a fresh deployed run produces an accepted terminal trace. | SM-1/2/3/4. No fabricated deployed record, in-memory recovery substitute, test-only inventory insertion or extra owner/UI retry after a failed fixture. | Complete orchestrator package, six restart boundaries, bootstrap-derived Git inventory regression, exact receipt/evidence/invalidation/transaction lineage assertions, and `git diff --check`. | Boundary verifier, full release gate, then a newly captured deployed trace.

### H1 Shared-Acquisition Binding: Required Field-Level Contract

H1 is locally closed for its owner-authored binding contract, but remains a
required input to the reopened C1/C4 integration handoff. The following fields
are deliberately owner-neutral;
their **value** is opaque outside Git.  An implementation may use different Go
field names only if it preserves these semantics and updates the contract and
event DTO tests in the same change.

| Value / message | Producer | Consumer | Required rule |
| --- | --- | --- | --- |
| capability snapshot `AcquisitionBinding` | Git capability acquisition | Orchestrator persistence and later Git commands | Stable receipt/snapshot identity, not a timestamp and not merely `ObservedRevision`; included in the snapshot's stable identity/hash. |
| admissibility `AcquisitionBinding` | Git admissibility acquisition | Orchestrator prerequisite commit / feasibility request | Must equal the capability binding when the operation relies on that capability state.  A missing binding is legal only for owners/profiles that declare no capability-state prerequisite. |
| feasibility request expected binding | Orchestrator, copied from committed fact | Git feasibility handler | Never synthesized from a revision.  It selects the owner-local acquired snapshot to test. |
| feasibility result binding | Git feasibility handler | Orchestrator event adapter | Echoes the exact bound snapshot or returns typed stale.  A response for another binding is invalid/late diagnostic data, never candidate infeasibility. |
| persisted prerequisite/feasibility identity | C0/C1 normalization | restart/CAS/replay logic | Includes the non-empty binding so a process restart cannot join facts from different acquisitions. |

The H1 transition table is mandatory:

| Condition | Git result | Orchestrator result | Budget/evidence consequence |
| --- | --- | --- | --- |
| expected binding matches fresh snapshot | feasible/inventory from that snapshot | may seal the candidate | ordinary sealed transaction rules |
| expected binding is absent, expired or different | typed `RuntimePrerequisiteStale` naming the Git observation | invalidate pending proposal/dispatch and reacquire | candidate/repair/provider/operation-recovery budget unchanged; no rejected evidence |
| duplicate request with same execution/acquisition identity | original durable snapshot/receipt | deduplicate | no second Git operation or refresh charge |
| old result arrives after a newer binding is committed | bounded late diagnostic only | retain lineage; do not overwrite fresh fact | no state regression or budget charge |

### Per-Change Evidence Record

Every pull request or local implementation batch must append a compact record
to the parent plan's implementation ledger.  This makes a later audit able to
distinguish a component-local proof from a system claim.

```text
Card / handoff: Cx / Hx
Status: locally closed | reopened | blocked (with reason)
Changed owner and contract: <package + typed command/result/value>
Affected state machines: SM-...
Legal transition proved: <test name + assertion>
Forbidden transition proved: <test name + assertion>
Replay/restart/CAS proof: <test name + stable identity/fence>
Budget/cache proof: <test name + exact counter or C0-C2 hash>
Direct command and result: <command, exit status, date>
Integration exit still open: <next card / P5 fixture>
```

It is invalid to write `done`, `verified`, or `deployed` without all fields.
If the direct test exposes another owner's state or repository, the record must
be rejected and the scenario moved to C1 or P5.

## Component Work Packages

### P0 — `internal/pkg/workorchruntimecontract` and orchestrator event DTOs

**Responsibility.** Define owner-neutral persisted values; event packages carry
concrete acquisition/probe/receipt data but no business decision.

**Deliverables.** `RuntimeResourceDomain`, dependency, observation, resource
change, generic stale and invalidation identity; receipt validation; aggregate
normalization/hash; typed EventHub result fields for observations and stale.

**Must prove.** Normalization is deterministic; observation identity excludes
query time; duplicate change/stale gets one signature; a stale receipt cannot
also be changed/no-op/effectful; no `any`/map/JSON bridge is introduced.

**Current evidence.** The generic types, receipt fields, observation-aware
event DTOs and aggregate freshness hash are present. Remaining work is only
schema tightening after all durable owners report their final change set.

**Direct gate.**

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/pkg/events -count=1
```

### P1 — `kernel/orchestrator` reducer, controller, executor and drive

**Responsibility.** Be the sole freshness intersection and transition
authority. This package decides neither owner implementation nor user intent;
it only reduces persisted facts and submits typed owner commands.

**Inputs.** Acquired facts with observations, seal-time probe stale, exact
owner receipt, resource changes, current aggregate/CAS version.

**Outputs.** Pending/satisfied prerequisite set, invalidated candidate or
dispatch record, refresh ledger, fresh authority transaction, typed terminal
only when refresh budget is exhausted, and delivery requests.

**Required implementation checklist.**

- [ ] Acquisition persists fact, observation and satisfied prerequisite in one
  CAS; immutable facts explicitly carry no mutable dependency.
- [ ] Probe stale invalidates the unsealed attempt without sealing a dispatch,
  failure fact, repair directive or evidence.
- [ ] Execution stale persists the owner receipt/fence and invalidates the
  active dispatch without executing the effect or writing rejected evidence.
- [ ] Accepted receipt intersects `ResourceChanges` exactly, preserves already
  accepted evidence, invalidates only affected observations and reopens only
  affected coverage.
- [ ] A partial dispatch never reuses its remaining operations after a
  resource-changing accepted operation; it reacquires then produces a new
  transaction.
- [ ] Duplicate, late-old-transaction and CAS-reloaded delivery consume no
  second refresh budget; a late old stale is diagnostic only.
- [ ] Progress signature includes observation, invalidation, recovery and
  coverage-reacquire state, so restart cannot suppress required work.

**Current evidence.** Generic stale/change invalidation, exact intersection,
partial-dispatch coverage reopening and persisted dedupe are implemented.
Remaining P1 evidence is cross-process CAS and every owner-specific crash
fixture, not a second reducer path.

**Direct gate.**

```bash
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test -race ./internal/modules/kernel/orchestrator/... -count=1
```

### P2a — File owner

**Responsibility.** Own workspace manifest, target snapshot and File
admissibility observations, actual file effects and their receipt revisions.

**Contract.** Acquisition returns manifest/target/admissibility plus exact
`workspace_tree` and/or `workspace_target:<ref>` dependencies. Create, write,
edit and patch receipts report owner-observed target/tree changes when changed.
Probe sees target drift as generic stale, not infeasibility.

**Closure checklist.**

- [x] Each durable mutation variant supplies non-empty observed `AfterRevision`
  (`TestFileMutationReceiptObservationReportsAfterRevisionForAllStateChangingHandlers`).
- [x] Missing target and stale target are different typed outcomes
  (`TestRuntimeAdapterProbeRejectsMissingTextReplaceDespiteNewTargetSelector`,
  `TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation`).
- [x] External file mutation and receipt replay remain owner-local tests
  (`TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation`,
  `TestFileOwnerStateChangingReceiptReplaysWithoutSecondWrite`).
- [x] No-op and rejected mutation do not manufacture a resource change
  (`TestFileMutationReceiptObservationReportsAfterRevisionForAllStateChangingHandlers`).

**Direct gate.**

```bash
go test ./internal/modules/blocks/file/... -count=1
```

### P2b — Shell owner

**Responsibility.** Own shell policy/executable inventory, target validation
observation and managed validation execution; it does not own workspace truth.

**Contract.** Stable policy hash uses `owner_catalog`; each validation target
uses exact `workspace_target:<ref>` dependency. DNS/process/exit failures are
ordinary failures. Only a policy/target revision mismatch is generic stale.

**Closure checklist.**

- [x] Root directory mtime is absent from the revision algorithm
  (`TestRuntimeAdapterProbeDoesNotTreatWorkspaceDirectoryMTimeAsAdmissibilityStale`).
- [x] File creation does not stale Shell merely through root mtime; a declared
  validation target change does stale its exact observation
  (`TestRuntimeAdapterObservationTracksMissingValidationTargetForFileCreate`,
  `TestRuntimeAdapterProbeReturnsTypedStaleForObservedValidationTargetChange`).
- [x] Probe launches no process and cleanup remains idempotent
  (`TestRuntimeAdapterProbeRejectsMissingGoTestTargetWithoutStartingProcess`,
  managed-validation cleanup regressions).
- [x] A shell operation claiming durable effect reports bounded observed change,
  otherwise it is not automatically executable as durable work
  (`TestRuntimeAdapterRejectsStateChangingShellWithoutWorkspaceDelta`,
  `TestShellOwnerStateChangingReceiptReportsFileResourcesAndReplays`).

**Direct gate.**

```bash
go test ./internal/modules/blocks/shell/... -count=1
```

### P2c — Git owner

**Responsibility.** Own repository-state observation, Git admissibility and
sequence, execution fences and Git receipts.

**Contract.** Repository capability/admissibility share an exact
`repository_state:workspace_deliverable` observation. When acquisition uses
Git status or a workspace root to establish that state, it also declares the
File-owned `workspace_tree:workspace_deliverable` dependency. Init/add/commit
report the post-operation repository revision and every workspace-tree revision
actually observed. Repository drift is generic stale.

The capability-state sequence and the feasibility result used to seal an
operation must originate from the same acquired observation/snapshot. A reducer
must not combine a sequence from snapshot A with a feasibility answer from
snapshot B, even if both are individually fresh. After an accepted Git effect
invalidates that observation, the next candidate may bind only the newly
acquired sequence; it cannot replay an old required operation merely because a
previous candidate named it.

**Closure checklist.**

- [x] `init -> add -> commit` recomputes the required sequence after each
  accepted state change; old remaining operations are never copied forward
  (`TestRuntimeV2FileBackedGitCapabilityStateSequenceMatrix` asserts
  `init/add/commit -> add/commit -> commit` as separate candidate contracts).
- [x] `GitCapabilityState.RequiredOperationSequence` and the owner feasibility
  probe have the same opaque acquisition identity. A post-`add` candidate
  cannot be rejected from an operation shape belonging to an older snapshot
  (`TestGitOwnerCapabilityBoundAdmissibilityRejectsMismatchedProbeBindingAsStale`,
  `TestRuntimeV2FileBackedGitCapabilityStateSequenceMatrix`).
- [x] Receipt/query replay preserves execution scope and cannot run Git twice
  (`TestGitOwnerStateChangingReceiptReplaysWithoutSecondOperationAfterOwnerRecreation`).
- [x] External repository mutation, stale-before-effect and file-backed
  restart are direct fixtures (`TestRuntimeV2FileBackedGitStaleRefreshCrashRestartDoesNotReplayOldDispatch`
  and the Git restart fixture set).

**Implemented binding design.** Matching `ObservedRevision` alone remains
insufficient. Git now records a durable opaque `AcquisitionBinding` when it
accepts the capability-state receipt, copies it to admissibility and
feasibility, and includes it in stable identity/hash and the persisted
feasibility ledger. Orchestrator compares only this opaque value; it does not
synthesize a binding, interpret the Git sequence, or turn a mismatch into a
candidate failure. Git returns typed stale for a mismatch and normal P1
invalidation reacquires the facts.

**Direct gate.**

```bash
go test ./internal/modules/blocks/git/... -count=1
```

### P3a — MCP, SSH and Web owners

These are independent single-capability owners. Their common rule is to expose
only the revision they can actually observe; network/provider availability is
not freshness unless it proves an acquired revision changed.

| Component | Exact dependency | Stale condition | Receipt/change rule | Direct gate |
| --- | --- | --- | --- | --- |
| MCP | `owner_catalog` for server/policy; target only when operation declares it | catalog or confirmed remote revision drift | report `remote_resource` only when before/after revision is known | `go test ./internal/modules/blocks/mcp/... -count=1` |
| SSH | stable owner policy/config hash; future managed host uses exact host ref | policy/config drift only | timeout, DNS, auth remain ordinary failures; remote change only if owner observes revision | `go test ./internal/modules/blocks/ssh/... -count=1` |
| Web | provider config/catalog revision | configured provider revision drift | read-only request has no workspace change; durable remote mutation reports known remote revision only | `go test ./internal/modules/blocks/web/... -count=1` |

Each owner must test catalog/config drift, unrelated workspace mutation,
read-only probe behavior and receipt replay. It must not create a session,
execute a command or make a remote request merely to probe feasibility.

### P3b — LLM and Result owners

**LLM responsibility.** Consume a sealed candidate context and call the
selected provider. It cannot classify freshness or choose a fallback model.

- Render observations only in volatile C3-C5 context.
- Preserve C0 runtime/schema, C1 Goal/DoD and C2 declarative
  capability/governance bytes and `stablePrefixHash` over refresh.
- If the sealed provider/model target becomes unavailable, return typed
  prerequisite/governance failure; do not silently select another target.
- Do not issue a second same-transaction request after stale.

**Result responsibility.** Persist only the Orchestrator's final judgement.
No rejected Result is legal while prerequisite refresh remains recoverable;
duplicate terminal delivery is idempotent.

**Direct gates.**

```bash
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/result/... -count=1
```

### P4 — Run/Workspace delivery, application and presentation

**Run/Workspace responsibility.** The Run owner CAS-persists the aggregate and
terminal projection; Workspace persists active-run linkage. Neither selects an
action or makes freshness decisions.

**Application responsibility.** Build typed bootstrap and issue Start/Resume.
TUI workspace model edits apply only to future activation/run bootstrap; they
cannot patch the active Run's sealed provider/model target. Resume is the
single continuation vocabulary, whether the prior interruption was provider,
decision, restart or refresh related.

**Presentation responsibility.** Eventstream, workorchd read model, console and
TUI expose `prerequisite_refreshing` / `prerequisite_refresh_exhausted`, owner,
resource domain/ref, source and refresh used/limit. They must not render this
as candidate/provider retry or a replan/restart decision, and must not expose
a separate restart command.

**Closure checklist.**

- [ ] Active/blocked/terminal lifecycle remains authoritative after projection
  loss and projection replay.
- [ ] Refresh exhaustion has its own typed terminal code and only then allows
  rejected Result/Run/Workspace terminal delivery.
- [ ] API/read model/TUI tests show resume, not restart/replan, and distinguish
  stale from ordinary provider failure.
- [ ] UI model selection regression proves the next Run uses the saved target
  while the active Run remains sealed.

**Direct gates.**

```bash
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
```

## Supplementary User, Governance And Provider Work Packages

P0--P5 close the persisted freshness path.  The following packages close the
four user-visible symptoms that triggered this work without introducing a
second runtime state machine.  They are deliberately separate from owner
freshness adoption: each can be implemented and regression-tested locally,
but none may bypass P1 Orchestrator authority or claim the P5 integration
exit on its own.

### S1 — Decision runtime continuation and Approval evidence boundary

**Responsibility.** Decision owns Runtime V2 governance tickets, allow-lists,
resolution audit records and idempotency identities. It translates a user
intent into a ticket's persisted permitted resolution. Orchestrator alone
interprets that accepted resolution against the persisted Run and re-drives.
Approval owns approval state and audit evidence; it is not a Runtime V2 ticket
and cannot drive without a separately persisted aggregate-level binding.

| Input | Allowed output | Never allowed |
| --- | --- | --- |
| Decision ticket ID, user intent (`resume` or a replan-style intent), resolution idempotency key | persisted permitted resolution plus its bound runtime-governance identity; or typed conflict containing allowed actions/remediation | direct `ResumeRun`, aggregate mutation, provider/model replacement, V1 successor Run, inferred global allow-list |
| Approval ID and approve/reject intent | persisted approval state plus idempotent approval evidence | implicit ticket binding, scheduler Resume or direct runtime drive |

`replan` is accepted only as an ingress intent when the ticket's own
`AllowedRecoveryActions` explicitly maps it to a concrete persisted
resolution.  Otherwise the API returns a typed actionable conflict (including
the ticket and its allowed actions), rather than the raw
`resolution type ... is not allowed` validation message.  The original intent
is retained in the decision audit data; it is not a Runtime V2 transition.

**Local acceptance checklist.** Add/retain direct Decision tests for
ticket-specific allow-list lookup, normalization audit, unsupported-intent
conflict, duplicate HTTP/idempotency-key submission, restart after resolution
persistence, and exactly one bound Orchestrator continuation. Add Approval
owner/CLI tests that an approval transition/evidence is idempotent and sends no
scheduler continuation. Normal non-runtime decision/approval semantics must
remain unchanged.

**Direct gate.**

```bash
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

### S2 — Workspace preference and new-activation sealing

**Responsibility.** Workspace owns the mutable default model preference.
The activation builder owns choosing and sealing one provider/model target for
a *new* Run.  Run owns that sealed target thereafter.  TUI/Console only issue
the typed workspace update or typed new-activation request.

| Input | Allowed output | Never allowed |
| --- | --- | --- |
| workspace `/model` update | persisted `Workspace.DefaultLLMTarget`, visible as next-run default | mutation of an active/blocked Run's sealed target |
| explicit new-activation target, then workspace default, then system default | new `RuntimeActivationContext.ProviderTarget` and its selected catalog/config revision | resolving a target during candidate retry or Resume |
| `resume` for an existing Run | same aggregate and same sealed target | implicit provider/model failover or a new activation |

The read model must expose both values when they differ: the workspace's
next-run default and the active Run's sealed target.  If the latter is no
longer usable, LLM reports a typed failure for that exact target; policy may
govern the outcome, but no layer silently substitutes the newly selected
workspace default.

**Local acceptance checklist.** Test workspace update persistence, explicit
target/default/system-default precedence for a new activation, immutability of
an active Run after a workspace update, Resume retaining the old target, and
read-model/TUI distinction between next-run and current-run values.

**Direct gate.**

```bash
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/services/workorch/... -count=1
```

### S3 — LLM transport diagnostic and freshness separation

**Responsibility.** The LLM adapter owns one request to the sealed target and
its provider-facing transport diagnostic.  It consumes the P0/P1 prerequisite
contract but does not decide whether a fact is stale, spend a runtime budget,
or choose a retry/fallback.  This extends P3b; it does not create another
provider-control path.

| Observed outcome | Typed output | Orchestrator/presentation consequence |
| --- | --- | --- |
| deadline, connection failure or HTTP 5xx | provider transport failure with sealed target, endpoint class, phase, bounded cause and retryability | normal sealed failure/governance policy; show diagnosis; no freshness transition |
| invalid provider response/schema | candidate response failure with contract diagnostic | normal candidate handling for the same sealed transaction |
| observed provider catalog/config revision drift | exact prerequisite stale with expected/observed revision | P1 invalidates and reacquires; new transaction starts at candidate attempt 0 |
| sealed target absent after fresh acquisition | typed target-availability prerequisite/governance failure | no fallback provider/model |

The configured deadline is applied at the adapter boundary exactly once and
cancels the request context.  A client/proxy timeout with unknown server state
keeps its request correlation identity; neither UI refresh nor remote read
polling can manufacture success or submit a second operation.

**Local acceptance checklist.** Test context cancellation, status/error
mapping, target and correlation propagation through port DTOs, plain timeout
not becoming stale, confirmed revision drift becoming stale, C0--C2 stable
prefix preservation on refresh, and no duplicate request for one transaction.

**Direct gate.**

```bash
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
```

### S6 — Read projection and one-command user surface

**Responsibility.** Eventstream, workorchd read-model/remote mapping, TUI and
Console are projection consumers.  They display persisted facts and send only
typed Start, unified Resume, workspace preference update or ticket-resolution
intent.  They never determine which of those actions is legal.

| Projection input | Required display/route behavior | Forbidden behavior |
| --- | --- | --- |
| pending prerequisite or exhausted refresh | owner/domain/ref/source, expected/observed revision, used/limit and `prerequisite_refreshing` / `prerequisite_refresh_exhausted` | rendering it as candidate/provider retry or resolving it client-side |
| terminal-delivery pending | immutable terminal judgement and delivery phase | creating another Result or re-driving an owner operation |
| ordinary provider transport failure | sealed target, phase, bounded remediation/action | classifying it as stale or showing a fallback target |
| active/blocked recoverable Run | only `resume` continuation entry | separate runtime `restart` or `replan` command |
| workspace model preference differing from active target | next-run default and sealed current-run target | sending a model patch with Resume |

**Local acceptance checklist.** Use projection replay and remote-backend tests
to prove read paths do not publish runtime commands.  Add TUI/Console tests
for the single `resume` vocabulary, freshness versus provider-error rendering,
terminal-delivery state, and next-run versus sealed-target display.  Console
build is a compilation gate, not a substitute for those assertions.

**Direct gate.**

```bash
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
make console-build
```

## Required Scenario-to-Test Matrix

Every listed scenario needs a focused assertion, not merely a final success
assertion. `owner direct` means the owner package proves its own contract with
a fake local dependency; `aggregate` means Orchestrator fake owners; `file`
means file-backed persistence and restart.

| ID | Layer | Scenario | Required assertions |
| --- | --- | --- | --- |
| F-01 | contract | normalized observation/change/stale | stable identity/hash; invalid payload rejected |
| F-02 | aggregate | acquisition commit | fact + observation + satisfied prerequisite share one CAS |
| F-03 | aggregate | probe stale before seal | attempt invalidated; no attempt 1/failure/repair/evidence |
| F-04 | aggregate | stale receipt before effect | exact receipt/fence; no effect/evidence; dispatch invalidated |
| F-05 | aggregate | accepted effect change | accepted evidence retained; only intersecting facts go pending |
| F-06 | aggregate | duplicate/late stale/change | no extra revision, budget or invalidation |
| F-07 | aggregate | CAS conflict reload | persisted signature permits at most one budget use |
| F-08 | owner direct | File mutation/replay | exact target/tree change, no-op does not change |
| F-09 | owner direct | Shell policy and target | no root-mtime false stale; target drift is stale |
| F-10 | owner direct | Git sequence | fresh `init/add/commit` sequence and no replay |
| F-11 | owner direct | MCP/SSH/Web | catalog/config stale; transport failures remain ordinary |
| F-12 | LLM | refresh render | C0-C2/prefix stable; volatile context changes; no duplicate request |
| F-13 | Result | recoverable stale | no rejected Result before refresh exhaustion |
| F-14 | presentation | read/projection replay | display updates only; cannot drive or resolve |
| F-15 | file | File → Shell → Git positive path | candidate restarts at 0 on new transaction; final accepted |
| F-16 | file | six crash boundaries | effect and budget at most once; terminal delivery resumes |
| F-17 | deployed | fresh workspace | no `candidate_retry_exhausted` for stale; accepted terminal trace |

## P5 File-Backed And Deployed Runbook

The CI-safe fixture uses a mock LLM/provider and real file-backed File, Shell
and Git owners. Its fixed trace is:

```text
empty workspace
  -> acquire manifest/File/Shell/Git observations
  -> File creates main.go and reports target/tree change
  -> evidence retained; intersecting observations invalidated
  -> reacquire fresh facts
  -> Shell performs managed validation on the fresh target
  -> File edits target and refreshes again
  -> Git init/add/commit from fresh repository sequence
  -> validation + assessment -> accepted Result -> Run/Workspace projection
```

Restart the process exactly once at each boundary below, reload persisted state,
then re-drive. Assert the named effect is not duplicated and refresh budget is
charged once.

1. owner effect committed, before Orchestrator receives receipt;
2. receipt persisted, before evidence/invalidation CAS;
3. invalidation persisted, before prerequisite reacquisition;
4. reacquisition persisted, before the new candidate is sealed;
5. terminal persisted, before Result delivery;
6. Result delivered, before Run/Workspace projection.

Fresh deployed verification runs only after all local gates pass. Capture the
Run ID, aggregate invalidation records, observation IDs/revisions, candidate
transaction/attempt lineage, owner receipts, evidence and terminal projection.
The acceptance fails if any stale is classified as
`candidate_retry_exhausted`, an old sealed provider/model is patched, an
unrelated prerequisite refreshes, or a side effect is replayed.

### P5 positive-chain checkpoint — local gate restored (2026-07-20)

A bounded verbose rerun initially reached the Git phase with capability state
present but no Git `operation_admissibility` inventory. The root cause was the
file-backed fixture's sealed bootstrap: it produced `git/write` yet omitted the
Git owner-admissibility prerequisite. This was a bootstrap-contract fixture
drift, not a C1 persistence/merge/filter failure. Production activation already
derives one admissibility requirement per executable owner from the sealed
capability contract; the fixture now declares the same Git requirement.

`TestDriveRuntimeV2FileBackedFreshFileShellGitAcceptance` now passes and
asserts this persisted sequence after intersecting File effects:

```text
resource-change invalidation
  -> Git prerequisite pending
  -> new-focus requirement includes Git admissibility
  -> acquisition returns matching observation + inventory
  -> C1 commits both under the canonical prerequisite identity
  -> only then candidate feasibility/seal may use git/write
```

The Git inventory is acquired with the capability-state binding, invalidated by
the File tree change, reacquired under the canonical prerequisite identity and
used only by a fresh candidate at attempt 0. The fixture logs three Git state
transactions, each with attempt 0. This is not a test-only inventory insertion,
and it adds no owner/application/TUI retry.

The prior refresh-budget fix remains a separately proven narrow behavior: File
create, File edit, Git init and Git add can consume more than the historical
default of three refreshes; when the sealed budget is exhausted, the reducer
must prefer `prerequisite_refresh_exhausted` over pending reacquisition. The
positive fixture continues to seal budget 8 because it deliberately exercises
multiple durable effects. This fact does not establish the missing Git
inventory handoff.

The complete orchestrator package and `make
verify-runtime-v2-positive-convergence-file` pass after the correction. The
file-backed regression set contains the six listed restart-boundary fixtures;
the terminal boundaries are covered by
`TestDeliverRuntimeV2PrerequisiteRefreshExhaustedRestartsAfterTerminalPersistenceBeforeResult`
and
`TestDeliverRuntimeV2TerminalRestartsAfterResultPersistenceBeforeOwnerProjection`:
each recreates EventHub, Result owner and delivery Biz, reloads the persisted
Run aggregate, and proves a single Result followed by Run/Workspace projection.
The first test also proves the ordinary
`prerequisite_refresh_exhausted` terminal remains visible in Result closeout.

P5 local prerequisites now hold: canonical fixture observations, complete
bootstrap-derived owner inventories, full orchestrator package, positive chain,
boundary verifier and CI-safe gate. Remaining order is:

1. Run the repository release gate.
2. Deploy the resulting build and capture one new accepted-terminal trace with
   Run/workspace IDs, sealed target, observations, invalidations, receipt
   fences, authority transactions/attempts, budgets and terminal projection.

P5 remains **open for deployed evidence**; local gates do not substitute for a
fresh deployed trace.

## Completion Record Template

For each completed work package, append one concise record to the parent plan:

```text
Package: P<n>/<component>
Changed contract/owner: <typed DTO, aggregate field, owner adapter>
SM impact: <SM-1..SM-4>
Legal/forbidden transition proven: <test names>
Idempotency/restart/CAS proof: <test names>
Budget and cache proof: <test names>
Commands: <exact commands and result>
Remaining dependency: <P<n> or none>
```

A package is `done` only when its checklist and direct gate have passed. RPF is
complete only when P0-P5 pass, the parent plan's Definition Of Done is met, and
a fresh deployed run reaches accepted terminal.
