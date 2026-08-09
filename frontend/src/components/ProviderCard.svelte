<script lang="ts">
  import StatusBadge from './StatusBadge.svelte';
  import type { ProviderSummary } from '../lib/types';

  export let provider: ProviderSummary;
</script>

<article class="panel provider-card">
  <div>
    <h3>{provider.name}</h3>
    <p>{provider.mode}</p>
  </div>
  <StatusBadge state={provider.configured ? 'healthy' : 'unconfigured'} />
</article>
