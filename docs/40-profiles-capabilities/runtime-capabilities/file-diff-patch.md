# File Diff And Patch Capability

Status: normative

Type: profile-capability

Last Verified: 2026-08-23

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
- pre-effect target admission and post-effect CWI reconciliation are distinct File-private read paths: admission continues
  to reject an existing prospective create target, while reconciliation verifies the sealed owner/path against the current
  manifest instead of asking whether another create would be admissible;
- a sealed concrete target present in the current manifest yields only a factual `satisfied` observation with target/current
  revision; a missing target yields `unsatisfied`; ambiguous, non-canonical, out-of-workspace or out-of-manifest scope yields
  `unverifiable`;
- selector cardinality remains selector-specific: `file/workspace_deliverable` is the complete bounded manifest scope and
  reports every current target as satisfied even when there are multiple files; `file/unique_workspace_entry` and
  `file/profile_declared_candidates` require a unique current match and report multiple matches as unverifiable;
- a prospective path is valid only for `workspace-relative-new-file-target`, the complete bounded
  `file/workspace_deliverable` owner scope, or a path inside `file/profile_declared_candidates`. A non-empty
  workspace-deliverable scope may retain one eligible absent prospective path; it remains the exact candidate target and is
  never replaced by a current inventory member. A profile prospective path must always belong to its sealed candidate set,
  and another 1/N profile match cannot silently absorb an absent declaration. Prospective semantics cannot be granted to
  `file/unique_workspace_entry` or an unknown selector. Pre-effect `file/unique_workspace_entry` combined with any non-empty
  prospective path returns typed `unsupported` before operation admission for every current manifest cardinality (0/1/N),
  with no write; post-effect reconciliation classifies the same invalid selector combination as `unverifiable`;
- manifest-excluded roots or nested segments (`.git`, `.workorch-data`, `node_modules`, `vendor`, `dist`, `build`) cannot be
  prospective deliverable targets. Every prospective-capable selector rejects them as typed `unsupported` before effect;
- every absent prospective admission reserves capacity in the complete File listing: files and directories both consume the
  500-entry bound, and a nested target reserves its file plus every missing parent directory. The File owner serializes create
  effects per workspace and recomputes that exact additional-entry count immediately before the write, so stale admission or
  concurrent owner creates cannot commit item 501 or leave capacity-rejected parent directories behind;
  reconciliation of an already sealed legacy binding remains a factual `unverifiable` observation;
- existing exact target admits only variants that declare `exact_target` / `existing_owner_scope` and revision binding;
- `patch/apply` file-write variant rechecks `baseHash` at execution;
- text-replace/unified-diff variants follow their owner preflight/replay contract;
- ambiguous selector, unknown artifact URI or incomplete/binary/redacted snapshot is unverifiable, never guessed.

Capability owner reports feasibility/admissibility facts. Only Orchestrator SM-2 may seal/reopen CWI binding and only SM-3
may admit the resulting candidate/dispatch. File reconciliation never returns retain/reopen/CWI state and never grants new
credit.

## Target And Path Safety

- paths are workspace-relative and must remain in the allowed internal/deliverable boundary;
- a manifest is complete only after every non-excluded entry has been walked, normalized, inspected and content-hashed;
  ordinary walk/relative-path/info/hash failures fail closed rather than shrinking owner inventory into a false selection;
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
- after the first create, keep create admission rejected while post-effect reconciliation reports the exact target as
  satisfied at the current manifest revision, so reconciliation cannot request a second effect;
- cover the pre-effect `file/unique_workspace_entry` 0/1/N matrix with and without prospective target: ordinary cardinality
  remains absent/selected/ambiguous, while every prospective combination is typed unsupported and read-only;
- replay duplicate reconciliation and reload a fresh File owner with zero workspace write; deletion must produce a current
  unsatisfied observation, while ambiguous/out-of-workspace scope remains unverifiable;
- cover each File selector at zero/one/many current targets, with and without a prospective path; in particular, preserve
  full bounded workspace scope, preserve an exact current prospective declaration, and reject prospective unique-entry,
  non-participating workspace-deliverable/profile, and outside-profile-candidate selection;
- reject root and nested manifest-excluded prospective targets before effect for each prospective-capable selector, and
  keep missing or realized legacy bindings `unverifiable` without mutation;
- admit a manifest with exactly 500 includable file/directory entries, ignore excluded trees for that count, fail closed only
  after an actual 501st includable entry is observed, and reserve every missing parent entry before a nested create;
- inject walk, entry-info and content-hash failures after an includable entry and reject manifest plus unique/prospective
  admission without any workspace write;
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
