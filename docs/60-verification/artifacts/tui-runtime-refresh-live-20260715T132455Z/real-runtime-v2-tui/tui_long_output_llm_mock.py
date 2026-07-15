import json
import os
import re
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

REQUEST_LOG = os.environ.get("WORKORCH_TUI_LONG_OUTPUT_MOCK_REQUEST_LOG", "")

def long_lines(prefix, count):
    return "\n".join(f"{prefix} {idx:02d} - viewport scroll verification marker" for idx in range(1, count + 1))

def completion_payload(request):
    request_text = request.get("requestText", "")
    action_match = re.search(r'"Action"\s*:\s*"([^"]+)"', request_text)
    focus_match = re.search(r'"Focus"\s*:\s*\{\s*"ID"\s*:\s*"([^"]+)"', request_text)
    coverage_match = re.search(r'"CoverageWorkItemIDs"\s*:\s*\[\s*"([^"]+)"', request_text)
    action = action_match.group(1) if action_match else "produce_validation"
    focus_id = focus_match.group(1) if focus_match else "focus-001"
    coverage_id = coverage_match.group(1) if coverage_match else "cwi-001"
    validation_op_id = f"validate-{focus_id}"
    goal = long_lines("LONG-GOAL-OUTCOME", 40)
    implementation = long_lines("LONG-IMPLEMENTATION", 80)
    verification = long_lines("LONG-VERIFICATION", 24)
    if action == "produce_validation":
        return {
            "operations": [{
                "id": validation_op_id,
                "kind": "file/read",
                "coverageWorkItemIds": [coverage_id],
                "targetRefs": ["runtime-validation-source.txt"],
                "parameters": [
                    {"name": "path", "string": "runtime-validation-source.txt"},
                    {"name": "maxBytes", "integer": 16384},
                ],
            }],
        }
    if action == "produce_assessment":
        return {
            "focusId": focus_id,
            "coverageWorkItemIds": [coverage_id],
            "satisfied": True,
            "summary": "\n".join([goal, implementation, verification]),
            # file/read emits a stable workspace URI as an accepted evidence
            # ref. The dispatch/event IDs are intentionally volatile and must
            # not be guessed by a provider fixture.
            "evidenceRefs": ["workorch://workspace/runtime-validation-source.txt"],
        }
    if action == "request_governance":
        return {"externalDependency": {"kind": "none", "reason": "deterministic viewport verification does not require governance"}}
    # This profile normally enters validation directly. Keep an explicit typed
    # state-change candidate for a future profile change instead of returning
    # legacy completionState/thoughtSummary fields.
    return {
        "operations": [{
            "id": f"state-{focus_id}",
            "kind": "file/edit",
            "coverageWorkItemIds": [coverage_id],
            "parameters": [],
        }],
    }

def legacy_completion_payload(request):
    # Kept only as a readable fixture for the detailed assessment text below;
    # Runtime V2 never receives this legacy shape.
    step_id = request.get("stepId", "") or "step-1"
    goal = long_lines("LONG-GOAL-OUTCOME", 40)
    implementation = long_lines("LONG-IMPLEMENTATION", 80)
    verification = long_lines("LONG-VERIFICATION", 24)
    return {
        "thoughtSummary": "Generated deterministic long output for TUI viewport verification.",
        "completionState": "completed",
        "dodAssessment": {
            "satisfied": True,
            "summary": "TUI long output e2e completed with more than one terminal screen of content.",
            "goalOutcome": goal,
            "implementationSummary": implementation,
            "verificationSummary": verification,
            "items": [
                {
                    "item": "Final Result contains LONG-IMPLEMENTATION 70",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                },
                {
                    "item": "Final Result is long enough to require PgDown/PgUp viewport scrolling",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                },
                {
                    "item": "Workspace monitor remains visible while main display scrolls",
                    "state": "satisfied",
                    "evidenceRefs": [step_id]
                }
            ],
            "goalCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "goal": "Generate deterministic long final output for TUI viewport verification.",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    }
                ]
            },
            "acceptanceDimensions": {
                "satisfied": True,
                "items": [
                    {
                        "id": "functional_behavior",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    },
                    {
                        "id": "deliverable",
                        "state": "satisfied",
                        "evidenceRefs": [step_id]
                    }
                ]
            },
            "deliverableCoverage": {
                "satisfied": True,
                "items": [
                    {
                        "id": "source_change",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "validation_report",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "coverage_map",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    },
                    {
                        "id": "user_deliverable",
                        "state": "satisfied",
                        "artifactRefs": [step_id]
                    }
                ]
            },
            "knownLimitations": ["Manual terminal drag selection must be confirmed in the operator terminal."],
            "residualRisks": ["Terminal emulator key binding differences can affect PgUp/PgDown delivery."],
            "nextActions": ["Open the generated workspace in TUI, press PgDown/PgUp, and verify the side panel stays fixed."]
        }
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
        size = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(size)
        if self.path == "/v1/embeddings":
            response = {"data": [{"embedding": [0.1, 0.2, 0.3]}]}
        elif self.path == "/v1/chat/completions":
            payload = json.loads(body or b"{}")
            if REQUEST_LOG:
                with open(REQUEST_LOG, "w", encoding="utf-8") as handle:
                    json.dump(payload, handle, ensure_ascii=False, indent=2)
            payload_text = json.dumps(payload, ensure_ascii=False)
            id_matches = re.findall(r'run-\d+(?:-replan-\d+)?(?:/[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)*)?', payload_text)
            def run_sort_key(value):
                base = value.split("/", 1)[0]
                match = re.match(r"run-(\d+)", base)
                return int(match.group(1)) if match else -1
            step_candidates = [
                item for item in id_matches
                if re.search(r'/(step-\d+|repair-[A-Za-z0-9._-]+|provider-retry-[A-Za-z0-9._-]+|follow-up-[A-Za-z0-9._-]+|graph-[A-Za-z0-9._-]+)$', item)
            ]
            step_match = max(step_candidates, key=run_sort_key, default="")
            run_candidates = [item.split("/", 1)[0] for item in id_matches]
            run_match = max(run_candidates, key=run_sort_key, default="")
            if not step_match and run_match:
                step_match = run_match + "/step-1"
            request_text = "\n".join(str(item.get("content", "")) for item in payload.get("messages", []) if isinstance(item, dict))
            provider_request = {"runId": run_match, "stepId": step_match, "requestText": request_text}
            content = json.dumps(completion_payload(provider_request), separators=(",", ":"))
            response = {"choices": [{"message": {"content": content}}]}
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

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

host, port_text = os.environ.get("WORKORCH_TUI_LONG_OUTPUT_MOCK_ADDR", "127.0.0.1:18182").split(":", 1)
ThreadingHTTPServer((host, int(port_text)), Handler).serve_forever()
