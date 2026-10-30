#!/usr/bin/env python3
"""STEP04 verifier rejection tests; no service, model, network or M3."""
import copy
import base64
import hashlib
from datetime import datetime, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("step04", Path(__file__).with_name("verify-workspace-preparation-step04.py"))
step04 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step04)


class PreparationVerifierTest(unittest.TestCase):
    def test_bounded_fixture_keeps_default_template_consumer_checks_separate(self):
        fixture = object.__new__(step04.Fixture)
        payload = {"candidateAcceptance": [{"CandidateItemRef": "derived-dod:user:000001"}], "sourceApplicableVerificationDeclarations": [
            {"candidateItemRef": "derived-dod:user:000001", "sourceScope": "dodItem", "sourceId": "source-1",
             "declarationBindingMode": "one_applicable_reference", "mergedRequiredEvidenceKinds": ["validation"], "mergedRequiredConsumers": [],
             "declarations": [{"profileId": "bounded", "evidenceRequirementId": "*", "scope": "dodItem"}]}]}
        fixture.validate_source_consumers(payload)
        # The default STEP02 template must still reject missing terminal sources.
        with self.assertRaisesRegex(RuntimeError, "template consumer source missing"):
            step04.step02.validate_source_consumers(payload)
        payload["sourceApplicableVerificationDeclarations"][0]["mergedRequiredConsumers"] = [{"Kind": "terminal_owner_snapshot"}]
        with self.assertRaisesRegex(RuntimeError, "undeclared terminal consumers"):
            fixture.validate_source_consumers(payload)
        with self.assertRaisesRegex(RuntimeError, "lost ordinary declarations"):
            fixture.validate_source_consumers({"candidateAcceptance": [], "sourceApplicableVerificationDeclarations": []})

    def test_file_window_uses_complete_original_images_and_current_process_epoch(self):
        epoch = "file-preparation-epoch:" + "A" * 26
        started, observed, expires = "2026-10-27T00:00:00Z", "2026-10-27T00:00:01Z", "2026-10-27T00:00:30Z"
        attempt = {"RequestID": "original", "PayloadHash": "payload", "ExpectedWorkspaceRevision": 5, "DirectoryEvidenceHash": "directory", "ResourceEvidenceHash": "resources", "ManagedConfigEvidenceRef": "configs", "CompletedAt": observed, "ProviderCacheEvidenceRef": "provider"}
        directory = {"ContentHash": "directory", "WorkspaceID": "ws", "RequestID": "original", "PayloadHash": "payload", "ObservedWorkspaceRevision": 6,
                     "Internal": {"Path": "/internal"}, "Deliverable": {"Path": "/delivery"}, "CompletedAt": started,
                     "RevalidationVersion": "workorch-file-preparation-revalidation/v1"}
        observation = {"Version": directory["RevalidationVersion"], "OwnerEpoch": epoch, "StartedAt": started, "ObservedAt": observed, "ExpiresAt": expires,
                       "Scope": {"WorkspaceID": "ws", "RequestID": "original", "PayloadHash": "payload", "WorkspaceRevision": 6, "InternalDirectory": "/internal", "DeliverableDirectory": "/delivery", "ResourceRefs": ["raw.txt"], "Configs": None},
                       "Internal": directory["Internal"], "Deliverable": directory["Deliverable"], "Resources": [{"Ref": "raw.txt", "Content": "AP8NCg=="}], "Configs": None}
        directory["Revalidation"] = copy.deepcopy(observation)
        record = {"StartupPreparation": {"Attempts": [attempt]}, "PreparationDirectoryEvidence": [directory],
                  "PreparationResourceEvidence": [{"ContentHash": "resources", "Resources": [{"Observation": copy.deepcopy(observation["Resources"][0])}], "CompletedAt": started}],
                  "PreparationManagedConfigEvidence": [{"Ref": "configs", "Files": None, "CompletedAt": started}],
                  "PreparationProviderCacheEvidence": [{"Ref": "provider", "Facts": {"Observation": {"Catalog": {"ExpiresAt": "0001-01-01T00:00:00Z", "State": "unavailable", "ErrorCode": "cache_miss", "Revision": ""}}}}]}
        before = copy.deepcopy(record)
        now = datetime(2026, 10, 27, 0, 0, 2, tzinfo=timezone.utc)
        self.assertEqual(step04.archived_validity(record, now, epoch), "not_evaluated")
        self.assertEqual(step04.archived_validity(record, now, None), "stale")
        self.assertEqual(step04.archived_validity(record, now, "file-preparation-epoch:" + "B" * 26), "stale")
        self.assertEqual(step04.archived_validity(record, datetime(2026, 10, 27, 0, 0, 30, tzinfo=timezone.utc), epoch), "stale")
        self.assertEqual(record, before)
        changed = copy.deepcopy(record)
        changed["PreparationDirectoryEvidence"][0]["Revalidation"]["Resources"][0]["Content"] = "changed"
        self.assertEqual(step04.archived_validity(changed, now, epoch), "stale")
        for mutate in (
            lambda r: r["PreparationDirectoryEvidence"][0]["Revalidation"]["Scope"].update(RequestID="other"),
            lambda r: r["PreparationDirectoryEvidence"][0]["Revalidation"]["Scope"].update(ResourceRefs=[]),
            lambda r: r["PreparationDirectoryEvidence"][0]["Revalidation"].update(ExpiresAt="2026-10-27T00:01:00Z"),
            lambda r: r["PreparationDirectoryEvidence"][0]["Revalidation"].update(ObservedAt=expires),
            lambda r: r["PreparationDirectoryEvidence"].append(copy.deepcopy(r["PreparationDirectoryEvidence"][0])),
        ):
            changed = copy.deepcopy(record); mutate(changed)
            with self.assertRaises(RuntimeError):
                step04.archived_validity(changed, now, epoch)

    def test_full_bootstrap_summary_rejects_substitution_and_ready_claims(self):
        view, _, intent, _ = self.fixture()
        view["compilation"] = {"version": "workorch-original-prepared-bootstrap/v1", "snapshotHash": "snapshot", "manifestHash": "manifest",
                               "bootstrapHash": "prepared-bootstrap-sha256:" + "a" * 64}
        expected = copy.deepcopy(view["compilation"])
        self.assertEqual(step04.validate_bootstrap_compilation(view, intent), expected)
        self.assertEqual(view["compilation"], expected)
        for change in (lambda v: v.pop("compilation"), lambda v: v["compilation"].update(snapshotHash="other"),
                       lambda v: v["compilation"].update(manifestHash="other"), lambda v: v["compilation"].update(ready=True),
                       lambda v: v["compilation"].update(bootstrapHash="prepared-bootstrap-sha256:" + "z" * 64),
                       lambda v: v["compilation"].update(failureCode="provider_declaration_unavailable"),
                       lambda v: v.update(requestId="other"), lambda v: v["preparation"].update(operationState="ready")):
            bad = copy.deepcopy(view)
            change(bad)
            with self.assertRaises(RuntimeError):
                step04.validate_bootstrap_compilation(bad, intent)

    def fixture(self):
        intent = {"requestId": "original", "expectedWorkspaceRevision": 5}
        admitted = {"inputHash": "input", "derivationPermitId": "derive", "candidateHash": "candidate", "startupInputHash": "startup",
                    "proposal": {"proposalHash": "proposal", "specificationHash": "specification", "items": [1, 2], "requirements": [1, 2, 3], "obligations": [1, 2, 3], "verifications": [1, 2, 3]}}
        binding = {key: admitted[key] for key in ("inputHash", "derivationPermitId", "candidateHash", "startupInputHash")}
        binding.update(proposalHash="proposal", specificationHash="specification", manifestHash="manifest", snapshotHash="snapshot")
        checks = [{"id": key, "owner": owner, "required": True, "state": "passed", "reason": "provider_configuration_observed_remaining_checks" if key == "provider_cache" else "actual_observation"}
                  for key, owner in sorted({**step04.CHECK_OWNERS, "resource/exact-original": "file"}.items())]
        attempt = {**intent, "committedWorkspaceRevision": 7, "state": "blocked", "completedAt": "2026-10-27T00:00:00Z", "binding": binding, "checks": checks,
                   "assessment": {"version": "workorch-preparation-all-of-assessment/v1", "manifestHash": "manifest", "outcome": "checks_passed", "requiredCount": 11, "passedRequiredCount": 11,
                                  "outstanding": [{k: c[k] for k in ("id", "owner", "state", "reason")} for c in checks if c["state"] != "passed"]},
                   "definitionCompilation": {"compilerVersion": "workorch-preparation-definition-compiler/v1", "snapshotHash": "snapshot", "manifestHash": "manifest", "sourceInventoryHash": "inventory", "profileContractsHash": "profile", "sourceCount": 3, "requirementCount": 3, "obligationCount": 3, "verificationCount": 3},
                   "noRunEvaluationVersion": "workorch-preparation-no-run-final-fence/v1", "creationEpoch": "creation", "preparationEpoch": 0}
        attempt["executionInputVersion"] = "workorch-preparation-execution-inputs/v1"
        for field in ("directoryEvidenceHash", "resourceEvidenceHash", "managedConfigEvidenceRef", "soulEvidenceRef", "sealEvidenceRef", "skillRegistryEvidenceRef", "skillHubEffectRef", "providerCacheEvidenceRef", "runHistoryEvidenceRef", "memoryInventoryEvidenceRef", "mcpRegistryEvidenceRef"):
            attempt[field] = "original-" + field
        view = {"requestId": "original", "preparation": {"workspaceId": "ws", "workspaceRevision": 7, "operationState": "blocked", "currentValidity": "not_evaluated", "record": {"attempts": [attempt]}}}
        return view, "ws", intent, admitted

    def test_exact_bounded_blocked_history_not_readiness(self):
        data = self.fixture()
        result = step04.validate_blocked(*data)
        self.assertEqual(result, data[0]["preparation"])
        result["record"]["attempts"][0]["binding"]["manifestHash"] = "caller"
        self.assertEqual(data[0]["preparation"]["record"]["attempts"][0]["binding"]["manifestHash"], "manifest")

    def test_read_validity_uses_exact_archived_deadline_without_rewriting_history(self):
        record = {"StartupPreparation": {"Attempts": [{"ProviderCacheEvidenceRef": "old"}]},
                  "PreparationProviderCacheEvidence": [{"Ref": "old", "Facts": {"Observation": {"Catalog": {"ExpiresAt": "2026-10-27T10:26:11.477474208+08:00"}}}}]}
        original = copy.deepcopy(record)
        self.assertEqual(step04.archived_validity(record, datetime(2026, 10, 27, 2, 26, 11, tzinfo=timezone.utc)), "not_evaluated")
        self.assertEqual(step04.archived_validity(record, datetime(2026, 10, 27, 2, 26, 12, tzinfo=timezone.utc)), "stale")
        self.assertEqual(record, original)
        cold = copy.deepcopy(record)
        cold["PreparationProviderCacheEvidence"][0]["Facts"]["Observation"]["Catalog"] = {
            "ExpiresAt": "0001-01-01T00:00:00Z", "State": "unavailable", "ErrorCode": "cache_miss", "Revision": ""}
        self.assertEqual(step04.archived_validity(cold, datetime.now(timezone.utc)), "not_evaluated")
        cold["PreparationProviderCacheEvidence"][0]["Facts"]["Observation"]["Catalog"]["State"] = "fresh"
        with self.assertRaises(RuntimeError):
            step04.archived_validity(cold, datetime.now(timezone.utc))
        for mutation in (lambda r: r["PreparationProviderCacheEvidence"].clear(),
                         lambda r: r["PreparationProviderCacheEvidence"].append(copy.deepcopy(r["PreparationProviderCacheEvidence"][0])),
                         lambda r: r["PreparationProviderCacheEvidence"][0].update(Ref="different-operation")):
            changed = copy.deepcopy(record)
            mutation(changed)
            with self.assertRaises(RuntimeError):
                step04.archived_validity(changed, datetime.now(timezone.utc))
        data = self.fixture()
        data[0]["preparation"]["currentValidity"] = "stale"
        with self.assertRaises(RuntimeError):
            step04.validate_blocked(*data)
        step04.validate_blocked(*data, expected_validity="stale")
        data[0]["preparation"]["currentValidity"] = "current"
        with self.assertRaises(RuntimeError):
            step04.validate_blocked(*data, expected_validity="stale")

    def test_rejects_weakened_required_checks_and_fabricated_summary(self):
        mutations = {
            "missing": lambda a: a["checks"].pop(),
            "duplicate": lambda a: a["checks"].append(copy.deepcopy(a["checks"][0])),
            "owner": lambda a: a["checks"][0].update(owner="application"),
            "optional": lambda a: a["checks"][0].update(required=False),
            "non-provider blocker": lambda a: a["checks"][0].update(state="blocked"),
            "unconsumed provider": lambda a: next(c for c in a["checks"] if c["id"] == "provider_cache").update(reason="provider_declaration_only"),
            "summary count": lambda a: a["assessment"].update(passedRequiredCount=10),
            "summary fabricates blocker": lambda a: a["assessment"].update(outstanding=[{"id": "provider_cache"}]),
            "summary manifest": lambda a: a["assessment"].update(manifestHash="other"),
            "summary outcome": lambda a: a["assessment"].update(outcome="blocked"),
            "execution input version": lambda a: a.update(executionInputVersion="other"),
            "missing execution inputs": lambda a: a.pop("executionInputVersion"),
        }
        for label, change in mutations.items():
            with self.subTest(label=label), self.assertRaises(RuntimeError):
                data = self.fixture()
                change(data[0]["preparation"]["record"]["attempts"][0])
                step04.validate_blocked(*data)

    def test_rejects_identity_scope_m3_or_currentness_substitution(self):
        for field, value in (("workspaceId", "other"), ("workspaceRevision", 8), ("operationState", "ready"), ("currentValidity", "current"), ("activeRunId", "run")):
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                data = self.fixture()
                data[0]["preparation"][field] = value
                step04.validate_blocked(*data)
        for field, value in (("requestId", "new"), ("expectedWorkspaceRevision", 6), ("committedWorkspaceRevision", 8), ("state", "accepted"), ("completedAt", ""), ("creationEpoch", ""), ("preparationEpoch", 1), ("noRunEvaluationVersion", ""), ("sealEvidenceRef", "")):
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                data = self.fixture()
                data[0]["preparation"]["record"]["attempts"][0][field] = value
                step04.validate_blocked(*data)

    def test_rejects_compile_source_substitution(self):
        for field in ("compilerVersion", "snapshotHash", "manifestHash", "sourceInventoryHash", "profileContractsHash", "sourceCount", "requirementCount", "obligationCount", "verificationCount"):
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                data = self.fixture()
                compiled = data[0]["preparation"]["record"]["attempts"][0]["definitionCompilation"]
                compiled[field] = "" if isinstance(compiled[field], str) else 99
                step04.validate_blocked(*data)

    def test_append_preserves_complete_old_attempts(self):
        data = self.fixture()
        original = copy.deepcopy(data[0]["preparation"]["record"]["attempts"][0])
        data[0]["preparation"]["record"]["attempts"].insert(0, copy.deepcopy(original))
        step04.validate_blocked(*data, previous_attempts=[original])
        data[0]["preparation"]["record"]["attempts"][0]["binding"]["snapshotHash"] = "rewritten"
        with self.assertRaises(RuntimeError):
            step04.validate_blocked(*data, previous_attempts=[original])
    def test_rejects_binding_source_substitution(self):
        for field in ("inputHash", "derivationPermitId", "candidateHash", "startupInputHash", "proposalHash", "specificationHash"):
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                data = self.fixture()
                data[0]["preparation"]["record"]["attempts"][0]["binding"][field] = "other"
                step04.validate_blocked(*data)

    def test_cli_cannot_accept_existing_binary_live_config_or_model_opt_in(self):
        for argument in ("--binary", "--config-dir", "--allow-live"):
            completed = subprocess.run([sys.executable, str(Path(step04.__file__)), "--evidence-dir", "/unused", argument], capture_output=True)
            self.assertEqual(completed.returncode, 2)
            self.assertIn(b"unrecognized arguments", completed.stderr)

    def test_browser_window_is_bounded_before_evidence_or_service_creation(self):
        for value in ("-1", "1801"):
            completed = subprocess.run([sys.executable, str(Path(step04.__file__)), "--evidence-dir", "/unused", "--browser-hold-seconds", value], capture_output=True)
            self.assertNotEqual(completed.returncode, 0)
            self.assertIn(b"browser window must be bounded", completed.stderr)

    def test_explicit_listener_port_rejects_invalid_values_before_binding(self):
        for port in (True, False, 0, -1, 65536, "56147", 1.5):
            with self.subTest(port=port), self.assertRaises(RuntimeError):
                step04.step01.Verification(Path("/unused/binary"), Path("/unused/root"), {}, port=port)

    def test_browser_navigation_only_allows_the_explicit_preparation_effect_pair(self):
        root, project = Path("/isolated"), Path("/isolated/project")
        global_inventory = {"args": ["status", "--global", "--json", "--agent", "codex"], "cwd": str(root)}
        global_preparation = {"args": ["status", "--json", "--global"], "cwd": str(root)}
        local = {"args": ["status", "--json"], "cwd": str(project)}
        before = [global_inventory, local]
        after = before + [global_preparation, local]
        self.assertEqual(step04.browser_status_inventory_pairs(before, after, root, project), 0)
        for changed in (after + [global_preparation, local], after[:-1],
                        before + [global_inventory, local, global_preparation, local],
                        after + [global_inventory, local],
                        before + [global_inventory, local], list(reversed(after)),
                        before + [{"args": ["sync"], "cwd": str(root)}, local, global_preparation, local]):
            with self.assertRaises(RuntimeError):
                step04.browser_status_inventory_pairs(before, changed, root, project)

    def test_status_fixture_is_scoped_and_refuses_mutation(self):
        script = Path(__file__).parent / "fixtures/step04-skill-hub.py"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home = root / "hub"
            home.mkdir()
            environment = {**os.environ, "WORKORCH_STEP04_FIXTURE_ROOT": directory, "SKILL_HUB_HOME": str(home)}
            registered = subprocess.run([sys.executable, str(script.resolve()), "project", "register", str(root / "project"), "--request-id", "original", "--json"],
                                        cwd=root, env=environment, capture_output=True)
            self.assertNotEqual(registered.returncode, 0)  # directory is absent
            project = root / "project"
            project.mkdir()
            registered = subprocess.run([sys.executable, str(script.resolve()), "project", "register", str(project), "--request-id", "original", "--json"],
                                        cwd=root, env=environment, capture_output=True)
            self.assertEqual(registered.returncode, 0, registered.stderr)
            for args in (["status", "--json"], ["status", "--json", "--global"]):
                completed = subprocess.run([sys.executable, str(script.resolve()), *args], cwd=project, env=environment, capture_output=True)
                self.assertEqual(completed.returncode, 0, completed.stderr)
                self.assertEqual(json.loads(completed.stdout)["items"], [])
            before = (root / "skillhub-calls.jsonl").read_bytes()
            for args in (["sync"], ["use", "x"], ["status", "--repair"]):
                completed = subprocess.run([sys.executable, str(script.resolve()), *args], cwd=root, env=environment, capture_output=True)
                self.assertNotEqual(completed.returncode, 0)
            self.assertEqual((root / "skillhub-calls.jsonl").read_bytes(), before)
            outside = subprocess.run([sys.executable, str(script.resolve()), "status", "--json"], cwd=root.parent, env=environment, capture_output=True)
            self.assertNotEqual(outside.returncode, 0)

    def test_registration_fixture_retains_original_reply_and_queries_without_registering(self):
        script = (Path(__file__).parent / "fixtures/step04-skill-hub.py").resolve()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home, project, other = root / "hub", root / "project", root / "other"
            for p in (home, project, other):
                p.mkdir()
            env = {**os.environ, "WORKORCH_STEP04_FIXTURE_ROOT": str(root), "SKILL_HUB_HOME": str(home),
                   "WORKORCH_STEP04_DROP_REGISTRATION_REPLY": "1"}
            def call(*args, cwd=root, environment=env):
                return subprocess.run([sys.executable, str(script), *args], cwd=cwd, env=environment, capture_output=True)
            self.assertEqual(call("status", "--json", cwd=project).returncode, 1)
            self.assertEqual(call("project", "register", "--query", "--request-id", "missing", "--json").returncode, 1)
            self.assertFalse((home / "project-registrations/missing.json").exists())
            first = call("project", "register", str(project), "--request-id", "original", "--json")
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertEqual(first.stdout, b"")
            raw = (home / "project-registrations/original.json").read_bytes()
            self.assertEqual(raw, (root / "dropped-registration-reply.bin").read_bytes())
            for args in (["project", "register", "--query", "--request-id", "original", "--json"],
                         ["project", "register", str(project), "--request-id", "original", "--json"]):
                replay = call(*args)
                self.assertEqual(replay.returncode, 0, replay.stderr)
                self.assertEqual(replay.stdout, raw)
                self.assertEqual((home / "project-registrations/original.json").read_bytes(), raw)
            self.assertEqual(call("project", "register", str(other), "--request-id", "original", "--json").returncode, 1)
            self.assertEqual(call("status", "--json", cwd=other).returncode, 1)
            self.assertEqual(call("status", "--json", cwd=project).returncode, 0)
            self.assertNotEqual(call("project", "register", str(root.parent), "--request-id", "escape", "--json").returncode, 0)
            self.assertNotEqual(call("status", "--json", environment={**env, "SKILL_HUB_HOME": str(root.parent)}).returncode, 0)

    def registration_fixture(self):
        intent = {"requestId": "original", "expectedWorkspaceRevision": 5}
        project, home = Path("/isolated/project"), Path("/isolated/hub")
        external = "workorch-" + hashlib.sha256(b"ws\x00original").hexdigest()
        raw = json.dumps({"request_id": external, "project_path": str(project), "status": "succeeded", "executed": True, "finished": True,
                          "started_at": "2026-10-28T00:00:00Z", "completed_at": "2026-10-28T00:00:01Z",
                          "result": {"registered": True, "project_created": False, "created_directories": [], "skill_count": 0}}).encode()
        observation = {"CommandStarted": True, "ExitCodeKnown": True, "ExitCode": 0, "Complete": True, "ErrorComplete": True}
        for field, total, digest, data in (("Stdout", "TotalStdoutBytes", "StdoutSHA256", raw),
                                          ("Stderr", "TotalStderrBytes", "StderrSHA256", b""), ("RawError", "TotalErrorBytes", "ErrorSHA256", b"")):
            observation.update({field: base64.b64encode(data).decode(), total: len(data), digest: hashlib.sha256(data).hexdigest()})
        plan = {"WorkspaceID": "ws", "RequestID": "original", "DeliverableDirectory": str(project), "InternalDirectory": "/isolated/internal"}
        record = {"Revision": 9, "InternalDirectory": "/isolated/internal", "StartupInput": {"StartupInputHash": "startup"},
                  "PreparationResourceInitializations": [{"Version": "workorch-preparation-resource-initialization/v2", "ExpectedWorkspaceRevision": 5,
                    "CommittedWorkspaceRevision": 9, "StartupInputHash": "startup", "Plan": plan,
                    "Attempts": [{"Result": {"Command": copy.deepcopy(plan)}}],
                    "Registration": {"Plan": {"Version": "skillhub-project-registration/v1", "RequestID": external, "Directory": str(project), "Home": str(home)},
                                     "Attempts": [{"Query": False, "Result": observation}]}}]}
        view = {"requestId": "original", "preparation": {"workspaceId": "ws", "workspaceRevision": 9,
                "resourceInitializations": [{**intent, "committedWorkspaceRevision": 9, "state": "completed", "attempts": 1,
                                             "registrationState": "succeeded", "registrationAttempts": 1}]}}
        return view, record, "ws", intent, project, home

    def test_v2_initialization_rejects_old_contract_and_wrong_receipt_sources(self):
        step04.validate_initialization(*self.registration_fixture())
        mutations = [
            lambda v, r: v["preparation"].update(workspaceRevision=7),
            lambda v, r: v["preparation"]["resourceInitializations"][0].pop("registrationState"),
            lambda v, r: v["preparation"]["resourceInitializations"][0].update(registrationAttempts=0),
            lambda v, r: r["PreparationResourceInitializations"][0].update(Version="workorch-preparation-resource-initialization/v1"),
            lambda v, r: r["PreparationResourceInitializations"][0]["Registration"]["Plan"].update(Home="/other"),
            lambda v, r: r["PreparationResourceInitializations"][0]["Registration"]["Plan"].update(RequestID="other"),
            lambda v, r: r["PreparationResourceInitializations"][0]["Registration"]["Attempts"][0].update(Query=True),
            lambda v, r: r["PreparationResourceInitializations"][0]["Registration"]["Attempts"][0]["Result"].update(Complete=False),
            lambda v, r: r["PreparationResourceInitializations"][0]["Registration"]["Attempts"][0]["Result"].update(StdoutSHA256="wrong"),
            lambda v, r: r["PreparationResourceInitializations"][0]["Attempts"][0]["Result"]["Command"].update(RequestID="other"),
        ]
        for mutation in mutations:
            data = self.registration_fixture()
            mutation(*data[:2])
            with self.assertRaises(RuntimeError):
                step04.validate_initialization(*data)


    def test_private_execution_inputs_require_full_template_process_and_sidecar_source(self):
        version = "workorch-preparation-execution-inputs/v1"
        template = {"id": "confirmed"}
        policy = {"MaxRuntimeRepairAttempts": 10, "MaxDocumentRecoveryAttempts": 3}
        record = {"ID": "ws", "TemplateID": "confirmed", "DeliverableDirectory": "project", "InternalDirectory": "internal", "ProfileID": "profile",
                  "ContextHints": {}, "EvidencePolicies": None, "DefaultLLMSelection": None,
                  "DefinitionAuthoring": {"TemplateSnapshot": template},
                  "StartupPreparation": {"Attempts": [{"SealEvidenceRef": "original", "ExecutionInputVersion": version, "ExpectedWorkspaceRevision": 5, "ProviderCacheEvidenceRef": "provider"}]}}
        target = {"providerId": "ai-proxy", "modelId": "step02-fixture"}
        record["PreparationProviderCacheEvidence"] = [{"Ref": "provider", "Facts": {"Observation": {
            "Configuration": {"Version": "workorch-preparation-provider-configuration/v2", "DefaultChat": target},
            "OwnerEpoch": "frozen-owner", "Catalog": {"ErrorCode": "cache_miss"}}},
            "Selection": {"Version": "workorch-preparation-provider-selection/v2", "Source": "llm_owner_default", "Selection": {"target": target}, "OwnerEpoch": "frozen-owner", "FailureCode": ""}}]
        inputs = {"Version": version, "Template": copy.deepcopy(template),
                  "Process": {"Version": version, "WorkspaceID": "ws", "WorkspaceRevision": 6, "Before": policy, "After": copy.deepcopy(policy)},
                  "Configuration": {"Directory": "project", "InternalDirectory": "internal", "TemplateID": "confirmed", "ProfileID": "profile", "ContextHints": {}, "EvidencePolicies": None, "DefaultLLMSelection": None, "DefaultSoulID": "selected", "SoulBindings": None}}
        record["PreparationSealEvidence"] = [{"Ref": "original", "ExecutionInputs": inputs}]
        bindings = {"defaultSoulId": "selected"}
        step04.validate_execution_inputs(record, bindings)
        for mutate in (
            lambda r, b: r["PreparationSealEvidence"][0].pop("ExecutionInputs"),
            lambda r, b: r["PreparationSealEvidence"][0]["ExecutionInputs"].update(Version="other"),
            lambda r, b: r["PreparationSealEvidence"][0]["ExecutionInputs"]["Process"].update(WorkspaceID="neighbor"),
            lambda r, b: r["PreparationSealEvidence"][0]["ExecutionInputs"]["Process"]["After"].update(MaxRuntimeRepairAttempts=11),
            lambda r, b: r["PreparationSealEvidence"][0]["ExecutionInputs"]["Template"].update(id="live"),
            lambda r, b: r["PreparationSealEvidence"][0]["ExecutionInputs"]["Configuration"].update(ContextHints={"changed": True}),
            lambda r, b: b.update(defaultSoulId="other"),
            lambda r, b: r["PreparationProviderCacheEvidence"][0]["Selection"]["Selection"]["target"].update(modelId="neighbor"),
            lambda r, b: r["PreparationProviderCacheEvidence"][0]["Selection"].update(Version="workorch-preparation-provider-selection/v1"),
            lambda r, b: r["PreparationProviderCacheEvidence"][0]["Selection"].update(OwnerEpoch="other"),
        ):
            bad_record, bad_bindings = copy.deepcopy(record), copy.deepcopy(bindings)
            with self.assertRaises((RuntimeError, KeyError)):
                mutate(bad_record, bad_bindings)
                step04.validate_execution_inputs(bad_record, bad_bindings)


if __name__ == "__main__":
    unittest.main()

