package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

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
	candidate := cmd.Candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, candidate.WorkspaceID)
	if err != nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		workspace, err := m.workspaces.Get(bound, candidate.WorkspaceID)
		if err != nil {
			return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, err
		}
		currentRevision := uint64(0)
		for _, existing := range workspace.DefinitionAdmissionCandidates {
			if existing.CandidateID != candidate.CandidateID {
				continue
			}
			if existing.CandidateRevision == candidate.CandidateRevision {
				if existing.CandidateHash != candidate.CandidateHash {
					return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, common.ErrConflict
				}
				return workspaceevents.SubmitDefinitionAdmissionCandidateResult{Candidate: existing.Clone(), Disposition: workspaceevents.DefinitionAdmissionCandidateDuplicate}, nil
			}
			if existing.CandidateRevision > currentRevision {
				currentRevision = existing.CandidateRevision
			}
		}
		if cmd.ExpectedCandidateRevision != currentRevision || candidate.CandidateRevision != currentRevision+1 {
			return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, common.ErrConflict
		}
		next := *workspace
		next.DefinitionAdmissionCandidates = append(cloneDefinitionAdmissionCandidates(workspace.DefinitionAdmissionCandidates), candidate.Clone())
		if err := m.workspaces.CompareAndSwap(bound, &next, workspace.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, err
		}
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{Candidate: candidate.Clone(), Disposition: workspaceevents.DefinitionAdmissionCandidateCreated}, nil
	}
	return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, common.ErrConflict
}

func (m *Biz) promoteDefinitionAdmissionCandidate(ctx context.Context, cmd workspaceevents.PromoteDefinitionAdmissionCandidateCommand) (workspaceevents.PromoteDefinitionAdmissionCandidateResult, error) {
	cmd.CandidateID = strings.TrimSpace(cmd.CandidateID)
	cmd.CandidateHash = strings.TrimSpace(cmd.CandidateHash)
	if cmd.WorkspaceID == "" || cmd.CandidateID == "" || cmd.CandidateRevision == 0 || cmd.CandidateHash == "" || cmd.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		workspace, err := m.workspaces.Get(bound, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, err
		}
		candidate, current := currentDefinitionAdmissionCandidate(workspace.DefinitionAdmissionCandidates, cmd.CandidateID)
		if candidate == nil || !current || candidate.CandidateRevision != cmd.CandidateRevision || candidate.CandidateHash != cmd.CandidateHash {
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrConflict
		}
		if err := cmd.AdmissionResult.ValidateFor(*candidate); err != nil {
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: %v", common.ErrConflict, err)
		}
		if workspace.StartupInput != nil || workspace.ActivationAdmission != nil {
			if samePromotion(workspace, *candidate, cmd.AdmissionResult, cmd.BlockingTicketID) {
				return workspaceevents.PromoteDefinitionAdmissionCandidateResult{
					DefinitionRevision:                workspace.StartupInput.DefinitionRevision,
					StartupInputHash:                  workspace.StartupInput.StartupInputHash,
					CandidateItemToItemRefMappingHash: workspace.StartupInput.CandidateItemToItemRefMappingHash,
					StartupInput:                      workspace.StartupInput.Clone(), ActivationAdmission: workspace.ActivationAdmission.Normalized(), Duplicate: true,
				}, nil
			}
			if candidate.BaseDefinitionRevision == "" || candidate.BaseStartupInputHash == "" || workspace.StartupInput == nil || candidate.BaseDefinitionRevision != workspace.StartupInput.DefinitionRevision || candidate.BaseStartupInputHash != workspace.StartupInput.StartupInputHash {
				return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrConflict
			}
		}
		startup := startupInputFromCandidate(*candidate, cmd.AdmissionResult)
		if err := startup.Validate(); err != nil {
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		admission := workspacemodel.WorkspaceActivationAdmission{
			WorkspaceID: cmd.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
			DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
		}
		if cmd.BlockingTicketID != "" {
			admission.State = workspacemodel.WorkspaceActivationPendingClarificationResolution
			admission.BlockingTicketID = cmd.BlockingTicketID
		}
		if err := admission.Validate(startup); err != nil {
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		next := *workspace
		next.StartupInput = &startup
		next.ActivationAdmission = &admission
		if err := m.workspaces.CompareAndSwap(bound, &next, workspace.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, err
		}
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{
			DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
			CandidateItemToItemRefMappingHash: startup.CandidateItemToItemRefMappingHash,
			StartupInput:                      startup.Clone(), ActivationAdmission: admission.Normalized(),
		}, nil
	}
	return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrConflict
}

func (m *Biz) acknowledgeWorkspaceDefinitionClarification(ctx context.Context, cmd workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand) (workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult, error) {
	cmd.CandidateID = strings.TrimSpace(cmd.CandidateID)
	cmd.CandidateHash = strings.TrimSpace(cmd.CandidateHash)
	cmd.ResolutionBindingHash = strings.TrimSpace(cmd.ResolutionBindingHash)
	cmd.DefinitionRevision = strings.TrimSpace(cmd.DefinitionRevision)
	cmd.StartupInputHash = strings.TrimSpace(cmd.StartupInputHash)
	if cmd.WorkspaceID == "" || cmd.CandidateID == "" || cmd.CandidateRevision == 0 || cmd.CandidateHash == "" || cmd.BlockingTicketID == "" || cmd.ResolutionID == "" || cmd.ResolutionBindingHash == "" || cmd.DefinitionRevision == "" || cmd.StartupInputHash == "" {
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		workspace, err := m.workspaces.Get(bound, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, err
		}
		if workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
		}
		current := workspace.ActivationAdmission.Normalized()
		if current.WorkspaceID != cmd.WorkspaceID || current.CandidateID != cmd.CandidateID || current.CandidateRevision != cmd.CandidateRevision || current.CandidateHash != cmd.CandidateHash || current.BlockingTicketID != cmd.BlockingTicketID || current.DefinitionRevision != cmd.DefinitionRevision || current.StartupInputHash != cmd.StartupInputHash {
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
		}
		if current.State == workspacemodel.WorkspaceActivationEligible {
			if current.ClarificationResolutionID == cmd.ResolutionID && current.ClarificationResolutionBindingHash == cmd.ResolutionBindingHash {
				return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{ActivationAdmission: current, Transitioned: false}, nil
			}
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
		}
		if current.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
		}
		current.State = workspacemodel.WorkspaceActivationEligible
		current.ClarificationResolutionID = cmd.ResolutionID
		current.ClarificationResolutionBindingHash = cmd.ResolutionBindingHash
		if err := current.Validate(*workspace.StartupInput); err != nil {
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
		}
		next := *workspace
		next.ActivationAdmission = &current
		if err := m.workspaces.CompareAndSwap(bound, &next, workspace.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, err
		}
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{ActivationAdmission: current, Transitioned: true}, nil
	}
	return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrConflict
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
