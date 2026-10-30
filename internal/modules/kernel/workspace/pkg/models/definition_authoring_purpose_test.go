package workspace

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func requiredPurposeDerivation(t *testing.T, profileIDs ...string) (DefinitionAuthoringRecord, DefinitionDerivationRequest, DefinitionAuthoringMapping) {
	t.Helper()
	r, _, _ := fullSizeMappingFixture(t)
	profileID := "neutral-profile"
	if len(profileIDs) != 0 {
		profileID = profileIDs[0]
	}
	raw := []byte(`{"profileContracts":{"neutral-profile":{"deliverableExpectations":[{"id":"initial-effect","required":true,"description":"完整原交付义务\n后继仍需独立验收","requiredEvidencePurposeContractRef":"runtime/profile-deliverable-evidence-purpose/v2"}],"evidenceRequirements":[{"id":"initial-effect","scope":"deliverable","requiredEvidenceKinds":["state_change"],"acceptedStepKinds":["file/write"]},{"id":"*","scope":"dodItem","requiredEvidenceKinds":["validation"],"acceptedStepKinds":["validation/managed_session"]}]}}}`)
	raw = bytes.ReplaceAll(raw, []byte("neutral-profile"), []byte(profileID))
	if err := json.Unmarshal(raw, &r.TemplateSnapshot.ContextHints); err != nil {
		t.Fatal(err)
	}
	r.TemplateSnapshotHash = WorkspaceTemplateSnapshotHash(&r.TemplateSnapshot)
	r.DerivationRequests = nil
	r.SpecificationRequests = nil
	r, d, _, err := r.ReserveDerivation("required-purpose", "provider", nil, DefinitionAuthoringOwnerBoundV3)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"AdditionalItems":[]}`)
	p, err := DecodeOwnerBoundDerivation(d, data)
	if err != nil {
		t.Fatal(err)
	}
	a := semanticAudit(data)
	r, err = r.CompleteDerivationDocument(d.PermitID, &p, "", &a, nil)
	if err != nil {
		t.Fatal(err)
	}
	d = r.DerivationRequests[0]
	m := mappingFixtureForDerivation(t, d)
	for n := range m.Obligations {
		m.Obligations[n].Verification = DefinitionAuthoringVerificationMapping{Status: GoalVerificationDeclared, Declaration: "独立核对全部原义务", ProfileID: profileID, EvidenceRequirementID: "*", Scope: "dodItem"}
	}
	m.ProfileAcceptanceSelections = []DefinitionProfileAcceptanceSelection{{SourceRef: "profile/" + profileID + "/deliverable/initial-effect", AcceptanceItemRef: "derived-dod:user:000001"}}
	return r, d, m
}
func purposeSemanticAudit(t *testing.T, m DefinitionAuthoringMapping) DefinitionAuthoringSuccessByteAudit {
	t.Helper()
	body, _ := json.Marshal(m)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	for _, k := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"} {
		delete(fields, k)
	}
	raw, _ := json.Marshal(map[string]any{"mapping": fields})
	return semanticAudit(raw)
}
func TestRequiredProfilePurposeMinimalWireCompilesExactSnapshotAndRawLedger(t *testing.T) {
	r, d, m := requiredPurposeDerivation(t)
	a := purposeSemanticAudit(t, m)
	original := append([]byte(nil), a.Document.Content...)
	decoded, err := DecodeAuthoringMappingFor(d, a.Document.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, m) {
		t.Fatal("semantic selection changed")
	}
	s, err := CompileDefinitionAuthoringMapping(d, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.EvidencePurposeMappings) != 1 {
		t.Fatal("required edge omitted")
	}
	mapping := s.EvidencePurposeMappings[0]
	profiles, err := AuthoringProfileContracts(d)
	if err != nil {
		t.Fatal(err)
	}
	if mapping.HistoricalText != "完整原交付义务\n后继仍需独立验收" || mapping.HistoricalProfileHash != runtimecontract.StableRuntimeProfileContractsHash(profiles) {
		t.Fatal("model replaced source metadata")
	}
	if bytes.Contains(original, []byte("HistoricalProfileHash")) || !bytes.Equal(original, a.Document.Content) {
		t.Fatal("provider document was rewritten")
	}
	req := DefinitionAuthoringSpecificationRequest{RequestID: "purpose-proposal", DerivationPermitID: d.PermitID, ProviderBindingHash: d.ProviderBindingHash, Phase: DefinitionSpecificationProposal, WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, "proposal")}
	r, req, _, err = r.ReserveSpecification(req)
	if err != nil {
		t.Fatal(err)
	}
	r, err = r.CompleteSpecificationMapping(req.PermitID, m, a)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(r)
	if repeated, err := r.CompleteSpecificationMapping(req.PermitID, m, a); err != nil {
		t.Fatal(err)
	} else {
		next, _ := json.Marshal(repeated)
		if !bytes.Equal(first, next) {
			t.Fatal("duplicate completion changed ledger")
		}
	}
	var reload DefinitionAuthoringRecord
	if json.Unmarshal(first, &reload) != nil || reload.Validate() != nil {
		t.Fatal("fresh reload invalid")
	}
	if !bytes.Equal(reload.SpecificationRequests[0].SuccessAudit.Document.Content, original) {
		t.Fatal("reload lost raw selection")
	}
	if _, _, err := reload.ConfirmableSpecification(d.PermitID, s.ContentHash()); err == nil {
		t.Fatal("minimal edge bypassed six reviews")
	}
	for _, legacy := range []string{"", DefinitionAuthoringOwnerBoundV2} {
		old := d
		old.WireContractRef = legacy
		old.WireContractHash = AuthoringWireHash(legacy, "derive")
		if _, err := DecodeAuthoringMappingFor(old, original); err == nil {
			t.Fatal("v3 silently entered legacy wire")
		}
	}
}
func TestRequiredProfilePurposeRejectsMissingDuplicateUnknownAndMachineEcho(t *testing.T) {
	_, d, m := requiredPurposeDerivation(t)
	for name, change := range map[string]func(*DefinitionAuthoringMapping){
		"missing": func(m *DefinitionAuthoringMapping) { m.ProfileAcceptanceSelections = nil },
		"duplicate": func(m *DefinitionAuthoringMapping) {
			m.ProfileAcceptanceSelections = append(m.ProfileAcceptanceSelections, m.ProfileAcceptanceSelections[0])
		},
		"unknown": func(m *DefinitionAuthoringMapping) {
			m.ProfileAcceptanceSelections[0].SourceRef = "profile/other/deliverable/initial-effect"
		},
		"goal": func(m *DefinitionAuthoringMapping) {
			m.ProfileAcceptanceSelections[0].AcceptanceItemRef = DefinitionAuthoringGoalItemRef
		},
		"unknown consumer": func(m *DefinitionAuthoringMapping) { m.ProfileAcceptanceSelections[0].AcceptanceItemRef = "cwi-001" },
		"full metadata instead": func(m *DefinitionAuthoringMapping) {
			m.ProfileAcceptanceSelections = nil
			m.EvidencePurposeMappings = []GoalEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeProfileDeliverableEvidencePurposeV2, HistoricalItemRef: "profile/neutral-profile/deliverable/initial-effect", AcceptanceItemRef: "derived-dod:user:000001", HistoricalProfileID: "neutral-profile", HistoricalDeliverableID: "initial-effect", HistoricalProfileHash: "profile-hash", HistoricalText: "完整原文"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := m.Clone()
			change(&bad)
			a := purposeSemanticAudit(t, bad)
			parsed, err := DecodeAuthoringMappingFor(d, a.Document.Content)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := CompileDefinitionAuthoringMapping(d, parsed); err == nil {
				t.Fatal("invalid required relationship compiled")
			}
		})
	}
	raw := purposeSemanticAudit(t, m).Document.Content
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"SourceRef":`), []byte(`"Unknown":`), 1), bytes.Replace(raw, []byte(`"ProfileAcceptanceSelections":[`), []byte(`"ProfileAcceptanceSelections":null,"unused":[`), 1)} {
		if _, err := DecodeAuthoringMappingFor(d, bad); err == nil {
			t.Fatal("unclosed semantic output accepted")
		}
	}
	schema, err := AuthoringWireSchema(d.WireContractRef, "proposal")
	if err != nil || !strings.Contains(schema, "ProfileAcceptanceSelections") {
		t.Fatal("v3 schema missing selection")
	}
	old, _ := AuthoringWireSchema(DefinitionAuthoringOwnerBoundV2, "proposal")
	if strings.Contains(old, "ProfileAcceptanceSelections") {
		t.Fatal("v2 schema changed")
	}
}

func TestRequiredPurposeSelectionChangeCannotReuseReviewsOrCompletion(t *testing.T) {
	r, d, m := requiredPurposeDerivation(t)
	request := DefinitionAuthoringSpecificationRequest{RequestID: "purpose-proposal", DerivationPermitID: d.PermitID, ProviderBindingHash: d.ProviderBindingHash, Phase: DefinitionSpecificationProposal, WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, "proposal")}
	r, request, _, err := r.ReserveSpecification(request)
	if err != nil {
		t.Fatal(err)
	}
	a := purposeSemanticAudit(t, m)
	r, err = r.CompleteSpecificationMapping(request.PermitID, m, a)
	if err != nil {
		t.Fatal(err)
	}
	original := r.SpecificationRequests[0].Proposal.Clone()
	for _, kind := range goalReviewKinds {
		req := DefinitionAuthoringSpecificationRequest{RequestID: "review/" + kind, DerivationPermitID: d.PermitID, ProviderBindingHash: d.ProviderBindingHash, Phase: DefinitionSpecificationCritic, ReviewKind: kind, ProposalHash: original.ContentHash(), WireContractRef: d.WireContractRef, WireContractHash: AuthoringWireHash(d.WireContractRef, kind)}
		r, req, _, err = r.ReserveSpecification(req)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := original.RequiredReviewRequirementIDs(kind)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"review": map[string]any{"Status": "PASS", "Reason": "明确核对原义务及精确消费者", "EvidenceRefs": []string{DefinitionAuthoringGoalItemRef}, "RequirementIDs": ids}})
		review, err := DecodeOwnerBoundReview(req, raw)
		if err != nil {
			t.Fatal(err)
		}
		r, err = r.CompleteReviewDocument(req.PermitID, review, semanticAudit(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := r.ConfirmableSpecification(d.PermitID, original.ContentHash()); err != nil {
		t.Fatal(err)
	}
	changed := m.Clone()
	changed.ProfileAcceptanceSelections[0].AcceptanceItemRef = "derived-dod:user:000002"
	replacement, err := CompileDefinitionAuthoringMapping(d, changed)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ContentHash() == original.ContentHash() {
		t.Fatal("selection omitted from specification hash")
	}
	if _, err := r.CompleteSpecificationMapping(request.PermitID, changed, purposeSemanticAudit(t, changed)); err == nil {
		t.Fatal("duplicate permit replaced original selection")
	}
	if _, _, err := r.ConfirmableSpecification(d.PermitID, replacement.ContentHash()); err == nil {
		t.Fatal("old six PASS confirmed changed relationship")
	}
	body, _ := json.Marshal(r)
	var reload DefinitionAuthoringRecord
	if json.Unmarshal(body, &reload) != nil || reload.Validate() != nil {
		t.Fatal("reviewed v3 invalid after fresh load")
	}
	if _, _, err := reload.ConfirmableSpecification(d.PermitID, original.ContentHash()); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredPurposeConcurrentProfilesPreserveIndependentSnapshots(t *testing.T) {
	_, first, a := requiredPurposeDerivation(t, "music-profile")
	_, second, b := requiredPurposeDerivation(t, "inventory-profile")
	var wg sync.WaitGroup
	for _, input := range []struct {
		d       DefinitionDerivationRequest
		m       DefinitionAuthoringMapping
		profile string
	}{{first, a, "music-profile"}, {second, b, "inventory-profile"}} {
		for range 4 {
			wg.Go(func() {
				before, _ := json.Marshal(input.d)
				original := purposeSemanticAudit(t, input.m).Document.Content
				parsed, err := DecodeAuthoringMappingFor(input.d, original)
				if err != nil {
					t.Error(err)
					return
				}
				spec, err := CompileDefinitionAuthoringMapping(input.d, parsed)
				if err != nil {
					t.Error(err)
					return
				}
				mapping := spec.EvidencePurposeMappings[0]
				if mapping.HistoricalProfileID != input.profile || mapping.HistoricalItemRef != "profile/"+input.profile+"/deliverable/initial-effect" {
					t.Error("cross-profile source")
				}
				profiles, err := AuthoringProfileContracts(input.d)
				if err != nil || mapping.HistoricalProfileHash != runtimecontract.StableRuntimeProfileContractsHash(profiles) {
					t.Error("cross-profile snapshot", err)
				}
				after, _ := json.Marshal(input.d)
				if !bytes.Equal(before, after) || !bytes.Equal(original, purposeSemanticAudit(t, input.m).Document.Content) {
					t.Error("compiler mutated sealed or raw input")
				}
			})
		}
	}
	wg.Wait()
}
