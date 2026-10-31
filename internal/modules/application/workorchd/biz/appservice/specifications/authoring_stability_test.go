package specifications

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func stabilityDerivation(t *testing.T, id, goal string, wireRefs ...string) workspace.DefinitionDerivationRequest {
	t.Helper()
	defaults, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	var template workspace.WorkspaceTemplate
	for _, value := range defaults {
		if value.ID == "software-dev" {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &template); err != nil {
				t.Fatal(err)
			}
		}
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: common.WorkspaceID(id), Revision: 1, TemplateID: template.ID, GoalText: goal}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	wire := workspace.DefinitionAuthoringOwnerBound
	if len(wireRefs) != 0 {
		wire = wireRefs[0]
	}
	record, d, _, err := record.ReserveDerivation("derive-stability", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash}
	if wire == "" {
		record, err = completeTestDerivation(record, d.PermitID, &derived, "")
	} else {
		raw := []byte(" {\"AdditionalItems\":[]}\r\n")
		derived, err = workspace.DecodeOwnerBoundDerivation(d, raw)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		observation := workspace.DefinitionProviderObservation{Observed: true, Complete: true, Saved: true, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(sum[:]), Content: raw}
		audit := workspace.DefinitionAuthoringSuccessByteAudit{Document: observation, Wire: observation.Clone()}
		record, err = record.CompleteDerivationDocument(d.PermitID, &derived, "", &audit, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return record.DerivationRequests[0]
}

func stabilityProposal(t *testing.T, d workspace.DefinitionDerivationRequest) workspace.GoalSpecification {
	t.Helper()
	m, err := testkit.Mapping(d, workspace.DefinitionAuthoringVerificationMapping{Declaration: "按原义务验证行为；全部适用 owner 与终态约束仍合取", Status: workspace.GoalVerificationDeclared, ProfileID: "software-dev", EvidenceRequirementID: "*", Scope: "dodItem"})
	if err != nil {
		t.Fatal(err)
	}
	m, err = testkit.SelectProfileAcceptance(d, m, "derived-dod:template:000001")
	if err != nil {
		t.Fatal(err)
	}
	audit, err := testkit.Audit(m)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.OwnerBoundAuthoringWire(d.WireContractRef) {
		audit.Document.Content = testkit.OwnerBoundFixtureDocument(audit.Document.Content)
	}
	raw := audit.Document.Content
	p, err := ParseAuthoringProposal(raw, d)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAuthoringStableSingleReferenceRetainsMergedTerminalConsumers(t *testing.T) {
	d := stabilityDerivation(t, "ws-stability", "交付项目原需求，保留全部模板义务")
	p := stabilityProposal(t, d)
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"}
	command, err := AuthoringProposalCommand(d, target)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Options []authoringCandidateVerificationOptions `json:"sourceApplicableVerificationDeclarations"`
	}
	if err := json.Unmarshal(authoringTestPayload(t, command), &input); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"dod-003": "template-minimal-change-owner-receipt", "dod-004": "template-final-git-snapshot", "dod-013": "template-final-result-evidence-index"}
	seen := 0
	for _, option := range input.Options {
		if option.DeclarationBindingMode != authoringSingleDeclarationBinding || option.SourceScope != "dodItem" {
			t.Fatal("single reference contract missing")
		}
		if ref := want[option.SourceID]; ref != "" {
			if len(option.Declarations) != 2 || len(option.MergedRequiredConsumers) != 1 || option.MergedRequiredConsumers[0].Ref != ref {
				t.Fatalf("merged requirement lost or duplicated: %+v", option)
			}
			seen++
		}
	}
	if seen != len(want) || len(p.Verifications) != len(p.Obligations) {
		t.Fatal("source coverage or one-verification shape changed")
	}
	for _, kind := range ReviewKinds() {
		critic, err := AuthoringReviewCommand(d, p, kind, target)
		if err != nil {
			t.Fatal(err)
		}
		var reviewInput struct {
			Options []authoringCandidateVerificationOptions `json:"sourceApplicableVerificationDeclarations"`
		}
		if err := json.Unmarshal(authoringTestPayload(t, critic), &reviewInput); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(input.Options, reviewInput.Options) {
			t.Fatal("proposal and critic use different rules")
		}
		if !strings.HasPrefix(critic.Request.PromptContext.Sections[0].Content, authoringVerificationInstructions) || !strings.HasPrefix(command.Request.PromptContext.Sections[0].Content, authoringVerificationInstructions) {
			t.Fatal("proposal and critic lack the same versioned binding/merge contract")
		}
	}
}

func TestAuthoringStabilityReplaysOriginalDocumentsWithoutChangingFailureOrCoverage(t *testing.T) {
	for _, wire := range []string{workspace.DefinitionAuthoringOwnerBound} {
		d := stabilityDerivation(t, "ws-review-replay", "保持完整需求与真实失败", wire)
		p := stabilityProposal(t, d)
		before, _ := json.Marshal(d)
		for _, kind := range ReviewKinds() {
			ids, err := p.RequiredReviewRequirementIDs(kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, status := range []string{workspace.GoalReviewPass, workspace.GoalReviewFail, workspace.GoalReviewUnknown} {
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: status, Reason: "逐项独立判断，保留原结论和依据", SpecificationHash: p.ContentHash(), EvidenceRefs: []string{p.GoalItemRef}, RequirementIDs: ids}
				raw, err := json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{review})
				if err != nil {
					t.Fatal(err)
				}
				if wire != "" {
					raw = testkit.OwnerBoundFixtureDocument(raw)
				}
				original := bytes.Clone(raw)
				for range 5 {
					got, err := ParseAuthoringReview(raw, kind, p, d)
					if err != nil || !reflect.DeepEqual(got, review) || !bytes.Equal(raw, original) {
						t.Fatalf("review replay changed original result: %v", err)
					}
				}
			}
			review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, Reason: "漏审不得被补成通过", SpecificationHash: p.ContentHash(), EvidenceRefs: []string{p.GoalItemRef}, RequirementIDs: ids[:len(ids)-1]}
			raw, _ := json.Marshal(struct {
				Review workspace.GoalSpecificationCompletenessReview `json:"review"`
			}{review})
			if wire != "" {
				raw = testkit.OwnerBoundFixtureDocument(raw)
			}
			if _, err := ParseAuthoringReview(raw, kind, p, d); err == nil {
				t.Fatal("missing required coverage accepted")
			}
		}
		after, _ := json.Marshal(d)
		if !bytes.Equal(before, after) {
			t.Fatal("parsing changed persisted input")
		}

	}
}

func TestAuthoringStabilityConcurrentWorkspacesPreserveExactContexts(t *testing.T) {
	// Same Profile, unrelated projects; no shared mutable prompt or candidate.
	first := stabilityDerivation(t, "ws-project-a", "实现音乐分类项目", workspace.DefinitionAuthoringOwnerBound)
	second := stabilityDerivation(t, "ws-project-b", "实现库存统计项目", workspace.DefinitionAuthoringOwnerBound)
	firstProposal, secondProposal := stabilityProposal(t, first), stabilityProposal(t, second)
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"}
	var wg sync.WaitGroup
	for _, item := range []struct {
		d     workspace.DefinitionDerivationRequest
		p     workspace.GoalSpecification
		other string
	}{{first, firstProposal, second.Input.GoalText}, {second, secondProposal, first.Input.GoalText}} {
		for range 3 {
			wg.Go(func() {
				var prefix string
				for _, kind := range ReviewKinds() {
					command, err := AuthoringReviewCommand(item.d, item.p, kind, target)
					if err != nil {
						t.Error(err)
						return
					}
					if command.Request.WorkspaceID != string(item.d.Input.WorkspaceID) || strings.Contains(string(authoringTestPayload(t, command)), item.other) {
						t.Error("cross-workspace context")
					}
					if prefix != "" && prefix != command.Request.PromptContext.Sections[0].Content {
						t.Error("critic C0 drift")
					}
					prefix = command.Request.PromptContext.Sections[0].Content
				}
			})
		}
	}
	wg.Wait()
}

func TestRequiredPurposeV3ProductionCompilerAndReviewInputs(t *testing.T) {
	d := stabilityDerivation(t, "ws-v3-required", "保留完整原义务并完成交付", workspace.DefinitionAuthoringOwnerBound)
	p := stabilityProposal(t, d)
	if len(p.EvidencePurposeMappings) != 1 || p.EvidencePurposeMappings[0].HistoricalText == "" {
		t.Fatal("minimal provider choice did not compile full mapping")
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"}
	command, err := AuthoringProposalCommand(d, target)
	if err != nil {
		t.Fatal(err)
	}
	var proposalInput struct {
		Required []authoringRequiredEvidencePurpose `json:"requiredEvidencePurposes"`
	}
	if json.Unmarshal(authoringTestPayload(t, command), &proposalInput) != nil || len(proposalInput.Required) != 1 || len(proposalInput.Required[0].EligibleAcceptanceItemRefs) == 0 {
		t.Fatal("missing required catalog")
	}
	if !strings.Contains(command.Request.Schema, "ProfileAcceptanceSelections") {
		t.Fatal("wrong producer schema")
	}
	for _, kind := range ReviewKinds() {
		review, err := AuthoringReviewCommand(d, p, kind, target)
		if err != nil {
			t.Fatal(err)
		}
		var input struct {
			Required []authoringRequiredEvidencePurpose `json:"requiredEvidencePurposes"`
		}
		for _, section := range review.Request.PromptContext.Sections {
			if section.ID == "C2.specification-profiles" {
				if json.Unmarshal([]byte(section.Content), &input) != nil {
					t.Fatal("invalid review catalog")
				}
			}
		}
		if !reflect.DeepEqual(input.Required, proposalInput.Required) {
			t.Fatal("proposal/review required-set drift")
		}
	}
	missing := p.Clone()
	missing.EvidencePurposeMappings = nil
	if _, err := AuthoringReviewCommand(d, missing, ReviewKinds()[0], target); err == nil {
		t.Fatal("sent missing required relation for review")
	}
}
