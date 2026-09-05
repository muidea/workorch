#!/usr/bin/env python3
"""Read-only assertions for accepted Workspace execution-activation responses.

These are verifier expectations of RuntimeExecutionIntentAdmission, not a
classifier, lifecycle reducer or retry policy. Only the owner can admit an
intent; polling for subsequent Run facts remains a separate verification step.
"""

import argparse
import json


ACCEPTED_ADMISSIONS = {
    "reconciliation_requested": frozenset({
        "current_run_continuation_accepted", "pending_run_activated", "new_run_activated",
    }),
    "replacement_pending": frozenset({
        "duplicate_restart", "predecessor_supersede_requested", "terminal_predecessor_successor_requested",
    }),
}


def validate_accepted_activation(data, *, intent, request_reason=None, request_id=None,
                                 workspace_id=None, require_source=False):
    if not isinstance(data, dict):
        raise ValueError("activation response must be an object")
    if intent not in {"resume", "restart"}:
        raise ValueError(f"invalid expected intent: {intent!r}")
    errors = []
    expected_fields = {"intent": intent, "state": "accepted"}
    for field, value in (("requestReason", request_reason), ("requestId", request_id), ("workspaceId", workspace_id)):
        if value is not None:
            expected_fields[field] = value
    if data.get("accepted") is not True:
        errors.append(f"accepted is {data.get('accepted')!r}, want true")
    for field, expected in expected_fields.items():
        if data.get(field) != expected:
            errors.append(f"{field} is {data.get(field)!r}, want {expected!r}")
    for field in ("workspaceId", "requestId") + (("source",) if require_source else ()):
        value = data.get(field)
        if not isinstance(value, str) or not value.strip():
            errors.append(f"{field} must be a nonempty string")
    outcome, reason = data.get("outcome"), data.get("reason")
    if (not isinstance(outcome, str) or not isinstance(reason, str)
            or reason not in ACCEPTED_ADMISSIONS.get(outcome, ())):
        errors.append(f"owner admission outcome/reason is {outcome!r}/{reason!r}")
    if errors:
        raise ValueError("; ".join(errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("response_file")
    parser.add_argument("--intent", required=True, choices=("resume", "restart"))
    parser.add_argument("--request-reason")
    parser.add_argument("--request-id")
    parser.add_argument("--workspace-id")
    parser.add_argument("--require-source", action="store_true")
    args = parser.parse_args()
    try:
        with open(args.response_file, encoding="utf-8") as handle:
            data = json.load(handle)
        validate_accepted_activation(data, intent=args.intent, request_reason=args.request_reason,
                                     request_id=args.request_id, workspace_id=args.workspace_id,
                                     require_source=args.require_source)
    except (OSError, ValueError) as error:
        raise SystemExit(f"execution activation verification failed ({args.response_file}): {error}") from error


if __name__ == "__main__":
    main()
