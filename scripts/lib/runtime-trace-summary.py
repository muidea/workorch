#!/usr/bin/env python3
"""Summarize `go tool trace -d=parsed` regions; diagnostic only, no authority.

Inclusive wall durations overlap both nested regions and concurrent goroutines.
Self wall time excludes only nested Workorch regions on that same goroutine;
it includes waits and is not CPU time or an end-to-end elapsed-time sum.
"""

import argparse
import json
import re
import sys
from collections import defaultdict
from typing import Iterable


REGION = re.compile(r'\bG=(\d+) (RegionBegin|RegionEnd) Time=(\d+) Task=(\d+) Type="(workorch\.runtime\.[a-z_]+)"$')


def summarize(lines: Iterable[str]) -> dict:
    stacks: dict[int, list] = defaultdict(list)
    totals: dict[str, dict] = {}
    previous_times: dict[int, int] = {}
    first, last = None, None
    completed = 0
    for line in lines:
        event = REGION.search(line.rstrip("\r\n"))
        if event is None:
            # Do not silently skip malformed Workorch timing records.
            if "workorch.runtime." in line and ("RegionBegin" in line or "RegionEnd" in line):
                raise ValueError("malformed Workorch region")
            continue
        goroutine, kind, timestamp, task, name = event.groups()
        goroutine, timestamp = int(goroutine), int(timestamp)
        if timestamp < previous_times.get(goroutine, timestamp):
            raise ValueError("region time moved backwards on a goroutine")
        previous_times[goroutine] = timestamp
        first = timestamp if first is None else min(first, timestamp)
        last = timestamp if last is None else max(last, timestamp)
        stack = stacks[goroutine]
        if kind == "RegionBegin":
            if stack and timestamp < stack[-1][2]:
                raise ValueError("region begins before its parent")
            stack.append([name, task, timestamp, 0])
            continue
        if not stack or stack[-1][:2] != [name, task]:
            raise ValueError("unpaired or incorrectly nested Workorch region")
        _, _, started, children = stack.pop()
        duration = timestamp - started
        if duration < children:
            raise ValueError("region duration is negative or shorter than children")
        if stack:
            stack[-1][3] += duration
        row = totals.setdefault(name, {"count": 0, "inclusive_wall_ns": 0, "self_wall_ns": 0, "max_wall_ns": 0})
        row["count"] += 1
        row["inclusive_wall_ns"] += duration
        row["self_wall_ns"] += duration - children
        row["max_wall_ns"] = max(row["max_wall_ns"], duration)
        completed += 1
    unfinished = sum(len(stack) for stack in stacks.values())
    return {
        "schemaVersion": "workorch-runtime-trace-summary/v1",
        "diagnosticOnly": True,
        "complete": completed > 0 and unfinished == 0,
        "completedRegions": completed,
        "unfinishedRegions": unfinished,
        "observedSpanNs": 0 if first is None else last - first,
        "durationMeaning": "overlapping wall time; self excludes nested Workorch regions on the same goroutine; not CPU or summed end-to-end duration",
        "regions": dict(sorted(totals.items())),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("parsed_trace", nargs="?", default="-", help="full parsed trace, or stdin")
    args = parser.parse_args()
    try:
        if args.parsed_trace == "-":
            summary = summarize(sys.stdin)
        else:
            with open(args.parsed_trace, encoding="utf-8") as source:
                summary = summarize(source)
    except (ValueError, OSError) as error:
        # Input text/paths can contain private diagnostics; do not echo them.
        print("runtime trace summary failed: " + ("invalid region sequence" if isinstance(error, ValueError) else "input unavailable"), file=sys.stderr)
        return 2
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0 if summary["complete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
