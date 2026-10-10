package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const definitionAdmissionCommandSource = "application/workorchd"

func definitionAdmissionTopics() []string {
	return []string{
		workspaceevents.TopicWorkspaceDefinitionAdmissionIntent,
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
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	cmd, ok := ev.Data().(workspaceevents.DefinitionAdmissionIntentCommand)
	if !ok {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	response, err := m.submitDefinitionAdmissionIntent(ev.Context(), cmd)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) submitDefinitionAdmissionIntent(ctx context.Context, cmd workspaceevents.DefinitionAdmissionIntentCommand) (workspaceevents.DefinitionAdmissionIntentResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	if m.manager != nil {
		m.manager.definitionMu.Lock()
		defer m.manager.definitionMu.Unlock()
	}
	if err := m.verifyDefinitionClarificationProvenance(ctx, cmd); err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, err
	}
	transition, err := m.submitDefinitionAdmissionInternalEvent(ctx, definitionAdmissionInternalEvent{
		WorkspaceID: cmd.Candidate.WorkspaceID, Intent: &cmd,
	})
	if err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, err
	}
	if transition.Result == nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, common.ErrInvalidArgument
	}
	return *transition.Result, nil
}

func currentDefinitionAdmissionCandidate(values []workspacemodel.DefinitionAdmissionCandidate, candidateID string) (*workspacemodel.DefinitionAdmissionCandidate, bool) {
	var current *workspacemodel.DefinitionAdmissionCandidate
	lineageID := ""
	seenRevisions := map[uint64]struct{}{}
	for idx := range values {
		value := values[idx].Clone()
		if lineageID == "" {
			lineageID = value.CandidateID
		}
		if value.CandidateID != lineageID {
			return nil, false
		}
		if _, duplicate := seenRevisions[value.CandidateRevision]; duplicate {
			return nil, false
		}
		seenRevisions[value.CandidateRevision] = struct{}{}
		if current == nil || value.CandidateRevision > current.CandidateRevision {
			copy := value
			current = &copy
		}
	}
	return current, current != nil && current.CandidateID == candidateID && current.CandidateRevision == uint64(len(seenRevisions))
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
	specificationHash := ""
	if candidate.Specification != nil {
		specificationHash = candidate.Specification.AdmissionHash()
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
		RuntimeProfileContractsHash:       candidate.RuntimeProfileContractsHash,
		SpecificationHash:                 specificationHash,
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory,
		CandidateItemToItemRefMappings:    mappings,
	}).WithHash()
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
