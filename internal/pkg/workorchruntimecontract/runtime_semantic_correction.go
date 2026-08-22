package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const RuntimeSemanticCorrectionDirectiveSchemaCurrent = "workorch.runtime_semantic_correction_directive"

type RuntimeSemanticContractViolationKind string

const (
	RuntimeSemanticContractViolationDependencyOrder       RuntimeSemanticContractViolationKind = "dependency_order"
	RuntimeSemanticContractViolationCoverageOrderDocument RuntimeSemanticContractViolationKind = "coverage_order_document"
	RuntimeSemanticContractViolationProofBatchDocument    RuntimeSemanticContractViolationKind = "proof_batch_document"
	RuntimeSemanticContractViolationProposalConstraint    RuntimeSemanticContractViolationKind = "proposal_constraint"
)

type RuntimeSemanticConstraintRule string

const (
	RuntimeSemanticConstraintRequired     RuntimeSemanticConstraintRule = "required"
	RuntimeSemanticConstraintNonEmpty     RuntimeSemanticConstraintRule = "non_empty"
	RuntimeSemanticConstraintAllowedValue RuntimeSemanticConstraintRule = "allowed_value"
	RuntimeSemanticConstraintRelational   RuntimeSemanticConstraintRule = "relational"
)

// RuntimeSemanticContractViolation is a factual, provider-safe semantic
// contract observation. It does not choose correction, retry, or terminal.
type RuntimeSemanticContractViolation struct {
	Kind                        RuntimeSemanticContractViolationKind `json:"kind"`
	Path                        string                               `json:"path"`
	RequiredBeforeID            common.CoverageWorkItemID            `json:"requiredBeforeId"`
	RequiredAfterID             common.CoverageWorkItemID            `json:"requiredAfterId"`
	ActualBeforePosition        int                                  `json:"actualBeforePosition"`
	ActualAfterPosition         int                                  `json:"actualAfterPosition"`
	RequiredCoverageWorkItemIDs []common.CoverageWorkItemID          `json:"requiredCoverageWorkItemIds,omitempty"`
	CoverageWorkItemID          common.CoverageWorkItemID            `json:"coverageWorkItemId,omitempty"`
	ProposalIndex               int                                  `json:"proposalIndex"`
	Rule                        RuntimeSemanticConstraintRule        `json:"rule,omitempty"`
	Actual                      string                               `json:"actual,omitempty"`
	Required                    []string                             `json:"required,omitempty"`
	Allowed                     []string                             `json:"allowed,omitempty"`
}

// RuntimeSemanticProposalViolationError preserves one exact proposal failure
// across the pure document validator and strict payload decoder.
type RuntimeSemanticProposalViolationError struct {
	ViolationFact RuntimeSemanticContractViolation
	Cause         error
}

func (e *RuntimeSemanticProposalViolationError) Error() string {
	if e == nil || e.Cause == nil {
		return "runtime semantic proposal violates its contract"
	}
	return e.Cause.Error()
}

func (e *RuntimeSemanticProposalViolationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *RuntimeSemanticProposalViolationError) Violation() RuntimeSemanticContractViolation {
	if e == nil {
		return RuntimeSemanticContractViolation{}
	}
	return e.ViolationFact.Normalized()
}

func (v RuntimeSemanticContractViolation) Normalized() RuntimeSemanticContractViolation {
	v.Kind = RuntimeSemanticContractViolationKind(strings.TrimSpace(string(v.Kind)))
	v.Path = strings.TrimSpace(v.Path)
	v.RequiredBeforeID = common.CoverageWorkItemID(strings.TrimSpace(string(v.RequiredBeforeID)))
	v.RequiredAfterID = common.CoverageWorkItemID(strings.TrimSpace(string(v.RequiredAfterID)))
	v.RequiredCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(v.RequiredCoverageWorkItemIDs)
	v.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(v.CoverageWorkItemID)))
	v.Rule = RuntimeSemanticConstraintRule(strings.TrimSpace(string(v.Rule)))
	v.Actual = strings.TrimSpace(v.Actual)
	v.Required = normalizeStrings(v.Required)
	v.Allowed = normalizeStrings(v.Allowed)
	return v
}

func (v RuntimeSemanticContractViolation) Validate() error {
	v = v.Normalized()
	switch v.Kind {
	case RuntimeSemanticContractViolationDependencyOrder:
		if v.Path != "$.coverageOrder" || len(v.RequiredCoverageWorkItemIDs) != 0 {
			return fmt.Errorf("runtime semantic dependency violation requires the canonical order path")
		}
		if v.RequiredBeforeID == "" || v.RequiredAfterID == "" || v.RequiredBeforeID == v.RequiredAfterID {
			return fmt.Errorf("runtime semantic dependency violation requires distinct coverage ids")
		}
		if v.ActualBeforePosition < 0 || v.ActualAfterPosition < 0 || v.ActualBeforePosition < v.ActualAfterPosition {
			return fmt.Errorf("runtime semantic dependency violation positions do not describe an order violation")
		}
	case RuntimeSemanticContractViolationCoverageOrderDocument:
		if !strings.HasPrefix(v.Path, "$") || len(v.RequiredCoverageWorkItemIDs) == 0 ||
			v.RequiredBeforeID != "" || v.RequiredAfterID != "" || v.CoverageWorkItemID != "" ||
			v.Rule != "" || len(v.Required) != 0 || len(v.Allowed) != 0 {
			return fmt.Errorf("runtime semantic coverage-order document violation requires a field path and exact manifest coverage")
		}
	case RuntimeSemanticContractViolationProofBatchDocument:
		if !strings.HasPrefix(v.Path, "$") || v.RequiredBeforeID != "" || v.RequiredAfterID != "" || len(v.RequiredCoverageWorkItemIDs) == 0 {
			return fmt.Errorf("runtime semantic proof-batch violation requires a field path and exact batch coverage")
		}
	case RuntimeSemanticContractViolationProposalConstraint:
		if !strings.HasPrefix(v.Path, "$.proposals[") || v.CoverageWorkItemID == "" || v.ProposalIndex < 0 ||
			v.RequiredBeforeID != "" || v.RequiredAfterID != "" || len(v.RequiredCoverageWorkItemIDs) != 0 {
			return fmt.Errorf("runtime semantic proposal violation requires proposal identity and field path")
		}
		switch v.Rule {
		case RuntimeSemanticConstraintRequired, RuntimeSemanticConstraintNonEmpty, RuntimeSemanticConstraintAllowedValue, RuntimeSemanticConstraintRelational:
		default:
			return fmt.Errorf("runtime semantic proposal violation requires a supported rule")
		}
		if (v.Rule == RuntimeSemanticConstraintRequired || v.Rule == RuntimeSemanticConstraintNonEmpty || v.Rule == RuntimeSemanticConstraintRelational) && len(v.Required) == 0 {
			return fmt.Errorf("runtime semantic proposal violation requires exact required values")
		}
		// An empty allowed set is meaningful: the sealed owner catalog permits
		// no value for this field. It remains a factual, provider-safe
		// correction, not an internal contract defect.
	default:
		return fmt.Errorf("runtime semantic violation requires a supported kind")
	}
	return nil
}

func (v RuntimeSemanticContractViolation) ValidateAgainstManifest(manifest RuntimeSemanticPlanManifest) error {
	if err := v.Validate(); err != nil {
		return err
	}
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if v.Kind == RuntimeSemanticContractViolationCoverageOrderDocument || v.Kind == RuntimeSemanticContractViolationProofBatchDocument || v.Kind == RuntimeSemanticContractViolationProposalConstraint {
		known := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
		for _, item := range manifest.CoverageItems {
			known[item.ID] = struct{}{}
		}
		ids := v.RequiredCoverageWorkItemIDs
		if v.Kind == RuntimeSemanticContractViolationProposalConstraint {
			ids = []common.CoverageWorkItemID{v.CoverageWorkItemID}
		}
		for _, id := range ids {
			if _, found := known[id]; !found {
				return fmt.Errorf("runtime semantic document violation coverage %q is outside manifest", id)
			}
		}
		if v.Kind == RuntimeSemanticContractViolationCoverageOrderDocument && len(ids) != len(known) {
			return fmt.Errorf("runtime semantic coverage-order violation does not preserve exact manifest coverage")
		}
		return nil
	}
	for _, item := range manifest.CoverageItems {
		if item.ID == v.RequiredAfterID {
			for _, dependencyID := range item.PrerequisiteCoverageWorkItemIDs {
				if dependencyID == v.RequiredBeforeID {
					return nil
				}
			}
			break
		}
	}
	return fmt.Errorf("runtime semantic dependency violation is not sealed by the manifest")
}

func cloneRuntimeSemanticContractViolations(values []RuntimeSemanticContractViolation) []RuntimeSemanticContractViolation {
	if len(values) == 0 {
		return nil
	}
	ret := append([]RuntimeSemanticContractViolation(nil), values...)
	for idx := range ret {
		ret[idx].RequiredCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), values[idx].RequiredCoverageWorkItemIDs...)
		ret[idx].Required = append([]string(nil), values[idx].Required...)
		ret[idx].Allowed = append([]string(nil), values[idx].Allowed...)
	}
	return ret
}

func normalizeRuntimeSemanticContractViolations(values []RuntimeSemanticContractViolation) []RuntimeSemanticContractViolation {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeSemanticContractViolation, len(values))
	for idx := range values {
		ret[idx] = values[idx].Normalized()
	}
	return ret
}

// RuntimeSemanticCorrectionDirective is the sole persisted, actionable delta
// for semantic attempt 1. SM-3 owns whether this value is created or consumed.
type RuntimeSemanticCorrectionDirective struct {
	SchemaVersion       string                             `json:"schemaVersion"`
	Hash                string                             `json:"hash"`
	PlanStep            RuntimeSemanticPlanStep            `json:"planStep"`
	ManifestHash        string                             `json:"manifestHash"`
	DefinitionSeedHash  string                             `json:"definitionSeedHash"`
	StablePrefixHash    string                             `json:"stablePrefixHash"`
	SourceRejectionHash string                             `json:"sourceRejectionHash"`
	Attempt             int                                `json:"attempt"`
	Violations          []RuntimeSemanticContractViolation `json:"violations"`
}

// DeriveRuntimeSemanticCorrectionDirective is the owner-neutral, deterministic
// constructor used by the Orchestrator SM-3 reducer. It only packages already
// persisted rejection facts; it never chooses whether a retry is allowed.
func DeriveRuntimeSemanticCorrectionDirective(manifest RuntimeSemanticPlanManifest, expected RuntimeExpectedDocumentContract, stablePrefixHash string, rejection RuntimeOriginalDiagnostic) (RuntimeSemanticCorrectionDirective, error) {
	manifest = manifest.Normalized()
	expected = expected.Normalized()
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticCorrectionDirective{}, err
	}
	if err := rejection.Validate(); err != nil {
		return RuntimeSemanticCorrectionDirective{}, err
	}
	if strings.TrimSpace(stablePrefixHash) == "" || expected.DocumentKind != RuntimeProviderDocumentSemanticPlan || expected.ManifestHash != manifest.Hash || expected.DefinitionSeedHash != manifest.DefinitionSeedHash || expected.CorrectionAttempt != 1 {
		return RuntimeSemanticCorrectionDirective{}, fmt.Errorf("runtime semantic correction derivation identity is invalid")
	}
	if len(rejection.SemanticViolations) == 0 {
		return RuntimeSemanticCorrectionDirective{}, fmt.Errorf("runtime semantic correction derivation requires typed violations")
	}
	directive := RuntimeSemanticCorrectionDirective{
		SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      expected.PlanStep, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: strings.TrimSpace(stablePrefixHash), SourceRejectionHash: StableRuntimeSemanticPlanCorrectionHash(rejection), Attempt: 1,
		Violations: cloneRuntimeSemanticContractViolations(rejection.SemanticViolations),
	}
	directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
	if err := directive.ValidateAgainst(manifest, rejection); err != nil {
		return RuntimeSemanticCorrectionDirective{}, err
	}
	return directive, nil
}

func (d RuntimeSemanticCorrectionDirective) Normalized() RuntimeSemanticCorrectionDirective {
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.Hash = strings.TrimSpace(d.Hash)
	d.PlanStep = RuntimeSemanticPlanStep(strings.TrimSpace(string(d.PlanStep)))
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.DefinitionSeedHash = strings.TrimSpace(d.DefinitionSeedHash)
	d.StablePrefixHash = strings.TrimSpace(d.StablePrefixHash)
	d.SourceRejectionHash = strings.TrimSpace(d.SourceRejectionHash)
	d.Violations = normalizeRuntimeSemanticContractViolations(d.Violations)
	return d
}

func (d RuntimeSemanticCorrectionDirective) Clone() RuntimeSemanticCorrectionDirective {
	d = d.Normalized()
	d.Violations = cloneRuntimeSemanticContractViolations(d.Violations)
	return d
}

func (d RuntimeSemanticCorrectionDirective) Validate() error {
	d = d.Normalized()
	if d.SchemaVersion != RuntimeSemanticCorrectionDirectiveSchemaCurrent || d.Hash == "" ||
		d.PlanStep == "" || d.ManifestHash == "" || d.DefinitionSeedHash == "" ||
		d.StablePrefixHash == "" || d.SourceRejectionHash == "" || d.Attempt != 1 ||
		len(d.Violations) == 0 || len(d.Violations) > 32 {
		return fmt.Errorf("runtime semantic correction directive requires current bounded attempt-1 identity")
	}
	for idx := range d.Violations {
		if err := d.Violations[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic correction violation %d: %w", idx, err)
		}
	}
	if d.Hash != StableRuntimeSemanticCorrectionDirectiveHash(d) {
		return fmt.Errorf("runtime semantic correction directive hash does not match contents")
	}
	return nil
}

func (d RuntimeSemanticCorrectionDirective) ValidateAgainst(manifest RuntimeSemanticPlanManifest, rejection RuntimeOriginalDiagnostic) error {
	if err := d.Validate(); err != nil {
		return err
	}
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if d.ManifestHash != manifest.Hash || d.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return fmt.Errorf("runtime semantic correction directive manifest identity is stale")
	}
	if d.SourceRejectionHash != StableRuntimeSemanticPlanCorrectionHash(rejection) {
		return fmt.Errorf("runtime semantic correction directive rejection identity is stale")
	}
	canonicalRejection := rejection.Normalized()
	if canonicalRejection == nil || len(d.Violations) != len(canonicalRejection.SemanticViolations) {
		return fmt.Errorf("runtime semantic correction directive does not preserve rejection violations")
	}
	for idx := range d.Violations {
		switch d.Violations[idx].Kind {
		case RuntimeSemanticContractViolationDependencyOrder:
			if d.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
				return fmt.Errorf("runtime semantic dependency correction is outside coverage-order step")
			}
		case RuntimeSemanticContractViolationCoverageOrderDocument:
			if d.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
				return fmt.Errorf("runtime semantic coverage-order document correction is outside coverage-order step")
			}
		case RuntimeSemanticContractViolationProofBatchDocument:
			if d.PlanStep != RuntimeSemanticPlanStepProofBatch {
				return fmt.Errorf("runtime semantic proof-batch correction is outside proof-batch step")
			}
		case RuntimeSemanticContractViolationProposalConstraint:
			if d.PlanStep != RuntimeSemanticPlanStepProofBatch {
				return fmt.Errorf("runtime semantic proposal correction is outside proof-batch step")
			}
		}
		if !runtimeSemanticContractViolationEqual(d.Violations[idx], canonicalRejection.SemanticViolations[idx]) {
			return fmt.Errorf("runtime semantic correction directive violation %d differs from rejection", idx)
		}
		if err := d.Violations[idx].ValidateAgainstManifest(manifest); err != nil {
			return err
		}
	}
	return nil
}

func StableRuntimeSemanticCorrectionDirectiveHash(directive RuntimeSemanticCorrectionDirective) string {
	directive = directive.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeSemanticCorrectionDirectiveSchemaCurrent)
	writeHashString(h, string(directive.PlanStep))
	writeHashString(h, directive.ManifestHash)
	writeHashString(h, directive.DefinitionSeedHash)
	writeHashString(h, directive.StablePrefixHash)
	writeHashString(h, directive.SourceRejectionHash)
	writeHashString(h, fmt.Sprintf("%d", directive.Attempt))
	for _, violation := range directive.Violations {
		writeHashString(h, string(violation.Kind))
		writeHashString(h, violation.Path)
		writeHashString(h, string(violation.RequiredBeforeID))
		writeHashString(h, string(violation.RequiredAfterID))
		writeHashString(h, fmt.Sprintf("%d", violation.ActualBeforePosition))
		writeHashString(h, fmt.Sprintf("%d", violation.ActualAfterPosition))
		for _, id := range violation.RequiredCoverageWorkItemIDs {
			writeHashString(h, string(id))
		}
		writeHashString(h, string(violation.CoverageWorkItemID))
		writeHashString(h, fmt.Sprintf("%d", violation.ProposalIndex))
		writeHashString(h, string(violation.Rule))
		writeHashString(h, violation.Actual)
		writeHashStrings(h, violation.Required)
		writeHashStrings(h, violation.Allowed)
	}
	return "runtime-semantic-correction-directive-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func runtimeSemanticContractViolationEqual(left, right RuntimeSemanticContractViolation) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.Kind != right.Kind || left.Path != right.Path || left.RequiredBeforeID != right.RequiredBeforeID ||
		left.RequiredAfterID != right.RequiredAfterID || left.ActualBeforePosition != right.ActualBeforePosition ||
		left.ActualAfterPosition != right.ActualAfterPosition || left.CoverageWorkItemID != right.CoverageWorkItemID ||
		left.ProposalIndex != right.ProposalIndex || left.Rule != right.Rule || left.Actual != right.Actual ||
		len(left.RequiredCoverageWorkItemIDs) != len(right.RequiredCoverageWorkItemIDs) || !sameStrings(left.Required, right.Required) || !sameStrings(left.Allowed, right.Allowed) {
		return false
	}
	for idx := range left.RequiredCoverageWorkItemIDs {
		if left.RequiredCoverageWorkItemIDs[idx] != right.RequiredCoverageWorkItemIDs[idx] {
			return false
		}
	}
	return true
}

// RuntimeSemanticDependencyOrderError preserves the typed dependency edge
// rejected by the pure semantic document validator.
type RuntimeSemanticDependencyOrderError struct {
	RequiredBeforeID     common.CoverageWorkItemID
	RequiredAfterID      common.CoverageWorkItemID
	ActualBeforePosition int
	ActualAfterPosition  int
}

func (e *RuntimeSemanticDependencyOrderError) Error() string {
	if e == nil {
		return "runtime semantic document coverage order violates dependency"
	}
	return fmt.Sprintf("runtime semantic document coverage order violates dependency %q -> %q", e.RequiredBeforeID, e.RequiredAfterID)
}

func (e *RuntimeSemanticDependencyOrderError) Violation() RuntimeSemanticContractViolation {
	if e == nil {
		return RuntimeSemanticContractViolation{}
	}
	return RuntimeSemanticContractViolation{
		Kind: RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder",
		RequiredBeforeID: e.RequiredBeforeID, RequiredAfterID: e.RequiredAfterID,
		ActualBeforePosition: e.ActualBeforePosition, ActualAfterPosition: e.ActualAfterPosition,
	}
}
