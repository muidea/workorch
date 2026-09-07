import assert from 'node:assert/strict';
import test from 'node:test';
import { originalDiagnosticLabel, validationRepairPolicyLabel } from '../src/runtimeProgressPresentation.ts';

test('Profile repair label does not borrow a larger global convergence limit', () => {
  const projection = { validationRepairPolicyId: 'profile-policy', validationRepairMaxAttempts: 2, maxRepairAttempts: 48 };
  assert.equal(validationRepairPolicyLabel(projection), '2 (profile-policy)');
  assert.equal(projection.maxRepairAttempts, 48);
  assert.equal(validationRepairPolicyLabel(), '-');
  assert.equal(validationRepairPolicyLabel({ validationRepairMaxAttempts: 48 }), '-');
  assert.equal(validationRepairPolicyLabel({ validationRepairPolicyId: 'profile-policy' }), '-');
});

test('Owner cause remains distinct from a terminal guard and is display-only', () => {
  const projection = Object.freeze({
    failureCode: 'equivalent_authority_no_progress',
    originalDiagnostic: Object.freeze({ ownerModuleId: 'blocks/file', originalCode: 'patch_basehash_mismatch' })
  });
  assert.equal(originalDiagnosticLabel(projection.originalDiagnostic), 'blocks/file / patch_basehash_mismatch');
  assert.equal(projection.failureCode, 'equivalent_authority_no_progress');
  assert.equal(originalDiagnosticLabel(), '-');
  assert.equal(originalDiagnosticLabel({ omittedReason: 'not retained by legacy producer' }), '-');
});
