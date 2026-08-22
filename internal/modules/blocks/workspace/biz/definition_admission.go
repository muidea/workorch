package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const definitionAdmissionCommandSource = "application/workorchd"

func definitionAdmissionTopics() []string {
	return []string{
		workspaceevents.TopicWorkspaceCommandSubmitDefinitionAdmissionCandidate,
		workspaceevents.TopicWorkspaceCommandPromoteDefinitionAdmissionCandidate,
		workspaceevents.TopicWorkspaceCommandAcknowledgeDefinitionClarification,
	}
}

func (m *Biz) handleDefinitionAdmissionCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if m == nil || m.workspaces == nil || ev == nil || ev.Source() != definitionAdmissionCommandSource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	var response any
	var err error
	switch cmd := ev.Data().(type) {
	case workspaceevents.SubmitDefinitionAdmissionCandidateCommand:
		response, err = m.submitDefinitionAdmissionCandidate(ev.Context(), cmd)
	case workspaceevents.PromoteDefinitionAdmissionCandidateCommand:
		response, err = m.promoteDefinitionAdmissionCandidate(ev.Context(), cmd)
	case workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand:
		response, err = m.acknowledgeWorkspaceDefinitionClarification(ev.Context(), cmd)
	default:
		err = common.ErrInvalidArgument
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) submitDefinitionAdmissionCandidate(ctx context.Context, cmd workspaceevents.SubmitDefinitionAdmissionCandidateCommand) (workspaceevents.SubmitDefinitionAdmissionCandidateResult, error) {
	transition, err := m.submitDefinitionAdmissionInternalEvent(ctx, definitionAdmissionInternalEvent{
		Kind: definitionAdmissionInternalEventCandidateSubmitted, WorkspaceID: cmd.Candidate.WorkspaceID, CandidateSubmission: &cmd,
	})
	if err != nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, err
	}
	if transition.CandidateSubmissionResult == nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, common.ErrInvalidArgument
	}
	return *transition.CandidateSubmissionResult, nil
}

func (m *Biz) promoteDefinitionAdmissionCandidate(ctx context.Context, cmd workspaceevents.PromoteDefinitionAdmissionCandidateCommand) (workspaceevents.PromoteDefinitionAdmissionCandidateResult, error) {
	transition, err := m.submitDefinitionAdmissionInternalEvent(ctx, definitionAdmissionInternalEvent{
		Kind: definitionAdmissionInternalEventCandidatePromoted, WorkspaceID: cmd.WorkspaceID, CandidatePromotion: &cmd,
	})
	if err != nil {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, err
	}
	if transition.CandidatePromotionResult == nil {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrInvalidArgument
	}
	return *transition.CandidatePromotionResult, nil
}

func (m *Biz) acknowledgeWorkspaceDefinitionClarification(ctx context.Context, cmd workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand) (workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult, error) {
	transition, err := m.submitDefinitionAdmissionInternalEvent(ctx, definitionAdmissionInternalEvent{
		Kind: definitionAdmissionInternalEventClarificationResolutionObserved, WorkspaceID: cmd.WorkspaceID, ClarificationResolution: &cmd,
	})
	if err != nil {
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, err
	}
	if transition.ClarificationResolutionResult == nil {
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrInvalidArgument
	}
	return *transition.ClarificationResolutionResult, nil
}

func currentDefinitionAdmissionCandidate(values []workspacemodel.DefinitionAdmissionCandidate, candidateID string) (*workspacemodel.DefinitionAdmissionCandidate, bool) {
	var current *workspacemodel.DefinitionAdmissionCandidate
	var globalRevision uint64
	var globalID string
	for idx := range values {
		value := values[idx].Clone()
		if value.CandidateRevision > globalRevision || (value.CandidateRevision == globalRevision && value.CandidateID > globalID) {
			globalRevision, globalID = value.CandidateRevision, value.CandidateID
		}
		if value.CandidateID == candidateID && (current == nil || value.CandidateRevision > current.CandidateRevision) {
			copy := value
			current = &copy
		}
	}
	return current, current != nil && current.CandidateRevision == globalRevision && current.CandidateID == globalID
}

func startupInputFromCandidate(candidate workspacemodel.DefinitionAdmissionCandidate, result workspacemodel.DefinitionAdmissionResult) workspacemodel.WorkspaceStartupInputContract {
	candidate = candidate.Normalized()
	result = result.Normalized()
	mappings := make([]workspacemodel.CandidateItemToItemRefMapping, 0, 1+len(candidate.DoDInputs))
	mappings = append(mappings, workspacemodel.CandidateItemToItemRefMapping{CandidateItemRef: candidate.GoalInput.CandidateItemRef, ItemRef: "goal-item:" + stableAdmissionSuffix(candidate.CandidateID+"\x00"+candidate.GoalInput.CandidateItemRef)})
	dodRefs := make([]string, 0, len(candidate.DoDInputs))
	dodTexts := make([]string, 0, len(candidate.DoDInputs))
	for _, item := range candidate.DoDInputs {
		ref := "dod-item:" + stableAdmissionSuffix(candidate.CandidateID+"\x00"+item.CandidateItemRef)
		mappings = append(mappings, workspacemodel.CandidateItemToItemRefMapping{CandidateItemRef: item.CandidateItemRef, ItemRef: ref})
		dodRefs = append(dodRefs, ref)
		dodTexts = append(dodTexts, item.Text)
	}
	definitionRevision := candidate.GoalInput.SourceIdentity
	if len(candidate.DoDInputs) > 0 {
		definitionRevision += "|" + candidate.DoDInputs[0].SourceIdentity
	}
	return (workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID:                       candidate.WorkspaceID,
		DefinitionRevision:                definitionRevision,
		GoalDefinitionRef:                 mappings[0].ItemRef,
		GoalTextHash:                      workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		DoDDefinitionRefs:                 dodRefs,
		DoDItemSetHash:                    workspacemodel.StableDefinitionStringSetHash(dodTexts),
		WorkspaceTemplateRef:              candidate.WorkspaceTemplateSnapshot.TemplateRef,
		TemplateRevision:                  candidate.WorkspaceTemplateSnapshot.Revision,
		TemplateSnapshotHash:              candidate.WorkspaceTemplateSnapshot.SnapshotHash,
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory,
		CandidateItemToItemRefMappings:    mappings,
	}).WithHash()
}

func samePromotion(workspace *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate, result workspacemodel.DefinitionAdmissionResult, blockingTicketID common.TicketID) bool {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return false
	}
	expected := startupInputFromCandidate(candidate, result)
	current := workspace.ActivationAdmission.Normalized()
	return workspace.StartupInput.StartupInputHash == expected.StartupInputHash && current.CandidateID == candidate.CandidateID && current.CandidateRevision == candidate.CandidateRevision && current.CandidateHash == candidate.CandidateHash && current.BlockingTicketID == blockingTicketID
}

func cloneDefinitionAdmissionCandidates(values []workspacemodel.DefinitionAdmissionCandidate) []workspacemodel.DefinitionAdmissionCandidate {
	if values == nil {
		return nil
	}
	ret := make([]workspacemodel.DefinitionAdmissionCandidate, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func stableAdmissionSuffix(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])[:24]
}
