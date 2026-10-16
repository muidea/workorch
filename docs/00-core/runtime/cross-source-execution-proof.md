# Cross-Source Execution Proof

Status: normative

`runtime/cross-source-execution-proof/v1` is an opt-in source-mapping contract. It permits an execution lineage guard to
consume the uniquely named original DoD CWI's completed work rounds or repair followed by fresh validation. It neither
redirects ordinary behavioral evidence nor substitutes lineage for the consumer's original acceptance criterion.

## Confirmed source authority

A Profile rule declares `ExecutionSourceContractRef` only for execution-phase `completed_work_rounds` or
`repair_then_fresh_validation`. Other kinds, phases and contract versions are rejected. The Workspace specification seals
`ContractRef`, `ConsumerItemRef`, `RuleRef` and `SourceItemRef` for every opted-in rule. Both endpoints must identify different
original DoD candidate sources; the mapping is unique per consumer/rule and bounded to 512 entries. Unknown, self-referencing,
duplicate, missing or undeclared mappings block admission. Runtime never guesses an endpoint from prose, order or another CWI.

The complete mapping participates in specification review, exact user confirmation, candidate identity and the immutable
source-obligation inventory. Changing it requires renewed review and confirmation; recomputing its hash does not confer
confirmation. Original Goal/DoD and obligation text remain intact. This contract does not create an independent Goal CWI.
The authoring wire contains an optional versioned mapping array; all fields within an entry remain required and exact.
The Web confirmation view displays the original consumer/rule/source/version without automatically confirming it.

Application compiles the exact original source CWI as a prerequisite of its consumer. It cannot manufacture missing coverage
endpoints, and the existing all-of dependency validator rejects cycles. Bootstrap and fresh facts independently reject removal
of this predecessor even when a caller coordinates seed/hash changes. An absent extension preserves legacy JSON/hash;
an opted-in rule without a confirmed map never falls back to same-CWI consumption.

## Runtime consumption

### Confirmed Goal/root source domain

`runtime/goal-source-execution-proof/v2` is a separate opt-in domain, not a relaxation of v1. Original `goal` or
`goalCoverage` Profile work/repair rules may name the original Goal candidate as consumer and one original DoD candidate
as execution source. All mapped Goal rules must name that same unique source. The entry additionally requires
`ConsumerGoalText`, exactly the complete original Goal bytes (at most 4096 bytes, the ordinary source-check envelope).
Workspace checks the original input preimage, not a mapping-owned copy; the field and original rule/ref participate in all
six reviews, specification/candidate hashes, exact confirmation and immutable inventory. Missing, ambiguous, oversized,
unreviewed, undeclared or mismatched source/text blocks before activation. V1 rejects even an empty Goal wire extension.

There is no independent Goal work CWI. The explicitly selected original DoD's ordinary acceptance is the consumption
position: its checks retain every original DoD/source obligation AND one complete ordinary Goal criterion, while its
`GoalExecutionAssertions` retain the original Goal scope/ID/full-text SHA256 and exact Profile rule. Several Goal rules
share the one ordinary Goal check, not several fabricated work units. The source must declare ordinary managed validation;
a state-change-only historical source cannot be selected. No predecessor/self dependency or work purpose is invented.
Other original obligations remain mandatory all-of; this mapping neither transfers ordinary evidence nor supplies work credit.

Definition seed, validation requirement, semantic manifest and ordinary binding seal the projection. Bootstrap/fresh-load
independently reconstruct it from the original inventory and Profile, rejecting stripped ordinary Goal checks, omitted guards,
misplaced consumers and coordinated self-hashes. Composite review, when explicitly selected, preserves the full original Goal
check and the typed consumers. SM-2 only consumes after ordinary proof: exact current ordinary source binding and original
accepted semantic batch must include the same Goal assertion, then the existing work/repair consumers reverse-check actual
source dispatches/receipts and evidence cutoff. Goal acquires no synthetic work rounds. Missing/stale/currentness failures
remain blocked or follow existing exact reopen; old archives are neither upgraded nor given old credit.

SM-2 assessment retains ordinary consumer proof AND the execution guard. The typed guard reverse-checks the confirmed
Definition/startup/revision and complete source inventory against the Definition seed, finds the unique original source text,
and resolves its exact current binding reference from the original accepted semantic batch. Manifest, CWI, binding ID/hash,
focus and canonical ordinary Definition declaration must all match. Missing, retired or repair-purpose bindings cannot stand
in for the original ordinary source binding. The original consumer criterion and ordinary validation remain mandatory.

Work-round and repair consumers still verify original dispatches, owner receipts, evidence sequence and the assessment cutoff;
the source mapping itself is not a success credential. Future evidence cannot satisfy an earlier cutoff. Recovery repeats the
same source checks against the persisted aggregate and never upgrades old archives or reassigns mappings at runtime.

SM-3 retains the existing semantic/candidate/dispatch transactions. SM-4 only re-drives and delivers already-judged materials.
SM-1 and all owner boundaries remain unchanged. There is no new state machine, consumption ledger, budget, cross-owner writer,
terminal alternative or permission to restore stale evidence credit. Source mappings and their reviews do not by themselves
enable ImplementationWork availability or alter the built-in default policy.

## Verification boundary

`runtime_execution_proof_mapping_test.go` in the shared contract verifies exact accepted-batch source resolution, invalid and
ambiguous maps, source/document/binding drift, detached results, reload, prerequisite compilation and cycles. Workspace
`definitionadmission/execution_proof_mapping_test.go` verifies original source admission and confirmed inventory compilation.
Application `activationcore/runtime_execution_proof_mapping_test.go` verifies bootstrap, fresh-facts reload and coordinated
predecessor stripping. Authoring decoder/schema and Web component tests verify strict optional wire and visible confirmation.
These checks do not prove public activation, actual two-round/repair cross-CWI fullflow or live provider acceptance.

Goal-domain checks are in `runtime_goal_execution_proof_test.go`, Workspace `goal_execution_proof_mapping_test.go` and
`goal_execution_proof_wire_test.go`, Application `runtime_goal_execution_mapping_test.go`, and Web authoring component tests.
`runtime_goal_execution_source_mainline_test.go` exercises a private admitted startup with real File/Shell/Run/Result owners
and production repair/ordinary acceptance/delivery plus persisted CAS-cut reload. Private startup/scripted provider tests
do not establish public activation, real-model acceptance, live availability or independent process recovery.
