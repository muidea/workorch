#!/usr/bin/env python3
"""Deterministic C12 candidate mock for the managed MCP verifier.

The mock treats the provider prompt as an untrusted transport. It reads only
the current sealed C3 authority, selects an operation BindingRef exposed by
that authority, and derives the MCP server/tool pair from the binding's exact
owner-attested validation recipe. It never puts a raw CWI ID on an operation.
"""

import argparse
import json
import re
from typing import Any


class MockContractError(ValueError):
    """The verifier prompt is missing a required current-runtime contract."""


def _object(value: Any, *keys: str) -> Any:
    if not isinstance(value, dict):
        return None
    for key in keys:
        if key in value:
            return value[key]
    return None


def _strings(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, str) and item]


def _unique(values: list[str]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        if value and value not in seen:
            seen.add(value)
            result.append(value)
    return result


def _prompt_sections(content: str) -> dict[str, str]:
    sections: dict[str, str] = {}
    for match in re.finditer(r"(?ms)^###\s+([^\n]+)\n(.*?)(?=^###\s+|\Z)", content or ""):
        section_id = match.group(1).strip()
        body = match.group(2).strip()
        if section_id and body:
            sections[section_id] = body
    return sections


def _leading_json_object(body: str, section_id: str) -> dict[str, Any]:
    try:
        value, _ = json.JSONDecoder().raw_decode((body or "").lstrip())
    except json.JSONDecodeError as error:
        raise MockContractError(f"invalid {section_id} JSON: {error}") from error
    if not isinstance(value, dict):
        raise MockContractError(f"{section_id} must start with a JSON object")
    return value


def _current_authority(content: str) -> tuple[dict[str, Any], dict[str, Any], str]:
    sections = _prompt_sections(content)
    body = sections.get("C3.runtime-authority")
    if not body:
        raise MockContractError("missing C3.runtime-authority")
    authority = _leading_json_object(body, "C3.runtime-authority")
    context = _object(authority, "context", "Context")
    if not isinstance(context, dict):
        raise MockContractError("C3.runtime-authority has no context")
    return authority, context, sections.get("C4.execution-tail", "")


def _semantic_authority(content: str) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
    body = _prompt_sections(content).get("C3.semantic-authority")
    if not body:
        raise MockContractError("missing C3.semantic-authority")
    authority = _leading_json_object(body, "C3.semantic-authority")
    manifest = _object(authority, "manifest", "Manifest")
    seed = _object(authority, "definitionSeed", "DefinitionSeed")
    if not isinstance(manifest, dict) or not isinstance(seed, dict):
        raise MockContractError("C3 semantic authority has no manifest or definition seed")
    return authority, manifest, seed


def semantic_plan_payload(user_content: str, expected_server_id: str) -> dict[str, Any]:
    authority, manifest, seed = _semantic_authority(user_content)
    plan_step = _object(authority, "planStep", "PlanStep")
    schema = _object(authority, "schema", "Schema") or {}
    properties = _object(schema, "properties", "Properties") or {}
    order_schema = _object(properties, "coverageOrder", "CoverageOrder") or {}
    order_items = _object(order_schema, "items", "Items") or {}
    coverage_order = _unique(_strings(_object(order_items, "enum", "Enum")))
    if not coverage_order:
        coverage_order = _unique([
            _object(item, "id", "ID") or ""
            for item in _object(manifest, "coverageItems", "CoverageItems") or []
            if isinstance(item, dict)
        ])
    if not coverage_order:
        raise MockContractError("semantic request has no bounded coverage order")
    document = {
        "documentKind": "semantic_plan",
        "schemaVersion": "workorch.runtime_semantic_plan_document",
        "manifestHash": _object(manifest, "hash", "Hash"),
        "definitionSeedHash": _object(manifest, "definitionSeedHash", "DefinitionSeedHash") or _object(seed, "hash", "Hash"),
        "planStep": plan_step,
        "coverageOrder": coverage_order,
        "proposals": [],
    }
    if plan_step == "coverage_order":
        return document
    if plan_step != "proof_batch":
        raise MockContractError(f"semantic request has invalid plan step {plan_step!r}")
    batch_id = _object(authority, "batchId", "BatchID")
    batch_sequence = _object(authority, "batchSequence", "BatchSequence")
    batch_coverage = _unique(_strings(_object(authority, "batchCoverageWorkItemIds", "BatchCoverageWorkItemIDs")))
    if not isinstance(batch_id, str) or not batch_id or not isinstance(batch_sequence, int) or isinstance(batch_sequence, bool) or not batch_coverage:
        raise MockContractError("semantic proof batch has incomplete identity")
    if any(coverage_id not in coverage_order for coverage_id in batch_coverage):
        raise MockContractError("semantic proof batch is outside bounded coverage order")
    manifest_coverage = _unique([
        _object(item, "id", "ID") or ""
        for item in _object(manifest, "coverageItems", "CoverageItems") or []
        if isinstance(item, dict)
    ])
    if set(manifest_coverage) != set(batch_coverage):
        raise MockContractError("semantic proof manifest does not match bounded batch coverage")
    catalog = None
    for candidate in _object(manifest, "ownerCatalogs", "OwnerCatalogs") or []:
        if isinstance(candidate, dict) and _object(candidate, "ownerModuleId", "OwnerModuleID") == "blocks/mcp":
            catalog = candidate
            break
    if not isinstance(catalog, dict):
        raise MockContractError("semantic manifest has no MCP owner catalog")
    selectors = _strings(_object(catalog, "selectorRefs", "SelectorRefs"))
    if expected_server_id not in selectors:
        raise MockContractError("semantic MCP catalog does not contain verifier server")
    recipes = _strings(_object(catalog, "validationRecipeRefs", "ValidationRecipeRefs"))
    expected_prefix = f"mcp/tool_call/{expected_server_id}/"
    recipes = [recipe for recipe in recipes if recipe.startswith(expected_prefix)]
    if len(recipes) != 1:
        raise MockContractError("semantic MCP catalog does not contain one verifier tool recipe")
    proposals = []
    for coverage_id in batch_coverage:
        proposals.append({
            "coverageWorkItemId": coverage_id,
            "ownerModuleId": "blocks/mcp",
            "operationKind": "mcp/tool_call",
            "target": {"policy": "existing_owner_scope", "ownerSelectorRefs": [expected_server_id]},
            "requiredEvidenceKinds": ["validation"],
            "requiredEffectScopes": ["none"],
            "validationRecipeId": recipes[0],
            "requiredCheckRefs": [recipes[0]],
            "declarationRef": f"mock-semantic/{coverage_id}",
        })
    document["batchId"] = batch_id
    document["batchSequence"] = batch_sequence
    document["batchCoverageWorkItemIds"] = batch_coverage
    document["proposals"] = proposals
    return document


def _current_focus(context: dict[str, Any]) -> tuple[str, list[str], list[dict[str, Any]]]:
    focus = _object(context, "focus", "Focus")
    if not isinstance(focus, dict):
        raise MockContractError("C3 current focus is missing")
    focus_id = _object(focus, "id", "ID")
    coverage = _unique(_strings(_object(focus, "coverageWorkItemIds", "CoverageWorkItemIDs")))
    bindings = _object(focus, "coverageEvidenceBindings", "CoverageEvidenceBindings")
    if not isinstance(focus_id, str) or not focus_id or not coverage or not isinstance(bindings, list):
        raise MockContractError("C3 current focus is incomplete")
    return focus_id, coverage, [item for item in bindings if isinstance(item, dict)]


def _operation_contract(authority: dict[str, Any], kind: str) -> dict[str, Any]:
    output = _object(authority, "outputContract", "OutputContract")
    contracts = _object(output, "operationContracts", "OperationContracts")
    if not isinstance(contracts, list):
        raise MockContractError("C3 output contract has no operation contracts")
    for contract in contracts:
        if isinstance(contract, dict) and _object(contract, "kind", "Kind") == kind:
            return contract
    raise MockContractError(f"C3 output contract does not authorize {kind!r}")


def _mcp_recipe_from_sealed_binding(
    contract: dict[str, Any], bindings: list[dict[str, Any]], expected_server_id: str
) -> tuple[list[str], str, str]:
    binding_refs = _unique(_strings(_object(contract, "allowedBindingRefs", "AllowedBindingRefs")))
    if not binding_refs:
        raise MockContractError("sealed MCP operation has no binding refs")
    by_id = {
        _object(binding, "id", "ID"): binding
        for binding in bindings
        if isinstance(_object(binding, "id", "ID"), str) and _object(binding, "id", "ID")
    }
    recipes: list[str] = []
    for binding_ref in binding_refs:
        binding = by_id.get(binding_ref)
        if not isinstance(binding, dict):
            raise MockContractError(f"sealed MCP binding {binding_ref!r} is absent from C3 focus")
        proofs = _object(binding, "proofs", "Proofs")
        if not isinstance(proofs, list):
            raise MockContractError(f"sealed MCP binding {binding_ref!r} has no proof")
        matching = [
            _object(proof, "validationRecipeId", "ValidationRecipeID")
            for proof in proofs
            if isinstance(proof, dict)
            and _object(proof, "evidenceKind", "EvidenceKind") == "validation"
            and "mcp/tool_call" in _strings(_object(proof, "allowedOperationKinds", "AllowedOperationKinds"))
        ]
        if len(matching) != 1 or not isinstance(matching[0], str) or not matching[0]:
            raise MockContractError(f"sealed MCP binding {binding_ref!r} has no exact MCP validation recipe")
        recipes.append(matching[0])
    recipes = _unique(recipes)
    if len(recipes) != 1:
        raise MockContractError("sealed MCP operation has ambiguous validation recipe identities")
    prefix = "mcp/tool_call/"
    recipe = recipes[0]
    if not recipe.startswith(prefix):
        raise MockContractError(f"sealed MCP validation recipe {recipe!r} has invalid identity")
    server_and_tool = recipe[len(prefix):]
    server_id, separator, tool = server_and_tool.partition("/")
    if not separator or not server_id or not tool:
        raise MockContractError(f"sealed MCP validation recipe {recipe!r} is incomplete")
    if expected_server_id and server_id != expected_server_id:
        raise MockContractError(f"sealed MCP server {server_id!r} does not match verifier server")
    target = _object(contract, "target", "Target")
    selectors = _strings(_object(target, "ownerSelectorRefs", "OwnerSelectorRefs"))
    if selectors and server_id not in selectors:
        raise MockContractError("sealed MCP target selector does not authorize recipe server")
    return binding_refs, server_id, tool


def _current_evidence_refs(context: dict[str, Any], execution_tail: str, coverage: list[str]) -> list[str]:
    allowed = _unique(_strings(_object(context, "allowedAcceptedEvidenceRefs", "AllowedEvidenceRefs")))
    if allowed:
        return allowed
    if not execution_tail:
        return []
    try:
        tail = json.loads(execution_tail)
    except json.JSONDecodeError as error:
        raise MockContractError(f"invalid C4.execution-tail JSON: {error}") from error
    refs: list[str] = []
    expected_coverage = set(coverage)
    for evidence in _object(tail, "latestAcceptedEvidence", "AcceptedEvidence") or []:
        if not isinstance(evidence, dict):
            continue
        evidence_coverage = set(_strings(_object(evidence, "coverageWorkItemIds", "CoverageWorkItemIDs")))
        if expected_coverage and not expected_coverage.intersection(evidence_coverage):
            continue
        refs.extend(_strings(_object(evidence, "evidenceRefs", "EvidenceRefs")))
    return _unique(refs)


def decision_payload(user_content: str, expected_server_id: str) -> dict[str, Any]:
    authority, context, execution_tail = _current_authority(user_content)
    required_state = _object(context, "requiredState", "RequiredState")
    focus_id, coverage_ids, bindings = _current_focus(context)
    if required_state == "need_validation":
        contract = _operation_contract(authority, "mcp/tool_call")
        binding_refs, server_id, tool = _mcp_recipe_from_sealed_binding(contract, bindings, expected_server_id)
        return {
            "operations": [{
                "id": "mcp-stdio-call",
                "kind": "mcp/tool_call",
                "bindingRefs": binding_refs,
                "parameters": [
                    {"name": "serverId", "string": server_id},
                    {"name": "tool", "string": tool},
                    {"name": "input.path", "string": "."},
                ],
            }]
        }
    if required_state == "need_assessment":
        refs = _current_evidence_refs(context, execution_tail, coverage_ids)
        if not refs:
            raise MockContractError("C3/C4 has no current binding-matched evidence for assessment")
        return {
            "focusId": focus_id,
            "coverageWorkItemIds": coverage_ids,
            "disposition": "satisfied",
            "items": [
                {"coverageWorkItemId": coverage_id, "satisfied": True, "evidenceRefs": refs[:1]}
                for coverage_id in coverage_ids
            ],
            "summary": "MCP stdio invocation returned owner-attested tool evidence.",
            "evidenceRefs": refs[:1],
        }
    raise MockContractError(f"C3 current authority has unsupported requiredState {required_state!r}")


def _prompt(*sections: tuple[str, dict[str, Any]]) -> str:
    return "\n\n".join(f"### {name}\n{json.dumps(value, separators=(',', ':'))}" for name, value in sections)


def _authority(state: str, binding_refs: list[str] | None = None) -> dict[str, Any]:
    recipe = "mcp/tool_call/mcp-test/repo.summary"
    return {
        "outputContract": {"operationContracts": [{
            "kind": "mcp/tool_call", "allowedBindingRefs": binding_refs or ["binding/cwi-test"],
            "target": {"ownerSelectorRefs": ["mcp-test"]},
        }]},
        "context": {
            "workspaceId": "ws-test", "runId": "run-test", "requiredState": state,
            "focus": {"id": "focus-test", "coverageWorkItemIds": ["cwi-test"], "coverageEvidenceBindings": [{
                "id": "binding/cwi-test", "proofs": [{
                    "evidenceKind": "validation", "allowedOperationKinds": ["mcp/tool_call"],
                    "validationRecipeId": recipe,
                }],
            }]},
        },
    }


def self_test() -> None:
    order_manifest = {
        "Hash": "manifest-order", "DefinitionSeedHash": "seed-test",
        "CoverageItems": [{"ID": "cwi-first"}, {"ID": "cwi-test"}],
    }
    order = semantic_plan_payload(_prompt(("C3.semantic-authority", {
        "Manifest": order_manifest, "DefinitionSeed": {"Hash": "seed-test"},
        "Schema": {"properties": {"coverageOrder": {"items": {"enum": ["cwi-first", "cwi-test"]}}}},
        "PlanStep": "coverage_order",
    })), "mcp-test")
    if order.get("coverageOrder") != ["cwi-first", "cwi-test"] or order.get("proposals") != []:
        raise AssertionError(f"MCP semantic order was not bounded: {order}")

    semantic_manifest = {
        "Hash": "manifest-test", "DefinitionSeedHash": "seed-test", "CoverageItems": [{"ID": "cwi-test"}],
        "OwnerCatalogs": [{
            "OwnerModuleID": "blocks/mcp", "SelectorRefs": ["mcp-test"],
            "ValidationRecipeRefs": ["mcp/tool_call/mcp-test/repo.summary"],
        }],
    }
    semantic = semantic_plan_payload(_prompt(("C3.semantic-authority", {
        "Manifest": semantic_manifest, "DefinitionSeed": {"Hash": "seed-test"},
        "Schema": {"properties": {"coverageOrder": {"items": {"enum": ["cwi-first", "cwi-test"]}}}},
        "PlanStep": "proof_batch", "BatchID": "batch-test", "BatchSequence": 1,
        "BatchCoverageWorkItemIDs": ["cwi-test"],
    })), "mcp-test")
    if semantic.get("batchCoverageWorkItemIds") != ["cwi-test"] or semantic["proposals"][0].get("validationRecipeId") != "mcp/tool_call/mcp-test/repo.summary":
        raise AssertionError(f"MCP semantic plan lost owner recipe identity: {semantic}")

    validation = decision_payload(_prompt(("C3.runtime-authority", _authority("need_validation"))), "mcp-test")
    operation = validation["operations"][0]
    if operation.get("bindingRefs") != ["binding/cwi-test"] or "coverageWorkItemIds" in operation:
        raise AssertionError(f"MCP operation did not use C3 binding-only authorization: {validation}")
    if operation.get("targetRefs") is not None or operation["parameters"][0].get("string") != "mcp-test":
        raise AssertionError(f"MCP operation did not preserve selector-only target contract: {validation}")

    assessment = decision_payload(
        _prompt(
            ("C3.runtime-authority", _authority("need_assessment")),
            ("C4.execution-tail", {"latestAcceptedEvidence": [{"coverageWorkItemIds": ["cwi-test"], "evidenceRefs": ["workorch://mcp"]}]}),
        ),
        "mcp-test",
    )
    if assessment["coverageWorkItemIds"] != ["cwi-test"]:
        raise AssertionError(f"assessment lost current coverage context: {assessment}")

    missing_binding = _authority("need_validation", ["binding/missing"])
    malformed = _prompt(("C3.runtime-authority", missing_binding))
    for content in ("", malformed):
        try:
            decision_payload(content, "mcp-test")
        except MockContractError:
            continue
        raise AssertionError(f"invalid C3 contract was accepted: {content}")


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
