#!/usr/bin/env bash

# Exercises the real ai-proxy process and a real WorkOrch server without using
# external credentials. The Python process below is only the deterministic
# upstream model; model cataloguing and operation enforcement remain owned by
# ai-proxy.
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AI_PROXY_ROOT="${AI_PROXY_ROOT:-/home/rangh/aispace/ai-proxy}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
# Live outputs stay local/ephemeral by default (not committed under docs/).
ARTIFACT_DIR="${WORKORCH_AI_PROXY_LIVE_ARTIFACT_DIR:-${TMPDIR:-/tmp}/workorch-live-artifacts/ai-proxy-primary-llm-live-$STAMP}"
UPSTREAM_ADDR="${WORKORCH_AI_PROXY_LIVE_UPSTREAM_ADDR:-127.0.0.1:18182}"
AI_PROXY_ADDR="${WORKORCH_AI_PROXY_LIVE_PROXY_ADDR:-127.0.0.1:18183}"
WORKORCH_ADDR="${WORKORCH_AI_PROXY_LIVE_WORKORCH_ADDR:-127.0.0.1:18080}"
KEEP_ARTIFACTS="${WORKORCH_AI_PROXY_LIVE_KEEP_ARTIFACTS:-1}"

UPSTREAM_PID=""
AI_PROXY_PID=""
WORKORCH_PID=""
BUILD_DIR=""

fail_report() {
  local status=$?
  if [[ $status -ne 0 ]]; then
    echo "ai-proxy live provider verification failed; artifacts=$ARTIFACT_DIR" >&2
    for log in "$ARTIFACT_DIR"/*.log; do
      [[ -f "$log" ]] || continue
      echo "--- $log ---" >&2
      tail -n 100 "$log" >&2 || true
    done
  fi
}

stop_process() {
  local pid="$1"
  [[ -n "$pid" ]] || return 0
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
}

cleanup() {
  stop_process "$WORKORCH_PID"
  stop_process "$AI_PROXY_PID"
  stop_process "$UPSTREAM_PID"
  [[ -z "$BUILD_DIR" ]] || rm -rf "$BUILD_DIR"
  if [[ "$KEEP_ARTIFACTS" != "1" && "$KEEP_ARTIFACTS" != "true" ]]; then
    rm -rf "$ARTIFACT_DIR"
  fi
}

wait_http() {
  local url="$1"
  local name="$2"
  for _ in $(seq 1 100); do
    if curl --fail --silent --show-error "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  echo "$name did not become ready: $url" >&2
  return 1
}

assert_json() {
  local file="$1"
  local assertion="$2"
  python3 - "$file" "$assertion" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
assertion = sys.argv[2]
models = {
    item.get("id") or item.get("modelId"): item
    for item in data.get("data", data.get("catalog", {}).get("models", []))
}

if assertion == "catalog":
    expected = {
        "live-chat-a": ("chat_completions", 65536),
        "live-chat-b": ("chat_completions", 131072),
        "live-embedding": ("embeddings", 8192),
    }
    missing = []
    for model_id, (operation, ceiling) in expected.items():
        model = models.get(model_id)
        if not model or operation not in (model.get("operations") or []) or int(model.get("contextWindowTokens") or 0) != ceiling:
            missing.append(model_id)
    if missing:
        raise SystemExit(f"catalog missing or invalid models: {missing}; data={data}")
elif assertion == "chat":
    if data.get("choices", [{}])[0].get("message", {}).get("content") != "live chat response":
        raise SystemExit(f"unexpected chat response: {data}")
elif assertion == "embedding":
    vector = data.get("data", [{}])[0].get("embedding")
    if vector != [0.125, 0.25, 0.5]:
        raise SystemExit(f"unexpected embedding response: {data}")
PY
}

json_field() {
  local file="$1"
  local path="$2"
  python3 - "$file" "$path" <<'PY'
import json
import sys

value = json.load(open(sys.argv[1], encoding="utf-8"))
for key in sys.argv[2].split("."):
    value = value[int(key)] if isinstance(value, list) else value[key]
print(value)
PY
}

trap fail_report ERR
trap cleanup EXIT

[[ -d "$AI_PROXY_ROOT" ]] || { echo "AI_PROXY_ROOT does not exist: $AI_PROXY_ROOT" >&2; exit 2; }
mkdir -p "$ARTIFACT_DIR/config" "$ARTIFACT_DIR/data"
BUILD_DIR="$(mktemp -d)"

cat >"$ARTIFACT_DIR/fake_upstream.py" <<'PY'
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

host, port = os.environ["UPSTREAM_ADDR"].rsplit(":", 1)
request_log = os.environ["REQUEST_LOG"]

def runtime_candidate_response(payload):
    messages = payload.get("messages") or []
    content = "\n".join(str(item.get("content") or "") for item in messages if item.get("role") == "user")
    marker = "### C3.runtime-authority\n"
    if marker not in content:
        return None
    authority = json.loads(content.split(marker, 1)[1])
    decision = authority["decision"]
    focus = decision["Focus"]
    action = decision["Action"]
    coverage = focus["CoverageWorkItemIDs"]
    if action == "produce_validation":
        return {"operations": [{
            "id": "live-validation",
            "kind": "file/read",
            "coverageWorkItemIds": coverage,
            "targetRefs": ["live-validation-probe.txt"],
            "parameters": [
                {"name": "path", "string": "live-validation-probe.txt"},
                {"name": "optional", "boolean": True},
                {"name": "maxBytes", "integer": 1024},
            ],
        }]}
    if action == "produce_assessment":
        accepted = authority.get("context", {}).get("evidence", {}).get("Accepted") or []
        refs = [item.get("ID") for item in accepted if item.get("ID")]
        if not refs:
            raise ValueError("assessment request did not contain accepted evidence")
        return {
            "focusId": focus["ID"],
            "coverageWorkItemIds": coverage,
            "satisfied": True,
            "summary": "deterministic live validation evidence is accepted",
            "evidenceRefs": refs[:1],
        }
    raise ValueError("unsupported Runtime V2 action: " + str(action))

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size) or b"{}")
        with open(request_log, "a", encoding="utf-8") as handle:
            handle.write(json.dumps({"path": self.path, "model": payload.get("model"), "request": payload}, separators=(",", ":")) + "\n")
        if self.path == "/v1/chat/completions":
            candidate = runtime_candidate_response(payload)
            content = json.dumps(candidate, separators=(",", ":")) if candidate is not None else "live chat response"
            response = {"choices": [{"message": {"content": content}}], "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}}
        elif self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.125, 0.25, 0.5]}], "usage": {"prompt_tokens": 3, "total_tokens": 3}}
        else:
            self.send_response(404)
            self.end_headers()
            return
        encoded = json.dumps(response).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, _format, *_args):
        pass

ThreadingHTTPServer((host, int(port)), Handler).serve_forever()
PY

cat >"$ARTIFACT_DIR/config/ai-proxy.yaml" <<EOF
server:
  listen_addr: $AI_PROXY_ADDR
  request_timeout_seconds: 30
providers:
  live:
    enabled: true
    protocol: openai
    base_url: http://$UPSTREAM_ADDR/v1
    allow_unauthenticated: true
    endpoint_capabilities: chat_completions, embeddings
    models: live-chat-a, live-chat-b, live-embedding
model_catalog:
  live-chat-a:
    context_window_tokens: 65536
    max_output_tokens: 4096
    operations: chat_completions
  live-chat-b:
    context_window_tokens: 131072
    max_output_tokens: 8192
    operations: chat_completions
  live-embedding:
    context_window_tokens: 8192
    max_output_tokens: 1024
    operations: embeddings
EOF

cat >"$ARTIFACT_DIR/config/application.toml" <<EOF
endpointName = "workorch-live-ai-proxy"

[server]
addr = "$WORKORCH_ADDR"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "30s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "file"
dataDir = "$ARTIFACT_DIR/data"

[llm.aiProxy]
baseURL = "http://$AI_PROXY_ADDR"
timeout = "30s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 32

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "live-chat-a"

[llm.defaultEmbedding]
providerID = "ai-proxy"
modelID = "live-embedding"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
defaultMaxOutputTokens = 4096
maxRequestBytes = 4194304
EOF

UPSTREAM_ADDR="$UPSTREAM_ADDR" REQUEST_LOG="$ARTIFACT_DIR/upstream-requests.jsonl" \
  python3 "$ARTIFACT_DIR/fake_upstream.py" >"$ARTIFACT_DIR/upstream.log" 2>&1 &
UPSTREAM_PID=$!
wait_http "http://$UPSTREAM_ADDR/healthz" "fake upstream"

(cd "$AI_PROXY_ROOT" && go build -o "$BUILD_DIR/ai-proxy" ./cmd/ai-proxy)
"$BUILD_DIR/ai-proxy" -config "$ARTIFACT_DIR/config/ai-proxy.yaml" >"$ARTIFACT_DIR/ai-proxy.log" 2>&1 &
AI_PROXY_PID=$!
wait_http "http://$AI_PROXY_ADDR/v1/models" "ai-proxy"

curl --fail --silent --show-error "http://$AI_PROXY_ADDR/v1/models" >"$ARTIFACT_DIR/ai-proxy-models.json"
assert_json "$ARTIFACT_DIR/ai-proxy-models.json" catalog
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"model":"live-chat-a","messages":[{"role":"user","content":"hello"}]}' \
  "http://$AI_PROXY_ADDR/v1/chat/completions" >"$ARTIFACT_DIR/ai-proxy-chat.json"
assert_json "$ARTIFACT_DIR/ai-proxy-chat.json" chat
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"model":"live-embedding","input":"hello"}' \
  "http://$AI_PROXY_ADDR/v1/embeddings" >"$ARTIFACT_DIR/ai-proxy-embedding.json"
assert_json "$ARTIFACT_DIR/ai-proxy-embedding.json" embedding
if curl --silent --show-error -o "$ARTIFACT_DIR/ai-proxy-negative-operation.json" -w '%{http_code}' \
  -H 'Content-Type: application/json' -d '{"model":"live-embedding","messages":[{"role":"user","content":"hello"}]}' \
  "http://$AI_PROXY_ADDR/v1/chat/completions" | grep -qx '200'; then
  echo "embedding-only model unexpectedly accepted chat completion" >&2
  exit 1
fi

go build -o "$BUILD_DIR/workorch" ./application/workorch
"$BUILD_DIR/workorch" --server --config-dir "$ARTIFACT_DIR/config" >"$ARTIFACT_DIR/workorch.log" 2>&1 &
WORKORCH_PID=$!
wait_http "http://$WORKORCH_ADDR/readyz" "workorch"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/configuration/llm/models?providerId=ai-proxy" >"$ARTIFACT_DIR/workorch-catalog.json"
assert_json "$ARTIFACT_DIR/workorch-catalog.json" catalog
curl --fail --silent --show-error -X POST "http://$WORKORCH_ADDR/api/v1/workorch/configuration/llm/models/refresh?providerId=ai-proxy" >"$ARTIFACT_DIR/workorch-catalog-refresh.json"
assert_json "$ARTIFACT_DIR/workorch-catalog-refresh.json" catalog

curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"title":"ai-proxy runtime candidate probe","description":"Capture the sealed Runtime V2 candidate request through ai-proxy.","templateId":"generic-governed"}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces" >"$ARTIFACT_DIR/runtime-workspace.json"
WORKSPACE_ID="$(json_field "$ARTIFACT_DIR/runtime-workspace.json" workspace.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"text":"Produce a traceable neutral result."}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/goals" >"$ARTIFACT_DIR/runtime-goal.json"
GOAL_ID="$(json_field "$ARTIFACT_DIR/runtime-goal.json" goal.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"items":["A result is traceable"]}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/dods" >"$ARTIFACT_DIR/runtime-dod.json"
DOD_ID="$(json_field "$ARTIFACT_DIR/runtime-dod.json" dod.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"intent":"resume","requestId":"ai-proxy-runtime-probe","reason":"live_verification","source":"verification","goalId":"'"$GOAL_ID"'","doDId":"'"$DOD_ID"'","llmTarget":{"providerId":"ai-proxy","modelId":"live-chat-a"}}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/execution-activations" >"$ARTIFACT_DIR/runtime-activation.json"
for _ in $(seq 1 100); do
  if grep -q '### C3.runtime-authority' "$ARTIFACT_DIR/upstream-requests.jsonl" 2>/dev/null; then
    break
  fi
  sleep 0.2
done
grep -q '### C3.runtime-authority' "$ARTIFACT_DIR/upstream-requests.jsonl"
for _ in $(seq 1 100); do
  curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/runs" >"$ARTIFACT_DIR/runtime-runs.json"
  if [[ "$(json_field "$ARTIFACT_DIR/runtime-runs.json" runs.0.state)" == "completed" ]]; then
    break
  fi
  sleep 0.2
done
RUN_ID="$(json_field "$ARTIFACT_DIR/runtime-runs.json" runs.0.id)"
[[ "$(json_field "$ARTIFACT_DIR/runtime-runs.json" runs.0.state)" == "completed" ]]
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/snapshot" >"$ARTIFACT_DIR/runtime-snapshot-before-restart.json"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/observability" >"$ARTIFACT_DIR/runtime-observability-before-restart.json"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/events?workspaceId=$WORKSPACE_ID&runId=$RUN_ID" >"$ARTIFACT_DIR/runtime-events-before-restart.json"

stop_process "$WORKORCH_PID"
WORKORCH_PID=""
"$BUILD_DIR/workorch" --server --config-dir "$ARTIFACT_DIR/config" >"$ARTIFACT_DIR/workorch-restart.log" 2>&1 &
WORKORCH_PID=$!
wait_http "http://$WORKORCH_ADDR/readyz" "restarted workorch"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/snapshot" >"$ARTIFACT_DIR/runtime-snapshot-after-restart.json"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$WORKSPACE_ID/runs/$RUN_ID/observability" >"$ARTIFACT_DIR/runtime-observability-after-restart.json"
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/events?workspaceId=$WORKSPACE_ID&runId=$RUN_ID" >"$ARTIFACT_DIR/runtime-events-after-restart.json"

stop_process "$AI_PROXY_PID"
AI_PROXY_PID=""
python3 - "$ARTIFACT_DIR/config/ai-proxy.yaml" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
old = "live-chat-b:\n    context_window_tokens: 131072"
new = "live-chat-b:\n    context_window_tokens: 196608"
if source.count(old) != 1:
    raise SystemExit("expected exactly one live-chat-b catalog capacity to replace")
path.write_text(source.replace(old, new), encoding="utf-8")
PY
"$BUILD_DIR/ai-proxy" -config "$ARTIFACT_DIR/config/ai-proxy.yaml" >"$ARTIFACT_DIR/ai-proxy-restart.log" 2>&1 &
AI_PROXY_PID=$!
wait_http "http://$AI_PROXY_ADDR/v1/models" "restarted ai-proxy"
curl --fail --silent --show-error -X POST "http://$WORKORCH_ADDR/api/v1/workorch/configuration/llm/models/refresh?providerId=ai-proxy" >"$ARTIFACT_DIR/workorch-catalog-after-change.json"
python3 - "$ARTIFACT_DIR/workorch-catalog-after-change.json" <<'PY'
import json
import sys

catalog = json.load(open(sys.argv[1], encoding="utf-8"))["catalog"]
models = {item["modelId"]: item for item in catalog.get("models") or []}
model = models.get("live-chat-b")
if not model or int(model.get("contextWindowTokens") or 0) != 196608:
    raise SystemExit(f"refreshed catalog did not expose changed live-chat-b capacity: {catalog}")
PY

curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"title":"ai-proxy sealed target comparison","description":"Verify a second Runtime V2 Run seals a distinct ai-proxy model ceiling.","templateId":"generic-governed"}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces" >"$ARTIFACT_DIR/runtime-second-workspace.json"
SECOND_WORKSPACE_ID="$(json_field "$ARTIFACT_DIR/runtime-second-workspace.json" workspace.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"text":"Produce a second traceable neutral result."}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$SECOND_WORKSPACE_ID/goals" >"$ARTIFACT_DIR/runtime-second-goal.json"
SECOND_GOAL_ID="$(json_field "$ARTIFACT_DIR/runtime-second-goal.json" goal.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"items":["A second result is traceable"]}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$SECOND_WORKSPACE_ID/dods" >"$ARTIFACT_DIR/runtime-second-dod.json"
SECOND_DOD_ID="$(json_field "$ARTIFACT_DIR/runtime-second-dod.json" dod.id)"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"intent":"resume","requestId":"ai-proxy-runtime-second","reason":"live_verification","source":"verification","goalId":"'"$SECOND_GOAL_ID"'","doDId":"'"$SECOND_DOD_ID"'","llmTarget":{"providerId":"ai-proxy","modelId":"live-chat-b"}}' \
  "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$SECOND_WORKSPACE_ID/execution-activations" >"$ARTIFACT_DIR/runtime-second-activation.json"
for _ in $(seq 1 100); do
  curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$SECOND_WORKSPACE_ID/runs" >"$ARTIFACT_DIR/runtime-second-runs.json"
  if [[ "$(json_field "$ARTIFACT_DIR/runtime-second-runs.json" runs.0.state)" == "completed" ]]; then
    break
  fi
  sleep 0.2
done
SECOND_RUN_ID="$(json_field "$ARTIFACT_DIR/runtime-second-runs.json" runs.0.id)"
[[ "$(json_field "$ARTIFACT_DIR/runtime-second-runs.json" runs.0.state)" == "completed" ]]
curl --fail --silent --show-error "http://$WORKORCH_ADDR/api/v1/workorch/workspaces/$SECOND_WORKSPACE_ID/runs/$SECOND_RUN_ID/observability" >"$ARTIFACT_DIR/runtime-second-observability.json"

python3 - \
  "$ARTIFACT_DIR/runtime-snapshot-before-restart.json" \
  "$ARTIFACT_DIR/runtime-observability-before-restart.json" \
  "$ARTIFACT_DIR/runtime-events-before-restart.json" \
  "$ARTIFACT_DIR/runtime-snapshot-after-restart.json" \
  "$ARTIFACT_DIR/runtime-observability-after-restart.json" \
  "$ARTIFACT_DIR/runtime-events-after-restart.json" \
  "$ARTIFACT_DIR/runtime-second-observability.json" <<'PY'
import json
import sys

before_snapshot, before_observability, before_events, after_snapshot, after_observability, after_events, second_observability = [
    json.load(open(path, encoding="utf-8")) for path in sys.argv[1:]
]

def audit_signature(observability):
    traces = observability["observability"].get("llmUsageTraces") or []
    if len(traces) != 2:
        raise SystemExit(f"LLM usage trace count = {len(traces)}, want two Runtime V2 candidate requests")
    result = []
    for trace in traces:
        budget = trace.get("contextBudget") or {}
        required = {
            "providerId": "ai-proxy",
            "model": "live-chat-a",
            "operation": "chat_completions",
            "usageSource": "provider_native",
        }
        for key, want in required.items():
            if trace.get(key) != want:
                raise SystemExit(f"LLM usage {key} = {trace.get(key)!r}, want {want!r}: {trace}")
        if not budget.get("providerSubmitted") or budget.get("capabilitySource") != "sealed_ceiling":
            raise SystemExit(f"usage trace is missing sealed provider audit: {trace}")
        for key in ("capabilityRevision", "configurationRevision", "stablePrefixHash", "renderedStablePrefixHash"):
            if not str(budget.get(key) or "").strip():
                raise SystemExit(f"usage trace is missing {key}: {trace}")
        if int(budget.get("contextWindowTokens") or 0) != 65536 or int(budget.get("maxBytes") or 0) <= 98304:
            raise SystemExit(f"usage trace has an unexpected sealed ceiling: {trace}")
        if int(trace.get("totalTokens") or 0) != 5:
            raise SystemExit(f"usage trace lost provider-native usage: {trace}")
        result.append((
            budget["capabilityRevision"], budget["configurationRevision"], budget["stablePrefixHash"],
            budget["renderedStablePrefixHash"], budget["contextWindowTokens"], budget["maxBytes"], trace["totalTokens"],
        ))
    if len({item[2] for item in result}) != 1:
        raise SystemExit(f"stablePrefixHash drifted within one sealed Run: {result}")
    return result

for snapshot in (before_snapshot, after_snapshot):
    if snapshot["snapshot"]["run"].get("state") != "completed":
        raise SystemExit(f"Runtime V2 Run was not completed: {snapshot}")

before_signature = audit_signature(before_observability)
after_signature = audit_signature(after_observability)
if before_signature != after_signature:
    raise SystemExit(f"restart changed persisted LLM audit/ceiling: before={before_signature}, after={after_signature}")

for events in (before_events, after_events):
    topics = [item.get("topic") for item in events.get("eventRecords") or []]
    if topics.count("/run/llm_usage/recorded") != 2:
        raise SystemExit(f"persisted LLM usage event count = {topics.count('/run/llm_usage/recorded')}, want two: {topics}")

second_traces = second_observability["observability"].get("llmUsageTraces") or []
if len(second_traces) != 2:
    raise SystemExit(f"second Run LLM usage trace count = {len(second_traces)}, want two")
for trace in second_traces:
    budget = trace.get("contextBudget") or {}
    if trace.get("providerId") != "ai-proxy" or trace.get("model") != "live-chat-b":
        raise SystemExit(f"second Run did not use its sealed ai-proxy target: {trace}")
    if int(budget.get("contextWindowTokens") or 0) != 196608 or int(budget.get("maxBytes") or 0) <= before_signature[0][5]:
        raise SystemExit(f"second Run did not preserve a distinct larger sealed ceiling: {trace}")
PY

python3 - "$ARTIFACT_DIR/upstream-requests.jsonl" <<'PY'
import json
import sys

requests = [json.loads(line) for line in open(sys.argv[1], encoding="utf-8") if line.strip()]
expected = {("/v1/chat/completions", "live-chat-a"), ("/v1/embeddings", "live-embedding")}
actual = {(item.get("path"), item.get("model")) for item in requests}
missing = expected - actual
if missing:
    raise SystemExit(f"real ai-proxy did not forward expected upstream requests: {sorted(missing)}; actual={sorted(actual)}")
PY

git -C "$AI_PROXY_ROOT" rev-parse HEAD >"$ARTIFACT_DIR/ai-proxy-commit.txt"
printf 'artifact=%s\nai_proxy_addr=%s\nworkorch_addr=%s\n' "$ARTIFACT_DIR" "$AI_PROXY_ADDR" "$WORKORCH_ADDR" | tee "$ARTIFACT_DIR/summary.txt"
