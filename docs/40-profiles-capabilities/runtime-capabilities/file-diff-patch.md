# File Diff And Patch Capability

Status: normative

Type: profile-capability

Last Verified: 2026-08-18

## Purpose

`blocks/file` owns workspace-bounded file observation, admissibility and side effects. This document defines the stable
profile/capability semantics needed to select a legal operation without moving file rules into Orchestrator core.

## Operation Surface

| Operation | Target condition | Side effect |
| --- | --- | --- |
| `file/list` | workspace-relative directory/scope | none |
| `file/read` | existing readable file | none |
| `file/write` | target must not exist | create only |
| `file/edit` | existing exact target/current owner scope | revision-bound edit |
| `patch/preview` | existing/declared target and patch input | none |
| `patch/apply` | existing target or explicitly admitted creation variant | revision/base-hash-bound change |

`file/write` is strictly create-only. If the target exists, File owner returns a typed semantic rejection before any side
effect. A reopened CWI or revised candidate must use an owner-admitted replacement operation—normally `file/edit` or
`patch/apply`—and bind the latest target revision/base hash. Repeating `file/write` is not a valid correction.

## Planning And Reconciliation Contract

The File owner publishes current target/manifest observation and operation admissibility. Semantic planning must consume
that latest catalog:

- absent target + authorized prospective workspace scope may admit create-only `file/write`;
- existing exact target admits only variants that declare `exact_target` / `existing_owner_scope` and revision binding;
- `patch/apply` file-write variant rechecks `baseHash` at execution;
- text-replace/unified-diff variants follow their owner preflight/replay contract;
- ambiguous selector, unknown artifact URI or incomplete/binary/redacted snapshot is unverifiable, never guessed.

Capability owner reports feasibility/admissibility facts. Only Orchestrator SM-2 may seal/reopen CWI binding and only SM-3
may admit the resulting candidate/dispatch.

## Target And Path Safety

- paths are workspace-relative and must remain in the allowed internal/deliverable boundary;
- absolute paths, traversal, symlink escape and wrong-workspace targets fail closed;
- complete patchable snapshot includes target ref, content state, content hash/revision and non-truncated text;
- missing/truncated/binary/redacted content cannot be used as a patch base;
- File owner does not infer target from Goal/DoD prose, directory name or file extension;
- large content/diff uses bounded preview plus validated raw ref.

## Governance

Sensitive trust-boundary writes—such as secrets, `.env`, `.git` or CI workflow paths—return `governance_required` before
side effect. Execution after approval requires the exact typed grant bound to `ExecutionKey` and next fence; a boolean or
caller assertion cannot authorize it. Ordinary workspace-local change size alone is not an authorization source.

## Evidence Boundary

File owner returns one normalized `RuntimeOperationReceipt` with factual category, target refs, content/effect metadata,
bounded evidence/raw refs and safe original diagnostic. It does not:

- judge evidence quality or DoD coverage;
- choose retry/correction/governance/terminal;
- create accepted Result;
- mutate Runtime aggregate;
- expose a second status/event authority.

Orchestrator admits the receipt into its evidence ledger and projection derives Patch/ToolCall/Artifact views where
applicable. Technical file success is not business completion.

## Required Regression Cases

- create an absent target with `file/write` exactly once;
- reject `file/write` on an existing target without modifying bytes/revision;
- after CWI reopen, acquire latest target snapshot/catalog and choose revision-bound replacement;
- reject stale base hash/revision before overwrite;
- replay duplicate dispatch/receipt without duplicate side effect/evidence;
- reject path escape, ambiguous selector and incomplete target snapshot;
- gate sensitive paths and consume only the matching approval fence;
- persist/reload receipt and re-drive without converting a create into a second write.

Deployed coverage uses `make verify-workspace-file-diff-patch` plus the Runtime positive-convergence live gate. The original
failure mode—valid first create followed by repeated create-only `file/write` after reopen—must remain a negative fixture.

## Related Documents

- [Planning And Execution](../../00-core/runtime/flows/planning-execution.md)
- [Block Boundaries](../../30-module-architecture/block-boundaries.md)
- [Core Runtime Verification](../../00-core/runtime/verification.md)
- [File Block README](../../../internal/modules/blocks/file/readme.md)
