"""共享验收客户端的只读身份与私有输出合同；不连接部署服务。"""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('authoring_client', Path(__file__).with_name('create-authoring-verification-workspace.py'))
assert spec is not None and spec.loader is not None
client = importlib.util.module_from_spec(spec)
spec.loader.exec_module(client)


class ClientTest(unittest.TestCase):
    def responses(self):
        return {'': {'workspace': {'id': 'ws'}},
                '/goals': {'goals': [{'id': 'goal', 'workspaceId': 'ws'}]},
                '/dods': {'dods': [{'id': 'dod', 'workspaceId': 'ws'}]}}

    def test_formal_identities_are_read_not_invented(self):
        data = self.responses()
        calls = []
        def request(method, path):
            calls.append((method, path))
            return data[path.removeprefix('/api/v1/workorch/workspaces/ws')]
        values = dict(client.formal_outputs(request, 'ws'))
        self.assertEqual(values['goal']['goal']['id'], 'goal')
        self.assertEqual(values['dod']['dod']['id'], 'dod')
        self.assertTrue(all(method == 'GET' for method, _ in calls))
        data['/goals']['goals'][0]['workspaceId'] = 'other'
        with self.assertRaises(RuntimeError):
            client.formal_outputs(request, 'ws')
        data['/goals']['goals'] = []
        with self.assertRaises(RuntimeError):
            client.formal_outputs(request, 'ws')

    def test_private_output_replay_rejects_overwrite_and_symlink(self):
        with tempfile.TemporaryDirectory() as directory:
            prefix = str(Path(directory) / 'result')
            values = [('goal', {'goal': {'id': 'exact'}})]
            client.write_outputs(prefix, values)
            output = Path(prefix + '-goal.json')
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            client.write_outputs(prefix, values)
            original = output.read_bytes()
            with self.assertRaises(RuntimeError):
                client.write_outputs(prefix, [('goal', {'goal': {'id': 'new'}})])
            self.assertEqual(output.read_bytes(), original)
            link = Path(prefix + '-dod.json')
            link.symlink_to(output)
            with self.assertRaises(RuntimeError):
                client.write_outputs(prefix, [('dod', json.loads(original))])
            self.assertEqual(output.read_bytes(), original)


if __name__ == '__main__':
    unittest.main()
