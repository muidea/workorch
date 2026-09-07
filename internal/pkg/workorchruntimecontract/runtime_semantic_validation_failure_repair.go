package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationFailureRepairGenerationInput binds one repair semantic
// generation to the exact rejected validation fact. It is a pure compiler
// input, not a state transition or retry decision.
type RuntimeValidationFailureRepairGenerationInput struct {
	InputAddition              RuntimeValidationInputAddition `json:",omitzero"`
	CoverageWorkItemID         common.CoverageWorkItemID
	FailedFocusID              string
	FailureSignature           string
	TriggerOwnerModuleID       string
	TriggerOperationKind       string
	TriggerDiagnosticClass     string
	TriggerEvidenceTargetType  string
	TriggerOwnerDiagnosticCode string
}

func (i RuntimeValidationFailureRepairGenerationInput) Normalized() RuntimeValidationFailureRepairGenerationInput {
	i.InputAddition = i.InputAddition.Normalized()
	i.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(i.CoverageWorkItemID)))
	i.FailedFocusID = strings.TrimSpace(i.FailedFocusID)
	i.FailureSignature = strings.TrimSpace(i.FailureSignature)
	i.TriggerOwnerModuleID = strings.TrimSpace(i.TriggerOwnerModuleID)
	i.TriggerOperationKind = CanonicalRuntimeBlockKind(i.TriggerOperationKind)
	i.TriggerDiagnosticClass = strings.TrimSpace(i.TriggerDiagnosticClass)
	i.TriggerEvidenceTargetType = strings.TrimSpace(i.TriggerEvidenceTargetType)
	i.TriggerOwnerDiagnosticCode = strings.TrimSpace(i.TriggerOwnerDiagnosticCode)
	return i
}

func (i RuntimeValidationFailureRepairGenerationInput) Validate() error {
	i = i.Normalized()
	if i.CoverageWorkItemID == "" || i.FailedFocusID == "" || i.FailureSignature == "" ||
		i.TriggerOwnerModuleID == "" || i.TriggerOperationKind == "" || i.TriggerDiagnosticClass == "" ||
		i.TriggerEvidenceTargetType == "" || i.TriggerOwnerDiagnosticCode == "" {
		return fmt.Errorf("validation failure repair generation requires coverage, failed focus, failure signature and exact typed trigger")
	}
	if i.InputAddition != (RuntimeValidationInputAddition{}) {
		if err := i.InputAddition.Validate(); err != nil {
			return err
		}
	}
	if !runtimeValidationDiagnosticClassKnown(i.TriggerDiagnosticClass) {
		return fmt.Errorf("validation failure repair generation has unknown diagnostic class %q", i.TriggerDiagnosticClass)
	}
	if !runtimeValidationEvidenceTargetTypeKnown(i.TriggerEvidenceTargetType) {
		return fmt.Errorf("validation failure repair generation has unknown evidence target type %q", i.TriggerEvidenceTargetType)
	}
	return nil
}

// RuntimeSemanticValidationFailureRepair is the generation-specific repair
// proof authority sealed into one RuntimeSemanticProofObligation. It copies
// every Profile declaration fact needed for replay, plus the actual failure
// identity and the exact repair-owner catalog snapshot used by SM-3 planning.
// It never carries an exact prospective target; that authority is produced by
// a provider proposal, confirmed by repair-owner feasibility, and sealed only in the
// resulting CoverageEvidenceBinding.
type RuntimeSemanticValidationFailureRepair struct {
	RequireInputAddition       bool                           `json:",omitempty"`
	InputAddition              RuntimeValidationInputAddition `json:",omitzero"`
	FailedFocusID              string
	ConvergenceID              string
	ConvergenceHash            string
	RouteID                    string
	RouteHash                  string
	FailureSignature           string
	TriggerOwnerModuleID       string
	TriggerOperationKind       string
	TriggerDiagnosticClass     string
	TriggerEvidenceTargetType  string
	TriggerOwnerDiagnosticCode string

	RepairEvidenceKind                           string
	RepairOwnerModuleID                          string
	RepairOperationKinds                         []string
	RepairTargetPolicy                           RuntimeOperationTargetPolicy
	RepairTargetRole                             string
	RepairDeclarationTargetSelectorRef           string
	RepairDeclarationTargetSelectorOwnerRef      string
	RepairDeclarationTargetSelectorSubjectRef    string
	RepairDeclarationTargetSelectorCandidateRefs []string
	RepairOperationTargetSelectorRef             string
	RepairConstraintRefs                         []string
	OwnerCatalogSnapshotID                       string
	OwnerCatalogObservedRevision                 string
	RequireFreshValidation                       bool
	MaxAttempts                                  int
}

func (r RuntimeSemanticValidationFailureRepair) Normalized() RuntimeSemanticValidationFailureRepair {
	r.InputAddition = r.InputAddition.Normalized()
	r.FailedFocusID = strings.TrimSpace(r.FailedFocusID)
	r.ConvergenceID = strings.TrimSpace(r.ConvergenceID)
	r.ConvergenceHash = strings.TrimSpace(r.ConvergenceHash)
	r.RouteID = strings.TrimSpace(r.RouteID)
	r.RouteHash = strings.TrimSpace(r.RouteHash)
	r.FailureSignature = strings.TrimSpace(r.FailureSignature)
	r.TriggerOwnerModuleID = strings.TrimSpace(r.TriggerOwnerModuleID)
	r.TriggerOperationKind = CanonicalRuntimeBlockKind(r.TriggerOperationKind)
	r.TriggerDiagnosticClass = strings.TrimSpace(r.TriggerDiagnosticClass)
	r.TriggerEvidenceTargetType = strings.TrimSpace(r.TriggerEvidenceTargetType)
	r.TriggerOwnerDiagnosticCode = strings.TrimSpace(r.TriggerOwnerDiagnosticCode)
	r.RepairEvidenceKind = strings.TrimSpace(r.RepairEvidenceKind)
	r.RepairOwnerModuleID = strings.TrimSpace(r.RepairOwnerModuleID)
	r.RepairOperationKinds = canonicalizeOperationKinds(r.RepairOperationKinds)
	r.RepairTargetPolicy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(r.RepairTargetPolicy)))
	r.RepairTargetRole = strings.TrimSpace(r.RepairTargetRole)
	r.RepairDeclarationTargetSelectorRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorRef)
	r.RepairDeclarationTargetSelectorOwnerRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorOwnerRef)
	r.RepairDeclarationTargetSelectorSubjectRef = strings.TrimSpace(r.RepairDeclarationTargetSelectorSubjectRef)
	r.RepairDeclarationTargetSelectorCandidateRefs = normalizeStrings(r.RepairDeclarationTargetSelectorCandidateRefs)
	r.RepairOperationTargetSelectorRef = strings.TrimSpace(r.RepairOperationTargetSelectorRef)
	r.RepairConstraintRefs = normalizeStrings(r.RepairConstraintRefs)
	r.OwnerCatalogSnapshotID = strings.TrimSpace(r.OwnerCatalogSnapshotID)
	r.OwnerCatalogObservedRevision = strings.TrimSpace(r.OwnerCatalogObservedRevision)
	return r
}

func (r RuntimeSemanticValidationFailureRepair) Clone() RuntimeSemanticValidationFailureRepair {
	r = r.Normalized()
	r.RepairOperationKinds = append([]string(nil), r.RepairOperationKinds...)
	r.RepairDeclarationTargetSelectorCandidateRefs = append([]string(nil), r.RepairDeclarationTargetSelectorCandidateRefs...)
	r.RepairConstraintRefs = append([]string(nil), r.RepairConstraintRefs...)
	return r
}

func (r RuntimeSemanticValidationFailureRepair) convergence() RuntimeValidationFailureRepairConvergence {
	r = r.Normalized()
	return RuntimeValidationFailureRepairConvergence{
		ID: r.ConvergenceID,
		Routes: []RuntimeValidationFailureRepairRoute{{
			ID:                                           r.RouteID,
			RequireInputAddition:                         r.RequireInputAddition,
			TriggerOwnerModuleID:                         r.TriggerOwnerModuleID,
			TriggerOperationKind:                         r.TriggerOperationKind,
			TriggerDiagnosticClass:                       r.TriggerDiagnosticClass,
			TriggerEvidenceTargetType:                    r.TriggerEvidenceTargetType,
			TriggerOwnerDiagnosticCode:                   r.TriggerOwnerDiagnosticCode,
			RepairEvidenceKind:                           r.RepairEvidenceKind,
			RepairOwnerModuleID:                          r.RepairOwnerModuleID,
			RepairOperationKinds:                         append([]string(nil), r.RepairOperationKinds...),
			RepairTargetPolicy:                           r.RepairTargetPolicy,
			RepairTargetRole:                             r.RepairTargetRole,
			RepairDeclarationTargetSelectorRef:           r.RepairDeclarationTargetSelectorRef,
			RepairDeclarationTargetSelectorOwnerRef:      r.RepairDeclarationTargetSelectorOwnerRef,
			RepairDeclarationTargetSelectorSubjectRef:    r.RepairDeclarationTargetSelectorSubjectRef,
			RepairDeclarationTargetSelectorCandidateRefs: append([]string(nil), r.RepairDeclarationTargetSelectorCandidateRefs...),
			RepairOperationTargetSelectorRef:             r.RepairOperationTargetSelectorRef,
		}},
		RequireFreshValidation: r.RequireFreshValidation,
		MaxAttempts:            r.MaxAttempts,
	}.Normalized()
}

func (r RuntimeSemanticValidationFailureRepair) route() RuntimeValidationFailureRepairRoute {
	convergence := r.convergence()
	if len(convergence.Routes) != 1 {
		return RuntimeValidationFailureRepairRoute{}
	}
	return convergence.Routes[0].Clone()
}

func (r RuntimeSemanticValidationFailureRepair) Validate() error {
	r = r.Normalized()
	if r.FailedFocusID == "" || r.ConvergenceID == "" || r.ConvergenceHash == "" || r.RouteID == "" || r.RouteHash == "" ||
		r.FailureSignature == "" || r.TriggerDiagnosticClass == "" ||
		r.OwnerCatalogSnapshotID == "" || r.OwnerCatalogObservedRevision == "" {
		return fmt.Errorf("runtime semantic validation repair requires failed focus, failure signature, diagnostic and owner catalog identity")
	}
	convergence := r.convergence()
	if err := convergence.Validate(); err != nil {
		return err
	}
	route := r.route()
	if r.RequireInputAddition {
		if err := r.InputAddition.Validate(); err != nil {
			return err
		}
	} else if r.InputAddition != (RuntimeValidationInputAddition{}) {
		return fmt.Errorf("undeclared validation input addition")
	}
	if r.RouteHash != StableRuntimeValidationFailureRepairRouteHash(route) {
		return fmt.Errorf("runtime semantic validation repair route hash does not match declaration")
	}
	if !route.MatchesTrigger(r.TriggerOwnerModuleID, r.TriggerOperationKind, r.TriggerDiagnosticClass, r.TriggerEvidenceTargetType, r.TriggerOwnerDiagnosticCode) {
		return fmt.Errorf("runtime semantic validation repair trigger does not match convergence")
	}
	if r.RepairTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(r.RepairConstraintRefs) == 0 {
		return fmt.Errorf("runtime semantic prospective validation repair requires owner constraints")
	}
	return nil
}

func StableRuntimeSemanticValidationFailureRepairHash(value RuntimeSemanticValidationFailureRepair) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-semantic-validation-failure-repair")
	writeRuntimeValidationInputAdditionHash(h, value.InputAddition)
	if value.RequireInputAddition {
		writeHashString(h, "require-validation-input-addition/v1")
	}
	writeHashString(h, value.FailedFocusID)
	writeHashString(h, value.ConvergenceID)
	writeHashString(h, value.ConvergenceHash)
	writeHashString(h, value.RouteID)
	writeHashString(h, value.RouteHash)
	writeHashString(h, value.FailureSignature)
	writeHashString(h, value.TriggerOwnerModuleID)
	writeHashString(h, value.TriggerOperationKind)
	writeHashString(h, value.TriggerDiagnosticClass)
	writeHashString(h, value.TriggerEvidenceTargetType)
	writeHashString(h, value.TriggerOwnerDiagnosticCode)
	writeHashString(h, value.RepairEvidenceKind)
	writeHashString(h, value.RepairOwnerModuleID)
	writeHashStrings(h, value.RepairOperationKinds)
	writeHashString(h, string(value.RepairTargetPolicy))
	writeHashString(h, value.RepairTargetRole)
	writeHashString(h, value.RepairDeclarationTargetSelectorRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorOwnerRef)
	writeHashString(h, value.RepairDeclarationTargetSelectorSubjectRef)
	writeHashStrings(h, value.RepairDeclarationTargetSelectorCandidateRefs)
	writeHashString(h, value.RepairOperationTargetSelectorRef)
	writeHashStrings(h, value.RepairConstraintRefs)
	writeHashString(h, value.OwnerCatalogSnapshotID)
	writeHashString(h, value.OwnerCatalogObservedRevision)
	writeHashString(h, strconv.FormatBool(value.RequireFreshValidation))
	writeHashString(h, strconv.Itoa(value.MaxAttempts))
	return "runtime-semantic-validation-repair-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func sameRuntimeSemanticValidationFailureRepair(left, right *RuntimeSemanticValidationFailureRepair) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return StableRuntimeSemanticValidationFailureRepairHash(*left) == StableRuntimeSemanticValidationFailureRepairHash(*right)
}

func cloneRuntimeSemanticValidationFailureRepair(value *RuntimeSemanticValidationFailureRepair) *RuntimeSemanticValidationFailureRepair {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func compileRuntimeSemanticValidationFailureRepairObligation(
	base RuntimeSemanticProofObligation,
	source RuntimeDefinitionFocus,
	item RuntimeCoverageItem,
	catalogs []RuntimeOperationAdmissibility,
	input RuntimeValidationFailureRepairGenerationInput,
) (RuntimeSemanticProofObligation, RuntimeOperationAdmissibility, error) {
	base = base.Normalized()
	source = source.Normalized()
	item = item.Normalized()
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	if input.CoverageWorkItemID != item.ID || base.CoverageWorkItemID != item.ID || !containsCoverageWorkItemID(source.CoverageWorkItemIDs, item.ID) {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, fmt.Errorf("validation failure repair generation coverage does not match Definition focus")
	}
	if input.FailedFocusID != source.ID {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, fmt.Errorf("validation failure repair generation failed focus %q does not match Definition focus %q", input.FailedFocusID, source.ID)
	}
	if source.ValidationFailureRepairConvergence == nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, fmt.Errorf("coverage %q has no Profile validation failure repair convergence", item.ID)
	}
	convergence := source.ValidationFailureRepairConvergence.Clone()
	if err := convergence.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	route, found := convergence.RouteForTrigger(input.TriggerOwnerModuleID, input.TriggerOperationKind, input.TriggerDiagnosticClass, input.TriggerEvidenceTargetType, input.TriggerOwnerDiagnosticCode)
	if !found {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, fmt.Errorf("validation failure repair trigger does not match coverage %q Profile convergence", item.ID)
	}
	var catalog RuntimeOperationAdmissibility
	for _, raw := range NormalizeRuntimeOperationAdmissibilities(catalogs) {
		if raw.OwnerModuleID == route.RepairOwnerModuleID {
			catalog = raw
			break
		}
	}
	if catalog.OwnerModuleID == "" {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, fmt.Errorf("validation failure repair owner %q has no sealed catalog", route.RepairOwnerModuleID)
	}
	if err := catalog.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	operationKinds, constraintRefs, err := runtimeSemanticValidationRepairCatalogSurface(route, catalog)
	if err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	repair := RuntimeSemanticValidationFailureRepair{
		FailedFocusID:                                input.FailedFocusID,
		RequireInputAddition:                         route.RequireInputAddition,
		InputAddition:                                input.InputAddition,
		ConvergenceID:                                convergence.ID,
		ConvergenceHash:                              StableRuntimeValidationFailureRepairConvergenceHash(convergence),
		RouteID:                                      route.ID,
		RouteHash:                                    StableRuntimeValidationFailureRepairRouteHash(route),
		FailureSignature:                             input.FailureSignature,
		TriggerOwnerModuleID:                         route.TriggerOwnerModuleID,
		TriggerOperationKind:                         route.TriggerOperationKind,
		TriggerDiagnosticClass:                       input.TriggerDiagnosticClass,
		TriggerEvidenceTargetType:                    input.TriggerEvidenceTargetType,
		TriggerOwnerDiagnosticCode:                   input.TriggerOwnerDiagnosticCode,
		RepairEvidenceKind:                           route.RepairEvidenceKind,
		RepairOwnerModuleID:                          route.RepairOwnerModuleID,
		RepairOperationKinds:                         operationKinds,
		RepairTargetPolicy:                           route.RepairTargetPolicy,
		RepairTargetRole:                             route.RepairTargetRole,
		RepairDeclarationTargetSelectorRef:           route.RepairDeclarationTargetSelectorRef,
		RepairDeclarationTargetSelectorOwnerRef:      route.RepairDeclarationTargetSelectorOwnerRef,
		RepairDeclarationTargetSelectorSubjectRef:    route.RepairDeclarationTargetSelectorSubjectRef,
		RepairDeclarationTargetSelectorCandidateRefs: append([]string(nil), route.RepairDeclarationTargetSelectorCandidateRefs...),
		RepairOperationTargetSelectorRef:             route.RepairOperationTargetSelectorRef,
		RepairConstraintRefs:                         constraintRefs,
		OwnerCatalogSnapshotID:                       catalog.SnapshotID,
		OwnerCatalogObservedRevision:                 catalog.ObservedRevision,
		RequireFreshValidation:                       convergence.RequireFreshValidation,
		MaxAttempts:                                  convergence.MaxAttempts,
	}.Normalized()
	if err := repair.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	base.OwnerFactAcquisition = nil
	base.ValidationFailureRepair = &repair
	base = base.Normalized()
	if err := base.Validate(); err != nil {
		return RuntimeSemanticProofObligation{}, RuntimeOperationAdmissibility{}, err
	}
	return base, catalog, nil
}

func runtimeSemanticValidationRepairCatalogSurface(route RuntimeValidationFailureRepairRoute, catalog RuntimeOperationAdmissibility) ([]string, []string, error) {
	route = route.Normalized()
	catalog = catalog.Normalized()
	var expectedConstraints []string
	for _, kind := range route.RepairOperationKinds {
		if !runtimeOperationKindsContain(catalog.OperationKinds, kind) {
			return nil, nil, fmt.Errorf("validation failure repair operation %q is absent from owner catalog", kind)
		}
		matched := false
		contract, hasContract := RuntimeOperationContractForKind(kind)
		if hasContract && len(contract.Variants) != 0 {
			for _, raw := range catalog.OperationVariants {
				variant := raw.Normalized()
				if variant.Kind != kind || !runtimeOperationAdmissibilityVariantAllowsTargetPolicy(variant, route.RepairTargetPolicy) ||
					!containsString(variant.SelectorRefs, route.RepairOperationTargetSelectorRef) {
					continue
				}
				if matched && !sameStrings(expectedConstraints, variant.ConstraintRefs) {
					return nil, nil, fmt.Errorf("validation failure repair operation %q has incompatible owner constraint surfaces", kind)
				}
				if expectedConstraints == nil {
					expectedConstraints = append([]string(nil), variant.ConstraintRefs...)
				} else if !sameStrings(expectedConstraints, variant.ConstraintRefs) {
					return nil, nil, fmt.Errorf("validation failure repair operations do not share one owner constraint surface")
				}
				matched = true
			}
		} else {
			allowed, selectors, constraints, _, lookupErr := RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", route.RepairTargetPolicy)
			if lookupErr != nil {
				return nil, nil, lookupErr
			}
			if allowed && containsString(selectors, route.RepairOperationTargetSelectorRef) {
				if expectedConstraints == nil {
					expectedConstraints = append([]string(nil), constraints...)
				} else if !sameStrings(expectedConstraints, constraints) {
					return nil, nil, fmt.Errorf("validation failure repair operations do not share one owner constraint surface")
				}
				matched = true
			}
		}
		if !matched {
			return nil, nil, fmt.Errorf("validation failure repair operation %q lacks matching owner policy and selector facts", kind)
		}
	}
	return append([]string(nil), route.RepairOperationKinds...), normalizeStrings(expectedConstraints), nil
}

func containsCoverageWorkItemID(values []common.CoverageWorkItemID, wanted common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validateRuntimeSemanticValidationFailureRepair(
	proofSet RuntimeSemanticCWIProofSet,
	repair *RuntimeSemanticValidationFailureRepair,
	proposalIndex int,
) error {
	if repair == nil {
		return nil
	}
	sealed := repair.Normalized()
	if len(proofSet.Proposals) != 1 {
		return &RuntimeSemanticProposalViolationError{ViolationFact: RuntimeSemanticContractViolation{
			Kind: RuntimeSemanticContractViolationProposalConstraint,
			Path: "$.proposals", CoverageWorkItemID: proofSet.CoverageWorkItemID, ProposalIndex: proposalIndex,
			Rule: RuntimeSemanticConstraintRelational, Actual: fmt.Sprintf("%d proposals", len(proofSet.Proposals)),
			Required: []string{"exactly one validation-failure repair proof"},
		}, Cause: fmt.Errorf("runtime semantic validation repair for coverage %q requires exactly one proposal", proofSet.CoverageWorkItemID)}
	}
	proposal := proofSet.Proposals[0].Normalized()
	checks := []struct {
		field   string
		actual  string
		allowed []string
		valid   bool
	}{
		{field: "ownerModuleId", actual: proposal.OwnerModuleID, allowed: []string{sealed.RepairOwnerModuleID}, valid: proposal.OwnerModuleID == sealed.RepairOwnerModuleID},
		{field: "operationKind", actual: proposal.OperationKind, allowed: sealed.RepairOperationKinds, valid: runtimeOperationKindsContain(sealed.RepairOperationKinds, proposal.OperationKind)},
		{field: "requiredEvidenceKinds", actual: strings.Join(proposal.RequiredEvidenceKinds, ","), allowed: []string{EvidenceKindStateChange}, valid: len(proposal.RequiredEvidenceKinds) == 1 && proposal.RequiredEvidenceKinds[0] == EvidenceKindStateChange},
		{field: "target.policy", actual: string(proposal.Target.Policy), allowed: []string{string(sealed.RepairTargetPolicy)}, valid: proposal.Target.Policy == sealed.RepairTargetPolicy},
		{field: "target.ownerSelectorRefs", actual: strings.Join(proposal.Target.OwnerSelectorRefs, ","), allowed: []string{sealed.RepairOperationTargetSelectorRef}, valid: len(proposal.Target.OwnerSelectorRefs) == 1 && proposal.Target.OwnerSelectorRefs[0] == sealed.RepairOperationTargetSelectorRef},
		{field: "target.prospectiveConstraintRefs", actual: strings.Join(proposal.Target.ProspectiveConstraintRefs, ","), allowed: sealed.RepairConstraintRefs, valid: sameStrings(proposal.Target.ProspectiveConstraintRefs, sealed.RepairConstraintRefs)},
		{field: "declarationRef", actual: proposal.DeclarationRef, allowed: []string{sealed.ConvergenceID}, valid: proposal.DeclarationRef == sealed.ConvergenceID},
	}
	if sealed.RequireInputAddition {
		checks = append(checks, struct {
			field   string
			actual  string
			allowed []string
			valid   bool
		}{field: "target.prospectiveTargetRefs", actual: strings.Join(proposal.Target.ProspectiveTargetRefs, ","), allowed: []string{sealed.InputAddition.TargetRef}, valid: len(proposal.Target.ProspectiveTargetRefs) == 1 && proposal.Target.ProspectiveTargetRefs[0] == sealed.InputAddition.TargetRef})
	}
	for _, check := range checks {
		if !check.valid {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, check.field, RuntimeSemanticConstraintAllowedValue, check.actual, nil, check.allowed,
				fmt.Errorf("runtime semantic validation repair for coverage %q does not match sealed %s", proofSet.CoverageWorkItemID, check.field))
		}
	}
	if proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
		len(sealed.RepairDeclarationTargetSelectorCandidateRefs) != 0 &&
		!runtimeSemanticStringsWithin(proposal.Target.ProspectiveTargetRefs, sealed.RepairDeclarationTargetSelectorCandidateRefs) {
		return runtimeSemanticProposalViolation(proposal, proposalIndex, "target.prospectiveTargetRefs", RuntimeSemanticConstraintAllowedValue,
			strings.Join(proposal.Target.ProspectiveTargetRefs, ","), nil, sealed.RepairDeclarationTargetSelectorCandidateRefs,
			fmt.Errorf("runtime semantic validation repair prospective targets are outside Profile declaration candidates"))
	}
	return nil
}
