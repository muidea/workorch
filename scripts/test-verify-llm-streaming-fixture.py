#!/usr/bin/env python3
"""LLM 三模型夹具合同测试；只执行提取的纯 usage 断言，不调用部署服务。"""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('verify-llm-workspace-execution.sh').read_text()


class StreamingFixtureContractTest(unittest.TestCase):
    def test_three_models_use_distinct_preseal_creation_and_no_postseal_selector(self):
        self.assertNotIn('activate_workspace_execution_with_provider', SCRIPT)
        self.assertNotIn('/llm-selection', SCRIPT)
        for prefix, model in [('stream', 'verify-model'), ('stream-native', 'verify-stream-native-model'), ('stream-estimated', 'verify-stream-estimated-model')]:
            creation = SCRIPT.index(f'create_acceptance_workspace "{prefix}"')
            activation_prefix = 'stream-baseline' if prefix == 'stream' else prefix
            activation = SCRIPT.index(f'activate_workspace_execution_without_memory "$TMP_DIR/{activation_prefix}-activation.json"')
            assertion = SCRIPT.index(f'assert_workspace_llm_usage_breakdown "$workspace_id" "{model}"')
            self.assertLess(creation, activation)
            self.assertLess(activation, assertion)
        self.assertIn('--provider-id ai-proxy --model-id "$model_id"', SCRIPT)

    def test_usage_assertion_requires_exact_workspace_model_and_positive_entries(self):
        section = SCRIPT.split('assert_workspace_llm_usage_breakdown() {', 1)[1]
        assertion = section.split("<<'PY'\n", 1)[1].split('\nPY\n', 1)[0]
        for model in ('verify-model', 'verify-stream-native-model', 'verify-stream-estimated-model'):
            for workspace, observed_model, total, success in [('ws', model, 1, True), ('other', model, 1, False), ('ws', 'wrong', 1, False), ('ws', model, 0, False)]:
                with self.subTest(model=model, workspace=workspace, observed_model=observed_model, total=total), tempfile.TemporaryDirectory() as directory:
                    path = Path(directory) / 'snapshot.json'
                    path.write_text(json.dumps({'snapshot': {'workspace': {'id': workspace}, 'llmUsageSummary': {'providerModelBreakdown': [{'providerId': 'ai-proxy', 'model': observed_model, 'entryTotal': total}]}}}))
                    result = subprocess.run([sys.executable, '-c', assertion, str(path), model, 'ws'], capture_output=True, timeout=5)
                    self.assertEqual(result.returncode == 0, success, result.stderr.decode())


if __name__ == '__main__':
    unittest.main()
