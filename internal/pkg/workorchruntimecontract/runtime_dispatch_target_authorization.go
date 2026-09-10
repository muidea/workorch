package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
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

// ValidateRuntimeDispatchOperationTargetAuthorizationProjection validates the
// accepted admission's standalone projection before a dispatch is considered.
// Candidate bijection is checked separately when that admission is matched to
// its dispatch; this fence ensures the independently persisted preimage is
// canonical and cannot hide duplicate or incomplete target policy entries.
func ValidateRuntimeDispatchOperationTargetAuthorizationProjection(values []RuntimeDispatchOperationTargetAuthorization) error {
	normalized := NormalizeRuntimeDispatchOperationTargetAuthorizations(values)
	if !reflect.DeepEqual(values, normalized) {
		return fmt.Errorf("runtime admission target authorization projection is not canonical")
	}
	seen := make(map[string]struct{}, len(normalized))
	for _, authorization := range normalized {
		if authorization.OperationID == "" || authorization.OperationHash == "" {
			return fmt.Errorf("runtime admission target authorization requires operation id and hash")
		}
		if _, duplicate := seen[authorization.OperationID]; duplicate {
			return fmt.Errorf("runtime admission target authorization for operation %q is duplicated", authorization.OperationID)
		}
		seen[authorization.OperationID] = struct{}{}
		target := authorization.Target
		switch target.Policy {
		case RuntimeOperationTargetPolicyExactTarget:
			if authorization.RequireTargets && len(target.ExactTargetRefs) == 0 {
				return fmt.Errorf("runtime admission target authorization for operation %q requires a non-empty exact target set", authorization.OperationID)
			}
		case RuntimeOperationTargetPolicyExistingOwnerScope:
			if len(target.OwnerScopeTargetRefs) == 0 && len(target.OwnerSelectorRefs) == 0 {
				return fmt.Errorf("runtime admission target authorization for operation %q requires owner scope or selector refs", authorization.OperationID)
			}
		case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
			if len(target.OwnerSelectorRefs) == 0 || len(target.ProspectiveTargetRefs) == 0 {
				return fmt.Errorf("runtime admission target authorization for operation %q requires prospective selector and target refs", authorization.OperationID)
			}
		default:
			return fmt.Errorf("runtime admission target authorization for operation %q has unsupported target policy %q", authorization.OperationID, target.Policy)
		}
	}
	return nil
}

// RuntimeDispatchOperationTargetAuthorizationProjectionsEqual compares the
// full canonical preimages, not only their hashes.
func RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(left, right []RuntimeDispatchOperationTargetAuthorization) bool {
	return reflect.DeepEqual(
		NormalizeRuntimeDispatchOperationTargetAuthorizations(left),
		NormalizeRuntimeDispatchOperationTargetAuthorizations(right),
	)
}

// RuntimeCandidateAdmissionTargetAuthorizationsFromManifest derives the
// accepted admission's standalone target projection from its exact
// manifest/candidate preimage. It intentionally does not consume a dispatch:
// the accepted attempt remains an independent fence against a coordinated
// rewrite of dispatch authority during persistence or recovery.
func RuntimeCandidateAdmissionTargetAuthorizationsFromManifest(
	manifest RuntimeCandidateContractManifest,
	candidate RuntimeCandidate,
) ([]RuntimeDispatchOperationTargetAuthorization, error) {
	return runtimeOperationTargetAuthorizationsFromManifest(manifest, candidate)
}

// RuntimeDispatchOperationTargetAuthorizationsFromManifest derives the
// dispatch-local target authority from the exact sealed manifest preimage. The
// manifest is validated against the candidate again at this final SM-3 seal
// boundary so a caller cannot pass a candidate admitted under another shape.
func RuntimeDispatchOperationTargetAuthorizationsFromManifest(
	manifest RuntimeCandidateContractManifest,
	candidate RuntimeCandidate,
) ([]RuntimeDispatchOperationTargetAuthorization, error) {
	return runtimeOperationTargetAuthorizationsFromManifest(manifest, candidate)
}

func runtimeOperationTargetAuthorizationsFromManifest(
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
	if err := ValidateRuntimeDispatchOperationTargetAuthorizationProjection(values); err != nil {
		return err
	}
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

// RuntimeDispatchOperationTargetAuthorizationForOperation resolves one
// operation's immutable target authority from a complete sealed dispatch
// preimage. It first validates the candidate/authorization bijection, then
// requires the supplied operation to be the exact candidate operation rather
// than an ID-only substitute. Callers therefore cannot rebuild target scope
// from mutable owner facts or accidentally apply another operation's scope.
func RuntimeDispatchOperationTargetAuthorizationForOperation(
	candidate RuntimeCandidate,
	values []RuntimeDispatchOperationTargetAuthorization,
	operation RuntimeOperation,
) (RuntimeDispatchOperationTargetAuthorization, error) {
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, values); err != nil {
		return RuntimeDispatchOperationTargetAuthorization{}, err
	}
	operation = operation.Normalized()
	if operation.ID == "" {
		return RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("runtime dispatch target authorization lookup requires operation id")
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return RuntimeDispatchOperationTargetAuthorization{}, err
	}
	foundOperation := false
	for _, rawCandidateOperation := range operations {
		candidateOperation := rawCandidateOperation.Normalized()
		if candidateOperation.ID != operation.ID {
			continue
		}
		if StableRuntimeOperationHash(candidateOperation) != StableRuntimeOperationHash(operation) {
			return RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("runtime dispatch operation %q does not match sealed candidate operation", operation.ID)
		}
		foundOperation = true
		break
	}
	if !foundOperation {
		return RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("runtime dispatch operation %q is not present in sealed candidate", operation.ID)
	}
	for _, rawAuthorization := range values {
		authorization := rawAuthorization.Normalized()
		if authorization.OperationID == operation.ID {
			return authorization.Clone(), nil
		}
	}
	return RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("runtime dispatch operation %q has no target authorization", operation.ID)
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
		writeRuntimeExcludedTargetsHash(h, value.Target.ExcludedTargetRefs)
	}
	return hex.EncodeToString(h.Sum(nil))
}
