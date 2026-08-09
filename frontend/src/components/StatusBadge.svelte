<script lang="ts">
  export let state: 'healthy' | 'degraded' | 'offline' | 'unconfigured' | 'ready' | 'pending';

  const labels: Record<typeof state, string> = {
    healthy: '정상',
    degraded: '불안정',
    offline: '오프라인',
    unconfigured: '미설정',
    ready: '준비됨',
    pending: '대기'
  };
</script>

<span class="badge" data-state={state}>{labels[state]}</span>
