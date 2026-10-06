"""仅用临时合成 child 和 pipe 校准观察器，不连接或实现 LSP。"""
from __future__ import annotations

import importlib.util
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import textwrap
import unittest

spec = importlib.util.spec_from_file_location("lsp_bridge_blackbox_producer", Path(__file__).with_name("lsp_bridge_blackbox_producer.py"))
assert spec and spec.loader
producer = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = producer
spec.loader.exec_module(producer)


class ScriptedTransport:
    def __init__(self):
        self.tools = [{"name": name, "inputSchema": {"type": "object", "properties": {}}}
                      for name in sorted(producer.TOOL_NAMES)]
        self.responses = {}

    def list_tools(self):
        return self.tools

    def call_tool(self, name, arguments):
        return self.responses[name]


class Executor:
    def __init__(self, observation):
        self.observation = observation
        self.called = False

    def run(self, grant):
        self.called = True
        return self.observation


class LocationAdapter:
    def definition_locations(self, response):
        # 本测试显式提供的合成 adapter 合同，不从 PRD 推断 wire 字段。
        return response.get("fixtureLocations")


class Measurements:
    def measure(self, name):
        return producer.MeasurementObservation(name, 1.5, "ms", "synthetic", 2)


class ProducerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.pins = {"workspaceId": "ws-x", "runId": "run-x", "startupIdentity": "sha256:" + "a" * 64,
                     "deliveryCommit": "b" * 40, "serviceSourceIdentity": "sha256:" + "c" * 64}
        self.transport = ScriptedTransport()
        self.out = b"synthetic stdout\x00"
        self.observation = producer.ProcessObservation(("/synthetic/check",), str(self.root), 0, self.out, b"stderr", 1.0)
        self.executor = Executor(self.observation)
        self.subject = producer.LSPBridgeProducer(self.transport, self.executor, Measurements(), self.root, self.pins)

    def test_public_tool_surface_is_exact_partial_fact_not_prd_pass(self):
        fact = self.subject.inspect_tool_surface()
        self.assertEqual(set(fact["observedNames"]), producer.TOOL_NAMES)
        self.assertEqual(fact["schemaJudgement"], "observed_only; PRD defines no per-tool input-schema contract")
        self.assertTrue(fact["observedInputSchemas"])
        self.assertEqual(self.subject.partial_facts[0]["status"], "VERIFIED_PARTIAL")
        report = self.subject.finish()
        lsp01 = next(row for row in report["requirements"] if row["id"] == "LSP-01")
        self.assertEqual(lsp01["status"], "NOT_VERIFIED")
        self.assertTrue(lsp01["partialFacts"])
        self.assertFalse(report["acceptanceEstablished"])
        self.assertTrue(all(row["status"] == "NOT_VERIFIED" for row in report["requirements"]))

    def test_tool_surface_drift_and_duplicate_names_rejected(self):
        for rows, expected in ((self.transport.tools[:-1], "names drift"),
                               (self.transport.tools + [self.transport.tools[0]], "duplicate"),
                               ([{"name": "x", "inputSchema": []}], "names drift")):
            transport = ScriptedTransport()
            transport.tools = rows
            subject = producer.LSPBridgeProducer(transport, self.executor, Measurements(), self.root, self.pins)
            with self.subTest(expected=expected), self.assertRaisesRegex(producer.ProducerError, expected):
                subject.inspect_tool_surface()

    def test_tool_documentation_requires_explicit_delivered_file_and_hash(self):
        docs = self.root / "DELIVERED.txt"
        docs.write_text("Tools: " + " ".join(sorted(producer.TOOL_NAMES)), encoding="utf-8")
        fact = self.subject.verify_documented_tool_names([docs.name])
        self.assertFalse(fact["missingToolNames"])
        self.assertEqual(fact["documentHashes"][docs.name], "sha256:" + hashlib.sha256(docs.read_bytes()).hexdigest())
        outside = Path(self.temp.name).parent / (Path(self.temp.name).name + "-outside.txt")
        self.addCleanup(outside.unlink)
        outside.write_text("x", encoding="utf-8")
        with self.assertRaisesRegex(producer.ProducerError, "escapes delivery"):
            self.subject.verify_documented_tool_names(["../" + outside.name])
        self.assertEqual(self.subject.finish()["requirements"][0]["status"], "NOT_VERIFIED")

    def test_raw_response_parse_and_definition_fact_against_explicit_fixture_target(self):
        raw = {"content": [{"type": "text", "text": json.dumps({"items": [{"path": "target.go", "line": 2, "col": 4}], "complete": True, "truncated": False, "elapsed_ms": 3})}]}
        self.transport.responses["lsp_definition"] = raw
        payload = self.subject.observe_tool_call("definition", "lsp_definition", {"path": "source.go", "line": 1, "col": 1})
        self.assertTrue(payload["complete"])
        self.assertFalse(payload["truncated"])
        fact = self.subject.verify_definition_result({"fixtureLocations": [{"path": "target.go", "line": 2, "column": 4}]}, "target.go", LocationAdapter())
        self.assertTrue(fact["expectedFixturePathMatched"])
        wrong = {"fixtureLocations": [{"path": "wrong.go", "line": 0, "column": 0}]}
        with self.assertRaisesRegex(producer.ProducerError, "fixture target"):
            self.subject.verify_definition_result(wrong, "target.go", LocationAdapter())
        with self.assertRaisesRegex(producer.ProducerError, "explicit response adapter"):
            self.subject.verify_definition_result({"items": []}, "target.go")
        with self.assertRaisesRegex(producer.ProducerError, "adapter location"):
            self.subject.verify_definition_result({"fixtureLocations": [{"path": "x", "line": True, "column": 0}]}, "x", LocationAdapter())

    def test_tool_errors_and_malformed_result_cannot_be_pass(self):
        cases = [
            ({"isError": True, "content": [{"type": "text", "text": "bad"}]}, "isError"),
            ({"content": [{"type": "text", "text": "no-json"}]}, "not JSON"),
            ({"content": [{"type": "text", "text": "{}"}, {"type": "text", "text": "{}"}]}, "exactly one"),
        ]
        for response, error in cases:
            self.transport.responses["lsp_definition"] = response
            with self.subTest(error=error), self.assertRaisesRegex(producer.ProducerError, error):
                self.subject.observe_tool_call("bad", "lsp_definition", {})

    def test_explicit_process_grant_captures_real_child_observation_bytes(self):
        grant = producer.CommandGrant(("/synthetic/check",), str(self.root), (), (("/synthetic/check",),), (), 1.0)
        observed = self.subject.run_child("child", grant)
        self.assertTrue(self.executor.called)
        self.assertEqual(observed.stdout, self.out)
        self.assertEqual(self.subject.artifacts[0].sha256, "sha256:" + hashlib.sha256(self.out).hexdigest())
        self.assertEqual((self.root / self.subject.artifacts[0].relative_path).read_bytes(), self.out)
        bad = producer.CommandGrant(("/other",), str(self.root), (), (("/synthetic/check",),), (), 1.0)
        with self.assertRaisesRegex(producer.ProducerError, "not explicitly granted"):
            self.subject.run_child("bad", bad)
        outside = producer.CommandGrant(("/synthetic/check",), "/tmp", (), (("/synthetic/check",),), (), 1.0)
        with self.assertRaisesRegex(producer.ProducerError, "outside"):
            self.subject.run_child("bad", outside)

    def test_grant_rejects_malformed_env_and_nonfinite_or_boolean_timeout(self):
        argv = ("/synthetic/check",)
        for env in ((("KEY", 1),), (("KEY", "x"), ("KEY", "y")), (("BAD=KEY", "x"),)):
            with self.subTest(env=env), self.assertRaises(producer.ProducerError):
                producer.CommandGrant(argv, str(self.root), env, (argv,), ("KEY",), 1).validate(self.root)
        for timeout in (True, 0, -1, float('nan'), float('inf')):
            with self.subTest(timeout=timeout), self.assertRaisesRegex(producer.ProducerError, "positive and finite"):
                producer.CommandGrant(argv, str(self.root), (), (argv,), (), timeout).validate(self.root)

    def test_decoded_transport_does_not_fabricate_raw_wire(self):
        self.transport.responses["synthetic"] = {"content": [{"type": "text", "text": '{"ok":true}'}]}
        self.subject.observe_tool_call("decoded", "synthetic", {})
        report = self.subject.finish()
        self.assertFalse(report["transportObservation"]["rawWireObserved"])
        self.assertEqual(report["exchangeArtifacts"], [])
        self.assertIsNone(report["probes"][0]["request_wire_ref"])
        self.assertIsNone(report["probes"][0]["response_wire_ref"])

    def test_artifact_collision_never_overwrites_bytes_and_directory_is_private(self):
        self.subject._artifact("exact", b"first\x00")
        path = self.root / self.subject.artifacts[0].relative_path
        with self.assertRaisesRegex(producer.ProducerError, "collision"):
            self.subject._artifact("exact", b"second")
        self.assertEqual(path.read_bytes(), b"first\x00")
        self.assertEqual(path.parent.stat().st_mode & 0o777, 0o700)
        self.assertEqual(path.stat().st_mode & 0o777, 0o600)
        self.assertEqual(len(self.subject.artifacts), 1)
        self.assertEqual(list(path.parent.glob('.pending-*')), [])

    def test_document_symlink_and_raw_traversal_rejected(self):
        document = self.root / "delivered.txt"
        document.write_text("Tools", encoding="utf-8")
        link = self.root / "linked.txt"
        link.symlink_to(document)
        with self.assertRaisesRegex(producer.ProducerError, "symlink"):
            self.subject.verify_documented_tool_names([link.name])
        with self.assertRaisesRegex(producer.ProducerError, "escapes delivery"):
            self.subject.verify_documented_tool_names(["dir/../delivered.txt"])

    def test_only_external_identities_and_partial_output_serialized(self):
        with self.assertRaisesRegex(producer.ProducerError, "external identity"):
            producer.LSPBridgeProducer(self.transport, self.executor, Measurements(), self.root, {"runId": "x"})
        self.subject.measure("measure", "latency")
        self.pins["runId"] = "tampered-external-dict"
        report = json.loads(producer.encode_json(self.subject.finish()))
        self.assertEqual(report["runId"], "run-x")
        self.assertEqual(len(report["requirements"]), 20)
        self.assertEqual(next(row for row in report["requirements"] if row["id"] == "LSP-09")["status"], "NOT_VERIFIED")


class StdioJSONRPCClientTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.script = self.root / "temporary_child.py"

    def grant(self, source, timeout=1.0):
        self.script.write_text(textwrap.dedent(source), encoding="utf-8")
        argv = (sys.executable, str(self.script))
        return producer.CommandGrant(argv, str(self.root), (), (argv,), (), timeout)

    def test_stdio_jsonrpc_roundtrip_with_ids(self):
        grant = self.grant('''
            import json, sys
            for raw in sys.stdin:
                req = json.loads(raw)
                if req.get("method") == "notifications/initialized": continue
                method = req["method"]
                if method == "initialize": result = {"protocolVersion": "2024-11-05", "capabilities": {}}
                elif method == "tools/list": result = {"tools": [{"name": "synthetic"}]}
                else: result = {"content": [{"type": "text", "text": "{\\\"ok\\\":true}"}]}
                print(json.dumps({"jsonrpc": "2.0", "id": req["id"], "result": result}), flush=True)
        ''')
        client = producer.StdioJSONRPCClient(grant, self.root)
        try:
            self.assertEqual(client.initialize()["protocolVersion"], "2024-11-05")
            self.assertEqual(client.list_tools()[0]["name"], "synthetic")
            self.assertEqual(client.call_tool("synthetic", {}), {"content": [{"type": "text", "text": '{"ok":true}'}]})
            requests = [row for row in client.exchanges if "id" in row["request"]]
            self.assertEqual([row["request"]["id"] for row in requests], [1, 2, 3])
            self.assertEqual([row["response"]["id"] for row in requests], [1, 2, 3])
            notification = client.exchanges[1]
            self.assertEqual(notification["request"]["method"], "notifications/initialized")
            self.assertEqual(notification["rawRequest"], notification["rawSent"])
        finally:
            client.close()
        report = self.subject(client).finish()
        notification_row = report["transportObservation"]["exchanges"][1]
        self.assertIsNone(notification_row["rawResponseRef"])
        self.assertTrue(notification_row["requestWriteComplete"])
        self.assertEqual(self.artifact_bytes(report, notification_row["rawSentRef"]), notification["rawRequest"])

    def test_id_mismatch_and_malformed_protocol_preserve_partial_bytes(self):
        grant = self.grant('''
            import json, sys
            for raw in sys.stdin:
                req = json.loads(raw)
                print(json.dumps({"jsonrpc":"2.0", "id":999, "result":{}}), flush=True)
        ''')
        client = producer.StdioJSONRPCClient(grant, self.root)
        try:
            with self.assertRaisesRegex(producer.ProducerError, "id mismatch"):
                client.request("initialize", {})
            self.assertIn(b'"id": 999', client.exchanges[0]["partialStdout"])
        finally:
            client.close()
        grant = self.grant("import sys\nfor line in sys.stdin:\n print('not-json', flush=True)\n")
        client = producer.StdioJSONRPCClient(grant, self.root)
        try:
            with self.assertRaisesRegex(producer.ProducerError, "malformed/partial"):
                client.request("initialize", {})
            self.assertIn(b"not-json", client.exchanges[0]["partialStdout"])
        finally:
            client.close()

    def subject(self, client):
        pins = {key: "synthetic-" + key for key in producer.IDENTITY_KEYS}
        return producer.LSPBridgeProducer(client, Executor(None), Measurements(), self.root, pins)

    def artifact_bytes(self, report, ref):
        artifact = next(row for row in report["artifacts"] if row["ref"] == ref)
        raw = (self.root / artifact["relative_path"]).read_bytes()
        self.assertEqual(artifact["byte_length"], len(raw))
        self.assertEqual(artifact["sha256"], "sha256:" + hashlib.sha256(raw).hexdigest())
        return raw

    def test_actual_envelope_to_producer_and_noncanonical_wire_final_artifacts(self):
        grant = self.grant('''
            import json, sys
            for raw in sys.stdin:
                req = json.loads(raw)
                result = {"content": [{"type":"text", "text":'{"marker":"Ω"}'}]}
                line = json.dumps({"result": result, "id":req["id"], "jsonrpc":"2.0"}, ensure_ascii=False, indent=None)
                sys.stderr.buffer.write(b"diagnostic\\x00\\xff")
                sys.stderr.buffer.flush()
                sys.stdout.buffer.write(("  " + line + " \\r\\n").encode("utf-8"))
                sys.stdout.buffer.flush()
        ''')
        client = producer.StdioJSONRPCClient(grant, self.root)
        subject = self.subject(client)
        try:
            self.assertEqual(subject.observe_tool_call("unicode", "synthetic", {}), {"marker": "Ω"})
            with self.assertRaisesRegex(producer.ProducerError, "close granted"):
                subject.finish()
        finally:
            client.close()
        report = subject.finish()
        observed = report["transportObservation"]
        self.assertTrue(observed["streamsComplete"])
        exchange = observed["exchanges"][0]
        wire = self.artifact_bytes(report, exchange["rawResponseRef"])
        self.assertTrue(wire.startswith(b'  {"result":'))
        self.assertTrue(wire.endswith(b" \r\n"))
        self.assertEqual(wire, client.exchanges[0]["rawResponse"])
        self.assertEqual(self.artifact_bytes(report, observed["stdoutRef"]), wire)
        self.assertEqual(self.artifact_bytes(report, observed["stderrRef"]), b"diagnostic\x00\xff")
        self.assertEqual(self.artifact_bytes(report, exchange["rawRequestRef"]), client.exchanges[0]["rawSent"])
        self.assertEqual(report["probes"][0]["response_wire_ref"], exchange["rawResponseRef"])
        self.assertEqual(subject.finish(), report)
        detached = subject.finish()
        detached["transportObservation"]["exchanges"][0]["request"]["method"] = "tampered"
        self.assertEqual(subject.finish(), report)
        with self.assertRaisesRegex(producer.ProducerError, "finalized"):
            subject.observe_tool_call("late", "synthetic", {})
        self.assertFalse(report["acceptanceEstablished"])

    def test_every_rejected_response_retains_exact_wire_in_final_report(self):
        cases = [
            (b"not-json\n", "malformed/partial"),
            (b' {"jsonrpc":"2.0","id":9,"result":{}} \n', "id mismatch"),
            (b'{"jsonrpc":"2.0","id":1,"error":{"message":"failure"}}\n', "JSON-RPC error"),
            (b'{"jsonrpc":"2.0","id":1,"result":{},"result":{}}\n', "duplicate JSON"),
            (b'{"jsonrpc":"2.0","id":1,"result":{"x":NaN}}\n', "non-finite"),
            (b'{"jsonrpc":"2.0","id":1,"result":{"x":"\\ud800"}}\n', "malformed/partial"),
            (b'\xff\n', "malformed/partial"),
            (b'{"jsonrpc":"2.0","id":1,"result":{}}', "missing newline"),
        ]
        for wire, error in cases:
            with self.subTest(wire=wire):
                grant = self.grant(f'import sys\nsys.stdin.buffer.readline()\nsys.stdout.buffer.write({wire!r})\nsys.stdout.buffer.flush()\n')
                client = producer.StdioJSONRPCClient(grant, self.root)
                subject = self.subject(client)
                try:
                    with self.assertRaisesRegex(producer.ProducerError, error):
                        subject.observe_tool_call("rejected", "synthetic", {})
                finally:
                    client.close()
                report = subject.finish()
                row = report["transportObservation"]["exchanges"][0]
                self.assertEqual(self.artifact_bytes(report, row["rawResponseRef"]), wire)
                self.assertEqual(self.artifact_bytes(report, report["transportObservation"]["stdoutRef"]), wire)
                self.assertTrue(row["error"])
                self.assertFalse(report["acceptanceEstablished"])

    def test_timeout_partial_without_newline_is_not_lost_or_replayed(self):
        prefix = b'{"jsonrpc":"2.0", "raw": "\xff'
        grant = self.grant(f'import sys,time\nsys.stdin.buffer.readline()\nsys.stdout.buffer.write({prefix!r})\nsys.stdout.buffer.flush()\nsys.stderr.buffer.write(b"error-prefix\\x00")\nsys.stderr.buffer.flush()\ntime.sleep(10)\n', timeout=0.5)
        client = producer.StdioJSONRPCClient(grant, self.root)
        subject = self.subject(client)
        try:
            with self.assertRaisesRegex(producer.ProducerError, "timeout"):
                subject.observe_tool_call("partial", "synthetic", {})
            self.assertEqual(client.exchanges[0]["partialStdout"], prefix)
            with self.assertRaisesRegex(producer.ProducerError, "no automatic replay"):
                client.request("tools/call", {})
            self.assertEqual(client.exchanges[1]["rawSent"], b"")
        finally:
            client.close()
        report = subject.finish()
        observed = report["transportObservation"]
        self.assertEqual(self.artifact_bytes(report, observed["stdoutRef"]), prefix)
        self.assertEqual(self.artifact_bytes(report, observed["stderrRef"]), b"error-prefix\x00")
        self.assertIsNone(observed["exchanges"][0]["rawResponseRef"])
        self.assertEqual(self.artifact_bytes(report, observed["exchanges"][0]["partialStdoutRef"]), prefix)
        self.assertFalse(observed["exchanges"][1]["requestWriteComplete"])

    def test_eof_and_preexited_child_still_publish_request_and_empty_streams(self):
        grant = self.grant('import sys\nsys.stdin.buffer.readline()\n')
        client = producer.StdioJSONRPCClient(grant, self.root)
        subject = self.subject(client)
        try:
            with self.assertRaisesRegex(producer.ProducerError, "unavailable"):
                subject.observe_tool_call("eof", "synthetic", {})
        finally:
            client.close()
        report = subject.finish()
        observed = report["transportObservation"]
        self.assertEqual(self.artifact_bytes(report, observed["stdoutRef"]), b"")
        self.assertTrue(observed["exchanges"][0]["requestWriteComplete"])
        self.assertIsNone(observed["exchanges"][0]["rawResponseRef"])

    def test_close_stops_readers_when_external_pipe_holder_has_not_sent_eof(self):
        # 本测试仅创建 pipe，不启动额外进程；写端由测试持有以模拟未到 EOF。
        from unittest import mock
        import os
        import time
        read_out, write_out = os.pipe()
        read_err, write_err = os.pipe()
        read_in, write_in = os.pipe()
        process = mock.Mock()
        process.stdin = os.fdopen(write_in, "wb", buffering=0)
        process.stdout = os.fdopen(read_out, "rb", buffering=0)
        process.stderr = os.fdopen(read_err, "rb", buffering=0)
        process.poll.return_value = 0
        with mock.patch.object(producer.subprocess, "Popen", return_value=process):
            client = producer.StdioJSONRPCClient(self.grant('unused', timeout=0.05), self.root)
        try:
            os.write(write_out, b"partial-wire\x00")
            deadline = time.monotonic() + 1
            while not client._stdout and time.monotonic() < deadline:
                time.sleep(0.001)
            client.close()
            self.assertFalse(client._out_thread.is_alive())
            self.assertFalse(client._err_thread.is_alive())
            report = self.subject(client).finish()
            observed = report["transportObservation"]
            self.assertFalse(observed["streamsComplete"])
            self.assertTrue(observed["incompleteReason"])
            self.assertEqual(self.artifact_bytes(report, observed["stdoutRef"]), b"partial-wire\x00")
            process.terminate.assert_not_called()
            process.kill.assert_not_called()
        finally:
            client.close()
            for fd in (write_out, write_err, read_in):
                os.close(fd)

    def test_reader_failure_cannot_mark_partial_stream_complete(self):
        client = producer.StdioJSONRPCClient(self.grant('import sys\nsys.stdin.buffer.readline()\n'), self.root)
        subject = self.subject(client)
        client.reader_failures.append("synthetic read failure")
        client.close()
        report = subject.finish()
        observed = report["transportObservation"]
        self.assertFalse(observed["streamsComplete"])
        self.assertEqual(observed["readerFailures"], ["synthetic read failure"])
        self.assertTrue(observed["incompleteReason"])
        self.assertFalse(report["acceptanceEstablished"])

    def test_child_not_reading_stdin_has_bounded_partial_write(self):
        grant = self.grant('import time\ntime.sleep(10)\n', timeout=0.1)
        client = producer.StdioJSONRPCClient(grant, self.root)
        subject = self.subject(client)
        try:
            with self.assertRaisesRegex(producer.ProducerError, "stdin timeout"):
                subject.observe_tool_call("blocked-write", "synthetic", {"large": "Ω" * 100_000})
            row = client.exchanges[0]
            self.assertGreater(len(row["rawSent"]), 0)
            self.assertLess(len(row["rawSent"]), len(row["rawRequest"]))
            self.assertTrue(row["rawRequest"].startswith(row["rawSent"]))
        finally:
            client.close()
        report = subject.finish()
        row = report["transportObservation"]["exchanges"][0]
        self.assertFalse(row["requestWriteComplete"])
        self.assertEqual(self.artifact_bytes(report, row["rawSentRef"]), client.exchanges[0]["rawSent"])

    def test_rpc_timeout_preserves_request_then_closes_only_granted_child(self):
        grant = self.grant("import sys, time\nfor line in sys.stdin:\n time.sleep(1)\n", timeout=0.05)
        client = producer.StdioJSONRPCClient(grant, self.root)
        child_pid = client.process.pid
        try:
            with self.assertRaisesRegex(producer.ProducerError, "timeout"):
                client.request("initialize", {})
            self.assertEqual(client.exchanges[0]["request"]["id"], 1)
        finally:
            client.close()
        self.assertIsNotNone(client.process.poll())
        self.assertGreater(child_pid, 0)


if __name__ == "__main__":
    unittest.main()
