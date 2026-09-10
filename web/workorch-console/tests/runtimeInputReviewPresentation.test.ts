import assert from 'node:assert/strict';
import test from 'node:test';
import { inputReviewFeedbackLines } from '../src/runtimeProgressPresentation.ts';
import type { RuntimeOriginalDiagnostic } from '../src/types.ts';

test('input review displays every original obligation and exact source without choosing an action', () => {
  const value: RuntimeOriginalDiagnostic = {
    schemaVersion: 'workorch.runtime_original_diagnostic', ownerModuleId: 'kernel/orchestrator', originalClass: 'contract',
    originalCode: 'validation_input_review_coverage_incomplete', originalMessage: 'input review blocked validation', signature: 'review-1',
    validationInputReviews: [{
      reviewRef: 'review-1', protocolRef: 'validation/input-consistency/v2', coverageWorkItemId: 'cwi-1',
      requirementHash: 'original-requirement', reportHash: 'original-report', redacted: true,
      report: { subjectHash: 'exact-input-version', checks: [{
        requiredCheckRef: 'original-check', conclusion: 'conflict', reason: 'The documentation assertion is missing.',
        inputTargetRefs: ['checks/input'], obligations: [
          { criterionText: 'provide tools', finding: 'covered', reason: 'inventory assertion exists', inputTargetRefs: ['checks/input'] },
          { criterionText: 'document tools', finding: 'missing', reason: 'no documentation check', inputTargetRefs: ['checks/input'] },
        ],
      }] },
    }],
  };
  const before = structuredClone(value);
  const text = inputReviewFeedbackLines(value).join('\n');
  for (const part of ['cwi-1', 'original-check', 'original-requirement', 'exact-input-version', 'original-report', 'provide tools', 'document tools', 'no documentation check', 'checks/input', '不是验证执行或验收证据', '已脱敏']) {
    assert.ok(text.includes(part), part);
  }
  assert.deepEqual(value, before);
  assert.deepEqual(inputReviewFeedbackLines(undefined), []);
  assert.deepEqual(inputReviewFeedbackLines({ ...value, validationInputReviews: undefined }), []);
});
