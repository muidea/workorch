#!/usr/bin/env python3
"""对已启动服务验证持久创作链路；显式执行才会调用 provider，不自动启动 Run。"""
import argparse
import fcntl
import json
import os
from pathlib import Path
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


def require_zero_runs(response):
    runs = response.get("runs")
    if not isinstance(runs, list):
        raise RuntimeError("Run 查询响应缺少有效 runs，不能证明零 Run")
    if runs:
        raise RuntimeError("创作、编译或确认阶段出现未显式启动的 Run")


def require_ready_proposal(view):
    if view.get("state") != "ready_to_confirm" or view.get("templateCurrent") is not True:
        raise RuntimeError("未得到当前模板下可确认规格；保留草稿供诊断")
    proposal = view.get("proposal")
    if not isinstance(proposal, dict):
        raise RuntimeError("待确认规格响应缺少 proposal")
    for value in (view.get("inputHash"), view.get("derivationPermitId"), proposal.get("proposalHash"), proposal.get("specificationHash")):
        if not isinstance(value, str) or not value:
            raise RuntimeError("待确认规格缺少精确确认身份")
    if proposal.get("derivationPermitId") != view["derivationPermitId"]:
        raise RuntimeError("待确认规格派生身份不一致")
    reviews = proposal.get("reviews")
    kinds = {"dimension_coverage", "negative_path", "regression", "resource_environment", "traceability", "verifier_feasibility"}
    if not isinstance(reviews, list) or len(reviews) != 6 or any(not isinstance(review, dict) for review in reviews):
        raise RuntimeError("六审查响应不完整")
    if {review.get("kind") for review in reviews} != kinds or any(review.get("status") != "PASS" or review.get("specificationHash") != proposal["proposalHash"] for review in reviews):
        raise RuntimeError("六审查未全部 PASS 或未绑定当前提案")
    if not isinstance(proposal.get("items"), list) or not proposal["items"]:
        raise RuntimeError("派生验收清单为空")
    return proposal


def validate_selection(value):
    if not isinstance(value, dict) or set(value) != {'providerId', 'modelId', 'reasoningEffort'} or any(not isinstance(value[key], str) for key in value) or not value['providerId'] or not value['modelId'] or any(text != text.strip() for text in value.values()):
        raise RuntimeError('模型选择身份无效')


def workspace_selection(workspace):
    value = workspace.get('defaultLLMSelection')
    if value is None:
        return None
    if not isinstance(value, dict) or set(value) - {'target', 'reasoningEffort'}:
        raise RuntimeError('owner 模型选择响应无效')
    target = value.get('target')
    if not isinstance(target, dict) or set(target) != {'providerId', 'modelId'}:
        raise RuntimeError('owner 模型选择 target 无效')
    selection = {**target, 'reasoningEffort': value.get('reasoningEffort', '')}
    validate_selection(selection)
    return selection


def ensure_preseal_selection(request, journal, path):
    record = journal.record
    desired = record['selection']
    if desired is None:
        return
    workspace = request('GET', path)['workspace']
    revision = workspace.get('revision')
    if workspace.get('id') != record['workspaceId'] or type(revision) is not int or revision < 1:
        raise RuntimeError('模型选择 Workspace 身份无效')
    current = workspace_selection(workspace)
    pending = record.get('selectionRequest')
    if current == desired:
        if pending is not None and not record.get('selectionApplied') and revision != pending['expectedRevision'] + 1:
            raise RuntimeError('模型选择恢复 revision 冲突，保留原请求')
        record['selectionApplied'] = True
        journal.save()
        return
    if record.get('confirmation') or record.get('selectionApplied'):
        raise RuntimeError('原模型选择已改变，禁止改写密封或恢复身份')
    view = request('GET', path + '/authoring')['authoring']
    if view.get('workspaceId') != record['workspaceId'] or view.get('state') != 'draft':
        raise RuntimeError('模型选择必须在编译与确认前完成')
    if pending is None:
        pending = {'workspaceId': record['workspaceId'], 'expectedRevision': revision, 'baseline': current}
        record['selectionRequest'] = pending
        journal.save()
    if pending['workspaceId'] != workspace['id'] or pending['expectedRevision'] != revision or pending['baseline'] != current:
        raise RuntimeError('模型选择 CAS 基线冲突，保留原请求')
    response = request('PUT', path + '/llm-selection', {**desired, 'expectedRevision': pending['expectedRevision']})['workspace']
    if response.get('id') != workspace['id'] or response.get('revision') != revision + 1 or workspace_selection(response) != desired:
        raise RuntimeError('模型选择回执身份不匹配')
    record['selectionApplied'] = True
    journal.save()


class RecoveryJournal:
    """本地私有回执，不是 owner authority；锁覆盖整个验证过程。"""
    def __init__(self, path, base, payload, selection=None):
        if selection is not None:
            validate_selection(selection)
        self.selection = selection
        self.path = Path(path).expanduser().absolute()
        self.base = base
        self.payload = payload
        self.lock = None
        self.record = None

    def __enter__(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        fd = os.open(str(self.path) + '.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        self.lock = os.fdopen(fd, 'w')
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            if self.path.is_symlink():
                raise RuntimeError('恢复文件不能是符号链接')
            if self.path.exists():
                if self.path.stat().st_mode & 0o077:
                    raise RuntimeError('恢复文件必须仅当前用户可读写（0600）')
                self.record = json.loads(self.path.read_text())
                value = self.record
                if not isinstance(value, dict) or value.get('version') != 2 or value.get('baseUrl') != self.base or 'selection' not in value or value['selection'] != self.selection or not isinstance(value.get('payload'), dict):
                    raise RuntimeError('恢复文件版本或实例不匹配')
                original = value['payload']
                expected = {**self.payload, 'requestId': original.get('requestId')}
                if original != expected or not isinstance(original.get('requestId'), str) or not original['requestId'] or (self.payload['requestId'] and self.payload['requestId'] != original['requestId']):
                    raise RuntimeError('恢复必须使用原创建输入与 requestId，不能覆盖恢复文件')
                if 'workspaceId' in value and (not isinstance(value['workspaceId'], str) or not value['workspaceId']):
                    raise RuntimeError('恢复文件 Workspace 身份无效')
                if 'selectionApplied' in value and (value['selectionApplied'] is not True or self.selection is None or not value.get('workspaceId')):
                    raise RuntimeError('模型选择恢复状态无效')
                if 'selectionRequest' in value:
                    pending = value['selectionRequest']
                    if self.selection is None or not isinstance(pending, dict) or set(pending) != {'workspaceId', 'expectedRevision', 'baseline'} or pending['workspaceId'] != value.get('workspaceId') or type(pending['expectedRevision']) is not int or pending['expectedRevision'] < 1:
                        raise RuntimeError('模型选择恢复请求无效')
                    if pending['baseline'] is not None:
                        validate_selection(pending['baseline'])
                if 'confirmation' in value:
                    if self.selection is not None and not value.get('selectionApplied'):
                        raise RuntimeError('确认恢复缺少模型选择完成事实')
                    validate_confirmation(value['confirmation'])
                    if not value.get('workspaceId'):
                        raise RuntimeError('恢复确认缺少 Workspace 身份')
            else:
                self.record = {'version': 2, 'baseUrl': self.base, 'selection': self.selection, 'payload': {**self.payload, 'requestId': self.payload['requestId'] or str(uuid.uuid4())}}
                self.save()
            return self
        except BaseException:
            self.lock.close()
            raise

    def save(self):
        fd, temporary = tempfile.mkstemp(prefix=self.path.name + '.', dir=self.path.parent)
        try:
            with os.fdopen(fd, 'w') as output:
                json.dump(self.record, output, ensure_ascii=False)
                output.flush()
                os.fsync(output.fileno())
            os.replace(temporary, self.path)
            directory = os.open(self.path.parent, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)

    def __exit__(self, *_):
        if self.lock is not None:
            self.lock.close()


def validate_confirmation(exact):
    fields = ('inputHash', 'derivationPermitId', 'proposalHash', 'specificationHash')
    if not isinstance(exact, dict) or any(not isinstance(exact.get(key), str) or not exact[key] for key in fields) or type(exact.get('expectedWorkspaceRevision')) is not int or exact['expectedWorkspaceRevision'] < 1:
        raise RuntimeError('恢复确认身份无效')


def require_admitted(view, workspace_id):
    if view.get('workspaceId') != workspace_id or view.get('state') != 'admitted' or view.get('activationState') != 'eligible' or not isinstance(view.get('startupInputHash'), str) or not view['startupInputHash'] or type(view.get('workspaceRevision')) is not int or view['workspaceRevision'] < 1:
        raise RuntimeError('确认后未得到 matching eligible Workspace')


def await_confirmation(request, path, workspace_id, exact, timeout):
    deadline = time.monotonic() + timeout
    view = request('POST', path + '/authoring/confirm', exact)['authoring']
    while view.get('state') in ('confirming', 'admitting'):
        if view.get('workspaceId') != workspace_id:
            raise RuntimeError('确认恢复返回不同 Workspace')
        require_zero_runs(request('GET', path + '/runs'))
        if time.monotonic() >= deadline:
            raise RuntimeError('确认准入仍在恢复；保留原确认身份，不确认新提案')
        time.sleep(2)
        view = request('GET', path + '/authoring')['authoring']
        if view.get('state') in ('confirming', 'admitting'):
            # 查询不驱动 owner 事务；只重放原确认，保留原 expected revision。
            view = request('POST', path + '/authoring/confirm', exact)['authoring']
    require_admitted(view, workspace_id)
    return view


def verify(request, journal, timeout, confirm):
    record = journal.record
    payload = record['payload']
    print(f"requestId={payload['requestId']}", flush=True)
    view = request('POST', '/api/v1/workorch/workspace-authoring', payload)['authoring']
    workspace_id = view['workspaceId']
    if record.get('workspaceId', workspace_id) != workspace_id:
        raise RuntimeError('创建重放返回不同 Workspace')
    record['workspaceId'] = workspace_id
    journal.save()
    path = '/api/v1/workorch/workspaces/' + urllib.parse.quote(workspace_id, safe='')
    print(f'workspaceId={workspace_id}', flush=True)
    replay = request('POST', '/api/v1/workorch/workspace-authoring', payload)['authoring']
    if replay['workspaceId'] != workspace_id or replay['inputHash'] != view['inputHash']:
        raise RuntimeError('重复创建没有返回相同持久身份')
    require_zero_runs(request('GET', path + '/runs'))
    ensure_preseal_selection(request, journal, path)
    exact = record.get('confirmation')
    if exact:
        if not confirm:
            raise RuntimeError('存在原确认请求，请显式加 --confirm 恢复；不会编译或确认新提案')
        view = request('GET', path + '/authoring')['authoring']
        if view['state'] == 'preparation_failed':
            require_zero_runs(request('GET', path + '/runs'))
            raise RuntimeError('创作准备已知失败 preparation_failed；保留原诊断，不自动重新编译')
    else:
        view = request('GET', path + '/authoring')['authoring']
        if view['state'] == 'preparation_failed':
            # Preparation failures are known terminal owner facts (before any
            # provider permit); only explicit user action may retry them.
            require_zero_runs(request('GET', path + '/runs'))
            raise RuntimeError('创作准备已知失败 preparation_failed；保留原诊断，不自动重新编译')
        if view['state'] in ('draft', 'derived'):
            # 恢复 drive 仍由 owner reserve 授权；已未决的 permit 不会重发。
            view = request('POST', path + '/authoring/compile')['authoring']
        deadline = time.monotonic() + timeout
        while view['state'] not in ('ready_to_confirm', 'review_blocked', 'failed', 'preparation_failed', 'clarification_required', 'admitted', 'confirming', 'admitting'):
            if time.monotonic() >= deadline:
                raise RuntimeError('编译超时或结果未决；保留原身份，不自动重发 provider')
            time.sleep(2)
            view = request('GET', path + '/authoring')['authoring']
        if view['state'] == 'preparation_failed':
            require_zero_runs(request('GET', path + '/runs'))
            raise RuntimeError('创作准备已知失败 preparation_failed；保留原诊断，不自动重新编译')
        if view['state'] in ('admitted', 'confirming', 'admitting'):
            raise RuntimeError('owner 已进入确认事务但本地缺少原确认身份；不能构造替代确认')
        proposal = require_ready_proposal(view)
        require_zero_runs(request('GET', path + '/runs'))
        if confirm:
            exact = {'inputHash': view['inputHash'], 'derivationPermitId': view['derivationPermitId'],
                     'proposalHash': proposal['proposalHash'], 'specificationHash': proposal['specificationHash'],
                     'expectedWorkspaceRevision': view['workspaceRevision']}
            validate_confirmation(exact)
            record['confirmation'] = exact
            journal.save()
    if exact:
        confirmed = await_confirmation(request, path, workspace_id, exact, timeout)
        repeated = await_confirmation(request, path, workspace_id, exact, timeout)
        if repeated['startupInputHash'] != confirmed['startupInputHash'] or repeated['workspaceRevision'] != confirmed['workspaceRevision']:
            raise RuntimeError('精确确认重放发生额外变更')
        ensure_preseal_selection(request, journal, path)
    require_zero_runs(request('GET', path + '/runs'))
    print('PASS: 持久创建／六审查' + ('／确认准入重放' if confirm else '') + '／零自动 Run')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    parser.add_argument("--template-id", required=True)
    parser.add_argument("--goal", required=True)
    parser.add_argument("--directory", default="")
    parser.add_argument("--resource", action="append", default=[])
    parser.add_argument("--requirement", action="append", default=[])
    parser.add_argument("--request-id", default=None, help="响应丢失后用同一身份与原输入重放")
    parser.add_argument('--recovery-file', required=True, help='私有恢复文件（含原输入）；必须位于仓库外，重复执行使用同一文件')
    parser.add_argument("--timeout", type=int, default=300)
    parser.add_argument("--confirm", action="store_true", help="精确确认审查结果并要求 eligible；仍不启动 Run")
    args = parser.parse_args()
    if args.timeout < 1 or args.timeout > 3600:
        parser.error("timeout 必须在 1～3600 秒内")
    base = args.base_url.rstrip("/")
    if not base.startswith(("http://", "https://")):
        parser.error("base-url 必须为 HTTP(S)")

    def request(method, path, data=None):
        body = None if data is None else json.dumps(data, ensure_ascii=False).encode()
        req = urllib.request.Request(base + path, data=body, method=method, headers={"Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=60) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            # 不输出原响应、provider 文本或配置，避免泄漏敏感输入。
            raise RuntimeError(f"HTTP {error.code}: {method} {path}") from None

    request("GET", "/healthz")
    request("GET", "/readyz")
    payload = {"requestId": args.request_id, "title": "规格创作 live 验证", "templateId": args.template_id,
               "deliverableDirectory": args.directory,
               "input": {"goalText": args.goal, "acceptanceRequirements": args.requirement, "resourceRefs": args.resource}}
    recovery_path = Path(args.recovery_file).expanduser().resolve()
    repo = Path(__file__).resolve().parent.parent
    if recovery_path.is_relative_to(repo):
        parser.error("恢复文件含原输入，必须位于仓库外")
    with RecoveryJournal(args.recovery_file, base, payload) as journal:
        verify(request, journal, args.timeout, args.confirm)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, urllib.error.URLError, KeyError, ValueError, OSError) as error:
        message = str(error) if isinstance(error, RuntimeError) else type(error).__name__
        raise SystemExit(f"FAIL: {message}；保留原恢复文件后重试") from None
