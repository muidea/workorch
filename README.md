# workorch

Phase1 minimal skeleton, rectified to the go-multi-module-dev module-oriented layout.

## Structure

- `application/workorchd`: executable entry
- `application/workorchd/bootstrap`: backend/runtime assembly and repository backend selection
- `application/workorchd/demo`: phase1 runnable demo assembly and acceptance slices
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: phase1 kernel modules
- `internal/modules/blocks/{approval,legacy}`: reusable/block modules
- `internal/pkg/{common,events,repository}`: shared internal primitives
- `docs/`: repo docs
- `.github/workflows/ci.yml`: GitHub Actions CI workflow
- `Makefile`: local and CI entry targets

Each module is now normalized around the go-multi-module-dev minimum shape:

- `module.go`: module-level constructor/export entry
- `biz/`: business logic
- `service/`: module-facing contract layer or adapter-facing surface
- `pkg/common`: module-local constants
- `pkg/models`: module-local models when the module owns domain data

The application layer is also split by responsibility:

- `main.go`: process entry and output loop
- `bootstrap/`: backend assembly, repository wiring, and runtime construction
- `demo/`: phase1 acceptance scenarios and snapshot composition
- `presenter/`: observable output rendering

Inside `demo/`, responsibilities are further separated:

- `demo.go`: scenario registration and top-level execution
- `scenarios.go`: phase1 scenario flow
- `snapshot.go`: snapshot loading/composition

## Phase1 semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`
- blocks: `approval`, `legacy`
- shared common/events/repository primitives moved under `internal/pkg`

## Development workflow

This repository is still a single `go.mod` codebase. Here, `go-multi-module-dev` means module-oriented directory organization and dependency boundaries inside one repository, not a split into multiple Go modules yet.

Common entrypoints:

```bash
make fmt
make fmt-check
make vet
make test
make test-race
make build
make run
make run-file
make ci
```

Notes:

- `make build` writes the binary to `./bin/workorchd`
- `make run-file` runs the demo with the file-backed repository backend
- file-backed demo execution is now repeatable against the same data directory for the shipped phase1 demo IDs
- `make ci` is the local equivalent of the GitHub Actions baseline

## Phase1 acceptance demo

Run:

```bash
make run
# or
go run ./application/workorchd
```

Optional file-backed demo run:

```bash
make run-file
# or
WORKORCH_REPOSITORY_BACKEND=file WORKORCH_DATA_DIR=.workorch-data go run ./application/workorchd
```

Expected observable output:

- `phase1 runnable slice`: shows workspace/run/step reaching `completed`
- `phase1 decision-blocked-resume slice`: shows a decision ticket being created, resolved, then the run resuming to `completed`
- `phase1 approval-blocked-resume slice`: shows an approval gate being created, approved, then the run resuming to `completed`
- `phase1 approval-rejected-terminal slice`: shows an approval gate being rejected, then the run reaching `failed` without dispatching a runtime step
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
