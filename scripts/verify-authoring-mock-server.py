#!/usr/bin/env python3
"""模板验收专用 authoring mock；无 Runtime 响应，无外部 provider 转发。"""
import argparse
import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from verify_authoring_mock import AuthoringMockError, authoring_response


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ('/v1/models', '/models'):
            self.send_error(404)
            return
        self.respond(200, {'data': [{'id': 'verify-model', 'object': 'model',
                                   'context_window': 32768, 'max_output_tokens': 4096}]})

    def do_POST(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        try:
            size = int(self.headers.get('Content-Length', '0'))
            if not 0 < size <= 1048576:
                raise ValueError()
            payload = json.loads(self.rfile.read(size))
            if not isinstance(payload, dict):
                raise ValueError()
            decision = authoring_response(str(payload.get('input', '')))
            if decision is None:
                raise AuthoringMockError('仅支持创作请求')
        except (AuthoringMockError, ValueError, TypeError):
            self.respond(422, {'error': {'message': 'authoring mock 合同无效'}})
            return
        content = json.dumps(decision)
        usage = {'input_tokens': 256, 'output_tokens': 128, 'total_tokens': 384}
        if payload.get('stream'):
            events = [{'type': 'response.output_text.delta', 'delta': content},
                      {'type': 'response.completed', 'response': {'status': 'completed', 'output': [], 'usage': usage}}]
            encoded = ''.join('data: ' + json.dumps(item) + '\n\n' for item in events).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.send_header('Content-Length', str(len(encoded)))
            self.end_headers()
            self.wfile.write(encoded)
            return
        self.respond(200, {'status': 'completed', 'output_text': content,
                           'output': [], 'usage': usage})

    def respond(self, code, body):
        encoded = json.dumps(body).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, format, *args):
        pass


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, required=True)
    parser.add_argument('--ready-file', required=True)
    args = parser.parse_args()
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    with open(args.ready_file, 'x') as ready:
        ready.write(str(os.getpid()))
    server.serve_forever()
