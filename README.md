# workorch

Phase1 minimal skeleton, rectified to the go-multi-module-dev multi-module layout.

## Structure

- `application/workorchd`: executable entry
- `internal/modules/kernel/{workspace,run,decision,orchestrator}`: phase1 kernel modules
- `internal/modules/blocks/{approval,legacy}`: reusable/block modules
- `internal/pkg/{common,contracts,events,repository}`: shared internal contracts and primitives
- `docs/`: repo docs

## Phase1 semantics retained

- kernel: `workspace`, `run`, `decision`, `orchestrator`
- blocks: `approval`, `legacy`
- shared contracts/events/repository moved under `internal/pkg`
