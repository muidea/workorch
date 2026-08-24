# Activation And Definition Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-08-24

## Purpose

本文描述 formal Workspace/Goal/DoD 如何形成 typed `RuntimeBootstrap`，以及 Run owner/Orchestrator 如何初始化并首次
drive。它绑定 SM-1、SM-2 与 SM-4，但不另立状态迁移。

## Entry Flow

```text
fresh Start/Restart authoring submits DefinitionAdmissionCandidate
  -> Workspace owner normalizes and CAS-persists immutable candidate
  -> Workspace pure admission compiler checks complete all-of semantics, exact Profile proof closure and typed owner-fact closure
       Definition invalid/ambiguous -> Decision owner rewrite_definition_all_of blocking ticket
       Profile proof/prerequisite invalid -> Decision owner select_or_fix_template_profile blocking ticket
       either failure -> no formal Definition, Run, provider request or capability work
       admitted          -> Workspace owner promotes formal Definition + StartupInputContract
       corrected ticket  -> promotion remains pending until Decision resolution acknowledgement
  -> user explicitly submits fresh Start/Restart execution intent
  -> Workspace owner verifies activation admission is eligible
  -> workorchd reads canonical Workspace / effective Goal / effective DoD
  -> reads and validates referenced Soul / Skill / MCP / Memory + provider selection
  -> compiles typed RuntimeBootstrap for a fresh activation when required
  -> Workspace owner validates definition/scope and forwards typed immutable intent
  -> Run owner chooses owner-local SM-1 path
       no current Run/fresh start -> create + seal bootstrap + activate
       resume                 -> resume same eligible Run; ignore fresh replacement path
       restart                -> persist predecessor ReplacementIntent
  -> RunStateChanged / inventory fact
  -> Orchestrator InitializeRuntimeFromRun + DriveRuntime
```

An existing-Run Resume enters at the execution-intent step, reuses the startup input sealed with that Run and ignores a newer
candidate/promotion. It cannot absorb a new Definition revision or use pending new input as a patch.

The public request does not choose the target Run or inject activation context. Workspace and Run owners resolve the current
lineage under their own state. Every Start/Restart ingress rejects missing, pending or mismatched activation admission with
typed `definition_clarification_pending`; a formal Definition alone is not execution eligibility.

## Definition Resolution

A fresh bootstrap requires an admitted and activation-eligible startup input containing:

- one Workspace and its resolved project workspace directory plus internal boundary;
- exact Goal and DoD item set with stable definition revision/lineage and `StartupInputHash`;
- exact Workspace Template snapshot and any independently bound Profile/policy snapshot;
- candidate-bound exact normalized typed Profile contract snapshot plus the matching startup-input Profile hash; every required
  Profile acceptance/deliverable CWI has an exact evidence requirement, nonempty supported evidence kinds and at least one typed
  owner-receipt proof operation;
- every Profile-required owner fact has a preexisting or unique acquisition/reobserve declaration, and the candidate manifest equals
  the manifest independently derived from the exact Profile snapshot;
- Workspace canonical provider selection and referenced Soul/Skill/MCP/Memory resources;
- template/profile/capability/evidence policy declarations;
- bounded convergence/recovery policy.

Missing, pending, cross-scoped or non-`all_of` definitions fail before Run creation. Definition identity/revision is sealed with the Run and must
remain equal across bootstrap, facts, authority transaction and candidate. A caller cannot rebuild it from a projection or
map during Resume.

Profile wildcard evidence declarations remain policy for Definition-derived rows only; they cannot close an exact Profile-owned required
CWI. A proof-incomplete Profile never reaches bootstrap compilation. The Runtime compiler repeats the Profile validation only as a
fail-closed SM-2 input defense if an internal caller bypasses normal admission; it does not open a second ticket or become a state owner.

## Typed RuntimeBootstrap

`RuntimeBootstrap` contains typed Definition, Activation, Definition seed, initial focus/slice/coverage/requirement inputs and
bounded convergence/recovery policy. Important boundaries are:

- `RuntimeDefinitionContext`: immutable formal definition identity and change impact;
- `RuntimeActivationContext`: sealed provider selection, dialog language, frozen Soul/Skill/MCP/Memory content, evidence
  policy, execution contract and bounded continuation history;
- `RuntimeDefinitionSeed`: semantic-planning input and stable declaration identities; it carries no owner feasibility or
  `CoverageEvidenceBinding` authority;
- initial coverage/focus data: activation baseline only; later semantic plan/feasibility/reconciliation may seal or reopen the
  generation-specific binding queue only through SM-2 private internal events.

The bootstrap must pass `Validate()` before persistence. It cannot contain credentials/provider client, live repository,
EventHub, HTTP DTO, `map[string]any` authority bridge or execution-context patch.

## Semantic Plan And Binding Seal

For a fresh definition generation:

1. SM-2 records `semantic_contract_required` from the sealed bootstrap;
2. SM-3 requests at most the bounded semantic-plan document from LLM owner;
3. capability owners return current typed feasibility/target observations;
4. Orchestrator private internal event validates and seals `CoverageEvidenceBinding` plus reconciliation contract;
5. only then may the candidate/sealed-dispatch path begin.

LLM proposes; capability owners report feasibility; neither may change CWI/focus state. Application cannot pre-seal proof
bindings in the bootstrap.

## InitializeRuntimeFromRun

`InitializeRuntimeFromRun` is idempotent:

- no Runtime record: validate sealed bootstrap and create initial aggregate/facts;
- existing record: strictly decode and validate the one current shape, then load it without duplicating reducer transition,
  candidate attempt or evidence;
- missing/invalid bootstrap: fail closed through typed terminal handling;
- invalid/retired aggregate data: repository load fails before Runtime initialization, with no owner transition or
  provider/capability work.

Initialize does not call LLM, execute a capability, infer action from Step/checkpoint, or rebuild definition from display data.

## Resume

Resume carries only workspace/run scope inside the owner contract. Run owner validates SM-1 eligibility and resolved blocker,
then publishes an immutable state fact. Orchestrator loads the same aggregate and re-drives it.

Resume does not:

- recompile or replace bootstrap;
- re-read Workspace provider/Soul/capability selection;
- change Goal/DoD revision;
- add a candidate attempt because of restart;
- accept `ExecutionContextPatch` or any target lifecycle state.

## Restart And Successor

Restart is a fresh activation request but not immediate application-created Run creation:

1. workorchd compiles fresh bootstrap from current Workspace state;
2. Workspace owner validates activation eligibility and forwards the restart intent;
3. Run owner CAS-persists immutable `ReplacementIntent` on the current predecessor;
4. predecessor converges to sealed superseded terminal and completes `Result -> Workspace -> Run` delivery;
5. Run-private reducer creates, links and activates one successor using the sealed fresh bootstrap;
6. successor independently initializes and drives.

Application, Workspace, Orchestrator and projection cannot create/link the successor. Duplicate request/delivery must resolve
to the same lineage. A predecessor that cannot be read as the current shape never enters this flow and creates no intent or successor.

## Invariants

- all formal definitions originate from Workspace owner state;
- Run owner alone chooses SM-1 create/resume/replacement transitions;
- Orchestrator alone mutates SM-2/SM-3/SM-4 aggregate semantics;
- fresh activation and Resume cannot be confused by a shared context patch;
- no provider/capability call occurs before a valid current-baseline aggregate and sealed prerequisite/candidate contract;
- repeated notification, initialization and drive are idempotent.

## Related Documents

- [Runtime Flow Overview](../runtime/flows/overview.md)
- [Planning And Execution](../runtime/flows/planning-execution.md)
- [Repair And Restart](../runtime/flows/repair-and-restart.md)
- [Runtime Stage Contract](../runtime-stage-contract.md)
- [Definition Admission Contract](definition-admission.md)
- [Canonical Definition Model](canonical-definition.md)
- [Workspace Definition Clarification](clarification.md)
- [Orchestrator README](../../../internal/modules/kernel/orchestrator/readme.md)
