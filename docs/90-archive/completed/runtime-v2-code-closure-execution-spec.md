# Runtime V2 代码收口逐项实施规格

Status: completed

Type: implementation execution specification

Parent plan: [runtime-v2-prerequisite-freshness-closure.md](runtime-v2-prerequisite-freshness-closure.md)

Supporting documents: [component closure matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md), [system component supplement](runtime-v2-system-component-closure-supplement.md), and [residual-code runbook](runtime-v2-residual-code-closure-runbook.md)

Last Updated: 2026-07-20

## Purpose And Precedence

This is the single implementation queue for the reopened Runtime V2 work. It
turns the parent plan into independently deliverable code cards: each card
names the formal-state writer, typed boundary, affected state machines, local
regression, handoff gate and stop condition.

It introduces no new runtime semantics. In case of conflict, use this order:

1. [`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) for
   neutral semantics and SM-1--SM-4;
2. `internal/pkg/workorchruntimecontract` and
   `internal/modules/kernel/orchestrator/readme.md` for implementation
   contracts;
3. this specification for execution order and evidence;
4. the supporting documents for detailed fixtures and historical records.

Every card preserves these non-negotiable boundaries:

- Runtime V2 only: typed Start/Resume enters `DriveRuntimeV2`; V1 packets,
  graph dispatch and next-action paths must not return.
- Orchestrator is the sole authority for reducer action, freshness
  intersection, invalidation, budgets, candidate/dispatch lifecycle and
  governance continuation.
- Capability owners return a concrete observation, typed stale, normal failure
  or persisted fenced receipt. They do not choose retries or write Run/Result.
- Cross-owner interaction is typed EventHub only. No cross-owner repository,
  service facade, `map[string]any`, envelope or JSON compatibility bridge.
- EventHub Module/Block direction stays
  `module.go -> biz (embed basebiz.Base) -> service/adapter`; Base is plumbing
  only, never a policy, contract registry, repository facade or UI bridge.
- TUI, Console, remote client, Eventstream and projections are read/intent
  surfaces only. They do not drive, resolve, retry, create Result or rebuild
  authority.

## Completion Levels

| Level | Meaning | Required evidence | Does not prove |
| --- | --- | --- | --- |
| `implemented` | requested code and typed contract exist | focused unit test | a consumer can use it |
| `locally closed` | card legal/forbidden/replay/boundary tests pass | direct gate record | restart or deployment behavior |
| `integration closed` | named cross-owner file-backed handoff passes | handoff gate | user deployment acceptance |
| `system closed` | all dependent cards plus fresh accepted trace pass | complete evidence bundle | unrelated future deployment |

No card skips levels. A UI 2xx, Console build, repeated Resume, in-memory-only
fixture, or owner-local retry is never equivalent evidence.

## Fixed Authority And State-Machine Handoffs

```text
TUI / Console / HTTP
  typed Start | scoped Resume | decision-resolution intent | read query
                         |
                         v
SM-1 Run admission -- aggregate/load --> SM-4 Drive singleflight
                                         |
                                         v
                                SM-2 reducer authority
                                         |
                                         v
                                SM-3 seal / execute
                                         |
             typed EventHub acquire / probe / execute / receipt
                                         v
File Shell Git MCP SSH Web LLM ----------------> Result / projection
  owner facts only                               derived delivery only
```

| Machine | Formal writer | May do | Must not do |
| --- | --- | --- | --- |
| SM-1 | Run owner with Orchestrator narrow projection | Start/Resume admission and lifecycle persistence | choose RuntimeAction or patch active target |
| SM-2 | Orchestrator reducer | facts, focus, invalidation, governance and terminal judgement | execute owner operation |
| SM-3 | Orchestrator seal/executor | candidate attempt, seal, fence, receipt/evidence transaction | choose next action/retry policy |
| SM-4 | Orchestrator drive and owner projection writers | bounded re-drive and idempotent delivery | rebuild authority from projection |

| Input | Only legal path | Ledger allowed to change | Forbidden shortcut |
| --- | --- | --- | --- |
| prerequisite drift before effect | invalidated -> pending -> reacquire -> new transaction attempt 0 | prerequisite-refresh | candidate/repair/provider/recovery debit |
| accepted receipt changes resource | retain receipt/evidence; atomically invalidate exact observations | no debit for invalidation | reuse affected dispatch operations |
| sealed LLM deadline/connect/5xx | normal typed provider transport failure | provider transport only | stale classification or target fallback |
| allowed Decision resolution | durable Decision commit -> bound Orchestrator singleflight | governance delivery only | application/TUI second Resume |
| terminal delivery fault | retry only delivery leg | delivery only | new candidate/Result re-judgement |
| duplicate/late command/receipt | load identity/fence and return/join | none | second effect/request/debit |

The only runtime command exposed to users is `resume`. `restart` is historical
display text. `replan` is Decision ingress/audit intent; it is normalized by
the persisted ticket allow-list or rejected as a typed conflict containing
ticket, allowed actions and remediation.

## Final Closure Evidence

This ledger supersedes earlier open-card entries. A card is system-closed only
where its direct regressions, cross-owner gate and the relevant deployed
evidence have all passed.

| Card | Current evidence | Status | Next allowed action |
| --- | --- | --- | --- |
| E0 | Runtime V2 contracts and failure characterization are recorded in the parent completion record. | system closed | Reopen only for a newly reproducible earliest assertion. |
| E1 | Strict assessment DTO regression and managed LLM fixture pass. | system closed | Keep strict decoding. |
| E2-MCP / M2-H1 | Registry-to-activation metadata, exact current-slice mock response, MCP receipt and accepted Result pass in managed execution. | system closed | Retain owner-local and mock self-tests. |
| E3 | Owner observations/stale/receipts, generic invalidation, independent budgets and File→Shell→Git restart/crash paths pass. | system closed | Consume owner-authored facts only. |
| E4--E6 | Unified Resume, ticket-normalized replan ingress, sealed active target and read-only projection paths pass in the full package/release suite and CI-safe integration gate. | system closed | Preserve the one-command/read-only boundary. |
| E7 | Portable boundary verifier, truthful managed/release propagation and bounded HTTP verifier behavior pass. | system closed | Retain negative-path tests. |
| E8 | Fresh script-owned bounded fixtures reached `completed/accepted` after the receipt-only Git verifier correction; the file-backed restart matrix, CI-safe gate and release gate also pass. Artifact: `/tmp/workorch-e2e/runtime-v2-bounded-fixtures/20260720T015428Z/terminal-verification`. | system closed | Archive the completion evidence; a future failure reopens its earliest owner. |

The first managed MCP trace was correctly classified as an
**activation/capability-contract infeasibility**, not a provider failure,
prerequisite stale event or governance problem:

```text
focus-002 / produce_validation
accepted step kind: mcp/tool_call
selected capability: repo.summary (read_only)
sealed effect/idempotency: unknown / unknown
reducer disposition: need_governance / request_governance
```

That defect is now locally closed. In the newest retained M2 trace,
`focus-001` completes its assessment, `focus-002` seals and executes
`mcp-stdio-call`, and the owner commits its evidence with the exact selected
metadata. The next assertion is instead:

```text
candidate_retry_exhausted:
assessment item "cwi-001" is outside current slice coverage
```

This is a valuable strict-boundary success: the reducer and LLM candidate
decoder must continue to reject historical coverage. The fixture, rather than
the production authority, is emitting an invalid current-slice assessment.

### E2-MCP — Explicit read-only capability metadata and validation feasibility

**Earliest formal writer:** `blocks/mcpregistry` owns a capability's persisted
`effectScope` and `idempotencyClass`.  A capability creator/editor (including
the managed fixture through the capability HTTP command) supplies the values;
the registry persists them.  `activationports` copies and seals the selected
values into `RuntimeMCPCapabilityContext`.  It must never infer either value
from `riskLevel`, `repo.summary`, a server name, or an LLM proposal.

**Required typed handoff:**

```text
MCP capability command
  { id, serverId, kind=tool, effectScope=none,
    idempotencyClass=safe_to_replay }
  -> mcpregistry durable Capability
  -> selected capability read through typed EventHub port
  -> RuntimeMCPCapabilityContext (activation-sealed)
  -> FeasibleOperationKindsWithActivation
  -> validation candidate / mcp/tool_call / owner receipt
```

For the managed `repo.summary` fixture, `none` and `safe_to_replay` are an
explicit statement about the fixture tool: it only obtains a bounded summary,
does not mutate workspace or remote durable state, and duplicate invocation is
safe.  If a real tool cannot make both statements, it remains `unknown` (or
uses its actual durable/owner-managed values), is not admitted as this
validation operation, and the profile must select a separately declared
validation capability.  Declaring a read-only tool as `workspace_durable` to
escape the gate is forbidden.

| Boundary | Producer | Consumer | Required invariant | Forbidden substitute |
| --- | --- | --- | --- | --- |
| MCP capability command/model | registry HTTP/EventHub adapter -> `mcpregistry` Biz | registry repository | capability owner validates and persists explicit values unchanged | defaulting from `riskLevel=read_only` |
| selected capability mapping | registry typed read result | activation builder | copies exact persisted metadata before `RuntimeBootstrap` is sealed | Application changing an existing Run or filling values after Start |
| sealed runtime capability | activation builder | contract feasibility and candidate seal | `mcp/tool_call` is validation-feasible only when an enabled matching capability has an explicit non-durable scope compatible with `produce_validation` | generic kind-level MCP admission |
| execution receipt | MCP owner | Orchestrator via typed EventHub | expected/observed scope is the sealed scope; receipt is evidence only | MCP owner selecting retry, refresh, Result or next action |
| managed verifier assertion | fixture/read API/run snapshot | E8 acceptance | proves capability metadata, selected sealed metadata, `mcp/tool_call` evidence and completed Result | treating a `need_governance` Decision as successful validation |

**Legal transition:** an enabled selected tool with exact `none` +
`safe_to_replay`, a validation required-output contract and matching
`serverId`/`tool` parameters is feasible; candidate sealing accepts it; the MCP
owner executes it and emits validation evidence without a durable resource
change.

**Forbidden transitions:** missing/invalid metadata stays `unknown` and makes
the MCP kind infeasible; durable scope cannot satisfy validation; a capability
for a different server/tool cannot authorize the operation; `riskLevel` cannot
be used as an inference fallback; a duplicate command/receipt cannot cause a
second effect or budget debit; a transport/auth/stdio failure remains an
ordinary owner failure, never `RuntimePrerequisiteStale`.

**Local closure evidence (2026-07-20):** the fixture now sends explicit
metadata and asserts it survives the API read; registry create/update/read,
activation sealing, runtime-contract feasibility, MCP execution, activation
inventory classification and the Orchestrator EventHub candidate revalidation
all have focused regression coverage. In particular, a selected read-only MCP
tool is registered as a validation requirement when its sealed metadata is
compatible, while the candidate seal still checks its exact server/tool/scope.
The second validation must use `ValidateWithActivation`; re-validating the
returned candidate without the sealed activation is forbidden because it
silently turns the tool back into `unknown`/mutating.

**Required independent regressions before the managed rerun:**

1. `mcpregistry`: create/update/read round-trip preserves explicit effect and
   idempotency, while omitted values remain `unknown`.
2. `activationports`: selected capability produces a byte-identical sealed
   `RuntimeMCPCapabilityContext`; a later registry/workspace edit cannot alter
   the active Run.
3. `workorchruntimecontract`: `mcp/tool_call` is feasible for validation only
   with an exact enabled capability of non-durable compatible scope; test
   unknown, durable, disabled, wrong server, wrong tool and wrong action.
4. `blocks/mcp`: feasibility plus execution uses the sealed scope and
   idempotency, returns validation evidence for the read-only tool, and has no
   durable change; duplicate fence is idempotent.
5. `scripts/verify-mcp-stdio-workspace-execution.sh`: explicitly sends
   `effectScope: "none"` and `idempotencyClass: "safe_to_replay"`, then asserts
   the persisted/read selected capability fields before activation.  It must
   not rely on an implicit server-derived default.

**Direct and handoff gates:**

```bash
go test ./internal/modules/blocks/mcpregistry/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationports -count=1
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/mcp/... -count=1
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
WORKORCH_MANAGED_VERIFY_TARGET=verify-mcp-stdio \
./scripts/verify-workorch-managed-full.sh
```

**Handoff result:** the direct gates and the retained-artifact M2 command pass.
The MCP capability contract is integration-closed; retain its local tests as
M3/M4 guardrails.

### M2-H1 — Managed assessment current-slice isolation

**Scope and formal writer:**
`scripts/verify-mcp-stdio-workspace-execution.sh` owns only the deterministic
mock-provider response used by M2. Its `decision_payload` helper is the sole
writer of the mock assessment DTO. It owns no persisted Runtime state and does
not change an SM writer; it is a harness contract for the strict SM-2/SM-3
input boundary.

**Observed failure:** the helper currently uses a whole-prompt pattern such as
`re.findall(... cwi-...)`. A provider request includes C3 current focus and C4
history/evidence, so that scan returns both a completed focus's `cwi-001` and
the current focus's `cwi-002`. The mock then emits an `items[]` entry for the
former. The LLM boundary correctly rejects it because the persisted current
slice contains only `cwi-002`.

**Required harness contract:**

```text
rendered RuntimeCandidateContext
  -> final structured current Focus / RequiredOutput coverage slice
  -> mock assessment CoverageWorkItemIDs and items[] (exact same ordered set)
  -> strict LLM candidate boundary
  -> Orchestrator assessment reducer
```

| Aspect | Required behavior | Forbidden behavior |
| --- | --- | --- |
| coverage source | Decode the final structured current `Focus` or its `RequiredOutput` and take its declared coverage IDs only. Prefer the one whose focus ID matches the request's final focus ID. | Globally scanning all prompt text, evidence refs or history for `cwi-*`. |
| completeness/order | Emit every current coverage ID once, preserving the declared slice order; make one assessment item for each ID. | Adding a prior focus item, deduplicating by incidental global position, or substituting a default `cwi-001`. |
| missing/ambiguous input | Fail the mock request deterministically with a fixture-contract error and retain its prompt artifact. | Fabricating a satisfied assessment, silently using a historical slice, or weakening the production decoder. |
| evidence refs | Select only current-focus operation evidence; a satisfied item must retain a current-run reference. | Reusing historical evidence merely because it appears in the prompt. |
| production authority | Leave `RuntimeAssessmentCandidate` validation and reducer slice checks unchanged. | Accepting cross-slice IDs, filtering them in the reducer, or retrying through Decision/Resume. |

**Independent regression:** extract the coverage selection into a pure mock
helper and add a hermetic script-level self-test (for example,
`scripts/verify-mcp-stdio-workspace-execution.sh --self-test`) that supplies
captured prompt fragments. It must prove all of the following without a
running WorkOrch service:

1. a prompt containing historical `cwi-001` and current `focus-002` /
   `cwi-002` produces only `cwi-002` in both coverage fields;
2. a current slice with several IDs preserves its exact ordered membership in
   `coverageWorkItemIds` and `items[]`;
3. no final structured slice, conflicting final focus, or no current evidence
   returns a fixture-contract error rather than a synthetic satisfied result;
4. the existing Go LLM boundary regression continues to reject a deliberately
   injected out-of-slice item.

**Handoff evidence (2026-07-20):** the self-test, script syntax check and LLM
strict-boundary package pass, followed by a retained-artifact M2 run whose
workspace is `ws-9370e08122ab50ee` and Run reaches `completed`. Keep the same
commands as the regression gate:

```bash
scripts/verify-mcp-stdio-workspace-execution.sh --self-test
bash -n scripts/verify-mcp-stdio-workspace-execution.sh
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
WORKORCH_MANAGED_VERIFY_TARGET=verify-mcp-stdio \
./scripts/verify-workorch-managed-full.sh
```

**Stop condition:** do not modify E3 reducer logic, Decision allow-lists,
Resume, provider/model selection, File owner admissibility or the Console to
make M2 pass. A resulting provider/transport failure is routed to E2-LLM/MCP
ordinary-failure coverage, not freshness or governance. Only after M2 reaches
a completed Result may M3 become the next managed card.

## Ordered Delivery Cards

### E0 — Contract baseline and evidence record

**Boundary:** documentation, DTO characterization and verifier fixtures; no
production behavior mutation.

**Do:** record each fresh workspace/run, verifier target, exact failure and
earliest responsible card. Turn a newly observed deployed symptom into a
reproducible direct assertion before another deployment retry.

**Gate:**

```bash
git diff --check
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/pkg/events -count=1
```

**Stop:** a new cross-owner field first belongs to its DTO owner; consumers may
not independently add a compatibility field.

### E1 — Managed LLM assessment fixture/schema alignment

**Boundary:** `scripts/verify-llm-workspace-execution.sh` and LLM candidate
schema regression. It is a harness card; schema ownership remains
`blocks/llm/biz/runtimev2`.

**Required output for satisfied coverage:**

```json
{
  "focusId": "...",
  "coverageWorkItemIds": ["..."],
  "disposition": "satisfied",
  "items": [{
    "coverageWorkItemId": "...",
    "satisfied": true,
    "evidenceRefs": ["current-run evidence ref"]
  }],
  "summary": "...",
  "evidenceRefs": ["current-run evidence ref"]
}
```

Construct items from the full requested coverage slice, unless a test is
deliberately exercising partial assessment. Negative/partial cases retain
explicit disposition and per-item truth values; they never restore a
top-level boolean.

**SM impact:** no production transition; validates SM-2 assessment input and
SM-3 candidate schema at the harness boundary.

**Must prove:** valid satisfied/partial/negative forms; current-run evidence
for satisfied item; unknown top-level fields still rejected; no extra provider
request repairs schema drift.

**Gate:**

```bash
go test ./internal/modules/blocks/llm/... -count=1
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
WORKORCH_MANAGED_VERIFY_TARGET=verify-llm \
./scripts/verify-workorch-managed-full.sh
```

**Handoff:** E8. A true assessment/reducer invariant failure reopens E3, not
the mock workaround.

### E2 — Owner-neutral freshness and capability owners

**Owners:** `workorchruntimecontract`, Orchestrator event DTOs, File, Shell,
Git, MCP, SSH and Web. Deliver each owner alone using the shared contract.

| Owner | Sole responsibility | Output | Must not do | Direct gate |
| --- | --- | --- | --- | --- |
| contract/events | identity, dependency, observation/change/stale validation | normalized values and concrete results | interpret resource/select action | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/pkg/events -count=1` |
| File | manifest/target/admissibility and workspace effect | exact tree/target observation, stale or fenced change receipt | refresh/candidate decision | `go test ./internal/modules/blocks/file/... -count=1` |
| Shell | policy/catalog, validation target and managed validation | exact policy/target observation, stale or bounded receipt | root-mtime freshness/process in probe | `go test ./internal/modules/blocks/shell/... -count=1` |
| Git | snapshot, admissibility/sequence and Git receipt | one opaque acquisition binding | reuse old sequence after effect | `go test ./internal/modules/blocks/git/... -count=1` |
| MCP/SSH/Web | observable catalog/config/policy and known remote revision | revision stale only when observed | classify timeout/auth/DNS as stale | `go test ./internal/modules/blocks/<owner>/... -count=1` |

For every owner, test exact identity, stale-before-effect with no change,
accepted observed change, unrelated change, duplicate fence, external
mutation, and read-only probe. Tests use its typed public adapter, never
another owner's repository.

**SM impact:** owner facts feed SM-2/SM-3 through SM-4 only. **Handoff:** E3.

### E3 — Orchestrator/Run aggregate convergence authority

**Owners:** `kernel/orchestrator/biz/runtimev2`, drive/EventHub adapters and
`kernel/run` CAS persistence.

**Input -> output:** owner fact/stale/receipt plus aggregate version -> one
aggregate CAS containing observations, exact invalidations, refresh ledger,
candidate/dispatch disposition and reducer-visible facts. Run owns persistence;
Orchestrator owns the decision.

**Must prove:** atomic acquisition; pre-seal stale invalidates without
candidate/repair/evidence debit; execution stale saves exact fence then
invalidates before effect/evidence; accepted changes preserve evidence but
invalidate only intersections; refresh keeps sealed model and restarts at
attempt 0; duplicate/late/CAS reload is free; exhaustion is only
`prerequisite_refresh_exhausted`; restart loads aggregate only.

**Regression:** legal/forbidden transition, one-to-many/unrelated change,
stale cannot seal, partial dispatch cannot reuse operations, duplicate/late,
CAS, every crash point, separate budgets, immutable terminal delivery.

```bash
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test -race ./internal/modules/kernel/orchestrator/... -count=1
```

**Stop:** no owner `Refresh*` escape hatch or application continuation queue.
New prerequisite kinds start in E2.

### E4 — Governance ingress and unified Resume

**Owners:** Decision, Approval, Orchestrator governance adapter and workorchd
ingress. Ticket-scoped resolution intent becomes Decision's durable,
idempotent normalized result or `RuntimeDecisionActionConflict`; only the
bound Orchestrator continuation joins singleflight. Approval writes evidence,
not continuation.

**Must prove:** legal replan-style intent; unsupported replan returns typed 409
with zero Drive/Resume traffic; duplicate/restart has one ResolutionID and one
continuation; late resolution is diagnostic; generic approval cannot fabricate
ticket binding.

**SM:** SM-1 blocker, SM-2 governance fact, SM-3 binding/fence, SM-4 delivery.

```bash
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

### E5 — Workspace preference and sealed provider/model target

| Value | Writer/lifetime | Rule |
| --- | --- | --- |
| workspace default target | Workspace owner, mutable | TUI `/model` affects only next Run |
| activation target | activation builder/new Run, immutable | seal explicit -> workspace -> system before Start |
| provider ceiling observation | LLM prerequisite on sealed Run | refresh observes availability; never migrates target |

Prove active aggregate/ceiling is byte-identical after workspace model update;
Resume rejects override; next Run uses preference; unavailable sealed target
has no fallback; timeout is transport failure, not stale. SM impact is SM-1
new admission and SM-4 Start only.

```bash
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
```

### E6 — Result, projection and read surfaces

**Owners:** Result, Run/Workspace delivery, Eventstream, workorchd DTO,
remote backend, TUI and Console. Judged fact -> idempotent Result/projection/
descriptive DTO only.

Prove recoverable stale has no Result; duplicate terminal creates one Result;
read refresh/replay has zero Drive/governance/Result/operation counters; only
scoped resume, ticket intent and workspace preference reach ingress; restart
and Runtime replan fail before HTTP/EventHub traffic.

```bash
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/biz/backends -count=1
go test ./internal/services/workorch/service/tui -count=1
cd web/workorch-console && npm run build
```

### E7 — Boundary/release truthfulness

**Owners:** verifier and release scripts; no runtime-policy write. Prove
verifier works without `rg`, rejects Hub topology/DTO violations, release stops
at first failed cross-build and never prints false success, and HTTP verifier
uses bounded connect/total timeouts.

```bash
make verify-boundary-contract
make release-check
bash -n scripts/verify-workorch-http.sh
git diff --check
```

### E8 — File-backed restart, managed verification and deployed trace

**Fixed chain:** empty workspace -> acquire File/Shell/Git -> File create and
tree/target receipt -> exact invalidation/reacquire -> Shell validation -> File
edit/refresh -> fresh Git init/add/commit sequence -> validation/assessment ->
accepted Result -> Run/Workspace projection.

**Managed sub-gate order:** run each target against a newly created workspace
and keep its artifacts.  A later target never masks an earlier one:

| Order | Target | Isolates | Required accepted evidence |
| --- | --- | --- | --- |
| M1 | `verify-llm` | strict candidate/assessment wire DTO and provider request assembly | item-level assessment evidence and accepted terminal |
| M2 | `verify-mcp-stdio` | selected MCP metadata -> activation seal -> validation feasibility -> stdio receipt | exact `none`/`safe_to_replay` sealed values, one MCP validation evidence reference and completed Result |
| M3 | `verify-ci-mvp-file` | CI-safe File/Shell/Git/MCP fixture integration | every declared capability has an acquired admissibility observation and no forbidden owner bypass |
| M4 | `verify-workorch-managed-full.sh` default Runtime V2 queue | strict M1 -> M2 -> M3 order, retained artifact and non-zero propagation | no false success; first failed V2 target and its persisted Run identity are reported |

M1--M4 are green. M4's retained artifact bundle at
`/tmp/tmp.CHS1AjeNpA` shows the exact M1 -> M2 -> M3 queue on one managed
service; the nested M3 MCP run completes after M2 seals a 16-item (still
bounded) context budget. `verify-e2e-full` is a separately invoked historical
aggregate; it contains retired V1 mock protocols such as
`completionState`/`artifactPlan` and is not M4 evidence. Do not make a later
target pass by deleting an earlier target from M4, lowering the profile evidence
contract, accepting `need_governance` as terminal success, filtering an invalid
item in the reducer, or adding a Decision/Resume retry.

For the bounded Git fixture, live acceptance counts only owner receipts whose
`outcome=accepted`. The ledger also retains unexecuted operations from
dispatches invalidated after an earlier durable Git effect; those are
diagnostic lineage, not repeated side effects. The accepted chain must be
ordered `init`, `add`, `commit`, contain each variant once, retain its
required-operation reference, and use a distinct non-empty
`capabilityStateHash` after each durable effect. This is the deployed form of
the RPF Git contract: `init/add/commit -> add/commit -> commit` comes from
three fresh sealed transactions, not one reusable snapshot. The hermetic guard
is `scripts/verify-runtime-v2-positive-convergence-live.sh --self-test`.

Restart once at: owner effect before receipt delivery; receipt before
evidence/invalidation CAS; invalidation before acquisition; acquisition before
new seal; terminal before Result; Result before projection. Each proves no
repeated effect/result and no duplicate refresh debit.

```bash
make verify-runtime-v2-positive-convergence-file
make verify-ci-mvp-file
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
./scripts/verify-workorch-managed-full.sh
```

Only after all gates pass deploy the resulting build and make a new workspace
and Run. Record IDs; sealed target/revision; observations/revisions;
invalidations; execution keys/fences/receipts; authority transaction/attempt;
all budgets; accepted Result; and Run/Workspace projection. An unreachable
service is failed evidence, not a successful trace.

## Mandatory Per-Card Record

```text
Card: E<n>/<component>
Status: implemented | locally closed | integration closed | reopened | system closed
Changed writer and typed boundary: <paths; concrete command/result/value>
Affected SMs: <SM-1..SM-4, or none for fixture/gate>
Legal transition proof: <test and assertion>
Forbidden transition proof: <test and assertion>
Duplicate/restart/CAS proof: <identity/fence and test>
Budget/cache proof: <ledger/stable-prefix test, or N/A>
Direct gate: <command + exit code + date>
Handoff gate: <command + exit code + date>
Remaining dependency: <next E-card/deployed trace>
```

For EventHub components also record Biz embedding location, command/result
owner, scope fields, sync/async delivery and proof that Post handlers do not
use `event.Result`.

## Final Exit

The parent work is system-closed. The final deployed trace uses fresh,
script-owned workspaces `ws-f85931b73f8d7d7b` and
`ws-3d999d6a994e6c87`; both reached `completed/accepted`, released their
active Run, and left the global projection ready and aligned. The fixture
cleanup returned the deployed instance to its original single workspace.
M1--M4, the six restart boundaries, `make verify-ci-mvp-file` and
`make release-check` provide the matching local and CI evidence. Future
failures must reopen the earliest owner/card rather than add an entry-layer
retry.
