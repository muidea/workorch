# workorch Deployment

## Container image

Build the image:

```bash
docker build -t workorch:local .
```

Or use the repository target:

```bash
make docker-build
```

Run with the in-memory repository backend:

```bash
docker run --rm -p 8080:8080 workorch:local
```

Run with the file-backed repository backend:

```bash
mkdir -p .workorch-runtime
chmod 0777 .workorch-runtime
docker run --rm \
  -p 8080:8080 \
  -v "$(pwd)/config/container-file/application.toml:/etc/workorch/application.toml:ro" \
  -v "$(pwd)/.workorch-runtime:/var/lib/workorch" \
  workorch:local
```

With `config/container-file/application.toml`, internal runtime state is stored
under `/var/lib/workorch/.workorch-data/internal/workspaces/<workspace-id>/`.
When a workspace is created without an explicit `deliverableDirectory`, the
default user project root is created under
`/var/lib/workorch/.workorch-workspaces/<workspace-id>/`. Mounting the parent
`/var/lib/workorch` keeps both internal state and default deliverables
persistent.

The image starts with `/etc/workorch/application.toml` through the magicCommon
configuration manager. Runtime parameters should be managed through a
configuration directory and passed with `workorch --server -config-dir <dir>` for local
runs, or by mounting a replacement `application.toml` file in containers.

## Verification

After the container starts, verify the common service endpoints:

```bash
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8080/readyz
curl -f http://127.0.0.1:8080/metrics | grep workorch_http_requests_total
WORKSPACE_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d '{"title":"demo workspace","description":"demo workspace description"}')"
WORKSPACE_ID="$(printf '%s\n' "${WORKSPACE_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
test -n "${WORKSPACE_ID}"
GOAL_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/goals \
  -H 'Content-Type: application/json' \
  -d '{"text":"deliver local MVP orchestration"}')"
GOAL_ID="$(printf '%s\n' "${GOAL_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
DOD_JSON="$(curl -fsS -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/dods \
  -H 'Content-Type: application/json' \
  -d '{"items":["workspace created","run completed"]}')"
DOD_ID="$(printf '%s\n' "${DOD_JSON}" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p' | head -n1)"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"run-local-verify\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/run-local-verify/start
```

For the file-backed backend, also verify persisted read endpoints:

```bash
RUN_ID="run-demo-$(date +%s)"
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/snapshot
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/decisions
curl -f "http://127.0.0.1:8080/api/v1/workorch/decisions?workspaceId=${WORKSPACE_ID}"
curl -f "http://127.0.0.1:8080/api/v1/workorch/decisions?runId=${BLOCKED_RUN_ID}"
TICKET_ID="ticket-run-runtime-decision-block-demo-runtime-decision"
curl -f http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/decisions/${TICKET_ID}/resolve
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${BLOCKED_RUN_ID}/resume
APPROVAL_RUN_ID="run-runtime-approval-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\",\"goalId\":\"${GOAL_ID}\",\"doDId\":\"${DOD_ID}\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/approvals
curl -f "http://127.0.0.1:8080/api/v1/workorch/approvals?workspaceId=${WORKSPACE_ID}"
curl -f "http://127.0.0.1:8080/api/v1/workorch/approvals?runId=${APPROVAL_RUN_ID}"
APPROVAL_ID="approval-run-runtime-approval-block-demo-runtime-approval"
curl -f http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/approvals/${APPROVAL_ID}/approve
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${APPROVAL_RUN_ID}/resume
curl -f http://127.0.0.1:8080/api/v1/workorch/runs
curl -f "http://127.0.0.1:8080/api/v1/workorch/runs?workspaceId=${WORKSPACE_ID}"
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}
```

Automated verification entrypoints:

```bash
make verify-service
make verify-service-file
make verify-workspace-template-soul
make verify-workspace-template-soul-file
make verify-llm
make verify-llm-file
make verify-real-llm
make verify-real-vector-memory
make docker-verify
make docker-verify-file
```

All non-docker e2e verification targets connect to the currently deployed
`workorch --server` instance through `WORKORCH_BASE_URL` and must not build,
start, stop, or restart the service. The `*-file` targets are retained as
aliases for the same deployed-instance checks.

`make verify-workspace-template-soul` and
`make verify-workspace-template-soul-file` verify deployed Soul management,
Workspace Template management, and template-based Workspace setup with Goal,
DoD, and Workspace Soul binding read models.

`make verify-llm` and `make verify-llm-file` start only a local
OpenAI-compatible mock provider and verify the LLM-driven Workspace execution
loop through the deployed service API.

`make verify-real-llm` verifies an already running `workorch --server` service against
its configured real LLM provider. By default it targets
`http://127.0.0.1:8080`, reads the default ready provider from
`/api/v1/workorch/configuration`, creates a new Workspace / Goal / DoD, and
expects this execution chain:

```text
llm/workspace_execution
-> executionGraph
-> shell/command
-> llm/workspace_execution graph follow-up
-> evidence-backed dodAssessment
-> Run completed
```

The target service and provider can be overridden:

```bash
WORKORCH_REAL_LLM_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_REAL_LLM_PROVIDER_ID=local \
make verify-real-llm
```

The real-provider verification calls the configured model and may consume paid
provider quota. It writes request / response artifacts to a temporary directory
and prints that path on completion.

## Configuration file

Use `config/application.toml` as the baseline local configuration:

```bash
go run ./application/workorch --server -config-dir config
```

Use `config/file/application.toml` for the local file-backed backend:

```bash
rm -rf .workorch-data
go run ./application/workorch --server -config-dir config/file
```

Main sections:

- `[server]`: `addr`, timeout values, and `maxConcurrentRuns`
- `[repository]`: `backend` and `dataDir`
- `[runtime]`: runtime defaults such as `maxRuntimeRepairAttempts`,
  `maxProviderRetryAttempts`, and `shellCommandTimeout`
- `[llm]`: default LLM provider selection and named provider definitions

Runtime shell commands default to `runtime.shellCommandTimeout` when an
individual `shell/command` step does not provide `timeoutMillis`. The default
configuration uses `60s`; individual commands can still request a bounded
override with `timeoutMillis` up to 600000 milliseconds.

Runtime repair attempts default to `runtime.maxRuntimeRepairAttempts = 10`.
This budget controls state-changing repair, repair exhausted tickets, and
replan governance. Provider retry uses the separate
`runtime.maxProviderRetryAttempts` budget.

`CONFIG_PATH` can point to the configuration directory when command-line
arguments are not convenient. This follows the magicCommon
`framework/configuration` convention of loading `application.toml` and optional
module files from `config.d/`.

## LLM provider configuration

LLM-driven Workspace execution is enabled by configuring a provider before
starting `workorch --server`.

OpenAI SDK provider:

```toml
[llm]
defaultProvider = "reasoning"

[llm.providers.reasoning]
provider = "openai"
baseURL = "https://api.openai.com/v1"
apiKey = "..."
model = "..."
timeout = "60s"
disabled = false
```

DeepSeek, MiniMax, and other services confirmed to work with the OpenAI SDK
should also use `provider = "openai"` with their own `baseURL`.

Custom OpenAI-compatible endpoint:

```toml
[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
apiKey = "local-key"
model = "local-model"
timeout = "60s"
disabled = false
```

Supported LLM fields:

- `defaultProvider`: provider ID used when activation does not specify `llmProviderId`
- `providers.<id>.provider`: `openai` or `compatible`
- `providers.<id>.baseURL`: optional for `openai` and passed to the OpenAI SDK when set; required for `compatible`
- `providers.<id>.apiKey`: bearer token used only inside `blocks/llm`
- `providers.<id>.model`: model name sent to `/chat/completions`
- `providers.<id>.timeout`: request timeout, default `60s`
- `providers.<id>.disabled`: set to `true` to force explicit provider-unavailable failure

Current provider boundary:

- The primary decision protocol is OpenAI-compatible `/chat/completions`.
- Provider-declared streaming and non-chat operations are supported only when
  the selected provider advertises the required capability.
- Embedding-backed Memory recall is enabled only when an embedding provider is
  configured and available.
- LLM output must be one JSON object matching `LLMExecutionDecision`.
- OpenAI-compatible calls use JSON object mode where supported.
- Transport formatting drift is normalized only for markdown fences and first
  JSON object extraction; business schema validation remains strict.
- Schema-invalid output triggers one LLM repair retry and then either continues
  with a valid decision or fails with parse / repair diagnostics.
- The LLM cannot execute shell, patch, or MCP directly; it can only request
  those actions through structured decision output consumed by the orchestrator.
- For multi-step work, the LLM should emit `executionGraph`; the orchestrator
  owns graph traversal and dispatches a follow-up LLM step after graph leaf
  completion when a DoD assessment is required.

MCP runtime boundary:

- `transport=http` and `transport=https` invoke the configured endpoint with a
  governed HTTP POST.
- `transport=stdio` starts the configured endpoint as a local command, exchanges
  newline-delimited JSON-RPC over stdin/stdout, and performs `initialize`,
  `notifications/initialized`, then `tools/call`.
- `stdio` endpoint parsing is command plus whitespace-separated args. Shell
  expansion is not applied by workorch; use an explicit wrapper command when
  complex environment setup is required.
- Local verification and load-test scripts write managed MCP `stdio` endpoint
  helpers under `.workorch-data/system/mcp-endpoints/{mcpServerId}/` by default.
  Override `WORKORCH_MCP_ENDPOINT_DIR` when the deployed service uses a
  different data root or when endpoints should be shared outside the repository.
  Runtime artifacts and logs may still use temporary directories, but persisted
  MCP server endpoint paths should not point to deleted `/tmp` files.

## Acceptance expectation

A deployable `workorch --server` instance is acceptable when:

- `GET /healthz` returns `200`
- `GET /readyz` returns `200`
- `GET /metrics` returns service counters and gauges
- `make verify-llm` and `make verify-llm-file` pass against the deployed
  service with local mock-provider execution.
- `make verify-mcp-stdio` and `make verify-mcp-stdio-file` pass against the
  deployed service, proving the governed MCP `stdio` tool-call path through LLM
  graph execution.
- `make verify-workspace-template-soul` and
  `make verify-workspace-template-soul-file` pass against the deployed service,
  proving Soul management, Workspace Template management, and template-based
  Workspace setup with Goal / DoD / Soul binding persistence.
- `make verify-mvp` and `make verify-mvp-file` pass as the grouped local
  deployed-instance MVP verification bundles with the configured real LLM
  provider. CI uses `make verify-ci-mvp-file` instead.
- after deploying `workorch --server`, `make verify-real-llm` and
  `make verify-real-mcp-stdio` pass against the running service. The real MCP
  stdio check temporarily appends a local mock provider through the
  configuration API, restores provider config on exit, creates a governed MCP
  `stdio` server, and verifies the deployed process can execute it.
- `make verify-real-vector-memory` passes against the running service. It
  temporarily appends an embedding-capable mock provider through the
  configuration API, restores provider config on exit, creates vectorized
  workspace Memory, and verifies `memoryRetrieval` evidence on the activated
  Run.
- `make verify-real-weather` passes against the running service. It uses the
  configured real LLM provider, registers a managed local MCP `stdio` weather
  endpoint, queries Open-Meteo for Zhejiang Hangzhou current weather, and
  validates weather ToolCall, ValidationResult, RuntimeCheckpoint, and DoD
  evidence in the Run snapshot.
- `make verify-weather-ticket-approval` passes against the running service. It
  uses the configured real LLM provider and an MCP `stdio` weather tool,
  resolves a Decision Ticket with the specified city, approves the generated
  Approval, queries Open-Meteo, and validates Decision, Approval, ToolCall,
  ValidationResult, RuntimeCheckpoint, and DoD evidence in the Run snapshot.
  The target city can be changed with `WORKORCH_WEATHER_COUNTRY`,
  `WORKORCH_WEATHER_PROVINCE`, `WORKORCH_WEATHER_CITY`, and
  `WORKORCH_WEATHER_LOOKUP_NAME`.
- `make verify-weather-ticket-approval-mock` is the deterministic mock LLM
  variant for CI-style checks only. Local acceptance should use the real LLM
  target above so provider behavior and Web/manual governance assumptions are
  not hidden by a scripted provider.
- `make verify-ci-mvp-file` is the CI-safe grouped verification entrypoint. It
  intentionally uses mock LLM/MCP checks and does not run real LLM or external
  weather governance checks.
- `make verify-real-mvp` passes as the grouped verification entrypoint for an
  already running deployed service, covering real LLM execution, MCP `stdio`,
  and vector Memory recall.
- `POST /api/v1/workorch/workspaces` creates one `workorch` workspace instance, accepts an optional `description`, defaults `deliverableDirectory` to `.workorch-workspaces/<generated-workspace-id>` for the file backend when omitted, stores runtime state under `repository.dataDir/internal/workspaces/<workspace-id>`, and returns the generated `workspace.id`
- with file-backed repository config, `GET /api/v1/workorch/workspaces` returns the current `workorch` workspace list
- with file-backed repository config, `GET /api/v1/workorch/workspaces/{workspaceId}` returns one persisted `workorch` workspace instance
- with file-backed repository config, `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot` returns one aggregated `workorch` workspace read model
- with file-backed repository config, `POST /api/v1/workorch/workspaces/{workspaceId}/runs` creates one persisted `workorch` run instance
- with file-backed repository config, `GET /api/v1/workorch/runs` returns persisted `workorch` run instances and supports `workspaceId` filtering
- with file-backed repository config, `GET /api/v1/workorch/runs/{runId}` returns one persisted `workorch` run instance
- with file-backed repository config, `GET /api/v1/workorch/runs/{runId}/snapshot` returns one aggregated `workorch` run read model
- with file-backed repository config, `GET /api/v1/workorch/runs/{runId}/status` returns one summarized `workorch` run status
- with file-backed repository config, `POST /api/v1/workorch/runs/{runId}/start` advances one persisted `workorch` run instance and returns its updated state
- with file-backed repository config, `GET /api/v1/workorch/decisions` returns persisted decision tickets and supports `workspaceId` / `runId` filtering
- with file-backed repository config, `GET /api/v1/workorch/decisions/{ticketId}` returns one persisted decision ticket
- with file-backed repository config, `POST /api/v1/workorch/decisions/{ticketId}/resolve` resolves one decision ticket with `provide_missing_info`, `reject_current_path`, or `replan`
- with file-backed repository config, `POST /api/v1/workorch/decisions/resolve-batch` resolves multiple open decision tickets after prevalidating the whole batch
- with file-backed repository config, `POST /api/v1/workorch/runs/{runId}/resume` can complete after decision resolve or fail terminally after decision reject
- with file-backed repository config, `GET /api/v1/workorch/approvals` returns persisted approval items and supports `workspaceId` / `runId` filtering
- with file-backed repository config, `GET /api/v1/workorch/approvals/{approvalId}` returns one persisted approval item
- with file-backed repository config, `POST /api/v1/workorch/approvals/{approvalId}/approve` approves one approval item
- with file-backed repository config, `POST /api/v1/workorch/approvals/{approvalId}/reject` rejects one approval item
- with file-backed repository config, `POST /api/v1/workorch/approvals/resolve-batch` resolves multiple pending approval items after prevalidating the whole batch
- with file-backed repository config, `POST /api/v1/workorch/runs/{runId}/resume` can complete after approval approve or fail terminally after approval reject
- with file-backed repository config, repository state is persisted under the configured `dataDir`
- with file-backed repository config, restarting `workorch --server` on the same `dataDir` preserves workspace / run / decision / approval terminal read paths
- with configured real LLM provider, `make verify-real-llm` completes a
  graph-backed Workspace execution with shell evidence and a satisfied
  evidence-backed `dodAssessment`
