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
	RuntimeSemanticContractViolationDependencyOrder    RuntimeSemanticContractViolationKind = "dependency_order"
	RuntimeSemanticContractViolationProofBatchDocument RuntimeSemanticContractViolationKind = "proof_batch_document"
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
}

func (v RuntimeSemanticContractViolation) Normalized() RuntimeSemanticContractViolation {
	v.Kind = RuntimeSemanticContractViolationKind(strings.TrimSpace(string(v.Kind)))
	v.Path = strings.TrimSpace(v.Path)
	v.RequiredBeforeID = common.CoverageWorkItemID(strings.TrimSpace(string(v.RequiredBeforeID)))
	v.RequiredAfterID = common.CoverageWorkItemID(strings.TrimSpace(string(v.RequiredAfterID)))
	v.RequiredCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(v.RequiredCoverageWorkItemIDs)
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
	case RuntimeSemanticContractViolationProofBatchDocument:
		if !strings.HasPrefix(v.Path, "$") || v.RequiredBeforeID != "" || v.RequiredAfterID != "" || len(v.RequiredCoverageWorkItemIDs) == 0 {
			return fmt.Errorf("runtime semantic proof-batch violation requires a field path and exact batch coverage")
		}
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
	if v.Kind == RuntimeSemanticContractViolationProofBatchDocument {
		known := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
		for _, item := range manifest.CoverageItems {
			known[item.ID] = struct{}{}
		}
		for _, id := range v.RequiredCoverageWorkItemIDs {
			if _, found := known[id]; !found {
				return fmt.Errorf("runtime semantic proof-batch violation coverage %q is outside manifest", id)
			}
		}
		return nil
	}
	for _, item := range manifest.CoverageItems {
		if item.ID == v.RequiredAfterID {
			for _, dependency := range item.ReopenDependencies {
				if dependency.CoverageWorkItemID == v.RequiredBeforeID {
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
		case RuntimeSemanticContractViolationProofBatchDocument:
			if d.PlanStep != RuntimeSemanticPlanStepProofBatch {
				return fmt.Errorf("runtime semantic proof-batch correction is outside proof-batch step")
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
	}
	return "runtime-semantic-correction-directive-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func runtimeSemanticContractViolationEqual(left, right RuntimeSemanticContractViolation) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.Kind != right.Kind || left.Path != right.Path || left.RequiredBeforeID != right.RequiredBeforeID ||
		left.RequiredAfterID != right.RequiredAfterID || left.ActualBeforePosition != right.ActualBeforePosition ||
		left.ActualAfterPosition != right.ActualAfterPosition || len(left.RequiredCoverageWorkItemIDs) != len(right.RequiredCoverageWorkItemIDs) {
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
