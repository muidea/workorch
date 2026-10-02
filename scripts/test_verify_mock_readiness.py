#!/usr/bin/env python3
"""验收 mock 必须由本进程成功绑定后发布就绪身份。"""
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parent


class MockReadinessTest(unittest.TestCase):
    def test_authoring_mock_conflicting_port_never_publishes_ready(self):
        with tempfile.TemporaryDirectory() as directory, socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen()
            ready = Path(directory) / 'ready'
            result = subprocess.run([sys.executable, str(ROOT / 'verify-authoring-mock-server.py'), '--port', str(listener.getsockname()[1]), '--ready-file', str(ready)], capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(ready.exists())

    def test_authoring_mock_publishes_own_pid_after_bind(self):
        with tempfile.TemporaryDirectory() as directory:
            ready = Path(directory) / 'ready'
            process = subprocess.Popen([sys.executable, str(ROOT / 'verify-authoring-mock-server.py'), '--port', '0', '--ready-file', str(ready)], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
            try:
                deadline = time.monotonic() + 5
                while not ready.exists() and process.poll() is None and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(ready.exists())
                self.assertEqual(ready.read_text(), str(process.pid))
                os.kill(process.pid, 0)
            finally:
                process.terminate()
                process.communicate(timeout=5)

    def test_embedded_runtime_mocks_publish_after_bind(self):
        for filename in ('verify-llm-workspace-execution.sh', 'verify-mcp-stdio-workspace-execution.sh'):
            with self.subTest(filename=filename):
                script = (ROOT / filename).read_text()
                provider = script.split("cat >\"$TMP_DIR/llm_mock_provider.py\" <<'PY'\n", 1)[1].split('\nPY\n', 1)[0]
                with tempfile.TemporaryDirectory() as directory, socket.socket() as listener:
                    listener.bind(('127.0.0.1', 0))
                    listener.listen()
                    ready = Path(directory) / 'ready'
                    env = dict(os.environ, PYTHONPATH=str(ROOT), WORKORCH_VERIFY_MOCK_READY_FILE=str(ready))
                    for key in ('WORKORCH_LLM_VERIFY_MOCK_ADDR', 'WORKORCH_MCP_STDIO_VERIFY_MOCK_ADDR'):
                        env[key] = '127.0.0.1:' + str(listener.getsockname()[1])
                    result = subprocess.run([sys.executable, '-c', provider], env=env, capture_output=True, timeout=10)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse(ready.exists())


if __name__ == '__main__':
    unittest.main()
