package workspace

import (
	"fmt"
	"reflect"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 修改只作用于独立待审输入，不清空当前 seal/journal 或覆盖正式 Goal/DoD。
func reduceDefinitionChangePreparation(o definitionSpecificationObservation, cmd workspaceevents.PrepareDefinitionChangeCommand, proposal decisionmodel.ContractChangeProposal) (definitionSpecificationTransition, error) {
	w := o.workspace
	if cmd.Validate() != nil || proposal.Validate() != nil || w == nil || w.ID != cmd.WorkspaceID || proposal.WorkspaceID != w.ID || proposal.RunID != "" ||
		proposal.Hash() != cmd.ProposalHash || w.DefinitionAuthoring == nil || w.ActiveRunID != "" || w.DefinitionSpecificationLedger != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return definitionSpecificationTransition{}, err
	}
	if _, _, err := w.ValidateActivationAdmissionPersistence(); err != nil {
		return definitionSpecificationTransition{}, err
	}
	if current := w.DefinitionChangeCandidate; current != nil {
		if current.Validate(w) != nil || current.RequestID != cmd.RequestID || current.ExpectedWorkspaceRevision != cmd.ExpectedWorkspaceRevision ||
			current.TicketID != cmd.TicketID || current.ProposalRevision != cmd.ProposalRevision || current.ProposalHash != cmd.ProposalHash {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		return definitionSpecificationTransition{DefinitionChange: definitionChangePreparationResult(w, *current)}, nil
	}
	if w.Revision != cmd.ExpectedWorkspaceRevision || o.template == nil || workspace.WorkspaceTemplateSnapshotHash(o.template) != w.DefinitionAuthoring.TemplateSnapshotHash ||
		w.DefinitionAuthoringConfirmation != nil && w.DefinitionAuthoringConfirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if proposal.BaseInputRevision != 0 && (proposal.BaseInputRevision != w.DefinitionAuthoring.Input.Revision || proposal.BaseInputHash != w.DefinitionAuthoring.InputHash) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if proposal.BaseDefinitionRevision != "" && (w.StartupInput == nil || proposal.BaseDefinitionRevision != w.StartupInput.DefinitionRevision || proposal.BaseStartupInputHash != w.StartupInput.StartupInputHash) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	// 当前准备合同不能冻结尚待结算的原 permit；拒绝准备不消费或抹去原 unknown。
	for _, request := range w.DefinitionAuthoring.DerivationRequests {
		if request.Proposal == nil && request.FailureCode == "" {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	}
	for _, request := range w.DefinitionAuthoring.SpecificationRequests {
		if request.Proposal == nil && request.Review == nil && request.FailureCode == "" {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	}
	input, err := definitionChangeInput(w.DefinitionAuthoring.Input, proposal.Differences)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	// 原 permit、unknown、失败和诊断继承到同一有界 ledger，不重开预算。
	record := w.DefinitionAuthoring.Clone()
	record.Input, record.InputHash = input, input.InputHash()
	candidate := workspace.DefinitionChangeCandidate{
		RequestID: cmd.RequestID, ExpectedWorkspaceRevision: cmd.ExpectedWorkspaceRevision, RecordedWorkspaceRevision: w.Revision + 1,
		TicketID: cmd.TicketID, ProposalRevision: cmd.ProposalRevision, ProposalHash: cmd.ProposalHash,
		BaseInputHash: w.DefinitionAuthoring.InputHash, Record: record,
	}
	if w.StartupInput != nil {
		candidate.BaseDefinitionRevision, candidate.BaseStartupInputHash = w.StartupInput.DefinitionRevision, w.StartupInput.StartupInputHash
	}
	next := *w
	next.Revision = candidate.RecordedWorkspaceRevision
	next.DefinitionChangeCandidate = &candidate
	if candidate.Validate(&next) != nil {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	return definitionSpecificationTransition{Next: &next, DefinitionChange: definitionChangePreparationResult(&next, candidate)}, nil
}

func definitionChangeInput(base workspace.DefinitionAuthoringInput, differences []decisionmodel.ContractChangeDifference) (workspace.DefinitionAuthoringInput, error) {
	if base.Validate() != nil || base.Revision == ^uint64(0) {
		return workspace.DefinitionAuthoringInput{}, common.ErrConflict
	}
	input := base.Clone()
	input.Revision++
	removed := make(map[int]bool)
	added := []string{}
	for _, d := range differences {
		if d.Kind == "goal" && d.ItemID == workspace.DefinitionAuthoringGoalItemRef && d.Action == "modify" && d.OriginalText == base.GoalText {
			input.GoalText = d.ProposedText
			continue
		}
		if d.Kind != "criterion" {
			return workspace.DefinitionAuthoringInput{}, fmt.Errorf("%w: candidate preparation cannot map difference %q to original input", common.ErrConflict, d.ItemID)
		}
		if d.Action == "add" && d.ItemID == fmt.Sprintf("derived-dod:user:%06d", len(base.AcceptanceRequirements)+len(added)+1) && d.OriginalText == "" {
			added = append(added, d.ProposedText)
			continue
		}
		matched := false
		for n, text := range base.AcceptanceRequirements {
			if d.ItemID != fmt.Sprintf("derived-dod:user:%06d", n+1) || d.OriginalText != text {
				continue
			}
			switch d.Action {
			case "modify":
				input.AcceptanceRequirements[n] = d.ProposedText
			case "remove":
				removed[n] = true
			default:
				return workspace.DefinitionAuthoringInput{}, common.ErrConflict
			}
			matched = true
			break
		}
		if !matched {
			return workspace.DefinitionAuthoringInput{}, fmt.Errorf("%w: difference %q has no exact original user criterion", common.ErrConflict, d.ItemID)
		}
	}
	if len(removed) != 0 {
		items := []string{}
		for n, text := range input.AcceptanceRequirements {
			if !removed[n] {
				items = append(items, text)
			}
		}
		input.AcceptanceRequirements = items
	}
	input.AcceptanceRequirements = append(input.AcceptanceRequirements, added...)
	if input.Validate() != nil || input.GoalText == base.GoalText && reflect.DeepEqual(input.AcceptanceRequirements, base.AcceptanceRequirements) {
		return workspace.DefinitionAuthoringInput{}, common.ErrConflict
	}
	return input, nil
}

func definitionChangePreparationResult(w *workspace.Workspace, c workspace.DefinitionChangeCandidate) *workspaceevents.PrepareDefinitionChangeResult {
	return &workspaceevents.PrepareDefinitionChangeResult{WorkspaceID: w.ID, WorkspaceRevision: w.Revision, Candidate: workspaceevents.DefinitionChangeCandidate{
		RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision, RecordedWorkspaceRevision: c.RecordedWorkspaceRevision,
		TicketID: c.TicketID, ProposalRevision: c.ProposalRevision, ProposalHash: c.ProposalHash,
		BaseDefinitionRevision: c.BaseDefinitionRevision, BaseStartupInputHash: c.BaseStartupInputHash, BaseInputHash: c.BaseInputHash,
		Record: workspaceeventmapping.DefinitionAuthoringRecordDTO(c.Record),
	}}
}
