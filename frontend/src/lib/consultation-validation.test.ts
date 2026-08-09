import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { validateConsultationDraft } from './consultation-validation.ts';

describe('validateConsultationDraft', () => {
  test('accepts a concrete repository review', () => {
    const result = validateConsultationDraft({
      repositoryRoot: 'C:\\projects\\ledger',
      objective: '중복 capture가 발생하는 실패 경로를 검토한다.',
      maximumCostMinor: 800
    });
    assert.equal(result.valid, true);
    assert.deepEqual(result.errors, {});
  });

  test('rejects missing context and invalid cost', () => {
    const result = validateConsultationDraft({
      repositoryRoot: ' ',
      objective: '짧음',
      maximumCostMinor: -1
    });
    assert.equal(result.valid, false);
    assert.ok(result.errors.repositoryRoot);
    assert.ok(result.errors.objective);
    assert.ok(result.errors.maximumCostMinor);
  });

  test('rejects a null byte and an oversized objective', () => {
    const result = validateConsultationDraft({
      repositoryRoot: '/tmp/repository\0escape',
      objective: '가'.repeat(8_001),
      maximumCostMinor: 0
    });
    assert.equal(result.valid, false);
    assert.ok(result.errors.repositoryRoot);
    assert.ok(result.errors.objective);
  });
});
