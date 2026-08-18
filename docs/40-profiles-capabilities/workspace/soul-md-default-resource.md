# Workspace Soul Markdown Default Resources

Status: active

Type: profile-capability

Last Verified: 2026-08-18

## Current Layout

```text
internal/pkg/workorchdefaults/defaults/
  souls.json
  souls/<template>/<role>.md
```

`internal/pkg/workorchdefaults` embeds and loads these files. `souls.json` contains metadata plus `soulMDRef`; each referenced
Markdown file provides the runtime `soulMd` body.

## Load Contract

- `soulMDRef` is required for embedded Souls and inline `soulMD` is rejected in the default JSON;
- the reference must be a relative `.md` path exactly below `souls/<template>/` and cannot contain traversal/empty segments;
- the target must exist and trim to non-empty content;
- the loader returns owner-neutral management DTOs and does not write repositories.

## Ownership

At startup, System coordinates `EnsureDefaultsCommand` defined by Soul owner. Soul owner validates and persists each default;
System, workorchd and the defaults package do not become Soul state owners.

Default Souls are seed-once, user-editable resources. Soul owner creates a built-in ID only when it is missing; a repeated
ensure preserves the entire existing object even when its content differs from the embedded declaration. Deleting a built-in
Soul makes it eligible for recreation on the next ensure. Updating embedded Markdown does not silently overwrite persisted
Souls and requires an explicit migration contract if a forced refresh is ever needed.

On the product surface, APIs and projections expose full `soulMd`; `soulMDRef` is an embedded-resource implementation detail.
Workspace templates store Soul IDs/bindings, not filesystem references.

## Runtime Boundary

Fresh activation resolves owner-managed Soul content into an immutable typed snapshot. Soul text is model guidance only: it
cannot grant operations, choose Runtime state, seal dispatch or accept evidence. Resume/recovery does not reread edited default
files into an existing Run.

## Validation

- all 36 default Soul references resolve and their ID/path roles agree;
- all bodies are non-empty;
- path traversal and inline-body fixtures fail closed;
- ensure is idempotent at Soul owner;
- active guidance must not demand a removed provider-facing authority schema.

All embedded bodies satisfy the last condition. Boundary/default-resource tests reject retired provider authority fields so a
future Soul edit cannot silently restore them.

Commands:

- `go test ./internal/pkg/workorchdefaults -count=1`
- `make verify-workspace-template-soul`

See [Soul routing](soul-routing.md) for activation selection and
[built-in templates](../builtin-templates-and-souls.md) for inventory.
