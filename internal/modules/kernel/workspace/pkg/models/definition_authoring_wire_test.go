package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
)

func semanticAudit(payload []byte) DefinitionAuthoringSuccessByteAudit {
	h := sha256.Sum256(payload)
	o := DefinitionProviderObservation{Observed: true, Complete: true, Saved: true, ObservedBytes: uint64(len(payload)), SHA256: hex.EncodeToString(h[:]), Content: append([]byte{}, payload...)}
	return DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}
}
func semanticMappingFixture(t *testing.T, d DefinitionDerivationRequest) (DefinitionAuthoringMapping, DefinitionAuthoringSuccessByteAudit) {
	t.Helper()
	m := mappingFixtureForDerivation(t, d)
	for i := range m.Obligations {
		m.Obligations[i].Verification = DefinitionAuthoringVerificationMapping{Declaration: "ordinary validation of original item", Status: GoalVerificationDeclared, ProfileID: "profile", EvidenceRequirementID: "*", Scope: "dodItem"}
	}
	raw, _ := json.Marshal(m)
	var body map[string]json.RawMessage
	_ = json.Unmarshal(raw, &body)
	for _, k := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"} {
		delete(body, k)
	}
	payload, _ := json.Marshal(map[string]any{"mapping": body})
	return m, semanticAudit(payload)
}
func TestOwnerBoundAuthoringExactLedgerAcrossReloadAndSixReviews(t *testing.T) {
	r, _ := authoringLedgerFixture(t)
	r.DerivationRequests = nil
	r.SpecificationRequests = nil
	r, d, fresh, err := r.ReserveDerivation("v2-derive", "provider", nil, DefinitionAuthoringOwnerBoundV2)
	if err != nil || !fresh {
		t.Fatal(err)
	}
	deriveBytes := []byte(" {\"AdditionalItems\":[]}\r\n")
	p, err := DecodeOwnerBoundDerivation(d, deriveBytes)
	if err != nil {
		t.Fatal(err)
	}
	audit := semanticAudit(deriveBytes)
	if _, err = r.CompleteDerivation(d.PermitID, &p, ""); err == nil {
		t.Fatal("accepted v2 result without original bytes")
	}
	r, err = r.CompleteDerivationDocument(d.PermitID, &p, "", &audit, nil)
	if err != nil {
		t.Fatal(err)
	}
	d = r.DerivationRequests[0]
	req := DefinitionAuthoringSpecificationRequest{RequestID: "v2-proposal", DerivationPermitID: d.PermitID, ProviderBindingHash: d.ProviderBindingHash, Phase: DefinitionSpecificationProposal, WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, "proposal")}
	r, req, fresh, err = r.ReserveSpecification(req)
	if err != nil || !fresh {
		t.Fatal(err)
	}
	m, a := semanticMappingFixture(t, d)
	decoded, err := DecodeAuthoringMappingFor(d, a.Document.Content)
	if err != nil || !reflect.DeepEqual(m, decoded) {
		t.Fatal(err)
	}
	if _, err := DecodeDefinitionAuthoringMapping(a.Document.Content); err == nil {
		t.Fatal("v2 silently accepted by v1")
	}
	r, err = r.CompleteSpecificationMapping(req.PermitID, m, a)
	if err != nil {
		t.Fatal(err)
	}
	proposal := r.SpecificationRequests[0].Proposal
	for _, kind := range goalReviewKinds {
		reviewRequest := DefinitionAuthoringSpecificationRequest{RequestID: "v2-" + kind, DerivationPermitID: d.PermitID, ProviderBindingHash: d.ProviderBindingHash, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: proposal.ContentHash(), WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, kind)}
		r, reviewRequest, _, err = r.ReserveSpecification(reviewRequest)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := proposal.RequiredReviewRequirementIDs(kind)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(map[string]any{"review": map[string]any{"Status": "PASS", "Reason": "explicit local fixture checks exact source coverage", "EvidenceRefs": []string{DefinitionAuthoringGoalItemRef}, "RequirementIDs": ids}})
		review, err := DecodeOwnerBoundReview(reviewRequest, payload)
		if err != nil {
			t.Fatal(err)
		}
		r, err = r.CompleteReviewDocument(reviewRequest.PermitID, review, semanticAudit(payload))
		if err != nil {
			t.Fatal(err)
		}
	}
	bytes, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var reload DefinitionAuthoringRecord
	if err = json.Unmarshal(bytes, &reload); err != nil || reload.Validate() != nil {
		t.Fatalf("reload: %v %v", err, reload.Validate())
	}
	if _, _, fresh, err = reload.ReserveDerivation("v2-derive", "provider", nil, DefinitionAuthoringOwnerBoundV2); err != nil || fresh {
		t.Fatal("replay issued second permit", err)
	}
	if _, _, fresh, err = reload.ReserveSpecification(req); err != nil || fresh {
		t.Fatal("proposal replay issued second permit", err)
	}
	if _, _, err = reload.ConfirmableSpecification(d.PermitID, proposal.ContentHash()); err != nil {
		t.Fatal(err)
	}
	bad := reload.Clone()
	bad.SpecificationRequests[1].ProposalHash = "other"
	bad.SpecificationRequests[1].PermitID = bad.SpecificationRequests[1].ExpectedPermitID()
	if bad.Validate() == nil {
		t.Fatal("review rebound to different proposal")
	}
	bad = reload.Clone()
	bad.DerivationRequests[0].SuccessAudit.Document.Content[0] = '!'
	if bad.Validate() == nil {
		t.Fatal("raw bytes drift accepted")
	}
	bad = reload.Clone()
	bad.SpecificationRequests[0].WireContractHash = "drift"
	bad.SpecificationRequests[0].PermitID = bad.SpecificationRequests[0].ExpectedPermitID()
	if bad.Validate() == nil {
		t.Fatal("wire drift accepted")
	}
}
func TestOwnerBoundAuthoringClosedSemanticDocumentsAndLegacyIsolation(t *testing.T) {
	r, _ := authoringLedgerFixture(t)
	r.DerivationRequests = nil
	r.SpecificationRequests = nil
	_, d, _, err := r.ReserveDerivation("v2", "provider", nil, DefinitionAuthoringOwnerBoundV2)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"AdditionalItems":[],"AuthoringInputHash":"forged"}`, `{"AdditionalItems":[],"AdditionalItems":[]}`, `{"AdditionalItems":null}`, `{"AdditionalItems":[]} {}`, `{"additionalitems":[]}`, `{"AdditionalItems":["\ud800"]}`} {
		if _, err := DecodeOwnerBoundDerivation(d, []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, _, _, err := r.ReserveDerivation("v2", "provider", nil, "unknown"); err == nil {
		t.Fatal("unknown wire selected")
	}
}

// Frozen from 439b3428's schema, independently hashed from the original source.
func TestLegacyOwnerBoundV2WireHashesRemainExact(t *testing.T) {
	for phase, want := range map[string]string{
		"derive":       "authoring-wire-sha256:018582e6f647621e4bc358b14e7852b28f5f4168fc282d620c34e61d6243d7ef",
		"proposal":     "authoring-wire-sha256:81b01904bf5bbb4c85336898d7e53448d4a14ecc2cb2d9e5b8796d6118ad4421",
		"traceability": "authoring-wire-sha256:b94dbe284446d1fb54e6d8b21ebddb2708b86aa7f700c516a261dcfef4efdc9f",
	} {
		if got := AuthoringWireHash(DefinitionAuthoringOwnerBoundV2, phase); got != want {
			t.Fatalf("%s: old wire identity changed: %s", phase, got)
		}
	}
}
