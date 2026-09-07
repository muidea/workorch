// Read-only labels. Limits are projected declarations, never client policy.
export function validationRepairPolicyLabel(value?: {
  validationRepairPolicyId?: string;
  validationRepairMaxAttempts?: number;
}): string {
  const id = value?.validationRepairPolicyId?.trim();
  const limit = value?.validationRepairMaxAttempts;
  return id && Number.isInteger(limit) && Number(limit) > 0 ? `${limit} (${id})` : '-';
}
