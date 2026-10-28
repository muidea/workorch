# Terminal Material Proof

Status: normative

`runtime/terminal-material-proof/v1` is an opt-in, closed proof-consumer contract. It applies only to newly sealed Profile
rules `terminal_result_evidence_index` and `terminal_owner_operation_receipt`, each declaring the exact `TerminalContractRef`
and `ConsumptionPoint=post_terminal_delivery`. Missing/unknown versions, mixed union fields, unsupported owner/kind/variant/
effect inventories and missing/ambiguous receipt-source selectors are rejected, not upgraded or inferred.
Absent extensions preserve original rule JSON/hash identities. Recovery never adds rules to an old activation.

## Review and ownership

V3 input review preserves the entire original criterion and source declaration. Its `proofConsumerRefs` map obligations to
the same-CWI sealed rule references; a terminal receipt rule can select a different original source only through its sealed
`ReceiptSourceScope + ReceiptSourceID`. Selectors are exact and resolve to one original Definition CWI, not provider-proposed
IDs, wildcard matches or first-match projections. A Result-index declaration requires original DoD inventory.

Review seals responsibilities, never claims future receipts, Result records or ACKs already exist. Ordinary all-of validation,
assessment and execution-cutoff guards remain mandatory. Terminal consumers do not grant SM-2 credit, change the original
criterion, execute an operation, create a second acceptance ledger or rewrite a sealed terminal. SM-1 remains Run-owned;
SM-2/SM-3 retain their existing transactions. SM-4 uses the existing material-seal event and ordered delivery.
Without snapshot rules the event remains scope-only; V2 permits only bounded typed snapshot observations, never caller material.

## Material and actual delivery

For a completed outcome, SM-4 checks material obligations against independently loaded original authority before its first
immutable terminal-material CAS and before Result I/O. The seal and subsequent fresh loads repeat the same source check.
An invalid success material fails closed; it cannot be delivered as acceptance, changed into failure, repaired by rewriting
the terminal, or treated as an instruction to run another operation. Existing bounded delivery recovery is unchanged.

`terminal_result_evidence_index` checks every original DoD CWI and each original source-obligation row: exact source/text,
current covered/satisfied outcome, nonempty accepted refs, unique resolution to current accepted proof, and inclusion in the
formal top-level index. The user-facing DoDAssessment and summary must have exactly one matching source-note row with the
same original text and evidence refs. The normal aggregate gate independently validates the original receipt/admission chain;
the index consumer is a material/proof projection, not an owner-currentness observation.

`terminal_owner_operation_receipt` checks the exact selected original source's current accepted terminal proof, committed
ordinary dispatch, canonical owner/kind/variant/effect, original execution key/fence, unique receipt/hash, receipt-derived
evidence/hash, processed-event fence and global evidence chronology. Both the evidence ID and original receipt ref must be
present in the immutable material index. Owner-committed without evidence commit, implementation-work effects, stale proofs,
rejected/no-op durable receipts, unrelated CWI and future facts cannot satisfy it. This contract proves an original operation
receipt; it does **not** prove final repository HEAD/tree/blob bytes or the latest owner/resource revision. A final Git snapshot
needs its own actual owner-produced and consumed proof, not a reinterpretation of this receipt kind.

A V1/V2 Git input-byte membership observation is not a terminal-material consumer. Its versioned read boundary must
independently compare exact original tree/blob paths and full bytes. Orchestrator supplies reviewed inputs only after
reverse-proving the original evidence-committed validation, current exact binding and the unique accepted semantic batch
which generated it, and uniquely match every subject file's exact bytes to that original validation owner's actual read set.
A caller-supplied subject/hash or another historical consistent report is not review provenance.
An original membership remains historical when HEAD changes; it cannot become currentness, working-tree/index cleanliness
or the complete actual owner dependency union. Such observations must not be injected into the scope-only V1 seal
or used to satisfy the V1 receipt rule without a separately versioned material-consumption contract.

## V2 immutable owner snapshot

`runtime/terminal-material-proof/v2` adds `terminal_owner_snapshot`, only for operation variants declaring
`runtime/owner-terminal-snapshot/v1`. The rule seals exact original receipt and validation source selectors. Review maps
these obligations without claiming the future snapshot exists. Both sources must resolve uniquely to original committed,
evidence-committed operations with the exact current binding and actual original validation read set; implementation work
effects, unrelated sources, historical consistent reports, no-op/rejected/stale receipts cannot substitute.

The snapshot owner captures immutable snapshot/root identities in the original accepted receipt, alongside its archive
address and complete object-read dependency set. SM-4 queries the real owner before the first seal and transports only
bounded observations into its private material event. The reducer independently reconstructs the exact sources, matches
the observed previous state revision, original receipt identities and dependencies, every original validation-read byte
membership and every reviewed input membership. Both source evidence IDs and receipt refs must be in the formal Result index.
Missing/ambiguous observations, dirty state, HEAD drift, altered root/head, partial membership or rehashed wrong sources fail
closed. The same source checks run on fresh load; the observations cannot change authority, credit, budgets or terminal.

Git's V3 query binds the original receipt's exact commit/root identity at the command, original-receipt, archive and response
boundaries; another object in the same complete graph cannot substitute. V1/V2 reject this identity extension.
It uses the same concrete read topic and separately samples bounded complete worktree/index/untracked status
before and after graph readback, rejecting changes or truncation. It joins exact original regular/executable tree/blob paths
and full bytes for reviewed inputs and the entire original Shell workspace-byte read set. This does not trace every subprocess
read or prove all owners are current, and is a bounded point-in-time observation, not a continuous lock. Unsupported read
kinds are rejected rather than omitted. Core consumes owner-neutral identities; only the Git adapter interprets Git grammar.

The first material CAS attaches detached proofs and includes them in the closeout/material hash. Drain admits these exact
terminal observations only through the existing validated terminal-seal lane. Once sealed, delivery/recovery use the original
immutable material without requerying Git or requiring capability owners to be restarted; later workspace changes do not
rewrite the historical delivery. Actual Result → Workspace → Run ACKs remain necessary. No owner operation is dispatched by
this consumer and no additional execution budget or state machine is introduced.

Only after the original seal and real ordered Result → Workspace → Run ACKs are durable can the delivery evaluator report
`satisfied`. Result ACK contains the actual persisted exact closeout; response loss replays the same seal, owner-derived ResultID
and idempotency identity. With 0/1/2 ACKs, material obligations remain `pending`, not pre-proven.

Failed/cancelled/superseded outcomes still deliver their original immutable material and all three ACKs. Success-material
obligations evaluate `unsatisfied`, never satisfied; delivery completion remains distinct from fulfilling acceptance obligations.
No impossible success proof may trap a faithfully delivered failure in endless re-drive. No budgets, grants, state machines,
owner I/O topics, mutable terminal bodies or retrospective credit are added.

## Verification

`runtime_terminal_proof_consumer_test.go` fixes closed union/version, exact selector, absent-extension identity, original DoD
index, current proof, deletion/duplicate/text/ref tampering, detached reload and substituted-failure rejection.
`runtimeprofiledecode/lineage_assertion_test.go` fixes typed configuration decoding and null/type/missing-field rejection.
LLM v3 rendering fixes exact versioned C3 declarations and stable C0–C2/input-change boundaries.

`biz/runtime_terminal_proof_mainline_test.go` uses real File/Shell/Run/Result owners, file-backed aggregate persistence,
scripted review and private startup. It checks actual receipt/index/seal/ordered ACKs, zero extra provider budget at review,
binding, receipt, material-seal CAS-before/CAS-after and each owner-persistence response-loss/restart cut, duplicate terminal delivery, source-obligation omission/
conflict/clarification, and faithful failed delivery with unsatisfied material obligations. It is not a final Git snapshot,
public authoring/activation, implementation-work availability, fresh OS-process or live-provider verification.

`TestRuntimeReviewedGitTerminalSnapshotOwnerMainlineAndRestart` exercises actual File/Shell/Git owners, original V3 review,
native build/test/race/behavior, commit snapshot producer, first seal and three real delivery ACKs. It checks before/after
material CAS and Result response-loss recovery, duplicate/reload isolation and coordinated head/root/input/source/revision/
dependency tampering. A separately persisted reconciliation observation may execute the original project checks again;
it is not another validation receipt or new CWI credit. Private scripted startup is not public or live proof.

`TestRuntimeReviewedGitTerminalSnapshotMultipleConsumerMainlineAndRestart` independently counts original Definition
consumers and checks each CWI's material obligation against the same actual snapshot, with independent rule/proof identities.
It repeats the material CAS-before/CAS-after and Result response-loss cuts; one consumer cannot stand in for another.

## Closed receipt operation choice

`runtime/terminal-receipt-choice/v1` optionally replaces the single exact operation selector on
`terminal_owner_operation_receipt`. It declares 2–4 distinct, contiguous exact kind/variant alternatives, all with the same
owner and durable effect scope. One matching original accepted receipt is required; alternatives are OR, while ordinary
validation and the other lineage rules remain AND. It cannot coexist with the primary operation selector, apply to another
rule kind or consume at execution phase. Unknown versions, variants, owners, duplicates and holes are rejected.
The typed four-slot array omits its zero value from legacy JSON, preserving legacy rule hashes. Raw Profile input rejects
lists larger than four before decoding. This changes only SM-4 proof selection; source binding, chronology, receipt fence,
current terminal material and all existing rejection checks remain mandatory. It grants no dispatch or early proof credit.
