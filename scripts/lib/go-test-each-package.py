#!/usr/bin/env python3
"""校验 go test -json 的逐包实际执行，完整转发测试输出。"""

import json
import sys
from typing import TextIO


def _unique_keys(rows: list[tuple[str, object]]) -> dict[str, object]:
    result = {}
    for key, value in rows:
        if key in result:
            raise ValueError(f"重复事件字段：{key}")
        result[key] = value
    return result


def verify(stream: TextIO, output: TextIO, errors: TextIO) -> bool:
    packages: dict[str, dict[str, object]] = {}
    invalid = False
    for line in stream:
        try:
            event = json.loads(line, object_pairs_hook=_unique_keys)
            if not isinstance(event, dict):
                raise ValueError("事件不是对象")
            package, action = event.get("Package"), event.get("Action")
            if not isinstance(package, str) or not package or not isinstance(action, str) or action not in {"start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip"}:
                raise ValueError("缺少 Package 或未知 Action")
            test = event.get("Test", "")
            text = event.get("Output", "")
            if not isinstance(test, str) or not isinstance(text, str):
                raise ValueError("Test／Output 类型错误")
        except (ValueError, TypeError) as exc:
            output.write(line)
            errors.write(f"verification error: 无效 go test JSON 事件：{exc}\n")
            invalid = True
            continue
        output.write(text)
        state = packages.setdefault(package, {"started": False, "ran": set(), "passed": set(), "test_terminal": {}, "terminal": "", "failed": False})
        if action == "start":
            if test or state["started"]:
                errors.write(f"verification error: {package} 非法或重复 package start\n")
                invalid = True
            state["started"] = True
        elif not state["started"]:
            errors.write(f"verification error: {package} 在 package start 前有事件\n")
            invalid = True
        ran, passed, test_terminal = state["ran"], state["passed"], state["test_terminal"]
        assert isinstance(ran, set) and isinstance(passed, set) and isinstance(test_terminal, dict)
        if state["terminal"]:
            errors.write(f"verification error: {package} 在 package 终态后仍有事件\n")
            invalid = True
        if action == "run" and test:
            if test in ran:
                errors.write(f"verification error: {package}/{test} 重复运行身份\n")
                invalid = True
            ran.add(test)
        elif action in {"pass", "skip", "fail"} and test:
            if test not in ran or test in test_terminal:
                errors.write(f"verification error: {package}/{test} 未运行或重复 test 终态\n")
                invalid = True
            test_terminal[test] = action
            if action == "pass":
                passed.add(test)
        if action == "fail":
            state["failed"] = True
        if not test and action in {"pass", "fail", "skip"}:
            if state["terminal"]:
                errors.write(f"verification error: {package} 重复 package 终态\n")
                invalid = True
            state["terminal"] = action
    if not packages:
        errors.write("verification error: 未执行任何包的测试\n")
        return False
    for package, state in packages.items():
        test_terminal = state["test_terminal"]
        assert isinstance(test_terminal, dict)
        if not state["started"] or state["terminal"] != "pass" or state["failed"] or not state["ran"] or not state["passed"] or set(test_terminal) != state["ran"]:
            errors.write(f"verification error: {package} 未实际运行通过测试（空选择、无测试、仅 skip 或失败）\n")
            invalid = True
    return not invalid


if __name__ == "__main__":
    raise SystemExit(0 if verify(sys.stdin, sys.stdout, sys.stderr) else 1)
