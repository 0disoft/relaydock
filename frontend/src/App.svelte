<script lang="ts">
  import { onMount } from 'svelte';
  import AppShell from './components/AppShell.svelte';
  import Overview from './pages/Overview.svelte';
  import Providers from './pages/Providers.svelte';
  import Architect from './pages/Architect.svelte';
  import MCP from './pages/MCP.svelte';
  import Settings from './pages/Settings.svelte';
  import type { ConsultationSummary, NavigationKey, ProviderSummary, RuntimeStatus } from './lib/types';
  import { getRuntimeStatus, listConsultations, listProviders } from './lib/runtime-adapter';

  let active: NavigationKey = 'overview';
  let status: RuntimeStatus = {
    version: 'loading',
    ipcReady: false,
    mcpConfigured: false,
    gatewayStarting: false,
    gatewayReady: false
  };
  let providers: ProviderSummary[] = [];
  let consultations: ConsultationSummary[] = [];
  let loadError = '';

  async function refreshStatus(): Promise<void> {
    status = await getRuntimeStatus();
  }

  async function refreshConsultations(): Promise<void> {
    consultations = await listConsultations();
  }

  async function refreshProviders(): Promise<void> {
    providers = await listProviders();
  }

  async function loadInitialState(): Promise<void> {
    loadError = '';
    try {
      [status, providers, consultations] = await Promise.all([
        getRuntimeStatus(),
        listProviders(),
        listConsultations()
      ]);
    } catch (error) {
      loadError = error instanceof Error ? error.message : String(error);
    }
  }

  onMount(() => {
    void loadInitialState();
  });
</script>

<AppShell {active} onNavigate={(key) => (active = key)}>
  {#if loadError}
    <div class="error-banner" role="alert">{loadError}</div>
  {/if}
  {#if active === 'overview'}
    <Overview {status} onRefresh={refreshStatus} />
  {:else if active === 'providers'}
    <Providers {providers} gatewayBusy={status.gatewayReady || status.gatewayStarting} onChanged={refreshProviders} />
  {:else if active === 'architect'}
    <Architect {consultations} onChanged={refreshConsultations} />
  {:else if active === 'mcp'}
    <MCP onChanged={refreshStatus} />
  {:else}
    <Settings />
  {/if}
</AppShell>
