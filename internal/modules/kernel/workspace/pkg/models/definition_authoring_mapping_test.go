package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fullSizeMappingFixture(t testing.TB) (DefinitionAuthoringRecord, DefinitionAuthoringMapping, DefinitionAuthoringSuccessByteAudit) {
	t.Helper()
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-full-mapping", Revision: 1, GoalText: "交付完整目标，保留独立Goal原文", TemplateID: "mapping-template"}
	for i := 1; i <= 5; i++ {
		input.AcceptanceRequirements = append(input.AcceptanceRequirements, fmt.Sprintf("用户义务 %02d：保持原字节与all_of", i))
	}
	template := WorkspaceTemplate{ID: input.TemplateID}
	for i := 1; i <= 13; i++ {
		template.DoDItems = append(template.DoDItems, fmt.Sprintf("模板义务 %02d：独立来源", i))
	}
	derived := DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{}}
	for i := 1; i <= 19; i++ {
		derived.AdditionalItems = append(derived.AdditionalItems, fmt.Sprintf("派生义务 %02d：必须显式映射", i))
	}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	record, d, _, err := record.ReserveDerivation("derive/"+input.InputHash(), "mock-provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(d.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	d = record.DerivationRequests[0]
	m := mappingFixtureForDerivation(t, d)
	audit := mappingAuditFixture(t, m)
	return record, m, audit
}
func mappingFixtureForDerivation(t testing.TB, d DefinitionDerivationRequest) DefinitionAuthoringMapping {
	t.Helper()
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		t.Fatal(err)
	}
	m := DefinitionAuthoringMapping{Protocol: DefinitionAuthoringMappingProtocol, InputHash: d.Input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&d.Template), ResourceSnapshotHash: d.ResourceSnapshotHash, GoalRequirement: DefinitionAuthoringGoalMapping{DimensionRef: "whole", ResourceIDs: []string{}}, Dimensions: []DefinitionAuthoringDimensionMapping{{Ref: "whole", ResourceIDs: []string{}}}, Requirements: []DefinitionAuthoringRequirementMapping{}, Obligations: []DefinitionAuthoringObligationMapping{}}
	for _, kind := range []string{GoalRequirementUserExplicit, GoalRequirementTemplateRequired, GoalRequirementDerived} {
		req := DefinitionAuthoringRequirementMapping{Ref: kind, DimensionRef: "whole", Text: "同来源分组 " + kind, SourceItemRefs: []string{}, ResourceIDs: []string{}}
		for _, item := range items {
			if authoringItemKind(item) != kind {
				continue
			}
			req.SourceItemRefs = append(req.SourceItemRefs, item.CandidateItemRef)
			m.Obligations = append(m.Obligations, DefinitionAuthoringObligationMapping{CandidateItemRef: item.CandidateItemRef, RequirementRef: kind, Verification: DefinitionAuthoringVerificationMapping{Declaration: "明确缺少可信声明", Status: GoalVerificationUnverified}})
		}
		if len(req.SourceItemRefs) > 0 {
			m.Requirements = append(m.Requirements, req)
		}
	}
	return m
}
func mappingAuditFixture(t testing.TB, m DefinitionAuthoringMapping) DefinitionAuthoringSuccessByteAudit {
	t.Helper()
	payload, err := json.Marshal(struct {
		Mapping DefinitionAuthoringMapping `json:"mapping"`
	}{m})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	document := DefinitionProviderObservation{Content: payload, ObservedBytes: uint64(len(payload)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return DefinitionAuthoringSuccessByteAudit{Document: document, Wire: document.Clone()}
}
func TestSealedMappingCompilesFull37AndIndependentGoal(t *testing.T) {
	r, m, audit := fullSizeMappingFixture(t)
	d := r.DerivationRequests[0]
	decoded, err := DecodeDefinitionAuthoringMapping(audit.Document.Content)
	if err != nil || !reflect.DeepEqual(decoded, m) {
		t.Fatalf("strict mapping decode: %v", err)
	}
	spec, err := CompileDefinitionAuthoringMapping(d, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Obligations) != 37 || len(spec.Verifications) != 37 || len(spec.Requirements) != 4 {
		t.Fatalf("full-size cardinality obligations=%d verifications=%d requirements=%d", len(spec.Obligations), len(spec.Verifications), len(spec.Requirements))
	}
	items, _ := d.Proposal.CandidateItems(d.Input, &d.Template)
	counts := map[string]int{}
	for _, item := range items {
		found := 0
		for _, o := range spec.Obligations {
			if slices.Equal(o.DoDItemRefs, []string{item.CandidateItemRef}) && o.Text == item.Text {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("candidate %s not mapped exactly once", item.CandidateItemRef)
		}
		counts[authoringItemKind(item)]++
	}
	if counts[GoalRequirementUserExplicit] != 5 || counts[GoalRequirementTemplateRequired] != 13 || counts[GoalRequirementDerived] != 19 {
		t.Fatal(counts)
	}
	goals := 0
	for _, req := range spec.Requirements {
		if slices.Equal(req.SourceItemRefs, []string{DefinitionAuthoringGoalItemRef}) {
			goals++
			if req.Text != d.Input.GoalText || !req.Required {
				t.Fatal("Goal changed")
			}
		}
	}
	if goals != 1 {
		t.Fatal("Goal requirement not independent")
	}
	if spec.ContentHash() == audit.Document.SHA256 {
		t.Fatal("canonical and raw wire hashes conflated")
	}
	// Explicit grouping identities and sealed source membership, not array positions
	// or model labels, determine canonical identities and order.
	reordered := m.Clone()
	slices.Reverse(reordered.Dimensions)
	slices.Reverse(reordered.Requirements)
	slices.Reverse(reordered.Obligations)
	for i := range reordered.Requirements {
		slices.Reverse(reordered.Requirements[i].SourceItemRefs)
	}
	other, err := CompileDefinitionAuthoringMapping(d, reordered)
	if err != nil || !reflect.DeepEqual(spec, other) {
		t.Fatalf("array order influenced canonical result: %v", err)
	}
	renamed := m.Clone()
	renamed.Dimensions[0].Ref = "renamed"
	renamed.GoalRequirement.DimensionRef = "renamed"
	for i := range renamed.Requirements {
		old := renamed.Requirements[i].Ref
		renamed.Requirements[i].Ref = "new-" + old
		renamed.Requirements[i].DimensionRef = "renamed"
		for j := range renamed.Obligations {
			if renamed.Obligations[j].RequirementRef == old {
				renamed.Obligations[j].RequirementRef = "new-" + old
			}
		}
	}
	other, err = CompileDefinitionAuthoringMapping(d, renamed)
	if err != nil || !reflect.DeepEqual(spec, other) {
		t.Fatalf("wire labels influenced canonical ID: %v", err)
	}
}
func TestSealedMappingFull37RejectsMissingDuplicateMixedAndDangling(t *testing.T) {
	r, m, _ := fullSizeMappingFixture(t)
	d := r.DerivationRequests[0]
	tests := []struct {
		name   string
		change func(*DefinitionAuthoringMapping)
	}{
		{"missing obligation", func(m *DefinitionAuthoringMapping) { m.Obligations = m.Obligations[:36] }},
		{"duplicate obligation", func(m *DefinitionAuthoringMapping) { m.Obligations[1] = m.Obligations[0] }},
		{"unknown candidate", func(m *DefinitionAuthoringMapping) { m.Obligations[0].CandidateItemRef = "unknown" }},
		{"dangling requirement", func(m *DefinitionAuthoringMapping) { m.Obligations[0].RequirementRef = "unknown" }},
		{"wrong group", func(m *DefinitionAuthoringMapping) { m.Obligations[0].RequirementRef = GoalRequirementDerived }},
		{"missing grouped source", func(m *DefinitionAuthoringMapping) {
			m.Requirements[0].SourceItemRefs = m.Requirements[0].SourceItemRefs[:4]
		}},
		{"duplicate grouped source", func(m *DefinitionAuthoringMapping) {
			m.Requirements[0].SourceItemRefs[1] = m.Requirements[0].SourceItemRefs[0]
		}},
		{"mixed source", func(m *DefinitionAuthoringMapping) {
			m.Requirements[0].SourceItemRefs = append(m.Requirements[0].SourceItemRefs, m.Requirements[1].SourceItemRefs[0])
			m.Requirements[1].SourceItemRefs = m.Requirements[1].SourceItemRefs[1:]
		}},
		{"Goal mixed with candidate", func(m *DefinitionAuthoringMapping) {
			m.Requirements[0].SourceItemRefs[0] = DefinitionAuthoringGoalItemRef
		}},
		{"resource invented", func(m *DefinitionAuthoringMapping) { m.Requirements[0].ResourceIDs = []string{"fake-resource"} }},
		{"input drift", func(m *DefinitionAuthoringMapping) { m.InputHash = "fake" }},
		{"template drift", func(m *DefinitionAuthoringMapping) { m.TemplateSnapshotHash = "fake" }},
		{"resource drift", func(m *DefinitionAuthoringMapping) { m.ResourceSnapshotHash = "fake" }},
		{"old protocol", func(m *DefinitionAuthoringMapping) { m.Protocol = "old" }},
		{"invalid binding", func(m *DefinitionAuthoringMapping) { m.Obligations[0].Verification.ProfileID = "fake" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := m.Clone()
			tt.change(&bad)
			if _, err := CompileDefinitionAuthoringMapping(d, bad); err == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
}
func TestSealedMappingClosedFramingAndShape(t *testing.T) {
	_, _, a := fullSizeMappingFixture(t)
	valid := string(a.Document.Content)
	expected, err := DecodeDefinitionAuthoringMapping(a.Document.Content)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", " ", "\t\r\n "} {
		decoded, err := DecodeDefinitionAuthoringMapping([]byte(valid + suffix))
		if err != nil || !reflect.DeepEqual(decoded, expected) {
			t.Fatalf("合法根对象和 JSON 空白未原样解码：%q %v", suffix, err)
		}
	}
	tests := map[string][]byte{
		"trailing NUL": []byte(valid + "\x00"), "trailing vertical tab": []byte(valid + "\v"), "trailing form feed": []byte(valid + "\f"), "trailing unicode whitespace": []byte(valid + " "),
		"old canonical shape": []byte(`{"specification":{}}`), "array root": []byte(`[]`), "scalar": []byte(`true`), "multiple root": []byte(valid + valid), "trailing": []byte(valid + " extra"), "fence": []byte("```json\n" + valid + "\n```"), "UTF8": append([]byte(valid), 0xff), "oversize": []byte(strings.Repeat(" ", DefinitionAuthoringDiagnosticDocumentLimit) + valid),
		"missing": []byte(strings.Replace(valid, `"Protocol":"`+DefinitionAuthoringMappingProtocol+`",`, "", 1)), "case": []byte(strings.Replace(valid, `"Protocol"`, `"protocol"`, 1)), "unknown": []byte(strings.Replace(valid, `"mapping":{`, `"mapping":{"Resources":[],`, 1)), "duplicate": []byte(strings.Replace(valid, `"mapping":{`, `"mapping":{"Protocol":"duplicate",`, 1)), "null": []byte(strings.Replace(valid, `"Dimensions":[`, `"Dimensions":null,"duplicate": [`, 1)), "surrogate": []byte(strings.Replace(valid, `"whole"`, `"\ud800"`, 1)),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDefinitionAuthoringMapping(payload); err == nil {
				t.Fatal("invalid document accepted")
			}
		})
	}
}
func TestSealedMappingLedgerRawReparseAndAuditIdentity(t *testing.T) {
	r, m, a := fullSizeMappingFixture(t)
	req := DefinitionAuthoringSpecificationRequest{RequestID: "proposal/current", DerivationPermitID: r.DerivationRequests[0].PermitID, Phase: DefinitionSpecificationProposal, ProviderBindingHash: "mock-provider"}
	pending, req, _, err := r.ReserveSpecification(req)
	if err != nil {
		t.Fatal(err)
	}
	complete, err := pending.CompleteSpecificationMapping(req.PermitID, m, a)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := complete.CompleteSpecificationMapping(req.PermitID, m, a)
	if err != nil || !reflect.DeepEqual(complete, replay) {
		t.Fatalf("duplicate changed completion: %v", err)
	}
	if complete.Validate() != nil {
		t.Fatal("persisted ledger invalid")
	}
	view := complete.Clone()
	view.SpecificationRequests[0] = view.SpecificationRequests[0].Metadata()
	if view.ValidateView() != nil {
		t.Fatal("metadata view rejected")
	}
	if view.Validate() == nil {
		t.Fatal("metadata view passed owner raw validation")
	}
	altered := complete.Clone()
	altered.SpecificationRequests[0].SuccessAudit.Document.Content[0] = ' '
	if altered.Validate() == nil {
		t.Fatal("raw bytes changed without hash rejected too late")
	}
	altered = complete.Clone()
	altered.SpecificationRequests[0].Mapping.Requirements[0].Text = "coordinated typed claim"
	if altered.Validate() == nil {
		t.Fatal("typed mapping/raw mismatch accepted")
	}
	// A valid other raw with a matching raw hash cannot authorize the original mapping.
	other := m.Clone()
	other.Requirements[0].Text = "another proposal"
	otherAudit := mappingAuditFixture(t, other)
	if _, err := pending.CompleteSpecificationMapping(req.PermitID, m, otherAudit); err == nil {
		t.Fatal("legal mapping + another complete raw accepted")
	}
	withUnsavedWire := a.Clone()
	withUnsavedWire.Wire = DefinitionProviderObservation{Observed: true, Complete: true, Saved: false, ObservedBytes: DefinitionAuthoringDiagnosticDocumentLimit + 1, SHA256: strings.Repeat("a", 64), Reason: "observation_storage_limit"}
	if withUnsavedWire.Validate() != nil {
		t.Fatal("unsaved transport wire incorrectly invalidated complete successful document")
	}
	if _, err := pending.CompleteSpecificationMapping(req.PermitID, m, withUnsavedWire); err != nil {
		t.Fatal(err)
	}
	badAudit := a.Clone()
	badAudit.Document = withUnsavedWire.Wire
	if badAudit.Validate() == nil {
		t.Fatal("unsaved document authorized canonical compilation")
	}
	if _, err := pending.CompleteSpecification(req.PermitID, complete.SpecificationRequests[0].Proposal, nil, ""); err == nil {
		t.Fatal("old direct canonical completion accepted")
	}
	if DefinitionAuthoringSpecificationRequestLimit != 112 || DefinitionAuthoringSpecificationAttemptLimit != 2 {
		t.Fatal("budget enlarged")
	}
}
