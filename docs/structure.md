# workorch structure

This repository was normalized to the `go-multi-module-dev` layout with minimal semantic changes.

## Mapping

- old `application/*` service files -> `internal/modules/kernel/*/biz`
- old `domain/*` -> module-local `pkg/models`
- old `domain/shared` + `workorchcommon` -> `internal/pkg/common`
- old `workorchcontracts` -> `internal/pkg/contracts`
- old `workorchevents` -> `internal/pkg/events`
- old `repository` -> `internal/pkg/repository`
- old `adapters/legacy` runtime/tool adapters -> `internal/modules/blocks/legacy/{biz,service}`
- old `main.go` -> `application/workorchd/main.go`
