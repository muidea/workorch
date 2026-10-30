#!/usr/bin/env python3
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("runtime_trace_summary", Path(__file__).parent / "lib/runtime-trace-summary.py")
assert spec and spec.loader
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


def event(kind, time, name="commit", goroutine=1, task=0):
    return f'M=20 P=1 G={goroutine} Region{kind} Time={time} Task={task} Type="workorch.runtime.{name}"\n'


class RuntimeTraceSummaryTest(unittest.TestCase):
    def test_nested_regions_are_not_added_to_self_time(self):
        report = helper.summarize([event("Begin", 10), event("Begin", 20, "run_cas"), event("End", 50, "run_cas"), event("End", 100)])
        self.assertTrue(report["complete"])
        self.assertEqual(report["regions"]["workorch.runtime.commit"], {"count": 1, "inclusive_wall_ns": 90, "self_wall_ns": 60, "max_wall_ns": 90})
        self.assertEqual(report["observedSpanNs"], 90)

    def test_concurrent_goroutines_overlap_and_same_region_can_recurse(self):
        report = helper.summarize([event("Begin", 10), event("Begin", 20, goroutine=2), event("Begin", 30), event("End", 50), event("End", 60, goroutine=2), event("End", 80)])
        row = report["regions"]["workorch.runtime.commit"]
        self.assertEqual(row["count"], 3)
        self.assertEqual(row["inclusive_wall_ns"], 130)
        self.assertEqual(row["self_wall_ns"], 110)
        self.assertEqual(report["observedSpanNs"], 70)

    def test_unfinished_or_empty_trace_is_not_complete(self):
        for lines in ([], ["unrelated event\n"], [event("Begin", 10)], [event("Begin", 10), event("Begin", 20, "run_cas"), event("End", 30, "run_cas")]):
            self.assertFalse(helper.summarize(lines)["complete"])

    def test_mismatch_wrong_task_and_negative_time_are_rejected(self):
        for lines in (
            [event("End", 10)],
            [event("Begin", 10), event("End", 20, "run_cas")],
            [event("Begin", 10), event("End", 20, task=1)],
            [event("Begin", 20), event("End", 10)],
            [event("Begin", 20), event("Begin", 10, "run_cas")],
            [event("Begin", 10), event("Begin", 20, "run_cas"), event("End", 40, "run_cas"), event("End", 25)],
            [event("Begin", 10), event("Begin", 20, "run_cas"), event("End", 40, "run_cas"), event("Begin", 30, "run_cas"), event("End", 50, "run_cas"), event("End", 100)],
            ['G=1 RegionBegin Time=10 Task=0 Type="workorch.runtime.commit" extra\n'],
        ):
            with self.subTest(lines=lines), self.assertRaises(ValueError):
                helper.summarize(lines)

    def test_multiple_occurrences_and_unrelated_output(self):
        report = helper.summarize(["private unrelated text\n", event("Begin", 10), event("End", 20), event("Begin", 30), event("End", 50)])
        self.assertEqual(report["regions"]["workorch.runtime.commit"]["max_wall_ns"], 20)
        self.assertEqual(report["regions"]["workorch.runtime.commit"]["count"], 2)
        self.assertNotIn("private", str(report))


if __name__ == "__main__":
    unittest.main()
