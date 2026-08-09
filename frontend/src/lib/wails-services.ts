import { Call as $Call } from '@wailsio/runtime';
import type {
  ConsultationCreateInput,
  ConsultationSummary,
  ContextPackRaw,
  ProviderSummary,
  RuntimeStatus,
  Settings,
  WebHandoff
} from './types';

const prefix = 'github.com/0disoft/relaydock/internal/desktopwails';

function call<T>(methodName: string, ...args: unknown[]): Promise<T> {
  return $Call.ByName(methodName, ...args);
}

export const RuntimeService = {
  status: () => call<RuntimeStatus>(`${prefix}.RuntimeService.Status`),
  listProviders: () => call<ProviderSummary[]>(`${prefix}.RuntimeService.ListProviders`),
  saveProviderCredential: (providerId: string, value: string) =>
    call<void>(`${prefix}.RuntimeService.SaveProviderCredential`, providerId, value),
  deleteProviderCredential: (providerId: string) =>
    call<void>(`${prefix}.RuntimeService.DeleteProviderCredential`, providerId),
  startGateway: (port: number) => call<void>(`${prefix}.RuntimeService.StartLocalGateway`, port),
  stopGateway: () => call<void>(`${prefix}.RuntimeService.StopLocalGateway`),
  mcpConfigSnippet: (bridgePath: string) =>
    call<string>(`${prefix}.RuntimeService.MCPConfigSnippet`, bridgePath)
};

export const ConsultationService = {
  preview: (request: {
    repositoryRoot: string;
    objective: string;
    candidatePaths: string[];
    successCriteria: string[];
    maximumBytes: number;
    attempts: unknown[];
    openQuestions: string[];
  }) => call<ContextPackRaw>(`${prefix}.ConsultationService.PreviewContext`, request),
  create: (input: ConsultationCreateInput) =>
    call<{ consultation: ConsultationSummary; contextPack: ContextPackRaw }>(
      `${prefix}.ConsultationService.Create`,
      input
    ),
  list: (limit: number) =>
    call<ConsultationSummary[]>(`${prefix}.ConsultationService.List`, limit),
  approve: (id: string) =>
    call<ConsultationSummary>(`${prefix}.ConsultationService.Approve`, id),
  cancel: (id: string) =>
    call<ConsultationSummary>(`${prefix}.ConsultationService.Cancel`, id),
  runAPIExpert: (consultationId: string, model = '', reasoningMode = 'pro', reasoningEffort = 'max') =>
    call<ConsultationSummary>(`${prefix}.ConsultationService.RunAPIExpert`, {
      consultationId,
      model,
      reasoningMode,
      reasoningEffort
    }),
  createWebHandoff: (consultationId: string) =>
    call<WebHandoff>(`${prefix}.ConsultationService.CreateWebHandoff`, consultationId),
  importWebResult: (consultationId: string, payload: string) =>
    call<ConsultationSummary>(`${prefix}.ConsultationService.ImportWebResult`, consultationId, payload)
};

export const SettingsService = {
  get: () => call<Settings>(`${prefix}.SettingsService.Get`),
  save: (settings: Settings) => call<void>(`${prefix}.SettingsService.Save`, settings)
};
