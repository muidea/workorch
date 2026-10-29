#!/usr/bin/env python3
"""Isolated register/query/status transport fixture; never a readiness proof."""
from datetime import datetime, timezone
import fcntl
import json
import os
from pathlib import Path
import re
import sys


def write_new(path, raw):
    with path.open("xb") as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())


def registration(args, root, home):
    query = len(args) == 6 and args[:3] == ["project", "register", "--query"]
    if len(args) != 6 or args[:2] != ["project", "register"] or args[3] != "--request-id" or args[5] != "--json" or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}", args[4]):
        raise RuntimeError("fixture refuses unknown registration command")
    directory = None if query else Path(args[2])
    if directory is not None and (not directory.is_absolute() or str(directory.resolve(strict=True)) != args[2] or not directory.is_relative_to(root) or directory == root):
        raise RuntimeError("fixture registration escaped exact isolated project")
    receipts = home / "project-registrations"
    receipts.mkdir(exist_ok=True)
    path = receipts / (args[4] + ".json")
    if path.exists():
        raw = path.read_bytes()
        if directory is not None and json.loads(raw)["project_path"] != str(directory):
            raise RuntimeError("fixture request scope conflict")
    elif query:
        # Missing is not proof that an operation was never executed.
        print(json.dumps({"error": {"code": "REQUEST_NOT_FOUND", "message": "original receipt unavailable"}}))
        return 1
    else:
        now = datetime.now(timezone.utc).isoformat()
        raw = json.dumps({"request_id": args[4], "project_path": str(directory), "status": "succeeded",
                          "executed": True, "finished": True, "started_at": now, "completed_at": now,
                          "result": {"registered": True, "project_created": False, "created_directories": [], "skill_count": 0}}).encode()
        write_new(path, raw)
    marker = root / "dropped-registration-reply.bin"
    if not query and os.environ.get("WORKORCH_STEP04_DROP_REGISTRATION_REPLY") == "1" and not marker.exists():
        # Model transport loss after a durable receipt. Preserve original bytes.
        write_new(marker, raw)
    else:
        sys.stdout.buffer.write(raw)
    return 0


def main():
    args = sys.argv[1:]
    allowed = (["status", "--json"], ["status", "--json", "--global"],
               ["status", "--global", "--json", "--agent", "codex"])
    root = Path(os.environ["WORKORCH_STEP04_FIXTURE_ROOT"]).resolve(strict=True)
    directory = Path.cwd().resolve(strict=True)
    home = Path(os.environ["SKILL_HUB_HOME"])
    if not home.is_absolute() or home.resolve(strict=True) != home or not home.is_relative_to(root) or not directory.is_relative_to(root):
        raise RuntimeError("fixture refuses non-isolated scope")
    if args not in allowed and not (len(args) == 6 and args[:2] == ["project", "register"] and args[3] == "--request-id" and args[5] == "--json"):
        raise RuntimeError("fixture refuses unsupported command")
    with (home / "fixture.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        # Invalid registration scope is rejected before logging or writing.
        if args[:2] == ["project", "register"]:
            if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}", args[4]):
                raise RuntimeError("fixture refuses invalid request identity")
            if args[2] != "--query":
                target = Path(args[2])
                if not target.is_absolute() or str(target.resolve(strict=True)) != args[2] or not target.is_relative_to(root) or target == root:
                    raise RuntimeError("fixture registration escaped exact isolated project")
        with (root / "skillhub-calls.jsonl").open("a", encoding="utf-8") as log:
            log.write(json.dumps({"args": args, "cwd": str(directory)}) + "\n")
        if args[:2] == ["project", "register"]:
            return registration(args, root, home)
        global_scope = "--global" in args
        if not global_scope and not any(json.loads(p.read_bytes()).get("project_path") == str(directory)
                                        for p in (home / "project-registrations").glob("*.json")):
            print(json.dumps({"error": {"code": "PROJECT_NOT_FOUND", "message": "project not registered"}}))
            return 1
        print(json.dumps({"scope": "global" if global_scope else "project",
                          "project_path": "" if global_scope else str(directory),
                          "skill_count": 0, "items": [], "orphaned": [], "agents": []}))
    return 0


if __name__ == "__main__":
    os.umask(0o077)
    sys.exit(main())
