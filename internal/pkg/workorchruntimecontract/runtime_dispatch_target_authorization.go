package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RuntimeDispatchOperationTargetAuthorization is the minimal immutable target
// authority retained by SM-3 for one sealed operation. It is derived from the
// exact candidate manifest used for admission; fresh-load validation never
// recompiles it from mutable owner facts.
type RuntimeDispatchOperationTargetAuthorization struct {
	OperationID    string
	OperationHash  string
	RequireTargets bool
	Target         RuntimeOperationTargetAuthorization
}

func (a RuntimeDispatchOperationTargetAuthorization) Normalized() RuntimeDispatchOperationTargetAuthorization {
	a.OperationID = strings.TrimSpace(a.OperationID)
	a.OperationHash = strings.TrimSpace(a.OperationHash)
	a.Target = a.Target.Normalized()
	return a
}

func (a RuntimeDispatchOperationTargetAuthorization) Clone() RuntimeDispatchOperationTargetAuthorization {
	return a.Normalized()
}

func NormalizeRuntimeDispatchOperationTargetAuthorizations(values []RuntimeDispatchOperationTargetAuthorization) []RuntimeDispatchOperationTargetAuthorization {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeDispatchOperationTargetAuthorization, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Normalized())
	}
	sort.Slice(ret, func(left, right int) bool {
		return ret[left].OperationID < ret[right].OperationID
	})
	return ret
}

func CloneRuntimeDispatchOperationTargetAuthorizations(values []RuntimeDispatchOperationTargetAuthorization) []RuntimeDispatchOperationTargetAuthorization {
	return NormalizeRuntimeDispatchOperationTargetAuthorizations(values)
}

// RuntimeDispatchOperationTargetAuthorizationsFromManifest derives the
// dispatch-local target authority from the exact sealed manifest preimage. The
// manifest is validated against the candidate again at this final SM-3 seal
// boundary so a caller cannot pass a candidate admitted under another shape.
func RuntimeDispatchOperationTargetAuthorizationsFromManifest(
	manifest RuntimeCandidateContractManifest,
	candidate RuntimeCandidate,
) ([]RuntimeDispatchOperationTargetAuthorization, error) {
	manifest = manifest.Normalized()
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		return nil, err
	}
	if candidate.Assessment != nil || candidate.Governance != nil {
		return nil, nil
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return nil, err
	}
	ret := make([]RuntimeDispatchOperationTargetAuthorization, 0, len(operations))
	for _, rawOperation := range operations {
		operation := rawOperation.Normalized()
		authorization, found := runtimeCandidatePayloadOperationManifest(manifest, operation.Kind)
		if !found {
			return nil, fmt.Errorf("runtime dispatch operation %q has no manifest target authorization", operation.ID)
		}
		ret = append(ret, RuntimeDispatchOperationTargetAuthorization{
			OperationID:    operation.ID,
			OperationHash:  StableRuntimeOperationHash(operation),
			RequireTargets: authorization.RequireTargetRefs,
			Target:         authorization.Target.Normalized(),
		})
	}
	ret = NormalizeRuntimeDispatchOperationTargetAuthorizations(ret)
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

// ValidateRuntimeDispatchOperationTargetAuthorizations proves a strict
// candidate-operation/snapshot bijection and replays the canonical target
// matcher from only the sealed dispatch. Operation-bearing candidates require
// exactly one record per operation; assessment/governance candidates require
// none.
func ValidateRuntimeDispatchOperationTargetAuthorizations(
	candidate RuntimeCandidate,
	values []RuntimeDispatchOperationTargetAuthorization,
) error {
	candidateKindCount := 0
	for _, present := range []bool{
		candidate.StateChange != nil,
		candidate.Validation != nil,
		candidate.Assessment != nil,
		candidate.Governance != nil,
	} {
		if present {
			candidateKindCount++
		}
	}
	if candidateKindCount != 1 {
		return fmt.Errorf("runtime dispatch target authorization requires exactly one candidate kind")
	}
	if candidate.Assessment != nil || candidate.Governance != nil {
		if len(values) != 0 {
			return fmt.Errorf("non-operation runtime dispatch cannot carry operation target authorization")
		}
		return nil
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return err
	}
	if len(values) != len(operations) {
		return fmt.Errorf("runtime dispatch requires exactly one target authorization per candidate operation")
	}
	byID := make(map[string]RuntimeDispatchOperationTargetAuthorization, len(values))
	for _, raw := range values {
		authorization := raw.Normalized()
		if authorization.OperationID == "" || authorization.OperationHash == "" {
			return fmt.Errorf("runtime dispatch target authorization requires operation id and hash")
		}
		if _, duplicate := byID[authorization.OperationID]; duplicate {
			return fmt.Errorf("runtime dispatch target authorization for operation %q is duplicated", authorization.OperationID)
		}
		byID[authorization.OperationID] = authorization
	}
	for _, rawOperation := range operations {
		operation := rawOperation.Normalized()
		authorization, found := byID[operation.ID]
		if !found {
			return fmt.Errorf("runtime dispatch operation %q has no target authorization", operation.ID)
		}
		if authorization.OperationHash != StableRuntimeOperationHash(operation) {
			return fmt.Errorf("runtime dispatch operation %q target authorization hash mismatch", operation.ID)
		}
		if err := ValidateRuntimeOperationTargetsAgainstAuthorization(authorization.Target, operation.TargetRefs, authorization.RequireTargets); err != nil {
			return fmt.Errorf("runtime dispatch operation %q target authorization: %w", operation.ID, err)
		}
	}
	return nil
}

// StableRuntimeDispatchOperationTargetAuthorizationsHash seals the canonical
// per-operation snapshot set into the enclosing dispatch authority hash.
func StableRuntimeDispatchOperationTargetAuthorizationsHash(values []RuntimeDispatchOperationTargetAuthorization) string {
	values = NormalizeRuntimeDispatchOperationTargetAuthorizations(values)
	h := sha256.New()
	writeHashString(h, "runtime-dispatch-operation-target-authorizations/v1")
	writeHashString(h, fmt.Sprintf("%d", len(values)))
	for _, value := range values {
		writeHashString(h, value.OperationID)
		writeHashString(h, value.OperationHash)
		writeHashString(h, fmt.Sprintf("%t", value.RequireTargets))
		writeHashString(h, string(value.Target.Policy))
		writeHashStrings(h, value.Target.ExactTargetRefs)
		writeHashStrings(h, value.Target.OwnerScopeTargetRefs)
		writeHashStrings(h, value.Target.OwnerSelectorRefs)
		writeHashStrings(h, value.Target.ProspectiveConstraintRefs)
		writeHashStrings(h, value.Target.ProspectiveTargetRefs)
	}
	return hex.EncodeToString(h.Sum(nil))
}
