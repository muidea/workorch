# workorch structure

This repository was normalized to the `go-multi-module-dev` layout with minimal semantic changes.

It remains a single `go.mod` repository. The current normalization target is module-oriented structure consistency inside one repo, not a physical split into multiple Go modules.

## Mapping

- `application/workorchd/main.go` -> thin process entry that starts `magicCommon/framework/service.DefaultService` and explicitly loads `workorchruntime`, `internal/modules/kernel/*`, and `internal/modules/blocks/*`
- `internal/initiators/workorchruntime` -> magicCommon initiator that provides repository, event hub, and cross-module service wiring for plugin-loaded workorch modules
- `internal/modules/application/workorchd` -> application plugin module loaded by the magicCommon module lifecycle
- `internal/modules/application/workorchd/biz/bootstrap` -> repository/backend wiring, runtime assembly, and a thin `Runtime` shell around grouped `Exports()` plus module lifecycle teardown
- `internal/modules/application/workorchd/biz/appservice` -> application-level orchestration / workflow composition, grouped runtime adapter, managed runtime wrapper, workspace writes kept behind module service, runtime usage split into guard/snapshot read faces plus workspace/run/decision/approval action faces, adapter dependencies grouped as reads/actions capabilities, exports-to-runtime mapping centralized in a single helper, managed runtime assembly centralized in a single helper, and repeated runtime lifecycle handling unified through a session helper
- `internal/modules/application/workorchd/pkg/observed` -> workorch acceptance observed adapter, explicit workorch event-to-snapshot contract mapper, typed snapshot contract, and collector that now consumes typed/validated/final-version workorch event helpers from `internal/pkg/events`
- `internal/modules/application/workorchd/pkg/presenter` -> observable output rendering
- old `application/*` service files -> `internal/modules/kernel/*/biz`
- old `domain/*` -> module-local `pkg/models`
- old `domain/shared` + `workorchcommon` -> `internal/pkg/common`
- old `workorchevents` -> `internal/pkg/events` including shared workorch event payload builders/keys, typed workorch event helpers, strict workorch version rules, and topic-scoped contract lookup
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
- let the application entry explicitly choose which initiators and modules are loaded through imports
- move application-specific implementation to `internal/modules/application/<app>/{biz,service,pkg}`
- expose lifecycle-bearing app runtime through `internal/modules/application/<app>/module.go`
- let env-backed execution entry live in `appservice` when the flow itself is application business orchestration
- let application-specific runtime adapters live in `appservice`, not in `bootstrap`
- let `bootstrap` export grouped capabilities such as `Repositories` / `Services` / `Infrastructure`, rather than a flat field bag
- let application-layer writes prefer module `service/` entrypoints; repository direct access should be read-side first
- let observed/event snapshot assembly live outside `presenter` when it represents application semantics rather than rendering
- keep rendering / presentation adapters separate from runtime assembly
- keep `demo` as a compatibility shim unless it regains independent responsibilities
