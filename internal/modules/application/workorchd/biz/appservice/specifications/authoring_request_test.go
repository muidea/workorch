package specifications

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringCommandsKeepSealedIdentitiesAndIndependentCriticContext(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
	template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
	if err := json.Unmarshal([]byte(`{"profileContracts":{"sealed-profile":{"evidenceRequirements":[{"id":"*","scope":"dodItem","requiredEvidenceKinds":["validation"],"acceptedStepKinds":["shell/run"]}]}}}`), &template.ContextHints); err != nil {
		t.Fatal(err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充验收"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	record, derivation, _, err := record.ReserveDerivation("derive-1", "provider", nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	derivation = record.DerivationRequests[0]
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "test"}

	proposalCommand, err := AuthoringProposalCommand(derivation, target)
	if err != nil {
		t.Fatal(err)
	}
	if proposalCommand.Request.Kind != "workspace/specification/proposal" || proposalCommand.Target != target || proposalCommand.Request.Model != target.ModelID {
		t.Fatalf("proposal request lost its provider identity: %+v", proposalCommand)
	}
	if proposalCommand.Request.SchemaFormat != "json_schema" || !strings.Contains(proposalCommand.Request.PromptContext.Sections[0].Content, proposalCommand.Request.Schema) {
		t.Fatal("full typed schema missing from C0")
	}
	changedInput := input
	changedInput.Revision++
	changedInput.GoalText = "另一个目标"
	changedRecord := workspace.DefinitionAuthoringRecord{Input: changedInput, InputHash: changedInput.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	changedRecord, changedDerivation, _, changeErr := changedRecord.ReserveDerivation("derive-changed", "provider", nil)
	if changeErr != nil {
		t.Fatal(changeErr)
	}
	changedDerived := derived
	changedDerived.AuthoringInputHash = changedInput.InputHash()
	changedRecord, changeErr = changedRecord.CompleteDerivation(changedDerivation.PermitID, &changedDerived, "")
	if changeErr != nil {
		t.Fatal(changeErr)
	}
	changedCommand, changeErr := AuthoringProposalCommand(changedRecord.DerivationRequests[0], target)
	if changeErr != nil {
		t.Fatal(changeErr)
	}
	if changedCommand.Request.PromptContext.Sections[0].Content != proposalCommand.Request.PromptContext.Sections[0].Content || changedCommand.Request.Schema != proposalCommand.Request.Schema || changedCommand.Request.PromptContext.Sections[1].Content == proposalCommand.Request.PromptContext.Sections[1].Content {
		t.Fatal("dynamic input changed C0/schema or failed to change C3")
	}
	if len(proposalCommand.Request.PromptContext.Sections) != 2 {
		t.Fatal("proposal did not separate stable contract from volatile input")
	}
	stable := proposalCommand.Request.PromptContext.Sections[0]
	volatile := proposalCommand.Request.PromptContext.Sections[1]
	if !stable.Protected || stable.Cacheability != "stable" || !strings.HasPrefix(stable.ID, "C0.") || !volatile.Protected || volatile.Cacheability != "volatile" || !strings.HasPrefix(volatile.ID, "C3.") {
		t.Fatalf("proposal cache boundaries changed: %+v", proposalCommand.Request.PromptContext.Sections)
	}
	if strings.Contains(stable.Content, input.GoalText) || strings.Contains(stable.Content, derivation.PermitID) || strings.Contains(volatile.Content, derivation.ProviderBindingHash) || strings.Contains(volatile.Content, derivation.PermitID) {
		t.Fatal("volatile identity or provider binding leaked into provider-facing stable prefix")
	}
	var proposalPayload struct {
		Goal                workspace.DefinitionCandidateItem        `json:"goal"`
		CandidateAcceptance []workspace.DefinitionCandidateItem      `json:"candidateAcceptance"`
		InputHash           string                                   `json:"inputHash"`
		TemplateHash        string                                   `json:"templateHash"`
		Profiles            []runtimecontract.RuntimeProfileContract `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(volatile.Content), &proposalPayload); err != nil {
		t.Fatal(err)
	}
	if proposalPayload.Goal.Text != input.GoalText || proposalPayload.InputHash != input.InputHash() || proposalPayload.TemplateHash != workspace.WorkspaceTemplateSnapshotHash(&template) || len(proposalPayload.CandidateAcceptance) != 3 {
		t.Fatalf("proposal omitted sealed input: %+v", proposalPayload)
	}
	if len(proposalPayload.Profiles) != 1 || proposalPayload.Profiles[0].ID != "sealed-profile" || len(proposalPayload.Profiles[0].EvidenceRequirements) != 1 || proposalPayload.Profiles[0].EvidenceRequirements[0].ID != "*" || strings.Contains(stable.Content, "sealed-profile") {
		t.Fatal("封存 Profile 声明未精确进入 C3 或污染了 C0")
	}
	if !strings.Contains(volatile.Content, `"resourceSnapshotHash":"`+derivation.ResourceSnapshotHash+`"`) {
		t.Fatal("proposal omitted the sealed resource snapshot hash")
	}
	for _, item := range proposalPayload.CandidateAcceptance {
		if item.Text == "" || item.CandidateItemRef == "" || item.SourceIdentity == "" {
			t.Fatalf("unsealed acceptance item: %+v", item)
		}
	}

	// The critic sees the immutable proposal, not any other critic's conclusion.
	// A non-empty Reviews slice must be rejected, rather than silently stripped.
	spec := authoringCommandFixture(t, input, template, derived)
	command, err := AuthoringReviewCommand(derivation, spec, workspace.GoalReviewTraceability, target)
	if err != nil {
		t.Fatal(err)
	}
	if command.Request.Kind != "workspace/specification/review/"+workspace.GoalReviewTraceability || !strings.Contains(command.Request.PromptContext.Sections[0].Content, "FAIL") || !strings.Contains(command.Request.PromptContext.Sections[0].Content, "UNKNOWN") || !strings.Contains(command.Request.PromptContext.Sections[0].Content, "ExpectedResources") {
		t.Fatalf("critic lacks independent review and unknown-resource contract: %+v", command.Request)
	}
	var reviewPayload struct {
		Kind                string                                   `json:"kind"`
		ProposalHash        string                                   `json:"proposalHash"`
		Goal                workspace.DefinitionCandidateItem        `json:"goal"`
		CandidateAcceptance []workspace.DefinitionCandidateItem      `json:"candidateAcceptance"`
		Specification       workspace.GoalSpecification              `json:"specification"`
		Profiles            []runtimecontract.RuntimeProfileContract `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(command.Request.PromptContext.Sections[1].Content), &reviewPayload); err != nil {
		t.Fatal(err)
	}
	if reviewPayload.Kind != workspace.GoalReviewTraceability || reviewPayload.ProposalHash != spec.ContentHash() || reviewPayload.Goal.Text != input.GoalText || len(reviewPayload.CandidateAcceptance) != 3 || len(reviewPayload.Specification.Reviews) != 0 {
		t.Fatalf("critic received wrong or contaminated proposal: %+v", reviewPayload)
	}
	if runtimecontract.StableRuntimeProfileContractsHash(reviewPayload.Profiles) != runtimecontract.StableRuntimeProfileContractsHash(proposalPayload.Profiles) || strings.Contains(command.Request.PromptContext.Sections[0].Content, "sealed-profile") {
		t.Fatal("独立审查读取了不同 Profile 声明或改变了稳定前缀")
	}
	if !strings.Contains(command.Request.PromptContext.Sections[1].Content, `"resourceSnapshotHash":"`+derivation.ResourceSnapshotHash+`"`) {
		t.Fatal("critic omitted the sealed resource snapshot hash")
	}
	spec.Reviews = []workspace.GoalSpecificationCompletenessReview{{Kind: workspace.GoalReviewTraceability, Status: workspace.GoalReviewPass, SpecificationHash: spec.ContentHash()}}
	if _, err := AuthoringReviewCommand(derivation, spec, workspace.GoalReviewTraceability, target); err == nil {
		t.Fatal("critic accepted another critic's review")
	}
	if _, err := AuthoringReviewCommand(derivation, reviewPayload.Specification, "other", target); err == nil {
		t.Fatal("critic accepted unknown dimension")
	}
	derivation.Input.Revision++
	if _, err := AuthoringProposalCommand(derivation, target); err == nil {
		t.Fatal("stale derivation accepted")
	}
}

func TestResourceBoundAuthoringCommandsBindSameSnapshotAndRejectProviderResourceClaims(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	content := "验收材料正文"
	sum := sha256.Sum256([]byte(content))
	resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: input.ResourceRefs[0], Content: content, ContentHash: "sha256:" + hex.EncodeToString(sum[:])}}
	var err error
	var derivation workspace.DefinitionDerivationRequest
	record, derivation, _, err = record.ReserveDerivation("derive-1", "provider", resources)
	if err != nil {
		t.Fatal(err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "test"}
	deriveCommand, err := DerivationCommand(derivation, target)
	if err != nil {
		t.Fatal(err)
	}
	var derivePayload struct {
		Resources            []workspace.DefinitionAuthoringResourceSnapshot `json:"resources"`
		ResourceSnapshotHash string                                          `json:"resourceSnapshotHash"`
	}
	if err := json.Unmarshal([]byte(deriveCommand.Request.PromptContext.Sections[1].Content), &derivePayload); err != nil || derivePayload.ResourceSnapshotHash != derivation.ResourceSnapshotHash || len(derivePayload.Resources) != 1 || derivePayload.Resources[0].Content != content {
		t.Fatalf("derivation resource context changed: %+v %v", derivePayload, err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}}
	record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	derivation = record.DerivationRequests[0]
	proposalCommand, err := AuthoringProposalCommand(derivation, target)
	if err != nil {
		t.Fatal(err)
	}
	var proposalPayload struct {
		Resources            []workspace.DefinitionAuthoringResourceSnapshot `json:"resources"`
		ExpectedResources    []workspace.GoalSpecificationResource           `json:"expectedResources"`
		ResourceSnapshotHash string                                          `json:"resourceSnapshotHash"`
		OutputShape          workspace.GoalSpecification                     `json:"outputShape"`
	}
	if err := json.Unmarshal([]byte(proposalCommand.Request.PromptContext.Sections[1].Content), &proposalPayload); err != nil || proposalPayload.ResourceSnapshotHash != derivation.ResourceSnapshotHash || len(proposalPayload.Resources) != 1 || proposalPayload.Resources[0].Content != content || len(proposalPayload.ExpectedResources) != 1 {
		t.Fatalf("proposal resource context changed: %+v %v", proposalPayload, err)
	}
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef, GoalTextHash: workspace.StableDefinitionTextHash(input.GoalText), Resources: proposalPayload.ExpectedResources, Dimensions: []workspace.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}}
	for n, item := range append([]workspace.DefinitionCandidateItem{{CandidateItemRef: spec.GoalItemRef, Text: input.GoalText}}, items...) {
		ref := items[0].CandidateItemRef
		kind := workspace.GoalRequirementDerived
		if n == 0 {
			kind = workspace.GoalRequirementUserExplicit
		}
		id := "requirement-" + string(rune('a'+n))
		spec.Requirements = append(spec.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "dimension", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, ResourceIDs: []string{spec.Resources[0].ID}, Text: item.Text})
		spec.Obligations = append(spec.Obligations, workspace.GoalSpecificationObligation{ID: "obligation-" + id, RequirementID: id, DoDItemRefs: []string{ref}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, workspace.GoalSpecificationVerification{ID: "verification-" + id, ObligationID: "obligation-" + id, Status: workspace.GoalVerificationUnverified, Declaration: "待评估"})
	}
	if err := workspace.ValidateAuthoringSpecification(input, &template, derived, resources, spec); err != nil {
		t.Fatal(err)
	}
	reviewCommand, err := AuthoringReviewCommand(derivation, spec, workspace.GoalReviewResourceEnvironment, target)
	if err != nil {
		t.Fatal(err)
	}
	var reviewPayload struct {
		Resources            []workspace.DefinitionAuthoringResourceSnapshot `json:"resources"`
		ExpectedResources    []workspace.GoalSpecificationResource           `json:"expectedResources"`
		ResourceSnapshotHash string                                          `json:"resourceSnapshotHash"`
		Specification        workspace.GoalSpecification                     `json:"specification"`
	}
	if err := json.Unmarshal([]byte(reviewCommand.Request.PromptContext.Sections[1].Content), &reviewPayload); err != nil || reviewPayload.ResourceSnapshotHash != derivation.ResourceSnapshotHash || len(reviewPayload.Resources) != 1 || reviewPayload.Resources[0].Content != content || len(reviewPayload.Specification.Reviews) != 0 {
		t.Fatalf("critic resource context changed: %+v %v", reviewPayload, err)
	}
	if len(reviewPayload.ExpectedResources) != len(proposalPayload.ExpectedResources) || len(reviewPayload.ExpectedResources) != 1 || reviewPayload.ExpectedResources[0] != proposalPayload.ExpectedResources[0] || reviewPayload.ExpectedResources[0] != reviewPayload.Specification.Resources[0] {
		t.Fatal("critic 未读取同一 owner 派生的 ExpectedResources，不能只在指令中引用不存在的输入")
	}
	for _, kind := range ReviewKinds() {
		critic, err := AuthoringReviewCommand(derivation, spec, kind, target)
		if err != nil {
			t.Fatal(err)
		}
		var input struct {
			ExpectedResources []workspace.GoalSpecificationResource `json:"expectedResources"`
			Specification     workspace.GoalSpecification           `json:"specification"`
			ProposalHash      string                                `json:"proposalHash"`
		}
		if err := json.Unmarshal([]byte(critic.Request.PromptContext.Sections[1].Content), &input); err != nil {
			t.Fatal(err)
		}
		if len(input.ExpectedResources) != 1 || input.ExpectedResources[0] != proposalPayload.ExpectedResources[0] || len(input.Specification.Reviews) != 0 || input.ProposalHash != spec.ContentHash() {
			t.Fatalf("独立 critic %s 读取不同资源、其他结论或非 exact proposal", kind)
		}
		if !strings.Contains(critic.Request.PromptContext.Sections[0].Content, "审查 Specification.Resources 是否精确等于 ExpectedResources") || strings.Contains(critic.Request.PromptContext.Sections[0].Content, "Verification 使用 UNVERIFIED") {
			t.Fatal("review 指令不能要求修改不可变 proposal 或回显不属于 review wire 的字段")
		}
	}
	changed := spec.Clone()
	changed.Resources[0].Available = false
	if _, err := AuthoringReviewCommand(derivation, changed, workspace.GoalReviewResourceEnvironment, target); err == nil {
		t.Fatal("provider changed the File-owner availability fact")
	}
	derivation.Resources[0].Content = "另一个正文"
	if _, err := AuthoringProposalCommand(derivation, target); err == nil {
		t.Fatal("stale content with old hash reached provider request")
	}
}

func authoringCommandFixture(t *testing.T, input workspace.DefinitionAuthoringInput, template workspace.WorkspaceTemplate, derived workspace.DefinitionDerivedAcceptance) workspace.GoalSpecification {
	t.Helper()
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: workspace.DefinitionAuthoringGoalItemRef, GoalTextHash: workspace.StableDefinitionTextHash(input.GoalText), Dimensions: []workspace.GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}}
	for n, item := range append([]workspace.DefinitionCandidateItem{{CandidateItemRef: spec.GoalItemRef, Text: input.GoalText}}, items...) {
		kind := workspace.GoalRequirementDerived
		switch n {
		case 0, 1:
			kind = workspace.GoalRequirementUserExplicit
		case 2:
			kind = workspace.GoalRequirementTemplateRequired
		}
		id := "requirement-" + string(rune('a'+n))
		spec.Requirements = append(spec.Requirements, workspace.GoalSpecificationRequirement{ID: id, DimensionID: "dimension", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		dodRef := item.CandidateItemRef
		if n == 0 {
			dodRef = items[0].CandidateItemRef
		}
		spec.Obligations = append(spec.Obligations, workspace.GoalSpecificationObligation{ID: "obligation-" + id, RequirementID: id, DoDItemRefs: []string{dodRef}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, workspace.GoalSpecificationVerification{ID: "verification-" + id, ObligationID: "obligation-" + id, Status: workspace.GoalVerificationUnverified, Declaration: "待评估"})
	}
	if err := workspace.ValidateAuthoringSpecification(input, &template, derived, nil, spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestSpecificationRequestBoundIncludesStableContract(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "model"}
	payload := []byte(strings.Repeat("x", maxDocumentBytes-len(authoringProposalInstructions)))
	if _, err := authoringSpecificationCommand("ws", target, "proposal", authoringProposalInstructions, payload); err == nil {
		t.Fatal("schema bytes omitted from bounded context")
	}
	if _, err := completionCommand(workspace.DefinitionAdmissionCandidate{}, "proposal", proposalInstructions, payload); err == nil {
		t.Fatal("legacy builder omitted schema bytes from bounded context")
	}
}
