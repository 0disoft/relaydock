<script lang="ts">
  import type { ConsultationSummary } from '../lib/types';

  export let consultation: ConsultationSummary;
</script>

<article class="panel consultation-card">
  <div>
    <span>{consultation.route === 'openai_api_pro' ? 'API Pro' : 'Web Handoff'}</span>
    <h3>{consultation.objective}</h3>
  </div>
  <div class="consultation-meta">
    <strong>{consultation.state}</strong>
    <time datetime={consultation.updatedAt}>{consultation.updatedAt}</time>
  </div>
</article>
