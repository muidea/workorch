#!/usr/bin/env python3
"""逐包 Go 测试门禁的 synthetic 输出校准，不调用 Go。"""

import importlib.util
import io
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("each_package", Path(__file__).parent / "lib/go-test-each-package.py")
assert spec and spec.loader
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def events(package: str, action: str, test: str = "", output: str = "") -> str:
    return json.dumps({"Package": package, "Action": action, "Test": test, "Output": output}) + "\n"


def passing(package: str) -> str:
    return events(package, "start") + events(package, "run", "TestContract") + events(package, "output", "TestContract", "原始 Unicode 输出\n") + events(package, "pass", "TestContract") + events(package, "pass", output=f"ok\t{package}\n")


class EachPackageTests(unittest.TestCase):
    def check(self, raw: str) -> tuple[bool, str, str]:
        output, errors = io.StringIO(), io.StringIO()
        result = helper.verify(io.StringIO(raw), output, errors)
        return result, output.getvalue(), errors.getvalue()

    def test_each_package_must_pass_and_output_is_preserved(self) -> None:
        valid, output, errors = self.check(passing("one") + passing("two"))
        self.assertTrue(valid)
        self.assertEqual(output, "原始 Unicode 输出\nok\tone\n原始 Unicode 输出\nok\ttwo\n")
        self.assertEqual(errors, "")

    def test_empty_package_is_not_hidden_by_another_passing_package(self) -> None:
        for empty in (events("two", "pass", output="ok\ttwo [no tests to run]\n"), events("two", "skip", output="?\ttwo [no test files]\n")):
            with self.subTest(empty=empty):
                self.assertFalse(self.check(passing("one") + empty)[0])

    def test_skip_only_cannot_pass(self) -> None:
        raw = events("one", "run", "TestContract") + events("one", "skip", "TestContract") + events("one", "pass")
        self.assertFalse(self.check(raw)[0])

    def test_failed_package_and_failed_test_are_rejected(self) -> None:
        for failure in (events("one", "fail"), events("one", "fail", "TestFailure")):
            with self.subTest(failure=failure):
                self.assertFalse(self.check(passing("one") + failure)[0])

    def test_missing_terminal_duplicate_terminal_and_pass_without_run_rejected(self) -> None:
        for raw in (events("one", "run", "TestContract"), passing("one") + events("one", "pass"), events("one", "pass", "TestContract") + events("one", "pass")):
            with self.subTest(raw=raw):
                self.assertFalse(self.check(raw)[0])

    def test_empty_malformed_and_wrong_typed_events_rejected(self) -> None:
        for raw in ("", "not JSON\n", "[]\n", '{}\n', events("", "pass"), '{"Package":"one","Action":"pass","Output":null}\n'):
            with self.subTest(raw=raw):
                self.assertFalse(self.check(raw)[0])

    def test_mixed_skip_and_actual_pass_is_accepted(self) -> None:
        raw = events("one", "start") + events("one", "run", "TestOptional") + events("one", "skip", "TestOptional") + passing("one").split("\n", 1)[1]
        self.assertTrue(self.check(raw)[0])

    def test_missing_duplicate_and_test_scoped_package_start_rejected(self) -> None:
        for raw in (
            passing("one").split("\n", 1)[1],
            events("one", "start") + passing("one"),
            events("one", "start", "TestContract") + passing("one").split("\n", 1)[1],
            events("one", "output", output="before start") + passing("one"),
        ):
            with self.subTest(raw=raw):
                self.assertFalse(self.check(raw)[0])

    def test_duplicate_json_unknown_action_and_unfinished_test_rejected(self) -> None:
        cases = [
            '{"Package":"one","Action":"fail","Action":"pass"}\n',
            events("one", "unknown"),
            passing("one") + events("one", "output", output="late"),
            events("one", "run", "Unfinished") + passing("one"),
            events("one", "run", "TestContract") + passing("one"),
            events("one", "run", "TestContract") + events("one", "pass", "TestContract") + events("one", "pass", "TestContract") + events("one", "pass"),
        ]
        for raw in cases:
            with self.subTest(raw=raw):
                self.assertFalse(self.check(raw)[0])


if __name__ == "__main__":
    unittest.main()
