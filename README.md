# workorch

Phase1 minimal skeleton, rectified to the go-multi-module-dev multi-module layout.

## Structure

- `application/workorchd`: executable entry
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: phase1 kernel modules
- `internal/modules/blocks/{approval,legacy}`: reusable/block modules
- `internal/pkg/{common,contracts,events,repository}`: shared internal contracts and primitives
- `docs/`: repo docs
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

## Phase1 semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`
- blocks: `approval`, `legacy`
- shared contracts/events/repository moved under `internal/pkg`

## Phase1 acceptance demo

Run:

```bash
make run
# or
go run ./application/workorchd
```

Expected observable output:

- `phase1 runnable slice`: shows workspace/run/step reaching `completed`
- `phase1 decision-blocked-resume slice`: shows a decision ticket being created, resolved, then the run resuming to `completed`
- `phase1 approval-blocked-resume slice`: shows an approval gate being created, approved, then the run resuming to `completed`
- `observed`: emits minimal envelope-style JSON snapshots for workspace/run/step/blocking objects, only as acceptance output, not a full event bus

## Development targets

```bash
make fmt
make vet
make test
make build
make ci
```

## Docs

- `docs/structure.md`: layout normalization mapping
- `docs/phase1-status.md`: delivered scope, acceptance baseline, and next batch boundary
