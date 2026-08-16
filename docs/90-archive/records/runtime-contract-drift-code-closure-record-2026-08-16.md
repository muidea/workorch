# Runtime Contract Drift Code Closure Record — 2026-08-16

Status: completed

Type: verification-record

## Outcome

CONTRACT-DRIFT is closed. Active defaults, examples, verifiers, Make targets and container packaging now describe and exercise
the current four-state-machine, single-owner Runtime path. No new owner, state machine, transition or cross-owner state write
was introduced.

| Slice | Result |
| --- | --- |
| CD-1 defaults/Souls | retired provider authority keys removed; semantic-plan/operations/binding/receipt/evidence guidance installed; stable prefix tests passed |
| CD-2 scripts/examples | direct Run writes removed; current activation/owner/recovery tests installed; empty focused test selections fail closed |
| CD-3 container | embedded console copied; dependency layer cached; image build, ephemeral mode and persistent restart mode passed |
| CD-4 guards | authority, routes, configuration, package/test-selector paths, projection compatibility and governance-continuation recurrence guards passed |

## State-Machine Assessment

- SM-1: workorchd submits only typed workspace execution intent; Run owner remains the lifecycle/replacement transition owner.
- SM-2: defaults describe current CWI binding, receipt credit, assessment and terminal facts; no capability execution moved into
  the reducer.
- SM-3: candidates use current semantic-plan/operations documents and sealed dispatch; File Owner create-only/revision-bound
  admissibility is exercised directly.
- SM-4: restart/replay and `Result -> Workspace -> Run` delivery remain Orchestrator-owned; application and Governance do not
  auto-resume or create successors.

## Original Failure Regression

The File Owner regression gate explicitly exercises:

- `file/write` as create-only;
- existing targets requiring a revision/base-hash-bound replacement;
- `file_write`, `text_replace` and `unified_diff` patch variants;
- stale base-hash rejection;
- owner-receipt response loss, replay proof and CAS/restart recovery.

This preserves the fail-closed behavior from the reported revision-93 counterexample while allowing current semantic guidance
to select the legal replacement operation.

## Verification

Passed on 2026-08-16:

- `make release-check` — `go vet`, full `go test ./...`, boundary verification, console build and four-platform release build.
- `make fmt-check` and `git diff --check`.
- focused provider-prefix tests for C3 isolation, typed context and correction retry stability.
- `scripts/verify-acceptance-slice-result-e2e.sh` and its file-backed wrapper.
- `scripts/verify-runtime-positive-convergence-file.sh` with fail-closed test selection; retired Run recovery,
  governance continuation, Eventstream payload, terminal projection, bootstrap and TUI selector names were rebound to the
  current owner contracts.
- `scripts/verify-workspace-file-diff-patch.sh`.
- `scripts/verify-auto-governance-ticket-approval.sh`.
- `scripts/verify-boundary-contract.sh`.
- `docker build --network=host --build-arg GOPROXY=https://goproxy.cn,direct -t workorch:contract-closure .`.
- `scripts/verify-workorch-container.sh` with `ephemeral` and `persistent` storage modes.

The Docker proxy/network override was needed because this host's Docker bridge timed out or reset connections to public Go
module proxies. Earlier failed attempts stopped in module/image metadata transfer; the successful build proved application
compilation and embedded console packaging separately from that network condition.

## Deliberate Removals

- `scripts/verify-workorch-runtime-core-flow.sh`
- `scripts/verify-workorch-llm-context-integrity.sh`

Both were unreferenced legacy contract verifiers; their current assertions are covered by the owner convergence matrix,
boundary guard and focused LLM/Runtime tests.

## Remaining Active Work

- RUNTIME-LIVE: fresh-data, real-provider positive convergence through later CWI and terminal delivery.
- WRE-LIVE: deployed Workspace model/reasoning selection across Web/TUI, Resume/Restart and workspace isolation.

Neither item is closed by this deterministic/container record.
