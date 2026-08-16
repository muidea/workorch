#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${WORKORCH_VERIFY_PORT:-18081}"
BASE_URL="http://127.0.0.1:$PORT"
IMAGE_TAG="${WORKORCH_VERIFY_IMAGE:-workorch:verify}"
STORAGE_MODE="${WORKORCH_VERIFY_STORAGE_MODE:-ephemeral}"
MOCK_PORT="${WORKORCH_VERIFY_MOCK_PORT:-18193}"
MOCK_ADDR="0.0.0.0:$MOCK_PORT"
MOCK_BASE_URL="http://host.docker.internal:$MOCK_PORT/v1"
TMP_DIR="$(mktemp -d)"
CONFIG_DIR="$TMP_DIR/config"
DATA_DIR="$TMP_DIR/data"
CONTAINER_ID=""
MOCK_PID=""
WORKSPACE_ID=""
RUN_ID=""
DOCKER_GOPROXY="${WORKORCH_DOCKER_GOPROXY:-$(go env GOPROXY)}"
DOCKER_BUILD_NETWORK="${WORKORCH_DOCKER_BUILD_NETWORK:-default}"

if [[ "$STORAGE_MODE" != "ephemeral" && "$STORAGE_MODE" != "persistent" ]]; then
  echo "WORKORCH_VERIFY_STORAGE_MODE must be ephemeral or persistent" >&2
  exit 2
fi

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "container verification failed storageMode=$STORAGE_MODE artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $(basename "$file") ---" >&2
    python3 -m json.tool "$file" >&2 || sed -n '1,200p' "$file" >&2
  done
  if [[ -n "$CONTAINER_ID" ]]; then
    echo "--- container log ---" >&2
    docker logs "$CONTAINER_ID" >&2 || true
  fi
}

cleanup() {
  if [[ -n "$CONTAINER_ID" ]]; then
    docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true
    CONTAINER_ID=""
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ -d "$DATA_DIR" ]] && docker image inspect "$IMAGE_TAG" >/dev/null 2>&1; then
    docker run --rm --user 0 --entrypoint /bin/chmod -v "$DATA_DIR:/data" "$IMAGE_TAG" -R a+rwX /data >/dev/null 2>&1 || true
  fi
  docker image rm "$IMAGE_TAG" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR" || true
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chmod 0777 "$DATA_DIR"

cat >"$TMP_DIR/provider.py" <<'PY'
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

host, port = sys.argv[1].rsplit(":", 1)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path.split("?", 1)[0] not in {"/models", "/v1/models"}:
            self.send_error(404)
            return
        payload = {"object": "list", "data": [{
            "id": "verify-model",
            "object": "model",
            "owned_by": "verify",
            "supported_endpoints": ["/v1/responses"],
            "contextWindowTokens": 128000,
            "maxOutputTokens": 8192,
        }]}
        body = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        # The packaging smoke only needs activation materialization. Returning
        # a bounded provider failure lets the Runtime fail closed asynchronously.
        body = json.dumps({"error": {"message": "container smoke provider has no candidate document"}}).encode()
        self.send_response(503)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass

ThreadingHTTPServer((host, int(port)), Handler).serve_forever()
PY

python3 "$TMP_DIR/provider.py" "$MOCK_ADDR" >"$TMP_DIR/provider.log" 2>&1 &
MOCK_PID=$!

cat >"$CONFIG_DIR/application.toml" <<EOF
endpointName = "workorch"

[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "file"
dataDir = "/var/lib/workorch/.workorch-data"

[runtime]
maxRuntimeRepairAttempts = 2
maxProviderRetryAttempts = 1
shellCommandTimeout = "30s"

[runtime.git]
defaultTimeout = "30s"

[runtime.llmOutput]
maxDocumentRecoveryAttempts = 1

[llm.aiProxy]
baseURL = "$MOCK_BASE_URL"
apiKey = "container-smoke"
timeout = "10s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "verify-model"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
EOF

start_container() {
  local args=(run -d --add-host=host.docker.internal:host-gateway -p "$PORT:8080" -v "$CONFIG_DIR/application.toml:/etc/workorch/application.toml:ro")
  if [[ "$STORAGE_MODE" == "persistent" ]]; then
    args+=(-v "$DATA_DIR:/var/lib/workorch")
  fi
  args+=("$IMAGE_TAG")
  CONTAINER_ID="$(docker "${args[@]}")"
}

wait_ready() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if curl -fsS --connect-timeout 2 --max-time 5 "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"; then
      return
    fi
    sleep 0.5
  done
  return 1
}

post_json() {
  local url="$1"
  local payload="$2"
  local output="$3"
  local expected="$4"
  local status
  status="$(curl -sS --connect-timeout 5 --max-time 30 -o "$output" -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d "$payload" "$url")"
  if [[ "$status" != "$expected" ]]; then
    echo "POST $url returned $status, want $expected" >&2
    return 1
  fi
}

cd "$ROOT_DIR"
docker build --network="$DOCKER_BUILD_NETWORK" --build-arg "GOPROXY=$DOCKER_GOPROXY" -t "$IMAGE_TAG" .
start_container
wait_ready

curl -fsS "$BASE_URL/healthz" >"$TMP_DIR/healthz.json"
curl -fsSL "$BASE_URL/console/" >"$TMP_DIR/console.html"
grep -Eqi '<html|<!doctype' "$TMP_DIR/console.html"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"container Runtime smoke","deliverableDirectory":"/srv/workorch/workspaces/container-smoke","templateId":"generic-governed"}' \
  "$TMP_DIR/workspace.json" 201
WORKSPACE_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["workspace"]["id"])' "$TMP_DIR/workspace.json")"

post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" \
  '{"text":"Produce one bounded container smoke artifact through the typed Runtime path."}' \
  "$TMP_DIR/goal-create.json" 201
post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" \
  '{"items":["A current Run is created through workspace execution activation and remains queryable."]}' \
  "$TMP_DIR/dod-create.json" 201

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" >"$TMP_DIR/goals.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" >"$TMP_DIR/dods.json"
python3 - "$TMP_DIR/goals.json" "$TMP_DIR/dods.json" <<'PY'
import json
import sys
if not json.load(open(sys.argv[1])).get("goals"):
    raise SystemExit("workspace Goal did not persist")
if not json.load(open(sys.argv[2])).get("dods"):
    raise SystemExit("workspace DoD did not persist")
PY

post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/execution-activations" \
  '{"intent":"restart","requestId":"container-runtime-smoke","reason":"container_smoke","source":"docker_verifier"}' \
  "$TMP_DIR/activation.json" 202
python3 - "$TMP_DIR/activation.json" <<'PY'
import json
import sys
item = json.load(open(sys.argv[1]))
if item.get("accepted") is not True or item.get("intent") != "restart":
    raise SystemExit(f"unexpected activation response: {item!r}")
PY

deadline=$((SECONDS + 30))
while (( SECONDS <= deadline )); do
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" >"$TMP_DIR/runs.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/snapshot" >"$TMP_DIR/workspace-snapshot.json"
  RUN_ID="$(python3 -c 'import json,sys; runs=json.load(open(sys.argv[1])).get("runs") or []; print(runs[-1].get("id", "") if runs else "")' "$TMP_DIR/runs.json")"
  [[ -n "$RUN_ID" ]] && break
  sleep 0.5
done
[[ -n "$RUN_ID" ]]

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID" >"$TMP_DIR/run.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/status" >"$TMP_DIR/status.json"

post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" '{}' "$TMP_DIR/removed-run-create.json" 404
post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/start" '{}' "$TMP_DIR/removed-run-start.json" 404
post_json "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/resume" '{}' "$TMP_DIR/removed-run-resume.json" 404

if [[ "$STORAGE_MODE" == "persistent" ]]; then
  docker rm -f "$CONTAINER_ID" >/dev/null
  CONTAINER_ID=""
  start_container
  wait_ready
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID" >"$TMP_DIR/workspace-after-restart.json"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID" >"$TMP_DIR/run-after-restart.json"
fi

echo "container verification passed storageMode=$STORAGE_MODE workspaceId=$WORKSPACE_ID runId=$RUN_ID"
