#!/usr/bin/env python3
"""Pure Runtime V2 live-verifier assertions with hermetic regressions."""

from __future__ import annotations

import sys
from typing import Any


def bounded_git_sequence_errors(operations: list[dict[str, Any]]) -> list[str]:
    """Validate accepted Git effects, excluding invalidated-dispatch history."""

    git_operations = [item for item in operations if item.get("kind") == "git/write"]
    accepted = [item for item in git_operations if item.get("outcome") == "accepted"]
    expected_variants = ["init", "add", "commit"]
    errors: list[str] = []

    got_variants = [item.get("variantId") for item in accepted]
    if got_variants != expected_variants:
        errors.append(
            f"accepted git variants={got_variants}, want ordered {expected_variants}"
        )
    for variant in expected_variants:
        count = sum(1 for item in accepted if item.get("variantId") == variant)
        if count != 1:
            errors.append(f"accepted git sequence variant {variant} count={count}, want 1")
    for item in accepted:
        if not item.get("requiredOperationRef"):
            errors.append(
                f"accepted git operation {item.get('operationId')} lacks requiredOperationRef"
            )

    hashes = [item.get("capabilityStateHash") for item in accepted]
    if any(not value for value in hashes):
        errors.append("accepted git operation lacks capabilityStateHash")
    elif len(set(hashes)) != len(expected_variants):
        errors.append(
            "accepted git operations must use one fresh capabilityStateHash per durable effect; "
            f"got {hashes}"
        )
    return errors


def _accepted(variant: str, snapshot_hash: str, operation_id: str) -> dict[str, Any]:
    return {
        "kind": "git/write",
        "variantId": variant,
        "outcome": "accepted",
        "capabilityStateHash": snapshot_hash,
        "requiredOperationRef": operation_id,
        "operationId": f"op-{variant}",
    }


def _self_test() -> None:
    # Unexecuted add/commit operations from invalidated dispatches are retained
    # for lineage.  They are not duplicate durable effects.
    valid = [
        _accepted("init", "snapshot-init", "repo-init"),
        {"kind": "git/write", "variantId": "add", "capabilityStateHash": "snapshot-init"},
        {"kind": "git/write", "variantId": "commit", "capabilityStateHash": "snapshot-init"},
        _accepted("add", "snapshot-add", "stage-changes"),
        {"kind": "git/write", "variantId": "commit", "capabilityStateHash": "snapshot-add"},
        _accepted("commit", "snapshot-commit", "create-commit"),
    ]
    if errors := bounded_git_sequence_errors(valid):
        raise AssertionError(f"valid refreshed Git chain rejected: {errors}")

    duplicate = valid + [_accepted("commit", "snapshot-extra", "create-commit")]
    if not bounded_git_sequence_errors(duplicate):
        raise AssertionError("duplicate accepted Git commit was accepted")

    reused_snapshot = [
        _accepted("init", "snapshot-one", "repo-init"),
        _accepted("add", "snapshot-one", "stage-changes"),
        _accepted("commit", "snapshot-one", "create-commit"),
    ]
    if not bounded_git_sequence_errors(reused_snapshot):
        raise AssertionError("reused Git capability-state snapshot was accepted")

    print("runtime-v2 live contract self-test passed")


if __name__ == "__main__":
    if sys.argv[1:] != ["--self-test"]:
        raise SystemExit("usage: verify_runtime_v2_live_contract.py --self-test")
    _self_test()
