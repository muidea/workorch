#!/usr/bin/env python3
"""Validate the evidence artifact produced by the Runtime P6 scripted gate."""

from __future__ import annotations

import argparse
import json
import tempfile
from pathlib import Path
from typing import Any


SUCCESS_MARKERS = (
    "workorch HTTP verification passed",
    "workspace template and soul e2e verification passed",
    "workorch LLM workspace execution verification passed",
    "workorch MCP stdio verification passed",
)


def artifact_errors(artifact_dir: Path) -> tuple[list[str], dict[str, Any]]:
    artifact_dir = artifact_dir.resolve()
    errors: list[str] = []
    state_path = artifact_dir / "managed-runtime-state.json"
    state: dict[str, Any] = {}
    if not state_path.is_file():
        errors.append("missing managed-runtime-state.json")
    else:
        try:
            state = json.loads(state_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            errors.append(f"invalid managed-runtime-state.json: {exc}")

    if state.get("state") != "stopped":
        errors.append(f"managed runtime state={state.get('state')!r}, want 'stopped'")

    isolation: dict[str, str] = {}
    for field, expected_name in (
        ("configDir", "config"),
        ("dataDir", ".workorch-data"),
        ("serverLog", "workorch-server.log"),
    ):
        raw = state.get(field)
        if not isinstance(raw, str) or not raw:
            errors.append(f"managed runtime state missing {field}")
            continue
        path = Path(raw).resolve()
        if path != artifact_dir / expected_name:
            errors.append(f"{field}={path} is not artifact-local {expected_name}")
            continue
        if expected_name == "workorch-server.log":
            if not path.is_file():
                errors.append("missing artifact-local workorch-server.log")
        elif not path.is_dir():
            errors.append(f"missing artifact-local {expected_name} directory")
        isolation[field] = str(path)

    config_path = artifact_dir / "config" / "application.toml"
    if not config_path.is_file():
        errors.append("missing artifact-local config/application.toml")

    step_dir = artifact_dir / "step-logs"
    if not step_dir.is_dir():
        errors.append("missing step-logs directory")
        logs: list[Path] = []
    else:
        logs = sorted(step_dir.glob("*.log"))

    required_step_patterns = (
        "*make-fmt-check.log",
        "*make-console-build.log",
        "*make-vet.log",
        "*go-test-*.log",
        "*go-build-*application-workorch.log",
        "*make-verify-ci-mvp-file.log",
    )
    matched: dict[str, Path] = {}
    for pattern in required_step_patterns:
        entries = sorted(step_dir.glob(pattern)) if step_dir.is_dir() else []
        if len(entries) != 1:
            errors.append(f"expected exactly one step log matching {pattern}, got {len(entries)}")
            continue
        matched[pattern] = entries[0]

    test_log = next((path for pattern, path in matched.items() if "go-test" in pattern), None)
    if test_log is not None:
        test_body = test_log.read_text(encoding="utf-8", errors="replace")
        if "FAIL\t" in test_body or "panic:" in test_body:
            errors.append("all-package Go test log contains a failure")
        if "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz" not in test_body:
            errors.append("all-package Go test log lacks orchestrator coverage")
        if "github.com/muidea/workorch/internal/pkg/workorchruntimecontract" not in test_body:
            errors.append("all-package Go test log lacks runtime contract coverage")

    ci_log = matched.get("*make-verify-ci-mvp-file.log")
    if ci_log is not None:
        ci_body = ci_log.read_text(encoding="utf-8", errors="replace")
        for marker in SUCCESS_MARKERS:
            if marker not in ci_body:
                errors.append(f"CI MVP log lacks success marker: {marker}")
        for marker in ("runState=completed", "completedDispatches=", "evidenceSequence="):
            if marker not in ci_body:
                errors.append(f"CI MVP log lacks Runtime convergence marker: {marker}")

    summary = {
        "kind": "runtime_p6_scripted",
        "artifactContractVersion": 1,
        "target": "verify-ci-mvp-file",
        "isolation": {
            "configDir": isolation.get("configDir"),
            "dataDir": isolation.get("dataDir"),
            "serverLog": isolation.get("serverLog"),
            "managedHarnessTemporaryRoot": str(artifact_dir),
        },
        "gates": {
            "fmtCheck": "*make-fmt-check.log" in matched,
            "consoleBuild": "*make-console-build.log" in matched,
            "vet": "*make-vet.log" in matched,
            "goTestAll": test_log is not None,
            "binaryBuild": "*go-build-*application-workorch.log" in matched,
            "ciMvpFile": ci_log is not None,
        },
        "status": "passed" if not errors else "failed",
    }
    return errors, summary


def validate(artifact_dir: Path, write_summary: bool = False) -> dict[str, Any]:
    errors, summary = artifact_errors(artifact_dir)
    if write_summary:
        (artifact_dir / "p6-scripted-summary.json").write_text(
            json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
    if errors:
        raise ValueError("invalid Runtime P6 scripted artifact: " + "; ".join(errors))
    return summary


def _write_valid_fixture(artifact_dir: Path) -> None:
    (artifact_dir / "config").mkdir(parents=True)
    (artifact_dir / ".workorch-data").mkdir()
    (artifact_dir / "config" / "application.toml").write_text("endpointName = 'workorch'\n", encoding="utf-8")
    (artifact_dir / "workorch-server.log").write_text("ready\n", encoding="utf-8")
    (artifact_dir / "managed-runtime-state.json").write_text(
        json.dumps(
            {
                "state": "stopped",
                "configDir": str(artifact_dir / "config"),
                "dataDir": str(artifact_dir / ".workorch-data"),
                "serverLog": str(artifact_dir / "workorch-server.log"),
            }
        ),
        encoding="utf-8",
    )
    steps = artifact_dir / "step-logs"
    steps.mkdir()
    for name, body in {
        "001-make-fmt-check.log": "",
        "002-make-console-build.log": "",
        "003-make-vet.log": "",
        "004-go-test-count=1-.-....log": "ok\tgithub.com/muidea/workorch/internal/modules/kernel/orchestrator/biz\n"
        "ok\tgithub.com/muidea/workorch/internal/pkg/workorchruntimecontract\n",
        "005-go-build-o-bin-workorch-.-application-workorch.log": "",
        "006-make-verify-ci-mvp-file.log": "\n".join((*SUCCESS_MARKERS, "runState=completed", "completedDispatches=4", "evidenceSequence=4")),
    }.items():
        (steps / name).write_text(body, encoding="utf-8")


def self_test() -> None:
    with tempfile.TemporaryDirectory(prefix="workorch-p6-scripted-contract-") as raw_dir:
        artifact_dir = Path(raw_dir)
        _write_valid_fixture(artifact_dir)
        summary = validate(artifact_dir, write_summary=True)
        if summary["status"] != "passed":
            raise AssertionError(f"valid P6 artifact summary={summary}")
        if not (artifact_dir / "p6-scripted-summary.json").is_file():
            raise AssertionError("P6 summary was not written")
        state_path = artifact_dir / "managed-runtime-state.json"
        state = json.loads(state_path.read_text(encoding="utf-8"))
        state["dataDir"] = "/tmp/not-artifact-local"
        state_path.write_text(json.dumps(state), encoding="utf-8")
        try:
            validate(artifact_dir)
        except ValueError as exc:
            if "dataDir" not in str(exc):
                raise
        else:
            raise AssertionError("non-isolated P6 artifact was accepted")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("artifact_dir", nargs="?", type=Path)
    parser.add_argument("--write-summary", action="store_true")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print("runtime P6 scripted artifact contract self-test passed")
        return
    if args.artifact_dir is None:
        parser.error("artifact_dir is required unless --self-test is used")
    summary = validate(args.artifact_dir.resolve(), write_summary=args.write_summary)
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
