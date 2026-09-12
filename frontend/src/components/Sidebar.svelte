<script lang="ts">
  import type { NavigationKey } from '../lib/types';

  export let active: NavigationKey;
  export let onNavigate: (key: NavigationKey) => void;

  const items: Array<{ key: NavigationKey; label: string }> = [
    { key: 'overview', label: '개요' },
    { key: 'providers', label: '공급자' },
    { key: 'architect', label: 'Architect' },
    { key: 'mcp', label: 'MCP 연결' },
    { key: 'settings', label: '설정' }
  ];
</script>

<aside class="sidebar">
  <div class="brand">
    <div class="brand-mark">RD</div>
    <div>
      <strong>RelayDock</strong>
      <span>Local AI Gateway</span>
    </div>
  </div>

  <nav aria-label="주요 메뉴">
    {#each items as item}
      <button
        type="button"
        class:active={active === item.key}
        aria-current={active === item.key ? 'page' : undefined}
        on:click={() => onNavigate(item.key)}
      >
        {item.label}
      </button>
    {/each}
  </nav>

  <div class="sidebar-footer">
    <span class="status-dot"></span>
    로컬 모드
  </div>
</aside>
