/* llmnav/1 module
id=relaydock.frontend.runtime-adapter
role=Keep desktop UI calls behind typed Wails service adapters and build privacy-aware ContextPack previews before consultation creation.
owns=frontend runtime adapter|frontend consultation operations|ContextPack preview defaults
excludes=Wails binding generation|backend authorization
search=frontend runtime calls|preview ContextPack UI|desktop consultation adapter
invariant=UI modules consume this adapter instead of importing generated bindings directly.
invariant=ContextPack previews use an explicit byte ceiling and report redaction findings.
stability=architecture
*/

import type {
  ConsultationCreateInput,
  ConsultationSummary,
  ContextPackPreview,
  ProviderSummary,
  RuntimeStatus,
  Settings,
  WebHandoff
} from './types';
import { ConsultationService, RuntimeService, SettingsService } from './wails-services';

export const getRuntimeStatus = (): Promise<RuntimeStatus> => RuntimeService.status();
export const listProviders = (): Promise<ProviderSummary[]> => RuntimeService.listProviders();
export const saveProviderCredential = (providerId: string, value: string): Promise<void> =>
  RuntimeService.saveProviderCredential(providerId, value);
export const deleteProviderCredential = (providerId: string): Promise<void> =>
  RuntimeService.deleteProviderCredential(providerId);
export const listConsultations = (): Promise<ConsultationSummary[]> => ConsultationService.list(50);
export const getSettings = (): Promise<Settings> => SettingsService.get();
export const saveSettings = (settings: Settings): Promise<void> => SettingsService.save(settings);
export const startGateway = (port: number): Promise<void> => RuntimeService.startGateway(port);
export const stopGateway = (): Promise<void> => RuntimeService.stopGateway();
export const getMCPConfigSnippet = (bridgePath: string): Promise<string> =>
  RuntimeService.mcpConfigSnippet(bridgePath);

export async function previewContextPack(
  objective: string,
  repositoryRoot: string,
  candidatePaths: string[] = []
): Promise<ContextPackPreview> {
  if (objective.trim().length === 0 || repositoryRoot.trim().length === 0) {
    return { id: '', files: 0, estimatedBytes: 0, estimatedTokens: 0, secretFindings: 0, excludedFiles: 0 };
  }
  const pack = await ConsultationService.preview({
    repositoryRoot,
    objective,
    candidatePaths,
    successCriteria: [],
    maximumBytes: 524_288,
    attempts: [],
    openQuestions: []
  });
  return {
    id: pack.id,
    files: pack.evidence.length,
    estimatedBytes: pack.estimatedBytes,
    estimatedTokens: pack.estimatedTokens,
    secretFindings: pack.redactionReport.removedFiles + pack.redactionReport.removedSegments,
    excludedFiles: pack.excludedFiles
  };
}

export async function createConsultation(input: ConsultationCreateInput): Promise<ConsultationSummary> {
  const output = await ConsultationService.create(input);
  return output.consultation;
}

export const approveConsultation = (id: string): Promise<ConsultationSummary> =>
  ConsultationService.approve(id);
export const runAPIExpert = (id: string): Promise<ConsultationSummary> =>
  ConsultationService.runAPIExpert(id);
export const createWebHandoff = (id: string): Promise<WebHandoff> =>
  ConsultationService.createWebHandoff(id);
export const importWebResult = (id: string, payload: string): Promise<ConsultationSummary> =>
  ConsultationService.importWebResult(id, payload);
