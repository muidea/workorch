#!/usr/bin/env python3
"""使用临时合成仓库测试字节身份清单；不构建或连接服务。"""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).with_name("build_identity.py")
spec = importlib.util.spec_from_file_location("build_identity", SCRIPT)
assert spec and spec.loader
build_identity = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build_identity)


class BuildIdentityTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="workorch-build-identity-test-")
        self.addCleanup(self.temp.cleanup)
        base = Path(self.temp.name)
        self.root = base / "repo"
        self.root.mkdir()
        self.artifacts = base / "artifacts"
        self.artifacts.mkdir()
        self.console = self.root / "web/consoleassets/dist"
        self.console.mkdir(parents=True)
        self.binary = self.artifacts / "workorch"
        self.binary.write_bytes(b"synthetic executable bytes\x00")
        self.snapshot = self.artifacts / "pre-build-snapshot.json"
        self.manifest = self.artifacts / "build-identity.json"
        self._git("init", "-q")
        self._git("config", "user.email", "build-identity-test@example.invalid")
        self._git("config", "user.name", "Build Identity Test")
        self._write("go.mod", "module example.invalid/workorch\n")
        self._write("go.sum", "synthetic checksum\n")
        self._write("go.work", "go 1.26.7\n")
        self._write("Makefile", "build: console-build\n")
        self._write("application/main.go", "package main\n")
        self._write("internal/embeddata/defaults.json", "{}\n")
        self._write("internal/tests/fixture.txt", "test data consumed by go test")
        self._write("vendor/example.invalid/dependency.go", "package dependency\n")
        self._write("web/workorch-console/package.json", "{}\n")
        self._write("web/workorch-console/package-lock.json", "{}\n")
        self._write("web/workorch-console/vite.config.ts", "export {}\n")
        self._write("web/workorch-console/src/main.ts", "export {}\n")
        self._write("web/consoleassets/assets.go", "package consoleassets\n")
        self._write("web/consoleassets/embed_helper.go", "package consoleassets\n")
        (self.console / "index.html").write_bytes(b"<html>synthetic</html>\n")
        self._git("add", "go.mod", "go.sum", "go.work", "Makefile", "application", "internal", "vendor", "web")
        self._git("commit", "-qm", "synthetic source")

    def _git(self, *args: str) -> str:
        result = subprocess.run(["git", "-C", str(self.root), *args], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        return result.stdout.strip()

    def _write(self, rel: str, content: str) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")

    def _capture_to(self, target: Path | None = None) -> dict[str, str]:
        snapshot = {"schemaVersion": build_identity.SCHEMA_VERSION, **build_identity.capture(self.root, self.console)}
        destination = target or self.snapshot
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(json.dumps(snapshot, sort_keys=True), encoding="utf-8")
        return snapshot

    def _produce(self, snapshot: Path | None = None) -> dict[str, str]:
        return build_identity.produce(
            self.root, self.console, self.binary, self.manifest, self.artifacts,
            snapshot or self.snapshot,
        )

    def _expected(self, payload: dict[str, str]) -> dict[str, str]:
        return {key: payload[key] for key in build_identity.IDENTITY_FIELDS}

    def _verify(self, payload: dict[str, str] | None = None) -> dict[str, str]:
        expected = self._expected(payload) if payload else self.expected_identity
        return build_identity.verify(self.root, self.console, self.binary, self.manifest, expected, self.artifacts)

    def test_pre_capture_produce_and_complete_external_pin_verify(self) -> None:
        before = self._capture_to()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        self.assertEqual(payload["sourceCommit"], self._git("rev-parse", "HEAD"))
        self.assertEqual(payload["binarySha256"], hashlib.sha256(self.binary.read_bytes()).hexdigest())
        self.assertEqual(payload["buildInputSha256"], before["buildInputSha256"])
        self.assertEqual(self._verify(), payload)

    def test_allowlisted_tracked_dirty_and_untracked_source_change_hash(self) -> None:
        original = build_identity.capture(self.root, self.console)["buildInputSha256"]
        self._write("application/main.go", "package main // tracked edit\n")
        tracked = build_identity.capture(self.root, self.console)["buildInputSha256"]
        self.assertNotEqual(tracked, original)
        self._write("internal/new_source.go", "package internal\n")
        untracked = build_identity.capture(self.root, self.console)["buildInputSha256"]
        self.assertNotEqual(untracked, tracked)

    def test_tracked_deletion_is_hashed_as_absence(self) -> None:
        before = build_identity.capture(self.root, self.console)["buildInputSha256"]
        (self.root / "application/main.go").unlink()
        after = build_identity.capture(self.root, self.console)["buildInputSha256"]
        self.assertNotEqual(after, before)

    def test_only_declared_inputs_are_enumerated_and_runtime_secrets_are_untouched(self) -> None:
        original = build_identity.capture(self.root, self.console)["buildInputSha256"]
        self._write("config/application.toml", "must not read")
        self._write("logs/provider.log", "must not read")
        self._write("coverage/coverage.out", "must not read")
        self._write(".tmp/output", "must not read")
        self._write(".env.live", "must not read")
        self._write("scripts/arbitrary.py", "not in declared build graph")
        self._write("docs/private.md", "not in declared build graph")
        self._write("internal/.pytest_cache/v/cache", "must not read")
        self._write("web/workorch-console/node_modules/pkg/index.js", "must not read")
        external = Path(self.temp.name) / "external-node-modules"
        external.mkdir()
        shutil.rmtree(self.root / "web/workorch-console/node_modules")
        (self.root / "web/workorch-console/node_modules").symlink_to(external, target_is_directory=True)
        self.assertEqual(build_identity.capture(self.root, self.console)["buildInputSha256"], original)

    def test_source_and_console_mutation_after_prebuild_capture_rejected(self) -> None:
        self._capture_to()
        self._write("application/main.go", "package main // changed after capture\n")
        with self.assertRaisesRegex(build_identity.IdentityError, "构建输入在构建前后发生变化"):
            self._produce()
        self._write("application/main.go", "package main\n")
        self._capture_to()
        (self.console / "index.html").write_bytes(b"changed console")
        with self.assertRaisesRegex(build_identity.IdentityError, "构建输入在构建前后发生变化"):
            self._produce()

    def test_console_and_binary_mutations_are_detected_and_external_pin_is_complete(self) -> None:
        self._capture_to()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        (self.console / "index.html").write_bytes(b"different console")
        with self.assertRaisesRegex(build_identity.IdentityError, "consoleTreeSha256与当前"):
            self._verify()
        self._capture_to()
        self.manifest.unlink()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        self.binary.write_bytes(b"different binary")
        with self.assertRaisesRegex(build_identity.IdentityError, "外部固定的完整身份不一致"):
            # A rewritten local manifest cannot change the externally pinned expected binary hash.
            changed = dict(self.expected_identity, binarySha256=hashlib.sha256(self.binary.read_bytes()).hexdigest())
            forged = {"schemaVersion": build_identity.SCHEMA_VERSION, **changed}
            self.manifest.write_text(json.dumps(forged), encoding="utf-8")
            self._verify()

    def test_expected_identity_requires_all_four_values_and_commit_is_external(self) -> None:
        self._capture_to()
        payload = self._produce()
        incomplete = {"sourceCommit": payload["sourceCommit"]}
        with self.assertRaisesRegex(build_identity.IdentityError, "必须且只能包含sourceCommit"):
            build_identity.verify(self.root, self.console, self.binary, self.manifest, incomplete, self.artifacts)
        wrong = self._expected(payload)
        wrong["sourceCommit"] = "0" * len(payload["sourceCommit"])
        with self.assertRaisesRegex(build_identity.IdentityError, "外部固定的完整身份不一致"):
            self._verify(wrong)

    def test_producer_refuses_existing_manifest(self) -> None:
        self._capture_to()
        self._produce()
        with self.assertRaisesRegex(build_identity.IdentityError, "refusing overwrite"):
            self._produce()

    def test_expected_identity_file_rejects_extra_and_missing_fields(self) -> None:
        self._capture_to()
        payload = self._produce()
        expected = self._expected(payload)
        path = self.artifacts / "expected-identity.json"
        path.write_text(json.dumps(dict(expected, unexpected="x")), encoding="utf-8")
        with self.assertRaisesRegex(build_identity.IdentityError, "只能包含sourceCommit"):
            build_identity._read_expected_identity(path)
        path.write_text(json.dumps({"sourceCommit": expected["sourceCommit"]}), encoding="utf-8")
        with self.assertRaisesRegex(build_identity.IdentityError, "只能包含sourceCommit"):
            build_identity._read_expected_identity(path)

    def test_snapshot_and_manifest_json_parse_the_same_stable_bytes(self) -> None:
        self._capture_to()
        with mock.patch.object(Path, "read_text", side_effect=AssertionError("不得二次读取元数据")), \
             mock.patch.object(Path, "read_bytes", side_effect=AssertionError("不得二次读取元数据")):
            payload = self._produce()
            self.expected_identity = self._expected(payload)
            self.assertEqual(self._verify(), payload)

        for width in (40, 64):
            self.assertRegex("a" * width, build_identity.COMMIT_RE)
        for width in (39, 41, 63, 65):
            self.assertNotRegex("a" * width, build_identity.COMMIT_RE)

    def test_strict_manifest_duplicate_utf8_nonfinite_fields_schema_and_hashes(self) -> None:
        self._capture_to()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        encoded = json.dumps(payload, sort_keys=True)
        self.manifest.write_text(encoded[:-1] + ',"schemaVersion":"workorch.build-identity/v1"}', encoding="utf-8")
        with self.assertRaisesRegex(build_identity.IdentityError, "duplicate JSON key"):
            self._verify()
        self.manifest.write_bytes(b"\xff")
        with self.assertRaisesRegex(build_identity.IdentityError, "UTF-8"):
            self._verify()
        self.manifest.write_text('{"nan":NaN}', encoding="utf-8")
        with self.assertRaisesRegex(build_identity.IdentityError, "non-finite"):
            self._verify()
        variants = [
            (dict(payload, extra="x"), "构建身份字段不匹配"),
            ({key: value for key, value in payload.items() if key != "binarySha256"}, "构建身份字段不匹配"),
            (dict(payload, schemaVersion="workorch.build-identity/v9"), "不支持此构建身份schemaVersion"),
            (dict(payload, binarySha256="sha256:" + "0" * 64), "必须是64位小写SHA-256摘要"),
        ]
        for value, error in variants:
            with self.subTest(error=error):
                self.manifest.write_text(json.dumps(value), encoding="utf-8")
                with self.assertRaisesRegex(build_identity.IdentityError, error):
                    self._verify()

    def test_root_console_source_and_binary_symlinks_fail_closed(self) -> None:
        self._capture_to()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        root_link = Path(self.temp.name) / "repo-link"
        root_link.symlink_to(self.root, target_is_directory=True)
        with self.assertRaisesRegex(build_identity.IdentityError, "symlink"):
            build_identity.capture(root_link, self.console)
        console_link = Path(self.temp.name) / "console-link"
        console_link.symlink_to(self.console, target_is_directory=True)
        with self.assertRaisesRegex(build_identity.IdentityError, "symlink"):
            build_identity.capture(self.root, console_link)
        source_link = self.root / "internal/link.go"
        source_link.symlink_to(self.root / "application/main.go")
        with self.assertRaisesRegex(build_identity.IdentityError, "symlink"):
            build_identity.capture(self.root, self.console)
        source_link.unlink()
        binary_link = self.artifacts / "linked-binary"
        binary_link.symlink_to(self.binary)
        with self.assertRaisesRegex(build_identity.IdentityError, "symlink"):
            build_identity.verify(self.root, self.console, binary_link, self.manifest, self.expected_identity, self.artifacts)

    def test_vite_output_is_an_independent_tree_comparison_not_binary_identity(self) -> None:
        self._write("web/consoleassets/dist/index.html", "same")
        vite = Path(self.temp.name) / "isolated-vite-output"
        vite.mkdir()
        (vite / "index.html").write_text("same", encoding="utf-8")
        identity = build_identity.capture(self.root, self.console, vite)
        self.assertEqual(identity["consoleTreeSha256"], build_identity._hash_tree(self.console))
        (vite / "tmp").mkdir()
        (vite / "tmp/generated.js").write_text("included output bytes", encoding="utf-8")
        with self.assertRaisesRegex(build_identity.IdentityError, "路径集合"):
            build_identity.capture(self.root, self.console, vite)
        (vite / "tmp").rename(vite / "cache")
        (vite / "cache").rename(vite / "tmp")
        import os
        os.rename(vite / "tmp", vite / "dist")
        # A generated-looking directory is still included in the output byte tree.
        with self.assertRaisesRegex(build_identity.IdentityError, "路径集合"):
            build_identity.capture(self.root, self.console, vite)

    def test_raw_parent_traversal_cannot_hide_symlink_component(self) -> None:
        link = Path(self.temp.name) / "root-link"
        link.symlink_to(self.root, target_is_directory=True)
        disguised = link / ".." / "repo"
        with self.assertRaisesRegex(build_identity.IdentityError, "parent traversal"):
            build_identity.capture(disguised, self.console)
        with self.assertRaisesRegex(build_identity.IdentityError, "parent traversal"):
            build_identity.capture(self.root, self.console / ".." / "dist")

    def test_walk_errors_fail_closed_in_source_and_output_inventory(self) -> None:
        original = build_identity.os.walk

        def failing_walk(path, **kwargs):
            self.assertIs(kwargs["onerror"], build_identity._walk_error)
            kwargs["onerror"](PermissionError("synthetic unreadable tree"))
            yield from original(path, **kwargs)

        with mock.patch.object(build_identity.os, "walk", side_effect=failing_walk):
            with self.assertRaisesRegex(build_identity.IdentityError, "无法完整枚举"):
                build_identity.capture(self.root, self.console)
            with self.assertRaisesRegex(build_identity.IdentityError, "无法完整枚举"):
                build_identity._hash_tree(self.console)

    def test_manifest_tree_hash_does_not_skip_output_named_tmp_or_dist(self) -> None:
        (self.console / "tmp").mkdir()
        (self.console / "tmp/data").write_text("embedded bytes", encoding="utf-8")
        (self.console / "dist").mkdir()
        (self.console / "dist/data").write_text("more embedded bytes", encoding="utf-8")
        tree_hash = build_identity._hash_tree(self.console)
        (self.console / "tmp/data").write_text("changed embedded bytes", encoding="utf-8")
        self.assertNotEqual(tree_hash, build_identity._hash_tree(self.console))

        self._capture_to()
        payload = self._produce()
        self.expected_identity = self._expected(payload)
        outside = Path(self.temp.name) / "outside"
        outside.mkdir()
        self.assertEqual(build_identity.capture(self.root, self.console)["sourceCommit"], self._git("rev-parse", "HEAD"))
        arbitrary_console = Path(self.temp.name) / "outside-console"
        arbitrary_console.mkdir()
        with self.assertRaisesRegex(build_identity.IdentityError, "匹配Go实际嵌入目录"):
            build_identity.capture(self.root, arbitrary_console)
        with self.assertRaisesRegex(build_identity.IdentityError, "escapes explicit artifact root"):
            build_identity.verify(self.root, self.console, outside / "binary", self.manifest, self.expected_identity, self.artifacts)
        self.binary.unlink()
        self.binary.mkdir()
        with self.assertRaisesRegex(build_identity.IdentityError, "regular file"):
            build_identity.verify(self.root, self.console, self.binary, self.manifest, self.expected_identity, self.artifacts)


if __name__ == "__main__":
    unittest.main(verbosity=2)
