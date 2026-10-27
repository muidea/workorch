#!/usr/bin/env python3
"""No-charge STEP04 blocked mainline: production HTTP and OS process restart.

Only authoring responses and empty SkillHub status use isolated fixtures.
STEP04 itself must issue zero provider HTTP requests (including catalog GET).
No live configuration is read; no Run, MCP, readiness or M3 is created.
Evidence stays in a new restricted durable directory, not in Git.
"""
import argparse
import copy
from datetime import datetime, timezone
import hashlib
import http.client
import importlib.util
import json
import os
import re
from pathlib import Path
import shutil
import time

import build_identity

spec = importlib.util.spec_from_file_location("step03", Path(__file__).with_name("verify-workspace-confirmation-step03.py"))
step03 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step03)
step02, step01 = step03.step02, step03.step01
require, save = step03.require, step03.save

CHECK_OWNERS = {"exact_seal": "workspace", "directory_boundary": "file", "managed_configuration": "file",
                "soul_selection": "soul", "skill_selection": "skill", "mcp_selection": "mcpregistry",
                "memory_selection": "memory", "provider_cache": "llm", "persistence_integrity": "workspace",
                "first_activation_no_run": "run"}
TEMPLATE = {"id": "step04-bounded", "name": "STEP04 有界检查（非 readiness）", "defaultSoulId": "step04-soul",
            "contextHints": {"managedWorkspaceConfigs": [{"path": "mcp/config.json", "content": {"fixture": "step04"}}],
                             "acceptanceSlicePolicy": {"schemaVersion": "workorch.acceptance_slice_policy", "maxWorkItems": 4, "maxFocuses": 64,
                                                       "maxCandidateContextItems": 4, "completionPolicy": "all_required_items_accepted", "resultPolicy": "slice_result_allowed"},
                             "profileContracts": {"software-dev": {"evidenceRequirements": [{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": ["validation"], "acceptedStepKinds": ["validation/managed_session"],
                                 "requiredOwnerFacts": [{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}],
                                 "ownerFactConvergences": [{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session", "requirePreexisting": True}]}]}}}}
RESOURCE = "STEP04 精确资源\r\n保持原始 bytes。\n".encode()
MANAGED = b'{\r\n"fixture":"step04"}\n'


class Fixture(step02.Fixture):
    def __init__(self, root):
        self.http_paths = []
        super().__init__(root)
        handler = self.server.RequestHandlerClass
        for method in ("GET", "POST"):
            original = getattr(handler, "do_" + method)
            def record(request, original=original, method=method):
                self.http_paths.append({"method": method, "path": request.path})
                original(request)
            setattr(handler, "do_" + method, record)


def preparation(view):
    # GET and POST have different envelopes; do not infer operation identity
    # from the latest attempt when an original request is selected.
    return view["preparation"]


def validate_bootstrap_compilation(view, intent):
    """Explicit structural product only; it cannot replace any checklist item."""
    require(view["requestId"] == intent["requestId"], "compilation selected wrong request")
    attempts = [a for a in preparation(view)["record"]["attempts"] if a["requestId"] == intent["requestId"]]
    require(len(attempts) == 1 and attempts[0]["expectedWorkspaceRevision"] == intent["expectedWorkspaceRevision"], "compilation selected wrong original revision")
    a, c = attempts[0], view.get("compilation", {})
    require(set(c) == {"version", "snapshotHash", "manifestHash", "bootstrapHash"} and
            c["version"] == "workorch-original-prepared-bootstrap/v1" and
            c["snapshotHash"] == a["binding"]["snapshotHash"] and c["manifestHash"] == a["binding"]["manifestHash"] and
            re.fullmatch(r"prepared-bootstrap-sha256:[0-9a-f]{64}", c["bootstrapHash"]),
            "original full bootstrap compilation unavailable or substituted: " + str(c))
    require(preparation(view)["operationState"] == "blocked" and a["assessment"]["outcome"] == "blocked",
            "structural compilation promoted readiness")
    return copy.deepcopy(c)


def file_revalidation(record):
    attempt = record["StartupPreparation"]["Attempts"][-1]
    matches = [e for e in record.get("PreparationDirectoryEvidence", []) if e["ContentHash"] == attempt.get("DirectoryEvidenceHash")]
    require(len(matches) == 1, "missing or ambiguous original File archive")
    directory = matches[0]
    require(directory.get("RevalidationVersion") == "workorch-file-preparation-revalidation/v1", "original File revalidation was not attempted")
    return directory, directory.get("Revalidation")


def archived_file_stale(record, now, current_file_epoch):
    directory, observation = file_revalidation(record)
    if observation is None:
        return True
    attempt = record["StartupPreparation"]["Attempts"][-1]
    scope = observation["Scope"]
    require(observation["Version"] == directory["RevalidationVersion"] and
            scope["WorkspaceID"] == directory["WorkspaceID"] and scope["RequestID"] == attempt["RequestID"] == directory["RequestID"] and
            scope["PayloadHash"] == attempt["PayloadHash"] == directory["PayloadHash"] and
            scope["WorkspaceRevision"] == directory["ObservedWorkspaceRevision"] == attempt["ExpectedWorkspaceRevision"] + 1 and
            scope["InternalDirectory"] == directory["Internal"]["Path"] and scope["DeliverableDirectory"] == directory["Deliverable"]["Path"],
            "File observation lost exact original scope")
    resources = [e for e in record["PreparationResourceEvidence"] if e["ContentHash"] == attempt["ResourceEvidenceHash"]]
    configs = [e for e in record["PreparationManagedConfigEvidence"] if e["Ref"] == attempt["ManagedConfigEvidenceRef"]]
    require(len(resources) == len(configs) == 1, "File revalidation lost original full sources")
    require((scope["ResourceRefs"] or []) == [r["Observation"]["Ref"] for r in resources[0]["Resources"] or []] and
            (scope["Configs"] or []) == [{"Scope": r["Scope"], "Path": r["Path"]} for r in configs[0]["Files"] or []],
            "File revalidation replaced or omitted original targets")
    timestamp = lambda value: datetime.fromisoformat(value.replace("Z", "+00:00"))
    started, observed, expires = (timestamp(observation[key]) for key in ("StartedAt", "ObservedAt", "ExpiresAt"))
    require((expires - started).total_seconds() == 30 and started <= observed < expires and
            all(started >= timestamp(e["CompletedAt"]) for e in (directory, resources[0], configs[0])) and
            observed <= timestamp(attempt["CompletedAt"]),
            "File age window was extended or invalid")
    changed = directory["Internal"] != observation["Internal"] or directory["Deliverable"] != observation["Deliverable"] or \
        [r["Observation"] for r in resources[0]["Resources"] or []] != (observation["Resources"] or []) or \
        [r["Observation"] for r in configs[0]["Files"] or []] != (observation["Configs"] or [])
    return changed or now < observed or now >= expires or observation["OwnerEpoch"] != current_file_epoch


def archived_validity(record, now, current_file_epoch=None):
    """Original deadlines/raw images and process identity; never renew an observation."""
    attempt = record["StartupPreparation"]["Attempts"][-1]
    matches = [e for e in record["PreparationProviderCacheEvidence"] if e["Ref"] == attempt["ProviderCacheEvidenceRef"]]
    require(len(matches) == 1, "missing or ambiguous original cache observation")
    catalog = matches[0]["Facts"]["Observation"]["Catalog"]
    expires = catalog["ExpiresAt"]
    if expires == "0001-01-01T00:00:00Z":
        require(catalog["State"] == "unavailable" and catalog["ErrorCode"] == "cache_miss" and not catalog["Revision"],
                "missing deadline is not an exact cold cache miss")
        cache_stale = False  # Known absence is still blocked, never current.
    else:
        require(expires, "fixture lost original cache deadline")
        cache_stale = now >= datetime.fromisoformat(expires.replace("Z", "+00:00"))
    # Legacy verifier unit inputs have no File archive. The production fixture
    # separately requires the new version on every original explicit check.
    file_stale = archived_file_stale(record, now, current_file_epoch) if record.get("PreparationDirectoryEvidence") else False
    return "stale" if cache_stale or file_stale else "not_evaluated"


def validate_blocked(view, workspace_id, intent, admitted, expected_outstanding=("provider_cache",), previous_attempts=(), expected_validity="not_evaluated"):
    require(view["requestId"] == intent["requestId"], "lost original operation identity")
    state = preparation(view)
    require(expected_validity in ("not_evaluated", "stale") and state["workspaceId"] == workspace_id and state["operationState"] == "blocked" and
            state["currentValidity"] == expected_validity and not state.get("activeRunId"), "invented readiness or wrong scope")
    attempts = state["record"]["attempts"]
    require(len(attempts) == len(previous_attempts) + 1 and attempts[:-1] == list(previous_attempts), "changed history or duplicate preparation attempt")
    attempt = attempts[-1]
    require(attempt.get("executionInputVersion") == "workorch-preparation-execution-inputs/v1", "missing original execution inputs")
    require(attempt["requestId"] == intent["requestId"] and attempt["expectedWorkspaceRevision"] == intent["expectedWorkspaceRevision"] and
            attempt["committedWorkspaceRevision"] == state["workspaceRevision"] == intent["expectedWorkspaceRevision"] + 2 and
            attempt["state"] == "blocked" and attempt.get("completedAt"), "original CAS identity changed")
    binding = attempt["binding"]
    for field in ("inputHash", "derivationPermitId", "candidateHash", "startupInputHash"):
        require(binding[field] == admitted[field], "seal binding differs from confirmed source: " + field)
    require(binding["proposalHash"] == admitted["proposal"]["proposalHash"] and
            binding["specificationHash"] == admitted["proposal"]["specificationHash"], "reviewed source changed")
    checks = attempt["checks"]
    expected = dict(CHECK_OWNERS)
    # Resource ID is source-defined; there must be exactly one real required
    # resource in this fixture, not an arbitrary count supplied by the server.
    resources = [c["id"] for c in checks if c["id"].startswith("resource/")]
    require(len(resources) == 1, "missing or duplicate reviewed resource")
    expected[resources[0]] = "file"
    require(len(checks) == len(expected) and {c["id"]: c["owner"] for c in checks} == expected and
            len({c["id"] for c in checks}) == len(checks) and all(c["required"] for c in checks), "required checklist weakened")
    outstanding = []
    for check in checks:
        if check["id"] in expected_outstanding:
            require(check["state"] in ("blocked", "failed") and check["reason"], "known blocker granted credit")
            outstanding.append({k: check[k] for k in ("id", "owner", "state", "reason")})
        else:
            require(check["state"] == "passed" and check["reason"], "non-provider fixture did not close: " + check["id"])
    summary = attempt["assessment"]
    require(summary["version"] == "workorch-preparation-all-of-assessment/v1" and summary["manifestHash"] == binding["manifestHash"] and
            summary["outcome"] == "blocked" and summary["requiredCount"] == len(expected) and
            summary["passedRequiredCount"] == len(expected) - len(expected_outstanding) and summary["outstanding"] == outstanding,
            "all-of summary differs from exact checks")
    compiled = attempt["definitionCompilation"]
    require(compiled["compilerVersion"] == "workorch-preparation-definition-compiler/v1" and
            compiled["snapshotHash"] == binding["snapshotHash"] and compiled["manifestHash"] == binding["manifestHash"] and
            compiled["sourceInventoryHash"] and compiled["profileContractsHash"] and
            compiled["sourceCount"] == len(admitted["proposal"]["items"]) + 1 and
            compiled["requirementCount"] == len(admitted["proposal"]["requirements"]) and
            compiled["obligationCount"] == len(admitted["proposal"]["obligations"]) and
            compiled["verificationCount"] == len(admitted["proposal"]["verifications"]), "lost full sealed-source compilation")
    require(attempt["noRunEvaluationVersion"] == "workorch-preparation-no-run-final-fence/v1" and
            attempt["creationEpoch"] and attempt.get("preparationEpoch", 0) == 0, "missing original no-Run fence")
    for field in ("directoryEvidenceHash", "resourceEvidenceHash", "managedConfigEvidenceRef", "soulEvidenceRef", "sealEvidenceRef",
                  "skillRegistryEvidenceRef", "skillHubEffectRef", "providerCacheEvidenceRef", "runHistoryEvidenceRef",
                  "memoryInventoryEvidenceRef", "mcpRegistryEvidenceRef"):
        require(attempt.get(field), "missing real owner archive: " + field)
    return copy.deepcopy(state)


def validate_execution_inputs(record, soul_bindings):
    """Inspect full private source values, not only the public version marker.

    This fixture never changes Definition/configuration; exact current source
    equality here is additional diagnostic coverage, not a historical restore.
    The Go owner independently validates original ledger/config hash and CAS.
    """
    for attempt in record["StartupPreparation"]["Attempts"]:
        matches = [e for e in record["PreparationSealEvidence"] if e["Ref"] == attempt["SealEvidenceRef"]]
        require(len(matches) == 1, "ambiguous original execution source")
        inputs = matches[0].get("ExecutionInputs")
        require(inputs and inputs["Version"] == attempt["ExecutionInputVersion"] == "workorch-preparation-execution-inputs/v1",
                "execution input archive absent or wrong version")
        process = inputs["Process"]
        require(process["Version"] == inputs["Version"] and process["WorkspaceID"] == record["ID"] and
                process["WorkspaceRevision"] == attempt["ExpectedWorkspaceRevision"] + 1 and
                process["Before"] == process["After"], "execution process input drift")
        require(process["Before"]["MaxRuntimeRepairAttempts"] == 10 and process["Before"]["MaxDocumentRecoveryAttempts"] == 3,
                "fixture process limits changed")
        require(inputs["Template"] == record["DefinitionAuthoring"]["TemplateSnapshot"], "borrowed live template instead of confirmed source")
        source = inputs["Configuration"]
        for archived, owned in (("Directory", "DeliverableDirectory"), ("InternalDirectory", "InternalDirectory"),
                                ("TemplateID", "TemplateID"), ("ProfileID", "ProfileID"), ("ContextHints", "ContextHints"),
                                ("EvidencePolicies", "EvidencePolicies"), ("DefaultLLMSelection", "DefaultLLMSelection")):
            require(source[archived] == record.get(owned), "lost original execution configuration: " + archived)
        require(source["DefaultSoulID"] == soul_bindings.get("defaultSoulId", "") and
                source["SoulBindings"] == soul_bindings.get("bindings"), "lost real sidecar configuration source")


def file_snapshot(directory):
    return {str(p.relative_to(directory)): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(directory.rglob("*")) if p.is_file()}


def skillhub_calls(root):
    path = root / "skillhub-calls.jsonl"
    return [json.loads(line) for line in path.read_bytes().splitlines()] if path.exists() else []


def browser_status_inventory_pairs(before, after, root, project):
    """Require only the explicit STEP04 pair during preparation navigation.

    Do not subtract arbitrary effects: preserve the prefix and require every new
    command, order and directory. Ordinary navigation must not run inventory.
    """
    require(after[:len(before)] == before, "browser changed original status history")
    calls = after[len(before):]
    require(len(calls) % 2 == 0, "incomplete browser status pair")
    preparation_pairs, inventory_pairs = 0, 0
    for index in range(0, len(calls), 2):
        global_call, project_call = calls[index:index + 2]
        require(global_call["cwd"] == str(root) and project_call == {"args": ["status", "--json"], "cwd": str(project)},
                "browser status escaped scope or command order")
        if global_call["args"] == ["status", "--json", "--global"]:
            preparation_pairs += 1
        else:
            require(global_call["args"] == ["status", "--global", "--json", "--agent", "codex"], "unknown browser status command")
            inventory_pairs += 1
    require(preparation_pairs == 1, "browser repeated or omitted preparation status effect")
    require(inventory_pairs == 0, "preparation navigation implicitly queried SkillHub inventory")
    return inventory_pairs


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--port", type=int, help="unused loopback port; checked before launching, never replaces a service")
    parser.add_argument("--browser-hold-seconds", type=int, default=0, help="bounded window for explicit first check in production Web; marker is not browser proof")
    args = parser.parse_args()
    require(0 <= args.browser_hold_seconds <= 1800, "browser window must be bounded")
    root = args.evidence_dir.absolute()
    require(not root.exists() and not root.is_relative_to(Path("/tmp")), "new durable evidence directory required")
    root.mkdir(parents=True, mode=0o700)
    os.umask(0o077)
    source = Path(__file__).resolve().parent.parent
    verification, fixture = None, None
    result = {"pass": False, "mode": "isolated-loopback-fixtures", "realModelCalls": 0, "browserVerified": False,
              "ready": False, "M3Issued": False}
    try:
        shim_dir, installed = root / "fixture-bin", root / "installed-skills"
        shim_dir.mkdir()
        installed.mkdir()
        shim = shim_dir / "skill-hub"
        shutil.copyfile(source / "scripts/fixtures/step04-skill-hub.py", shim)
        shim.chmod(0o700)
        environment = {**os.environ, "PATH": str(shim_dir) + os.pathsep + os.environ["PATH"],
                       "CODEX_SKILLS_DIR": str(installed), "WORKORCH_STEP04_FIXTURE_ROOT": str(root)}
        binary, identity = step01.build_candidate(source, root)
        result["buildIdentity"] = identity
        verification = step01.Verification(binary, root, identity, process_environment=environment, port=args.port)
        fixture = Fixture(root)
        config = {"server": {"addr": f"127.0.0.1:{verification.port}"}, "repository": {"backend": "file", "dataDir": str(verification.data)},
                  "llm": {"aiProxy": {"baseURL": f"http://127.0.0.1:{fixture.server.server_port}", "apiKey": "", "timeout": "30s",
                                      "modelOverrides": {"step02-fixture": {"contextWindowTokens": 128000, "maxOutputTokens": 16384, "operations": ["responses"]}}},
                          "defaultChat": {"providerID": "ai-proxy", "modelID": "step02-fixture"},
                          "budget": {"contextUtilizationRatio": 0.75, "estimatedTokensPerByte": 0.25}}}
        (verification.config / "application.toml").write_text(step02.toml_document(config), encoding="utf-8")
        verification.start()
        verification.request("POST", "/api/v1/workorch/souls", {"id": "step04-soul", "name": "STEP04 fixture Soul", "soulMd": "保留原始输入与 required all_of。\n",
                                                               "allowedSkillRefs": ["not-selected-by-this-bounded-profile"]}, 201)
        verification.request("POST", "/api/v1/workorch/workspace-templates", TEMPLATE, 201)
        project = root / "project"
        project.mkdir()
        (project / "requirements.txt").write_bytes(RESOURCE)
        payload = {"requestId": "step04-process-original", "title": "STEP04 独立进程阻塞验收", "templateId": TEMPLATE["id"], "deliverableDirectory": str(project),
                   "input": {"goalText": step02.GOAL, "acceptanceRequirements": step02.REQUIREMENTS, "resourceRefs": ["requirements.txt"]}}
        created = verification.create(payload)["authoring"]
        workspace_id = created["workspaceId"]
        authoring_path = f"/api/v1/workorch/workspaces/{workspace_id}/authoring"
        verification.request("POST", authoring_path + "/compile", {"expectedInputHash": created["inputHash"], "expectedTemplateSnapshotHash": created["templateSnapshotHash"]}, 202)
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            view = verification.read(workspace_id)
            if step02.compilation_settled(view):
                break
            time.sleep(0.2)
        step02.validate_candidate(view, payload)
        exact = step03.confirmation(view)
        admitted = verification.request("POST", authoring_path + "/confirm", exact)["authoring"]
        require(admitted["state"] == "admitted" and admitted["activationState"] == "eligible", "fixture did not seal exact input")
        workspace_file = verification.data / "internal/workspaces" / workspace_id / "workspace.json"
        current_file_epoch = None

        def capture_original_file_epoch(original):
            # Only a new explicit check executed in this still-running process
            # can establish its epoch. A restart/read/replay never borrows the
            # historical epoch as evidence of the new process's identity.
            nonlocal current_file_epoch
            owned = step02.workspace_record(json.loads(workspace_file.read_bytes()), workspace_id)
            attempt = owned["StartupPreparation"]["Attempts"][-1]
            require(attempt["RequestID"] == original["requestId"] and attempt["ExpectedWorkspaceRevision"] == original["expectedWorkspaceRevision"],
                    "current File epoch did not come from new original check")
            _, observation = file_revalidation(owned)
            require(observation is not None, "new original check lost File observation")
            current_file_epoch = observation["OwnerEpoch"]

        def current_validity():
            owned = step02.workspace_record(json.loads(workspace_file.read_bytes()), workspace_id)
            return archived_validity(owned, datetime.now(timezone.utc), current_file_epoch)

        def unchanged_history(observed, saved):
            expected = copy.deepcopy(saved)
            expected["currentValidity"] = current_validity()
            return preparation(observed) == expected

        record = step02.workspace_record(json.loads(workspace_file.read_bytes()), workspace_id)
        require(len(record["DefinitionAdmissionCandidates"]) == 1, "unexpected candidate count")
        step03.validate_sealed_activation(admitted, exact, record["DefinitionAdmissionCandidates"][0], record["StartupInput"], record["ActivationAdmission"])
        internal = Path(record["InternalDirectory"])
        require(internal.is_relative_to(verification.data), "fixture escaped isolated repository")
        config_paths = [internal / "mcp/config.json", verification.data / "system/managed-configs/mcp/config.json"]
        for path in config_paths:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(MANAGED)
        (project / ".agents/skills").mkdir(parents=True)
        resources_before = file_snapshot(project)
        path = f"/api/v1/workorch/workspaces/{workspace_id}/activation-preparation"
        provider_before = copy.deepcopy(fixture.http_paths)
        status_baseline = skillhub_calls(root)
        status_baseline_count = len(status_baseline)
        require(len(fixture.calls) == 8, "unexpected fixture authoring call count")
        not_checked = preparation(verification.request("GET", path))
        require(not_checked["operationState"] == "not_checked" and not not_checked.get("record"), "GET ran a check")
        require(skillhub_calls(root) == status_baseline, "not_checked GET invoked SkillHub")
        intent = {"requestId": "step04-check-original", "expectedWorkspaceRevision": admitted["workspaceRevision"]}
        if args.browser_hold_seconds:
            session = {"baseURL": verification.base, "workspaceId": workspace_id, "preparationPath": path,
                       "expectedWorkspaceRevision": admitted["workspaceRevision"], "pid": verification.process.pid,
                       "instruction": "Open preparation, explicitly run exactly one check, inspect blocked result; close/reopen/refresh must not append attempts. No execution intent. Save actual DOM/screenshots, then navigate the isolated test tab to about:blank before browser-done so whole-page reconnects cannot contaminate the following process-only restart checks."}
            save(root, "browser-session.json", session)
            print(json.dumps({"browserURL": verification.base + "/console/", **session}), flush=True)
            deadline = time.monotonic() + args.browser_hold_seconds
            while time.monotonic() < deadline and not (root / "browser-done").exists():
                time.sleep(1)
            require((root / "browser-done").exists(), "browser window unfinished; do not claim pass")
            observed = verification.request("GET", path)
            attempts = preparation(observed).get("record", {}).get("attempts", [])
            require(len(attempts) == 1 and attempts[0]["expectedWorkspaceRevision"] == intent["expectedWorkspaceRevision"],
                    "Web did not perform exactly one original check")
            intent["requestId"] = attempts[0]["requestId"]
            first = {"requestId": intent["requestId"], "preparation": preparation(observed)}
            result["browserWindowCompleted"] = True  # Separate DOM/screenshots are still mandatory proof.
            inventory_pairs = browser_status_inventory_pairs(status_baseline, skillhub_calls(root), root, project)
            result["browserPageInventoryStatusPairs"] = inventory_pairs
            # Keep the entire raw log; no implicit inventory effects are allowed.
            status_baseline_count += inventory_pairs * 2
        else:
            first = verification.request("POST", path + "/check", intent)
        capture_original_file_epoch(intent)
        saved = validate_blocked(first, workspace_id, intent, admitted, expected_validity=current_validity())
        if args.browser_hold_seconds:
            first = verification.request("POST", path + "/recover", intent)
        compilations = {intent["requestId"]: validate_bootstrap_compilation(first, intent)}
        save(root, "original-preparation.json", first)
        owner_before = workspace_file.read_bytes()
        validate_execution_inputs(step02.workspace_record(json.loads(owner_before), workspace_id), json.loads((workspace_file.parent / "config/soul_bindings.json").read_bytes()))
        require(len(skillhub_calls(root)) == status_baseline_count + 2, "original check did not execute exactly two status observations")
        status_before = (root / "skillhub-calls.jsonl").read_bytes()
        require(fixture.http_paths == provider_before, "STEP04 refreshed catalog or called provider")
        for action in ("", "/check", "/recover"):
            observed = verification.request("POST" if action else "GET", path + action, intent if action else None)
            require(unchanged_history(observed, saved), "original GET/replay/recovery changed history or deadline projection")
            require(validate_bootstrap_compilation(observed, intent) == compilations[intent["requestId"]] if action else "compilation" not in observed,
                    "GET compiled or original replay changed compilation")
        verification.request("POST", path + "/check", {**intent, "expectedWorkspaceRevision": intent["expectedWorkspaceRevision"] + 1}, 409)
        require(workspace_file.read_bytes() == owner_before, "rejected/replayed request wrote owner state")
        # Preserve all original files. Their absence after restart must not make
        # a history read/recovery a new observation or upgrade currentValidity.
        config_paths[0].rename(config_paths[0].with_suffix(".preserved"))
        verification.stop()
        first_pid = verification.pids[-1]
        verification.start()
        current_file_epoch = None
        require(verification.pids[-1] != first_pid, "not an independent OS process restart")
        for action in ("", "/check", "/recover"):
            observed = verification.request("POST" if action else "GET", path + action, intent if action else None)
            require(unchanged_history(observed, saved), "restart/replay/recovery lost original facts or deadline projection")
            require(validate_bootstrap_compilation(observed, intent) == compilations[intent["requestId"]] if action else "compilation" not in observed,
                    "restart borrowed new compiler inputs or GET compiled")
        require(workspace_file.read_bytes() == owner_before, "restart or recovery rewrote immutable attempt")
        require((root / "skillhub-calls.jsonl").read_bytes() == status_before, "GET/replay/recovery/restart reran status")
        require(fixture.http_paths == provider_before, "GET/replay/recovery/restart called provider")
        # A new explicit check, not replay/recovery, observes the missing file.
        # It may append history but may not replace the previous judgments.
        missing_intent = {"requestId": "step04-check-missing-config", "expectedWorkspaceRevision": saved["workspaceRevision"]}
        missing = verification.request("POST", path + "/check", missing_intent)
        capture_original_file_epoch(missing_intent)
        compilations[missing_intent["requestId"]] = validate_bootstrap_compilation(missing, missing_intent)
        missing_saved = validate_blocked(missing, workspace_id, missing_intent, admitted,
                                        ("managed_configuration", "provider_cache"), saved["record"]["attempts"], current_validity())
        save(root, "missing-config-preparation.json", missing)
        require(not config_paths[0].exists(), "check repaired resource without authority")
        config_paths[0].with_suffix(".preserved").rename(config_paths[0])
        repaired_intent = {"requestId": "step04-check-manually-restored", "expectedWorkspaceRevision": missing_saved["workspaceRevision"]}
        # Drop only the HTTP body, not the actual owner receipt. Read and
        # reconcile this exact identity; never switch IDs on transport loss.
        connection = http.client.HTTPConnection("127.0.0.1", verification.port, timeout=15)
        raw = json.dumps(repaired_intent).encode()
        (root / "lost-response-request.bin").write_bytes(raw)
        connection.request("POST", path + "/check", raw, {"Content-Type": "application/json"})
        response = connection.getresponse()
        require(response.status == 200, "lost-response setup did not settle original check")
        save(root, "lost-response-headers.json", {"status": response.status, "headers": list(response.getheaders()), "bodyRead": False})
        response.close()
        connection.close()
        capture_original_file_epoch(repaired_intent)
        reconciled = verification.request("GET", path)
        repaired = {"requestId": repaired_intent["requestId"], "preparation": preparation(reconciled)}
        repaired_saved = validate_blocked(repaired, workspace_id, repaired_intent, admitted,
                                         previous_attempts=missing_saved["record"]["attempts"], expected_validity=current_validity())
        restored_compilation = verification.request("POST", path + "/recover", repaired_intent)
        compilations[repaired_intent["requestId"]] = validate_bootstrap_compilation(restored_compilation, repaired_intent)
        save(root, "manually-restored-preparation.json", repaired)
        repaired_owner = workspace_file.read_bytes()
        validate_execution_inputs(step02.workspace_record(json.loads(repaired_owner), workspace_id), json.loads((workspace_file.parent / "config/soul_bindings.json").read_bytes()))
        repaired_status = (root / "skillhub-calls.jsonl").read_bytes()
        require(len(skillhub_calls(root)) == status_baseline_count + 6, "explicit checks did not execute exactly one status pair each")
        verification.stop()
        verification.start()
        current_file_epoch = None
        for original in (intent, missing_intent, repaired_intent):
            for action in ("/check", "/recover"):
                observed = verification.request("POST", path + action, original)
                require(validate_bootstrap_compilation(observed, original) == compilations[original["requestId"]],
                        "restart did not reproduce complete original bootstrap")
                require(observed["requestId"] == original["requestId"] and unchanged_history(observed, repaired_saved),
                        "multi-attempt recovery selected or rewrote a different operation")
        require(unchanged_history(verification.request("GET", path), repaired_saved) and workspace_file.read_bytes() == repaired_owner,
                "lost-response restart/recovery changed durable history")
        require((root / "skillhub-calls.jsonl").read_bytes() == repaired_status and fixture.http_paths == provider_before,
                "completed multi-attempt recovery repeated owner effects or provider I/O")
        require(file_snapshot(project) == resources_before, "STEP04 changed business files")
        require(verification.request("GET", "/api/v1/workorch/runs")["runs"] == [] and
                verification.request("GET", f"/api/v1/workorch/workspaces/{workspace_id}/runs")["runs"] == [], "STEP04 created Run")
        build_identity.verify(source, source / "web/consoleassets/dist", binary, root / "build-identity.json", {k: identity[k] for k in build_identity.IDENTITY_FIELDS}, root)
        (root / "original-owner-record.bin").write_bytes(owner_before)
        (root / "final-owner-record.bin").write_bytes(repaired_owner)
        result.update({"pass": True, "workspaceId": workspace_id, "request": intent, "restartReadbackPassed": True,
                       "originalBootstrapCompilations": compilations, "bootstrapCompilationRestartPassed": True,
                       "manualResourceRepairRecheckPassed": True, "lostResponseReconciliationPassed": True, "attemptCount": 3,
                       "providerPreparationCalls": 8, "step04ProviderHTTPRequests": 0, "embeddingCalls": 0, "runCount": 0,
                       "requiredCount": saved["record"]["attempts"][0]["assessment"]["requiredCount"],
                       "passedRequiredCount": saved["record"]["attempts"][0]["assessment"]["passedRequiredCount"],
                       "outstanding": ["provider_cache"]})
    finally:
        if verification:
            verification.stop()
            save(root, "http-trace.json", verification.trace)
            result["pids"] = verification.pids
        if fixture:
            fixture.close()
            save(root, "provider-http-paths.json", fixture.http_paths)
        save(root, "result.json", result)
    print(json.dumps(result, ensure_ascii=False), flush=True)


if __name__ == "__main__":
    main()
