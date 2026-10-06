#!/usr/bin/env python3
"""LSP-Bridge PRD 证据 fail-closed 校验器；只读取报告，不执行其中命令。"""
from __future__ import annotations

import argparse
import json
import math
from pathlib import Path
import re
import sys
from typing import Any

import verify_deployed_convergence_manifest as dce

SCHEMA_VERSION = "lsp-bridge.prd-evidence/v1"
HASH_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")
BINDING = {"workspaceId", "runId", "startupIdentity", "deliveryCommit", "serviceSourceIdentity"}
TOP_KEYS = BINDING | {"schemaVersion", "producerCoverage", "requirements", "artifacts"}
ARTIFACT_KEYS = BINDING | {"ref", "kind", "path", "sha256"}
REQUIREMENT_KEYS = {"id", "status", "evidence"}
EVIDENCE_KEYS = BINDING | {"checkId", "command", "exitCode", "result", "execution", "artifactRefs", "facts"}
KINDS = {"command_output", "test_report", "benchmark_report", "source_snapshot", "documentation_index"}

# 每项要求固定事实字段，命令退出码和自报状态不能替代这些事实。
FACT_FIELDS = {
    "LSP-01": {"tools"}, "LSP-02": {"definitions"}, "LSP-03": {"unsaved"},
    "LSP-04": {"diagnostic"}, "LSP-05": {"boundedOutputs"}, "LSP-06": {"status"},
    "LSP-07": {"shutdown"}, "LSP-08": {"repair"},
    "LSP-09": {"definitionLatency", "hoverLatency"}, "LSP-10": {"rss", "languages"},
    "LSP-11": {"configuration"}, "LSP-12": {"restart"}, "LSP-13": {"lifecycle"},
    "LSP-14": {"pathUri"}, "LSP-15": {"concurrency", "race"},
    "LSP-16": {"build", "test", "raceTest"}, "LSP-17": {"realGoLsp"},
    "LSP-18": {"configExample"}, "LSP-19": {"documents"}, "LSP-20": {"reproducibleDelivery"},
}
TOOLS = {"lsp_initialize", "lsp_sync", "lsp_discard_unsaved", "lsp_definition", "lsp_hover", "lsp_diagnostics", "lsp_references", "lsp_status", "lsp_shutdown", "lsp_repair"}
LANGUAGES = {"Go", "Rust", "Python", "TypeScript", "Shell"}


class EvidenceError(ValueError):
    """报告不满足严格证据合同。"""


def _obj(value: Any, keys: set[str], where: str) -> dict[str, Any]:
    if not isinstance(value, dict) or value.keys() != keys:
        missing = keys - value.keys() if isinstance(value, dict) else keys
        extra = value.keys() - keys if isinstance(value, dict) else set()
        raise EvidenceError(f"{where}: object keys mismatch missing={sorted(missing)} unknown={sorted(extra)}")
    return value


def _text(value: Any, where: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise EvidenceError(f"{where}: expected non-empty string")
    return value


def _bool(value: Any, where: str) -> bool:
    if type(value) is not bool:
        raise EvidenceError(f"{where}: expected boolean")
    return value


def _integer(value: Any, where: str, minimum: int = 1) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum:
        raise EvidenceError(f"{where}: expected integer >= {minimum}")
    return value


def _number(value: Any, where: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise EvidenceError(f"{where}: expected finite number; boolean/NaN/Infinity rejected")
    try:
        if not math.isfinite(value): raise EvidenceError(f"{where}: expected finite number; boolean/NaN/Infinity rejected")
        return float(value)
    except OverflowError as exc:
        raise EvidenceError(f"{where}: numeric overflow rejected") from exc


def _strings(value: Any, where: str, exact: set[str] | None = None) -> set[str]:
    if not isinstance(value, list) or any(not isinstance(v, str) or not v for v in value):
        raise EvidenceError(f"{where}: expected string array")
    values = set(value)
    if len(values) != len(value) or (exact is not None and values != exact):
        raise EvidenceError(f"{where}: duplicate or unexpected values")
    return values


def _enum(value: Any, allowed: set[str], where: str) -> str:
    if not isinstance(value, str) or value not in allowed:
        raise EvidenceError(f"{where}: invalid value")
    return value


def _truths(row: dict[str, Any], where: str) -> None:
    for key, value in row.items():
        if _bool(value, f"{where}.{key}") is not True:
            raise EvidenceError(f"{where}.{key} must be true")


def _facts(req_id: str, facts: Any) -> None:
    f = _obj(facts, FACT_FIELDS[req_id], f"{req_id}.facts")
    if req_id == "LSP-01":
        _strings(f["tools"], "tools", TOOLS)
    elif req_id == "LSP-02":
        rows = _obj(f["definitions"], LANGUAGES, "definitions")
        for lang, raw in rows.items():
            row = _obj(raw, {"passed", "symbol", "path", "line", "execution"}, lang)
            if row["passed"] is not True or row["execution"] != "real": raise EvidenceError(f"{lang}: require real passing definition query")
            _text(row["symbol"], f"{lang}.symbol"); _text(row["path"], f"{lang}.path"); _integer(row["line"], f"{lang}.line")
    elif req_id == "LSP-03":
        _truths(_obj(f["unsaved"], {"syncApplied", "discardRestored", "hoverLatest", "definitionLatest", "diagnosticsLatest", "unchangedDidChangeSkipped"}, "unsaved"), "unsaved")
    elif req_id == "LSP-04":
        row = _obj(f["diagnostic"], {"message", "line", "column", "severity"}, "diagnostic")
        _text(row["message"], "diagnostic.message"); _integer(row["line"], "diagnostic.line"); _integer(row["column"], "diagnostic.column")
        _enum(row["severity"], {"error", "warning", "information", "hint"}, "diagnostic.severity")
    elif req_id == "LSP-05":
        outputs = _obj(f["boundedOutputs"], {"hover", "diagnostics", "references", "overflowProbe"}, "boundedOutputs")
        for name in ("hover", "diagnostics", "references", "overflowProbe"):
            output_set = _obj(outputs[name], {"hover", "diagnostics", "references"}, f"{name}.outputs") if name == "overflowProbe" else {name: outputs[name]}
            for output_name, raw in output_set.items():
                row = _obj(raw, {"limit", "total", "returned", "truncated", "complete", "reason"}, output_name)
                limit = _integer(row["limit"], f"{output_name}.limit")
                total = _integer(row["total"], f"{output_name}.total", 0)
                returned = _integer(row["returned"], f"{output_name}.returned", 0)
                truncated = _bool(row["truncated"], f"{output_name}.truncated")
                complete = _bool(row["complete"], f"{output_name}.complete")
                _text(row["reason"], f"{output_name}.reason")
                expected_returned = min(total, limit)
                if returned != expected_returned or truncated != (total > limit) or complete != (total <= limit): raise EvidenceError(f"{output_name}: returned/total/limit completeness mismatch")
                if truncated and row["reason"] in {"", "none", "within_limit"}: raise EvidenceError(f"{output_name}: truncation requires explicit reason")
        probe = outputs["overflowProbe"]
        for output_name in ("hover", "diagnostics", "references"):
            item = probe[output_name]
            if item["total"] <= item["limit"] or not item["truncated"]: raise EvidenceError(f"overflowProbe.{output_name}: evidence must exercise total > limit")
    elif req_id == "LSP-06":
        row = _obj(f["status"], {"instanceState", "activity", "restartCount", "dependencyPresent", "dependencyRunning"}, "status")
        _enum(row["instanceState"], {"ready", "idle", "stopped"}, "status.instanceState")
        _enum(row["activity"], {"active", "idle"}, "status.activity")
        _integer(row["restartCount"], "restartCount", 0); _bool(row["dependencyPresent"], "dependencyPresent"); _bool(row["dependencyRunning"], "dependencyRunning")
    elif req_id == "LSP-07":
        _truths(_obj(f["shutdown"], {"singleClosed", "allClosed", "reinitialized"}, "shutdown"), "shutdown")
    elif req_id == "LSP-08":
        row = _obj(f["repair"], {"defaultIsAdvice", "applySafeOnly", "allowedAction"}, "repair")
        if _bool(row["defaultIsAdvice"], "repair.defaultIsAdvice") is not True or _bool(row["applySafeOnly"], "repair.applySafeOnly") is not True: raise EvidenceError("repair must default to advice and restrict apply to safe actions")
        _enum(row["allowedAction"], {"restart_exited_instance", "none"}, "repair.allowedAction")
    elif req_id == "LSP-09":
        for key in ("definitionLatency", "hoverLatency"):
            row = _obj(f[key], {"value", "unit", "warm", "scope"}, key); val = _number(row["value"], key)
            if val < 0 or val >= 50 or row["unit"] != "ms" or row["warm"] is not True or row["scope"] != "mcp_internal_excluding_lsp": raise EvidenceError(f"{key}: require warm 0<=ms<50 excluding LSP")
    elif req_id == "LSP-10":
        row = _obj(f["rss"], {"value", "unit", "scope", "simultaneous"}, "rss"); val = _number(row["value"], "rss.value")
        if val < 0 or val >= 64 or row["unit"] != "MB" or row["scope"] != "mcp_process_excluding_children" or row["simultaneous"] is not True: raise EvidenceError("RSS must be simultaneous 0<=MB<64 excluding children")
        langs = _strings(f["languages"], "languages")
        if len(langs) != 3 or not langs.issubset(LANGUAGES): raise EvidenceError("RSS test must enable three supported languages simultaneously")
    elif req_id == "LSP-11":
        row = _obj(f["configuration"], {"keys", "runtimeEffective"}, "configuration")
        _strings(row["keys"], "configuration.keys", {"defaultTimeout", "maxReferences", "maxDiagnostics", "maxHoverCharacters"})
        if row["runtimeEffective"] is not True: raise EvidenceError("configuration did not take effect")
    elif req_id == "LSP-12":
        row = _obj(f["restart"], {"trigger", "killEvidenceRef", "nextRequestRestarted", "initialized"}, "restart")
        if row["trigger"] != "manual_kill" or not isinstance(row["killEvidenceRef"], str) or not row["killEvidenceRef"] or row["nextRequestRestarted"] is not True or row["initialized"] is not True: raise EvidenceError("restart facts must prove manual child kill and next-request recovery")
    elif req_id == "LSP-13":
        _truths(_obj(f["lifecycle"], {"idleTtlReclaimed", "capacityLruReclaimed", "onlyIdleReclaimed"}, "lifecycle"), "lifecycle")
    elif req_id == "LSP-14":
        row = _obj(f["pathUri"], {"linuxPassed", "windowsClaimed", "windowsPassed"}, "pathUri")
        if row["linuxPassed"] is not True or not isinstance(row["windowsClaimed"], bool) or not isinstance(row["windowsPassed"], bool) or (row["windowsClaimed"] and not row["windowsPassed"]): raise EvidenceError("path/URI support claims lack passing evidence")
    elif req_id == "LSP-15":
        row = _obj(f["concurrency"], {"goroutines", "requestIdsMatched", "allCompleted"}, "concurrency")
        if _integer(row["goroutines"], "goroutines") != 10 or row["requestIdsMatched"] is not True or row["allCompleted"] is not True or f["race"] is not True: raise EvidenceError("10 concurrent queries or race check failed")
    elif req_id == "LSP-16":
        _strings(f["build"], "build", {"go build -buildvcs=false ./..."}); _strings(f["test"], "test", {"go test ./..."}); _strings(f["raceTest"], "raceTest", {"go test -race ./..."})
    elif req_id == "LSP-17":
        row = _obj(f["realGoLsp"], {"passed", "execution", "fixture", "server"}, "realGoLsp")
        if row["passed"] is not True or row["execution"] != "real" or row["fixture"] is not False: raise EvidenceError("requires passing real Go LSP integration")
        _text(row["server"], "realGoLsp.server")
    elif req_id == "LSP-18":
        _strings(f["configExample"], "configExample", {"runtime", "performance", "languageCommands"})
    elif req_id == "LSP-19":
        _strings(f["documents"], "documents", {"integration", "development", "performance", "dod", "systemPrompt"})
    elif req_id == "LSP-20":
        _truths(_obj(f["reproducibleDelivery"], {"sourcePresent", "testsPresent", "buildReproducible"}, "reproducibleDelivery"), "reproducibleDelivery")


def _artifact(manifest_path: Path, item: dict[str, Any]) -> None:
    _text(item["path"], "artifact.path")
    path, errors = dce._artifact_path_errors(manifest_path, item["path"])
    if errors or path is None: raise EvidenceError("; ".join(errors) or "invalid artifact path")
    actual = dce._sha256(path)
    if actual != item["sha256"]: raise EvidenceError(f"artifact digest mismatch: {item['path']}")


def validate(document: Any, manifest_path: Path, mode: str, expected: dict[str, str] | None = None) -> dict[str, Any]:
    if mode not in {"selftest", "live"}: raise EvidenceError("模式必须是 selftest 或 live")
    _reject_surrogates(document)
    doc = _obj(document, TOP_KEYS, "manifest")
    if doc["schemaVersion"] != SCHEMA_VERSION: raise EvidenceError("unknown schemaVersion")
    _enum(doc["producerCoverage"], {"unknown", "verified"}, "producerCoverage")
    for key in BINDING:
        value = _text(doc[key], key)
        if key.endswith("Identity") and key != "deliveryCommit" and not HASH_RE.fullmatch(value): raise EvidenceError(f"invalid {key}")
    if not COMMIT_RE.fullmatch(doc["deliveryCommit"]): raise EvidenceError("deliveryCommit must be full lowercase commit hash")
    if mode == "live":
        if expected is None: raise EvidenceError("live mode requires external trusted expected identities; local Git HEAD is never consulted")
        expected = _obj(expected, BINDING, "expected identities")
        for key in BINDING:
            if expected[key] != doc[key]: raise EvidenceError(f"{key} differs from explicit expected identity")
    artifacts = doc["artifacts"]
    if not isinstance(artifacts, list) or not artifacts: raise EvidenceError("artifacts must be non-empty")
    by_ref: dict[str, dict[str, Any]] = {}
    for index, raw in enumerate(artifacts):
        item = _obj(raw, ARTIFACT_KEYS, f"artifacts[{index}]"); ref = _text(item["ref"], "artifact.ref")
        if ref in by_ref: raise EvidenceError(f"duplicate artifact ref {ref}")
        kind = _text(item["kind"], f"artifact {ref}.kind")
        if kind not in KINDS: raise EvidenceError(f"invalid artifact kind {ref}")
        for key in BINDING:
            if item[key] != doc[key]: raise EvidenceError(f"artifact {ref}: {key} drift")
        if not isinstance(item["sha256"], str) or not HASH_RE.fullmatch(item["sha256"]): raise EvidenceError(f"artifact {ref}: invalid hash")
        _artifact(manifest_path, item); by_ref[ref] = item
    rows = doc["requirements"]
    if not isinstance(rows, list): raise EvidenceError("requirements must be array")
    found: set[str] = set()
    for index, raw in enumerate(rows):
        row = _obj(raw, REQUIREMENT_KEYS, f"requirements[{index}]"); req = _text(row["id"], "requirement.id")
        if req not in FACT_FIELDS: raise EvidenceError(f"unknown requirement id: {req}")
        if req in found: raise EvidenceError(f"duplicate requirement id: {req}")
        if row["status"] != "passed": raise EvidenceError(f"{req}: only passed status is accepted")
        evs = row["evidence"]
        if not isinstance(evs, list) or not evs: raise EvidenceError(f"{req}: missing evidence")
        checks: set[str] = set()
        for ei, raw_ev in enumerate(evs):
            ev = _obj(raw_ev, EVIDENCE_KEYS, f"{req}.evidence[{ei}]")
            for key in BINDING:
                if ev[key] != doc[key]: raise EvidenceError(f"{req}: {key} drift")
            if ev["checkId"] != req or ev["checkId"] in checks: raise EvidenceError(f"{req}: checkId mismatch/duplicate")
            checks.add(ev["checkId"]); _text(ev["command"], f"{req}.command")
            if isinstance(ev["exitCode"], bool) or ev["exitCode"] != 0 or not isinstance(ev["exitCode"], int) or ev["result"] != "passed": raise EvidenceError(f"{req}: command did not pass")
            execution = _text(ev["execution"], f"{req}.execution")
            if execution not in {"real", "fixture"}: raise EvidenceError(f"{req}: invalid execution kind")
            refs = ev["artifactRefs"]
            if not isinstance(refs, list) or not refs or any(not isinstance(ref, str) or not ref for ref in refs) or len(set(refs)) != len(refs): raise EvidenceError(f"{req}: artifactRefs must be unique, non-empty strings")
            kinds = set()
            for ref in refs:
                if ref not in by_ref: raise EvidenceError(f"{req}: unknown artifact {ref}")
                artifact = by_ref[ref]
                kinds.add(artifact["kind"])
            if not kinds.intersection({"command_output", "test_report", "benchmark_report"}): raise EvidenceError(f"{req}: source/docs alone cannot evidence a check")
            _facts(req, ev["facts"])
            if req == "LSP-12":
                kill_ref = ev["facts"]["restart"]["killEvidenceRef"]
                if kill_ref not in refs: raise EvidenceError("manual kill evidence ref must be listed in artifactRefs")
                kill_item = by_ref.get(kill_ref)
                if kill_item is None: raise EvidenceError("manual kill evidence ref is unknown")
                if kill_item["kind"] != "command_output": raise EvidenceError("manual kill evidence ref must identify command output")
                if any(kill_item[key] != doc[key] for key in BINDING): raise EvidenceError("manual kill evidence scope drift")
            if mode == "live" and execution != "real": raise EvidenceError(f"{req}: fixture/mock evidence rejected in live mode")
        found.add(req)
    missing = set(FACT_FIELDS) - found
    if missing: raise EvidenceError(f"missing requirement ids: {sorted(missing)}")
    result = {"mode": mode, "requirementsValidated": len(found), "artifactFilesHashed": len(by_ref), "producerCoverage": doc["producerCoverage"], "acceptanceEstablished": False}
    if mode == "selftest": result["status"] = "SYNTHETIC_SELFTEST_OK"
    else:
        result["status"] = "PENDING_UNVERIFIED"
        result["limitation"] = "No independently trusted producer is registered; typed facts and schema-valid reports are not producer attestation."
    return result


def _no_duplicate_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result: raise EvidenceError(f"duplicate JSON object key: {key}")
        result[key] = value
    return result


def _reject_surrogates(value: Any) -> None:
    if isinstance(value, str):
        if any(0xD800 <= ord(char) <= 0xDFFF for char in value): raise EvidenceError("unpaired Unicode surrogate rejected")
    elif isinstance(value, list):
        for item in value: _reject_surrogates(item)
    elif isinstance(value, dict):
        for key, item in value.items():
            _reject_surrogates(key); _reject_surrogates(item)


def _reject_constant(value: str) -> None:
    raise EvidenceError(f"invalid JSON number {value}")


def _reject_overflow(value: str) -> float:
    number = float(value)
    if not math.isfinite(number): raise EvidenceError("JSON numeric overflow rejected")
    return number


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path, help="只读授权 JSON 证据报告")
    parser.add_argument("--mode", required=True, choices=("selftest", "live"), help="selftest 为合成校验；live 只校验报告，不代表验收通过")
    parser.add_argument("--expected-identities", type=Path, help="live 必需：外部可信身份 JSON；不会读取本地 Git HEAD")
    args = parser.parse_args(argv)
    try:
        doc = json.loads(args.manifest.read_text(encoding="utf-8"), object_pairs_hook=_no_duplicate_pairs, parse_constant=_reject_constant, parse_float=_reject_overflow)
        expected = json.loads(args.expected_identities.read_text(encoding="utf-8"), object_pairs_hook=_no_duplicate_pairs, parse_constant=_reject_constant, parse_float=_reject_overflow) if args.expected_identities else None
        _reject_surrogates(doc)
        if expected is not None: _reject_surrogates(expected)
        result = validate(doc, args.manifest.resolve(), args.mode, expected)
    except (OSError, json.JSONDecodeError, ValueError, EvidenceError) as exc:
        print(json.dumps({"status": "REJECTED", "error": f"校验拒绝：{exc}"}, ensure_ascii=False), file=sys.stderr); return 1
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0 if args.mode == "selftest" else 2


if __name__ == "__main__":
    raise SystemExit(main())
