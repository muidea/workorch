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
		Profiles              []runtimecontract.RuntimeProfileContract `json:"profiles"`
		VerificationOptions   []authoringCandidateVerificationOptions  `json:"sourceApplicableVerificationDeclarations"`
		ProfileSnapshotHashes []authoringProfileSnapshotHash           `json:"profileSnapshotHashes"`
	}{input.Profiles, input.VerificationOptions, input.ProfileSnapshotHashes}
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
