import type { Settings } from './types';

export type SettingsPhase = 'loading' | 'ready' | 'saving' | 'load-error';

export function settingsChanged(draft: Settings | null, saved: Settings | null): boolean {
  return draft !== null && saved !== null &&
    (Object.keys(saved) as Array<keyof Settings>).some((key) => draft[key] !== saved[key]);
}

export function settingsError(settings: Settings | null): string {
  if (!settings) return '';
  if (!Number.isInteger(settings.gatewayPort) || settings.gatewayPort < 1024 || settings.gatewayPort > 65535) {
    return '게이트웨이 포트는 1024부터 65535 사이의 정수로 입력해 주세요.';
  }
  if (!Number.isSafeInteger(settings.maximumCostMinor) || settings.maximumCostMinor < 0) {
    return '작업당 최대 비용은 0 이상의 정수로 입력해 주세요.';
  }
  return '';
}

export function canSaveSettings(phase: SettingsPhase, draft: Settings | null, saved: Settings | null): boolean {
  return phase === 'ready' && settingsChanged(draft, saved) && settingsError(draft) === '';
}
