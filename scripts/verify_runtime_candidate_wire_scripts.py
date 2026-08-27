#!/usr/bin/env python3
"""Reject owner-authority fields in scripted provider Runtime candidates.

The deployed verifiers contain deterministic Python providers both as normal
``.py`` files and as Python heredocs embedded in shell scripts.  This check
walks every such source under ``scripts/`` and inspects literal Runtime
operation documents.  Audit/projection fixtures with a nested ``operations``
collection are intentionally outside this provider-wire check.
"""

from __future__ import annotations

import argparse
import ast
from dataclasses import dataclass
from pathlib import Path
import re
import sys


PROVIDER_OPERATION_FIELDS = frozenset(
    {"kind", "requiredOperationRef", "bindingRefs", "targetRefs", "parameters"}
)
HEREDOC_OPEN = re.compile(r"<<-?\s*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?")
OPERATIONS_LITERAL = re.compile(r"['\"]operations['\"]")


@dataclass(frozen=True)
class PythonSource:
    label: str
    text: str
    first_line: int = 1


@dataclass(frozen=True)
class Violation:
    source: PythonSource
    line: int
    fields: tuple[str, ...]

    def render(self) -> str:
        fields = ", ".join(self.fields)
        return f"{self.source.label}:{self.line}: provider operation has forbidden fields: {fields}"


def _literal_string(node: ast.expr | None) -> str | None:
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return node.value
    return None


def _python_heredocs(path: Path) -> list[PythonSource]:
    lines = path.read_text(encoding="utf-8").splitlines()
    result: list[PythonSource] = []
    start: int | None = None
    marker = ""
    for index, line in enumerate(lines):
        if start is None:
            match = HEREDOC_OPEN.search(line)
            if match and match.group(1).upper().startswith("PY"):
                start = index + 1
                marker = match.group(1)
            continue
        if line.strip() != marker:
            continue
        text = "\n".join(lines[start:index])
        # Most shell heredocs are request/config data. Parse only Python-like
        # markers whose body can actually contain a candidate document.
        if OPERATIONS_LITERAL.search(text):
            result.append(PythonSource(str(path), text, start + 1))
        start = None
        marker = ""
    return result


def python_sources(root: Path) -> list[PythonSource]:
    result: list[PythonSource] = []
    for path in sorted(root.rglob("*")):
        if path.suffix == ".py":
            result.append(PythonSource(str(path), path.read_text(encoding="utf-8")))
        elif path.suffix == ".sh":
            result.extend(_python_heredocs(path))
    return result


def inspect_source(source: PythonSource) -> tuple[list[Violation], int]:
    try:
        tree = ast.parse(source.text, filename=source.label)
    except SyntaxError as error:
        line = source.first_line + max((error.lineno or 1) - 1, 0)
        raise ValueError(f"{source.label}:{line}: cannot parse candidate-producing Python: {error.msg}") from error

    violations: list[Violation] = []
    candidate_count = 0
    for node in ast.walk(tree):
        if not isinstance(node, ast.Dict):
            continue
        entries = {
            key: value
            for key_node, value in zip(node.keys, node.values)
            if (key := _literal_string(key_node)) is not None
        }
        operations = entries.get("operations")
        if not isinstance(operations, (ast.List, ast.Tuple)):
            continue
        operation_nodes = []
        for item in operations.elts:
            if not isinstance(item, ast.Dict):
                continue
            item_fields = {_literal_string(key) for key in item.keys}
            if "kind" in item_fields:
                operation_nodes.append((item, item_fields))
        if not operation_nodes:
            continue
        # A strict provider candidate has exactly one top-level field. Audit
        # dispatch/projection fixtures may contain nested operations plus their
        # own id/state/fence and are not provider output.
        if set(entries) != {"operations"}:
            continue
        candidate_count += 1
        for operation, fields in operation_nodes:
            forbidden = sorted(
                "<dynamic-key>" if field is None else field
                for field in fields
                if field is None or field not in PROVIDER_OPERATION_FIELDS
            )
            if forbidden:
                violations.append(
                    Violation(
                        source=source,
                        line=source.first_line + operation.lineno - 1,
                        fields=tuple(forbidden),
                    )
                )
    return violations, candidate_count


def scan(root: Path) -> tuple[list[Violation], int]:
    violations: list[Violation] = []
    count = 0
    for source in python_sources(root):
        found, source_count = inspect_source(source)
        violations.extend(found)
        count += source_count
    return violations, count


def self_test() -> None:
    valid = PythonSource("valid.py", "return_value = {'operations': [{'kind': 'file/read', 'targetRefs': ['a']}]}")
    found, count = inspect_source(valid)
    if found or count != 1:
        raise AssertionError(f"valid provider candidate result = {found}, count={count}")

    for field in ("id", "dependsOn", "coverageWorkItemIds", "executionKey", "effectScope"):
        invalid = PythonSource(
            f"invalid-{field}.py",
            "return_value = {'operations': [{'kind': 'file/read', " + repr(field) + ": 'owner-value'}]}",
        )
        found, count = inspect_source(invalid)
        if count != 1 or len(found) != 1 or found[0].fields != (field,):
            raise AssertionError(f"forbidden provider field {field!r} was not detected: {found}")

    audit = PythonSource(
        "audit.py",
        "audit = {'id': 'dispatch-1', 'state': 'committed', "
        "'operations': [{'id': 'op-1', 'kind': 'file/read', 'executionKey': 'exec-1'}]}",
    )
    found, count = inspect_source(audit)
    if found or count:
        raise AssertionError(f"owner audit fixture was misclassified as provider output: {found}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path("scripts"))
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
    try:
        violations, count = scan(args.root)
    except (OSError, ValueError) as error:
        print(f"boundary contract violation: {error}", file=sys.stderr)
        return 1
    if violations:
        print(
            "boundary contract violation: scripted LLM/mock provider emits fields outside the strict Runtime operation wire",
            file=sys.stderr,
        )
        for violation in violations:
            print(violation.render(), file=sys.stderr)
        return 1
    if count == 0:
        print(
            "boundary contract violation: no scripted provider Runtime operation documents were discovered",
            file=sys.stderr,
        )
        return 1
    if args.verbose:
        print(f"scripted provider Runtime operation wire verified: {count} candidate documents")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
