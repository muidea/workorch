import assert from 'node:assert/strict';
import test from 'node:test';
import { workspaceTemplateSuggestions } from '../src/workspaceTemplateSuggestions.ts';

const template = { goalText: '模板建议目标', doDItems: ['模板验收一', '模板验收二'] };

test('初次选择模板只填充未编辑的空字段', () => {
  assert.deepEqual(workspaceTemplateSuggestions({ goalTouched: false, doDTouched: false }, template), {
    goalText: template.goalText,
    doDText: '模板验收一\n模板验收二'
  });
});

test('切换模板保留已有目标和用户验收条件，包括应用的草案', () => {
  for (const touched of [true, false]) {
    assert.deepEqual(workspaceTemplateSuggestions({
      goalText: '用户目标', doDText: '用户额外条件', goalTouched: touched, doDTouched: touched
    }, template), {});
  }
});

test('用户主动清空字段后不被模板重新填充', () => {
  assert.deepEqual(workspaceTemplateSuggestions({
    goalText: '', doDText: '  ', goalTouched: true, doDTouched: true
  }, template), {});
});

test('目标和验收字段独立保护，空模板不清空内容', () => {
  assert.deepEqual(workspaceTemplateSuggestions({
    goalText: '用户目标', goalTouched: true, doDTouched: false
  }, template), { doDText: '模板验收一\n模板验收二' });
  assert.deepEqual(workspaceTemplateSuggestions({ goalTouched: false, doDTouched: false }, {}), {});
});
