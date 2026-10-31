package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// completeTestDerivation supplies explicit current semantic bytes for synthetic provider fixtures.
func completeTestDerivation(r DefinitionAuthoringRecord, permit string, p *DefinitionDerivedAcceptance, failure string, ds ...*DefinitionAuthoringDiagnostic) (DefinitionAuthoringRecord, error) {
	if p == nil {
		return r.CompleteDerivation(permit, p, failure, ds...)
	}
	items := append([]string{}, p.AdditionalItems...)
	p = &DefinitionDerivedAcceptance{AuthoringInputHash: p.AuthoringInputHash, TemplateSnapshotHash: p.TemplateSnapshotHash, AdditionalItems: items}
	raw, err := json.Marshal(struct{ AdditionalItems []string }{items})
	if err != nil {
		return DefinitionAuthoringRecord{}, err
	}
	sum := sha256.Sum256(raw)
	o := DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	audit := DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}
	var diag *DefinitionAuthoringDiagnostic
	if len(ds) > 0 {
		diag = ds[0]
	}
	return r.CompleteDerivationDocument(permit, p, failure, &audit, diag)
}

func completeTestSpecification(r DefinitionAuthoringRecord, permit string, p *GoalSpecification, review *GoalSpecificationCompletenessReview, failure string, ds ...*DefinitionAuthoringDiagnostic) (DefinitionAuthoringRecord, error) {
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
		return DefinitionAuthoringRecord{}, err
	}
	sum := sha256.Sum256(raw)
	o := DefinitionProviderObservation{Content: raw, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return r.CompleteReviewDocument(permit, *review, DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()})
}

func decodeCurrentMappingFixture(payload []byte) (DefinitionAuthoringMapping, error) {
	return DecodeAuthoringMappingFor(DefinitionDerivationRequest{WireContractRef: DefinitionAuthoringOwnerBound, WireContractHash: AuthoringWireHash(DefinitionAuthoringOwnerBound, "derive")}, payload)
}
