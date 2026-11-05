package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"strings"
)

// Frozen combined raw/scope validator before the intrinsic compiler split.
func referenceAttemptScopeBeforeIntrinsicSplit(workspaceID common.WorkspaceID, runID common.RunID, values []RuntimeCandidateAttemptRecord) error {
	if err := ValidateRuntimeCandidateAttemptRecords(values); err != nil {
		return err
	}
	byContract := map[string][]RuntimeCandidateAttemptRecord{}
	for _, value := range values {
		if strings.TrimSpace(value.AuthorityContractID) == "" {
			return fmt.Errorf("runtime candidate attempt requires authority contract id")
		}
		contractID := CandidateAttemptAuthorityContractID(workspaceID, runID, value)
		byContract[contractID] = append(byContract[contractID], value)
		if value.AuthorityContractID != contractID || value.AuthorityContractID != StableAuthorityContractID(workspaceID, runID, value.Transaction) {
			return fmt.Errorf("runtime candidate authority contract id mismatch")
		}
		if value.BindingContradiction != nil {
			identity := value.BindingContradiction.Normalized()
			if err := identity.Validate(); err != nil || identity.WorkspaceID != workspaceID || identity.RunID != runID || identity.AuthorityInputID != value.Transaction.AuthorityInputID || value.State != RuntimeCandidateAttemptStateInvalidated || value.OwnerFeasibility == nil || !value.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
				return fmt.Errorf("runtime candidate binding contradiction identity mismatch")
			}
		}
	}
	for contractID, attempts := range byContract {
		attemptCounts := [2]int{}
		for _, attempt := range attempts {
			attemptCounts[attempt.Attempt]++
		}
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if len(attempts) > 2 || attemptCounts[0] > 1 || attemptCounts[1] > 1 || (hasSecond && (!hasFirst || first.State != RuntimeCandidateAttemptStateFailed || first.Correction == nil || first.Correction.Disposition != RuntimeCandidateCorrectionCorrectableOnce || !SameAuthorityContract(first.Transaction, second.Transaction) || first.StablePrefixHash != second.StablePrefixHash || first.ManifestHash != second.ManifestHash)) {
			return fmt.Errorf("runtime candidate authority contract %s exceeds the sealed attempt 0/1 transaction", contractID)
		}
		if hasFirst && first.State == RuntimeCandidateAttemptStateAccepted && hasSecond {
			return fmt.Errorf("runtime candidate authority contract %s retried after accepted attempt 0", contractID)
		}
	}
	return nil
}
