<script lang="ts">
  import { onMount } from 'svelte';
  import type { Settings } from '../lib/types';
  import { getSettings, saveSettings } from '../lib/runtime-adapter';

  let settings: Settings = {
    startAtLogin: false,
    minimizeToTray: true,
    defaultRoute: 'openai_api_pro',
    maximumCostMinor: 0,
    gatewayPort: 10100,
    defaultRepositoryRoot: '',
    mcpBridgePath: ''
  };
  let busy = false;
  let message = '';
  let errorMessage = '';

  async function load(): Promise<void> {
    try {
      settings = await getSettings();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }

  async function save(): Promise<void> {
    busy = true;
    message = '';
    errorMessage = '';
    try {
      await saveSettings(settings);
      message = '설정을 저장했다.';
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  onMount(() => void load());
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">PREFERENCES</span>
    <h1>설정</h1>
    <p>로컬 실행과 전문가 호출 정책을 조정한다.</p>
  </div>
</header>

<section class="panel settings-form">
  <label><input type="checkbox" bind:checked={settings.startAtLogin} /> 로그인할 때 실행</label>
  <label><input type="checkbox" bind:checked={settings.minimizeToTray} /> 창을 닫으면 트레이에 유지</label>
  <label>기본 저장소 경로<input bind:value={settings.defaultRepositoryRoot} /></label>
  <label>로컬 게이트웨이 포트<input type="number" min="1024" max="65535" bind:value={settings.gatewayPort} /></label>
  <label>작업당 최대 비용<input type="number" min="0" bind:value={settings.maximumCostMinor} /><small>0이면 별도 비용 상한을 적용하지 않는다. 상한을 쓰려면 모델 가격 환경 변수도 설정해야 한다.</small></label>
  <label>
    기본 전문가 경로
    <select bind:value={settings.defaultRoute}>
      <option value="openai_api_pro">API Pro</option>
      <option value="chatgpt_web_handoff">ChatGPT Web Handoff</option>
    </select>
  </label>
  <label>MCP 브리지 경로<input bind:value={settings.mcpBridgePath} placeholder="비워두면 설치 폴더에서 찾는다" /></label>
  {#if message}<div class="form-message success">{message}</div>{/if}
  {#if errorMessage}<div class="form-message error" role="alert">{errorMessage}</div>{/if}
  <div><button class="button-primary" type="button" disabled={busy} on:click={save}>저장</button></div>
</section>
