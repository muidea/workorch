#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_REAL_WEATHER_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
PROVIDER_ID="${WORKORCH_REAL_WEATHER_PROVIDER_ID:-}"
VERIFY_SUFFIX="${WORKORCH_REAL_WEATHER_VERIFY_SUFFIX:-$$}"
MCP_SERVER_ID="${WORKORCH_REAL_WEATHER_MCP_SERVER_ID:-mcp-weather-real-$VERIFY_SUFFIX}"
TMP_DIR="${WORKORCH_REAL_WEATHER_ARTIFACT_DIR:-$(mktemp -d)}"
RUN_WAIT_SECONDS="${WORKORCH_REAL_WEATHER_RUN_WAIT_SECONDS:-600}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_REAL_WEATHER_ACTIVATION_RUN_WAIT_SECONDS:-60}"
MCP_ENDPOINT_ROOT="${WORKORCH_MCP_ENDPOINT_DIR:-.workorch-data/system/mcp-endpoints}"
MCP_ENDPOINT_DIR="$MCP_ENDPOINT_ROOT/$MCP_SERVER_ID"
KEEP_ARTIFACTS="${WORKORCH_REAL_WEATHER_KEEP_ARTIFACTS:-1}"
WEATHER_HELPER="$MCP_ENDPOINT_DIR/weather_mcp_helper.py"

cleanup() {
  local status=$?
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "real weather query e2e verification failed" >&2
  echo "baseUrl=$BASE_URL" >&2
  echo "artifacts=$TMP_DIR" >&2
  for file in "$TMP_DIR"/*.json; do
    [[ -f "$file" ]] || continue
    echo "--- $file ---" >&2
    python3 -m json.tool "$file" >&2 || cat "$file" >&2
  done
}

json_path() {
  python3 - "$1" "$2" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    data = json.load(handle)
current = data
for part in sys.argv[2].split("."):
    current = current[part]
print(current)
PY
}

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body" >"$output"
}

capture_runs_before_activation() {
  local output_file="$1"
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$output_file"
}

validate_activation_command_response() {
  local response_file="$1"
  local expected_request_id="$2"
  local expected_reason="$3"
  python3 - "$response_file" "$expected_request_id" "$expected_reason" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
expected_request_id = sys.argv[2]
expected_reason = sys.argv[3]
errors = []
if data.get("accepted") is not True:
    errors.append(f"accepted is {data.get('accepted')!r}, want true")
if data.get("intent") != "resume":
    errors.append(f"intent is {data.get('intent')!r}, want resume")
if data.get("requestId") != expected_request_id:
    errors.append(f"requestId is {data.get('requestId')!r}, want {expected_request_id!r}")
if data.get("reason") != expected_reason:
    errors.append(f"reason is {data.get('reason')!r}, want {expected_reason!r}")
if errors:
    raise SystemExit("\n".join(errors))
PY
}

wait_activation_run() {
  local before_runs_file="$1"
  local runs_file="$2"
  local output_file="$3"
  local run_id=""
  local deadline=$((SECONDS + ACTIVATION_RUN_WAIT_SECONDS))
  while (( SECONDS <= deadline )); do
    if curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs" >"$runs_file"; then
      run_id="$(python3 - "$runs_file" "$before_runs_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
before_data = json.load(open(sys.argv[2], encoding="utf-8"))
before = {str(item.get("id") or "") for item in before_data.get("runs", []) if item.get("id")}
new_runs = [
    item for item in data.get("runs", [])
    if item.get("id") and str(item.get("id")) not in before
]
if not new_runs:
    raise SystemExit(1)
print(new_runs[-1]["id"])
PY
)" && {
        python3 - "$runs_file" "$run_id" "$output_file" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
run_id = sys.argv[2]
run = next((item for item in data.get("runs", []) if item.get("id") == run_id), {"id": run_id})
with open(sys.argv[3], "w", encoding="utf-8") as handle:
    json.dump({"run": run}, handle, ensure_ascii=False, indent=2)
    handle.write("\n")
PY
        echo "$run_id"
        return 0
      }
    fi
    sleep 1
  done
  echo "activation accepted but run did not materialize within ${ACTIVATION_RUN_WAIT_SECONDS}s for workspace $workspace_id" >&2
  return 1
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + RUN_WAIT_SECONDS))
  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status?waitState=$expected_state&waitTimeoutMillis=1000&waitIntervalMillis=100" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  cat "$output_file" >&2
  return 1
}

upsert_json() {
  local collection_url="$1"
  local resource_url="$2"
  local body="$3"
  local output="$4"
  local tmp="$output.tmp"
  local status
  status="$(curl -sS -H "Content-Type: application/json" -X POST "$collection_url" -d "$body" -o "$tmp" -w "%{http_code}" || true)"
  case "$status" in
    200|201)
      mv "$tmp" "$output"
      ;;
    409)
      rm -f "$tmp"
      curl -fsS -H "Content-Type: application/json" -X PUT "$resource_url" -d "$body" >"$output"
      ;;
    *)
      cat "$tmp" >&2 || true
      rm -f "$tmp"
      return 1
      ;;
  esac
}

create_weather_helper() {
  mkdir -p "$MCP_ENDPOINT_DIR"
  cat >"$WEATHER_HELPER" <<'PY'
#!/usr/bin/env python3
import datetime as dt
import json
import sys
import urllib.parse
import urllib.request

WEATHER_CODE_TEXT = {
    0: "晴朗",
    1: "大部晴朗",
    2: "局部多云",
    3: "阴",
    45: "雾",
    48: "雾凇",
    51: "小毛毛雨",
    53: "中等毛毛雨",
    55: "强毛毛雨",
    61: "小雨",
    63: "中雨",
    65: "大雨",
    71: "小雪",
    73: "中雪",
    75: "大雪",
    80: "小阵雨",
    81: "中等阵雨",
    82: "强阵雨",
    95: "雷暴",
}


def http_json(url):
    request = urllib.request.Request(url, headers={"User-Agent": "workorch-weather-e2e/1.0"})
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.loads(response.read().decode("utf-8"))


def pick_hangzhou(results):
    for item in results:
        if item.get("admin1") == "浙江" and ("杭州" in item.get("name", "") or item.get("admin2") == "杭州"):
            return item
    if results:
        return results[0]
    raise RuntimeError("Open-Meteo geocoding did not return Hangzhou")


def current_weather(arguments):
    city = arguments.get("city") or "杭州"
    province = arguments.get("province") or "浙江"
    country = arguments.get("country") or "中国"
    lookup_name = arguments.get("lookupName") or "Hangzhou"
    geocode_url = "https://geocoding-api.open-meteo.com/v1/search?" + urllib.parse.urlencode({
        "name": lookup_name,
        "count": "5",
        "language": "zh",
        "format": "json",
    })
    geocode = http_json(geocode_url)
    location = pick_hangzhou(geocode.get("results") or [])
    forecast_url = "https://api.open-meteo.com/v1/forecast?" + urllib.parse.urlencode({
        "latitude": location["latitude"],
        "longitude": location["longitude"],
        "current": "temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,cloud_cover,wind_speed_10m,wind_direction_10m",
        "timezone": "Asia/Shanghai",
    })
    forecast = http_json(forecast_url)
    current = forecast.get("current") or {}
    units = forecast.get("current_units") or {}
    weather_code = current.get("weather_code")
    structured = {
        "query": {"country": country, "province": province, "city": city},
        "location": {
            "name": location.get("name"),
            "admin1": location.get("admin1"),
            "admin2": location.get("admin2"),
            "country": location.get("country"),
            "latitude": location.get("latitude"),
            "longitude": location.get("longitude"),
            "timezone": location.get("timezone") or forecast.get("timezone"),
        },
        "current": {
            "observedAt": current.get("time"),
            "temperatureC": current.get("temperature_2m"),
            "apparentTemperatureC": current.get("apparent_temperature"),
            "humidityPercent": current.get("relative_humidity_2m"),
            "precipitationMm": current.get("precipitation"),
            "weatherCode": weather_code,
            "weatherDescription": WEATHER_CODE_TEXT.get(weather_code, f"WMO {weather_code}"),
            "cloudCoverPercent": current.get("cloud_cover"),
            "windSpeedKmh": current.get("wind_speed_10m"),
            "windDirectionDegree": current.get("wind_direction_10m"),
        },
        "units": units,
        "source": {
            "name": "Open-Meteo",
            "geocodingUrl": geocode_url,
            "forecastUrl": forecast_url,
        },
        "fetchedAt": dt.datetime.now(dt.timezone.utc).isoformat(),
    }
    text = (
        f"{province}{city}当前天气：{structured['current']['weatherDescription']}，"
        f"气温 {structured['current']['temperatureC']}°C，"
        f"体感 {structured['current']['apparentTemperatureC']}°C，"
        f"湿度 {structured['current']['humidityPercent']}%，"
        f"降水 {structured['current']['precipitationMm']} mm，"
        f"风速 {structured['current']['windSpeedKmh']} km/h。"
    )
    return {
        "content": [{"type": "text", "text": text}],
        "structuredContent": structured,
        "isError": False,
    }


def response(request_id, result=None, error=None):
    payload = {"jsonrpc": "2.0", "id": request_id}
    if error is not None:
        payload["error"] = error
    else:
        payload["result"] = result
    print(json.dumps(payload, ensure_ascii=False), flush=True)


for line in sys.stdin:
    if not line.strip():
        continue
    try:
        request = json.loads(line)
        method = request.get("method")
        request_id = request.get("id")
        if method == "initialize":
            response(request_id, {
                "protocolVersion": request.get("params", {}).get("protocolVersion", "2025-11-25"),
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "workorch-weather-mcp", "version": "test"},
            })
        elif method == "notifications/initialized":
            continue
        elif method == "tools/list":
            response(request_id, {"tools": [{
                "name": "weather.current",
                "description": "Query current weather for Zhejiang Hangzhou from Open-Meteo.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "country": {"type": "string"},
                        "province": {"type": "string"},
                        "city": {"type": "string"},
                        "lookupName": {"type": "string"},
                    },
                },
            }]})
        elif method == "tools/call":
            params = request.get("params", {})
            if params.get("name") != "weather.current":
                raise RuntimeError(f"unsupported tool {params.get('name')!r}")
            response(request_id, current_weather(params.get("arguments") or {}))
        else:
            response(request_id, error={"code": -32601, "message": "method not found"})
    except Exception as exc:
        response(request.get("id") if "request" in locals() else None, error={
            "code": -32000,
            "message": str(exc),
        })
PY
  chmod +x "$WEATHER_HELPER"
}

select_provider() {
  python3 - "$TMP_DIR/configuration.json" "$PROVIDER_ID" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
requested = sys.argv[2].strip()
llm = data.get("configuration", {}).get("llm", {})
provider_id = requested or llm.get("defaultProviderId", "")
providers = {item.get("id"): item for item in llm.get("providers", [])}
provider = providers.get(provider_id)
if not provider:
    raise SystemExit(f"provider {provider_id!r} is not configured")
if not provider.get("ready"):
    raise SystemExit(f"provider {provider_id!r} is not ready: {provider.get('reason', '')}")
if not provider.get("apiKeyConfigured"):
    raise SystemExit(f"provider {provider_id!r} has no configured API key")
print(provider_id)
PY
}

validate_weather_snapshot() {
  python3 - "$TMP_DIR/snapshot.json" "$TMP_DIR/observability.json" <<'PY'
import datetime as dt
import json
import sys

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[2], encoding="utf-8"))["observability"]
run = snapshot["run"]
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
steps = snapshot.get("steps") or ([] if not snapshot.get("step") else [snapshot["step"]])
tool_calls = snapshot.get("toolCalls") or []
validations = snapshot.get("validationResults") or []
checkpoints = snapshot.get("runtimeCheckpoints") or []
results = snapshot.get("results") or []
artifacts = snapshot.get("artifacts") or []
errors = []

if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != "soul-weather-query-execution":
    errors.append(f"execution soul binding is {execution_binding!r}, want soul-weather-query-execution")

mcp_steps = [item for item in steps if item.get("kind") == "mcp/tool_call"]
if not mcp_steps:
    errors.append("missing mcp/tool_call weather step")
    structured = {}
else:
    output = mcp_steps[0].get("output") or {}
    result = output.get("result") or {}
    structured = result.get("structuredContent") or {}
    current = structured.get("current") or {}
    location = structured.get("location") or {}
    if output.get("transport") != "stdio":
        errors.append(f"mcp transport is {output.get('transport')!r}, want stdio")
    if output.get("tool") != "weather.current":
        errors.append(f"tool is {output.get('tool')!r}, want weather.current")
    if location.get("admin1") != "浙江" or "杭州" not in str(location.get("name", "")):
        errors.append(f"unexpected weather location: {location}")
    if not isinstance(current.get("temperatureC"), (int, float)):
        errors.append(f"temperatureC is not numeric: {current}")
    if not isinstance(current.get("humidityPercent"), (int, float)):
        errors.append(f"humidityPercent is not numeric: {current}")
    if not current.get("observedAt"):
        errors.append(f"observedAt is empty: {current}")
    if (structured.get("source") or {}).get("name") != "Open-Meteo":
        errors.append(f"unexpected weather source: {structured.get('source')}")

assessment_steps = [item for item in steps if (item.get("output") or {}).get("stage") == "llm_dod_assessment"]
if not assessment_steps:
    errors.append("missing llm_dod_assessment follow-up step")
else:
    assessment = (assessment_steps[-1].get("output") or {}).get("dodAssessment") or {}
    if assessment.get("satisfied") is not True:
        errors.append(f"dodAssessment.satisfied is {assessment.get('satisfied')!r}, want true")
    if not any(item.get("evidenceRefs") for item in assessment.get("items") or []):
        errors.append(f"dodAssessment has no evidenceRefs: {assessment}")
    summary = str(assessment.get("summary") or "")
    if "杭州" not in summary or "气温" not in summary or "湿度" not in summary:
        errors.append(f"dodAssessment.summary does not expose final weather result: {summary!r}")

progress = observability.get("progress") or {}
if progress.get("terminal") is not True:
    errors.append(f"observability terminal is {progress.get('terminal')!r}, want true")
if not tool_calls:
    errors.append("missing ToolCall evidence")
if not validations:
    errors.append("missing ValidationResult evidence")
if not checkpoints:
    errors.append("missing RuntimeCheckpoint evidence")
accepted_results = [item for item in results if item.get("state") == "accepted"]
if not accepted_results:
    errors.append("missing accepted Result final outcome")
else:
    result = accepted_results[-1]
    if "杭州" not in str(result.get("summary") or ""):
        errors.append(f"Result summary does not expose final weather result: {result}")
    artifact_refs = set(result.get("artifactRefs") or [])
    linked_artifacts = [item for item in artifacts if item.get("resultId") == result.get("id")]
    if not linked_artifacts:
        errors.append(f"missing Artifact linked to accepted Result {result.get('id')}")
    elif not any(item.get("id") in artifact_refs and item.get("artifactType") == "text" for item in linked_artifacts):
        errors.append(f"missing text Artifact referenced by Result: result={result} artifacts={linked_artifacts}")

if errors:
    for item in errors:
        print(item, file=sys.stderr)
    raise SystemExit(1)

current = structured["current"]
location = structured["location"]
print(f"runId={run['id']}")
print(f"runState={run['state']}")
print(f"location={location.get('admin1')}{location.get('name')}")
print(f"observedAt={current.get('observedAt')}")
print(f"temperatureC={current.get('temperatureC')}")
print(f"weather={current.get('weatherDescription')}")
print(f"humidityPercent={current.get('humidityPercent')}")
print(f"windSpeedKmh={current.get('windSpeedKmh')}")
print(f"steps={len(steps)}")
print(f"toolCalls={len(tool_calls)}")
print(f"validations={len(validations)}")
print(f"results={len(results)}")
print(f"artifacts={len(artifacts)}")
print(f"runtimeCheckpoints={len(checkpoints)}")
PY
}

trap report_failure ERR
trap cleanup EXIT

mkdir -p "$TMP_DIR"
cd "$ROOT_DIR"
create_weather_helper

curl -fsS "$BASE_URL/readyz" >"$TMP_DIR/readyz.json"
python3 - "$TMP_DIR/readyz.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
if data.get("status") != "ready":
    raise SystemExit(f"readyz status is {data.get('status')!r}, want ready")
PY

curl -fsS "$BASE_URL/api/v1/workorch/configuration" >"$TMP_DIR/configuration.json"
PROVIDER_ID="$(select_provider)"

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"real weather query MCP","transport":"stdio","endpoint":"'"$WEATHER_HELPER"'","enabled":true,"allowedTools":["weather.current"],"allowedResources":[],"allowedPrompts":[],"defaultApprovalPolicy":"operator","sessionPolicy":"long_lived","timeoutMillis":30000}' \
  "$TMP_DIR/mcp-server.json"

post_json "$BASE_URL/api/v1/workorch/workspaces" \
  '{"title":"真实天气查询验收","description":"通过真实 LLM 和 MCP stdio 天气工具查询浙江杭州当前天气。","templateId":"weather-query"}' \
  "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" \
  '{"text":"查询中国浙江杭州当前天气。必须使用已选择的 MCP Server '"$MCP_SERVER_ID"' 调用 weather.current 工具，工具参数使用 {\"country\":\"中国\",\"province\":\"浙江\",\"city\":\"杭州\",\"lookupName\":\"Hangzhou\"}。第一轮 LLM 必须返回 executionGraph 或 mcpToolCalls，且包含一个 mcp/tool_call 节点调用 weather.current；不要使用 shell/command，不要请求人工决策或审批。工具返回后，后续 LLM 步骤必须基于 previousStepOutput 总结当前天气，并返回 completionState completed 与 dodAssessment，evidenceRefs 使用 previousStepId，dodAssessment.summary 必须包含当前天气结果文本，包括天气现象、气温、湿度和风速。"}' \
  "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" \
  '{"items":["通过 MCP weather.current 获取浙江杭州当前天气","天气结果包含温度、湿度、天气现象、风速和观测时间","Run snapshot 暴露 ToolCall、ValidationResult、RuntimeCheckpoint 和 DoD 评估证据"]}' \
  "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

activation_request_id="real-weather-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/activation-before-runs.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmProviderId":"'"$PROVIDER_ID"'","mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":3}' \
  "$TMP_DIR/activation.json"
validate_activation_command_response "$TMP_DIR/activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/activation-before-runs.json" "$TMP_DIR/activation-runs.json" "$TMP_DIR/activation-run.json")"
[[ -n "$run_id" ]]

wait_run_state "$run_id" "completed" "$TMP_DIR/run-status.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/observability.json"

echo "workspaceId=$workspace_id"
echo "provider=$PROVIDER_ID"
validate_weather_snapshot
echo "real weather query e2e verification passed for provider=$PROVIDER_ID"
echo "artifacts=$TMP_DIR"
