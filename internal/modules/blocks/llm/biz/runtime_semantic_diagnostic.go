package llm

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeSemanticDiagnosticProjection struct {
	OwnerModuleID    string                                  `json:"ownerModuleId"`
	Class            string                                  `json:"class"`
	Code             string                                  `json:"code"`
	Signature        string                                  `json:"signature"`
	OperationID      string                                  `json:"operationId,omitempty"`
	TargetRefs       []string                                `json:"targetRefs,omitempty"`
	ExpectedRevision string                                  `json:"expectedRevision,omitempty"`
	ObservedRevision string                                  `json:"observedRevision,omitempty"`
	FieldViolations  []runtimecontract.RuntimeFieldViolation `json:"fieldViolations,omitempty"`
	OmittedReason    string                                  `json:"omittedReason,omitempty"`
	Truncated        bool                                    `json:"truncated,omitempty"`
	Redacted         bool                                    `json:"redacted,omitempty"`
}

type runtimeSemanticTailProjection struct {
	Attempt             int                                                 `json:"attempt"`
	PreviousRejection   *runtimeSemanticDiagnosticProjection                `json:"previousRejection,omitempty"`
	CorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective `json:"correctionDirective,omitempty"`
	ExistingTarget      *runtimeSemanticExistingTargetProjection            `json:"existingTarget,omitempty"`
}

// runtimeSemanticExistingTargetProjection keeps the latest owner catalog and
// target revision beside an attempt-1 rejection. C3 remains the authoritative
// manifest; this bounded C4 projection makes the correction input explicit and
// prevents a provider from treating a create-only fact as an existing-target
// replacement. It contains no owner-authored message or next-action choice.
type runtimeSemanticExistingTargetProjection struct {
	OwnerModuleID             string   `json:"ownerModuleId"`
	FailureClass              string   `json:"failureClass,omitempty"`
	FailureCode               string   `json:"failureCode,omitempty"`
	FailureSignature          string   `json:"failureSignature,omitempty"`
	OperationID               string   `json:"operationId,omitempty"`
	TargetRefs                []string `json:"targetRefs,omitempty"`
	ExpectedRevision          string   `json:"expectedRevision,omitempty"`
	ObservedRevision          string   `json:"observedRevision,omitempty"`
	CatalogSnapshotID         string   `json:"catalogSnapshotId"`
	CatalogObservedRevision   string   `json:"catalogObservedRevision,omitempty"`
	CatalogTargetRefs         []string `json:"catalogTargetRefs,omitempty"`
	ConstraintRefs            []string `json:"constraintRefs,omitempty"`
	ExistingOperationKinds    []string `json:"existingOperationKinds,omitempty"`
	ExistingOperationVariants []string `json:"existingOperationVariants,omitempty"`
	ExistingTargetObserved    bool     `json:"existingTargetObserved"`
}

func runtimeProviderSafeSemanticDiagnostic(value *runtimecontract.RuntimeOriginalDiagnostic) *runtimeSemanticDiagnosticProjection {
	if value == nil {
		return nil
	}
	return &runtimeSemanticDiagnosticProjection{
		OwnerModuleID: value.OwnerModuleID, Class: value.OriginalClass, Code: value.OriginalCode,
		Signature: value.Signature, OperationID: value.OperationID,
		TargetRefs: append([]string(nil), value.TargetRefs...), ExpectedRevision: value.ExpectedRevision,
		ObservedRevision: value.ObservedRevision, FieldViolations: cloneRuntimeFieldViolations(value.FieldViolations),
		OmittedReason: value.OmittedReason, Truncated: value.Truncated, Redacted: value.Redacted,
	}
}

func runtimeSemanticExistingTargetForProvider(manifest runtimecontract.RuntimeSemanticPlanManifest, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) *runtimeSemanticExistingTargetProjection {
	if diagnostic == nil {
		return nil
	}
	manifest = manifest.Normalized()
	owner := strings.TrimSpace(diagnostic.OwnerModuleID)
	var catalog runtimecontract.RuntimeOperationAdmissibility
	found := false
	for _, value := range manifest.OwnerCatalogs {
		if value.OwnerModuleID == owner {
			catalog = value.Normalized()
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	projection := &runtimeSemanticExistingTargetProjection{
		OwnerModuleID: owner, FailureClass: strings.TrimSpace(diagnostic.OriginalClass), FailureCode: strings.TrimSpace(diagnostic.OriginalCode), FailureSignature: strings.TrimSpace(diagnostic.Signature), OperationID: strings.TrimSpace(diagnostic.OperationID), TargetRefs: append([]string(nil), diagnostic.TargetRefs...), ExpectedRevision: strings.TrimSpace(diagnostic.ExpectedRevision), ObservedRevision: strings.TrimSpace(diagnostic.ObservedRevision), CatalogSnapshotID: catalog.SnapshotID, CatalogObservedRevision: catalog.ObservedRevision, CatalogTargetRefs: append([]string(nil), catalog.TargetRefs...), ConstraintRefs: append([]string(nil), catalog.ConstraintRefs...),
	}
	if observation := diagnostic.TargetObservation; observation != nil {
		observed := observation.Normalized()
		projection.TargetRefs = appendUniqueStrings(projection.TargetRefs, observed.TargetRefs...)
		if projection.ObservedRevision == "" {
			projection.ObservedRevision = observed.ObservedRevision
		}
		projection.ExistingTargetObserved = len(observed.TargetRefs) != 0 || observed.ObservedRevision != ""
	}
	for _, target := range projection.TargetRefs {
		for _, catalogTarget := range catalog.TargetRefs {
			if strings.TrimSpace(target) != "" && strings.TrimSpace(target) == strings.TrimSpace(catalogTarget) {
				projection.ExistingTargetObserved = true
				break
			}
		}
	}
	for _, variant := range catalog.OperationVariants {
		variant = variant.Normalized()
		projection.ConstraintRefs = appendUniqueStrings(projection.ConstraintRefs, variant.ConstraintRefs...)
		allowedExact, _, _, _, exactErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, variant.Kind, variant.VariantID, runtimecontract.RuntimeOperationTargetPolicyExactTarget)
		allowedExisting, _, _, _, existingErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, variant.Kind, variant.VariantID, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
		if exactErr != nil || existingErr != nil || (!allowedExact && !allowedExisting) {
			continue
		}
		if variant.VariantID == "" {
			projection.ExistingOperationKinds = appendUniqueStrings(projection.ExistingOperationKinds, variant.Kind)
		} else {
			projection.ExistingOperationVariants = appendUniqueStrings(projection.ExistingOperationVariants, variant.Kind+"/"+variant.VariantID)
		}
	}
	for _, kind := range catalog.OperationKinds {
		if contract, found := runtimecontract.RuntimeOperationContractForKind(kind); found && len(contract.Variants) != 0 {
			continue
		}
		allowedExact, _, _, _, exactErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", runtimecontract.RuntimeOperationTargetPolicyExactTarget)
		allowedExisting, _, _, _, existingErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope)
		if exactErr == nil && existingErr == nil && (allowedExact || allowedExisting) {
			projection.ExistingOperationKinds = appendUniqueStrings(projection.ExistingOperationKinds, runtimecontract.CanonicalRuntimeBlockKind(kind))
		}
	}
	return projection
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if strings.TrimSpace(existing) == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

func cloneRuntimeFieldViolations(values []runtimecontract.RuntimeFieldViolation) []runtimecontract.RuntimeFieldViolation {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeFieldViolation, len(values))
	for idx, value := range values {
		ret[idx] = runtimecontract.RuntimeFieldViolation{
			Path: value.Path, Actual: value.Actual, Allowed: append([]string(nil), value.Allowed...),
			Required: append([]string(nil), value.Required...), Forbidden: append([]string(nil), value.Forbidden...),
		}
	}
	return ret
}
