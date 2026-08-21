# WorkOrch Deployment

Status: normative

Type: product-interface

Last Verified: 2026-08-21

## Build And Run

```bash
make deps
make build

# local TUI (default mode)
./bin/workorch -config-dir config

# HTTP/Web service
./bin/workorch --server -config-dir config
```

`go run ./application/workorch ...` is equivalent for development. `--remote <base-url>` selects remote CLI/TUI mode.

Configuration directory precedence is explicit `-config-dir`/`--config-dir`, then `WORKORCH_CONFIG_DIR`, then
`$HOME/.workorch`. The directory contains `application.toml`. `CONFIG_PATH` is an internal framework/image environment value,
not the documented CLI selection mechanism.

## Supplied Configuration Sets

| Directory/file | Purpose |
| --- | --- |
| `config/application.toml` | minimal local file-backed service defaults |
| `config/file/application.toml` | extended development file-backed config, including ai-proxy/runtime/governance settings |
| `config/container-file/application.toml` | container file-backed config rooted at `/var/lib/workorch` |

Canonical LLM config is ai-proxy-only: `llm.aiProxy`, `llm.defaultChat`, optional `llm.defaultEmbedding`, and `llm.budget`.
Secrets should be injected through a protected generated config or secret mount; public configuration responses are redacted.

The LLM owner freezes its active configuration during process startup. Management writes update persisted configuration only;
when persisted and active revisions differ, the projection reports `restartRequired`. Catalog refresh must not inject persisted
changes into the active owner, and an already sealed Run never changes provider selection on Resume.

SSH execution is optional and defaults to an empty catalog。`[ssh.targets]`、`[ssh.sessions]`与`[ssh.credentials]`构成一个closed
catalog；Runtime只使用opaque refs。credential只能引用environment或`ssh.secretsDirectory`下的relative private-key file，
不支持inline password/private key或任意key path；owner在process start解析并冻结配置，变更需要restart。exact schema和权限
规则见[SSH component contract](../../internal/modules/blocks/ssh/readme.md)。

## Data Layout

- repository `backend=file` stores internal state below configured `repository.dataDir`;
- default generated deliverables use the sibling `.workorch-workspaces/<workspace-id>` tree, not the internal repository tree;
- explicit Workspace deliverable directories remain user-owned locations;
- `.workorch-data`, `.workorch-workspaces`, `bin` and `node_modules` are runtime/generated data and must not be committed.

`make run-file` deliberately deletes `./.workorch-data` before starting with `config/file`; use it only for a clean local run.

## Container Behavior

The checked-in image entrypoint runs `workorch --server -config-dir /etc/workorch` and the bundled config selects the file
backend at `/var/lib/workorch/.workorch-data`.

- `make docker-run` uses that file backend inside an ephemeral container; data disappears with `--rm`.
- `make docker-run-file` mounts host `./.workorch-data` at `/var/lib/workorch` for persistence.
- the service runs as an unprivileged `workorch` user and exposes port 8080.

The builder copies `web/consoleassets`, caches `go mod download`, and accepts `WORKORCH_DOCKER_GOPROXY` plus
`WORKORCH_DOCKER_BUILD_NETWORK` overrides for constrained build networks. The container verifier uses Workspace/Goal/DoD plus
`execution-activations`, asserts removed Run mutations return `404`, and verifies ephemeral and mounted file-backed modes.
Both modes are enforced by the current container verifier; dated run records are intentionally not retained in `docs/`.

## Operational Checks

- liveness: `GET /healthz`
- readiness: `GET /readyz`
- metrics: `GET /metrics`
- console: `GET /console`

The current service has no documented multi-tenant authentication boundary. Do not expose it to an untrusted network without
an external access-control/reverse-proxy layer.

## Verification

```bash
make fmt-check
make vet
make test
make verify-boundary-contract
make build
```

Deployed acceptance targets require an already running service unless their script explicitly manages one. Real-provider tests
also require matching ai-proxy credentials and should not be treated as CI-safe.
