<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { Settings } from '../lib/types';
  import { getSettings, saveSettings } from '../lib/runtime-adapter';
  import { canSaveSettings, settingsChanged, settingsError, type SettingsPhase } from '../lib/settings-form';

  let settings: Settings | null = null;
  let saved: Settings | null = null;
  let phase: SettingsPhase = 'loading';
  let disposed = false;
  let message = '';
  let errorMessage = '';
  $: dirty = settingsChanged(settings, saved);
  $: validation = settingsError(settings);
  $: canSave = canSaveSettings(phase, settings, saved);

  async function load(): Promise<void> {
    phase = 'loading';
    errorMessage = '';
    try {
      const result = await getSettings();
      if (disposed) return;
      settings = { ...result };
      saved = { ...result };
      phase = 'ready';
    } catch (error) {
      if (disposed) return;
      phase = 'load-error';
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }

  async function save(): Promise<void> {
    if (!canSave || !settings) return;
    const candidate = { ...settings };
    phase = 'saving';
    message = '';
    errorMessage = '';
    try {
      await saveSettings(candidate);
      if (disposed) return;
      saved = candidate;
      message = '설정을 저장했어요.';
    } catch (error) {
      if (disposed) return;
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      if (!disposed) phase = 'ready';
    }
  }

  onMount(() => void load());
  onDestroy(() => { disposed = true; });
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">PREFERENCES</span>
    <h1>설정</h1>
    <p>앱 실행 방식과 로컬 게이트웨이를 설정하세요.</p>
  </div>
</header>

<form class="panel settings-form" aria-label="RelayDock 설정" aria-busy={phase === 'loading' || phase === 'saving'} on:submit|preventDefault={save} on:input={() => { message = ''; errorMessage = ''; }}>
  {#if phase === 'loading'}
    <p role="status">설정을 불러오는 중이에요…</p>
  {:else if settings}
    <fieldset disabled={phase === 'saving'}>
      <legend>앱 실행</legend>
      <label class="toggle-row"><input type="checkbox" bind:checked={settings.startAtLogin} /><span>로그인할 때 실행</span></label>
      <label class="toggle-row"><input type="checkbox" bind:checked={settings.minimizeToTray} /><span>창을 닫으면 트레이에 유지</span></label>
    </fieldset>
    <fieldset disabled={phase === 'saving'}>
      <legend>로컬 연결</legend>
      <label>기본 저장소 경로<input bind:value={settings.defaultRepositoryRoot} spellcheck="false" /></label>
      <div class="settings-columns">
        <label>게이트웨이 포트<input type="number" min="1024" max="65535" step="1" required bind:value={settings.gatewayPort} aria-describedby="gateway-port-help" /><small id="gateway-port-help">변경 후 게이트웨이를 다시 시작하세요. 클라이언트의 연결 주소도 맞춰 주세요.</small></label>
        <label>MCP 브리지 경로<input bind:value={settings.mcpBridgePath} spellcheck="false" placeholder="설치 폴더에서 자동으로 찾기" /><small>비워두면 설치 폴더의 브리지를 사용해요.</small></label>
      </div>
    </fieldset>
    <fieldset disabled={phase === 'saving'}>
      <legend>전문가 호출</legend>
      <div class="settings-columns">
        <label>기본 전문가 경로<select bind:value={settings.defaultRoute}><option value="openai_api_pro">API Pro</option><option value="chatgpt_web_handoff">ChatGPT Web Handoff</option></select></label>
        <label>작업당 최대 비용<input type="number" min="0" step="1" required bind:value={settings.maximumCostMinor} aria-describedby="cost-help" /><small id="cost-help">0이면 제한하지 않아요. 비용 상한을 사용하려면 모델 가격 환경 변수도 설정해야 해요.</small></label>
      </div>
    </fieldset>
    {#if validation}<div class="form-message error" role="alert">{validation}</div>{/if}
    <div class="settings-actions">
      <span role="status">{phase === 'saving' ? '저장 중…' : dirty ? '저장하지 않은 변경 사항이 있어요.' : message || '저장된 설정이에요.'}</span>
      <button class="button-secondary" type="button" disabled={!dirty || phase !== 'ready'} on:click={() => { settings = saved && { ...saved }; message = ''; errorMessage = ''; }}>변경 취소</button>
      <button class="button-primary" type="submit" disabled={!canSave}>{phase === 'saving' ? '저장 중…' : '변경 저장'}</button>
    </div>
  {/if}
  {#if errorMessage}<div class="form-message error" role="alert">{errorMessage}</div>{/if}
  {#if phase === 'load-error'}<div><button class="button-secondary" type="button" on:click={() => void load()}>설정 다시 불러오기</button></div>{/if}
</form>

<style>
  .settings-form { max-width: 900px; gap: 24px; }
  fieldset { min-width: 0; margin: 0; padding: 0 0 24px; border: 0; border-bottom: 1px solid var(--border); display: grid; gap: 16px; }
  legend { margin-bottom: 16px; padding: 0; font-weight: 700; }
  label { min-width: 0; font-size: 14px; }
  .toggle-row { justify-content: flex-start; min-height: 32px; }
  .toggle-row input { width: 18px; height: 18px; margin: 0; padding: 0; flex: 0 0 18px; accent-color: var(--accent); }
  .settings-columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; align-items: start; }
  input, select { min-width: 0; width: 100%; max-width: none; }
  small { color: var(--muted); line-height: 1.6; font-size: 12px; font-weight: 400; }
  .settings-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
  .settings-actions span { flex: 1 1 220px; color: var(--muted); font-size: 13px; }
  input:focus-visible, select:focus-visible, button:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
  @media (max-width: 1100px) { .settings-columns { grid-template-columns: minmax(0, 1fr); } }
</style>
