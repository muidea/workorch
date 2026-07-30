#!/usr/bin/env python3
"""Deterministic Runtime V2 candidate mock used by the managed MCP verifier.

The provider prompt contains both the current authority bundle and historical
evidence. This helper intentionally derives candidate coverage from the final
structured Focus only; it must never infer a current assessment slice by
searching all historical cwi-* text in the prompt. Candidate shape selection
uses the canonical C3 requiredState; RuntimeAction is not a Runtime V2 prompt
or fixture authority.
"""

import argparse
import json
import re
from typing import Any


class MockContractError(ValueError):
    """The verifier prompt is missing a required current-runtime contract."""


def _last_scalar(pattern: str, content: str) -> str:
    matches = re.findall(pattern, content or "")
    return matches[-1] if matches else ""


def _field_objects(content: str, field: str) -> list[dict[str, Any]]:
    """Decode every JSON object directly assigned to a named rendered field."""
    decoder = json.JSONDecoder()
    objects: list[dict[str, Any]] = []
    for match in re.finditer(rf'"{re.escape(field)}"\s*:\s*', content or ""):
        start = match.end()
        while start < len(content) and content[start].isspace():
            start += 1
        if start >= len(content) or content[start] != "{":
            continue
        try:
            value, _ = decoder.raw_decode(content[start:])
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            objects.append(value)
    return objects


def _coverage_ids(value: Any) -> list[str]:
    if not isinstance(value, list) or not value:
        raise MockContractError("current Focus has no CoverageWorkItemIDs")
    result: list[str] = []
    seen: set[str] = set()
    for item in value:
        if not isinstance(item, str) or not item:
            raise MockContractError("current Focus has an invalid coverage work item ID")
        if item in seen:
            raise MockContractError(f"current Focus has duplicate coverage work item ID {item!r}")
        seen.add(item)
        result.append(item)
    return result


def current_focus(content: str) -> tuple[str, list[str]]:
    """Return the final rendered current Focus and its exact ordered coverage."""
    focuses = _field_objects(content, "focus") or _field_objects(content, "Focus")
    if not focuses:
        raise MockContractError("rendered candidate context has no structured Focus")
    focus = focuses[-1]
    focus_id = focus.get("ID") or focus.get("id")
    if not isinstance(focus_id, str) or not focus_id:
        raise MockContractError("current Focus has no ID")
    coverage = focus.get("CoverageWorkItemIDs")
    if coverage is None:
        coverage = focus.get("coverageWorkItemIds")
    return focus_id, _coverage_ids(coverage)


def current_evidence_refs(content: str, operation_id: str) -> list[str]:
    refs = re.findall(
        r'"(workorch://[^"]+|run-[^"]+/graph-[^"]+|run-[^"]+/validation/[^"]+)"',
        content or "",
    )
    current = list(dict.fromkeys(ref for ref in refs if operation_id in ref))
    if not current:
        raise MockContractError(
            f"rendered candidate context has no current evidence for operation {operation_id!r}"
        )
    return current


def decision_payload(user_content: str, server_id: str) -> dict[str, Any]:
    required_state = _last_scalar(r'"requiredState"\s*:\s*"([^"]+)"', user_content)
    focus_id, coverage_ids = current_focus(user_content)

    if required_state == "need_assessment":
        operation_id = "mcp-stdio-call" if focus_id == "focus-002" else "mcp-stdio-validation"
        current_refs = current_evidence_refs(user_content, operation_id)
        return {
            "focusId": focus_id,
            "coverageWorkItemIds": coverage_ids,
            "disposition": "satisfied",
            "items": [
                {
                    "coverageWorkItemId": coverage_work_item_id,
                    "satisfied": True,
                    "evidenceRefs": current_refs[:1],
                }
                for coverage_work_item_id in coverage_ids
            ],
            "summary": "MCP stdio invocation returned structured tool evidence.",
            "evidenceRefs": current_refs[:1],
        }

    if required_state == "need_validation":
        if focus_id == "focus-002":
            return {
                "operations": [
                    {
                        "id": "mcp-stdio-call",
                        "kind": "mcp/tool_call",
                        "coverageWorkItemIds": coverage_ids,
                        "parameters": [
                            {"name": "serverId", "string": server_id},
                            {"name": "tool", "string": "repo.summary"},
                            {"name": "input.path", "string": "."},
                        ],
                    }
                ]
            }
        return {
            "operations": [
                {
                    "id": "mcp-stdio-validation",
                    "kind": "validation/managed_session",
                    "coverageWorkItemIds": coverage_ids,
                    "parameters": [{"name": "command", "string": "true"}],
                }
            ]
        }

    raise MockContractError(f"rendered candidate context has unsupported requiredState {required_state!r}")


def _prompt(*objects: dict[str, Any], evidence_ref: str = "") -> str:
    rendered = "\n".join(json.dumps(item, separators=(",", ":")) for item in objects)
    if evidence_ref:
        rendered += f'\n"{evidence_ref}"'
    return rendered


def self_test() -> None:
    historic_then_current = _prompt(
        {"requiredState": "need_assessment", "focus": {"id": "focus-001", "coverageWorkItemIds": ["cwi-001"]}},
        {"focus": {"id": "focus-002", "coverageWorkItemIds": ["cwi-002"]}},
        evidence_ref="workorch://runtime-operations/dispatch-2/mcp-stdio-call",
    )
    assessment = decision_payload(historic_then_current, "server-test")
    if assessment["coverageWorkItemIds"] != ["cwi-002"] or [item["coverageWorkItemId"] for item in assessment["items"]] != ["cwi-002"]:
        raise AssertionError(f"historical coverage leaked into assessment: {assessment}")

    multiple_current = _prompt(
        {"requiredState": "need_validation", "focus": {"id": "focus-003", "coverageWorkItemIds": ["cwi-003", "cwi-004"]}}
    )
    validation = decision_payload(multiple_current, "server-test")
    if validation["operations"][0]["coverageWorkItemIds"] != ["cwi-003", "cwi-004"]:
        raise AssertionError(f"current coverage ordering changed: {validation}")

    no_focus = json.dumps({"requiredState": "need_assessment"})
    no_evidence = _prompt(
        {"requiredState": "need_assessment", "focus": {"id": "focus-002", "coverageWorkItemIds": ["cwi-002"]}}
    )
    duplicate_coverage = _prompt(
        {"requiredState": "need_validation", "focus": {"id": "focus-002", "coverageWorkItemIds": ["cwi-002", "cwi-002"]}}
    )
    for content in (no_focus, no_evidence, duplicate_coverage):
        try:
            decision_payload(content, "server-test")
        except MockContractError:
            continue
        raise AssertionError(f"invalid mock contract was accepted: {content}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if not args.self_test:
        parser.error("only --self-test is supported when invoked directly")
    self_test()
    print("verify_mcp_stdio_mock self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
