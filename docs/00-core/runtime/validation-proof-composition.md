# Validation Proof Composition

Status: normative

`validation/input-consistency/v3` is an opt-in composition review protocol. It preserves every original criterion and
source-obligation declaration. It does not turn a lineage guard into ordinary behavioral evidence or open implementation-work
availability by itself.

An explicitly source-mapped execution guard follows [Cross-Source Execution Proof](cross-source-execution-proof.md).
Its original source is sealed by exact user confirmation; the consumer's ordinary proof is not redirected to that source.

The Definition compiler binds the exact CWI's Profile-declared `RuntimeLineageAssertion` values into its validation requirement.
The validation owner copies those declarations into a v3 input-review subject together with complete input bytes and native
check mappings. The requirement hash, subject hash, catalog, semantic manifest and report bind the complete declarations.
Consumers must reverse-check them against the original Definition; a self-consistent substituted hash is insufficient.
V1/V2 subjects and reports cannot carry the composition extension; absent extensions retain their existing identities.

Each v3 obligation retains an exact excerpt of the original criterion and declares canonical `proofConsumerRefs`. The reserved
`validation_input` consumer means actual assertions in the supplied validation inputs. Other references must identify exact
lineage rules in the same sealed CWI. Every check's mapping must include `validation_input` and every declared rule; excerpts
must still cover all non-whitespace original criterion characters. These structural checks prove provenance and completeness,
not natural-language entailment: the independent reviewer must explain how the named consumers jointly cover each obligation.
Missing or contradictory coverage is conflict; genuinely unavailable facts remain inconclusive. Comments, README claims,
future receipts, rule names, or a review's consistent conclusion are not execution evidence.

SM-3 persists the complete bounded report through the existing semantic request/correction transaction. SM-2 seals only after
the existing consistency and owner-feasibility gates. Ordinary all-of validation and assessment remain mandatory; execution
lineage guards independently consume original completed work/repair/owner receipts at the assessment evidence cutoff.
`post_terminal_delivery` declarations only describe the future delivery consumer during review: SM-4 evaluates immutable
material plus ordered Result/Workspace/Run ACKs after terminal, never fabricates future ACKs or adds execution credit.
SM-1 and owner boundaries are unchanged. There is no new ledger, transition, budget, cross-owner writer or waiver.
The versioned [Terminal Material Proof](terminal-material-proof.md) contract also checks exact source-bound original receipts
and complete formal Result indexes after material generation; it does not treat a receipt as final repository bytes.

Protocol selection is activation-sealed. Input/rule changes invalidate the subject and report; fresh-load replays the same
identities, never upgrades V1/V2 archives. Protocol-specific schema, inputs, declarations and instructions belong in C3, not
C0-C2. Unsupported protocol/adapter combinations fail closed before owner execution or provider submission.

The current Shell producer supports v3 only with the explicitly sealed
`shell/project-native-validation/v6` or explicitly opted-in `/v7` or `/v8` project-conjunction adapter. Its four actual all-of stages, exact native mappings,
source-integrity checks, output retention and single operation deadline are unchanged. Review does not substitute for any
stage. The built-in template does not select v3 or authorize general implementation work merely because this producer exists.

The optional `validation/diagnostic-source-review/v1` extension is declared by the validation owner on a v3 input subject,
not inferred from recipe names or failure text. `DiagnosticSources` seals every bounded native source's opaque exact
`SourceRef + NativeCaseRef + InputTargetRef`, along with complete input bytes and original requirement/native mappings in the
subject hash. Every source requires a separate report (`sourceRef`, `conclusion`, `reason`, exact `inputTargetRefs`), independently
judging compatibility with the unchanged Goal/DoD; another CWI's consistent check cannot supply that finding. Ordinary original
checks, full textual obligation coverage and typed consumers remain mandatory AND. Missing, duplicate, unknown, conflicting,
unreviewed or drifted sources block seal; neither inventory nor review supplies behavior, repair authority or credit.

Only this explicit extension selects its finite envelope: 64 input files / 256 KiB, 64 native sources, 64 KiB complete report.
Legacy subjects retain 16 files / 64 KiB / 32 KiB reports; no truncation, capacity fallback or archive upgrade is permitted.
Native identity grammar, input/implementation roles and actual per-source execution belong to the capability owner. Mutable
delivery implementation is not silently turned into immutable review input; its current actual bytes are proved independently
by execution/read dependencies. Missing external harness/fixture context remains inconclusive. Source findings and complete
declarations enter C3/schema/strict decoder, subject/report hashes, detached clones and same-version reload; C0-C2 do not change.

Source-specific diagnostic stages retain the selected observation protocol, complete original stage/output identities and
independently attributed target sets through receipt publication. Rejected-receipt admission and fresh-load reverse-check
every opaque source against the original sealed owner binding and every target against its original owner scope; the aggregate
source-diagnostic behavior set must equal the independently attributed union. Unique sources, actual failed completion, unchanged source versions,
chronological stages are mandatory. Repair authority additionally requires every original stage's stdout/stderr to have a
durable owner reference; omission or an absent storage declaration blocks it. These facts cannot make an incomplete conjunction
complete, borrow another source's behavior, or upgrade legacy rejected-stage histories into diagnostic authority.
The original sealed binding selects this gate even if optional per-source fields are erased. A structured failure's exact
repair candidates must equal the independently attributed union; deleting the observation, conjunction or entire union
cannot preserve repair authority. Unobserved failures may still be retained without a repair-authorizing observation.
Ordinary behavior observed before a later failed deadline/storage boundary may remain in a noncomplete rejected conjunction;
without source attribution it cannot supply a structured repair observation or any acceptance credit.
If an earlier failed source was independently attributed but a later source is unobserved, its real stage/behavior facts and
their exact union remain historical rejected evidence. This partial union does not stand in for the missing source and does
not authorize repair; the owner must still withhold the structured repair diagnostic and all acceptance credit.
Publication failure after actual attribution preserves the original source/behavior, output byte/hash identity, already saved
streams and explicit omission reasons. It is rejected history only, not complete diagnostic storage or repair authority;
reloading the original receipt cannot turn that history into a repair-authorizing failure observation.

Diagnostic-source uncertainty shares at most one information resolution per original Run/CWI, using the original sealed
information-retirement history rather than a new counter. Source names, paths, bytes, native/catalog versions or restart cannot
reset it; it cannot be laundered into an original check's new question. Conflict remains known conflict, never a coverage-only
omission or governance waiver. SM-3 persists the report; SM-2 seals only after consistency and owner feasibility; SM-4 recovers
from the aggregate and original receipts; SM-1 and all effect/repair/operation budgets remain unchanged.

An execution repair rule may explicitly seal `RepairHistoryContractRef=runtime/confirmed-binding-repair-proof/v1`.
This independent consumer reverse-checks same-ordinary-binding rejected validation, the original confirmed owner repair
commitment/admission, exact Profile route, unique confirmation, complete changed durable owner receipts and execution fences,
then strictly later exact independently-reviewed ordinary validation. Mutation facts retain empty CWI credit and their original
ledger; neither an accepted behavior receipt nor a rejected validation is promoted to ordinary acceptance proof. Missing,
duplicate, stale, source drift or invalid budget blocks. Absent extension retains the original retired/reopen consumer and hash.
Any later rejected validation for that exact ordinary binding blocks the proof at the original assessment cutoff, including
rejected facts with empty CWI credit. A later rejection naming the same CWI also blocks; unrelated sources and evidence after
the cutoff cannot change this historical judgment. Original rejection facts are never rewritten to infer acceptance credit.
This pure consumer adds no SM transition, ledger, budget or effect replay; SM-2 consumes it only as an additional AND.
The ordinary existing-owner finite target scope is an authorization set, not an all-target execution requirement. Fresh
validation must still match the exact sealed operation/binding and ordinary recipe/native all-of proof. Explicit prospective
target members, if declared, remain collectively mandatory; an empty accepted validation set never satisfies the consumer.

Requirements compiled with nonempty lineage declarations bind the full declarations in their content identity. Old
guard-bearing archives without this preimage are not backfilled, rehashed or upgraded during recovery; a compiler/preimage
mismatch remains an explicit contract rejection. Requirements without declarations retain their original JSON/hash.
