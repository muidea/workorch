package eventmapping

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func stepViewDTO(item *projectioncontract.StepView) (*workorchdevents.CLIStepViewInfo, error) {
	if item == nil {
		return nil, nil
	}
	input, err := workorcheventpayload.FromMap(item.Input)
	if err != nil {
		return nil, err
	}
	output, err := workorcheventpayload.FromMap(item.Output)
	if err != nil {
		return nil, err
	}
	audit, err := workorcheventpayload.FromMap(item.ContractAudit)
	if err != nil {
		return nil, err
	}
	return &workorchdevents.CLIStepViewInfo{ID: item.ID, Kind: item.Kind, State: item.State, Input: input, Output: output, ContractAudit: audit}, nil
}

func stepViewProjection(item *workorchdevents.CLIStepViewInfo) *projectioncontract.StepView {
	if item == nil {
		return nil
	}
	return &projectioncontract.StepView{ID: item.ID, Kind: item.Kind, State: item.State, Input: item.Input.Map(), Output: item.Output.Map(), ContractAudit: item.ContractAudit.Map()}
}

func gapBaselineDTO(item *projectioncontract.GapBaseline) (*workorchdevents.CLIGapBaselineInfo, error) {
	if item == nil {
		return nil, nil
	}
	focus, err := workorcheventpayload.FromMap(item.CurrentFocus)
	if err != nil {
		return nil, err
	}
	coverage, err := workorcheventpayload.FromMapSlice(item.OpenCoverageWorkItems)
	if err != nil {
		return nil, err
	}
	ret := &workorchdevents.CLIGapBaselineInfo{
		WorkspaceID: item.WorkspaceID, RunID: item.RunID, DefinitionRevision: item.DefinitionRevision, BaselineRevision: item.BaselineRevision,
		CurrentFocus: focus, OpenCoverageWorkItems: coverage, AcceptedEvidenceRefs: append([]string(nil), item.AcceptedEvidenceRefs...),
		RejectedEvidenceRefs: append([]string(nil), item.RejectedEvidenceRefs...),
		Diagnostics:          projectioncontract.CloneGapDiagnostics(item.Diagnostics),
	}
	if item.TodoItems != nil {
		ret.TodoItems = make([]workorchdevents.CLIGapTodoItemInfo, 0, len(item.TodoItems))
	}
	for _, todo := range item.TodoItems {
		metadata, mapErr := workorcheventpayload.FromMap(todo.Metadata)
		if mapErr != nil {
			return nil, mapErr
		}
		ret.TodoItems = append(ret.TodoItems, workorchdevents.CLIGapTodoItemInfo{
			ID: todo.ID, Source: todo.Source, SourceRef: todo.SourceRef,
			CoverageWorkItemIDs: append([]string(nil), todo.CoverageWorkItemIDs...), RequiredEvidenceKinds: append([]string(nil), todo.RequiredEvidenceKinds...),
			Status: todo.Status, Priority: todo.Priority, Reason: todo.Reason, Metadata: metadata,
		})
	}
	return ret, nil
}

func gapBaselineProjection(item *workorchdevents.CLIGapBaselineInfo) *projectioncontract.GapBaseline {
	if item == nil {
		return nil
	}
	ret := &projectioncontract.GapBaseline{
		WorkspaceID: item.WorkspaceID, RunID: item.RunID, DefinitionRevision: item.DefinitionRevision, BaselineRevision: item.BaselineRevision,
		CurrentFocus: item.CurrentFocus.Map(), OpenCoverageWorkItems: workorcheventpayload.MapSlice(item.OpenCoverageWorkItems),
		AcceptedEvidenceRefs: append([]string(nil), item.AcceptedEvidenceRefs...), RejectedEvidenceRefs: append([]string(nil), item.RejectedEvidenceRefs...),
		Diagnostics: projectioncontract.CloneGapDiagnostics(item.Diagnostics),
	}
	if item.TodoItems != nil {
		ret.TodoItems = make([]projectioncontract.GapTodoItem, 0, len(item.TodoItems))
	}
	for _, todo := range item.TodoItems {
		ret.TodoItems = append(ret.TodoItems, projectioncontract.GapTodoItem{
			ID: todo.ID, Source: todo.Source, SourceRef: todo.SourceRef,
			CoverageWorkItemIDs: append([]string(nil), todo.CoverageWorkItemIDs...), RequiredEvidenceKinds: append([]string(nil), todo.RequiredEvidenceKinds...),
			Status: todo.Status, Priority: todo.Priority, Reason: todo.Reason, Metadata: todo.Metadata.Map(),
		})
	}
	return ret
}
