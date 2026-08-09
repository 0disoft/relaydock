<script lang="ts">
  import ProviderCard from '../components/ProviderCard.svelte';
  import type { ProviderSummary } from '../lib/types';
  import { deleteProviderCredential, saveProviderCredential } from '../lib/runtime-adapter';
  export let providers: ProviderSummary[];
  export let gatewayBusy: boolean;
  export let onChanged: () => Promise<void>;

  async function save(providerId: string, value: string): Promise<void> {
    await saveProviderCredential(providerId, value);
    await onChanged();
  }

  async function remove(providerId: string): Promise<void> {
    await deleteProviderCredential(providerId);
    await onChanged();
  }
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">CONNECTIONS</span>
    <h1>공급자</h1>
    <p>공식 API, 로컬 모델, 사용자 소유 자격 증명을 관리한다.</p>
  </div>
  <button class="button-primary" type="button" disabled>공급자 추가</button>
</header>

{#if providers.length === 0}
  <section class="panel empty-state">
    <h2>연결된 공급자가 없다</h2>
    <p>첫 공급자를 연결하면 모델과 상태가 여기에 표시된다.</p>
  </section>
{:else}
  <section class="card-list">
    {#each providers as provider}
      <ProviderCard {provider} {gatewayBusy} onSave={save} onDelete={remove} />
    {/each}
  </section>
{/if}
