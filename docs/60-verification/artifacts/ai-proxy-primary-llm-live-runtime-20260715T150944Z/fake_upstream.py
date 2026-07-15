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
