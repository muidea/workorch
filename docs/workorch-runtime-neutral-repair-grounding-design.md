# WorkOrch Runtime Neutral Repair Grounding Design

## Purpose

This document defines a framework-neutral repair grounding contract for WorkOrch runtime.

The observed failure pattern is not a service stability problem. Runtime can reject invalid outputs and keep repairing, but long software-development-like workflows still fail to converge when repair prompts carry too much historical material and too little current, structured failure evidence.

The goal is to make repair input small, current, target-bound, and evidence-grounded without embedding any application-domain logic in WorkOrch core.

## Neutrality Boundary

Core runtime may do only mechanical processing:

- classify failure by stage, step kind, exit code, target ref, failure signature, and diagnostics;
- extract file-like references, line numbers, identifier-like tokens, field-like tokens, and target scopes from plain failure text;
- select candidate resources by path/name/token overlap against workspace manifest and existing target snapshots;
- compact or omit raw rejected payloads;
- enforce capability input contracts, patch syntax, target relevance, and retry budgets;
- require evidence refs or artifact refs for satisfied claims.

Core runtime must not do domain interpretation:

- no Go-specific, JavaScript-specific, HTTP-specific, or test-framework-specific rules;
- no assumptions about valid handler names, test structures, vendors, protocols, providers, or business goals;
- no direct mutation of workspace deliverables to fix a run;
- no business-domain fallback when LLM output is weak.

Domain-specific expectations belong in workspaceTemplate, profile contract, soul, skill, MCP/tool descriptions, and prompt content.

## Problem Statement

Recent run evidence showed these failure modes:

- repair request sizes returned to hundreds of KB;
- invalid patch text containing runtime redaction markers was repeatedly regenerated;
- malformed unified diffs and stale oldText were retried;
- LLM invented identifiers that were not grounded in current workspace resources;
- repair kept changing the same target without converging to validation or assessment.

The root issue is a mismatch between repair input and repair objective. A state-changing repair needs a small current failure package, not a broad historical conversation.

## Core Contract

### Structured Failure Signals

Every runtime repair should include `structuredFailureSignals` when failure output contains extractable signal.

The signal package is mechanical and domain-neutral:

```json
{
  "sources": ["stderr", "stdout", "error", "reason", "failureSignature"],
  "targetRefs": ["relative/path.ext"],
  "lineRefs": [
    {"targetRef": "relative/path.ext", "line": 12, "column": 34}
  ],
  "identifiers": ["identifierLikeToken"],
  "messages": ["bounded diagnostic lines"]
}
```

Rules:

- `targetRefs` are workspace-relative-looking paths only.
- `identifiers` are syntax-shaped tokens, not semantic conclusions.
- `messages` are bounded diagnostic lines with redaction markers removed or omitted.
- no raw invalid JSON, raw stream deltas, full rejected patch content, or full LLM response is included.

### Repair Action Package

`repairActionPackage` is the primary repair input for state-changing repair. It must contain:

- `requiredOutput`: required capability kind and target relevance rules;
- `previousRejectedOutputSummary`: bounded, non-reusable summary only;
- `structuredFailureSignals`: extracted neutral failure signals;
- `targetSnapshots`: current target resources when known;
- `workspaceFileManifest`: bounded current manifest only when needed;
- `forbiddenContentReuse`: markers explaining what must not be copied.

The package is authoritative over older fields. Older fields may remain for compatibility inside runtime internals, but LLM-facing context should prefer this package.

### Failure Product Isolation

Rejected artifacts are not repair source material.

When the failed output is caused by invalid JSON, redaction marker contamination, stale diff, malformed unified diff, context mismatch, or missing oldText, runtime may expose:

- failure class/signature;
- stage/status/error/reason;
- stream diagnostics and usage summary;
- rejected graph summary;
- target metadata and current target snapshots;
- patch repair suggestions derived from current target snapshots.

Runtime must not expose:

- raw invalid JSON;
- raw rejected diff/content;
- stream event text as patch source;
- redacted marker text;
- full historical step input/output when a compact package is sufficient.

### Identifier Grounding

If failure text contains identifier-like tokens or field-like tokens, runtime may include them as neutral `identifiers`.

LLM guidance must require:

- referenced identifiers in repair output must be grounded in current resource evidence, or introduced by the same state-changing artifact;
- when candidate resources are insufficient, read a bounded resource before patching;
- do not invent missing names solely to satisfy failure text.

Core does not know what an identifier means. It only transports the token and evidence refs.

### Patch Contract

For existing targets, state-changing repair must prefer:

- minimal `unified_diff` with explicit hunk line ranges;
- `file/edit` with exact current oldText/newText;
- `file_write` only when creating a missing target or replacing a complete safe snapshot intentionally.

Patch output must be rejected when:

- target is unrelated to explicit repair target scope;
- diff/content contains runtime redaction markers;
- hunk header is malformed;
- oldText does not exist;
- base/context is stale;
- patch is a marker/no-op unrelated to failure.

### Repeated Failure Convergence

Repeated failure on the same target scope should reduce freedom:

1. First repair: allow normal state-changing repair with target context.
2. Later repair: require repairActionPackage as primary input, omit rejected raw text, prefer exact current target snapshot.
3. Repeated invalid artifact: continue within repair budget only with smaller package and stronger target constraints.
4. Budget exhausted: failed assessment / blocker / rejected result, not unbounded repair.

## Context Budget Rules

Repair requests should shrink as failure scope becomes clearer.

Recommended policy:

- target-scoped repair input should normally stay under 120 KB;
- include at most the current target snapshot, compact failure signals, bounded manifest, and compact previous rejection summary;
- historical memories are allowed only as short lessons, not raw failed artifacts;
- provider/model/runtime dynamic binding must not be persisted into recovery context.

## Workspace Template Responsibilities

Software-development templates may add domain-specific rules:

- how to interpret tests, builds, lint, package managers, and language toolchains;
- what validation commands are expected;
- how to describe source changes, validation reports, coverage maps, and user deliverables;
- how to phrase software-specific repair instructions.

The template must still express these rules through neutral runtime fields: stage, profile contract, capability kind, evidence refs, target refs, and coverage ids.

Example software-development template guidance:

- when `repairActionPackage.targetSnapshots` is present, patch the referenced current target or read the referenced resource before patching;
- when `structuredFailureSignals.identifiers` is present, treat identifiers as diagnostics only and reconcile them with current source evidence before adding, removing, or renaming symbols;
- when `forbiddenContentReuse` contains stale patch, missing oldText, malformed diff, invalid JSON, or redaction marker classes, rebuild the patch from current snapshots instead of copying rejected text;
- when `repeatedTargetRepairMode.enabled=true`, prefer exact oldText/newText or a minimal unified diff against the current snapshot and avoid broad replacement.

## Implementation Plan

### P0

- Add neutral `structuredFailureSignals` extraction from failure output.
- Attach `structuredFailureSignals` to `repairActionPackage`.
- Add `forbiddenContentReuse` metadata when failure contains invalid JSON, redaction markers, stale patch, malformed diff, or oldText mismatch.
- Prefer repairActionPackage in LLM context compaction.
- Add tests for symbol/path/line extraction and redaction-safe repair package.

### P1

- Tighten LLM-facing repair input to omit duplicated `failedStepInput`, broad previous output, and raw graph payloads when repairActionPackage exists.
- Add repeated target-scope repair mode that lowers context budget and forces exact target snapshot grounding.
- Add stronger target relevance diagnostics to repairActionPackage.

### P2

- Surface repair target scope, structured failure signals, context bytes, and repeated failure reason in web observability.
- Add profile/template examples for software-development validation repair.

## Acceptance Criteria

- Core runtime remains domain-neutral.
- Repair input contains current target evidence and structured failure signals.
- Invalid rejected output is summarized, not reused.
- Patch repair is target-bound and syntax-bound.
- Long runs do not grow request payloads by repeatedly copying old failures.
- Software-specific behavior remains in workspaceTemplate/profile/soul.

## Progress

| Item | Status |
| --- | --- |
| Design document | Done |
| P0 structured failure signals | Done |
| P0 repairActionPackage integration | Done |
| P0 forbidden content reuse metadata | Done |
| P0 LLM context prefers repairActionPackage | Done |
| P0 tests | Done |
| P1 compact persisted repair input | Done |
| P1 repeated target-scope repair mode | Done |
| P1 target relevance diagnostics | Done |
| P2 repair grounding observability projection | Done |
| P2 web / TUI repair grounding display | Done |
| P2 profile/template examples | Done |
