<script lang="ts">
  import ContextPreview from '../components/ContextPreview.svelte';
  import ConsultationCard from '../components/ConsultationCard.svelte';
  import { validateConsultationDraft } from '../lib/consultation-validation';
  import type { ConsultationSummary, ContextPackPreview, Settings, WebHandoff } from '../lib/types';
  import {
    createConsultation,
    createWebHandoff,
    getSettings,
    importWebResult,
    previewContextPack,
    runAPIExpert
  } from '../lib/runtime-adapter';

  export let consultations: ConsultationSummary[];
  export let onChanged: () => Promise<void>;

  let objective = '';
  let repositoryRoot = '';
  let settings: Settings | null = null;
  let preview: ContextPackPreview = {
    id: '', files: 0, estimatedBytes: 0, estimatedTokens: 0, secretFindings: 0, excludedFiles: 0
  };
  let handoff: WebHandoff | null = null;
  let webResultPayload = '';
  let busy = false;
  let errorMessage = '';
  let successMessage = '';

  async function ensureSettings(): Promise<Settings> {
    if (settings) return settings;
    settings = await getSettings();
    repositoryRoot ||= settings.defaultRepositoryRoot;
    return settings;
  }

  function validateDraft(current: Settings): boolean {
    const result = validateConsultationDraft({
      repositoryRoot,
      objective,
      maximumCostMinor: current.maximumCostMinor
    });
    if (result.valid) return true;
    errorMessage = result.errors.repositoryRoot
      ?? result.errors.objective
      ?? result.errors.maximumCostMinor
      ?? '입력값을 확인해야 한다.';
    return false;
  }

  async function previewContext(): Promise<void> {
    busy = true;
    errorMessage = '';
    successMessage = '';
    try {
      const current = await ensureSettings();
      if (!validateDraft(current)) return;
      preview = await previewContextPack(objective, repositoryRoot);
      successMessage = `${preview.files}개 파일을 검토 대상으로 선별했다.`;
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function createReview(): Promise<void> {
    busy = true;
    errorMessage = '';
    successMessage = '';
    handoff = null;
    webResultPayload = '';
    try {
      const current = await ensureSettings();
      if (!validateDraft(current)) return;
      const created = await createConsultation({
        repositoryRoot,
        objective,
        route: current.defaultRoute,
        candidatePaths: [],
        successCriteria: [],
        attempts: [],
        openQuestions: [],
        maximumBytes: 524_288,
        maximumCostMinor: current.maximumCostMinor,
        autoApprove: false
      });
      if (current.defaultRoute === 'openai_api_pro') {
        await runAPIExpert(created.id);
        successMessage = '전문가 검토를 완료했다.';
        objective = '';
      } else {
        handoff = await createWebHandoff(created.id);
        successMessage = '웹 검토용 상담을 만들었다.';
      }
      await onChanged();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }

  async function copyHandoffToken(): Promise<void> {
    if (!handoff) return;
    try {
      await navigator.clipboard.writeText(handoff.readToken);
      successMessage = '읽기 토큰을 복사했다.';
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }

  async function importResult(): Promise<void> {
    if (!handoff || webResultPayload.trim().length === 0) return;
    busy = true;
    errorMessage = '';
    successMessage = '';
    try {
      await importWebResult(handoff.consultationId, webResultPayload);
      successMessage = '웹 전문가 결과를 가져왔다.';
      handoff = null;
      webResultPayload = '';
      objective = '';
      await onChanged();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    } finally {
      busy = false;
    }
  }
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">EXPERT ESCALATION</span>
    <h1>Architect</h1>
    <p>관련 코드만 선별해 고난도 설계 검토로 넘긴다.</p>
  </div>
</header>

<section class="panel composer">
  <label for="repositoryRoot">저장소 경로</label>
  <input id="repositoryRoot" bind:value={repositoryRoot} placeholder="C:\projects\my-app" on:focus={() => void ensureSettings()} />
  <label for="objective">검토할 문제</label>
  <textarea id="objective" bind:value={objective} rows="5" placeholder="예: 멀티테넌트 과금 원장의 중복 capture 위험을 검토해줘"></textarea>
  {#if errorMessage}<div class="form-message error" role="alert">{errorMessage}</div>{/if}
  {#if successMessage}<div class="form-message success">{successMessage}</div>{/if}
  <div class="composer-actions">
    <button class="button-secondary" type="button" disabled={busy} on:click={previewContext}>맥락 미리보기</button>
    <button class="button-primary" type="button" disabled={busy} on:click={createReview}>전문가 검토</button>
  </div>
</section>

<ContextPreview {preview} />

{#if handoff}
  <section class="panel handoff-panel" aria-live="polite">
    <div class="handoff-header">
      <div>
        <span class="eyebrow">CHATGPT WEB HANDOFF</span>
        <h2>웹 전문가 검토</h2>
        <p>상담 ID {handoff.consultationId} · 만료 {new Date(handoff.expiresAt).toLocaleString()}</p>
      </div>
      <button class="button-secondary" type="button" on:click={copyHandoffToken}>읽기 토큰 복사</button>
    </div>
    <code class="handoff-token">{handoff.readToken}</code>
    <label for="webResultPayload">구조화된 검토 결과 JSON</label>
    <textarea
      id="webResultPayload"
      bind:value={webResultPayload}
      rows="10"
      spellcheck="false"
      placeholder={'{"decision":"...","confidence":0.8,"assumptions":[],"criticalFindings":[],"recommendedArchitecture":{},"rejectedAlternatives":[],"failureScenarios":[],"migrationOrder":[],"verificationPlan":[],"unresolvedQuestions":[],"evidenceReferences":[]}'}
    ></textarea>
    <div class="composer-actions">
      <button class="button-primary" type="button" disabled={busy || webResultPayload.trim().length === 0} on:click={importResult}>결과 가져오기</button>
    </div>
  </section>
{/if}

<section class="section-block">
  <h2>최근 검토</h2>
  {#if consultations.length === 0}
    <div class="panel empty-state"><p>아직 생성한 검토가 없다.</p></div>
  {:else}
    <div class="card-list">
      {#each consultations as consultation}
        <ConsultationCard {consultation} />
      {/each}
    </div>
  {/if}
</section>
