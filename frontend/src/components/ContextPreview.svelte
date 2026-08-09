<script lang="ts">
  import type { ContextPackPreview } from '../lib/types';
  export let preview: ContextPackPreview;
</script>

<div class="context-grid">
  <div class="panel metric"><span>전송 파일</span><strong>{preview.files}</strong></div>
  <div class="panel metric"><span>전송 크기</span><strong>{Math.ceil(preview.estimatedBytes / 1024).toLocaleString()} KB</strong></div>
  <div class="panel metric"><span>예상 토큰</span><strong>{preview.estimatedTokens.toLocaleString()}</strong></div>
  <div class="panel metric"><span>제외된 비밀 후보</span><strong>{preview.secretFindings}</strong></div>
</div>
