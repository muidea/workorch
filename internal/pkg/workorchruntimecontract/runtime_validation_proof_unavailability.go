package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeValidationProofUnavailability is a current capability-owner fact, not
// a preparation, proof binding or Runtime disposition. The owner cannot make
// this exact requirement observable by its declared prospective acquisition.
// In particular, creating another input cannot repair an invalid existing one.
type RuntimeValidationProofUnavailability struct {
	Requirement        RuntimeValidationProofRequirement
	ValidationRecipeID string
	OriginalDiagnostic *RuntimeOriginalDiagnostic
}

func (v RuntimeValidationProofUnavailability) Normalized() RuntimeValidationProofUnavailability {
	v.Requirement = v.Requirement.Normalized()
	v.ValidationRecipeID = strings.TrimSpace(v.ValidationRecipeID)
	v.OriginalDiagnostic = v.OriginalDiagnostic.CloneOriginal()
	return v
}

func (v RuntimeValidationProofUnavailability) Validate() error {
	if err := v.Requirement.Validate(); err != nil {
		return err
	}
	if v.ValidationRecipeID == "" || (v.Requirement.ValidationRecipeID != "" && v.Requirement.ValidationRecipeID != v.ValidationRecipeID) {
		return fmt.Errorf("validation proof unavailability requires its exact recipe")
	}
	if v.OriginalDiagnostic == nil {
		return fmt.Errorf("validation proof unavailability requires the original owner diagnostic")
	}
	return v.OriginalDiagnostic.Validate()
}

func normalizeRuntimeValidationProofUnavailabilities(values []RuntimeValidationProofUnavailability) []RuntimeValidationProofUnavailability {
	out := make([]RuntimeValidationProofUnavailability, 0, len(values))
	for _, value := range values {
		out = append(out, value.Normalized())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requirement.Hash < out[j].Requirement.Hash })
	return nilIfEmpty(out)
}

// RuntimeOwnerFactHasUnavailableValidationProof only resolves owner facts. It
// cannot choose an action, consume a budget or interpret owner diagnostic codes.
func RuntimeOwnerFactHasUnavailableValidationProof(requirement RuntimeOwnerFactRequirement, catalogs []RuntimeOperationAdmissibility) bool {
	requirement = requirement.Normalized()
	if requirement.ValidationProofRequirementHash == "" || requirement.FactKind != RuntimeOwnerFactSelectableValidationSurface {
		return false
	}
	for _, catalog := range catalogs {
		if strings.TrimSpace(catalog.OwnerModuleID) != requirement.OwnerModuleID {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			if strings.TrimSpace(surface.OwnerModuleID) != requirement.OwnerModuleID || CanonicalRuntimeBlockKind(surface.OperationKind) != requirement.OperationKind {
				continue
			}
			for _, unavailable := range surface.ProofRequirementUnavailabilities {
				unavailable = unavailable.Normalized()
				if unavailable.Requirement.Hash == requirement.ValidationProofRequirementHash && unavailable.Validate() == nil &&
					unavailable.ValidationRecipeID == strings.TrimSpace(surface.ID) && unavailable.OriginalDiagnostic.OwnerModuleID == requirement.OwnerModuleID &&
					unavailable.OriginalDiagnostic.ObservedRevision == strings.TrimSpace(catalog.ObservedRevision) {
					return true
				}
			}
		}
	}
	return false
}

func writeRuntimeValidationProofUnavailabilitiesHash(h interface{ Write([]byte) (int, error) }, values []RuntimeValidationProofUnavailability) {
	// Empty extension must not change historical catalog/manifest identities.
	for _, value := range values {
		writeHashString(h, "validation-proof-unavailability/v1")
		writeHashString(h, value.Requirement.Hash)
		writeHashString(h, value.ValidationRecipeID)
		writeHashString(h, StableRuntimeFeedbackHash(RuntimeFeedback{OriginalDiagnostic: value.OriginalDiagnostic}))
	}
}
