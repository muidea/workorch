#!/usr/bin/env python3
"""生成并校验本地源码、控制台产物及二进制字节身份。

该工具只校验本地输入与产物字节的一致性；不证明构建因果关系、部署来源、acceptance 或可信 attestation。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
import tempfile
from typing import Any

SCHEMA_VERSION = "workorch.build-identity/v1"
FIELDS = frozenset({"schemaVersion", "sourceCommit", "buildInputSha256", "consoleTreeSha256", "binarySha256"})
SNAPSHOT_FIELDS = frozenset({"schemaVersion", "sourceCommit", "buildInputSha256", "consoleTreeSha256"})
IDENTITY_FIELDS = FIELDS - {"schemaVersion"}
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
COMMIT_RE = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")

# 构建输入范围明确如下：Go源码包含 application、internal、vendor（包括其中测试与嵌入文件）；
# 同时包含 module/workspace 文件、Dockerfile/Makefile 及 web/consoleassets 源码。
# 浏览器构建输入是完整 web/workorch-console 目录，仅排除依赖安装与语言缓存目录。
# 本仓库顶层 docs/tests/plans、运行时配置、日志、CI产物、workspace及agent状态不扫描。
# 此身份仅用于本地源码与控制台字节一致性，不证明工具链或环境输入的因果构建来源。
# console_root 必须为 Go 实际嵌入的 web/consoleassets/dist；可选隔离 Vite 输出需与该目录逐字节一致。
SOURCE_DIRS = ("application", "internal", "vendor", "web/workorch-console", "web/consoleassets")
SOURCE_FILES = (
    "Makefile", "Dockerfile", "go.mod", "go.sum", "go.work", "go.work.sum",
    "scripts/build_identity.py",
    "scripts/verify-workspace-draft-step01.py",
    "scripts/verify-workspace-candidate-step02.py",
)
GENERATED_EXACT = ("web/consoleassets/dist",)
EXCLUDED_DIR_NAMES = {
    ".git", ".claude", ".codex", ".ci-artifacts", ".workorch-data",
    ".workorch-workspaces", ".workorch-e2e", "node_modules", "__pycache__",
    ".pytest_cache", ".mypy_cache", ".ruff_cache", ".tox",
}


class IdentityError(ValueError):
    pass


def _path_abs(path: Path) -> Path:
    if ".." in path.parts:
        raise IdentityError("构建身份路径不允许 parent traversal")
    return path.absolute()


def _walk_error(error: OSError) -> None:
    raise IdentityError(f"构建输入树无法完整枚举：{error}") from error


def _reject_symlink_components(path: Path) -> None:
    absolute = _path_abs(path)
    current = Path(absolute.anchor)
    for component in absolute.parts[1:]:
        current /= component
        try:
            mode = current.lstat().st_mode
        except FileNotFoundError:
            continue
        except OSError as exc:
            raise IdentityError(f"cannot inspect path component {current}: {exc}") from exc
        if stat.S_ISLNK(mode):
            raise IdentityError(f"symlink is not allowed: {current}")


def _inside(path: Path, parent: Path) -> bool:
    try:
        path.relative_to(parent)
        return True
    except ValueError:
        return False


def _checked_root(path: Path, label: str) -> Path:
    raw = _path_abs(path)
    _reject_symlink_components(raw)
    try:
        resolved = raw.resolve(strict=True)
    except OSError as exc:
        raise IdentityError(f"{label} does not exist: {raw}: {exc}") from exc
    if not resolved.is_dir():
        raise IdentityError(f"{label} is not a directory: {resolved}")
    return resolved


def _regular_file(path: Path) -> os.stat_result:
    _reject_symlink_components(path)
    try:
        info = path.lstat()
    except OSError as exc:
        raise IdentityError(f"cannot stat file {path}: {exc}") from exc
    if not stat.S_ISREG(info.st_mode):
        raise IdentityError(f"expected regular file: {path}")
    return info


def _read_stable_bytes(path: Path) -> bytes:
    before = _regular_file(path)
    try:
        with path.open("rb") as stream:
            opened = os.fstat(stream.fileno())
            if not stat.S_ISREG(opened.st_mode) or (opened.st_dev, opened.st_ino) != (before.st_dev, before.st_ino):
                raise IdentityError(f"打开文件时身份已变化：{path}")
            chunks: list[bytes] = []
            byte_count = 0
            while True:
                chunk = stream.read(1024 * 1024)
                if not chunk:
                    break
                chunks.append(chunk)
                byte_count += len(chunk)
            after_fd = os.fstat(stream.fileno())
        after_path = _regular_file(path)
    except IdentityError:
        raise
    except OSError as exc:
        raise IdentityError(f"读取文件失败 {path}：{exc}") from exc
    stable = lambda item: (item.st_dev, item.st_ino, item.st_size, item.st_mtime_ns, item.st_ctime_ns)
    if stable(before) != stable(after_fd) or stable(before) != stable(after_path) or byte_count != after_fd.st_size:
        raise IdentityError(f"读取期间文件发生变化：{path}")
    return b"".join(chunks)


def _hash_file(path: Path) -> tuple[str, int]:
    before = _regular_file(path)
    digest = hashlib.sha256()
    byte_count = 0
    try:
        with path.open("rb") as stream:
            opened = os.fstat(stream.fileno())
            if not stat.S_ISREG(opened.st_mode) or (opened.st_dev, opened.st_ino) != (before.st_dev, before.st_ino):
                raise IdentityError(f"file identity changed while opening: {path}")
            while True:
                chunk = stream.read(1024 * 1024)
                if not chunk:
                    break
                digest.update(chunk)
                byte_count += len(chunk)
            after_fd = os.fstat(stream.fileno())
        after_path = _regular_file(path)
    except IdentityError:
        raise
    except OSError as exc:
        raise IdentityError(f"cannot read file {path}: {exc}") from exc
    stable = lambda item: (item.st_dev, item.st_ino, item.st_size, item.st_mtime_ns, item.st_ctime_ns)
    if stable(before) != stable(after_fd) or stable(before) != stable(after_path) or byte_count != after_fd.st_size:
        raise IdentityError(f"file changed while hashing: {path}")
    return digest.hexdigest(), byte_count


def _commit(root: Path) -> str:
    try:
        result = subprocess.run(
            ["git", "-C", str(root), "rev-parse", "--verify", "HEAD"],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, encoding="utf-8", errors="strict",
        )
    except (OSError, subprocess.CalledProcessError, UnicodeError) as exc:
        raise IdentityError(f"cannot resolve source commit from fixed root {root}: {exc}") from exc
    commit = result.stdout.strip()
    if not COMMIT_RE.fullmatch(commit):
        raise IdentityError("Git returned an invalid full source commit")
    return commit


def _tracked_paths(root: Path, pathspecs: list[str]) -> set[str]:
    try:
        result = subprocess.run(
            ["git", "-C", str(root), "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", *pathspecs],
            check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
        )
    except (OSError, subprocess.CalledProcessError) as exc:
        raise IdentityError(f"cannot enumerate tracked build inputs: {exc}") from exc
    paths: set[str] = set()
    for raw in result.stdout.split(b"\0"):
        if not raw:
            continue
        try:
            rel = raw.decode("utf-8", errors="strict")
        except UnicodeDecodeError as exc:
            raise IdentityError("tracked build-input path is not valid UTF-8") from exc
        _validate_relative_path(rel)
        paths.add(rel)
    return paths


def _validate_relative_path(rel: str) -> None:
    posix = PurePosixPath(rel)
    if not rel or posix.is_absolute() or ".." in posix.parts or "\\" in rel:
        raise IdentityError(f"unsafe build-input path: {rel!r}")
    rel.encode("utf-8", errors="strict")


def _ignored_relative(rel: str) -> bool:
    parts = PurePosixPath(rel).parts
    if any(part in EXCLUDED_DIR_NAMES for part in parts):
        return True
    if any(part.startswith(".env") for part in parts):
        return True
    # 误放在源码目录的运行配置也不读取；其他静态 TOML 构建配置仍在清单中。
    if parts[-1] in {"application.toml", "application.override.toml"}:
        return True
    if any(rel == path or rel.startswith(path + "/") for path in GENERATED_EXACT):
        return True
    return False


def _allowed_source_path(rel: str) -> bool:
    if rel in SOURCE_FILES:
        return True
    return any(rel == directory or rel.startswith(directory + "/") for directory in SOURCE_DIRS)


def _source_inventory(root: Path) -> tuple[list[str], set[str]]:
    specs = [*SOURCE_DIRS, *SOURCE_FILES]
    tracked = _tracked_paths(root, specs)
    found: set[str] = set()

    # 只遍历显式源码根；排除目录先裁剪，不检查其链接或内容。
    for rel_dir in SOURCE_DIRS:
        directory = root / rel_dir
        _reject_symlink_components(directory)
        if not directory.exists():
            continue
        if not directory.is_dir():
            raise IdentityError(f"declared build-input directory is not a directory: {directory}")
        for current, dirs, files in os.walk(directory, topdown=True, followlinks=False, onerror=_walk_error):
            current_path = Path(current)
            for name in list(dirs):
                child = current_path / name
                rel = child.relative_to(root).as_posix()
                _validate_relative_path(rel)
                if name in EXCLUDED_DIR_NAMES or _ignored_relative(rel):
                    dirs.remove(name)
                    continue
                if child.is_symlink():
                    raise IdentityError(f"symlink in declared build-input tree: {child}")
            for name in files:
                child = current_path / name
                rel = child.relative_to(root).as_posix()
                _validate_relative_path(rel)
                if not _allowed_source_path(rel) or _ignored_relative(rel):
                    continue
                if child.is_symlink():
                    raise IdentityError(f"symlink in declared build-input tree: {child}")
                found.add(rel)
    for rel in SOURCE_FILES:
        path = root / rel
        if path.exists() and not _ignored_relative(rel):
            if path.is_symlink():
                raise IdentityError(f"symlink in declared build-input tree: {path}")
            found.add(rel)
    # 已删除的受控路径以 tombstone 入 hash，不读取缺失内容。
    included = {rel for rel in tracked if _allowed_source_path(rel) and not _ignored_relative(rel)}
    all_paths = found | included
    return sorted(all_paths, key=lambda value: value.encode("utf-8")), found


def _hash_source_tree(root: Path) -> str:
    paths, present = _source_inventory(root)
    digest = hashlib.sha256(b"workorch-build-input-allowlist-v2\0")
    for rel in paths:
        name = rel.encode("utf-8")
        digest.update(len(name).to_bytes(8, "big"))
        digest.update(name)
        if rel not in present:
            digest.update(b"A")  # 已删除受控输入的缺失标记
            continue
        content_hash, size = _hash_file(root / rel)
        digest.update(b"P")
        digest.update(size.to_bytes(8, "big"))
        digest.update(bytes.fromhex(content_hash))
    return digest.hexdigest()


def _tree_inventory(root: Path) -> list[str]:
    _reject_symlink_components(root)
    if not root.is_dir():
        raise IdentityError(f"树目录不存在或不是目录：{root}")
    paths: list[str] = []
    for current, dirs, files in os.walk(root, topdown=True, followlinks=False, onerror=_walk_error):
        current_path = Path(current)
        for name in dirs:
            child = current_path / name
            if child.is_symlink():
                raise IdentityError(f"产物树不允许符号链接：{child}")
        dirs.sort(key=lambda value: value.encode("utf-8"))
        for name in files:
            child = current_path / name
            rel = child.relative_to(root).as_posix()
            _validate_relative_path(rel)
            if child.is_symlink():
                raise IdentityError(f"产物树不允许符号链接：{child}")
            _regular_file(child)
            paths.append(rel)
    return sorted(paths, key=lambda value: value.encode("utf-8"))


def _hash_tree(root: Path) -> str:
    digest = hashlib.sha256(b"workorch-tree-bytes-v1\0")
    for rel in _tree_inventory(root):
        name = rel.encode("utf-8")
        file_hash, size = _hash_file(root / rel)
        digest.update(len(name).to_bytes(8, "big"))
        digest.update(name)
        digest.update(size.to_bytes(8, "big"))
        digest.update(bytes.fromhex(file_hash))
    return digest.hexdigest()


def capture(root: Path, console_root: Path, vite_output_root: Path | None = None) -> dict[str, str]:
    # Go仅嵌入 web/consoleassets/dist；外部输出仅可作字节相等性比较。
    raw_root = _path_abs(root)
    raw_console = _path_abs(console_root)
    _reject_symlink_components(raw_root)
    _reject_symlink_components(raw_console)
    root = _checked_root(raw_root, "源码仓库根目录")
    console_root = _checked_root(raw_console, "控制台产物目录")
    embedded_console = _checked_root(root / "web/consoleassets/dist", "Go嵌入控制台目录")
    if console_root != embedded_console:
        raise IdentityError("二进制控制台身份必须匹配Go实际嵌入目录 web/consoleassets/dist")
    if vite_output_root is not None:
        vite_output = _checked_root(vite_output_root, "隔离Vite输出目录")
        if _tree_inventory(vite_output) != _tree_inventory(embedded_console):
            raise IdentityError("Go实际嵌入目录的路径集合与隔离Vite输出不一致")
        if _hash_tree(vite_output) != _hash_tree(embedded_console):
            raise IdentityError("Go实际嵌入目录字节与隔离Vite输出不一致")
    return {
        "sourceCommit": _commit(root),
        "buildInputSha256": _hash_source_tree(root),
        "consoleTreeSha256": _hash_tree(console_root),
    }


def _validate(payload: Any) -> dict[str, str]:
    if not isinstance(payload, dict) or set(payload) != FIELDS:
        missing = sorted(FIELDS - set(payload)) if isinstance(payload, dict) else sorted(FIELDS)
        extra = sorted(set(payload) - FIELDS) if isinstance(payload, dict) else []
        raise IdentityError(f"构建身份字段不匹配；缺少={missing}，多余={extra}")
    if payload["schemaVersion"] != SCHEMA_VERSION:
        raise IdentityError("不支持此构建身份schemaVersion")
    if not isinstance(payload["sourceCommit"], str) or not COMMIT_RE.fullmatch(payload["sourceCommit"]):
        raise IdentityError("sourceCommit必须是40或64位小写十六进制提交标识")
    for field in IDENTITY_FIELDS - {"sourceCommit"}:
        if not isinstance(payload[field], str) or not SHA256_RE.fullmatch(payload[field]):
            raise IdentityError(f"{field}必须是64位小写SHA-256摘要")
    return payload


def _decode_manifest(path: Path) -> dict[str, str]:
    _regular_file(path)
    try:
        raw = _read_stable_bytes(path)
        text = raw.decode("utf-8", errors="strict")
        payload = json.loads(text, object_pairs_hook=_no_duplicate_keys, parse_constant=_reject_constant)
    except IdentityError:
        raise
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise IdentityError(f"UTF-8 JSON读取失败 {path}：{exc}") from exc
    return payload


def _no_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise IdentityError(f"duplicate JSON key: {key!r}")
        result[key] = value
    return result


def _reject_constant(value: str) -> None:
    raise IdentityError(f"non-finite JSON number is not allowed: {value}")


def _expected_identity(value: Any) -> dict[str, str]:
    if not isinstance(value, dict) or set(value) != IDENTITY_FIELDS:
        raise IdentityError("外部固定身份必须且只能包含sourceCommit及三个SHA-256摘要")
    return _validate({"schemaVersion": SCHEMA_VERSION, **value})


def _read_expected_identity(path: Path) -> dict[str, str]:
    try:
        raw = _read_stable_bytes(path)
        value = json.loads(
            raw.decode("utf-8", errors="strict"),
            object_pairs_hook=_no_duplicate_keys,
            parse_constant=_reject_constant,
        )
    except IdentityError:
        raise
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise IdentityError(f"外部固定身份JSON读取失败：{exc}") from exc
    return _expected_identity(value)


def _manifest_path(path: Path, artifact_root: Path) -> Path:
    raw = _path_abs(path)
    _reject_symlink_components(raw.parent)
    parent = raw.parent.resolve(strict=True)
    target = parent / raw.name
    if not _inside(target, artifact_root):
        raise IdentityError("manifest escapes explicit artifact root")
    return target


def _create_manifest(payload: dict[str, str], path: Path, artifact_root: Path) -> None:
    target = _manifest_path(path, artifact_root)
    if target.exists() or target.is_symlink():
        raise IdentityError(f"manifest already exists; refusing overwrite: {target}")
    temp_name: str | None = None
    try:
        fd, temp_name = tempfile.mkstemp(prefix=f".{target.name}.", dir=target.parent)
        with os.fdopen(fd, "wb") as stream:
            stream.write((json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8"))
            stream.flush()
            os.fsync(stream.fileno())
        # 同目录 hard link 原子发布完整文件；其他 writer 已创建目标时拒绝，绝不覆盖。
        os.link(temp_name, target, follow_symlinks=False)
        os.unlink(temp_name)
        temp_name = None
        directory_fd = os.open(target.parent, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    except FileExistsError as exc:
        raise IdentityError(f"manifest already exists; refusing overwrite: {target}") from exc
    except OSError as exc:
        raise IdentityError(f"cannot atomically publish identity manifest {target}: {exc}") from exc
    finally:
        if temp_name and os.path.exists(temp_name):
            os.unlink(temp_name)


def produce(root: Path, console_root: Path, binary: Path, manifest: Path, artifact_root: Path,
            pre_build_snapshot: Path, vite_output_root: Path | None = None) -> dict[str, str]:
    root = _checked_root(root, "source root")
    artifact_root = _checked_root(artifact_root, "artifact root")
    snapshot_path = _path_abs(pre_build_snapshot)
    _reject_symlink_components(snapshot_path)
    snapshot_info = _regular_file(snapshot_path)
    del snapshot_info
    if not _inside(snapshot_path, artifact_root):
        raise IdentityError("pre-build snapshot escapes explicit artifact root")
    try:
        snapshot_raw = _read_stable_bytes(snapshot_path)
        snapshot_text = snapshot_raw.decode("utf-8", errors="strict")
        snapshot_payload = json.loads(
            snapshot_text,
            object_pairs_hook=_no_duplicate_keys,
            parse_constant=_reject_constant,
        )
    except IdentityError:
        raise
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise IdentityError(f"构建前快照UTF-8 JSON读取失败：{exc}") from exc
    if not isinstance(snapshot_payload, dict) or set(snapshot_payload) != SNAPSHOT_FIELDS:
        missing = sorted(SNAPSHOT_FIELDS - set(snapshot_payload)) if isinstance(snapshot_payload, dict) else sorted(SNAPSHOT_FIELDS)
        extra = sorted(set(snapshot_payload) - SNAPSHOT_FIELDS) if isinstance(snapshot_payload, dict) else []
        raise IdentityError(f"构建前快照字段不匹配；缺少={missing}，多余={extra}")
    if snapshot_payload["schemaVersion"] != SCHEMA_VERSION:
        raise IdentityError("不支持此构建前快照schemaVersion")
    before = {key: snapshot_payload[key] for key in SNAPSHOT_FIELDS}
    if not isinstance(before["sourceCommit"], str) or not COMMIT_RE.fullmatch(before["sourceCommit"]):
        raise IdentityError("构建前快照sourceCommit格式无效")
    for field in ("buildInputSha256", "consoleTreeSha256"):
        if not isinstance(before[field], str) or not SHA256_RE.fullmatch(before[field]):
            raise IdentityError(f"构建前快照{field}格式无效")
    current = capture(root, console_root, vite_output_root)
    for field in current:
        if before[field] != current[field]:
            raise IdentityError(f"构建输入在构建前后发生变化：{field}")
    binary = _path_abs(binary)
    if not _inside(binary, artifact_root):
        raise IdentityError("binary escapes explicit artifact root")
    binary_hash, _ = _hash_file(binary)
    payload = {"schemaVersion": SCHEMA_VERSION, **current, "binarySha256": binary_hash}
    _validate(payload)
    manifest_path = _manifest_path(manifest, artifact_root)
    _create_manifest(payload, manifest_path, artifact_root)
    return payload


def verify(root: Path, console_root: Path, binary: Path, manifest: Path,
           expected_identity: dict[str, str], artifact_root: Path) -> dict[str, str]:
    root = _checked_root(root, "source root")
    artifact_root = _checked_root(artifact_root, "artifact root")
    expected = _expected_identity(expected_identity)
    manifest_path = _manifest_path(manifest, artifact_root)
    payload = _decode_manifest(manifest_path)
    _validate(payload)
    if payload != {"schemaVersion": SCHEMA_VERSION, **expected}:
        raise IdentityError("构建清单与外部固定的完整身份不一致")
    binary = _path_abs(binary)
    if not _inside(binary, artifact_root):
        raise IdentityError("binary escapes explicit artifact root")
    current = capture(root, console_root)
    for field, value in current.items():
        if payload[field] != value:
            raise IdentityError(f"构建清单{field}与当前源码/控制台字节不一致")
    binary_hash, _ = _hash_file(binary)
    if payload["binarySha256"] != binary_hash:
        raise IdentityError("构建清单binarySha256与当前二进制字节不一致")
    return payload


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for command in ("capture", "produce", "verify"):
        item = sub.add_parser(command, help={"capture": "捕获构建前源码与内嵌控制台字节身份", "produce": "校验构建前后输入并写入新身份清单", "verify": "按外部固定的完整身份校验本地字节"}[command])
        item.add_argument("--root", type=Path, required=True, help="源码仓库固定根目录")
        item.add_argument("--console-root", type=Path, required=True, help="Go实际嵌入的控制台产物目录")
        if command != "capture":
            item.add_argument("--binary", type=Path, required=True, help="构建产物二进制")
            item.add_argument("--manifest", type=Path, required=True, help="新建或校验的身份清单路径")
            item.add_argument("--artifact-root", type=Path, required=True, help="二进制和清单所在隔离产物根目录")
        if command == "capture":
            item.add_argument("--vite-output-root", type=Path, help="可选隔离Vite输出；必须与Go嵌入目录字节一致")
        if command == "produce":
            item.add_argument("--pre-build-snapshot", type=Path, required=True, help="构建前保存的源码/内嵌控制台快照")
            item.add_argument("--vite-output-root", type=Path, help="与Go实际嵌入dist比较；此目录本身不声明为二进制嵌入来源")
        if command == "verify":
            item.add_argument("--expected-identity-json", type=Path, required=True,
                              help="外部固定的完整 source/build-input/console/binary 身份 JSON；不从当前检出推断")
    args = parser.parse_args(argv)
    try:
        if args.command == "capture":
            result = {"schemaVersion": SCHEMA_VERSION, **capture(args.root, args.console_root, args.vite_output_root)}
        elif args.command == "produce":
            result = produce(args.root, args.console_root, args.binary, args.manifest, args.artifact_root, args.pre_build_snapshot, args.vite_output_root)
        else:
            expected = _read_expected_identity(args.expected_identity_json)
            result = verify(args.root, args.console_root, args.binary, args.manifest, expected, args.artifact_root)
    except IdentityError as exc:
        print(f"构建身份{args.command}失败：{exc}", file=sys.stderr)
        return 2
    except (OSError, UnicodeError, ValueError, TypeError, KeyError) as exc:
        print(f"构建身份{args.command}输入无效：{exc}", file=sys.stderr)
        return 2
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
