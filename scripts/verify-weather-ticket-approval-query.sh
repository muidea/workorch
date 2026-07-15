#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${WORKORCH_WEATHER_GOVERNANCE_BASE_URL:-${WORKORCH_BASE_URL:-http://127.0.0.1:8080}}"
MOCK_ADDR="${WORKORCH_WEATHER_GOVERNANCE_MOCK_ADDR:-127.0.0.1:18190}"
MOCK_BASE_URL="http://$MOCK_ADDR/v1"
VERIFY_SUFFIX="${WORKORCH_WEATHER_GOVERNANCE_SUFFIX:-$$}"
ACTIVATION_RUN_WAIT_SECONDS="${WORKORCH_WEATHER_GOVERNANCE_ACTIVATION_RUN_WAIT_SECONDS:-60}"
LLM_MODE="${WORKORCH_WEATHER_GOVERNANCE_LLM_MODE:-}"
if [[ -z "$LLM_MODE" ]]; then
  if [[ "${CI:-}" == "true" ]]; then
    LLM_MODE="mock"
  else
    LLM_MODE="real"
  fi
fi
if [[ "$LLM_MODE" != "real" && "$LLM_MODE" != "mock" ]]; then
  echo "WORKORCH_WEATHER_GOVERNANCE_LLM_MODE must be real or mock, got $LLM_MODE" >&2
  exit 2
fi
PROVIDER_ID="${WORKORCH_WEATHER_GOVERNANCE_PROVIDER_ID:-}"
if [[ "$LLM_MODE" == "mock" && -z "$PROVIDER_ID" ]]; then
  PROVIDER_ID="verify_weather_governance_$VERIFY_SUFFIX"
fi
MCP_SERVER_ID="${WORKORCH_WEATHER_GOVERNANCE_MCP_SERVER_ID:-mcp-weather-governance-$VERIFY_SUFFIX}"
COUNTRY="${WORKORCH_WEATHER_COUNTRY:-中国}"
PROVINCE="${WORKORCH_WEATHER_PROVINCE:-浙江}"
CITY="${WORKORCH_WEATHER_CITY:-杭州}"
LOOKUP_NAME="${WORKORCH_WEATHER_LOOKUP_NAME:-Hangzhou}"
TMP_DIR="${WORKORCH_WEATHER_GOVERNANCE_ARTIFACT_DIR:-$(mktemp -d)}"
MCP_ENDPOINT_ROOT="${WORKORCH_MCP_ENDPOINT_DIR:-.workorch-data/system/mcp-endpoints}"
MCP_ENDPOINT_DIR="$MCP_ENDPOINT_ROOT/$MCP_SERVER_ID"
KEEP_ARTIFACTS="${WORKORCH_WEATHER_GOVERNANCE_KEEP_ARTIFACTS:-1}"
MOCK_PID=""
MOCK_LOG="$TMP_DIR/weather-governance-llm-mock.log"
WEATHER_HELPER="$MCP_ENDPOINT_DIR/weather_mcp_helper.py"
LOCK_DIR="/tmp/workorch-e2e-llm-config.lock"
LOCK_HELD=0

acquire_llm_config_lock() {
  local deadline=$((SECONDS + 60))
  while (( SECONDS <= deadline )); do
    if mkdir "$LOCK_DIR" >/dev/null 2>&1; then
      LOCK_HELD=1
      return 0
    fi
    sleep 0.2
  done
  echo "timed out acquiring LLM config lock: $LOCK_DIR" >&2
  return 1
}

cleanup() {
  local status=$?
  if [[ -f "$TMP_DIR/original-llm-update.json" ]]; then
    curl -fsS -H "Content-Type: application/json" -X PUT \
      "$BASE_URL/api/v1/workorch/configuration/llm" \
      -d @"$TMP_DIR/original-llm-update.json" >/dev/null 2>&1 || true
  fi
  if [[ -n "$MOCK_PID" ]]; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
    wait "$MOCK_PID" >/dev/null 2>&1 || true
  fi
  if [[ "$LOCK_HELD" == "1" ]]; then
    rmdir "$LOCK_DIR" >/dev/null 2>&1 || true
  fi
  if [[ "$KEEP_ARTIFACTS" != "1" && $status -eq 0 ]]; then
    rm -rf "$TMP_DIR"
  fi
}

report_failure() {
  local status=$?
  if [[ $status -eq 0 ]]; then
    return
  fi
  echo "weather ticket and approval e2e verification failed" >&2
  echo "baseUrl=$BASE_URL" >&2
  echo "llmMode=$LLM_MODE" >&2
  echo "provider=$PROVIDER_ID" >&2
  echo "city=$COUNTRY/$PROVINCE/$CITY lookup=$LOOKUP_NAME" >&2
  echo "artifacts=$TMP_DIR" >&2
  if [[ -f "$MOCK_LOG" ]]; then
    echo "--- weather-governance-llm-mock.log ---" >&2
    tail -n 200 "$MOCK_LOG" >&2 || true
  fi
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

url_encode() {
  python3 - "$1" <<'PY'
import sys
from urllib.parse import quote

print(quote(sys.argv[1], safe=""))
PY
}

post_json() {
  local url="$1"
  local body="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d "$body" >"$output"
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

post_json_file() {
  local url="$1"
  local body_file="$2"
  local output="$3"
  curl -fsS -H "Content-Type: application/json" -X POST "$url" -d @"$body_file" >"$output"
}

write_json_payload() {
  local output="$1"
  local kind="$2"
  python3 - "$output" "$kind" "$COUNTRY" "$PROVINCE" "$CITY" "$LOOKUP_NAME" "$MCP_SERVER_ID" <<'PY'
import json
import sys

output, kind, country, province, city, lookup_name, mcp_server_id = sys.argv[1:]
if kind == "workspace":
    payload = {
        "title": f"{province}{city}天气治理验收",
        "description": "通过 Decision Ticket 和 Approval 治理门后查询指定城市当前天气。",
        "templateId": "weather-query",
    }
elif kind == "goal":
    payload = {
        "text": (
            "本次是指定城市天气查询治理验收，必须严格按四个阶段执行。"
            "第一轮不要调用任何工具，必须返回 completionState=blocked_on_decision 和 requiredDecision，"
            "decisionKind=provide_missing_info，question 询问要查询哪个城市，context.requiredFields 必须包含 "
            "country, province, city, lookupName，context.suggestedCity 必须给出 "
            f"country={country}, province={province}, city={city}, lookupName={lookup_name}。"
            "收到 Decision Ticket resolution 后，第二轮不要调用工具，必须返回 "
            "completionState=blocked_on_approval 和 requiredApproval，审批原因必须说明将通过外部 MCP 天气工具查询"
            f"{country}{province}{city}当前天气。审批通过后，第三轮必须使用 MCP Server {mcp_server_id} "
            "调用 weather.current，工具参数必须包含 "
            f"country={country}, province={province}, city={city}, lookupName={lookup_name}。"
            "不要使用 shell/command。天气工具返回后，最后一轮必须基于 previousStepOutput 返回 "
            "completionState=completed 和 dodAssessment，evidenceRefs 使用 previousStepId，"
            "dodAssessment.summary 必须包含当前天气结果文本，包括天气现象、气温、湿度和风速。"
        )
    }
elif kind == "dod":
    payload = {
        "items": [
            "缺少目标城市时生成 Decision Ticket",
            "补充城市信息后生成 Approval 并等待批准",
            f"批准后通过 MCP weather.current 查询{province}{city}当前天气",
            "Run snapshot 暴露 Decision、Approval、ToolCall、ValidationResult、RuntimeCheckpoint 和 DoD 评估证据",
        ]
    }
else:
    raise SystemExit(f"unsupported payload kind {kind!r}")
with open(output, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, separators=(",", ":"))
PY
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
    request = urllib.request.Request(url, headers={"User-Agent": "workorch-weather-governance-e2e/1.0"})
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.loads(response.read().decode("utf-8"))


def pick_location(results, province, city):
    for item in results:
        admin1 = str(item.get("admin1") or "")
        name = str(item.get("name") or "")
        admin2 = str(item.get("admin2") or "")
        if province and province not in admin1:
            continue
        if city and city not in name and city not in admin2:
            continue
        return item
    if results:
        return results[0]
    raise RuntimeError("Open-Meteo geocoding did not return a matching location")


def current_weather(arguments):
    city = arguments.get("city") or "杭州"
    province = arguments.get("province") or "浙江"
    country = arguments.get("country") or "中国"
    lookup_name = arguments.get("lookupName") or city
    geocode_url = "https://geocoding-api.open-meteo.com/v1/search?" + urllib.parse.urlencode({
        "name": lookup_name,
        "count": "10",
        "language": "zh",
        "format": "json",
    })
    geocode = http_json(geocode_url)
    location = pick_location(geocode.get("results") or [], province, city)
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
        "query": {"country": country, "province": province, "city": city, "lookupName": lookup_name},
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
    request = {}
    try:
        request = json.loads(line)
        method = request.get("method")
        request_id = request.get("id")
        if method == "initialize":
            response(request_id, {
                "protocolVersion": request.get("params", {}).get("protocolVersion", "2025-11-25"),
                "capabilities": {"tools": {}},
                "serverInfo": {"name": "workorch-weather-governance-mcp", "version": "test"},
            })
        elif method == "notifications/initialized":
            continue
        elif method == "tools/list":
            response(request_id, {"tools": [{
                "name": "weather.current",
                "description": "Query current weather for a specified city from Open-Meteo.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "country": {"type": "string"},
                        "province": {"type": "string"},
                        "city": {"type": "string"},
                        "lookupName": {"type": "string"},
                    },
                    "required": ["city"],
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
        response(request.get("id"), error={"code": -32000, "message": str(exc)})
PY
  chmod +x "$WEATHER_HELPER"
}

start_mock_provider() {
  cat >"$TMP_DIR/weather_governance_llm_mock.py" <<'PY'
import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SERVER_ID = os.environ["WORKORCH_WEATHER_GOVERNANCE_MCP_SERVER_ID"]
COUNTRY = os.environ["WORKORCH_WEATHER_COUNTRY"]
PROVINCE = os.environ["WORKORCH_WEATHER_PROVINCE"]
CITY = os.environ["WORKORCH_WEATHER_CITY"]
LOOKUP_NAME = os.environ["WORKORCH_WEATHER_LOOKUP_NAME"]


def parse_provider_request(payload):
    messages = payload.get("messages") or []
    content = "\n".join(str(item.get("content", "")) for item in messages)
    try:
        request = json.loads(content)
    except Exception:
        request = {}
    request["raw"] = content
    if not request.get("runId"):
        run_matches = re.findall(r'"runId"\s*:\s*"([^"]+)"', content)
        if run_matches:
            request["runId"] = run_matches[-1]
    if not request.get("stepId"):
        step_matches = re.findall(r'"stepId"\s*:\s*"([^"]+)"', content)
        if step_matches:
            request["stepId"] = step_matches[-1]
    return request


def goal_text_from_request(request):
    input_payload = request.get("input") or {}
    for container_key in ("executionContextPackage", "executionContext"):
        container = input_payload.get(container_key) or {}
        goal = container.get("goal") or {}
        text = goal.get("text") or goal.get("Text")
        if text:
            return text
    goal = input_payload.get("goal") or {}
    return goal.get("text") or goal.get("Text") or "本次是指定城市天气查询治理验收"


def previous_evidence_ref(request):
    step_input = request.get("input") or {}
    raw = request.get("raw", "")
    previous_match = re.search(r'"previousStepId"\s*:\s*"([^"]+)"', raw)
    return step_input.get("previousStepId") or (previous_match.group(1) if previous_match else "") or request.get("stepId") or request.get("runId") or "weather-current"


def decision_payload(request):
    run_id = request.get("runId", "")
    step_id = request.get("stepId", "")
    step_input = request.get("input") or {}
    raw = request.get("raw", "")
    if run_id == "probe":
        return {"completionState": "failed", "thoughtSummary": "probe"}

    assessment_follow_up = (
        step_input.get("graphFollowUp")
        or step_input.get("previousStepOutput")
        or step_input.get("assessmentOnly")
        or step_input.get("assessmentOnlyContractCorrection")
        or "follow-up-graph" in step_id
        or "assessment-correction" in step_id
        or '"graphFollowUp"' in raw
        or '"previousStepOutput"' in raw
        or '"assessmentOnly"' in raw
        or '"assessmentOnlyContractCorrection"' in raw
    )
    if assessment_follow_up:
        previous_step_id = previous_evidence_ref(request)
        return {
            "thoughtSummary": f"{PROVINCE}{CITY} weather evidence was collected through the approved MCP tool call.",
            "completionState": "completed",
            "dodAssessment": {
                "satisfied": True,
                "summary": f"{PROVINCE}{CITY}当前天气：阴，气温 0°C，湿度 0%，风速 0 km/h。Decision Ticket and Approval were completed before querying current weather.",
                "items": [
                    {
                        "item": "缺少目标城市时生成 Decision Ticket",
                        "state": "satisfied",
                        "evidenceRefs": [previous_step_id],
                    },
                    {
                        "item": "补充城市信息后生成 Approval 并等待批准",
                        "state": "satisfied",
                        "evidenceRefs": [previous_step_id],
                    },
                    {
                        "item": f"批准后通过 MCP weather.current 查询{PROVINCE}{CITY}当前天气",
                        "state": "satisfied",
                        "evidenceRefs": [previous_step_id],
                    },
                    {
                        "item": "Run snapshot 暴露 Decision、Approval、ToolCall、ValidationResult、RuntimeCheckpoint 和 DoD 评估证据",
                        "state": "satisfied",
                        "evidenceRefs": [previous_step_id],
                    },
                ],
                "goalCoverage": {
                    "satisfied": True,
                    "items": [{
                        "requirement": goal_text_from_request(request),
                        "state": "satisfied",
                        "evidenceRefs": [previous_step_id],
                    }],
                },
            },
        }

    if "llm-after-approval" in step_id or "llm-after-approval" in raw:
        return {
            "thoughtSummary": f"Approval granted; plan the MCP weather.current call for {PROVINCE}{CITY}.",
            "completionState": "continue",
            "executionGraph": {
                "nodes": [{
                    "id": "weather-current",
                    "kind": "mcp/tool_call",
                    "input": {
                        "serverId": SERVER_ID,
                        "tool": "weather.current",
                        "input": {
                            "country": COUNTRY,
                            "province": PROVINCE,
                            "city": CITY,
                            "lookupName": LOOKUP_NAME,
                        },
                    },
                }]
            },
        }

    if "llm-after-decision" in step_id or "llm-after-decision" in raw:
        return {
            "thoughtSummary": f"Operator provided target city {COUNTRY}{PROVINCE}{CITY}; approval is required before external weather query.",
            "completionState": "blocked_on_approval",
            "requiredApproval": {
                "reason": f"Approve querying current weather for {COUNTRY}{PROVINCE}{CITY} through external Open-Meteo-backed MCP tool.",
                "action": "query specified city current weather",
                "context": {
                    "country": COUNTRY,
                    "province": PROVINCE,
                    "city": CITY,
                    "lookupName": LOOKUP_NAME,
                    "tool": "weather.current",
                },
            },
        }

    return {
        "thoughtSummary": "Target city is intentionally missing; request an operator Decision Ticket.",
        "completionState": "blocked_on_decision",
        "requiredDecision": {
            "reason": "The weather query must be bound to a specific city before execution.",
            "decisionKind": "provide_missing_info",
            "question": "Which city should workorch query for current weather?",
            "context": {
                "requiredFields": ["country", "province", "city", "lookupName"],
                "suggestedCity": {"country": COUNTRY, "province": PROVINCE, "city": CITY, "lookupName": LOOKUP_NAME},
            },
        },
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path in ("/v1/models", "/models"):
            # WorkOrch catalog expects explicit capacity + operations; without these models stay not-ready.
            models = [
                {
                    "id": "verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "gpt-4o",
                    "object": "model",
                    "owned_by": "openai",
                    "operations": ["chat_completions"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "text-embedding-3-small",
                    "object": "model",
                    "owned_by": "openai",
                    "operations": ["embeddings"],
                    "contextWindowTokens": 8192,
                    "maxOutputTokens": 1024,
                },

                {
                    "id": "verify-stream-native-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "verify-stream-estimated-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "web-search-e2e-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
                {
                    "id": "container-verify-model",
                    "object": "model",
                    "owned_by": "verify",
                    "operations": ["chat_completions", "embeddings"],
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                },
            ]
            response = {"object": "list", "data": models}
            encoded = json.dumps(response).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.12, 0.23, 0.34]}]}
            encoded = json.dumps(response, ensure_ascii=False).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        size = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(size) or b"{}")
        decision = decision_payload(parse_provider_request(payload))
        response = {"choices": [{"message": {"content": json.dumps(decision, ensure_ascii=False, separators=(",", ":"))}}]}
        encoded = json.dumps(response, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)


host, port_text = os.environ.get("WORKORCH_WEATHER_GOVERNANCE_MOCK_ADDR", "127.0.0.1:18190").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
PY
  WORKORCH_WEATHER_GOVERNANCE_MOCK_ADDR="$MOCK_ADDR" \
  WORKORCH_WEATHER_GOVERNANCE_MCP_SERVER_ID="$MCP_SERVER_ID" \
  WORKORCH_WEATHER_COUNTRY="$COUNTRY" \
  WORKORCH_WEATHER_PROVINCE="$PROVINCE" \
  WORKORCH_WEATHER_CITY="$CITY" \
  WORKORCH_WEATHER_LOOKUP_NAME="$LOOKUP_NAME" \
    python3 "$TMP_DIR/weather_governance_llm_mock.py" >>"$MOCK_LOG" 2>&1 &
  MOCK_PID=$!
}

wait_mock() {
  for _ in $(seq 1 50); do
    if curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
      -H "Content-Type: application/json" \
      -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  curl -fsS -X POST "$MOCK_BASE_URL/chat/completions" \
    -H "Content-Type: application/json" \
    -d '{"messages":[{"role":"user","content":"{\"runId\":\"probe\",\"stepId\":\"probe\"}"}]}' >/dev/null
}

write_llm_update_payloads() {
  python3 - "$TMP_DIR/configuration.json" "$TMP_DIR/original-llm-update.json" "$TMP_DIR/verify-llm-update.json" "$MOCK_BASE_URL" "$PROVIDER_ID" <<'PY'
import json
import sys

config_path, original_path, update_path, mock_base_url, provider_id = (sys.argv[1:] + [""])[:5]
provider_id = (provider_id or "verify").strip() or "verify"
data = json.load(open(config_path, encoding="utf-8"))
llm = data.get("configuration", {}).get("llm", {})
ai_proxy = llm.get("aiProxy") or {}
default_chat = llm.get("defaultChat") or {}
default_embedding = llm.get("defaultEmbedding") or {}
budget = llm.get("budget") or {}
custom_providers = llm.get("customProviders") or []

def custom_providers_payload(items, include_api_key=False, mock_base_url=""):
    payload = {}
    for item in items:
        provider_id = (item.get("id") or "").strip()
        if not provider_id:
            continue
        models = {}
        for model in item.get("models") or []:
            model_id = (model.get("id") or "").strip()
            if not model_id:
                continue
            models[model_id] = {
                "contextWindowTokens": int(model.get("contextWindowTokens") or 128000),
                "maxOutputTokens": int(model.get("maxOutputTokens") or 8192),
                "operations": list(model.get("operations") or ["chat_completions"]),
            }
        if not models:
            models["verify-model"] = {
                "contextWindowTokens": 128000,
                "maxOutputTokens": 8192,
                "operations": ["chat_completions", "embeddings"],
            }
        entry = {
            "provider": item.get("provider") or "compatible",
            "apiSurface": item.get("apiSurface") or "openai_compatible_http",
            "baseURL": item.get("baseUrl") or item.get("baseURL") or mock_base_url,
            "timeout": item.get("timeout") or "60s",
            "models": models,
        }
        if include_api_key:
            entry["apiKey"] = "verify-secret"
        payload[provider_id] = entry
    return payload

original = {
    "aiProxy": {
        "baseURL": ai_proxy.get("baseUrl") or ai_proxy.get("baseURL") or mock_base_url,
        "timeout": ai_proxy.get("timeout") or "180s",
        "catalogRequestTimeout": ai_proxy.get("catalogRequestTimeout") or "5s",
        "catalogTTL": ai_proxy.get("catalogTTL") or "5m",
        "catalogMaxStaleAge": ai_proxy.get("catalogMaxStaleAge") or "24h",
        "maxCatalogResponseBytes": int(ai_proxy.get("maxCatalogResponseBytes") or 1048576),
        "maxCatalogModels": int(ai_proxy.get("maxCatalogModels") or 1000),
    },
    "defaultChat": {
        "providerID": default_chat.get("providerId") or default_chat.get("providerID") or provider_id,
        "modelID": default_chat.get("modelId") or default_chat.get("modelID") or "verify-model",
    },
    "budget": {
        "contextUtilizationRatio": float(budget.get("contextUtilizationRatio") or 0.75),
        "estimatedTokensPerByte": float(budget.get("estimatedTokensPerByte") or 0.25),
        "defaultMaxOutputTokens": int(budget.get("defaultMaxOutputTokens") or 4096),
        "maxRequestBytes": int(budget.get("maxRequestBytes") or 4194304),
    },
    "customProviders": custom_providers_payload(custom_providers, include_api_key=False, mock_base_url=mock_base_url),
}
if default_embedding.get("modelId") or default_embedding.get("modelID"):
    original["defaultEmbedding"] = {
        "providerID": default_embedding.get("providerId") or default_embedding.get("providerID") or provider_id,
        "modelID": default_embedding.get("modelId") or default_embedding.get("modelID"),
    }
if not original["customProviders"]:
    original["customProviders"] = {
        provider_id: {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "timeout": "60s",
            "models": {
                "verify-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                }
            },
        }
    }
    original["defaultChat"] = {"providerID": provider_id, "modelID": "verify-model"}
with open(original_path, "w", encoding="utf-8") as handle:
    json.dump(original, handle, separators=(",", ":"))

update = {
    "aiProxy": {
        "baseURL": mock_base_url,
        "apiKey": "verify-secret",
        "timeout": "180s",
        "catalogRequestTimeout": "5s",
        "catalogTTL": "5m",
        "catalogMaxStaleAge": "24h",
        "maxCatalogResponseBytes": 1048576,
        "maxCatalogModels": 1000,
        "modelOverrides": {
            "verify-model": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]},
            "gpt-4o": {"contextWindowTokens": 128000, "maxOutputTokens": 8192, "operations": ["chat_completions", "embeddings"]},
        },
    },
    "defaultChat": {"providerID": provider_id, "modelID": "verify-model"},
    "defaultEmbedding": {"providerID": provider_id, "modelID": "verify-model"},
    "budget": original["budget"],
    "customProviders": {
        provider_id: {
            "provider": "compatible",
            "apiSurface": "openai_compatible_http",
            "baseURL": mock_base_url,
            "apiKey": "verify-secret",
            "timeout": "60s",
            "models": {
                "verify-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
                "verify-stream-native-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
                "verify-stream-estimated-model": {
                    "contextWindowTokens": 128000,
                    "maxOutputTokens": 8192,
                    "operations": ["chat_completions", "embeddings"],
                },
            },
        }
    },
}
with open(update_path, "w", encoding="utf-8") as handle:
    json.dump(update, handle, separators=(",", ":"))
PY
}

select_real_provider() {
  python3 - "$TMP_DIR/configuration.json" "$PROVIDER_ID" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
requested = sys.argv[2].strip()
llm = data.get("configuration", {}).get("llm", {})
default_chat = llm.get("defaultChat") or {}
provider_id = requested or default_chat.get("providerId") or default_chat.get("providerID") or ""
if not provider_id:
    raise SystemExit("no defaultChat provider configured")
if provider_id == "ai-proxy":
    ai_proxy = llm.get("aiProxy") or {}
    if not (ai_proxy.get("baseUrl") or ai_proxy.get("baseURL")):
        raise SystemExit("ai-proxy baseURL is not configured")
    print(provider_id)
else:
    customs = {item.get("id"): item for item in (llm.get("customProviders") or [])}
    provider = customs.get(provider_id)
    if not provider:
        raise SystemExit(f"custom provider {provider_id!r} is not configured")
    if not provider.get("apiKeyConfigured"):
        raise SystemExit(f"custom provider {provider_id!r} has no configured API key")
    print(provider_id)
PY
}

wait_run_state() {
  local run_id="$1"
  local expected_state="$2"
  local output_file="$3"
  local deadline=$((SECONDS + 180))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if grep '"state": "'"$expected_state"'"' "$output_file" >/dev/null; then
      return 0
    fi
    sleep 1
  done

  cat "$output_file" >&2
  return 1
}

wait_run_state_any() {
  local run_id="$1"
  local output_file="$2"
  shift 2
  local deadline=$((SECONDS + 180))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$output_file"
    if python3 - "$output_file" "$@" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
state = (data.get("status") or {}).get("state")
raise SystemExit(0 if state in set(sys.argv[2:]) else 1)
PY
    then
      return 0
    fi
    sleep 1
  done

  cat "$output_file" >&2
  return 1
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

wait_workspace_list_projection() {
  local workspace_id="$1"
  local run_id="$2"
  local expected_state="$3"
  local output_file="$4"
  local deadline=$((SECONDS + 180))

  while (( SECONDS <= deadline )); do
    curl -fsS "$BASE_URL/api/v1/workorch/workspaces" >"$output_file"
    if python3 - "$output_file" "$workspace_id" "$run_id" "$expected_state" <<'PY'
import json
import sys

path, workspace_id, run_id, expected_state = sys.argv[1:]
data = json.load(open(path, encoding="utf-8"))
for item in data.get("workspaces") or []:
    if item.get("id") != workspace_id:
        continue
    if item.get("state") == expected_state and item.get("activeRunId") == run_id:
        raise SystemExit(0)
    raise SystemExit(1)
raise SystemExit(1)
PY
    then
      return 0
    fi
    sleep 1
  done

  python3 - "$output_file" "$workspace_id" "$run_id" "$expected_state" <<'PY' >&2 || true
import json
import sys

path, workspace_id, run_id, expected_state = sys.argv[1:]
data = json.load(open(path, encoding="utf-8"))
for item in data.get("workspaces") or []:
    if item.get("id") == workspace_id:
        raise SystemExit(f"workspace list projection = {item}, want {expected_state} activeRunId={run_id}")
raise SystemExit(f"workspace {workspace_id} missing from workspace list")
PY
  return 1
}

validate_snapshot() {
  python3 - "$TMP_DIR/snapshot.json" "$TMP_DIR/observability.json" "$ticket_id" "$approval_id" "$COUNTRY" "$PROVINCE" "$CITY" "$BASE_URL" "$workspace_id" <<'PY'
import json
import sys
import urllib.parse
import urllib.request

snapshot = json.load(open(sys.argv[1], encoding="utf-8"))["snapshot"]
observability = json.load(open(sys.argv[2], encoding="utf-8"))["observability"]
ticket_id, approval_id, country, province, city, base_url, workspace_id = sys.argv[3:]
run = snapshot["run"]
routing = (run.get("executionContext") or {}).get("soulRouting") or {}
bindings = routing.get("bindings") or {}
execution_binding = bindings.get("execution") or {}
steps = snapshot.get("steps") or ([] if not snapshot.get("step") else [snapshot["step"]])
tool_calls = snapshot.get("toolCalls") or []
validations = snapshot.get("validationResults") or []
checkpoints = snapshot.get("runtimeCheckpoints") or []
decisions = snapshot.get("decisions") or []
approvals = snapshot.get("approvals") or []
results = snapshot.get("results") or []
artifacts = snapshot.get("artifacts") or []
errors = []

def read_runtime_payload(step_id):
    if not step_id:
        return {}
    ref = f"workorch://workspaces/{workspace_id}/runs/{run['id']}/steps/{step_id}/output.json"
    url = base_url.rstrip("/") + "/api/v1/workorch/runtime/payloads?ref=" + urllib.parse.quote(ref, safe="")
    try:
        with urllib.request.urlopen(url, timeout=20) as response:
            return json.loads(response.read().decode("utf-8"))
    except Exception as exc:
        errors.append(f"failed to read runtime payload {ref}: {exc}")
        return {}

if run.get("state") != "completed":
    errors.append(f"run state is {run.get('state')!r}, want completed: {run.get('failureReason', '')}")
if routing.get("status") != "resolved":
    errors.append(f"soulRouting.status is {routing.get('status')!r}, want resolved")
if execution_binding.get("soulId") != "soul-weather-query-execution":
    errors.append(f"execution soul binding is {execution_binding!r}, want soul-weather-query-execution")
if not any(item.get("id") == ticket_id and item.get("state") == "resolved" for item in decisions):
    errors.append(f"resolved Decision Ticket {ticket_id} missing from snapshot decisions={decisions}")
if not any(item.get("id") == approval_id and item.get("state") == "approved" for item in approvals):
    errors.append(f"approved Approval {approval_id} missing from snapshot approvals={approvals}")

output = {}
mcp_steps = [item for item in steps if item.get("kind") == "mcp/tool_call"]
if not mcp_steps:
    errors.append("missing mcp/tool_call weather step")
    structured = {}
else:
    selected_step = None
    for candidate in mcp_steps:
        candidate_output = candidate.get("output") or {}
        candidate_result = candidate_output.get("result") or {}
        candidate_structured = candidate_result.get("structuredContent") or {}
        if candidate_output.get("tool") == "weather.current" or candidate_structured.get("query"):
            selected_step = candidate
            break
    if selected_step is None:
        selected_step = mcp_steps[0]
    output = selected_step.get("output") or {}
    result = output.get("result") or {}
    structured = result.get("structuredContent") or {}
    if not structured:
        raw_output = read_runtime_payload(selected_step.get("id"))
        if raw_output:
            output = raw_output
            result = output.get("result") or {}
            structured = result.get("structuredContent") or {}
    query = structured.get("query") or {}
    current = structured.get("current") or {}
    if output.get("transport") != "stdio":
        errors.append(f"mcp transport is {output.get('transport')!r}, want stdio")
    if output.get("tool") != "weather.current":
        errors.append(f"tool is {output.get('tool')!r}, want weather.current")
    if query.get("country") != country or query.get("province") != province or query.get("city") != city:
        errors.append(f"weather query does not match target city: {query}")
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
    if city not in summary or "气温" not in summary or "湿度" not in summary:
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
    if city not in str(result.get("summary") or ""):
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
print(f"ticketId={ticket_id}")
print(f"approvalId={approval_id}")
print(f"location={location.get('admin1')}{location.get('name')}")
print(f"observedAt={current.get('observedAt')}")
print(f"temperatureC={current.get('temperatureC')}")
print(f"weather={current.get('weatherDescription')}")
print(f"humidityPercent={current.get('humidityPercent')}")
print(f"windSpeedKmh={current.get('windSpeedKmh')}")
print(f"steps={len(steps)}")
print(f"toolCalls={len(tool_calls)}")
print(f"validations={len(validations)}")
print(f"runtimeCheckpoints={len(checkpoints)}")
print(f"results={len(results)}")
print(f"artifacts={len(artifacts)}")
PY
}

trap report_failure ERR
trap cleanup EXIT

acquire_llm_config_lock

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
if [[ "$LLM_MODE" == "mock" ]]; then
  start_mock_provider
  wait_mock
  write_llm_update_payloads
  curl -fsS -H "Content-Type: application/json" -X PUT \
    "$BASE_URL/api/v1/workorch/configuration/llm" \
    -d @"$TMP_DIR/verify-llm-update.json" >"$TMP_DIR/configuration-updated.json"
else
  PROVIDER_ID="$(select_real_provider)"
fi

upsert_json "$BASE_URL/api/v1/workorch/mcp/servers" \
  "$BASE_URL/api/v1/workorch/mcp/servers/$MCP_SERVER_ID" \
  '{"id":"'"$MCP_SERVER_ID"'","name":"weather governance MCP","transport":"stdio","endpoint":"'"$WEATHER_HELPER"'","enabled":true,"allowedTools":["weather.current"],"defaultApprovalPolicy":"operator","sessionPolicy":"long_lived","timeoutMillis":30000}' \
  "$TMP_DIR/mcp-server.json"

write_json_payload "$TMP_DIR/workspace-request.json" workspace
post_json_file "$BASE_URL/api/v1/workorch/workspaces" "$TMP_DIR/workspace-request.json" "$TMP_DIR/workspace.json"
workspace_id="$(json_path "$TMP_DIR/workspace.json" "workspace.id")"
[[ -n "$workspace_id" ]]

write_json_payload "$TMP_DIR/goal-request.json" goal
post_json_file "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/goals" "$TMP_DIR/goal-request.json" "$TMP_DIR/goal.json"
goal_id="$(json_path "$TMP_DIR/goal.json" "goal.id")"
[[ -n "$goal_id" ]]

write_json_payload "$TMP_DIR/dod-request.json" dod
post_json_file "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/dods" "$TMP_DIR/dod-request.json" "$TMP_DIR/dod.json"
dod_id="$(json_path "$TMP_DIR/dod.json" "dod.id")"
[[ -n "$dod_id" ]]

activation_request_id="weather-governance-e2e-$VERIFY_SUFFIX"
activation_reason="e2e_resume"
capture_runs_before_activation "$TMP_DIR/decision-activation-before-runs.json"
post_json "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/execution-activations" \
  '{"intent":"resume","requestId":"'"$activation_request_id"'","reason":"'"$activation_reason"'","source":"e2e","goalId":"'"$goal_id"'","doDId":"'"$dod_id"'","llmTarget":{"providerId":"'"$PROVIDER_ID"'","modelId":"verify-model"}","mcpServerIds":["'"$MCP_SERVER_ID"'"],"priority":3}' \
  "$TMP_DIR/decision-activation.json"
validate_activation_command_response "$TMP_DIR/decision-activation.json" "$activation_request_id" "$activation_reason"
run_id="$(wait_activation_run "$TMP_DIR/decision-activation-before-runs.json" "$TMP_DIR/decision-activation-runs.json" "$TMP_DIR/decision-activation-run.json")"
wait_run_state_any "$run_id" "$TMP_DIR/decision-status.json" "blocked_on_decision" "blocked_on_approval" "completed"
activation_state="$(json_path "$TMP_DIR/decision-status.json" "status.state")"
activation_state_file="$TMP_DIR/decision-status.json"
activation_state_prefix="status"

python3 - "$TMP_DIR/decision-resolution.json" "$COUNTRY" "$PROVINCE" "$CITY" "$LOOKUP_NAME" <<'PY'
import json
import sys

output, country, province, city, lookup_name = sys.argv[1:]
payload = {
    "resolutionType": "provide_missing_info",
    "resolutionPayload": {
        "operator": "weather-governance-e2e",
        "country": country,
        "province": province,
        "city": city,
        "lookupName": lookup_name,
    },
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump(payload, handle, ensure_ascii=False, separators=(",", ":"))
PY

if [[ "$activation_state" == "blocked_on_decision" ]]; then
  ticket_id="$(json_path "$activation_state_file" "$activation_state_prefix.blockingTicketId")"
  ticket_path="$(url_encode "$ticket_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/decisions/$ticket_path?workspaceId=$workspace_id" >"$TMP_DIR/decision-ticket.json"
  grep '"decisionKind": "provide_missing_info"' "$TMP_DIR/decision-ticket.json" >/dev/null
  grep "$CITY" "$TMP_DIR/decision-ticket.json" >/dev/null
  curl -fsS -H "Content-Type: application/json" -X POST \
    "$BASE_URL/api/v1/workorch/decisions/$ticket_path/resolve?workspaceId=$workspace_id" \
    -d @"$TMP_DIR/decision-resolution.json" >"$TMP_DIR/decision-resolved.json"
  grep '"resolutionType": "provide_missing_info"' "$TMP_DIR/decision-resolved.json" >/dev/null
  wait_run_state "$run_id" "blocked_on_approval" "$TMP_DIR/approval-status.json"
elif [[ "$activation_state" == "blocked_on_approval" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$TMP_DIR/approval-status.json"
elif [[ "$activation_state" == "completed" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/decision-auto-list.json"
  grep '"decisionKind": "provide_missing_info"' "$TMP_DIR/decision-auto-list.json" >/dev/null
  grep '"state": "resolved"' "$TMP_DIR/decision-auto-list.json" >/dev/null
else
  echo "weather governance activation state=$activation_state, want blocked_on_decision, blocked_on_approval, or completed" >&2
  exit 1
fi

if [[ "$activation_state" != "completed" ]]; then
  wait_workspace_list_projection "$workspace_id" "$run_id" "blocked" "$TMP_DIR/workspaces-blocked.json"
  approval_id="$(json_path "$TMP_DIR/approval-status.json" "status.blockingApprovalId")"
  approval_path="$(url_encode "$approval_id")"
  curl -fsS "$BASE_URL/api/v1/workorch/approvals/$approval_path?workspaceId=$workspace_id" >"$TMP_DIR/approval-pending.json"
  grep '"state": "pending"' "$TMP_DIR/approval-pending.json" >/dev/null
  grep "$CITY" "$TMP_DIR/approval-pending.json" >/dev/null

  curl -fsS -X POST "$BASE_URL/api/v1/workorch/approvals/$approval_path/approve?workspaceId=$workspace_id" >"$TMP_DIR/approval-approved.json"
  grep '"state": "approved"' "$TMP_DIR/approval-approved.json" >/dev/null
  wait_run_state "$run_id" "completed" "$TMP_DIR/run-completed-status.json"
else
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/approval-auto-list.json"
  grep '"state": "approved"' "$TMP_DIR/approval-auto-list.json" >/dev/null
  curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/status" >"$TMP_DIR/run-completed-status.json"
  grep '"state": "completed"' "$TMP_DIR/run-completed-status.json" >/dev/null
fi

if [[ -z "${ticket_id:-}" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/decisions?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/decision-final-list.json"
  ticket_id="$(python3 - "$TMP_DIR/decision-final-list.json" <<'PY'
import json
import sys
items = json.load(open(sys.argv[1], encoding="utf-8")).get("decisions") or []
for item in items:
    if item.get("state") == "resolved":
        print(item.get("id", ""))
        break
else:
    raise SystemExit("missing resolved decision")
PY
)"
fi
if [[ -z "${approval_id:-}" ]]; then
  curl -fsS "$BASE_URL/api/v1/workorch/approvals?workspaceId=$workspace_id&runId=$run_id" >"$TMP_DIR/approval-final-list.json"
  approval_id="$(python3 - "$TMP_DIR/approval-final-list.json" <<'PY'
import json
import sys
items = json.load(open(sys.argv[1], encoding="utf-8")).get("approvals") or []
for item in items:
    if item.get("state") == "approved":
        print(item.get("id", ""))
        break
else:
    raise SystemExit("missing approved approval")
PY
)"
fi

curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/snapshot" >"$TMP_DIR/snapshot.json"
curl -fsS "$BASE_URL/api/v1/workorch/workspaces/$workspace_id/runs/$run_id/observability" >"$TMP_DIR/observability.json"

echo "workspaceId=$workspace_id"
echo "provider=$PROVIDER_ID"
echo "llmMode=$LLM_MODE"
validate_snapshot
echo "weather ticket and approval e2e verification passed for provider=$PROVIDER_ID mode=$LLM_MODE city=$COUNTRY/$PROVINCE/$CITY"
echo "artifacts=$TMP_DIR"
