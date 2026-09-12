import assert from 'node:assert/strict';
import test from 'node:test';
import { canSaveSettings, settingsChanged, settingsError } from './settings-form.ts';
import type { Settings } from './types';

const saved: Settings = { startAtLogin: false, minimizeToTray: true, defaultRoute: 'openai_api_pro', maximumCostMinor: 0, gatewayPort: 10100, defaultRepositoryRoot: 'C:/repo', mcpBridgePath: '' };

test('settings save guard follows load, edit, save, failure and success states', () => {
  const draft = { ...saved, gatewayPort: 10101 };
  assert.equal(canSaveSettings('loading', null, null), false);
  assert.equal(canSaveSettings('load-error', draft, saved), false);
  assert.equal(canSaveSettings('ready', { ...saved }, saved), false);
  assert.equal(canSaveSettings('ready', draft, saved), true);
  assert.equal(canSaveSettings('saving', draft, saved), false);
  assert.equal(canSaveSettings('ready', draft, saved), true); // Failed save preserves retry.
  assert.equal(canSaveSettings('ready', draft, { ...draft }), false);
  assert.equal(settingsChanged({ ...saved }, saved), false); // Reset restores the baseline.
});

test('blank, fractional and out-of-range numeric inputs cannot be saved', () => {
  for (const gatewayPort of [undefined, NaN, 1023, 65536, 10100.5]) {
    const draft = { ...saved, gatewayPort } as Settings;
    assert.ok(settingsError(draft));
    assert.equal(canSaveSettings('ready', draft, saved), false);
  }
  for (const maximumCostMinor of [undefined, NaN, -1, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.ok(settingsError({ ...saved, maximumCostMinor } as Settings));
  }
  assert.equal(settingsError({ ...saved, gatewayPort: 65535 }), '');
});
