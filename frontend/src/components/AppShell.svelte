<script lang="ts">
  import Sidebar from './Sidebar.svelte';
  import type { NavigationKey } from '../lib/types';

  export let active: NavigationKey;
  export let onNavigate: (key: NavigationKey) => void;
</script>

<div class="app-shell">
  <Sidebar {active} {onNavigate} />
  <main class="content">
    <slot />
  </main>
</div>
