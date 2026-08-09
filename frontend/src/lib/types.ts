export type NavigationKey = 'overview' | 'providers' | 'architect' | 'mcp' | 'settings';

export interface RuntimeStatus {
  version: string;
  ipcReady: boolean;
  mcpConfigured: boolean;
  gatewayReady: boolean;
  gatewayAddress?: string;
  lastError?: string;
}

export interface ProviderSummary {
  id: string;
  name: string;
  configured: boolean;
  mode: string;
}

export interface ConsultationSummary {
  id: string;
  objective: string;
  taskType?: string;
  route: 'openai_api_pro' | 'chatgpt_web_handoff';
  state: string;
  contextPackId?: string;
  resultId?: string;
  updatedAt: string;
}

export interface WebHandoff {
  consultationId: string;
  readToken: string;
  expiresAt: string;
}

export interface ContextPackPreview {
  id: string;
  files: number;
  estimatedBytes: number;
  estimatedTokens: number;
  secretFindings: number;
  excludedFiles: number;
}

export interface Settings {
  startAtLogin: boolean;
  minimizeToTray: boolean;
  defaultRoute: 'openai_api_pro' | 'chatgpt_web_handoff';
  maximumCostMinor: number;
  gatewayPort: number;
  defaultRepositoryRoot: string;
  mcpBridgePath: string;
}

export interface ConsultationCreateInput {
  repositoryRoot: string;
  objective: string;
  taskType?: string;
  route: 'openai_api_pro' | 'chatgpt_web_handoff';
  candidatePaths: string[];
  successCriteria: string[];
  attempts: Array<{ approach: string; result: string; reason: string }>;
  openQuestions: string[];
  maximumBytes: number;
  maximumCostMinor: number;
  autoApprove: boolean;
}

export interface ContextPackRaw {
  id: string;
  evidence: Array<{ reference: string; bytes: number }>;
  estimatedBytes: number;
  estimatedTokens: number;
  excludedFiles: number;
  redactionReport: {
    removedFiles: number;
    removedSegments: number;
  };
}
