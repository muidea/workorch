# Convergence Authority Dispatch Live Verification

Status: active

Type: verification-contract

Last Updated: 2026-06-26

Related:

- `../90-archive/superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `../70-roadmap/active/convergence-authority-simplification-design.md`
- `../70-roadmap/active/convergence-authority-dispatch-closure-checklist.md`
- `workorch-runtime-closure-modification-spec.md`

## Purpose

This verification contract is the live acceptance entry for convergence
authority dispatch closure. It verifies that a deployed WorkOrch instance can
observe at least two workspaces through the same owner-state and projection
chain used by Web/TUI, while also recording resource-envelope evidence.

It does not define business delivery rules. Goal/DoD semantics remain owned by
the workspace definition and template/Soul policy. This verification only checks
the neutral runtime contract:

```text
Goal/DoD Definition
  -> Coverage/Evidence Ledger
  -> Current Focus
  -> deterministic NextAction
  -> Stage Packet
  -> LLM/runtime candidate
  -> Contract Gate
  -> accepted/rejected evidence
  -> ledger recompute
  -> projection/ViewModel
```

## Contract Audit

- Scope: deployed workorchd runtime observation for at least two workspace
  snapshots, projection metadata, SSE events, LLM usage/cache samples, and
  process CPU/RSS/IO.
- Observed state: previous closure passes added local regression tests for
  dispatch authority, projection completeness, and projection/event IO pressure,
  but live verification still needs a repeatable command and artifact schema.
- Expected state: post-redeploy verification records the deployed process,
  workspace IDs, active run IDs, projection revisions, evidence/result counts,
  goal-convergence status, cache samples, EventHub publish failure deltas, and
  CPU/IO deltas in one artifact directory.
- Deviation: without a fixed live verification entry, later checks can omit
  workspace count, resource counters, cache evidence, or projection revision
  data and still appear manually complete.
- Owner: verification harness. Runtime owner state remains workorchd and
  orchestrator; TUI/Web remain display/input clients.
- Event path: deployed service -> `/readyz`, `/metrics`, workspace list,
  workspace snapshots, SSE stream, optional ai-proxy logs/interactions ->
  `runtime-observation-summary.json`.
- Contract gap: P2-2 needed a deterministic live verification gate for
  two-workspace convergence authority dispatch and resource-envelope evidence.
- Proposed change: use `make verify-convergence-authority-dispatch-live`, which
  wraps `scripts/observe-workorch-runtime-closure.sh` with two-workspace,
  workspace-progress/governed-blocker, process-resource, projection-drop, and
  runtime publish failure gates. Final Goal/DoD completion remains an optional
  stricter gate.
- Boundary check: the harness is business-neutral. It does not branch on
  template ID, Soul ID, programming language, command output, provider brand,
  workspace title, or live workspace ID.
- Verification: `bash -n scripts/observe-workorch-runtime-closure.sh`, target
  dry-run inspection, and post-redeploy execution against the live service.

## Required Command

For a deployed service with known workspace IDs:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_OBSERVE_WORKSPACE_IDS=ws-a,ws-b \
WORKORCH_OBSERVE_ARTIFACT_DIR=/tmp/workorch-convergence-authority-live \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
make verify-convergence-authority-dispatch-live
```

When workspace IDs are not supplied, the target samples the first two projected
workspaces through `WORKORCH_OBSERVE_AUTO_SNAPSHOT_LIMIT=2`. This is acceptable
only when the deployed service is known to contain the target workspaces.

## Optional Cache Evidence

If ai-proxy interaction metadata is available, include cache samples:

```bash
WORKORCH_OBSERVE_AI_PROXY_INTERACTIONS_DIR=/home/rangh/aispace/ai-proxy/interactions \
WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=1 \
WORKORCH_OBSERVE_MIN_CACHE_SAMPLE_COUNT=2 \
make verify-convergence-authority-dispatch-live
```

Provider/model filters may be added with:

```bash
WORKORCH_OBSERVE_CACHE_PROVIDER=<provider> \
WORKORCH_OBSERVE_CACHE_MODEL=<model>
```

## Required Artifacts

The verification artifact directory must contain:

- `runtime-observation-summary.json`
- `readyz.json`
- `workspaces.json`
- `metrics-before.txt`
- `metrics-after.txt`
- `sse-events.jsonl`
- `snapshot-<workspaceId>.json` for each observed workspace
- `projection-display-contract.json`
- `process-resource-before.json` and `process-resource-after.json` when process
  resource sampling is required

`runtime-observation-summary.json` must include:

- `observedWorkspaceIds` with at least two workspace IDs
- `observedWorkspaceThreshold=2`
- `workspaceSummaries[*].workspace.activeRunId`
- `workspaceSummaries[*].run.id`
- `workspaceSummaries[*].projection.workspaceRevision`
- `workspaceSummaries[*].projection.projectedSourceRevision`
- `workspaceSummaries[*].counts.acceptedResults`
- `workspaceSummaries[*].coverage.currentGap`
- `workspaceProgress.pass`
- `goalConvergence.pass`
- `metrics.projectionPendingRebuilds`
- `metrics.projectionScheduleDroppedDelta`
- `metrics.eventRecordIOPressure.*Delta`
- `metrics.runtimeEventPublishFailures.totalPublishFailureDelta`
- `cacheHitRate.sampleCount`, `latest`, and `average` when cache samples are
  required
- `processResource.cpuPercent`, `readBytesDelta`, and `writeBytesDelta`
- empty `errors`

## Pass Criteria

- Service readiness is `ready`.
- At least two workspace snapshots are observed.
- Projection list and each workspace snapshot expose displayable projection
  revision/timing fields. `ready` projections must not lag their source
  revision; `stale` and `degraded` projections may expose lagging revisions as
  projection health instead of failing the display contract.
- Workspace progress gate passes for each observed workspace. A passing
  workspace must have accepted evidence, a correct governed decision/approval
  blocker, or an active progress heartbeat.
- Final Goal/DoD convergence is optional for in-flight observation. Enable
  `WORKORCH_OBSERVE_REQUIRE_GOAL_CONVERGENCE=1` when the verification window is
  expected to prove completion.
- Runtime EventHub publish failure delta is `0`.
- Projection schedule dropped delta is `0`.
- Pending projection rebuilds remain within the configured threshold.
- CPU/IO deltas remain within configured thresholds.
- Cache samples are present and meet configured thresholds when cache sampling
  is enabled.

## Failure Handling

If this verification fails, do not immediately change runtime code. First map
the structured failure back to the owner:

- `goalConvergence` failure: inspect coverage ledger, accepted/rejected
  evidence, current focus, NextAction, and Stage Packet.
- `workspaceProgress` failure: inspect whether the workspace has accepted
  evidence, a current governed blocker, or active runtime progress before
  changing code.
- projection revision failure: inspect workorchd projection manager/store and
  port/client DTO propagation.
- EventHub publish failure: inspect scheduler command/result path and
  runtime-event publish diagnostics.
- process CPU/IO failure: inspect event record IO deltas, projection rebuild
  metrics, pprof artifacts, and repeated request-time scans.
- cache failure: inspect LLM request partitioning, request logs, provider
  interaction metadata, and stable prefix behavior.

Only after the owner path is identified should code be changed.
