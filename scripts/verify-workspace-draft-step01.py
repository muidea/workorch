#!/usr/bin/env python3
"""STEP01 only: isolated production processes, HTTP draft create/read/replay.

No PUT, compile, confirm, activation, model request or deletion is issued.
Browser refresh/reopen and late-response interaction still require Web evidence.
"""
import argparse
import copy
import hashlib
import http.client
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def workspace_id(request_id):
    return "ws-" + hashlib.sha256(b"workspace-authoring-create/v1\x00" + request_id.encode()).hexdigest()


class Verification:
    def __init__(self, binary, root):
        self.binary, self.root = binary, root
        self.data = root / "data"
        self.process = None
        self.pids, self.trace, self.checked = [], [], []
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            self.port = listener.getsockname()[1]
        self.base = f"http://127.0.0.1:{self.port}"
        self.config = root / "config"
        self.config.mkdir()
        # Deliberately no live config/credentials. No provider is needed by this slice.
        (self.config / "application.toml").write_text(
            f'endpointName = "workorch"\n[server]\naddr = "127.0.0.1:{self.port}"\n'
            f'[repository]\nbackend = "file"\ndataDir = {json.dumps(str(self.data))}\n'
            '[llm.aiProxy]\nbaseURL = "http://127.0.0.1:9"\napiKey = ""\n'
            '[llm.defaultChat]\nproviderID = "ai-proxy"\nmodelID = "step01-no-model"\n', encoding="utf-8")
        self.client = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def start(self):
        log = open(self.root / f"process-{len(self.pids)+1}.log", "wb")
        try:
            self.process = subprocess.Popen([str(self.binary), "--server", "-config-dir", str(self.config)], cwd=self.root, stdout=log, stderr=subprocess.STDOUT)
        finally:
            log.close()
        self.pids.append(self.process.pid)
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            require(self.process.poll() is None, "production service exited; see original process log")
            try:
                with self.client.open(self.base + "/readyz", timeout=1) as response:
                    if response.status == 200:
                        return
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(0.1)
        raise RuntimeError("production service readiness timeout")

    def stop(self):
        if self.process and self.process.poll() is None:
            self.process.send_signal(signal.SIGTERM)
            self.process.wait(timeout=15)
            require(self.process.returncode == 0, "service did not stop cleanly")

    def request(self, method, path, body=None, expected=200, headers=None):
        raw = None if body is None else json.dumps(body, ensure_ascii=False).encode()
        request = urllib.request.Request(self.base + path, raw, {"Content-Type": "application/json", **(headers or {})}, method=method)
        try:
            response = self.client.open(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data, status = response.read(), response.status
        number = len(self.trace)+1
        path_bytes = self.root / f"http-{number:03d}-response.bin"
        path_bytes.write_bytes(data)
        if raw is not None:
            (self.root / f"http-{number:03d}-request.bin").write_bytes(raw)
        self.trace.append({"method": method, "path": path, "status": status, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest(), "response": path_bytes.name})
        require(status == expected, f"{method} {path}: expected {expected}, got {status}; see {path_bytes}")
        if status == 403:
            return {}  # Boundary rejection is plain text; raw bytes remain in evidence.
        return json.loads(data)

    def create(self, payload, expected=201):
        return self.request("POST", "/api/v1/workorch/workspace-authoring", payload, expected)

    def read(self, workspace):
        return self.request("GET", f"/api/v1/workorch/workspaces/{workspace}/authoring")["authoring"]

    def assert_draft(self, view, payload):
        require(view["workspaceId"] == workspace_id(payload["requestId"]), "different creation identity")
        require(view["state"] == "draft" and view["inputRevision"] == 1 and view["inputHash"], "not an independently saved M1 draft")
        require(view["creation"]["requestId"] == payload["requestId"] and view["creation"]["directoryPrepared"] is True, "creation preparation incomplete")
        normalized = {"goalText": payload["input"]["goalText"].strip()}
        for key in ("resourceRefs", "acceptanceRequirements"):
            if payload["input"].get(key):
                normalized[key] = [value.strip() for value in payload["input"][key]]
        require(view["input"] == normalized, "saved input content/order changed")
        require(not view.get("proposal") and not view.get("startupInputHash") and not view.get("confirmationPhase"), "draft unexpectedly compiled/sealed")
        self.checked.append(view["workspaceId"])

    def run(self):
        self.start()
        payload = {"requestId": "step01-normal", "title": "草稿 · 原始名称", "templateId": "software-dev", "input": {"goalText": "  原目标\n第二行：保留内容及顺序  ", "resourceRefs": ["b.txt", "a.txt"], "acceptanceRequirements": ["要求二", "要求一"]}}
        first = self.create(payload)["authoring"]
        self.assert_draft(first, payload)
        require(first["deliverableDirectory"] == str(self.root / ".workorch-workspaces" / first["workspaceId"]), "blank directory resolved incorrectly")
        require(self.read(first["workspaceId"]) == first, "independent read differs")
        require(self.create(payload)["authoring"] == first, "duplicate changed draft/revision")
        conflict = copy.deepcopy(payload)
        conflict["input"]["goalText"] = "不能覆盖"
        self.create(conflict, 409)
        require(self.read(first["workspaceId"]) == first, "conflict changed original")
        for label, change in {
            "missing-title": {"title": ""}, "invalid-template": {"templateId": "does-not-exist"},
            "long-goal": {"input": {"goalText": "x" * (32768+1)}},
            "missing-goal": {"input": {"goalText": ""}}, "escaped-directory": {"deliverableDirectory": "../outside"},
        }.items():
            invalid = {**payload, "requestId": label, **change}
            self.create(invalid, 409 if label == "invalid-template" else 400)
        project = self.root / "existing-project"
        project.mkdir()
        sentinel = project / "sentinel.txt"
        sentinel.write_bytes("用户业务文件\r\n原文".encode())
        explicit = {**payload, "requestId": "step01-explicit", "deliverableDirectory": str(project)}
        self.assert_draft(self.create(explicit)["authoring"], explicit)

        # A08: obstacle before any owner metadata write; unknown is not 'saved'.
        failed = {**payload, "requestId": "step01-prewrite"}
        obstacle = self.data / "internal/workspaces" / workspace_id(failed["requestId"]) / "workspace.json"
        obstacle.mkdir(parents=True)
        failure = self.create(failed, 503)
        require(failure["creationRecovery"]["persistence"] == "unknown" and failure["creationRecovery"]["phase"] == "owner_persistence", "prewrite failure fabricated persisted state")
        require(not obstacle.is_file(), "prewrite failure created metadata")
        obstacle.rename(obstacle.with_name("prewrite-obstacle-preserved"))
        self.assert_draft(self.create(failed)["authoring"], failed)

        # A09: persist first, directory fails. GET/list cannot prepare it.
        blocked = self.root / "directory-obstacle"
        blocked.write_bytes(b"test-owned obstacle; preserve")
        partial = {**payload, "requestId": "step01-partial", "deliverableDirectory": str(blocked / "project")}
        failure = self.create(partial, 503)
        require(failure["creationRecovery"]["persistence"] == "persisted", "directory failure lost saved fact")
        observed = self.read(workspace_id(partial["requestId"]))
        require(observed["state"] == "draft" and not observed["creation"]["directoryPrepared"], "GET marked directory complete")
        self.stop()
        self.start()
        restored = self.read(first["workspaceId"])
        require(restored == first and self.pids[-1] != self.pids[-2], "real process restart changed draft")
        self.assert_draft(restored, payload)
        observed = self.read(observed["workspaceId"])
        require(not observed["creation"]["directoryPrepared"], "restart fabricated directory completion")
        blocked.rename(blocked.with_name("directory-obstacle-preserved"))
        self.assert_draft(self.create(observed["creation"]["replay"])["authoring"], partial)

        # A10: a real HTTP connection drops the success body, not an owner receipt fixture.
        lost = {**payload, "requestId": "step01-lost-response"}
        connection = http.client.HTTPConnection("127.0.0.1", self.port, timeout=15)
        raw = json.dumps(lost, ensure_ascii=False).encode()
        (self.root / "lost-response-original-request.bin").write_bytes(raw)
        connection.request("POST", "/api/v1/workorch/workspace-authoring", raw, {"Content-Type": "application/json"})
        response = connection.getresponse()
        require(response.status == 201, "response-drop setup did not save")
        connection.close()  # intentionally never read the body
        self.assert_draft(self.create(lost)["authoring"], lost)  # still-open page
        deadline = time.monotonic() + 5
        while True:
            listed = self.request("GET", "/api/v1/workorch/workspaces")["workspaces"]
            discovered = [item for item in listed if item["id"] == workspace_id(lost["requestId"])]
            if discovered or time.monotonic() >= deadline:
                break
            time.sleep(0.1)  # Existing projection is eventual; this is not an owner receipt.
        require(len(discovered) == 1, "server discovery missing/duplicate draft")
        recovered = self.read(discovered[0]["id"])
        self.assert_draft(self.create(recovered["creation"]["replay"])["authoring"], lost)
        count = len(listed)
        for method, path, body in [("POST", "/api/v1/workorch/workspace-authoring", {**payload, "requestId": "cross-origin-rejected"}), ("GET", "/api/v1/workorch/workspaces", None), ("GET", f'/api/v1/workorch/workspaces/{first["workspaceId"]}/authoring', None)]:
            self.request(method, path, body, 403, {"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site"})
        require(len(self.request("GET", "/api/v1/workorch/workspaces")["workspaces"]) == count, "denied access modified business state")
        require(sentinel.read_bytes() == "用户业务文件\r\n原文".encode() and list(project.iterdir()) == [sentinel], "business files changed")
        require(self.request("GET", "/api/v1/workorch/runs")["runs"] == [], "implicit Run created")
        for workspace in set(self.checked):
            persisted = json.loads((self.data / "internal/workspaces" / workspace / "workspace.json").read_bytes())
            metadata = persisted[workspace]
            # Inspect the actual owner record, not just a 2xx response.
            record = metadata.get("definitionAuthoring", metadata.get("DefinitionAuthoring"))
            require(isinstance(record, dict), "missing persisted authoring record")
            for key in ("derivationRequests", "DerivationRequests", "specificationRequests", "SpecificationRequests"):
                require(not record.get(key), "creation dispatched model/specification work")
        self.stop()

    def save_evidence(self, failure=None):
        source = Path(__file__).resolve().parent.parent
        head = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
        diff = subprocess.check_output(["git", "-C", str(source), "diff", "--binary", "HEAD"])
        (self.root / "candidate.patch").write_bytes(diff)
        paths = subprocess.check_output(["git", "-C", str(source), "diff", "--name-only", "HEAD", "-z"]).split(b"\x00") + subprocess.check_output(["git", "-C", str(source), "ls-files", "--others", "--exclude-standard", "-z"]).split(b"\x00")
        manifest = {os.fsdecode(path): hashlib.sha256((source / os.fsdecode(path)).read_bytes()).hexdigest() for path in set(paths) if path and (source / os.fsdecode(path)).is_file()}
        (self.root / "candidate-files.json").write_text(json.dumps({"baseSHA": head, "dirtyCandidate": bool(diff or manifest), "patchSHA256": hashlib.sha256(diff).hexdigest(), "files": manifest}, ensure_ascii=False, indent=2), encoding="utf-8")
        (self.root / "http-trace.json").write_text(json.dumps(self.trace, ensure_ascii=False, indent=2), encoding="utf-8")
        (self.root / "result.json").write_text(json.dumps({"binary": str(self.binary), "binarySHA256": hashlib.sha256(self.binary.read_bytes()).hexdigest(), "processIDs": self.pids, "workspaceIDs": sorted(set(self.checked)), "failure": failure, "scope": "STEP01 production HTTP/process only; browser cases require separate evidence"}, ensure_ascii=False, indent=2), encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    args = parser.parse_args()
    require(args.binary.resolve().is_file(), "build a production binary first")
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix="workorch-step01-"))
    verification = Verification(args.binary.resolve(), root)
    try:
        verification.run()
    except Exception as error:
        verification.save_evidence(str(error))
        raise
    finally:
        verification.stop()
        print(f"STEP01 evidence (retained, restricted): {root}", flush=True)
    verification.save_evidence()
    print("PASS: STEP01 HTTP create/read/replay and real process restart; Web interaction not asserted")


if __name__ == "__main__":
    main()
