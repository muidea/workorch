# Authority Live E2E Closure Verification

Status: active

Type: verification-contract

Last Updated: 2026-06-27

Related:

- `../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `../70-roadmap/active/convergence-authority-simplification-design.md`
- `../70-roadmap/active/authority-live-verification-e2e-closure-design.md`
- `convergence-authority-dispatch-live-verification.md`

## Purpose

This verification entry turns the authority live/e2e closure design into a
repeatable deployed-service harness. It records service readiness, compact
workspace projection, metrics, optional command submission, owner readback
progress and resource deltas without reading full workspace snapshots by
default.

The harness is business-neutral. It does not assume a template, language,
provider, file name, workspace title or concrete deliverable type.

## Command

Read-only observation:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
make verify-authority-live-e2e-closure
```

The default mode only reads:

- `/readyz`
- `/api/v1/workorch/workspaces`
- `/metrics`
- `/api/v1/workorch/decisions?workspaceId=<workspaceId>` for bounded authority readback
- `/api/v1/workorch/runs?workspaceId=<workspaceId>` for bounded authority readback

It writes a compact `report.json` under the artifact directory and does not
call snapshot/detail endpoints.

## Optional Resume Verification

To verify command accepted -> owner readback progress for a run:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_AUTHORITY_VERIFY_RESUME_WORKSPACE_ID=<workspaceId> \
WORKORCH_AUTHORITY_VERIFY_RESUME_RUN_ID=<runId> \
WORKORCH_AUTHORITY_VERIFY_WAIT_SECONDS=10 \
make verify-authority-live-e2e-closure
```

The command is accepted only if owner readback later shows at least one of:

- workspace/run state changed,
- active run changed,
- old blocker changed or cleared,
- prompt mode changed,
- ViewModel revision advanced,
- run/projection state is `active`, `completed`, `failed`, `blocked_on_approval`, or an equivalent governed/terminal projection,
- a new governance blocker replaces the previous blocker.

## Optional Decision Verification

To verify decision resolution -> owner continuation:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_AUTHORITY_VERIFY_DECISION_WORKSPACE_ID=<workspaceId> \
WORKORCH_AUTHORITY_VERIFY_DECISION_TICKET_ID=<ticketId> \
WORKORCH_AUTHORITY_VERIFY_DECISION_RESOLUTION_TYPE=replan \
WORKORCH_AUTHORITY_VERIFY_DECISION_PAYLOAD_JSON='{"operator":"authority-live-verification"}' \
WORKORCH_AUTHORITY_VERIFY_WAIT_SECONDS=10 \
make verify-authority-live-e2e-closure
```

Invalid decision input should be tested by expecting the script to fail and
checking that the ticket remains open in the projection or decision API.

## Resource Budget Options

The script records deltas for:

- `workorch_event_record_metadata_records_scanned_total`
- `workorch_event_record_payload_hydrates_total`
- `workorch_event_record_payload_hydrate_bytes_total`
- `workorch_workspace_projection_schedule_dropped_total`
- `workorch_workspace_projection_ineffective_retry_runs`

Optional hard limits:

```bash
WORKORCH_AUTHORITY_VERIFY_MAX_METADATA_RECORDS_SCANNED_DELTA=0 \
WORKORCH_AUTHORITY_VERIFY_MAX_PAYLOAD_HYDRATES_DELTA=0 \
WORKORCH_AUTHORITY_VERIFY_MAX_PAYLOAD_HYDRATE_BYTES_DELTA=0 \
make verify-authority-live-e2e-closure
```

Authority readback fetches are bounded by:

```bash
WORKORCH_AUTHORITY_VERIFY_MAX_WORKSPACE_DETAIL_FETCHES=20 \
make verify-authority-live-e2e-closure
```

The readback uses list endpoints only. It prioritizes workspaces that have an
active run, blocker or decision prompt, then fills the remaining budget with
other workspaces.

Use strict zero limits only for idle/list-only windows. A live executing
workspace may legitimately change metrics; in that case the report is the
auditable baseline and stricter thresholds should be applied in a controlled
scenario.

## Required Report Fields

`report.json` must include:

- `pass`
- `errors`
- `warnings`
- `command.kind`
- `command.status`
- `command.progress`
- `projection.before`
- `projection.after`
- `workspaceCount`
- `waitingDecisions`
- `authorityChecks`
- `authorityReadbackWorkspaceCount`
- `metrics.delta`

The `waitingDecisions` section classifies obvious contract-retry or violation
blockers separately from generic decision-required blockers so operators can
avoid treating all `blocked_on_decision` / decision prompt states as the same condition.

The `authorityChecks` section records compact evidence for:

- workspace decision prompt state and ticket state,
- active run decision blocker state,
- resolved replan successor or failed continuation diagnostic,
- completed continuation readback that still references the old resolved
  blocker.

## Pass Criteria

- Service readiness is `ready` before and after the observation window.
- Workspace projection state is `ready`.
- When projection alignment is required, `sourceEventRevision` equals
  `projectedSourceRevision` before and after.
- Workspace count is at least the configured minimum.
- Required metric markers are present.
- If a command is submitted, owner state must visibly advance or enter a new
  governed/terminal condition.
- A workspace cannot show prompt `decision`, and a run cannot be
  `blocked_on_decision`, unless the scoped active-run decision ticket is still
  open. Workspace owner state itself remains `active` for decision blockers.
- A run cannot remain `blocked_on_decision` unless the scoped blocking decision
  ticket is still open.
- A resolved replan decision must have successor evidence or a failed
  continuation diagnostic.
- A completed continuation diagnostic must not show the same old resolved
  blocker still present after readback.
- Optional resource budget limits are not exceeded.

## Failure Handling

If the read-only mode fails, inspect workorchd readiness, projection rebuild
health and metrics exposure before changing runtime code.

If command mode fails with `owner_state_not_advanced`, inspect:

- scheduler diagnostics,
- run state and old blocker,
- convergence authority snapshot,
- decision/approval state,
- latest rejected diagnostic evidence,
- projection revision.

Do not add template, language, provider or workspace-specific branches to core
to satisfy this verifier. Fix the owner path, projection path, resource path or
template policy according to the failing evidence.
