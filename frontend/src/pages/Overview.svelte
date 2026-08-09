<script lang="ts">
  import StatusBadge from '../components/StatusBadge.svelte';
  import type { RuntimeStatus } from '../lib/types';
  import { getSettings, startGateway, stopGateway } from '../lib/runtime-adapter';

  export let status: RuntimeStatus;
  export let onRefresh: () => Promise<void>;

  let busy = false;
  let errorMessage = '';

  async function toggleGateway(): Promise<void> {
    busy = true;
    errorMessage = '';
    try {
      if (status.gatewayReady) {
        await stopGateway();
      } else {
        const settings = await getSettings();
        await startGateway(settings.gatewayPort);
      }
      await onRefresh();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">LOCAL RUNTIME</span>
    <h1>개요</h1>
    <p>코딩 도구, 모델 공급자, 전문가 검토 경로를 한곳에서 관리한다.</p>
  </div>
  <StatusBadge state={status.ipcReady ? 'ready' : 'pending'} />
</header>

{#if errorMessage}<div class="error-banner" role="alert">{errorMessage}</div>{/if}

<section class="status-grid">
  <article class="panel status-card">
    <span>데스크톱 런타임</span>
    <strong>{status.version}</strong>
    <p>{status.ipcReady ? 'MCP 요청을 받을 수 있다.' : '로컬 연결을 시작하지 못했다.'}</p>
  </article>
  <article class="panel status-card">
    <span>MCP 연결</span>
    <strong>{status.mcpConfigured ? '설정 생성됨' : '연결 전'}</strong>
    <p>코딩 도구에서 Architect 도구를 호출한다.</p>
  </article>
  <article class="panel status-card">
    <span>로컬 게이트웨이</span>
    <strong>{status.gatewayReady ? '실행 중' : '중지됨'}</strong>
    <p>{status.gatewayAddress ?? 'OpenAI·Anthropic·Gemini 호환 엔드포인트를 제공한다.'}</p>
    <button class="button-secondary inline-action" type="button" disabled={busy} on:click={toggleGateway}>
      {status.gatewayReady ? '중지' : '시작'}
    </button>
  </article>
</section>
