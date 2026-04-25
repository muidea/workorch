# workorch Deployment

## Container image

Build the image:

```bash
docker build -t workorchd:local .
```

Or use the repository target:

```bash
make docker-build
```

Run with the in-memory repository backend:

```bash
docker run --rm -p 8080:8080 workorchd:local
```

Run with the file-backed repository backend:

```bash
mkdir -p .workorch-data
chmod 0777 .workorch-data
docker run --rm \
  -p 8080:8080 \
  -e WORKORCH_REPOSITORY_BACKEND=file \
  -e WORKORCH_DATA_DIR=/var/lib/workorch \
  -v "$(pwd)/.workorch-data:/var/lib/workorch" \
  workorchd:local
```

## Verification

After the container starts, verify the common service endpoints:

```bash
WORKSPACE_ID="ws-demo-$(date +%s)"
curl -f http://127.0.0.1:8080/healthz
curl -f http://127.0.0.1:8080/readyz
curl -f http://127.0.0.1:8080/metrics | grep workorch_http_requests_total
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces \
  -H 'Content-Type: application/json' \
  -d "{\"workspaceId\":\"${WORKSPACE_ID}\",\"title\":\"demo workspace\"}"
curl -f http://127.0.0.1:8080/api/v1/workorch/slices
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/runs
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs
```

For the file-backed backend, also verify persisted read endpoints:

```bash
RUN_ID="run-demo-$(date +%s)"
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}
curl -f http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/snapshot
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${RUN_ID}\",\"goalId\":\"goal-demo\",\"doDId\":\"dod-demo\"}"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/start
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/snapshot
curl -f http://127.0.0.1:8080/api/v1/workorch/runs/${RUN_ID}/status
BLOCKED_RUN_ID="run-runtime-decision-block-demo"
curl -f -X POST http://127.0.0.1:8080/api/v1/workorch/workspaces/${WORKSPACE_ID}/runs \
  -H 'Content-Type: application/json' \
  -d "{\"runId\":\"${BLOCKED_RUN_ID}\"}"
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
  -d "{\"runId\":\"${APPROVAL_RUN_ID}\"}"
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
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/snapshots
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/statuses
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/status
curl -f http://127.0.0.1:8080/api/v1/workorch/slices/workorch-runnable/snapshot
```

Automated verification entrypoints:

```bash
make verify-service
make verify-service-file
make docker-verify
make docker-verify-file
```

## Environment variables

Use `.env.workorchd.example` as the baseline configuration template.
For file-backed container runs, ensure the mounted host directory is writable by the container user.

Primary variables:

- `WORKORCH_SERVER_ADDR`
- `WORKORCH_REPOSITORY_BACKEND`
- `WORKORCH_DATA_DIR`
- `WORKORCH_SERVER_READ_HEADER_TIMEOUT`
- `WORKORCH_SERVER_READ_TIMEOUT`
- `WORKORCH_SERVER_WRITE_TIMEOUT`
- `WORKORCH_SERVER_IDLE_TIMEOUT`
- `WORKORCH_SERVER_SHUTDOWN_TIMEOUT`
- `WORKORCH_SERVER_MAX_CONCURRENT_RUNS`

## Acceptance expectation

A deployable `workorchd` instance is acceptable when:

- `GET /healthz` returns `200`
- `GET /readyz` returns `200`
- `GET /metrics` returns service counters and gauges
- `POST /api/v1/workorch/workspaces` creates one `workorch` workspace instance
- `GET /api/v1/workorch/slices` returns the executable `workorch` slice catalog
- `POST /api/v1/workorch/slices/{sliceId}/runs` returns the requested single `workorch` slice
- `POST /api/v1/workorch/runs` returns the current 17 `workorch` slices
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/workspaces` returns the current `workorch` workspace list
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/workspaces/{workspaceId}` returns one persisted `workorch` workspace instance
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot` returns one aggregated `workorch` workspace read model
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/workspaces/{workspaceId}/runs` creates one persisted `workorch` run instance
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/runs` returns persisted `workorch` run instances and supports `workspaceId` filtering
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/runs/{runId}` returns one persisted `workorch` run instance
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/runs/{runId}/snapshot` returns one aggregated `workorch` run read model
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/runs/{runId}/status` returns one summarized `workorch` run status
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/runs/{runId}/start` advances one persisted `workorch` run instance and returns its updated state
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/decisions` returns persisted decision tickets and supports `workspaceId` / `runId` filtering
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/decisions/{ticketId}` returns one persisted decision ticket
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/decisions/{ticketId}/resolve` resolves one decision ticket
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/decisions/{ticketId}/reject` rejects one decision ticket
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/runs/{runId}/resume` can complete after decision resolve or fail terminally after decision reject
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/approvals` returns persisted approval items and supports `workspaceId` / `runId` filtering
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/approvals/{approvalId}` returns one persisted approval item
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/approvals/{approvalId}/approve` approves one approval item
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/approvals/{approvalId}/reject` rejects one approval item
- with `WORKORCH_REPOSITORY_BACKEND=file`, `POST /api/v1/workorch/runs/{runId}/resume` can complete after approval approve or fail terminally after approval reject
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/slices/snapshots` returns the currently available persisted `workorch` slice snapshots
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/slices/statuses` returns the currently available persisted `workorch` slice summaries
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/slices/{sliceId}/status` returns the current `workorch` slice status summary when persisted state exists
- with `WORKORCH_REPOSITORY_BACKEND=file`, `GET /api/v1/workorch/slices/{sliceId}/snapshot` returns the current persisted `workorch` slice snapshot when persisted state exists
- with `WORKORCH_REPOSITORY_BACKEND=file`, repository state is persisted under the configured `WORKORCH_DATA_DIR`
