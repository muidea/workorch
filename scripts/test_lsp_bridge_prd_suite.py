"""只使用临时 Python synthetic child 校准；从不运行真实 LSP 或 Go/npm。"""
from __future__ import annotations

import base64
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import time
from typing import Callable, Protocol
import unittest

# worktree 未包含主尚未提交的 raw producer。只有用户授权的 exact 路径可读取。
RAW_PRODUCER = Path(__file__).with_name("lsp_bridge_blackbox_producer.py")
if not RAW_PRODUCER.exists():
    RAW_PRODUCER = Path("/home/rangh/aispace/workorch/scripts/lsp_bridge_blackbox_producer.py")
spec = importlib.util.spec_from_file_location("lsp_bridge_blackbox_producer", RAW_PRODUCER)
assert spec and spec.loader
bb = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = bb
spec.loader.exec_module(bb)
spec = importlib.util.spec_from_file_location("lsp_bridge_prd_suite", Path(__file__).with_name("lsp_bridge_prd_suite.py"))
assert spec and spec.loader
suite = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = suite
spec.loader.exec_module(suite)


# 合成 child 仅为响应脚本，不是 LSP-Bridge 实现。其状态演进用来校准观察断言。
CHILD = r'''
import json,sys
from pathlib import Path
from urllib.parse import unquote,urlparse
instances={}; unsaved={}; counter=0
names=['lsp_initialize','lsp_sync','lsp_discard_unsaved','lsp_definition','lsp_hover','lsp_diagnostics','lsp_references','lsp_status','lsp_shutdown','lsp_repair']
for raw in sys.stdin:
 req=json.loads(raw); method=req.get('method')
 if method=='notifications/initialized': continue
 if method=='initialize': result={'protocolVersion':'2024-11-05','capabilities':{}}
 elif method=='tools/list': result={'tools':[{'name':n,'inputSchema':{'type':'object'}} for n in names]}
 else:
  name=req['params']['name']; a=req['params']['arguments']; key=(a.get('root'),a.get('language'))
  path=a.get('path',''); path=unquote(urlparse(path).path) if path.startswith('file:') else path
  if name=='lsp_initialize':
   if key not in instances:
    counter+=1; instances[key]={'root':key[0],'language':key[1],'id':str(counter),'state':'ready','activity':'idle','restartCount':0,'dependencyPresent':True,'dependencyRunning':True}
   payload={'instance':instances[key]}
  elif name=='lsp_status': payload={'instances':list(instances.values())}
  elif name=='lsp_shutdown':
   if a.get('all'): instances.clear()
   else: instances.pop(key,None)
   payload={'closed':True}
  elif name=='lsp_sync': unsaved[path]=a['content']; payload={'version':1}
  elif name=='lsp_discard_unsaved': unsaved.pop(path,None); payload={'version':2}
  elif name=='lsp_definition':
   positions={'go':(1,5),'rust':(0,3),'python':(0,4),'typescript':(0,9),'shellscript':(0,0)}
   line,col=positions[a['language']]
   payload={'locations':[{'path':path,'line':line,'column':col}]}
  elif name=='lsp_hover':
   text='replacement' if 'replacement' in unsaved.get(path,'') else 'marker'
   payload={'text':text,'limit':16,'total':len(text),'complete':True,'truncated':False,'reason':'within_limit'}
  elif name=='lsp_diagnostics':
   text=unsaved.get(path,'')
   line=2 if 'replacement' in text else 0
   items=[{'message':'synthetic syntax error','line':line,'column':11,'severity':'error'}] if '(:' in text or 'replacement(' in text else []
   payload={'items':items,'limit':2,'total':len(items),'complete':True,'truncated':False,'reason':'within_limit'}
  elif name=='lsp_references': payload={'items':[],'limit':2,'total':0,'complete':True,'truncated':False,'reason':'within_limit'}
  else: payload={'actions':[],'executed':[]}
  result={'content':[{'type':'text','text':json.dumps(payload)}]}
 sys.stdout.write('  '+json.dumps({'jsonrpc':'2.0','id':req['id'],'result':result})+' \r\n');sys.stdout.flush()
'''


def profile():
    keys = ("root", "language", "path", "line", "column", "content", "all", "apply")
    arguments = {tool: {key: key for key in keys} for tool in bb.TOOL_NAMES}
    fields = {"definition.locations": "/locations", "location.path": "/path", "location.line": "/line", "location.column": "/column",
              "status.instances": "/instances", "repair.actions": "/actions", "repair.executed": "/executed", "action.kind": "/kind",
              "diagnostic.message": "/message", "diagnostic.line": "/line", "diagnostic.column": "/column", "diagnostic.severity": "/severity",
              "config.runtime": "/runtime", "config.performance": "/performance", "config.languageCommands": "/languageCommands"}
    for field in ("root", "language", "id", "state", "activity", "restartCount", "dependencyPresent", "dependencyRunning"):
        fields["instance." + field] = "/" + field
    for kind in ("hover", "diagnostics", "references"):
        for field in ("text" if kind == "hover" else "items", "limit", "total", "complete", "truncated", "reason"):
            fields[kind + "." + field] = "/" + field
    return suite.Profile("synthetic-explicit-wire/v1", arguments, fields)


GO_CALIBRATION_GRANTS: tuple[Path, str] | None = None


class GrantView(Protocol):
    argv: tuple[str, ...]
    cwd: str


class SyntheticExecutor:
    """明确标记的合成 process stream，不得以 real execution 调用。"""
    def __init__(self):
        self.calls = []
        self.responses = {}
        self.input = None
        self.on_run: Callable[[GrantView], object] | None = None

    def run(self, grant):
        self.calls.append(grant)
        if self.on_run is not None:
            self.on_run(grant)
        output, err, code = self.responses.get(grant.argv, (b"ok\texample.invalid/pkg\t0.1s\n", b"", 0))
        return bb.ProcessObservation(grant.argv, grant.cwd, code, output, err, 1.0)

    def run_with_input(self, grant, raw):
        self.input = bb._strict_json(raw)
        responses = []
        for request in self.input["requests"]:
            args = request["params"]["arguments"]
            payload = {"locations": [{"path": args["path"], "line": 1, "column": 5}]} if request["params"]["name"] == "lsp_definition" else {"text": "marker"}
            reply = {"jsonrpc": "2.0", "id": request["id"], "result": {"content": [{"type": "text", "text": json.dumps(payload)}]}}
            responses.append(json.dumps({"request": base64.b64encode(bb.encode_json(request)).decode(), "response": base64.b64encode(bb.encode_json(reply)).decode(),
                                         "sentNs": request["id"], "receivedNs": request["id"] + 1000}))
        return bb.ProcessObservation(grant.argv, grant.cwd, 0, ("\n".join(responses)+"\n").encode(), b"", 1.0)


class NeverMeasure:
    def measure(self, name):
        raise AssertionError("不能使用 selfreported measurement")


class Calibration(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        child = self.root / "synthetic.py"
        child.write_text(CHILD, encoding="utf-8")
        argv = (sys.executable, str(child))
        self.grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 1.0)
        self.client = bb.StdioJSONRPCClient(self.grant, self.root)
        self.addCleanup(self.client.close)
        self.client.initialize()
        pins = {key: "synthetic-" + key for key in bb.IDENTITY_KEYS}
        self.executor = SyntheticExecutor()
        self.producer = bb.LSPBridgeProducer(self.client, self.executor, NeverMeasure(), self.root, pins)
        purposes = frozenset({"fixtures", "empty-environment", "syntax", "unsaved", "shutdown", "engineering", "reproducibility",
                              "uri", "overflow", "configuration", "performance", "rss", "restart", "owned-child-kill", "repair", "real-go", "concurrency", "lifecycle", "timeout"})
        resources = suite.ResourceGrant(purposes, 10000, 1.0, {lang: self.grant for lang in suite.LANGUAGES})
        self.plan = suite.SuitePlan(resources=resources, build_outputs=("synthetic-build.bin",))
        self.executor.on_run = lambda grant: (self.root / "synthetic-build.bin").write_bytes(b"synthetic-build-output") if "build" in grant.argv else None
        for section, terms in suite.DOC_SECTIONS.items():
            path = section + ".txt"
            (self.root / path).write_text("合成校准文档不是真实验收。" + " ".join(terms) + " " + " ".join(sorted(bb.TOOL_NAMES)), encoding="utf-8")
            self.plan.documents[section] = path
        self.subject = suite.PRDSuite(self.producer, profile(), self.plan, execution="synthetic")

    def test_actual_raw_child_positive_functional_observations(self):
        for index in (1, 2, 4, 6, 7, 14, 19):
            with self.subTest(index=index):
                result = getattr(self.subject, f"check_{index:02d}")()
                self.assertIsInstance(result, dict)
        self.assertTrue(self.client.exchanges)
        self.assertTrue(all(e["rawSent"] == e["rawRequest"] for e in self.client.exchanges))
        self.assertTrue(self.client.exchanges[-1]["rawResponse"].startswith(b"  "))

    def test_full_entrypoint_produces_20_non_skeleton_rows_and_exact_raw_artifacts(self):
        report = self.subject.run()
        self.assertEqual(len(report["requirements"]), 20)
        passed = {row["id"] for row in report["requirements"] if row["status"] == "SYNTHETIC_PASS"}
        self.assertTrue({"LSP-01", "LSP-02", "LSP-04", "LSP-06", "LSP-07", "LSP-14", "LSP-19"} <= passed)
        self.assertFalse(report["acceptanceEstablished"])
        self.assertEqual(report["scope"], "synthetic-calibration-not-real-acceptance")
        self.assertFalse(any(row["status"] == "PASS" for row in report["requirements"]))
        raw = report["rawObservations"]
        self.assertTrue(raw["transportObservation"]["streamsComplete"])
        for artifact in raw["artifacts"]:
            content = (self.root / artifact["relative_path"]).read_bytes()
            self.assertEqual(suite.digest(content), artifact["sha256"])
            self.assertEqual(len(content), artifact["byte_length"])
        self.assertEqual(self.subject.finish(), report)

    def test_profile_is_selection_not_pass_adapter_and_wrong_position_fails(self):
        f = self.subject.init("Go")
        for loc in ({"path": f["path"], "line": 0, "column": 5}, {"path": f["path"], "line": True, "column": 5}, {"path": "other.go", "line": 1, "column": 5}):
            with self.subTest(loc=loc), self.assertRaises(bb.ProducerError):
                self.subject.locations({"locations": [loc], "passed": True}, "definition.locations", f)
        with self.assertRaises(suite.Unverified):
            suite.Profile("missing", {}, {}).get({"passed": True}, "definition.locations")
        with self.assertRaises(bb.ProducerError):
            suite.Profile("wrong", {}, {"definition.locations": "/passed"}).get({}, "definition.locations")

    def test_bounds_positive_and_silent_truncation_wrong_cardinality_negative(self):
        for kind in ("hover", "references", "diagnostics"):
            content_key = "text" if kind == "hover" else "items"
            content = "ab" if kind == "hover" else [{"path": "x"}, {"path": "y"}]
            good = {content_key: content, "limit": 2, "total": 3, "truncated": True, "complete": False, "reason": "limit"}
            self.assertEqual(self.subject.bounded(good, kind, 2, True)["returned"], 2)
            for patch in ({"complete": True}, {"truncated": False}, {"total": True}, {"limit": 3}, {"reason": "none"}, {content_key: content[:1]}):
                with self.subTest(kind=kind, patch=patch), self.assertRaises(bb.ProducerError):
                    self.subject.bounded(good | patch, kind, 2, True)
            normal = good | {"total": 2, "truncated": False, "complete": True, "reason": "within_limit"}
            self.subject.bounded(normal, kind, 2, False)

    def test_diagnostics_need_message_line_column_severity_not_selfreported_true(self):
        good = {"message": "expected syntax", "line": 2, "column": 5, "severity": "error"}
        self.assertTrue(self.subject.diagnostics({"items": [good]}, 2))
        for patch in ({"message": ""}, {"line": 1}, {"column": True}, {"severity": "hint"}, {"severity": True}):
            with self.subTest(patch=patch), self.assertRaises(bb.ProducerError):
                self.subject.diagnostics({"items": [good | patch], "passed": True}, 2)

    def test_missing_grants_are_unverified_without_starting_or_guessing_dependencies(self):
        self.plan.resources = None
        before = len(self.client.exchanges)
        for index in (2, 3, 4, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 20):
            with self.subTest(index=index), self.assertRaises(suite.Unverified):
                getattr(self.subject, f"check_{index:02d}")()
        self.assertEqual(len(self.client.exchanges), before)
        self.assertFalse(self.executor.calls)

    def test_missing_backend_tap_does_not_use_sut_elapsed_or_measurement_source(self):
        with self.assertRaises(suite.Unverified):
            self.subject.check_09()
        with self.assertRaises(suite.Unverified):
            self.subject.check_03()
        with self.assertRaises(suite.Unverified):
            self.subject.check_10()

    def test_empty_env_requires_explicit_grant_and_absolute_lsp_executable(self):
        self.plan.resources = suite.ResourceGrant(frozenset({"fixtures"}), 1000, 1, {"Go": self.grant})
        with self.assertRaisesRegex(bb.ProducerError, "空环境"):
            self.subject.make_fixture("Go")
        argv = ("gopls",)
        grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 1)
        resource = suite.ResourceGrant(frozenset({"fixtures", "empty-environment"}), 1000, 1, {"Go": grant})
        with self.assertRaisesRegex(bb.ProducerError, "绝对路径"):
            resource.validate(self.root)

    def test_document_paths_and_snapshots_never_escape_delivery(self):
        for path in ("../prd.md", "/tmp/a", "a/../b"):
            with self.subTest(path=path), self.assertRaises(bb.ProducerError):
                self.subject.file(path)
        link = self.root / "link"
        link.symlink_to(self.root / "integration.txt")
        with self.assertRaisesRegex(bb.ProducerError, "symlink"):
            self.subject.file("link")

    def test_config_example_positive_and_missing_language_negative(self):
        config = {"runtime": {"idleTTL": 60, "maxInstances": 8},
                  "performance": {"defaultTimeout": 1, "maxReferences": 10, "maxDiagnostics": 10, "maxHoverCharacters": 40},
                  "languageCommands": {lang: ["/synthetic/" + lang] for lang in suite.LANGUAGES}}
        path = self.root / "example.json"
        path.write_text(json.dumps(config))
        self.plan.config_example = path.name
        self.assertEqual(len(self.subject.check_18()["configExample"]), 3)
        del config["languageCommands"]["Go"]
        path.write_text(json.dumps(config))
        with self.assertRaisesRegex(bb.ProducerError, "五语言"):
            self.subject.check_18()

    def command(self, label, argv=None):
        argv = argv or ("/synthetic/go",) + suite.COMMANDS[label][1:]
        return bb.CommandGrant(argv, str(self.root), (), (argv,), (), 1)

    def test_engineering_exact_three_commands_and_empty_skip_failed_negative(self):
        for label in suite.COMMANDS:
            self.plan.command_grants[label] = self.command(label)
        self.assertEqual(set(self.subject.check_16()), set(suite.COMMANDS))
        (self.root / "synthetic-build.bin").unlink()
        for output, err, code in ((b"", b"", 0), (b"skipped", b"", 0), (b"ok", b"WARNING: DATA RACE", 0), (b"ok", b"", 1)):
            (self.root / "synthetic-build.bin").unlink(missing_ok=True)
            grant = self.plan.command_grants["test"]
            self.executor.responses[grant.argv] = (output, err, code)
            with self.subTest(output=output, code=code), self.assertRaises(bb.ProducerError):
                self.subject.check_16()

    def test_real_integration_empty_skip_and_fake_pass_rejected(self):
        grant = self.command("test", ("/synthetic/go", "test", "-json", "-run", "RealGo", "./..."))
        self.plan.real_go_test_grant = grant
        for output in (b'{"passed":true}\n', b'{"Action":"skip","Test":"TestRealGo"}\n', b''):
            self.executor.responses[grant.argv] = (output, b"", 0)
            with self.subTest(output=output), self.assertRaises(suite.Unverified):
                self.subject.check_17()

    def test_goroutine_source_fixed_and_10_raw_id_semantics_are_derived(self):
        path = self.root / "suite_goroutines.go"
        path.write_text(suite.GO_CONCURRENT_HARNESS)
        self.plan.goroutine_grant = self.command("test", ("/synthetic/go", "run", "-race", str(path)))
        self.plan.concurrent_mcp_grant = self.grant
        self.plan.command_grants["race"] = self.command("race")
        trace = self.command("test", ("/synthetic/go", "test", "-race", "-json", "./..."))
        self.plan.command_grants["race-json"] = trace
        self.executor.responses[trace.argv] = (b'{"Action":"run","Test":"TestConcurrent"}\n{"Action":"pass","Test":"TestConcurrent"}\n', b"", 0)
        result = self.subject.check_15()
        self.assertEqual(result["goroutines"], 10)
        self.assertEqual(result["requestIds"], list(range(100, 110)))
        harness_input = self.executor.input
        if harness_input is None:
            self.fail("未观察harness实际输入")
        self.assertEqual(len(harness_input["requests"]), 10)
        path.write_text("package main // fake success")
        with self.assertRaisesRegex(bb.ProducerError, "固定合同"):
            self.subject.check_15()

    def test_actual_granted_python_process_capture_and_timeout_only_owned_child(self):
        script = self.root / "process.py"
        script.write_text("import sys\nsys.stdout.buffer.write(b'raw\\x00')\nsys.stderr.buffer.write(b'err\\xff')\n")
        argv = (sys.executable, str(script))
        grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 1)
        executor = suite.GrantedExecutor(self.root)
        result = executor.run(grant)
        self.assertEqual((result.stdout, result.stderr, result.exit_code), (b"raw\x00", b"err\xff", 0))
        script.write_text("import sys,time\nsys.stdout.buffer.write(b'partial');sys.stdout.flush();time.sleep(5)\n")
        grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 0.1)
        result = executor.run(grant)
        self.assertTrue(result.timed_out)
        self.assertEqual(result.stdout, b"partial")
        self.assertNotEqual(result.exit_code, 0)

    def test_unsaved_didchange_latest_and_discard_with_independent_synthetic_tap(self):
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        self.subject.boundary = recorder
        original = self.client.call_tool
        content = {}
        def observed(name, arguments):
            if name == "lsp_sync" and content.get(arguments["path"]) != arguments["content"]:
                content[arguments["path"]] = arguments["content"]
                raw = bb.encode_json({"jsonrpc": "2.0", "method": "textDocument/didChange", "params": {
                    "textDocument": {"uri": Path(arguments["path"]).as_uri()},
                    "contentChanges": [{"text": arguments["content"]}]}})
                recorder.capture("synthetic-instance", "request", raw, raw)
            return original(name, arguments)
        self.client.call_tool = observed
        result = self.subject.check_03()
        self.assertTrue(result["latestDiagnostic"])
        self.assertEqual(len(recorder.events()), 1)
        # 负向：同内容多发一次 didChange 不能被response自报skip掩盖。
        old_capture = observed
        def duplicate(name, arguments):
            result = old_capture(name, arguments)
            if name == "lsp_sync":
                raw = bb.encode_json({"jsonrpc": "2.0", "method": "textDocument/didChange", "params": {
                    "contentChanges": [{"text": arguments["content"]}]}})
                recorder.capture("synthetic-instance", "request", raw, raw)
            return result
        self.client.call_tool = duplicate
        with self.assertRaises(bb.ProducerError):
            self.subject.check_03()

    def test_warm_timing_independent_monotonic_boundaries_positive_negative(self):
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        self.subject.boundary = recorder
        original = self.client.call_tool
        counter = 0
        def timed(name, arguments):
            nonlocal counter
            if name not in ("lsp_definition", "lsp_hover"):
                return original(name, arguments)
            counter += 1
            method = "textDocument/definition" if name == "lsp_definition" else "textDocument/hover"
            recorder.capture("synthetic", "request", bb.encode_json({"jsonrpc": "2.0", "id": counter, "method": method}), b"frame-request")
            result = original(name, arguments)
            recorder.capture("synthetic", "response", bb.encode_json({"jsonrpc": "2.0", "id": counter, "result": {}}), b"frame-response")
            return result
        self.client.call_tool = timed
        result = self.subject.check_09()
        self.assertTrue(all(len(r["samplesMs"]) == 3 and r["maxMs"] < 50 for r in result.values()))
        def slow(name, arguments):
            result = timed(name, arguments)
            if name in ("lsp_definition", "lsp_hover"):
                time.sleep(0.055)
            return result
        self.client.call_tool = slow
        with self.assertRaisesRegex(bb.ProducerError, "50ms"):
            self.subject.check_09()

    def test_rss_uses_actual_owned_server_proc_not_children_or_response_field(self):
        rss = suite.RssGrant(self.client.process, f"/proc/{self.client.process.pid}/statm", os.sysconf("SC_PAGE_SIZE"))
        self.subject.boundary = suite.GrantedBoundaryRecorder(self.root, self.plan.resources, rss)
        result = self.subject.check_10()
        self.assertEqual(result["pid"], self.client.process.pid)
        self.assertGreater(result["rssMB"], 0)
        self.assertLess(result["rssMB"], 64)
        self.subject.boundary.rss = suite.RssGrant(self.client.process, "/proc/1/statm", 4096)
        with self.assertRaisesRegex(bb.ProducerError, "handle"):
            self.subject.check_10()

    def test_owned_child_exit_recorder_requires_own_handle_and_exact_command(self):
        purposes = self.plan.resources.purposes | {"owned-child-start"}
        script = self.root / "owned.py"
        script.write_text("import time\ntime.sleep(10)\n")
        argv = (sys.executable, str(script))
        grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 1)
        resources = suite.ResourceGrant(purposes, 10000, 1, {"Go": grant})
        recorder = suite.GrantedBoundaryRecorder(self.root, resources)
        self.subject.plan.resources = resources
        self.subject.boundary = recorder
        with self.assertRaises(suite.Unverified):
            recorder.kill_owned("SUT-untrusted-pid")
        child = recorder.start_owned("owned", "Go")
        self.addCleanup(lambda: child.poll() is None and child.kill())
        ref = self.subject.kill_owned("owned")
        self.assertTrue(ref)
        self.assertLess(child.returncode, 0)
        with self.assertRaises(bb.ProducerError):
            self.subject.kill_owned("owned")

    def test_three_output_overflow_and_runtime_config_effect_synthetic(self):
        configured = bb.StdioJSONRPCClient(self.grant, self.root)
        self.addCleanup(configured.close)
        configured.initialize()
        self.plan.configured_transport = configured
        self.plan.configured_launch_grant = self.grant
        config = self.root / "temporary-config.json"
        config.write_text(json.dumps({"runtime": {"idleTTL": 0.1, "maxInstances": 2},
                                     "performance": {k: v for k, v in self.plan.config_values.items() if k not in ("idleTTL", "maxInstances")}}))
        self.plan.configured_input_files = (config.name,)
        original = configured.call_tool
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        event_id = 0
        overflow = False
        delayed = False
        def controlled(name, args):
            nonlocal overflow, delayed, event_id
            if delayed and name == "lsp_definition":
                time.sleep(self.plan.config_values["defaultTimeout"])
                delayed = False
                return {"isError": True, "content": [{"type": "text", "text": "timeout synthetic boundary"}]}
            if name == "lsp_sync": overflow = True
            if name == "lsp_discard_unsaved": overflow = False
            if name in ("lsp_hover", "lsp_diagnostics", "lsp_references"):
                key = {"lsp_hover": "maxHoverCharacters", "lsp_diagnostics": "maxDiagnostics", "lsp_references": "maxReferences"}[name]
                limit = self.plan.config_values[key]
                total = limit + 3 if overflow else 1
                content = "x" * min(total, limit) if name == "lsp_hover" else [{"message": "syntax", "path": args["path"], "line": 0, "column": 1, "severity": "error"}] * min(total, limit)
                event_id += 1
                kind = name.removeprefix("lsp_")
                method = {"hover": "textDocument/hover", "references": "textDocument/references", "diagnostics": "textDocument/diagnostic"}[kind]
                recorder.capture("synthetic-config", "request", bb.encode_json({"jsonrpc": "2.0", "id": event_id, "method": method}), b"frame-request")
                backend_result = {"contents": "x" * total} if kind == "hover" else ({"items": [{"message": "syntax"}] * total} if kind == "diagnostics" else [{"path": args["path"]}] * total)
                recorder.capture("synthetic-config", "response", bb.encode_json({"jsonrpc": "2.0", "id": event_id, "result": backend_result}), b"frame-response")
                payload = {"text" if name == "lsp_hover" else "items": content, "total": total, "limit": limit,
                           "truncated": overflow, "complete": not overflow, "reason": "limit" if overflow else "within_limit"}
                return {"content": [{"type": "text", "text": json.dumps(payload)}]}
            return original(name, args)
        configured.call_tool = controlled
        class DelayBoundary:
            def events(inner): return recorder.events()
            def delay_next_response(inner, seconds):
                nonlocal delayed
                self.assertGreater(seconds, self.plan.config_values["defaultTimeout"])
                delayed = True
        self.plan.configured_boundary = DelayBoundary()
        bounds = self.subject.check_05()
        self.assertEqual(set(bounds["boundedOutputs"]["overflowProbe"]), {"hover", "diagnostics", "references"})
        result = self.subject.check_11()
        self.assertGreater(result["configuration"]["timeoutObservedSeconds"], 0.025)
        config.write_text(json.dumps({"runtime": {}, "performance": {}}))
        with self.assertRaisesRegex(bb.ProducerError, "配置输入值"):
            self.subject.check_05()

    def test_repair_advice_safe_apply_and_next_request_restart_positive(self):
        f = self.subject.init("Go")
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        exited = False
        restarts = 0
        original = self.client.call_tool
        def capture_init():
            recorder.capture("1", "request", bb.encode_json({"jsonrpc": "2.0", "id": 777, "method": "initialize"}), b"init-request")
            recorder.capture("1", "response", bb.encode_json({"jsonrpc": "2.0", "id": 777, "result": {"capabilities": {}}}), b"init-response")
        def controlled(name, args):
            nonlocal exited, restarts
            if name == "lsp_status":
                return {"content": [{"type": "text", "text": json.dumps({"instances": [{
                    "root": f["root"], "language": f["language"], "id": "1", "state": "stopped" if exited else "ready",
                    "activity": "idle", "restartCount": restarts, "dependencyPresent": True, "dependencyRunning": not exited}]})}]}
            if name == "lsp_repair":
                actions = [{"kind": "restart_exited_instance"}] if exited else []
                executed = []
                if args.get("apply") and exited:
                    exited = False; restarts += 1; executed = actions; capture_init()
                return {"content": [{"type": "text", "text": json.dumps({"actions": actions, "executed": executed})}]}
            if name == "lsp_definition" and exited:
                exited = False; restarts += 1; capture_init()
            return original(name, args)
        self.client.call_tool = controlled
        class OwnedSynthetic:
            def events(inner): return recorder.events()
            def kill_owned(inner, instance):
                nonlocal exited
                exited = True
                return bb.ProcessObservation(self.grant.argv, self.grant.cwd, -9, b"synthetic owned exit", b"", 1)
        self.subject.boundary = OwnedSynthetic()
        self.assertTrue(self.subject.check_08()["applySafeOnly"])
        self.assertEqual(self.subject.check_12()["trigger"], "manual_kill")
        # 非安全动作不会被Profile结果的另一个passed字段掩盖。
        def unsafe(name, args):
            response = controlled(name, args)
            if name == "lsp_repair" and args.get("apply"):
                response = {"content": [{"type": "text", "text": json.dumps({"actions": [], "executed": [{"kind": "delete_workspace"}], "passed": True})}]}
            return response
        self.client.call_tool = unsafe
        with self.assertRaises(bb.ProducerError):
            self.subject.check_08()

    def test_lifecycle_synthetic_capacity_lru_ttl_and_only_idle(self):
        # 此明确synthetic场景脚本不代表真实运行时；只校准suite主动过程和断言。
        configured = bb.StdioJSONRPCClient(self.grant, self.root)
        self.addCleanup(configured.close)
        configured.initialize()
        self.plan.configured_transport = configured
        self.plan.configured_launch_grant = self.grant
        path = self.root / "temporary-config.json"
        path.write_text(json.dumps({"runtime": {"idleTTL": 0.1, "maxInstances": 2},
                                    "performance": {k: v for k, v in self.plan.config_values.items() if k not in ("idleTTL", "maxInstances")}}))
        self.plan.configured_input_files = (path.name,)
        active = {}
        held = set()
        original = configured.call_tool
        def controlled(name, args):
            if name == "lsp_initialize":
                if len(active) == 2 and args["root"] not in active:
                    active.pop(min(active, key=lambda k: active[k]["last"]))
                active[args["root"]] = {"root": args["root"], "language": args["language"], "id": args["root"], "state": "ready", "last": time.monotonic()}
            if name == "lsp_definition" and args["root"] in active:
                active[args["root"]]["last"] = time.monotonic()
            if name == "lsp_status":
                for key, row in list(active.items()):
                    if key not in held and time.monotonic() - row["last"] > 0.1:
                        del active[key]
                return {"content": [{"type": "text", "text": json.dumps({"instances": [{k: v for k, v in row.items() if k != "last"} for row in active.values()]})}]}
            return original(name, args)
        configured.call_tool = controlled
        class Future:
            def __init__(inner, args): inner.args = args
            def release(inner): held.remove(inner.args["root"])
            def result(inner):
                envelope = original("lsp_definition", inner.args)
                return json.loads(envelope["content"][0]["text"])
        class HoldBoundary:
            def hold_query(inner, args, transport):
                held.add(args["root"])
                return Future(args)
        self.plan.configured_boundary = HoldBoundary()
        result = self.subject.check_13()
        self.assertTrue(all(result.values()))

    def test_reproducible_snapshot_sources_tests_outputs_positive_and_drift_negative(self):
        source = b"package main\nfunc main() {}\n"
        test = b"package main\nimport \"testing\"\nfunc TestX(t *testing.T) {}\n"
        for directory in ("source", "clean-a", "clean-b"):
            root = self.root / directory
            root.mkdir()
            (root / "main.go").write_bytes(source)
            (root / "main_test.go").write_bytes(test)
        self.executor.on_run = lambda grant: (Path(grant.cwd) / "binary").write_bytes(b"same-synthetic-output")
        self.plan.delivery_inventory = ("main.go", "main_test.go")
        self.plan.resources = suite.ResourceGrant(self.plan.resources.purposes | {"git-snapshot"}, 10000, 1,
                                                 self.plan.resources.language_commands)
        self.plan.git_repository_root = "temporary-git"
        (self.root / "temporary-git").mkdir()
        self.plan.git_executable = "/synthetic/git"
        self.producer.identities["deliveryCommit"] = "a" * 40
        tree = b"100644 blob " + b"b" * 40 + b"\tmain.go\x00" + b"100644 blob " + b"c" * 40 + b"\tmain_test.go\x00"
        grants = []
        for arguments, output in ((("cat-file", "-t", "a" * 40), b"commit\n"),
                                  (("ls-tree", "-rz", "--full-tree", "a" * 40), tree),
                                  (("cat-file", "blob", "b" * 40), source), (("cat-file", "blob", "c" * 40), test)):
            argv = ("/synthetic/git",) + arguments
            grants.append(bb.CommandGrant(argv, str(self.root / "temporary-git"), (), (argv,), (), 1))
            self.executor.responses[argv] = (output, b"", 0)
        self.plan.git_grants = tuple(grants)
        self.plan.source_files = ("source/main.go",)
        self.plan.test_files = ("source/main_test.go",)
        self.plan.snapshot_source_root = "source"
        self.plan.reproducible_outputs = (("clean-a/binary", "clean-b/binary"),)
        for label, directory in (("reproduce-a", "clean-a"), ("reproduce-b", "clean-b")):
            argv = ("/synthetic/go", "build", "-buildvcs=false", "./...")
            self.plan.command_grants[label] = bb.CommandGrant(argv, str(self.root / directory), (), (argv,), (), 1)
        result = self.subject.check_20()
        self.assertEqual(len(result["outputs"]), 1)
        for directory in ("clean-a", "clean-b"):
            (self.root / directory / "binary").unlink()
        (self.root / "clean-b/main_test.go").write_bytes(b"drift")
        with self.assertRaisesRegex(bb.ProducerError, "bytes.*漂移"):
            self.subject.check_20()

    def test_review_lineage_direction_rss_fresh_inventory_and_overlap_negative(self):
        # 非SUT child不能借instance+argv通过真实restart。
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        self.subject.boundary = recorder
        self.subject.execution = "real"
        with self.assertRaises(suite.Unverified):
            self.subject.kill_owned("unbound")
        # direction不能把response伪装request；完整framing原文需保全。
        self.subject.execution = "synthetic"
        recorder.capture("x", "request", bb.encode_json({"jsonrpc": "2.0", "id": 1, "result": {}}), b"Content-Length: 36\r\n\r\nraw")
        with self.assertRaisesRegex(bb.ProducerError, "direction"):
            self.subject.backend()
        # old build artifact不能证明本次真实build。
        (self.root / "synthetic-build.bin").write_bytes(b"old")
        for label in suite.COMMANDS:
            self.plan.command_grants[label] = self.command(label)
        with self.assertRaisesRegex(bb.ProducerError, "预存"):
            self.subject.check_16()
        root = self.root / "inventory"
        root.mkdir()
        (root / "main.go").write_bytes(b"package main")
        (root / "extra").write_bytes(b"x")
        with self.assertRaisesRegex(bb.ProducerError, "extra"):
            self.subject.inventory(root, ("main.go",))

    def test_harness_rejects_serial_ids_even_when_all_results_match(self):
        path = self.root / "suite_goroutines.go"
        path.write_text(suite.GO_CONCURRENT_HARNESS)
        self.plan.goroutine_grant = self.command("test", ("/synthetic/go", "run", "-race", str(path)))
        self.plan.concurrent_mcp_grant = self.grant
        original = self.executor.run_with_input
        def serial(grant, raw):
            result = original(grant, raw)
            lines = []
            for index, line in enumerate(result.stdout.splitlines()):
                row = json.loads(line)
                row["sentNs"], row["receivedNs"] = index * 10, index * 10 + 1
                lines.append(json.dumps(row))
            return bb.ProcessObservation(result.argv, result.cwd, 0, ("\n".join(lines) + "\n").encode(), b"", 1)
        self.executor.run_with_input = serial
        with self.assertRaisesRegex(bb.ProducerError, "pending overlap"):
            self.subject.check_15()

    def test_framing_raw_artifact_and_report_idempotency_preserved(self):
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        raw = b'{"jsonrpc":"2.0","id":7,"method":"synthetic/request"}'
        framing = b"Content-Length: " + str(len(raw)).encode() + b"\r\n\r\n" + raw
        recorder.capture("synthetic", "request", raw, framing)
        self.subject.boundary = recorder
        # 仅finish校准；完整suite入口另有主动20项测试。
        self.subject._finished = True
        result = self.subject.finish()
        artifacts = result["rawObservations"]["artifacts"]
        frames = [a for a in artifacts if "backend-framing" in a["ref"]]
        self.assertEqual(len(frames), 1)
        self.assertEqual((self.root / frames[0]["relative_path"]).read_bytes(), framing)
        self.assertEqual(self.subject.finish(), result)

    def test_actual_forwarding_parent_pipe_and_raw_framing_provenance(self):
        helper_source = Path(__file__).with_name("lsp_bridge_prd_forwarder.py").read_bytes()
        helper = self.root / "lsp_bridge_prd_forwarder.py"
        helper.write_bytes(helper_source)
        backend = self.root / "synthetic_backend.py"
        backend.write_text('''import sys,json\nwhile True:\n header=sys.stdin.buffer.readline()\n if not header: break\n n=int(header.split(b":")[1]);sys.stdin.buffer.readline();raw=sys.stdin.buffer.read(n)\n req=json.loads(raw);body=json.dumps({"jsonrpc":"2.0","id":req["id"],"result":{"marker":"raw-value"}}).encode()\n sys.stdout.buffer.write(b"Content-Length: "+str(len(body)).encode()+b"\\r\\n\\r\\n"+body);sys.stdout.buffer.flush()\n''')
        argv = (sys.executable, str(backend))
        backend_grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 2)
        helper_argv = (sys.executable, str(helper))
        helper_grant = bb.CommandGrant(helper_argv, str(self.root), (), (helper_argv,), (), 2)
        resources = suite.ResourceGrant(frozenset({"empty-environment", "backend-forwarding", "proc-lineage", "owned-child-start", "owned-child-kill", "timeout", "lifecycle"}),
                                       10000, 2, {"Go": backend_grant})
        relay = suite.OwnedLSPForwarder(self.root, resources, helper_grant, helper_source)
        self.addCleanup(relay.close)
        language_grant = relay.route("instance-Go", "Go")
        sut = self.root / "synthetic_sut_parent.py"
        sut.write_text('''import sys,subprocess,json\ncmd=json.loads(sys.argv[1]);child=subprocess.Popen(cmd,stdin=subprocess.PIPE,stdout=subprocess.PIPE)\nraw=b'{"jsonrpc":"2.0","id":7,"method":"textDocument/definition"}'\nframe=b"Content-Length: "+str(len(raw)).encode()+b"\\r\\n\\r\\n"+raw\nchild.stdin.write(frame);child.stdin.flush()\nheader=child.stdout.readline();n=int(header.split(b":")[1]);child.stdout.readline();body=child.stdout.read(n)\nsys.stdout.buffer.write(body+b"\\n");sys.stdout.flush()\nsys.stdin.buffer.readline()\nchild.stdin.close();child.terminate();child.wait()\n''')
        sut_argv = (sys.executable, str(sut), json.dumps(language_grant.argv))
        sut_grant = bb.CommandGrant(sut_argv, str(self.root), (), (sut_argv,), (), 3)
        sut_grant.validate(self.root)
        import subprocess
        process = subprocess.Popen(sut_grant.argv, cwd=sut_grant.cwd, env=dict(sut_grant.env), stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        def cleanup():
            # parent自己仍持有helper Popen，先关闭其stdin触发finally/ownhandle cleanup。
            stream = process.stdin
            if stream is not None and not stream.closed:
                stream.close()
            try:
                process.wait(timeout=3)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=2)
            for stream in (process.stdout, process.stderr):
                if stream is not None:
                    stream.close()
        self.addCleanup(cleanup)
        relay.delay_next_response(0.08)
        relay.attach_mcp("instance-Go", process)
        response_start = time.monotonic()
        import select
        self.assertIsNotNone(process.stdout)
        self.assertIsNotNone(process.stdin)
        output, input_stream = process.stdout, process.stdin
        if output is None or input_stream is None:
            self.fail("synthetic owned parent缺pipe")
        self.assertTrue(select.select([output], [], [], 2)[0], relay.failures)
        reply = bb._strict_json(output.readline())
        self.assertEqual(reply["result"]["marker"], "raw-value")
        self.assertGreaterEqual(time.monotonic() - response_start, 0.075)
        deadline = time.monotonic() + 1
        while len(relay.events()) < 2 and time.monotonic() < deadline: time.sleep(0.002)
        events = relay.events()
        self.assertEqual(len(events), 2)
        self.assertEqual([e.direction for e in events], ["request", "response"])
        for event in events:
            self.assertEqual(suite.frame_body(event.framing, 10000), event.raw)
        lineage = relay.owned_lineage("instance-Go", process)
        self.assertIs(lineage["mcpHandle"], process)
        self.assertIsNone(lineage["childHandle"].poll())
        alive = relay.alive_backends(process, ("Go",))
        self.assertEqual(alive[0]["pid"], lineage["childHandle"].pid)
        with self.assertRaises(suite.Unverified):
            relay.capture("instance-Go", "request", b"{}", b"{}")
        with self.assertRaises(suite.Unverified):
            relay.bind_forwarding("instance-Go", process, 0, 1)
        relay.kill_owned("instance-Go")
        with self.assertRaises(bb.ProducerError):
            relay.alive_backends(process, ("Go",))
        input_stream.write(b"done\n");input_stream.flush()
        process.wait(timeout=2)

    def test_actual_control_eof_crash_restart_notifications_and_cleanup(self):
        helper_bytes = Path(__file__).with_name("lsp_bridge_prd_forwarder.py").read_bytes()
        helper = self.root / "relay_helper.py"
        helper.write_bytes(helper_bytes)
        backend = self.root / "control_backend.py"
        backend.write_text(r'''import sys,json
while True:
 header=sys.stdin.buffer.readline()
 if not header: break
 n=int(header.split(b':')[1]);sys.stdin.buffer.readline();body=sys.stdin.buffer.read(n);req=json.loads(body)
 def send(obj):
  raw=json.dumps(obj,ensure_ascii=False).encode();sys.stdout.buffer.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);sys.stdout.buffer.flush()
 if 'id' not in req:
  send({'jsonrpc':'2.0','method':'textDocument/publishDiagnostics','params':{'uri':req.get('params',{}).get('uri'),'diagnostics':[]}});continue
 send({'jsonrpc':'2.0','id':req['id'],'result':{'uri':req.get('params',{}).get('uri'),'marker':'last-frame'}})
 if req.get('method')=='exitAfterReply': break
''')
        bargv = (sys.executable, str(backend))
        bgrant = bb.CommandGrant(bargv, str(self.root), (), (bargv,), (), 2)
        hargv = (sys.executable, str(helper))
        hgrant = bb.CommandGrant(hargv, str(self.root), (), (hargv,), (), 2)
        resources = suite.ResourceGrant(frozenset({"empty-environment", "backend-forwarding", "proc-lineage", "owned-child-start", "owned-child-kill"}),
                                       20000, 2, {"Go": bgrant})
        relay = suite.OwnedLSPForwarder(self.root, resources, hgrant, helper_bytes)
        self.addCleanup(relay.close)
        route = relay.route("route", "Go")
        directories = [p.parent for p in relay.routes["route"]["paths"]]
        parent = self.root / "control_parent.py"
        parent.write_text(r'''import json,subprocess,sys
argv=json.loads(sys.argv[1]);child=None
def start():
 global child
 child=subprocess.Popen(argv,stdin=subprocess.PIPE,stdout=subprocess.PIPE)
def receive():
 header=child.stdout.readline()
 if not header:return None
 n=int(header.split(b':')[1]);child.stdout.readline();return json.loads(child.stdout.read(n))
start()
for line in sys.stdin:
 cmd=json.loads(line)
 if cmd['op']=='start':start();print('started',flush=True);continue
 if cmd['op']=='crash':child.kill();child.wait();print('crashed',flush=True);continue
 if cmd['op']=='eof':child.stdin.close();child.wait(timeout=2);print('closed',flush=True);continue
 raw=json.dumps(cmd['message'],ensure_ascii=False).encode();child.stdin.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);child.stdin.flush()
 reply=receive();print(json.dumps(reply,ensure_ascii=False),flush=True)
 if cmd['message'].get('method')=='exitAfterReply':child.wait(timeout=2)
if child is not None and child.poll() is None:child.stdin.close();child.wait(timeout=2)
''')
        import subprocess, select
        argv = (sys.executable, str(parent), json.dumps(route.argv))
        grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 5)
        grant.validate(self.root)
        process = subprocess.Popen(grant.argv, cwd=grant.cwd, env=dict(grant.env), stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        def cleanup():
            # parent自己仍持有helper Popen，先关闭其stdin触发finally/ownhandle cleanup。
            stream = process.stdin
            if stream is not None and not stream.closed:
                stream.close()
            try:
                process.wait(timeout=3)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=2)
            for stream in (process.stdout, process.stderr):
                if stream is not None:
                    stream.close()
        self.addCleanup(cleanup)
        relay.attach_mcp("route", process)
        input_stream, output = process.stdin, process.stdout
        if input_stream is None or output is None: self.fail("missing parent pipes")
        def issue(command):
            input_stream.write(bb.encode_json(command));input_stream.flush()
            self.assertTrue(select.select([output], [], [], 3)[0], relay.failures)
            return output.readline().decode().strip()
        def wait_dead():
            deadline=time.monotonic()+2
            while ("route" not in relay._children or relay._children["route"][0].poll() is None) and time.monotonic()<deadline:time.sleep(0.005)
            self.assertIn("route",relay._children)
            self.assertIsNotNone(relay._children["route"][0].poll())
        uri = (self.root / "空 格#%.go").as_uri()
        reply = json.loads(issue({"op":"query","message":{"jsonrpc":"2.0","id":1,"method":"exitAfterReply","params":{"uri":uri}}}))
        self.assertEqual(reply["result"], {"uri":uri,"marker":"last-frame"})
        wait_dead()
        first = relay._children["route"][0]
        self.assertEqual(relay.routes["route"]["generation"], 1)
        self.assertEqual(issue({"op":"start"}),"started")
        reply = json.loads(issue({"op":"query","message":{"jsonrpc":"2.0","id":1,"method":"textDocument/definition","params":{"uri":uri}}}))
        self.assertEqual(reply["result"]["uri"], uri)
        self.assertEqual(relay.routes["route"]["generation"], 2)
        self.assertIsNot(relay._children["route"][0], first)
        # 双向notification没有被响应ID匹配误判；source/generation独立来自转发点。
        notification = json.loads(issue({"op":"query","message":{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"uri":uri}}}))
        self.assertEqual(notification["method"],"textDocument/publishDiagnostics")
        self.assertEqual(issue({"op":"crash"}),"crashed")
        wait_dead()
        self.assertEqual(issue({"op":"start"}),"started")
        json.loads(issue({"op":"query","message":{"jsonrpc":"2.0","id":2,"method":"textDocument/definition"}}))
        self.assertEqual(relay.routes["route"]["generation"], 3)
        self.assertEqual(issue({"op":"eof"}),"closed")
        wait_dead()
        input_stream.close()
        process.wait(timeout=2)
        self.assertFalse(relay.failures)
        events=relay.events()
        self.assertEqual({e.generation for e in events}, {1,2,3})
        self.assertTrue(any(e.source=="backend_to_sut" and bb._strict_json(e.raw).get("method")=="textDocument/publishDiagnostics" for e in events))
        self.assertTrue(any(e.source=="sut_to_backend" and bb._strict_json(e.raw).get("method")=="textDocument/didChange" for e in events))
        relay.close()
        self.assertTrue(all(child.poll() is not None for child in relay._owned_history))
        self.assertTrue(all(not path.exists() for path in directories))

    def test_framing_decoder_rejects_raw_mismatch_partial_and_duplicate_length(self):
        raw = b'{"jsonrpc":"2.0","id":1,"result":{}}'
        frame = b"Content-Length: " + str(len(raw)).encode() + b"\r\n\r\n" + raw
        self.assertEqual(suite.frame_body(frame, 10000), raw)
        for bad in (frame[:-1], frame.replace(b"\r\n\r\n", b"\r\nContent-Length: 1\r\n\r\n"), frame + frame):
            with self.subTest(frame=bad), self.assertRaises(bb.ProducerError):
                suite.frame_body(bad, 10000)
        recorder = suite.GrantedBoundaryRecorder(self.root, self.plan.resources)
        self.subject.boundary = recorder
        self.subject.execution = "real"
        recorder.capture("x", "response", raw, frame)
        with self.assertRaises(suite.Unverified): self.subject.backend()
        with self.assertRaises(suite.Unverified): recorder.bind_forwarding("x", self.client.process, 0, 1)

    def test_actual_temporary_git_commit_tree_and_wrong_pin_or_blob_negative(self):
        # 只读/写自有temporary Git；不读取真实交付repo或全环境。
        executable = "/usr/bin/git"
        if not Path(executable).is_file():
            self.skipTest("temporary Git executable未安装；不下载安装")
        repo = self.root / "git-owner"
        repo.mkdir()
        env = (("GIT_CONFIG_NOSYSTEM", "1"), ("HOME", str(self.root)),
               ("GIT_AUTHOR_NAME", "Synthetic"), ("GIT_AUTHOR_EMAIL", "synthetic@example.invalid"),
               ("GIT_COMMITTER_NAME", "Synthetic"), ("GIT_COMMITTER_EMAIL", "synthetic@example.invalid"),
               ("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z"), ("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z"))
        executor = suite.GrantedExecutor(self.root)
        def command(*args):
            argv = (executable,) + args
            grant = bb.CommandGrant(argv, str(repo), env, (argv,), tuple(k for k, _ in env), 2)
            result = executor.run(grant)
            self.assertEqual(result.exit_code, 0, result.stderr)
            return grant, result.stdout
        command("init", "--quiet")
        source = b"package main\nfunc main() {}\n"
        (repo / "main.go").write_bytes(source)
        nested = repo / "nested" / "deeper"
        nested.mkdir(parents=True)
        nested_source = b"package deeper\n"
        (nested / "entry.go").write_bytes(nested_source)
        command("add", "main.go", "nested/deeper/entry.go")
        command("commit", "--quiet", "-m", "synthetic")
        _, commit_raw = command("rev-parse", "HEAD")
        commit = commit_raw.decode().strip()
        _, tree_raw = command("ls-tree", "-rz", "--full-tree", commit)
        entries = {}
        for row in tree_raw.split(b"\x00"):
            if row:
                metadata, name = row.split(b"\t")
                entries[name.decode()] = metadata.split(b" ")[2].decode()
        blob = entries["main.go"]
        grants = [command("cat-file", "-t", commit)[0], command("ls-tree", "-rz", "--full-tree", commit)[0]]
        grants.extend(command("cat-file", "blob", object_id)[0] for object_id in entries.values())
        self.subject.plan.git_repository_root = "git-owner"
        self.subject.plan.git_executable = executable
        self.subject.plan.git_grants = tuple(grants)
        self.subject.plan.delivery_inventory = ("main.go", "nested/deeper/entry.go")
        resources = self.subject.resources()
        self.subject.plan.resources = suite.ResourceGrant(resources.purposes | {"git-snapshot"}, 10000, 2, resources.language_commands)
        self.subject.producer.process_executor = executor
        self.subject.producer.identities["deliveryCommit"] = commit
        hashes = {"main.go": suite.digest(source), "nested/deeper/entry.go": suite.digest(nested_source)}
        self.assertEqual(self.subject.git_snapshot(hashes), entries)
        with self.assertRaisesRegex(bb.ProducerError, "tree.*bytes"):
            self.subject.git_snapshot(hashes | {"main.go": suite.digest(b"wrong-delivery")})
        self.subject.producer.identities["deliveryCommit"] = "f" * 40
        # Pin文本未授予实际Git读取，不能造commit-tree通过。
        with self.assertRaises(suite.Unverified):
            self.subject.git_snapshot({"main.go": suite.digest(source)})

    def test_held_actual_raw_query_future_releases_and_derives_payload(self):
        # 实际raw MCP child会回答；gate不是返回pass，而是独立控制请求完成边界。
        gate = __import__("threading").Event()
        original = self.client.call_tool
        def held(name, args):
            if name == "lsp_definition":
                gate.wait(timeout=1)
            return original(name, args)
        self.client.call_tool = held
        f = self.subject.init("Go")
        future = suite.HeldToolQuery(self.client, profile().args("lsp_definition", {k: f[k] for k in ("root", "language", "path", "line", "column")}), gate, 1)
        future.start()
        self.assertTrue(future.started.wait(1))
        self.assertTrue(future.thread.is_alive())
        future.release()
        self.subject.locations(future.result(), "definition.locations", f)

    def test_actual_idle_lru_and_held_active_backend_handles(self):
        helper_bytes = Path(__file__).with_name("lsp_bridge_prd_forwarder.py").read_bytes()
        helper = self.root / "ttl_helper.py"
        helper.write_bytes(helper_bytes)
        backend = self.root / "ttl_backend.py"
        backend.write_text(r'''import json,sys
while True:
 header=sys.stdin.buffer.readline()
 if not header:break
 n=int(header.split(b':')[1]);sys.stdin.buffer.readline();obj=json.loads(sys.stdin.buffer.read(n))
 if 'id' not in obj:continue
 raw=json.dumps({'jsonrpc':'2.0','id':obj['id'],'result':{}}).encode();sys.stdout.buffer.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);sys.stdout.buffer.flush()
''')
        argv = (sys.executable, str(backend))
        backend_grant = bb.CommandGrant(argv, str(self.root), (), (argv,), (), 2)
        hargv = (sys.executable, str(helper))
        helper_grant = bb.CommandGrant(hargv, str(self.root), (), (hargv,), (), 2)
        resources = suite.ResourceGrant(frozenset({"empty-environment", "backend-forwarding", "proc-lineage", "owned-child-start", "owned-child-kill", "lifecycle"}), 20000, 2,
                                       {lang:backend_grant for lang in ("Go","Python","TypeScript")})
        relay = suite.OwnedLSPForwarder(self.root, resources, helper_grant, helper_bytes)
        self.addCleanup(relay.close)
        grants = {lang:relay.route(lang,lang) for lang in ("Go","Python","TypeScript")}
        parent = self.root / "ttl_parent.py"
        parent.write_text(r'''import json,subprocess,sys,time
commands=json.loads(sys.argv[1]);children={}
def close(name):
 child=children.pop(name);child.stdin.close()
 try:child.wait(timeout=2)
 except subprocess.TimeoutExpired:child.kill();child.wait(timeout=2)
try:
 for line in sys.stdin:
  cmd=json.loads(line);op=cmd['op'];name=cmd.get('language')
  if op=='open':children[name]=subprocess.Popen(commands[name],stdin=subprocess.PIPE,stdout=subprocess.PIPE)
  elif op=='close':close(name)
  elif op=='query':
   child=children[name];raw=json.dumps({'jsonrpc':'2.0','id':1,'method':'textDocument/definition'}).encode();child.stdin.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);child.stdin.flush()
   header=child.stdout.readline();n=int(header.split(b':')[1]);child.stdout.readline();child.stdout.read(n)
  print('done',flush=True)
finally:
 for name in list(children):close(name)
''')
        import subprocess,select
        argv=(sys.executable,str(parent),json.dumps({lang:list(g.argv) for lang,g in grants.items()}))
        grant=bb.CommandGrant(argv,str(self.root),(),(argv,),(),5);grant.validate(self.root)
        process=subprocess.Popen(grant.argv,cwd=grant.cwd,env=dict(grant.env),stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        def cleanup():
            if process.stdin is not None and not process.stdin.closed:process.stdin.close()
            try:process.wait(timeout=3)
            except subprocess.TimeoutExpired:process.kill();process.wait(timeout=2)
            for stream in (process.stdout,process.stderr):
                if stream is not None:stream.close()
        self.addCleanup(cleanup)
        for lang in grants:relay.attach_mcp(lang,process)
        inp,out=process.stdin,process.stdout
        if inp is None or out is None:self.fail('missing synthetic pipes')
        def issue(op,lang):
            inp.write(bb.encode_json({'op':op,'language':lang}));inp.flush()
            self.assertTrue(select.select([out],[],[],3)[0],relay.failures)
            self.assertEqual(out.readline(),b'done\n')
        for lang in ("Go","Python"):
            issue('open',lang);issue('query',lang)
        oldest=relay._children['Go'][0]
        # 合成SUT的LRU策略由其ownchild close实现；观察器只跟随实际helper退出。
        issue('close','Go');issue('open','TypeScript');issue('query','TypeScript')
        deadline=time.monotonic()+2
        while oldest.poll() is None and time.monotonic()<deadline:time.sleep(0.005)
        self.assertIsNotNone(oldest.poll())
        active=relay._children['TypeScript'][0]
        idle=relay._children['Python'][0]
        issue('close','Python')
        deadline=time.monotonic()+2
        while idle.poll() is None and time.monotonic()<deadline:time.sleep(0.005)
        self.assertIsNotNone(idle.poll())
        self.assertIsNone(active.poll())
        issue('close','TypeScript')
        inp.close();process.wait(timeout=3);relay.close()
        self.assertFalse(relay.failures)
        self.assertTrue(all(child.poll() is not None for child in relay._owned_history))
        self.assertFalse(list(self.root.glob('.lsp-relay-*')))

    def test_controlled_go_integration_same_session_wire_source_and_negative_hash(self):
        helper_bytes=Path(__file__).with_name('lsp_bridge_prd_forwarder.py').read_bytes()
        helper=self.root/'integration_helper.py';helper.write_bytes(helper_bytes)
        backend=self.root/'gopls'
        backend.write_text('#!'+sys.executable+'\n'+r'''import sys,json
from urllib.parse import urlparse,unquote
while True:
 header=sys.stdin.buffer.readline()
 if not header:break
 n=int(header.split(b':')[1]);sys.stdin.buffer.readline();request=json.loads(sys.stdin.buffer.read(n))
 if 'id' not in request:continue
 uri=request.get('params',{}).get('textDocument',{}).get('uri','')
 result=[{'uri':uri,'range':{'start':{'line':1,'character':5}}}] if request['method']=='textDocument/definition' else {'contents':'func marker() int'}
 raw=json.dumps({'jsonrpc':'2.0','id':request['id'],'result':result}).encode();sys.stdout.buffer.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);sys.stdout.buffer.flush()
''')
        backend.chmod(0o700)
        bargv=(str(backend),);bg=bb.CommandGrant(bargv,str(self.root),(),(bargv,),(),2)
        hargv=(sys.executable,str(helper));hg=bb.CommandGrant(hargv,str(self.root),(),(hargv,),(),2)
        purposes=frozenset({'fixtures','empty-environment','backend-forwarding','proc-lineage','owned-child-start','owned-child-kill','real-go'})
        resources=suite.ResourceGrant(purposes,20000,2,{'Go':bg})
        relay=suite.OwnedLSPForwarder(self.root,resources,hg,helper_bytes);self.addCleanup(relay.close)
        command=relay.route('actual-Go','Go')
        parent=self.root/'integration_sut.py'
        parent.write_text(r'''import json,sys,subprocess
from urllib.parse import urlparse,unquote
argv=json.loads(sys.argv[1]);child=None;identity=None;seq=0
try:
 for line in sys.stdin:
  req=json.loads(line)
  if req['method']=='notifications/initialized':continue
  if req['method']=='initialize':result={'protocolVersion':'2024-11-05','capabilities':{}}
  else:
   name=req['params']['name'];a=req['params']['arguments']
   if name=='lsp_initialize':
    if child is None:child=subprocess.Popen(argv,stdin=subprocess.PIPE,stdout=subprocess.PIPE);identity={'root':a['root'],'language':a['language'],'id':'SUT-id-not-route-id','state':'ready','dependencyRunning':True}
    payload={}
   elif name=='lsp_status':payload={'instances':[identity]}
   else:
    seq+=1;uri=__import__('pathlib').Path(a['path']).as_uri();method='textDocument/definition' if name=='lsp_definition' else 'textDocument/hover'
    raw=json.dumps({'jsonrpc':'2.0','id':seq,'method':method,'params':{'textDocument':{'uri':uri},'position':{'line':a['line'],'character':a['column']}}}).encode()
    child.stdin.write(b'Content-Length: '+str(len(raw)).encode()+b'\r\n\r\n'+raw);child.stdin.flush()
    header=child.stdout.readline();n=int(header.split(b':')[1]);child.stdout.readline();reply=json.loads(child.stdout.read(n))
    if name=='lsp_definition':payload={'locations':[{'path':unquote(urlparse(r['uri']).path),'line':r['range']['start']['line'],'column':r['range']['start']['character']} for r in reply['result']]}
    else:payload={'text':reply['result']['contents']}
   result={'content':[{'type':'text','text':json.dumps(payload)}]}
  print(json.dumps({'jsonrpc':'2.0','id':req['id'],'result':result}),flush=True)
finally:
 if child is not None:
  child.stdin.close()
  try:child.wait(timeout=2)
  except subprocess.TimeoutExpired:child.kill();child.wait(timeout=2)
''')
        argv=(sys.executable,str(parent),json.dumps(command.argv));grant=bb.CommandGrant(argv,str(self.root),(),(argv,),(),3)
        client=bb.StdioJSONRPCClient(grant,self.root);self.addCleanup(client.close)
        relay.attach_mcp('actual-Go',client.process);client.initialize()
        pins={key:'synthetic-'+key for key in bb.IDENTITY_KEYS}
        producer=bb.LSPBridgeProducer(client,self.executor,NeverMeasure(),self.root,pins)
        plan=suite.SuitePlan(resources=resources,real_go_executable_sha256=suite.digest(backend.read_bytes()))
        subject=suite.PRDSuite(producer,profile(),plan,relay,execution='synthetic')
        result=subject.check_17()
        self.assertEqual(result['instance'],'actual-Go')
        self.assertEqual(result['mcpPid'],client.process.pid)
        self.assertTrue(result['fixture'])
        self.assertEqual(result['generation'],1)
        plan.real_go_executable_sha256='sha256:'+'0'*64
        with self.assertRaisesRegex(bb.ProducerError,'identity'):
            subject.check_17()
        # 清理按本对象MCP stdinEOF，synthetic parent仍持helper handle做boundedfinally。
        if client.process.stdin is not None:client.process.stdin.close()
        client.process.wait(timeout=3)
        client.close();relay.close()

    def test_fixed_go_harness_actual_plain_race_and_failure_calibration(self):
        """重Go只由main明确串行slot调用此单测试；默认Pythonsuite不启动Go。"""
        grants = GO_CALIBRATION_GRANTS
        if grants is None:
            self.skipTest("Go串行CommandGrants未授予，NOT_RUN不是PASS")
        directory, executable = grants
        directory.mkdir(mode=0o700, exist_ok=False)
        harness = directory / 'suite_goroutines.go';harness.write_text(suite.GO_CONCURRENT_HARNESS)
        child = directory / 'synthetic_mcp.py'
        child.write_text(r'''import sys,json
mode=sys.argv[1];pending=[]
for raw in sys.stdin:
 req=json.loads(raw)
 if req.get('method')=='notifications/initialized':continue
 if req['method']=='initialize':
  reply={'jsonrpc':'2.0','id':999 if mode=='badsetup' else req['id'],'result':{'protocolVersion':'2024-11-05','capabilities':{}}};print(json.dumps(reply),flush=True)
 elif req['id']==2:print(json.dumps({'jsonrpc':'2.0','id':2,'result':{'content':[{'type':'text','text':'{}'}]}}),flush=True)
 else:
  if mode=='eof':break
  pending.append(req)
  if len(pending)==10:
   for item in reversed(pending):print(json.dumps({'jsonrpc':'2.0','id':item['id'],'result':{'content':[{'type':'text','text':'{"marker":"raw"}'}]}}),flush=True)
''')
        request={'jsonrpc':'2.0','id':1,'method':'initialize','params':{}}
        workspace={'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'lsp_initialize','arguments':{}}}
        queries=[{'jsonrpc':'2.0','id':i+100,'method':'tools/call','params':{'name':'lsp_definition' if i%2==0 else 'lsp_hover','arguments':{}}} for i in range(10)]
        executor=suite.GrantedExecutor(self.root)
        results=[]
        for race, mode in ((False,'ok'),(True,'ok'),(False,'badsetup'),(False,'eof')):
            argv=(executable,'run')+(('-race',) if race else ())+ (str(harness),)
            env=(('HOME',str(directory)),('GOCACHE',str(directory/'cache')),('GOPATH',str(directory/'gopath')),('GOTOOLCHAIN','local'),('CGO_ENABLED','1'),('CC','/usr/bin/gcc'))
            grant=bb.CommandGrant(argv,str(directory),env,(argv,),tuple(k for k,_ in env),60)
            plan={'argv':[sys.executable,str(child),mode],'cwd':str(directory),'env':{},'initialize':request,'workspace':workspace,'requests':queries,'timeoutMs':2000}
            result=executor.run_with_input(grant,bb.encode_json(plan))
            if mode=='ok':
                self.assertEqual(result.exit_code,0,result.stderr)
                rows=[bb._strict_json(raw) for raw in result.stdout.splitlines()]
                self.assertEqual(len(rows),10)
                self.assertEqual({bb._strict_json(base64.b64decode(row['response']))['id'] for row in rows},set(range(100,110)))
                self.assertLess(max(row['sentNs'] for row in rows),min(row['receivedNs'] for row in rows))
                self.assertNotIn(b'DATA RACE',result.stderr)
            else:self.assertNotEqual(result.exit_code,0)
            self.assertFalse(result.timed_out)
            results.append((race,mode,result.exit_code,result.duration_ms))
        print('SYNTHETIC_GO_CALIBRATION',results)

    def test_decoded_only_real_never_passes_even_document_checks(self):
        self.client.close()
        class Decoded:
            def list_tools(inner):
                return [{"name": name} for name in bb.TOOL_NAMES]
            def call_tool(inner, name, arguments):
                return {"content": [{"type": "text", "text": "{}"}]}
        self.producer.transport = Decoded()
        self.plan.resources = None
        self.subject.execution = "real"
        report = self.subject.run()
        self.assertFalse(any(row["status"] == "PASS" for row in report["requirements"]))
        self.assertFalse(report["acceptanceEstablished"])


if __name__ == "__main__":
    unittest.main()
