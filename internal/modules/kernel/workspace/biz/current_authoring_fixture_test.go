package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"testing"
)

// completeTestDerivation supplies explicit current semantic bytes for synthetic provider fixtures.
func completeTestDerivation(r workspace.DefinitionAuthoringRecord, permit string, p *workspace.DefinitionDerivedAcceptance, failure string, ds ...*workspace.DefinitionAuthoringDiagnostic) (workspace.DefinitionAuthoringRecord, error) {
	if p == nil {
		return r.CompleteDerivation(permit, p, failure, ds...)
	}
	items := append([]string{}, p.AdditionalItems...)
	p = &workspace.DefinitionDerivedAcceptance{AuthoringInputHash: p.AuthoringInputHash, TemplateSnapshotHash: p.TemplateSnapshotHash, AdditionalItems: items}
	raw, err := json.Marshal(struct{ AdditionalItems []string }{items})
	if err != nil {
		return workspace.DefinitionAuthoringRecord{}, err
	}
	sum := sha256.Sum256(raw)
	o := workspace.DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	audit := workspace.DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}
	var diag *workspace.DefinitionAuthoringDiagnostic
	if len(ds) > 0 {
		diag = ds[0]
	}
	return r.CompleteDerivationDocument(permit, p, failure, &audit, diag)
}

func completeTestSpecification(r workspace.DefinitionAuthoringRecord, permit string, p *workspace.GoalSpecification, review *workspace.GoalSpecificationCompletenessReview, failure string, ds ...*workspace.DefinitionAuthoringDiagnostic) (workspace.DefinitionAuthoringRecord, error) {
	if review == nil {
		return r.CompleteSpecification(permit, p, review, failure, ds...)
	}
	raw, err := json.Marshal(struct {
		Review struct {
			Status         string
			Reason         string
			EvidenceRefs   []string
			RequirementIDs []string
		} `json:"review"`
	}{Review: struct {
		Status         string
		Reason         string
		EvidenceRefs   []string
		RequirementIDs []string
	}{review.Status, review.Reason, append([]string{}, review.EvidenceRefs...), append([]string{}, review.RequirementIDs...)}})
	if err != nil {
		return workspace.DefinitionAuthoringRecord{}, err
	}
	sum := sha256.Sum256(raw)
	o := workspace.DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return r.CompleteReviewDocument(permit, *review, workspace.DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()})
}

func derivationTestAudit(t *testing.T, p *workspace.DefinitionDerivedAcceptance) *workspace.DefinitionAuthoringSuccessByteAudit {
	t.Helper()
	raw, err := json.Marshal(struct{ AdditionalItems []string }{append([]string{}, p.AdditionalItems...)})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	o := workspace.DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return &workspace.DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}
}

func reviewTestAudit(t *testing.T, review *workspace.GoalSpecificationCompletenessReview) *workspace.DefinitionAuthoringSuccessByteAudit {
	t.Helper()
	raw, err := json.Marshal(struct {
		Review struct {
			Status         string
			Reason         string
			EvidenceRefs   []string
			RequirementIDs []string
		} `json:"review"`
	}{Review: struct {
		Status         string
		Reason         string
		EvidenceRefs   []string
		RequirementIDs []string
	}{review.Status, review.Reason, append([]string{}, review.EvidenceRefs...), append([]string{}, review.RequirementIDs...)}})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	o := workspace.DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return &workspace.DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}
}
