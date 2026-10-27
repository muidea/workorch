#!/usr/bin/env python3
"""Isolated status-only transport fixture, never a real SkillHub readiness test."""
import json
import os
from pathlib import Path
import sys


def main():
    args = sys.argv[1:]
    allowed = (["status", "--json"], ["status", "--json", "--global"],
               ["status", "--global", "--json", "--agent", "codex"])
    root = Path(os.environ["WORKORCH_STEP04_FIXTURE_ROOT"]).resolve(strict=True)
    directory = Path.cwd().resolve(strict=True)
    if args not in allowed or not directory.is_relative_to(root):
        raise RuntimeError("fixture refuses non-status or non-isolated work")
    with (root / "skillhub-calls.jsonl").open("a", encoding="utf-8") as log:
        log.write(json.dumps({"args": args, "cwd": str(directory)}) + "\n")
    global_scope = "--global" in args
    print(json.dumps({"scope": "global" if global_scope else "project",
                      "project_path": "" if global_scope else str(directory),
                      "skill_count": 0, "items": [], "orphaned": [], "agents": []}))


if __name__ == "__main__":
    main()
