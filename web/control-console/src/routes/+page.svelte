<script lang="ts">
  import type { PageData } from './$types';
  export let data: PageData;

  const formatTime = (value: string): string => value ? new Date(value).toLocaleString() : '확인 불가';
</script>

<svelte:head>
  <title>AI Runtime Control</title>
  <meta name="description" content="AI Runtime Gateway control plane" />
</svelte:head>

<div class="shell">
  <aside>
    <div class="brand"><strong>AI Runtime</strong><span>Control Plane</span></div>
    <nav aria-label="주요 메뉴">
      <a class="active" href="/">Overview</a>
      <span>Projects</span>
      <span>Virtual Keys</span>
      <span>Model Routes</span>
      <span>Providers</span>
      <span>Audit</span>
    </nav>
    <div class="endpoint"><span>Control API</span><code>{data.baseURL}</code></div>
  </aside>

  <main>
    <header>
      <div>
        <p class="eyebrow">CONTROL PLANE</p>
        <h1>운영 상태</h1>
        <p class="subtitle">가상 모델과 서명된 런타임 스냅샷 상태를 확인한다.</p>
      </div>
      <div class:ready={data.readiness.ok} class="status-pill">
        <span></span>{data.readiness.ok ? 'Ready' : 'Not ready'}
      </div>
    </header>

    <section class="metrics" aria-label="운영 지표">
      <article>
        <span>프로세스 상태</span>
        <strong>{data.health.ok ? 'Healthy' : 'Offline'}</strong>
        <small>HTTP {data.health.status || '연결 실패'}</small>
      </article>
      <article>
        <span>스냅샷 리비전</span>
        <strong>{data.revision.toLocaleString()}</strong>
        <small>생성 {formatTime(data.generatedAt)}</small>
      </article>
      <article>
        <span>가상 모델</span>
        <strong>{data.models.length.toLocaleString()}</strong>
        <small>만료 {formatTime(data.expiresAt)}</small>
      </article>
    </section>

    <section class="panel">
      <div class="panel-header">
        <div><p class="eyebrow">ROUTING</p><h2>가상 모델</h2></div>
        <span>Revision {data.revision}</span>
      </div>
      {#if data.models.length === 0}
        <div class="empty">
          <strong>게시된 모델 경로가 없다.</strong>
          <p>Control API 연결과 bearer token 설정을 확인해야 한다.</p>
        </div>
      {:else}
        <div class="routes">
          {#each data.models as route}
            <article class="route">
              <div><strong>{route.virtualModel}</strong><span>{route.policyId || '기본 정책'}</span></div>
              <code>{route.candidates.join(' → ')}</code>
            </article>
          {/each}
        </div>
      {/if}
    </section>
  </main>
</div>

<style>
  :global(*) { box-sizing: border-box; }
  :global(body) { margin: 0; min-width: 960px; background: #0d0f14; color: #e8ebf2; font-family: Inter, ui-sans-serif, system-ui, sans-serif; }
  .shell { display: grid; grid-template-columns: 230px 1fr; min-height: 100vh; }
  aside { position: sticky; top: 0; height: 100vh; display: flex; flex-direction: column; padding: 26px 18px; border-right: 1px solid #293040; background: #10131a; }
  .brand { padding: 0 10px 28px; }
  .brand strong, .brand span { display: block; }
  .brand strong { font-size: 17px; }
  .brand span { margin-top: 3px; color: #939bad; font-size: 12px; }
  nav { display: grid; gap: 5px; }
  nav a, nav span { padding: 10px 12px; border-radius: 8px; color: #939bad; font-size: 14px; text-decoration: none; }
  nav .active { color: #fff; background: #202635; }
  .endpoint { margin-top: auto; padding: 12px 10px; border-top: 1px solid #293040; }
  .endpoint span, .endpoint code { display: block; }
  .endpoint span { margin-bottom: 6px; color: #939bad; font-size: 11px; }
  .endpoint code { overflow: hidden; color: #c8c3ff; font-size: 11px; text-overflow: ellipsis; }
  main { width: 100%; max-width: 1480px; padding: 46px 52px 80px; }
  header { display: flex; justify-content: space-between; align-items: flex-start; gap: 24px; margin-bottom: 30px; }
  .eyebrow { margin: 0; color: #9188ff; font-size: 11px; font-weight: 800; letter-spacing: .14em; }
  h1 { margin: 6px 0 8px; font-size: 34px; letter-spacing: -.03em; }
  h2 { margin: 5px 0 0; font-size: 18px; }
  .subtitle { margin: 0; color: #939bad; }
  .status-pill { display: inline-flex; align-items: center; gap: 8px; padding: 8px 12px; border: 1px solid #63323a; border-radius: 999px; color: #ff9ea7; background: #2d161b; font-size: 13px; }
  .status-pill span { width: 8px; height: 8px; border-radius: 50%; background: currentColor; }
  .status-pill.ready { border-color: #28543e; color: #73dfa5; background: #13271e; }
  .metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
  .metrics article, .panel { border: 1px solid #293040; border-radius: 12px; background: #151922; }
  .metrics article { padding: 20px; }
  .metrics span, .metrics small { display: block; color: #939bad; font-size: 12px; }
  .metrics strong { display: block; margin: 12px 0 8px; font-size: 23px; }
  .panel { margin-top: 26px; overflow: hidden; }
  .panel-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 20px 22px; border-bottom: 1px solid #293040; }
  .panel-header > span { color: #939bad; font-size: 12px; }
  .routes { display: grid; }
  .route { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 17px 22px; border-top: 1px solid #242b39; }
  .route:first-child { border-top: 0; }
  .route strong, .route span { display: block; }
  .route span { margin-top: 5px; color: #939bad; font-size: 12px; }
  .route code { color: #c8c3ff; font-size: 12px; }
  .empty { padding: 48px 22px; text-align: center; }
  .empty p { margin: 8px 0 0; color: #939bad; }
</style>
