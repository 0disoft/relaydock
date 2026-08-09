<script lang="ts">
  import StatusBadge from './StatusBadge.svelte';
  import type { ProviderSummary } from '../lib/types';

  export let provider: ProviderSummary;
  export let gatewayBusy: boolean;
  export let onSave: (providerId: string, value: string) => Promise<void>;
  export let onDelete: (providerId: string) => Promise<void>;

  let credential = '';
  let busy = false;
  let message = '';
  let errorMessage = '';
  const inputId = `provider-credential-${provider.id}`;
  const errorId = `${inputId}-error`;

  const sourceLabels: Record<ProviderSummary['credentialSource'], string> = {
    embedded: '내장 공급자',
    environment: '환경 변수에서 불러옴',
    system: '시스템 자격 증명 저장소에 저장됨',
    none: '저장된 자격 증명 없음',
    anonymous: '자격 증명 없이 사용자 지정 엔드포인트 사용',
    unavailable: '이 플랫폼에서 시스템 자격 증명 저장소를 사용할 수 없음'
  };

  async function save(): Promise<void> {
    busy = true;
    message = '';
    errorMessage = '';
    try {
      await onSave(provider.id, credential);
      message = 'API 키를 시스템 자격 증명 저장소에 저장했다.';
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      credential = '';
      busy = false;
    }
  }

  async function remove(): Promise<void> {
    busy = true;
    message = '';
    errorMessage = '';
    try {
      await onDelete(provider.id);
      message = '저장된 API 키를 삭제했다.';
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      credential = '';
      busy = false;
    }
  }
</script>

<article class="panel provider-card" aria-busy={busy}>
  <div class="provider-identity">
    <h3>{provider.name}</h3>
    <p>{provider.mode}</p>
    <small>{sourceLabels[provider.credentialSource]}</small>
  </div>
  <div class="provider-status"><StatusBadge state={provider.configured ? 'healthy' : 'unconfigured'} /></div>
  {#if provider.credentialWritable}
    <div class="provider-credential">
      <label for={inputId}>
        API 키
        <input
          id={inputId}
          type="password"
          bind:value={credential}
          autocomplete="off"
          spellcheck="false"
          maxlength="2048"
          aria-invalid={errorMessage ? 'true' : 'false'}
          aria-describedby={errorMessage ? errorId : undefined}
          placeholder={provider.configured ? '새 키로 교체' : '시스템 저장소에 저장'}
          disabled={busy || gatewayBusy}
        />
      </label>
      <div class="provider-actions">
        <button class="button-primary" type="button" disabled={busy || gatewayBusy || credential.length === 0} on:click={save}>
          {provider.configured ? '교체' : '저장'}
        </button>
        {#if provider.credentialSource === 'system'}
          <button class="button-secondary" type="button" disabled={busy || gatewayBusy} aria-label={`${provider.name} 저장 API 키 삭제`} on:click={remove}>저장 키 삭제</button>
        {/if}
      </div>
      {#if gatewayBusy}<small>키를 바꾸려면 로컬 게이트웨이를 먼저 중지한다.</small>{/if}
      {#if message}<div class="form-message success" role="status">{message}</div>{/if}
      {#if errorMessage}<div id={errorId} class="form-message error" role="alert">{errorMessage}</div>{/if}
    </div>
  {/if}
</article>
