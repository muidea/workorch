#!/usr/bin/env python3
"""live 验证器本地合同回归，不发送 HTTP 或调用 provider。"""
import copy
import importlib.util
from pathlib import Path
import tempfile
from unittest.mock import patch
import unittest

spec = importlib.util.spec_from_file_location("authoring_live", Path(__file__).with_name("verify-workspace-authoring-live.py"))
assert spec is not None and spec.loader is not None
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


def owner_selection(selection):
    value = {'target': {key: selection[key] for key in ('providerId', 'modelId')}}
    if selection['reasoningEffort']:
        value['reasoningEffort'] = selection['reasoningEffort']
    return value


def ready_view():
    return {"state": "ready_to_confirm", "templateCurrent": True, "inputHash": "input", "derivationPermitId": "permit",
            "proposal": {"derivationPermitId": "permit", "proposalHash": "proposal", "specificationHash": "spec",
                         "items": [{"ref": "item"}], "reviews": [{"kind": kind, "status": "PASS", "specificationHash": "proposal"}
                         for kind in ("dimension_coverage", "negative_path", "regression", "resource_environment", "traceability", "verifier_feasibility")]}}


class AuthoringLiveContractTest(unittest.TestCase):
    def test_owner_selection_uses_nested_target_and_rejects_request_shape(self):
        self.assertIsNone(live.workspace_selection({}))
        for effort in ('', 'none', 'high'):
            selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': effort}
            self.assertEqual(live.workspace_selection({'defaultLLMSelection': owner_selection(selection)}), selection)
        for value in ({'providerId': 'ai-proxy', 'modelId': 'native'}, {}, {'target': None},
                      {'target': {'providerId': 'ai-proxy'}},
                      {'target': {'providerId': 'ai-proxy', 'modelId': ' native'}},
                      {'target': {'providerId': 'ai-proxy', 'modelId': 'native'}, 'reasoningEffort': None}):
            with self.subTest(value=value), self.assertRaises(RuntimeError):
                live.workspace_selection({'defaultLLMSelection': value})

    def test_selection_is_applied_before_compile_and_confirmation_reads_latest_revision(self):
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        workspace = {'id': 'ws', 'revision': 1}
        confirmed = False
        order = []
        ready = {**ready_view(), 'workspaceId': 'ws', 'workspaceRevision': 2}
        def request(method, route, body=None):
            nonlocal confirmed
            order.append((method, route))
            if route.endswith('/runs'):
                return {'runs': []}
            if route.endswith('/workspace-authoring'):
                return {'authoring': {**ready, 'state': 'draft'}}
            if route.endswith('/llm-selection'):
                self.assertFalse(confirmed)
                workspace.update(revision=2, defaultLLMSelection=owner_selection(selection))
                return {'workspace': copy.deepcopy(workspace)}
            if route == '/api/v1/workorch/workspaces/ws':
                return {'workspace': copy.deepcopy(workspace)}
            if route.endswith('/authoring/confirm'):
                self.assertEqual(body['expectedWorkspaceRevision'], 2)
                confirmed = True
                return {'authoring': {**ready, 'state': 'admitted', 'activationState': 'eligible', 'startupInputHash': 'startup'}}
            if route.endswith('/authoring/compile'):
                self.assertEqual(workspace.get('defaultLLMSelection'), owner_selection(selection))
                return {'authoring': ready}
            return {'authoring': {**ready, 'state': 'draft'}}
        with tempfile.TemporaryDirectory() as directory:
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', {'requestId': 'stable'}, selection) as journal:
                live.verify(request, journal, 1, True)
        self.assertEqual(sum(method == 'PUT' for method, _ in order), 1)
        self.assertLess(next(i for i, (_, path) in enumerate(order) if path.endswith('/llm-selection')), next(i for i, (_, path) in enumerate(order) if path.endswith('/compile')))
        self.assertFalse(any('execution-activations' in path for _, path in order))

    def test_selection_response_loss_recovers_readonly_and_keeps_original_cas(self):
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        workspace = {'id': 'ws', 'revision': 1}
        writes = []
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            def request(method, route, body=None):
                if route.endswith('/authoring'):
                    return {'authoring': {'workspaceId': 'ws', 'state': 'draft'}}
                if method == 'PUT':
                    persisted = live.json.loads(path.read_text())
                    self.assertEqual(persisted['selection'], selection)
                    self.assertEqual(persisted['selectionRequest']['expectedRevision'], body['expectedRevision'])
                    writes.append(copy.deepcopy(body))
                    workspace.update(revision=2, defaultLLMSelection=owner_selection(selection))
                    raise RuntimeError('响应丢失')
                return {'workspace': copy.deepcopy(workspace)}
            with live.RecoveryJournal(path, 'http://localhost', payload, selection) as journal:
                journal.record['workspaceId'] = 'ws'
                journal.save()
                with self.assertRaisesRegex(RuntimeError, '响应丢失'):
                    live.ensure_preseal_selection(request, journal, '/ws')
            with live.RecoveryJournal(path, 'http://localhost', payload, selection) as journal:
                live.ensure_preseal_selection(request, journal, '/ws')
                self.assertTrue(journal.record['selectionApplied'])
                self.assertEqual(journal.record['selectionRequest']['expectedRevision'], 1)
                journal.record['confirmation'] = {'inputHash': 'input', 'derivationPermitId': 'permit', 'proposalHash': 'proposal', 'specificationHash': 'spec', 'expectedWorkspaceRevision': 8}
                journal.save()
                live.ensure_preseal_selection(request, journal, '/ws')
            self.assertEqual(writes, [{**selection, 'expectedRevision': 1}])
            before = path.read_bytes()
            with self.assertRaises(RuntimeError):
                with live.RecoveryJournal(path, 'http://localhost', payload, {**selection, 'modelId': 'other'}):
                    self.fail('不能替换原模型身份')
            self.assertEqual(path.read_bytes(), before)

    def test_selection_matching_value_with_unrelated_revision_drift_is_not_receipt(self):
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', {'requestId': 'stable'}, selection) as journal:
                journal.record.update(workspaceId='ws', selectionRequest={'workspaceId': 'ws', 'expectedRevision': 1, 'baseline': None})
                journal.save()
                before = path.read_bytes()
                def request(method, route, body=None):
                    self.assertEqual(method, 'GET')
                    return {'workspace': {'id': 'ws', 'revision': 3, 'defaultLLMSelection': owner_selection(selection)}}
                with self.assertRaisesRegex(RuntimeError, 'revision 冲突'):
                    live.ensure_preseal_selection(request, journal, '/ws')
                self.assertEqual(path.read_bytes(), before)
                self.assertNotIn('selectionApplied', journal.record)

    def test_selection_conflicting_revision_or_seal_never_writes(self):
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        for state, revision in [('draft', 2), ('admitted', 1)]:
            with self.subTest(state=state), tempfile.TemporaryDirectory() as directory:
                with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', {'requestId': 'stable'}, selection) as journal:
                    journal.record.update(workspaceId='ws', selectionRequest={'workspaceId': 'ws', 'expectedRevision': 1, 'baseline': None})
                    journal.save()
                    def request(method, route, body=None):
                        self.assertEqual(method, 'GET')
                        if route.endswith('/authoring'):
                            return {'authoring': {'workspaceId': 'ws', 'state': state}}
                        return {'workspace': {'id': 'ws', 'revision': revision}}
                    with self.assertRaises(RuntimeError):
                        live.ensure_preseal_selection(request, journal, '/ws')
                    self.assertNotIn('selectionApplied', journal.record)

    def test_selection_read_loss_and_put_conflict_preserve_original_request(self):
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        payload = {'requestId': 'stable'}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', payload, selection) as journal:
                journal.record.update(workspaceId='ws', selectionRequest={'workspaceId': 'ws', 'expectedRevision': 3, 'baseline': None})
                journal.save()
                original = path.read_bytes()
                def unavailable(method, route, body=None):
                    self.assertEqual(method, 'GET')
                    raise RuntimeError('读取响应丢失')
                with self.assertRaisesRegex(RuntimeError, '读取响应丢失'):
                    live.ensure_preseal_selection(unavailable, journal, '/ws')
                self.assertEqual(path.read_bytes(), original)
                def conflict(method, route, body=None):
                    if method == 'PUT':
                        self.assertEqual(body, {**selection, 'expectedRevision': 3})
                        raise RuntimeError('HTTP 409')
                    if route.endswith('/authoring'):
                        return {'authoring': {'workspaceId': 'ws', 'state': 'draft'}}
                    return {'workspace': {'id': 'ws', 'revision': 3}}
                with self.assertRaisesRegex(RuntimeError, 'HTTP 409'):
                    live.ensure_preseal_selection(conflict, journal, '/ws')
                self.assertEqual(path.read_bytes(), original)

    def test_selection_corrupt_journal_and_sealed_mismatch_fail_closed(self):
        selection = {'providerId': 'ai-proxy', 'modelId': 'native', 'reasoningEffort': ''}
        payload = {'requestId': 'stable'}
        exact = {'inputHash': 'input', 'derivationPermitId': 'permit', 'proposalHash': 'proposal', 'specificationHash': 'spec', 'expectedWorkspaceRevision': 8}
        for pending in (None, {}, {'workspaceId': 'other', 'expectedRevision': 1, 'baseline': None}, {'workspaceId': 'ws', 'expectedRevision': True, 'baseline': None}):
            with self.subTest(pending=pending), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / 'recovery.json'
                with live.RecoveryJournal(path, 'http://localhost', payload, selection) as journal:
                    journal.record.update(workspaceId='ws', selectionRequest=pending)
                    journal.save()
                original = path.read_bytes()
                with self.assertRaises(RuntimeError):
                    with live.RecoveryJournal(path, 'http://localhost', payload, selection):
                        self.fail('损坏选择请求不能恢复')
                self.assertEqual(path.read_bytes(), original)
        with tempfile.TemporaryDirectory() as directory:
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload, selection) as journal:
                journal.record.update(workspaceId='ws', selectionApplied=True, confirmation=exact)
                journal.save()
                def request(method, route, body=None):
                    self.assertEqual(method, 'GET')
                    return {'workspace': {'id': 'ws', 'revision': 12, 'defaultLLMSelection': owner_selection({**selection, 'modelId': 'other'})}}
                with self.assertRaisesRegex(RuntimeError, '禁止改写密封'):
                    live.ensure_preseal_selection(request, journal, '/ws')
                self.assertEqual(journal.record['confirmation'], exact)

    def test_old_journal_schema_is_rejected_without_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', {'requestId': 'stable'}) as journal:
                journal.record['version'] = 1
                journal.save()
            before = path.read_bytes()
            with self.assertRaises(RuntimeError):
                with live.RecoveryJournal(path, 'http://localhost', {'requestId': 'stable'}):
                    self.fail('旧 schema 不能恢复')
            self.assertEqual(path.read_bytes(), before)

    def test_valid_ready_proposal(self):
        view = ready_view()
        self.assertEqual(live.require_ready_proposal(view), view["proposal"])

    def test_admitted_receipt_requires_scope_hash_and_revision(self):
        view = {'workspaceId': 'ws', 'state': 'admitted', 'activationState': 'eligible', 'startupInputHash': 'startup', 'workspaceRevision': 12}
        live.require_admitted(view, 'ws')
        for field, value in [('workspaceId', 'other'), ('startupInputHash', []), ('workspaceRevision', True), ('workspaceRevision', 0), ('activationState', 'pending')]:
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                live.require_admitted({**view, field: value}, 'ws')

    def test_missing_runs_is_not_zero_runs(self):
        live.require_zero_runs({"runs": []})
        for response in ({}, {"runs": None}, {"runs": {}}, {"runs": [{"id": "unexpected"}]}):
            with self.subTest(response=response), self.assertRaises(RuntimeError):
                live.require_zero_runs(response)

    def test_confirmation_response_loss_replays_original_request_after_reopen(self):
        payload = {'requestId': None, 'title': '验证', 'templateId': 'template', 'deliverableDirectory': '', 'input': {'goalText': '目标'}}
        ready = {**ready_view(), 'workspaceId': 'ws', 'workspaceRevision': 8}
        admitted = {**ready, 'state': 'admitted', 'activationState': 'eligible', 'startupInputHash': 'startup', 'workspaceRevision': 12}
        confirmations = []
        lost = True
        path = None

        def request(method, route, body=None):
            nonlocal lost
            if route.endswith('/workspace-authoring'):
                assert body is not None
                self.assertEqual(body['requestId'], original_id)
                return {'authoring': ready if lost else admitted}
            if route.endswith('/runs'):
                return {'runs': []}
            if route.endswith('/authoring/compile'):
                self.fail('已完成或确认恢复不应重编')
            if route.endswith('/authoring/confirm'):
                confirmations.append(copy.deepcopy(body))
                assert path is not None
                persisted = live.json.loads(path.read_text())
                self.assertEqual(persisted['confirmation'], body)
                if lost:
                    lost = False
                    raise RuntimeError('模拟确认已成功但响应丢失')
                return {'authoring': admitted}
            return {'authoring': ready}

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', payload) as journal:
                original_id = journal.record['payload']['requestId']
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                with self.assertRaisesRegex(RuntimeError, '响应丢失'):
                    live.verify(request, journal, 1, True)
            with live.RecoveryJournal(path, 'http://localhost', payload) as journal:
                live.verify(request, journal, 1, True)
            self.assertEqual(len(confirmations), 3)
            self.assertTrue(all(value == confirmations[0] for value in confirmations))
            self.assertEqual(confirmations[0]['expectedWorkspaceRevision'], 8)

    def test_journal_rejects_public_permissions_and_symlinks_without_overwrite(self):
        payload = {'requestId': None, 'input': {'goalText': '原目标'}}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', payload):
                pass
            original = path.read_bytes()
            path.chmod(0o644)
            with self.assertRaisesRegex(RuntimeError, '0600'):
                with live.RecoveryJournal(path, 'http://localhost', payload):
                    self.fail('不安全权限不得读取恢复输入')
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(path.stat().st_mode & 0o777, 0o644)
            link = Path(directory) / 'linked.json'
            link.symlink_to(path)
            with self.assertRaisesRegex(RuntimeError, '符号链接'):
                with live.RecoveryJournal(link, 'http://localhost', payload):
                    self.fail('不得跟随恢复文件符号链接')
            self.assertEqual(path.read_bytes(), original)

    def test_journal_rejects_different_input_instance_and_concurrent_use(self):
        payload = {'requestId': None, 'input': {'goalText': '原目标'}}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', payload):
                with self.assertRaises(BlockingIOError):
                    with live.RecoveryJournal(path, 'http://localhost', payload):
                        self.fail('不应允许并发验证')
            before = path.read_bytes()
            for base, value in [('http://other', payload), ('http://localhost', {**payload, 'input': {'goalText': '新目标'}})]:
                with self.assertRaises(RuntimeError):
                    with live.RecoveryJournal(path, base, value):
                        self.fail('不能改写原身份')
                self.assertEqual(path.read_bytes(), before)

    def test_create_response_loss_preserves_identity_before_first_http_write(self):
        payload = {'requestId': None, 'input': {'goalText': '目标'}}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'recovery.json'
            with live.RecoveryJournal(path, 'http://localhost', payload) as journal:
                original = journal.record['payload']
                def request(_method, _route, body=None):
                    self.assertEqual(live.json.loads(path.read_text())['payload'], body)
                    raise RuntimeError('模拟创建响应丢失')
                with self.assertRaisesRegex(RuntimeError, '创建响应丢失'):
                    live.verify(request, journal, 1, False)
            with live.RecoveryJournal(path, 'http://localhost', payload) as journal:
                self.assertEqual(journal.record['payload'], original)

    def test_unresolved_result_only_polls_and_does_not_compile(self):
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        def request(_method, route, _body=None):
            if route.endswith('/compile'):
                self.fail('未决请求不得自动重编')
            return {'runs': []} if route.endswith('/runs') else {'authoring': {**ready_view(), 'workspaceId': 'ws', 'state': 'outcome_unresolved'}}
        with tempfile.TemporaryDirectory() as directory, patch.object(live.time, 'sleep'), patch.object(live.time, 'monotonic', side_effect=[0, 0, 2]):
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                original = copy.deepcopy(journal.record)
                with self.assertRaisesRegex(RuntimeError, '结果未决'):
                    live.verify(request, journal, 1, False)
                self.assertEqual(journal.record['payload'], original['payload'])
                self.assertNotIn('confirmation', journal.record)

    def test_inflight_unresolved_completes_without_compile_or_confirmation(self):
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        polls = 0
        def request(method, route, body=None):
            nonlocal polls
            if route.endswith(('/compile', '/confirm')):
                self.fail('在途请求不得重编或替代确认')
            if route.endswith('/runs'):
                return {'runs': []}
            view = {**ready_view(), 'workspaceId': 'ws', 'workspaceRevision': 8}
            if method == 'GET':
                polls += 1
            if polls < 3:
                view['state'] = 'outcome_unresolved'
            return {'authoring': view}
        with tempfile.TemporaryDirectory() as directory, patch.object(live.time, 'sleep'):
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                live.verify(request, journal, 10, False)
                self.assertNotIn('confirmation', journal.record)
        self.assertEqual(polls, 3)

    def test_derived_recovery_requests_drive_once_and_keeps_creation_identity(self):
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        ready = {**ready_view(), 'workspaceId': 'ws', 'workspaceRevision': 8}
        calls = []
        def request(method, route, body=None):
            calls.append((method, route))
            if route.endswith('/runs'):
                return {'runs': []}
            if route.endswith('/workspace-authoring'):
                assert body is not None
                self.assertEqual(body['requestId'], 'stable')
            if route.endswith('/compile'):
                return {'authoring': ready}
            return {'authoring': {**ready, 'state': 'derived'}}
        with tempfile.TemporaryDirectory() as directory:
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                live.verify(request, journal, 1, False)
        self.assertEqual(sum(route.endswith('/compile') for _, route in calls), 1)
        self.assertFalse(any(route.endswith('/confirm') for _, route in calls))

    def test_confirmation_intermediate_states_replay_only_original_identity(self):
        exact = {'inputHash': 'input', 'derivationPermitId': 'permit', 'proposalHash': 'proposal', 'specificationHash': 'spec', 'expectedWorkspaceRevision': 8}
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        admitted = {**ready_view(), 'workspaceId': 'ws', 'workspaceRevision': 12, 'state': 'admitted', 'activationState': 'eligible', 'startupInputHash': 'startup'}
        confirmations = []
        def request(method, route, body=None):
            if route.endswith('/runs'):
                return {'runs': []}
            if route.endswith('/compile'):
                self.fail('确认恢复不能编译')
            if route.endswith('/confirm'):
                confirmations.append(copy.deepcopy(body))
                return {'authoring': {**admitted, 'state': 'confirming' if len(confirmations) == 1 else 'admitted'}}
            return {'authoring': {**admitted, 'state': 'admitting'}}
        with tempfile.TemporaryDirectory() as directory, patch.object(live.time, 'sleep'):
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                journal.record.update(workspaceId='ws', confirmation=exact)
                journal.save()
                live.verify(request, journal, 10, True)
                self.assertEqual(journal.record['confirmation'], exact)
        self.assertEqual(confirmations, [exact, exact, exact])

    def test_missing_confirmation_identity_fails_closed_for_owner_transaction(self):
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        for state in ('confirming', 'admitting', 'admitted'):
            with self.subTest(state=state), tempfile.TemporaryDirectory() as directory:
                def request(method, route, body=None):
                    if route.endswith(('/compile', '/confirm')):
                        self.fail('缺原确认不得写事务')
                    return {'runs': []} if route.endswith('/runs') else {'authoring': {**ready_view(), 'workspaceId': 'ws', 'state': state}}
                with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                    with self.assertRaisesRegex(RuntimeError, '缺少原确认身份'):
                        live.verify(request, journal, 1, True)

    def test_confirmation_timeout_preserves_original_identity(self):
        exact = {'inputHash': 'input', 'derivationPermitId': 'permit', 'proposalHash': 'proposal', 'specificationHash': 'spec', 'expectedWorkspaceRevision': 8}
        def request(method, route, body=None):
            if route.endswith('/confirm'):
                self.assertEqual(body, exact)
            return {'runs': []} if route.endswith('/runs') else {'authoring': {'workspaceId': 'ws', 'state': 'admitting'}}
        with self.assertRaisesRegex(RuntimeError, '保留原确认身份'):
            live.await_confirmation(request, '/ws', 'ws', exact, 0)

    def test_pending_confirmation_without_explicit_confirm_does_not_compile(self):
        exact = {'inputHash': 'input', 'derivationPermitId': 'permit', 'proposalHash': 'proposal', 'specificationHash': 'spec', 'expectedWorkspaceRevision': 8}
        payload = {'requestId': 'stable', 'input': {'goalText': '目标'}}
        routes = []
        def request(method, route, body=None):
            routes.append(route)
            return {'runs': []} if route.endswith('/runs') else {'authoring': {**ready_view(), 'workspaceId': 'ws'}}
        with tempfile.TemporaryDirectory() as directory:
            with live.RecoveryJournal(Path(directory) / 'recovery.json', 'http://localhost', payload) as journal:
                journal.record.update(workspaceId='ws', confirmation=exact)
                journal.save()
                with self.assertRaisesRegex(RuntimeError, '--confirm'):
                    live.verify(request, journal, 1, False)
        self.assertFalse(any(route.endswith(('/compile', '/confirm')) for route in routes))

    def test_fail_closed_for_incomplete_or_stale_identity(self):
        for field in ("inputHash", "derivationPermitId"):
            view = ready_view()
            del view[field]
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)
        for field in ("proposalHash", "specificationHash", "derivationPermitId", "items"):
            view = ready_view()
            del view["proposal"][field]
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)
        view = ready_view()
        view["templateCurrent"] = False
        with self.assertRaises(RuntimeError):
            live.require_ready_proposal(view)

    def test_reviews_require_canonical_kinds_and_exact_proposal(self):
        base = ready_view()
        for field, value in (("kind", "invented"), ("status", "UNKNOWN"), ("status", "FAIL"), ("specificationHash", "stale")):
            view = copy.deepcopy(base)
            view["proposal"]["reviews"][0][field] = value
            with self.subTest(field=field, value=value), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)


if __name__ == "__main__":
    unittest.main()
