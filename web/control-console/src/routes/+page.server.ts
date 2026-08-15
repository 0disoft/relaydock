/* llmnav/1 module
id=relaydock.control.console-overview
role=Load authenticated health, readiness, model-route, and signed-snapshot status for the server-rendered control overview.
owns=control overview data loading|Control API probe aggregation|server-only bearer forwarding
search=control console overview|probe Control API status|load model route snapshot
invariant=The control bearer token is read only in the server loader and is never returned in page data.
invariant=Probe failures degrade to explicit offline or empty status without aborting the complete overview.
effect=net.call(control_api)
risk=availability
stability=architecture
*/

import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';

interface ProbeResult {
  ok: boolean;
  status: number;
  error?: string;
}

interface ModelRoute {
  virtualModel: string;
  candidates: string[];
  policyId?: string;
}

interface ModelsResponse {
  revision: number;
  models: ModelRoute[];
}

interface SnapshotResponse {
  revision: number;
  generatedAt: string;
  expiresAt: string;
}

function headers(): HeadersInit {
  const token = env.CONTROL_BEARER_TOKEN?.trim();
  return token ? { authorization: `Bearer ${token}` } : {};
}

async function probe(fetcher: typeof fetch, baseURL: string, path: string): Promise<ProbeResult> {
  try {
    const response = await fetcher(`${baseURL}${path}`, { headers: headers() });
    return { ok: response.ok, status: response.status };
  } catch (error) {
    return { ok: false, status: 0, error: error instanceof Error ? error.message : String(error) };
  }
}

async function readJSON<T>(fetcher: typeof fetch, baseURL: string, path: string): Promise<T | null> {
  try {
    const response = await fetcher(`${baseURL}${path}`, { headers: headers() });
    if (!response.ok) return null;
    return await response.json() as T;
  } catch {
    return null;
  }
}

export const load: PageServerLoad = async ({ fetch }) => {
  const baseURL = (env.CONTROL_API_BASE_URL ?? 'http://127.0.0.1:8081').replace(/\/$/, '');
  const [health, readiness, models, snapshot] = await Promise.all([
    probe(fetch, baseURL, '/healthz'),
    probe(fetch, baseURL, '/readyz'),
    readJSON<ModelsResponse>(fetch, baseURL, '/v1/models'),
    readJSON<SnapshotResponse>(fetch, baseURL, '/v1/snapshot')
  ]);

  return {
    baseURL,
    health,
    readiness,
    models: models?.models ?? [],
    revision: models?.revision ?? snapshot?.revision ?? 0,
    generatedAt: snapshot?.generatedAt ?? '',
    expiresAt: snapshot?.expiresAt ?? ''
  };
};
