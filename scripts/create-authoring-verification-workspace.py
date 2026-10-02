#!/usr/bin/env python3
"""隔离部署验收共享客户端：持久创作／精确确认，不自动启动 Run。"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import urllib.error
import urllib.parse
import urllib.request

spec = importlib.util.spec_from_file_location('authoring_live', Path(__file__).with_name('verify-workspace-authoring-live.py'))
assert spec is not None and spec.loader is not None
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


def formal_outputs(request, workspace_id):
    path = '/api/v1/workorch/workspaces/' + urllib.parse.quote(workspace_id, safe='')
    workspace = request('GET', path)
    goals = request('GET', path + '/goals').get('goals')
    dods = request('GET', path + '/dods').get('dods')
    if not isinstance(goals, list) or not isinstance(dods, list) or len(goals) != 1 or len(dods) != 1:
        raise RuntimeError('确认后正式 Goal／DoD 身份不唯一')
    if not isinstance(workspace.get('workspace'), dict) or workspace['workspace'].get('id') != workspace_id:
        raise RuntimeError('正式 Workspace 身份不匹配')
    for definition in (goals[0], dods[0]):
        if not isinstance(definition, dict) or not isinstance(definition.get('id'), str) or not definition['id'] or definition.get('workspaceId') != workspace_id:
            raise RuntimeError('正式定义身份或作用域不匹配')
    return [('workspace', workspace), ('goal', {'goal': goals[0]}), ('dod', {'dod': dods[0]})]


def write_outputs(prefix, values):
    for suffix, value in values:
        output = Path(prefix + '-' + suffix + '.json')
        output.parent.mkdir(parents=True, exist_ok=True)
        if output.is_symlink():
            raise RuntimeError('输出文件不能是符号链接')
        if output.exists():
            if output.stat().st_mode & 0o077 or json.loads(output.read_text()) != value:
                raise RuntimeError('输出文件身份或权限不匹配，拒绝覆盖')
            continue
        fd, temporary = tempfile.mkstemp(prefix=output.name + '.', dir=output.parent)
        try:
            with os.fdopen(fd, 'w') as handle:
                json.dump(value, handle, ensure_ascii=False)
                handle.flush()
                os.fsync(handle.fileno())
            os.link(temporary, output)
        finally:
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', required=True)
    parser.add_argument('--payload-file', required=True)
    parser.add_argument('--recovery-file', required=True)
    parser.add_argument('--output-prefix', required=True)
    parser.add_argument('--provider-id')
    parser.add_argument('--model-id')
    parser.add_argument('--reasoning-effort', default='')
    parser.add_argument('--timeout', type=int, default=300)
    args = parser.parse_args()
    if not 1 <= args.timeout <= 3600:
        parser.error('timeout 必须在 1～3600 秒内')
    repo = Path(__file__).resolve().parent.parent
    recovery = Path(args.recovery_file).expanduser().resolve()
    artifacts = Path(args.output_prefix).expanduser().resolve().parent
    if recovery.is_relative_to(repo) or recovery.is_relative_to(artifacts):
        parser.error('恢复 journal 必须位于仓库及输出 artifacts 目录外')
    payload = json.loads(Path(args.payload_file).read_text())
    if not isinstance(payload, dict):
        parser.error('创作输入必须为对象')
    payload.setdefault('requestId', None)
    base = args.base_url.rstrip('/')
    if not base.startswith(('http://', 'https://')):
        parser.error('base-url 必须为 HTTP(S)')
    def request(method, path, body=None):
        encoded = None if body is None else json.dumps(body, ensure_ascii=False).encode()
        req = urllib.request.Request(base + path, data=encoded, method=method, headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(req, timeout=60) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            raise RuntimeError(f'HTTP {error.code}: {method} {path}') from None
    if bool(args.provider_id) != bool(args.model_id) or (args.reasoning_effort and not args.model_id):
        parser.error('provider-id 与 model-id 必须同时指定')
    selection = None if not args.model_id else {'providerId': args.provider_id, 'modelId': args.model_id, 'reasoningEffort': args.reasoning_effort}
    with live.RecoveryJournal(args.recovery_file, base, payload, selection) as journal:
        live.verify(request, journal, args.timeout, True)
        workspace_id = journal.record['workspaceId']
        write_outputs(args.output_prefix, formal_outputs(request, workspace_id))


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, urllib.error.URLError) as error:
        raise SystemExit('FAIL: ' + (str(error) if isinstance(error, RuntimeError) else type(error).__name__) + '；保留私有 journal') from None
