# workorch structure

This repository was normalized to the `go-multi-module-dev` layout with minimal semantic changes.

It remains a single `go.mod` repository. The current normalization target is module-oriented structure consistency inside one repo, not a physical split into multiple Go modules.

## Mapping

- `application/workorchd/main.go` -> thin process entry only
- `application/workorchd/bootstrap` -> repository/backend wiring and runtime assembly
- `application/workorchd/demo` -> acceptance scenarios only
- `application/workorchd/presenter` -> observable output rendering
- old `application/*` service files -> `internal/modules/kernel/*/biz`
- old `domain/*` -> module-local `pkg/models`
- old `domain/shared` + `workorchcommon` -> `internal/pkg/common`
- old `workorchevents` -> `internal/pkg/events`
- old `repository` -> `internal/pkg/repository`
- old `adapters/legacy` runtime/tool adapters -> `internal/modules/blocks/legacy/{biz,service}`
- old `main.go` -> `application/workorchd/main.go`

## Module Shape

Each module should converge on the minimum `go-multi-module-dev` shape:

- `module.go`: module-level entry and constructor re-exports
- `biz/`: business implementation
- `service/`: module-facing service contract or external adapter surface
- `pkg/common`: module-local constants such as `ModuleID`
- `pkg/models`: module-local entities / DTO / view model when needed

For `application/{app}`, prefer the same separation idea:

- keep `main.go` as thin as possible
- move assembly/bootstrap/demo flow into sibling packages under the app directory
- keep rendering / presentation adapters separate from runtime assembly
- if `demo` grows, split scenario execution and snapshot/view assembly into separate files or subpackages
