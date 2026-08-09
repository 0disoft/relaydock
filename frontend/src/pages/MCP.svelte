<script lang="ts">
  import { getMCPConfigSnippet, getSettings } from '../lib/runtime-adapter';

  export let onChanged: () => Promise<void>;

  let snippet = `[mcp_servers.ai-runtime-expert]\ncommand = "mcp-bridge"`;
  let message = '';
  let errorMessage = '';

  async function generate(): Promise<void> {
    message = '';
    errorMessage = '';
    try {
      const settings = await getSettings();
      snippet = await getMCPConfigSnippet(settings.mcpBridgePath);
      await navigator.clipboard.writeText(snippet);
      message = '설정 문구를 클립보드에 복사했다.';
      await onChanged();
    } catch (error) {
      errorMessage = error instanceof Error ? error.message : String(error);
    }
  }
</script>

<header class="page-header">
  <div>
    <span class="eyebrow">TOOL BRIDGE</span>
    <h1>MCP 연결</h1>
    <p>코딩 도구에서 Architect와 맥락 선별 도구를 호출한다.</p>
  </div>
</header>

<section class="panel setup-card">
  <div>
    <h2>Codex</h2>
    <p>STDIO 브리지 실행 파일과 로컬 런타임을 연결한다.</p>
  </div>
  <button class="button-primary" type="button" on:click={generate}>설정 복사</button>
</section>

{#if message}<div class="form-message success">{message}</div>{/if}
{#if errorMessage}<div class="form-message error" role="alert">{errorMessage}</div>{/if}

<section class="panel code-card"><pre><code>{snippet}</code></pre></section>
