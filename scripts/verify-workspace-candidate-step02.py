#!/usr/bin/env python3
"""STEP02 production HTTP/process verification, stopping before confirmation.

fixture replaces only the outgoing provider HTTP response with a loopback server.
live requires explicit opt-in and uses the selected existing configuration.
Evidence must be stored in a new, durable, restricted directory, never /tmp.
"""
import argparse
import base64
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import threading
import time
import tomllib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import build_identity

spec = importlib.util.spec_from_file_location("step01", Path(__file__).with_name("verify-workspace-draft-step01.py"))
step01 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step01)
require = step01.require
KINDS = ("dimension_coverage", "negative_path", "regression", "resource_environment", "traceability", "verifier_feasibility")
GOAL = "编写一个本地 Go 命令行工具，将参数作为 UTF-8 文本逐行输出；保留输入顺序，空参数时输出明确错误并返回非零状态。"
REQUIREMENTS = ["正常输入按原顺序逐行输出，不排序、不去重、不截断。", "空输入返回非零退出码并提供明确错误，不伪称成功。"]


def save(root, name, value):
    (root / name).write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")


def toml_document(data):
    """Serialize configuration values without logging or copying them into reports."""
    lines = []
    def table(value, path):
        if path:
            lines.append("[" + ".".join(json.dumps(key) for key in path) + "]")
        for key, item in value.items():
            if not isinstance(item, dict):
                require(isinstance(item, (str, int, float, bool, list)), "unsupported configuration value")
                lines.append(json.dumps(key) + " = " + json.dumps(item, ensure_ascii=False))
        for key, item in value.items():
            if isinstance(item, dict):
                table(item, path + [key])
    table(data, [])
    return "\n".join(lines) + "\n"


def prompt_payload(text):
    decoder = json.JSONDecoder()
    matches = []
    for index, char in enumerate(text):
        if char != "{":
            continue
        try:
            value, _ = decoder.raw_decode(text[index:])
        except ValueError:
            continue
        if isinstance(value, dict) and ("outputShape" in value or "candidateAcceptance" in value):
            matches.append(value)
    require(len(matches) == 1, "fixture requires exactly one complete prompt payload")
    return matches[0]


class Fixture:
    def __init__(self, root):
        self.root, self.calls, self.reviews = root, [], {}
        self.post_paths = []  # Include rejected endpoints, e.g. unintended embedding.
        fixture = self
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_GET(self):
                if self.path != "/v1/models":
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                wire = json.dumps({"object": "list", "data": [{"id": "step02-fixture", "supported_endpoints": ["/v1/responses"], "contextWindowTokens": 128000, "maxOutputTokens": 16384}]}).encode()
                (fixture.root / "provider-models-response.bin").write_bytes(wire)
                self.wfile.write(wire)
            def do_POST(self):
                fixture.post_paths.append(self.path)
                if self.path != "/v1/responses":
                    self.send_error(404)
                    return
                raw = self.rfile.read(int(self.headers["Content-Length"]))
                number = len(fixture.calls) + 1
                (fixture.root / f"provider-{number:02d}-request.bin").write_bytes(raw)
                try:
                    request = json.loads(raw)
                    require(request["model"] == "step02-fixture", "unexpected fixture model")
                    payload = prompt_payload(request["input"])
                    if "outputShape" in payload:
                        kind = "derive"
                        result = {**payload["outputShape"], "AdditionalItems": ["对正常和错误路径分别声明可执行验证，并保留回归验证。"]}
                    elif "kind" not in payload:
                        kind = "proposal"
                        items = payload["candidateAcceptance"]
                        fixture.validate_source_consumers(payload)
                        declaration = {"Declaration": "绑定已发布的 software-dev 逐条验收能力声明，尚未执行。", "Status": "DECLARED", "ProfileID": "software-dev", "EvidenceRequirementID": "*", "Scope": "dodItem"}
                        result = {"mapping": {
                            "Protocol": "workorch.authoring.sealed-ref-mapping/v1", "InputHash": payload["inputHash"], "TemplateSnapshotHash": payload["templateHash"], "ResourceSnapshotHash": payload["resourceSnapshotHash"],
                            "GoalRequirement": {"DimensionRef": "all", "ResourceIDs": []}, "Dimensions": [{"Ref": "all", "ResourceIDs": []}],
                            "Requirements": [{"Ref": item["CandidateItemRef"], "DimensionRef": "all", "SourceItemRefs": [item["CandidateItemRef"]], "ResourceIDs": [], "Text": item["Text"]} for item in items],
                            "Obligations": [{"CandidateItemRef": item["CandidateItemRef"], "RequirementRef": item["CandidateItemRef"], "Verification": declaration} for item in items],
                        }}
                    else:
                        kind = "review/" + payload["kind"]
                        proposal = payload["specification"]
                        require(not proposal.get("Reviews"), "critic saw another critic's review")
                        previous = fixture.reviews.setdefault(payload["proposalHash"], proposal)
                        require(proposal == previous, "critics received different proposals")
                        result = {"review": {"Kind": payload["kind"], "Status": "PASS", "SpecificationHash": payload["proposalHash"], "Reason": "逐条对照固定源承诺，exact Goal、required 与 all_of 原文及声明绑定保留；这是确定性 fixture，不代替人工语义评审。", "EvidenceRefs": [payload["goal"]["CandidateItemRef"]], "RequirementIDs": payload["requiredRequirementIDs"]}}
                    if payload.get("wireContractRef") == "workorch.authoring.owner-bound/v2":
                        if kind == "derive":
                            result = {"AdditionalItems": result["AdditionalItems"]}
                        elif kind == "proposal":
                            for field in ("Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"):
                                del result["mapping"][field]
                        else:
                            del result["review"]["Kind"]
                            del result["review"]["SpecificationHash"]
                    fixture.calls.append(kind)
                    wire = ("data: " + json.dumps({"type": "response.completed", "response": {"status": "completed", "output_text": json.dumps(result, ensure_ascii=False), "usage": {"input_tokens": 128, "output_tokens": 128, "total_tokens": 256}}}, ensure_ascii=False) + "\n\n").encode()
                    (fixture.root / f"provider-{number:02d}-response.bin").write_bytes(wire)
                    self.send_response(200)
                    self.send_header("Content-Type", "text/event-stream")
                    self.end_headers()
                    self.wfile.write(wire)
                except Exception as error:
                    save(fixture.root, "fixture-error.json", {"type": type(error).__name__, "message": str(error)})
                    self.send_error(500, "fixture rejected request; inspect restricted evidence")
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
    def validate_source_consumers(self, payload):
        validate_source_consumers(payload)

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


def validate_source_consumers(payload):
    options = payload['sourceApplicableVerificationDeclarations']
    expected = {'derived-dod:template:000003': 'terminal_owner_operation_receipt',
                'derived-dod:template:000004': 'terminal_owner_snapshot',
                'derived-dod:template:000013': 'terminal_result_evidence_index'}
    observed = set()
    for option in options:
        ref = option['candidateItemRef']
        for declaration in option['declarations']:
            consumers = declaration['consumers']
            if ref not in expected:
                require(not consumers, 'guard leaked to unrelated source')
                continue
            require(len(consumers) == 1 and consumers[0]['Kind'] == expected[ref] and
                    consumers[0]['ConsumptionPoint'] == 'post_terminal_delivery' and
                    consumers[0]['TerminalContractRef'], 'missing exact terminal consumer')
            observed.add(ref)
    require(observed == set(expected), 'template consumer source missing')


def validate_owner_bound_ledger(record):
    ledger = record['DefinitionAuthoring']
    derives, requests = ledger['DerivationRequests'], ledger['SpecificationRequests']
    require(len(derives) == 1 and len(requests) == 7, 'unexpected provider ledger cardinality')
    for request in [*derives, *requests]:
        require(request['WireContractRef'] == 'workorch.authoring.owner-bound/v2' and
                request['WireContractHash'].startswith('authoring-wire-sha256:'), 'wire identity not sealed')
        audit = request.get('ReviewAudit') or request.get('SuccessAudit')
        require(audit is not None, 'successful transaction lost original document')
        document = audit['Document']
        raw = base64.b64decode(document['Content'], validate=True)
        require(document['Complete'] and document['Saved'] and len(raw) == document['ObservedBytes'] and
                hashlib.sha256(raw).hexdigest() == document['SHA256'], 'original bytes mismatch')
        value = json.loads(raw)
        if 'AdditionalItems' in value:
            require(set(value) == {'AdditionalItems'}, 'derive echoed owner identity')
        elif 'mapping' in value:
            require(not {'Protocol', 'InputHash', 'TemplateSnapshotHash', 'ResourceSnapshotHash'} & set(value['mapping']), 'mapping echoed owner identity')
        else:
            require(set(value['review']) == {'Status', 'Reason', 'EvidenceRefs', 'RequirementIDs'}, 'review echoed owner identity')


def compilation_settled(view):
    # A FAIL may project review_blocked while subsequent independent critics
    # are still running. It is not a completed six-review observation.
    if view["state"] in ("failed", "preparation_failed"):
        return True
    reviews = view.get("proposal", {}).get("reviews", [])
    return view["state"] in ("ready_to_confirm", "review_blocked") and len(reviews) == len(KINDS) and {review["kind"] for review in reviews} == set(KINDS)


def validate_candidate(view, source):
    require(view["input"] == source["input"], "persisted input changed")
    proposal = view.get("proposal")
    require(view["state"] == "ready_to_confirm" and proposal, "candidate did not reach ready_to_confirm")
    require(proposal["goalText"] == source["input"]["goalText"], "exact Goal changed")
    required = {item["id"] for item in proposal["requirements"] if item["required"]}
    for text in [source["input"]["goalText"], *source["input"]["acceptanceRequirements"]]:
        # Obligations preserve exact source even when requirements group sources.
        require(any(item["text"] == text for item in proposal["obligations"]) or any(item["text"] == text and item["required"] for item in proposal["requirements"]), "fixed original commitment missing")
    reviews = proposal["reviews"]
    require(len(reviews) == 6 and {review["kind"] for review in reviews} == set(KINDS), "not exactly six review kinds")
    for review in reviews:
        require(review["status"] == "PASS" and review["reason"] and review["evidenceRefs"] and required <= set(review["requirementIds"]) and review["specificationHash"] == proposal["proposalHash"], "incomplete independent review")
    require(proposal["verifications"] and all(item["status"] == "DECLARED" and item.get("declaration", "").strip() and item.get("profileId") and item.get("evidenceRequirementId") and item.get("scope") for item in proposal["verifications"]), "empty or unbound validation declaration")


def workspace_record(document, workspace_id):
    require(set(document) == {workspace_id}, "repository record scope mismatch")
    record = document[workspace_id]
    require(record["ID"] == workspace_id and record.get("DefinitionAuthoring"), "missing saved authoring record")
    return record


def assert_no_transition(verification, workspace_id):
    require(verification.request("GET", f"/api/v1/workorch/workspaces/{workspace_id}/runs")["runs"] == [], "STEP02 created a Run")
    record = workspace_record(json.loads((verification.data / "internal/workspaces" / workspace_id / "workspace.json").read_bytes()), workspace_id)
    # Repository JSON uses exported Go field names, not management DTO names.
    for field in ("DefinitionAuthoringConfirmation", "StartupInput", "ActivationAdmission", "ActiveRunID", "DefinitionAdmissionCandidates"):
        require(not record.get(field), "STEP02 crossed confirmation/admission")
    return record


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--mode", choices=("fixture", "live"), default="fixture")
    parser.add_argument("--allow-live", action="store_true")
    parser.add_argument("--config-dir", type=Path)
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--browser-hold-seconds", type=int, default=0, help="bounded time to inspect the production console before restart")
    args = parser.parse_args()
    require(args.mode != "live" or args.allow_live and args.config_dir, "live requires explicit authorization and config directory")
    require(0 < args.timeout <= 600 and 0 <= args.browser_hold_seconds <= 1800, "verification must remain bounded")
    root = args.evidence_dir.absolute()
    require(not root.exists() and not str(root).startswith("/tmp/"), "use a new durable evidence directory outside /tmp")
    root.mkdir(parents=True, mode=0o700)
    os.umask(0o077)
    source = Path(__file__).resolve().parent.parent
    fixture, verification = None, None
    result = {"mode": args.mode, "pass": False}
    try:
        binary, identity = step01.build_candidate(source, root)
        verification = step01.Verification(binary, root, identity)
        if args.mode == "fixture":
            fixture = Fixture(root)
            config = {"llm": {"aiProxy": {"baseURL": f"http://127.0.0.1:{fixture.server.server_port}", "apiKey": "", "timeout": "30s", "modelOverrides": {"step02-fixture": {"contextWindowTokens": 128000, "maxOutputTokens": 16384, "operations": ["responses"]}}}, "defaultChat": {"providerID": "ai-proxy", "modelID": "step02-fixture"}, "budget": {"contextUtilizationRatio": 0.75, "estimatedTokensPerByte": 0.25}}}
        else:
            config = tomllib.loads((args.config_dir / "application.toml").read_text())
        config.setdefault("server", {})["addr"] = f"127.0.0.1:{verification.port}"
        config["repository"] = {"backend": "file", "dataDir": str(verification.data)}
        document = toml_document(config)
        require(tomllib.loads(document) == config, "isolated configuration must round-trip exactly")
        (verification.config / "application.toml").write_text(document, encoding="utf-8")
        result.update(buildIdentity=identity, configSha256=hashlib.sha256((verification.config / "application.toml").read_bytes()).hexdigest())
        verification.start()
        payload = {"requestId": "step02-" + args.mode, "title": "STEP02 合同候选验收", "templateId": "software-dev", "input": {"goalText": GOAL, "acceptanceRequirements": REQUIREMENTS}}
        save(root, "fixed-source-commitments.json", payload)
        created = verification.create(payload)["authoring"]
        workspace_id = created["workspaceId"]
        path = f"/api/v1/workorch/workspaces/{workspace_id}/authoring"
        require(verification.read(workspace_id) == created, "opening query mutated draft")
        if fixture:
            require(fixture.calls == [], "read-only opening contacted provider")
        expected = {"expectedInputHash": created["inputHash"], "expectedTemplateSnapshotHash": created["templateSnapshotHash"]}
        verification.request("POST", path + "/compile", {}, 400)
        verification.request("POST", path + "/compile", {**expected, "expectedInputHash": "stale"}, 409)
        accepted = verification.request("POST", path + "/compile", expected, 202)["authoring"]
        require(accepted["compilationAccepted"] and accepted["inputHash"] == created["inputHash"], "acceptance identity changed")
        view = accepted
        deadline = time.monotonic() + args.timeout
        while time.monotonic() < deadline:
            view = verification.read(workspace_id)
            if compilation_settled(view):
                break
            time.sleep(1)
        save(root, "candidate-result.json", view)
        result.update(workspaceId=workspace_id, state=view["state"], inputHash=view["inputHash"], templateSnapshotHash=view["templateSnapshotHash"])
        require(compilation_settled(view), "bounded observation ended before compilation settled; retain facts, do not retry")
        validate_owner_bound_ledger(assert_no_transition(verification, workspace_id))
        for request in view.get("requests", []):
            if request.get("diagnostic"):
                diagnostic = verification.request("GET", path + "/diagnostics/" + request["permitId"])
                save(root, "diagnostic-" + request["phase"] + "-" + (request.get("reviewKind") or "proposal") + ".json", diagnostic)
        replay = verification.request("POST", path + "/compile", expected)["authoring"]
        require(replay == view, "completed replay changed result")
        if fixture and view["state"] == "ready_to_confirm":
            require(fixture.calls == ["derive", "proposal", *("review/"+kind for kind in KINDS)], "physical call replay/sequence mismatch")
        if args.browser_hold_seconds:
            save(root, "browser-session.json", {"baseURL": verification.base, "workspaceId": workspace_id, "pid": verification.process.pid, "holdSeconds": args.browser_hold_seconds})
            print(json.dumps({"browserURL": verification.base+"/console/", "workspaceId": workspace_id}), flush=True)
            # Bounded external browser inspection window. Poll without mutation.
            browser_deadline = time.monotonic() + args.browser_hold_seconds
            while time.monotonic() < browser_deadline and not (root / "browser-done").exists():
                time.sleep(1)
        verification.stop()
        verification.start()
        recovered = verification.read(workspace_id)
        require(recovered == view, "real process restart lost candidate or independent reviews")
        save(root, "restarted-candidate.json", recovered)
        validate_owner_bound_ledger(assert_no_transition(verification, workspace_id))
        result["restartReadbackPassed"] = True
        build_identity.verify(source, source / "web/consoleassets/dist", binary, root / "build-identity.json", {key: identity[key] for key in build_identity.IDENTITY_FIELDS}, root)
        validate_candidate(view, payload)
        result.update({"pass": True, "pids": verification.pids, "proposalHash": view["proposal"]["proposalHash"], "specificationHash": view["proposal"]["specificationHash"], "reviewCount": len(view["proposal"]["reviews"])})
    finally:
        if verification:
            verification.stop()
            save(root, "http-trace.json", verification.trace)
            result["pids"] = verification.pids
        if fixture:
            fixture.close()
            result["physicalCalls"] = fixture.calls
            result["postPaths"] = fixture.post_paths
            require(fixture.post_paths == ["/v1/responses"] * len(fixture.calls), "unexpected provider POST path")
        save(root, "result.json", result)
    print(json.dumps(result, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
