"""authoring mock HTTP 编码测试；不启动服务、不修改部署资源。"""
import importlib.util
import io
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('mock_server', Path(__file__).with_name('verify-authoring-mock-server.py'))
assert spec is not None and spec.loader is not None
server = importlib.util.module_from_spec(spec)
# 脚本正常执行时 Python 自动包含 scripts；测试显式加载同目录模块。
import sys
mock_spec = importlib.util.spec_from_file_location('verify_authoring_mock', Path(__file__).with_name('verify_authoring_mock.py'))
assert mock_spec is not None and mock_spec.loader is not None
mock = importlib.util.module_from_spec(mock_spec)
mock_spec.loader.exec_module(mock)
sys.modules['verify_authoring_mock'] = mock
spec.loader.exec_module(server)


class EncodingTest(unittest.TestCase):
    def call(self, payload):
        body = json.dumps(payload).encode()
        handler = object.__new__(server.Handler)
        handler.path = '/v1/responses'
        handler.headers = {'Content-Length': str(len(body))}
        handler.rfile = io.BytesIO(body)
        handler.wfile = io.BytesIO()
        statuses, headers = [], {}
        handler.send_response = statuses.append
        handler.send_header = lambda key, value: headers.update({key: value})
        handler.end_headers = lambda: None
        handler.do_POST()
        return statuses, headers, handler.wfile.getvalue()

    def test_catalog_matches_responses_model_contract(self):
        for path in ('/models', '/v1/models'):
            with self.subTest(path=path):
                handler = object.__new__(server.Handler)
                handler.path = path
                responses = []
                handler.respond = lambda code, body: responses.append((code, body))
                handler.do_GET()
                self.assertEqual(responses, [(200, {'object': 'list', 'data': [{
                    'id': 'verify-model', 'object': 'model',
                    'supported_endpoints': ['/v1/responses'],
                    'contextWindowTokens': 32768, 'maxOutputTokens': 4096,
                }]})])

    def test_nonstream_and_stream_preserve_same_derivation_identity(self):
        prompt = '### C3.derivation-input\n' + json.dumps({'outputShape': {'AuthoringInputHash': 'exact', 'TemplateSnapshotHash': 'sealed'}})
        statuses, _, body = self.call({'input': prompt})
        self.assertEqual(statuses, [200])
        expected = json.loads(json.loads(body)['output_text'])
        statuses, headers, body = self.call({'input': prompt, 'stream': True})
        self.assertEqual(statuses, [200])
        self.assertEqual(headers['Content-Type'], 'text/event-stream')
        events = [json.loads(line.removeprefix('data: ')) for line in body.decode().splitlines() if line.startswith('data: ')]
        self.assertEqual(json.loads(events[0]['delta']), expected)
        self.assertEqual(events[-1]['type'], 'response.completed')

    def test_invalid_and_runtime_requests_are_redacted_422(self):
        for payload in ([], {'input': '### C3.specification-request\n秘密损坏正文'}, {'input': '### C3.semantic-authority\n{}'}):
            statuses, _, body = self.call(payload)
            self.assertEqual(statuses, [422])
            self.assertNotIn('秘密', body.decode())


if __name__ == '__main__':
    unittest.main()
