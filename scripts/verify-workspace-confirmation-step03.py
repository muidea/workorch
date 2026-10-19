#!/usr/bin/env python3
"""No-charge STEP03 production HTTP/browser/restart verification, no Run.

Only provider output is a loopback fixture. All authoring, confirmation,
admission, File and Decision facts are produced by real production owners.
Raw evidence/configuration stays in a new restricted durable directory.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import time

import build_identity

spec = importlib.util.spec_from_file_location("step02", Path(__file__).with_name("verify-workspace-candidate-step02.py"))
step02 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step02)
step01, require, save = step02.step01, step02.require, step02.save
BLOCKED_TEMPLATE = {"id": "step03-blocked", "name": "STEP03 缺交付证明阻塞", "contextHints": {"profileContracts": {"software-dev": {"evidenceRequirements": [{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": ["validation"], "acceptedStepKinds": ["validation/managed_session"]}], "deliverableExpectations": [{"id": "step03-required-unproven", "required": True}]}}}}


def confirmation(view):
    return {"inputHash": view["inputHash"], "derivationPermitId": view["derivationPermitId"], "proposalHash": view["proposal"]["proposalHash"], "specificationHash": view["proposal"]["specificationHash"], "expectedWorkspaceRevision": view["workspaceRevision"]}


def inspect(verification, view, original, blocked):
    workspace_id = view["workspaceId"]
    record = step02.workspace_record(json.loads((verification.data / "internal/workspaces" / workspace_id / "workspace.json").read_bytes()), workspace_id)
    journal = record["DefinitionAuthoringConfirmation"]
    require(journal["Phase"] == "materialized", "confirmation not materialized")
    for wire, stored in (("inputHash", "InputHash"), ("derivationPermitId", "DerivationPermitID"), ("proposalHash", "ProposalHash"), ("specificationHash", "SpecificationHash")):
        require(journal[stored] == original[wire], "journal changed original confirmation")
    candidates = record["DefinitionAdmissionCandidates"]
    require(len(candidates) == 1, "duplicate candidate")
    candidate = candidates[0]
    require(candidate["CandidateHash"] == view["candidateHash"], "candidate hash projection mismatch")
    for wire, stored in (("inputHash", "InputHash"), ("derivationPermitId", "DerivationPermitID"), ("proposalHash", "ProposalHash"), ("specificationHash", "SpecificationHash")):
        require(candidate["AuthoringRef"][stored] == original[wire], "candidate lost authoring source")
    path = f"/api/v1/workorch/workspaces/{workspace_id}"
    goals = verification.request("GET", path + "/goals")["goals"]
    dods = verification.request("GET", path + "/dods")["dods"]
    require(len(goals) == 1 and len(dods) == 1, "formal materialization duplicated/missing")
    require(goals[0]["text"] == step02.GOAL and journal["Goal"]["Text"] == step02.GOAL, "original Goal changed")
    require(verification.request("GET", path + "/runs")["runs"] == [] and not record.get("ActiveRunID"), "STEP03 created a Run")
    directory = Path(record["DeliverableDirectory"])
    require(directory.is_dir() and not any(directory.iterdir()), "STEP03 dispatched a deliverable effect")
    if blocked:
        require(view["state"] == "clarification_required" and view.get("blockingTicketId") and not view.get("startupInputHash") and not record.get("StartupInput") and not record.get("ActivationAdmission"), "blocked candidate became eligible")
    else:
        startup, activation = record["StartupInput"], record["ActivationAdmission"]
        require(view["state"] == "admitted" and view["activationState"] == "eligible" and view["startupInputHash"], "not eligible")
        require(startup["StartupInputHash"] == activation["StartupInputHash"] == view["startupInputHash"], "startup identity mismatch")
        require(startup["DefinitionRevision"] == activation["DefinitionRevision"], "definition revision mismatch")
    return {"journal": journal, "candidate": candidate, "startup": record.get("StartupInput"), "activation": record.get("ActivationAdmission"), "goals": goals, "dods": dods}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--browser-hold-seconds", type=int, default=0, help="browser must explicitly confirm both prepared candidates; finish with browser-done marker")
    args = parser.parse_args()
    require(0 <= args.browser_hold_seconds <= 1800, "bounded browser window required")
    root = args.evidence_dir.absolute()
    require(not root.exists() and not str(root).startswith("/tmp/"), "new durable evidence directory required")
    root.mkdir(parents=True, mode=0o700)
    os.umask(0o077)
    source = Path(__file__).resolve().parent.parent
    verification, fixture = None, None
    result = {"pass": False, "mode": "loopback-fixture", "realModelCalls": 0, "browserRequested": bool(args.browser_hold_seconds)}
    try:
        binary, identity = step01.build_candidate(source, root)
        verification = step01.Verification(binary, root, identity)
        fixture = step02.Fixture(root)
        config = {"server": {"addr": f"127.0.0.1:{verification.port}"}, "repository": {"backend": "file", "dataDir": str(verification.data)}, "llm": {"aiProxy": {"baseURL": f"http://127.0.0.1:{fixture.server.server_port}", "apiKey": "", "timeout": "30s", "modelOverrides": {"step02-fixture": {"contextWindowTokens": 128000, "maxOutputTokens": 16384, "operations": ["responses"]}}}, "defaultChat": {"providerID": "ai-proxy", "modelID": "step02-fixture"}, "budget": {"contextUtilizationRatio": 0.75, "estimatedTokensPerByte": 0.25}}}
        (verification.config / "application.toml").write_text(step02.toml_document(config), encoding="utf-8")
        result["buildIdentity"] = identity
        verification.start()
        verification.request("POST", "/api/v1/workorch/workspace-templates", BLOCKED_TEMPLATE, 201)
        prepared = []
        for blocked in (False, True):
            payload = {"requestId": "step03-blocked" if blocked else "step03-eligible", "title": "STEP03 阻塞验收" if blocked else "STEP03 成功验收", "templateId": "step03-blocked" if blocked else "software-dev", "input": {"goalText": step02.GOAL, "acceptanceRequirements": step02.REQUIREMENTS}}
            created = verification.create(payload)["authoring"]
            path = f'/api/v1/workorch/workspaces/{created["workspaceId"]}/authoring'
            verification.request("POST", path + "/compile", {"expectedInputHash": created["inputHash"], "expectedTemplateSnapshotHash": created["templateSnapshotHash"]}, 202)
            deadline = time.monotonic() + 60
            view = created
            while time.monotonic() < deadline:
                view = verification.read(created["workspaceId"])
                if step02.compilation_settled(view):
                    break
                time.sleep(0.2)
            step02.validate_candidate(view, payload)
            step02.assert_no_transition(verification, view["workspaceId"])
            exact = confirmation(view)
            # Bad first intents cannot create a journal. These 409s precede writes.
            verification.request("POST", path + "/confirm", {**exact, "inputHash": "stale"}, 409)
            verification.request("POST", path + "/confirm", {**exact, "expectedWorkspaceRevision": exact["expectedWorkspaceRevision"] - 1}, 409)
            require(verification.read(view["workspaceId"]) == view, "rejected first intent mutated owner")
            prepared.append({"workspaceId": view["workspaceId"], "blocked": blocked, "exact": exact, "ready": view})
        calls = list(fixture.calls)
        require(len(calls) == 16, "fixture preparation call count")
        save(root, "prepared-candidates.json", prepared)
        if args.browser_hold_seconds:
            save(root, "browser-session.json", {"baseURL": verification.base, "workspaces": prepared, "pid": verification.process.pid})
            print(json.dumps({"browserURL": verification.base + "/console/", "workspaces": [item["workspaceId"] for item in prepared]}), flush=True)
            deadline = time.monotonic() + args.browser_hold_seconds
            while time.monotonic() < deadline and not (root / "browser-done").exists():
                time.sleep(1)
            require((root / "browser-done").exists(), "browser inspection not completed; do not claim browser pass")
        else:
            for item in prepared:
                verification.request("POST", f'/api/v1/workorch/workspaces/{item["workspaceId"]}/authoring/confirm', item["exact"])
        snapshots = []
        for item in prepared:
            view = verification.read(item["workspaceId"])
            facts = inspect(verification, view, item["exact"], item["blocked"])
            replay = verification.request("POST", f'/api/v1/workorch/workspaces/{item["workspaceId"]}/authoring/confirm', item["exact"])["authoring"]
            require(replay == view, "exact replay changed owner facts")
            snapshots.append({"view": view, "facts": facts})
        require(fixture.calls == calls, "confirmation/admission called provider")
        save(root, "confirmed-owner-facts.json", snapshots)
        verification.stop()
        verification.start()
        for item, snapshot in zip(prepared, snapshots):
            observed = verification.read(item["workspaceId"])
            require(observed == snapshot["view"], "process restart changed confirmation identity")
            require(inspect(verification, observed, item["exact"], item["blocked"]) == snapshot["facts"], "durable owner facts changed after restart")
            replay = verification.request("POST", f'/api/v1/workorch/workspaces/{item["workspaceId"]}/authoring/confirm', observed["confirmationRecovery"])["authoring"]
            require(replay == observed, "restart exact recovery changed facts")
        require(fixture.calls == calls, "restart/recovery called provider")
        require(fixture.post_paths == ["/v1/responses"] * 16, "unexpected provider POST/embedding attempt")
        require(verification.request("GET", "/api/v1/workorch/runs")["runs"] == [], "Run inventory not empty")
        build_identity.verify(source, source / "web/consoleassets/dist", binary, root / "build-identity.json", {key: identity[key] for key in build_identity.IDENTITY_FIELDS}, root)
        result.update({"pass": True, "restartReadbackPassed": True, "providerPreparationCalls": len(calls), "confirmationProviderCalls": 0, "embeddingCalls": 0, "runCount": 0, "workspaces": [{"workspaceId": item["workspaceId"], "blocked": item["blocked"], **item["exact"]} for item in prepared]})
    finally:
        if verification:
            verification.stop()
            save(root, "http-trace.json", verification.trace)
            result["pids"] = verification.pids
        if fixture:
            fixture.close()
            result["physicalCalls"] = fixture.calls
            result["providerPostPaths"] = fixture.post_paths
        save(root, "result.json", result)
    print(json.dumps(result, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
