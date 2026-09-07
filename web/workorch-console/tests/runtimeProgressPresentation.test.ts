import assert from 'node:assert/strict';
import test from 'node:test';
import { validationRepairPolicyLabel } from '../src/runtimeProgressPresentation.ts';

test('Profile repair label does not borrow a larger global convergence limit', () => {
  const projection = { validationRepairPolicyId: 'profile-policy', validationRepairMaxAttempts: 2, maxRepairAttempts: 48 };
  assert.equal(validationRepairPolicyLabel(projection), '2 (profile-policy)');
  assert.equal(projection.maxRepairAttempts, 48);
  assert.equal(validationRepairPolicyLabel(), '-');
  assert.equal(validationRepairPolicyLabel({ validationRepairMaxAttempts: 48 }), '-');
  assert.equal(validationRepairPolicyLabel({ validationRepairPolicyId: 'profile-policy' }), '-');
});
