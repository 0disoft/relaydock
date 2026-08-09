import { Call as $Call } from '/wails/runtime.js';
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

export const RuntimeService = {
  status: () => $Call.ByName<RuntimeStatus>(`${prefix}.RuntimeService.Status`),
  listProviders: () => $Call.ByName<ProviderSummary[]>(`${prefix}.RuntimeService.ListProviders`),
  startGateway: (port: number) => $Call.ByName<void>(`${prefix}.RuntimeService.StartLocalGateway`, port),
  stopGateway: () => $Call.ByName<void>(`${prefix}.RuntimeService.StopLocalGateway`),
  mcpConfigSnippet: (bridgePath: string) =>
    $Call.ByName<string>(`${prefix}.RuntimeService.MCPConfigSnippet`, bridgePath)
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
  }) => $Call.ByName<ContextPackRaw>(`${prefix}.ConsultationService.PreviewContext`, request),
  create: (input: ConsultationCreateInput) =>
    $Call.ByName<{ consultation: ConsultationSummary; contextPack: ContextPackRaw }>(
      `${prefix}.ConsultationService.Create`,
      input
    ),
  list: (limit: number) =>
    $Call.ByName<ConsultationSummary[]>(`${prefix}.ConsultationService.List`, limit),
  approve: (id: string) =>
    $Call.ByName<ConsultationSummary>(`${prefix}.ConsultationService.Approve`, id),
  cancel: (id: string) =>
    $Call.ByName<ConsultationSummary>(`${prefix}.ConsultationService.Cancel`, id),
  runAPIExpert: (consultationId: string, model = '', reasoningMode = 'pro', reasoningEffort = 'max') =>
    $Call.ByName<ConsultationSummary>(`${prefix}.ConsultationService.RunAPIExpert`, {
      consultationId,
      model,
      reasoningMode,
      reasoningEffort
    }),
  createWebHandoff: (consultationId: string) =>
    $Call.ByName<WebHandoff>(`${prefix}.ConsultationService.CreateWebHandoff`, consultationId),
  importWebResult: (consultationId: string, payload: string) =>
    $Call.ByName<ConsultationSummary>(`${prefix}.ConsultationService.ImportWebResult`, consultationId, payload)
};

export const SettingsService = {
  get: () => $Call.ByName<Settings>(`${prefix}.SettingsService.Get`),
  save: (settings: Settings) => $Call.ByName<void>(`${prefix}.SettingsService.Save`, settings)
};
