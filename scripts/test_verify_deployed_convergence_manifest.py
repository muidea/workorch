#!/usr/bin/env python3
"""Hermetic schema, reference, assertion and aggregation regressions."""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest

import verify_deployed_convergence_manifest as verifier


class ManifestFixture:
    def __init__(
        self,
        root: Path,
        scenario_id: str,
        *,
        provider_class: str = "mock",
        scenario_variant: str = "ci-safe",
    ) -> None:
        self.root = root / f"{scenario_id.lower()}-{scenario_variant}-{provider_class}"
        self.root.mkdir(parents=True)
        self.scenario_id = scenario_id
        self.rule = verifier.SCENARIO_RULES[scenario_id]
        self.data_root_identity = "sha256:" + "d" * 64
        self.refs_by_field: dict[str, list[str]] = {
            field: [] for field in verifier.REFERENCE_FIELDS
        }
        self.hashes_by_field: dict[str, list[str]] = {
            field: [] for field in verifier.HASH_FIELDS
        }
        self.artifacts: list[dict[str, object]] = []
        self.assertion_refs: list[str] = []
        self.workspace_ids = [
            f"ws-{scenario_id.lower()}-{index + 1}"
            for index in range(self.rule.min_workspaces)
        ]
        self.run_ids = [f"run-{scenario_id.lower()}-{index + 1}" for index in range(self.rule.min_runs)]

        for field in sorted(self.rule.required_refs):
            self._add_reference(field, verifier.REFERENCE_FIELDS[field])
        for field in sorted(self.rule.required_hashes):
            self._add_hash(field, verifier.HASH_FIELDS[field])
        if scenario_id == "DC-10":
            for workspace_index, workspace_id in enumerate(self.workspace_ids[1:], start=2):
                for field in sorted(self.rule.required_refs):
                    self._add_reference(
                        field,
                        verifier.REFERENCE_FIELDS[field],
                        suffix=f"{field}-{workspace_index}",
                        workspace_id=workspace_id,
                    )
                for field in sorted(self.rule.required_hashes):
                    self._add_hash(
                        field,
                        verifier.HASH_FIELDS[field],
                        suffix=f"{field}-{workspace_index}",
                        workspace_id=workspace_id,
                    )

        self.restart_epochs: list[dict[str, object]] = []
        for index in range(self.rule.min_restart_epochs):
            ref = self._add_artifact("restart_epoch", f"restart-{index + 1}")
            self.restart_epochs.append(
                {
                    "epochId": f"epoch-{index + 1}",
                    "processId": 2000 + index,
                    "startedAt": f"2026-08-26T00:00:0{index + 1}Z",
                    "evidenceRef": ref,
                }
            )

        if not self.artifacts:
            self._add_reference("evidenceRefs", "accepted_evidence")

        assertion_ids = sorted(self.rule.assertions)
        assertion_refs = list(self.assertion_refs)
        self.assertions = [
            {
                "id": assertion_id,
                "status": "passed",
                "evidenceRefs": assertion_refs if index == 0 else assertion_refs[:1],
            }
            for index, assertion_id in enumerate(assertion_ids)
        ]
        self.document: dict[str, object] = {
            "schemaVersion": verifier.SCHEMA_VERSION,
            "scenarioId": scenario_id,
            "scenarioVariant": scenario_variant,
            "profileId": "software-dev" if scenario_id == "DC-04" else "generic",
            "providerClass": provider_class,
            "producerEntrypoint": f"fixture:{scenario_id}",
            "invocationId": f"fixture-{scenario_id.lower()}-{scenario_variant}-{provider_class}",
            "serviceRevision": "revision-under-test",
            "configIdentity": "sha256:" + "c" * 64,
            "dataRootIdentity": self.data_root_identity,
            "startedAt": "2026-08-26T00:00:00Z",
            "completedAt": "2026-08-26T00:00:10Z",
            "boundedDeadlineSeconds": 60,
            "workspaceIds": self.workspace_ids,
            "runIds": self.run_ids,
            "definitionRevisions": [
                f"definition-{index + 1}" for index in range(self.rule.min_definitions)
            ],
            "runtimeStateRevisions": [
                index + 1 for index in range(self.rule.min_runtime_revisions)
            ],
            **self.refs_by_field,
            **self.hashes_by_field,
            "restartEpochs": self.restart_epochs,
            "assertions": self.assertions,
            "artifacts": self.artifacts,
        }
        self.path = self.root / f"{scenario_id.lower()}.dce.json"
        self.write()

    def _add_artifact(self, kind: str, suffix: str, workspace_id: str | None = None) -> str:
        selected_workspace_id = workspace_id or self.workspace_ids[0]
        workspace_index = self.workspace_ids.index(selected_workspace_id)
        selected_run_id = ""
        if self.run_ids:
            selected_run_id = self.run_ids[min(workspace_index, len(self.run_ids) - 1)]
        ref = f"workorch-evidence://{self.scenario_id.lower()}/{suffix}"
        artifact_path = self.root / f"{suffix}.json"
        content = json.dumps(
            {"scenarioId": self.scenario_id, "kind": kind, "suffix": suffix},
            sort_keys=True,
        ).encode()
        artifact_path.write_bytes(content)
        digest = "sha256:" + hashlib.sha256(content).hexdigest()
        self.artifacts.append(
            {
                "ref": ref,
                "kind": kind,
                "path": artifact_path.name,
                "sha256": digest,
                "dataRootIdentity": self.data_root_identity,
                "workspaceId": selected_workspace_id,
                "runId": selected_run_id,
            }
        )
        self.assertion_refs.append(ref)
        return ref

    def _add_reference(
        self,
        field: str,
        kind: str,
        *,
        suffix: str | None = None,
        workspace_id: str | None = None,
    ) -> None:
        self.refs_by_field[field].append(
            self._add_artifact(kind, suffix or field, workspace_id)
        )

    def _add_hash(
        self,
        field: str,
        kind: str,
        *,
        suffix: str | None = None,
        workspace_id: str | None = None,
    ) -> None:
        self._add_artifact(kind, suffix or field, workspace_id)
        self.hashes_by_field[field].append(str(self.artifacts[-1]["sha256"]))

    def write(self) -> None:
        self.path.write_text(json.dumps(self.document, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def fixture_for_requirement(
    root: Path,
    requirement: verifier.ProfileRequirement,
) -> ManifestFixture:
    provider_class = "real" if requirement.provider_constraint == "real" else "mock"
    return ManifestFixture(
        root,
        requirement.scenario_id,
        provider_class=provider_class,
        scenario_variant=requirement.scenario_variant,
    )


def register_fixture(
    registry: dict[tuple[str, str], frozenset[str]],
    fixture: ManifestFixture,
) -> None:
    identity = (fixture.scenario_id, str(fixture.document["scenarioVariant"]))
    producers = set(registry.get(identity, frozenset()))
    producers.add(str(fixture.document["producerEntrypoint"]))
    registry[identity] = frozenset(producers)


class DeployedConvergenceManifestTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.schema = verifier._load_schema(verifier.DEFAULT_SCHEMA)

    def test_valid_dc01_fixture_passes_schema_refs_and_assertions(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-01")
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertEqual([], errors)
            self.assertIsNotNone(value)

    def test_checked_in_invalid_fixture_is_rejected_by_schema(self) -> None:
        path = (
            Path(__file__).parent
            / "testdata"
            / "deployed-convergence-manifest"
            / "invalid-schema.dce.json"
        )
        document = verifier.load_json(path)
        value, errors = verifier.validate_manifest(document, path, self.schema)
        self.assertIsNone(value)
        self.assertTrue(any("is required" in error for error in errors), errors)
        self.assertTrue(any("not allowed" in error for error in errors), errors)

    def test_unknown_field_fails_closed(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-01")
            fixture.document["coveragePassed"] = True
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertIn("manifest.coveragePassed is not allowed by schema", errors)

    def test_artifact_digest_mismatch_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-01")
            fixture.artifacts[0]["sha256"] = "sha256:" + "0" * 64
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("digest mismatch" in error for error in errors), errors)

    def test_artifact_path_traversal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            fixture = ManifestFixture(root, "DC-01")
            outside = root / "outside.json"
            outside.write_text("{}", encoding="utf-8")
            fixture.artifacts[0]["path"] = "../outside.json"
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("must stay relative" in error for error in errors), errors)

    def test_reference_kind_mismatch_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-01")
            dispatch_ref = fixture.refs_by_field["dispatchRefs"][0]
            artifact = next(item for item in fixture.artifacts if item["ref"] == dispatch_ref)
            artifact["kind"] = "owner_receipt"
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("want 'sealed_dispatch'" in error for error in errors), errors)

    def test_artifact_outside_workspace_scope_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-10")
            fixture.artifacts[0]["workspaceId"] = "ws-outside-manifest"
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("outside manifest scope" in error for error in errors), errors)

    def test_dc10_required_ref_must_cover_each_workspace_and_run(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-10")
            fixture.refs_by_field["dispatchRefs"].pop()
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(
                any("DC-10 manifest.dispatchRefs lacks per-workspace evidence" in error for error in errors),
                errors,
            )
            self.assertTrue(
                any("DC-10 manifest.dispatchRefs lacks per-run evidence" in error for error in errors),
                errors,
            )

    def test_dc10_required_hash_must_cover_each_workspace_and_run(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-10")
            fixture.hashes_by_field["budgetViewHashes"].pop()
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(
                any("DC-10 manifest.budgetViewHashes lacks per-workspace evidence" in error for error in errors),
                errors,
            )
            self.assertTrue(
                any("DC-10 manifest.budgetViewHashes lacks per-run evidence" in error for error in errors),
                errors,
            )

    def test_missing_mandatory_assertion_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-06")
            removed = fixture.assertions.pop()
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any(removed["id"] in error for error in errors), errors)

    def test_failed_assertion_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-11")
            fixture.assertions[0]["status"] = "failed"
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("did not pass" in error for error in errors), errors)

    def test_restart_epoch_allows_pid_reuse_but_requires_time_order(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-05")
            fixture.restart_epochs[1]["processId"] = fixture.restart_epochs[0]["processId"]
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertEqual([], errors)
            self.assertIsNotNone(value)

            fixture.restart_epochs[1]["startedAt"] = fixture.restart_epochs[0]["startedAt"]
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("strictly ordered by startedAt" in error for error in errors), errors)

    def test_restart_epochs_cannot_reuse_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-05")
            fixture.restart_epochs[1]["evidenceRef"] = fixture.restart_epochs[0]["evidenceRef"]
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("reuses evidenceRef" in error for error in errors), errors)

    def test_timestamp_must_use_strict_rfc3339_separator_and_offset(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            fixture = ManifestFixture(Path(temporary), "DC-01")
            fixture.document["startedAt"] = "2026-08-26 00:00:00+0000"
            value, errors = verifier.validate_manifest(fixture.document, fixture.path, self.schema)
            self.assertIsNone(value)
            self.assertTrue(any("startedAt must be an RFC3339 timestamp" in error for error in errors), errors)

    def test_aggregate_rejects_symlinked_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            evidence = root / "evidence"
            evidence.mkdir()
            fixture = ManifestFixture(source, "DC-01")
            linked = evidence / "dc-01.dce.json"
            try:
                os.symlink(fixture.path, linked)
            except OSError as exc:
                self.skipTest(f"symlink unavailable: {exc}")
            aggregate, errors = verifier.aggregate_manifests(evidence, self.schema, "ci-safe")
            self.assertEqual("failed", aggregate["status"])
            self.assertTrue(any("manifest path must not be a symlink" in error for error in errors), errors)

    def test_ci_safe_aggregate_explicitly_fails_when_dc06_is_missing(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for requirement in verifier.PROFILE_REQUIREMENTS["ci-safe"]:
                if requirement.scenario_id != "DC-06":
                    fixture_for_requirement(root, requirement)
            aggregate, errors = verifier.aggregate_manifests(root, self.schema, "ci-safe")
            self.assertEqual("failed", aggregate["status"])
            self.assertTrue(any("missing DC-06" in error for error in errors), errors)

    def test_complete_ci_safe_fixture_set_aggregates_with_explicit_test_registry(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            test_registry: dict[tuple[str, str], frozenset[str]] = {}
            for requirement in verifier.PROFILE_REQUIREMENTS["ci-safe"]:
                register_fixture(test_registry, fixture_for_requirement(root, requirement))
            aggregate, errors = verifier.aggregate_manifests(
                root,
                self.schema,
                "ci-safe",
                producer_registry=test_registry,
            )
            self.assertEqual([], errors)
            self.assertEqual("passed", aggregate["status"])
            self.assertEqual(8, len(aggregate["validatedManifests"]))

    def test_release_registry_rejects_structurally_valid_dc06_fixture(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            ManifestFixture(root, "DC-06")
            aggregate, errors = verifier.aggregate_manifests(root, self.schema, "ci-safe")
            self.assertEqual("failed", aggregate["status"])
            self.assertTrue(
                any("not a registered deployed producer for DC-06" in error for error in errors),
                errors,
            )

    def test_real_provider_profile_rejects_mock_manifests(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            ManifestFixture(root, "DC-01", provider_class="mock")
            ManifestFixture(root, "DC-02", provider_class="mock")
            aggregate, errors = verifier.aggregate_manifests(root, self.schema, "real-provider")
            self.assertEqual("failed", aggregate["status"])
            self.assertTrue(any("providerClass=real" in error for error in errors), errors)

    def test_real_provider_profile_accepts_only_exact_registered_variant(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            test_registry: dict[tuple[str, str], frozenset[str]] = {}
            for requirement in verifier.PROFILE_REQUIREMENTS["real-provider"]:
                register_fixture(test_registry, fixture_for_requirement(root, requirement))
            aggregate, errors = verifier.aggregate_manifests(
                root,
                self.schema,
                "real-provider",
                producer_registry=test_registry,
            )
            self.assertEqual([], errors)
            self.assertEqual("passed", aggregate["status"])

    def test_full_profile_requires_both_dc01_variants(self) -> None:
        for omitted_variant in ("ci-safe", "real-provider"):
            with self.subTest(omitted_variant=omitted_variant), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                test_registry: dict[tuple[str, str], frozenset[str]] = {}
                for requirement in verifier.PROFILE_REQUIREMENTS["full"]:
                    if requirement.scenario_id == "DC-01" and requirement.scenario_variant == omitted_variant:
                        continue
                    register_fixture(test_registry, fixture_for_requirement(root, requirement))
                aggregate, errors = verifier.aggregate_manifests(
                    root,
                    self.schema,
                    "full",
                    producer_registry=test_registry,
                )
                self.assertEqual("failed", aggregate["status"])
                self.assertTrue(
                    any(
                        "missing DC-01" in error
                        and f"scenarioVariant='{omitted_variant}'" in error
                        for error in errors
                    ),
                    errors,
                )

    def test_full_profile_selects_two_independent_dc01_manifests(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            test_registry: dict[tuple[str, str], frozenset[str]] = {}
            for requirement in verifier.PROFILE_REQUIREMENTS["full"]:
                register_fixture(test_registry, fixture_for_requirement(root, requirement))
            aggregate, errors = verifier.aggregate_manifests(
                root,
                self.schema,
                "full",
                producer_registry=test_registry,
            )
            self.assertEqual([], errors)
            self.assertEqual("passed", aggregate["status"])
            selected_dc01 = [
                item for item in aggregate["validatedManifests"] if item["scenarioId"] == "DC-01"
            ]
            self.assertEqual(
                {"ci-safe", "real-provider"},
                {item["scenarioVariant"] for item in selected_dc01},
            )
            self.assertEqual(len(verifier.PROFILE_REQUIREMENTS["full"]), len(aggregate["validatedManifests"]))

    def test_duplicate_json_keys_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "duplicate.dce.json"
            path.write_text('{"scenarioId":"DC-01","scenarioId":"DC-02"}', encoding="utf-8")
            with self.assertRaises(verifier.DuplicateKeyError):
                verifier.load_json(path)


if __name__ == "__main__":
    unittest.main()
