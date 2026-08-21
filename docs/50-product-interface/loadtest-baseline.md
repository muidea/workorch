# WorkOrch Governed Concurrency Load Test

Status: normative

Type: verification-contract

Last Verified: 2026-08-18

## Purpose

`scripts/loadtest-governed-workspace-concurrency.py` exercises multiple independent Workspace executions against a deployed
service and records latency, throughput, overlap, failures and final owner projections. This document defines the harness, not a
permanent capacity claim.

## Entry Points

```bash
make loadtest-governed       # deterministic mock provider
make loadtest-governed-real  # configured real provider
```

Both target an already running service. Default base URL is `http://127.0.0.1:8080`.

## Current Defaults

| Variable | Default | Meaning |
| --- | --- | --- |
| `WORKORCH_LOAD_CONCURRENCY` | `1` | client worker count |
| `WORKORCH_LOAD_TOTAL` | `10` | Workspace cases |
| `WORKORCH_LOAD_MIN_SUCCESS_RATE` | `1.0` | pass threshold |
| `WORKORCH_LOAD_LLM_MODE` | `mock` | `mock` or `real` |
| `WORKORCH_LOAD_POLL_INTERVAL_SECONDS` | `1` | verifier status-read interval; not a product refresh loop |
| `WORKORCH_LOAD_ACTIVATION_RUN_TIMEOUT_SECONDS` | `180` | mock/current activation timeout |
| `WORKORCH_LOAD_REAL_RUN_TIMEOUT_SECONDS` | `900` | real-provider Run timeout |
| `WORKORCH_LOAD_EXPECT_MAX_CONCURRENT_RUNS` | `0` | optional asserted server setting |
| `WORKORCH_LOAD_EXPECT_ACTIVE_OVERLAP` | `0` | enable overlap gate |
| `WORKORCH_LOAD_MCP_SESSION_POLICY` | `transient` | `transient` or `long_lived` fixture |

Additional provider, city, timeout, metrics sampling, artifact directory and mock-address variables are declared at the top of
the script and validated before execution.

## Per-Workspace Flow

1. create a template-scoped Workspace;
2. write Goal and DoD;
3. submit workspace `execution-activations` with `intent=restart` and a unique request ID;
4. wait for the owner-selected Run and drive until terminal, resolving only genuine typed Decision/Approval objects if present;
5. read Run snapshot/status/observability and Workspace snapshot;
6. validate formal Result, evidence/artifact summaries, usage traces, unresolved governance and scope consistency.

The fixture does not require a fixed number or order of governance objects and does not fabricate graph/next-action fields.

## Pass Criteria

- success rate is at least `WORKORCH_LOAD_MIN_SUCCESS_RATE`;
- when active-overlap checking is enabled and at least two Runs can overlap, observed Run windows or in-flight metrics reach the
  expected overlap;
- every successful case reaches a valid terminal Result and passes snapshot/observability consistency checks;
- HTTP/provider/protocol failures are classified and retained per Workspace.

The harness reports observed performance but does not enforce undocumented p95/throughput thresholds. A capacity baseline is
valid only when its command, service revision, config, host limits and generated summary are stored as an external run artifact,
not as a normative document under `docs/`.

## Outputs

By default the harness keeps an artifact directory and prints its path. It contains:

- `summary.json` and `report.md`;
- per-Workspace request, activation, Run, snapshot, observability and failure evidence;
- latency distributions for activation, completion and reads;
- throughput, success rate, overlap and sampled service metrics;
- slowest cases and failure-class counts.

Set `WORKORCH_LOAD_ARTIFACT_DIR` for a stable location or `WORKORCH_LOAD_KEEP_ARTIFACTS=0` for cleanup.

## Interpretation

- the default c1/t10 run is a deterministic harness regression, not a capacity ceiling;
- real-provider results include external latency and quota behavior and must be labeled separately;
- a 100% successful serialized run does not prove concurrency overlap;
- a load result never replaces Runtime owner/invariant/restart tests.
