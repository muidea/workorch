import type { RuntimeOriginalDiagnostic } from './types.ts';

// Read-only labels. Limits and owner causes are projected facts, never client policy.
export function originalDiagnosticLabel(value?: RuntimeOriginalDiagnostic): string {
  return [value?.ownerModuleId, value?.originalCode].filter(Boolean).join(' / ') || '-';
}

export function validationRepairPolicyLabel(value?: {
  validationRepairPolicyId?: string;
  validationRepairMaxAttempts?: number;
}): string {
  const id = value?.validationRepairPolicyId?.trim();
  const limit = value?.validationRepairMaxAttempts;
  return id && Number.isInteger(limit) && Number(limit) > 0 ? `${limit} (${id})` : '-';
}
