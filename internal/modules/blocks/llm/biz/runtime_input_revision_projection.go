package llm

import (
	"slices"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Provider input contains the current maintenance task, never the complete
// authority catalog for other CWIs. The owner retains the original catalog for
// fresh-load reverse proof. Semantic findings and structural observations stay
// separate; neither is execution/acceptance evidence.
type runtimeInputRevisionProjection struct {
	SourceManifestHash     string `json:",omitempty"`
	SourceLogicalRequestID string `json:",omitempty"`
	SourceDocumentHash     string `json:",omitempty"`
	OwnerModuleID          string
	InputTargetRefs        []string
	Binding                rc.RuntimeValidationProofRequirementBinding
	Recipe                 *rc.RuntimeValidationRecipe                `json:",omitempty"`
	Report                 *rc.RuntimeValidationInputReview           `json:"report,omitempty"`
	Observation            *runtimeInputRevisionObservationProjection `json:"ownerObservation,omitempty"`
}
type runtimeInputRevisionObservationProjection struct {
	ObservationID      string
	CatalogSnapshotID  string
	OriginalDiagnostic *rc.RuntimeOriginalDiagnostic
	Files              []rc.RuntimeValidationInputReviewFile
}

func inputRevisionForProvider(v *rc.RuntimeValidationInputRevision) *runtimeInputRevisionProjection {
	if v == nil {
		return nil
	}
	v = v.Clone()
	p := &runtimeInputRevisionProjection{SourceManifestHash: v.SourceManifestHash, SourceLogicalRequestID: v.SourceLogicalRequestID, SourceDocumentHash: v.SourceDocumentHash, OwnerModuleID: v.OwnerModuleID, InputTargetRefs: slices.Clone(v.InputTargetRefs), Binding: v.Binding.Clone()}
	p.Recipe = v.Recipe
	if o := v.OwnerObservation; o != nil {
		p.Observation = &runtimeInputRevisionObservationProjection{ObservationID: o.Observation.ObservationID, CatalogSnapshotID: o.Catalog.SnapshotID, Files: slices.Clone(v.InputFiles())}
		for _, s := range o.Catalog.ValidationSurfaces {
			for _, u := range s.ProofRequirementUnavailabilities {
				if u.Requirement.Hash == v.Binding.RequirementHash {
					p.Observation.OriginalDiagnostic = u.OriginalDiagnostic.CloneOriginal()
				}
			}
		}
	} else {
		r := v.Report.Clone()
		p.Report = &r
	}
	return p
}
func (p *runtimeInputRevisionProjection) inputFiles() []rc.RuntimeValidationInputReviewFile {
	if p == nil {
		return nil
	}
	if p.Observation != nil {
		return p.Observation.Files
	}
	if p.Binding.InputReview != nil {
		return p.Binding.InputReview.Files
	}
	return nil
}
