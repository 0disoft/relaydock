export interface ConsultationDraft {
  repositoryRoot: string;
  objective: string;
  maximumCostMinor: number;
}

export interface ValidationResult {
  valid: boolean;
  errors: Partial<Record<keyof ConsultationDraft, string>>;
}

export function validateConsultationDraft(draft: ConsultationDraft): ValidationResult {
  const errors: ValidationResult['errors'] = {};
  const repositoryRoot = draft.repositoryRoot.trim();
  const objective = draft.objective.trim();

  if (!repositoryRoot) {
    errors.repositoryRoot = '저장소 경로를 입력해야 한다.';
  } else if (repositoryRoot.includes('\0')) {
    errors.repositoryRoot = '저장소 경로에 잘못된 문자가 포함되어 있다.';
  }

  if (objective.length < 12) {
    errors.objective = '검토할 문제를 12자 이상으로 구체적으로 입력해야 한다.';
  } else if (objective.length > 8_000) {
    errors.objective = '검토할 문제는 8,000자를 넘길 수 없다.';
  }

  if (!Number.isSafeInteger(draft.maximumCostMinor) || draft.maximumCostMinor < 0) {
    errors.maximumCostMinor = '최대 비용은 0 이상의 정수여야 한다.';
  }

  return { valid: Object.keys(errors).length === 0, errors };
}
