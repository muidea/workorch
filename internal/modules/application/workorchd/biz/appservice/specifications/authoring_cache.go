package specifications

import (
	"encoding/json"
	"fmt"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// partitionAuthoringReview preserves the entire input across messages. These
// are unconfirmed review data, never developer/system instructions or credit.
func partitionAuthoringReview(command llmevents.CompleteProviderCommand, input authoringReviewInput, ref string) (llmevents.CompleteProviderCommand, error) {
	snapshot := struct {
		ReviewEvidenceRefs   []string                                        `json:"reviewEvidenceRefs"`
		ProposalHash         string                                          `json:"proposalHash"`
		ResourceSnapshotHash string                                          `json:"resourceSnapshotHash"`
		Resources            []workspace.DefinitionAuthoringResourceSnapshot `json:"resources"`
		ExpectedResources    []workspace.GoalSpecificationResource           `json:"expectedResources"`
		Goal                 workspace.DefinitionCandidateItem               `json:"goal"`
		CandidateAcceptance  []workspace.DefinitionCandidateItem             `json:"candidateAcceptance"`
		Specification        workspace.GoalSpecification                     `json:"specification"`
	}{input.ReviewEvidenceRefs, input.ProposalHash, input.ResourceSnapshotHash, input.Resources, input.ExpectedResources, input.Goal, input.CandidateAcceptance, input.Specification}
	profiles := struct {
		Profiles                 []runtimecontract.RuntimeProfileContract `json:"profiles"`
		VerificationOptions      []authoringCandidateVerificationOptions  `json:"sourceApplicableVerificationDeclarations"`
		ProfileSnapshotHashes    []authoringProfileSnapshotHash           `json:"profileSnapshotHashes"`
		RequiredEvidencePurposes []authoringRequiredEvidencePurpose       `json:"requiredEvidencePurposes,omitempty"`
	}{input.Profiles, input.VerificationOptions, input.ProfileSnapshotHashes, input.RequiredEvidencePurposes}
	phase := struct {
		RequiredRequirementIDs []string `json:"requiredRequirementIDs"`
		Kind                   string   `json:"kind"`
		WireContractRef        string   `json:"wireContractRef,omitempty"`
		WireContractHash       string   `json:"wireContractHash,omitempty"`
	}{RequiredRequirementIDs: input.RequiredRequirementIDs, Kind: input.Kind, WireContractRef: ref}
	if ref != "" {
		phase.WireContractHash = workspace.AuthoringWireHash(ref, input.Kind)
	}
	common, err := json.Marshal(snapshot)
	if err != nil {
		return command, err
	}
	contracts, err := json.Marshal(profiles)
	if err != nil {
		return command, err
	}
	current, err := json.Marshal(phase)
	if err != nil {
		return command, err
	}
	if len(command.Request.PromptContext.Sections[0].Content)+len(common)+len(contracts)+len(current) > maxDocumentBytes {
		return llmevents.CompleteProviderCommand{}, fmt.Errorf("authoring review exceeds bounded context")
	}
	command.Request.PromptContext.Sections = append(command.Request.PromptContext.Sections[:1],
		llmmodels.PromptSection{ID: "C1.specification-snapshot", Name: "完整待审快照", Role: "user", Cacheability: "stable", Protected: true, DisplayOrder: 1, Content: string(common)},
		llmmodels.PromptSection{ID: "C2.specification-profiles", Name: "封存能力声明", Role: "user", Cacheability: "stable", Protected: true, DisplayOrder: 2, Content: string(contracts)},
		llmmodels.PromptSection{ID: "C3.specification-request", Name: "本次独立审查", Role: "user", Cacheability: "volatile", Protected: true, DisplayOrder: 3, Content: string(current)})
	return command, nil
}

// Proposal inputs are complete persisted source data, shared unchanged by the
// bounded revision. Feedback is added only in C4 by the revision command.
func partitionAuthoringProposal(command llmevents.CompleteProviderCommand) (llmevents.CompleteProviderCommand, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(command.Request.PromptContext.Sections[1].Content), &fields); err != nil {
		return command, err
	}
	source, contracts, phase := map[string]json.RawMessage{}, map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for key, value := range fields {
		switch key {
		case "goal", "candidateAcceptance", "inputHash", "templateHash", "resourceSnapshotHash", "resources", "expectedResources", "goalTextHash":
			source[key] = value
		case "profiles", "sourceApplicableVerificationDeclarations", "profileSnapshotHashes", "requiredEvidencePurposes":
			contracts[key] = value
		case "wireContractRef", "wireContractHash":
			phase[key] = value
		default:
			return command, fmt.Errorf("unsupported proposal input field %q", key)
		}
	}
	parts := []struct {
		id, name string
		fields   map[string]json.RawMessage
	}{
		{"C1.specification-source", "完整原始来源", source},
		{"C2.specification-profiles", "适用能力声明", contracts},
		{"C3.specification-request", "本次生成任务", phase},
	}
	command.Request.PromptContext.Sections = command.Request.PromptContext.Sections[:1]
	for i, part := range parts {
		body, err := json.Marshal(part.fields)
		if err != nil {
			return command, err
		}
		cacheability := "stable"
		if i == 2 {
			cacheability = "volatile"
		}
		command.Request.PromptContext.Sections = append(command.Request.PromptContext.Sections, llmmodels.PromptSection{ID: part.id, Name: part.name, Role: "user", Cacheability: cacheability, Protected: true, DisplayOrder: i + 1, Content: string(body)})
	}
	return command, nil
}
