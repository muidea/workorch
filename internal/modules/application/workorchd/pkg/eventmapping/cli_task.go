package eventmapping

import (
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func taskNodeDTOs(items []projectioncontract.TaskDecompositionNode) ([]workorchdevents.CLITaskDecompositionNodeInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLITaskDecompositionNodeInfo, 0, len(items))
	for _, item := range items {
		input, err := workorcheventpayload.FromMap(item.Input)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLITaskDecompositionNodeInfo{
			ID: item.ID, Kind: item.Kind, Title: item.Title, State: item.State, DependsOn: append([]string(nil), item.DependsOn...),
			Branch: item.Branch, StepID: item.StepID, StepState: item.StepState, RetryOf: item.RetryOf, RetryAttempt: item.RetryAttempt,
			FailureClass: item.FailureClass, FailureSignature: item.FailureSignature, TargetScope: item.TargetScope,
			BlockingTicketID: item.BlockingTicketID, EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			CoverageTargets: append([]string(nil), item.CoverageTargets...), Input: input,
		})
	}
	return ret, nil
}

func taskNodeProjections(items []workorchdevents.CLITaskDecompositionNodeInfo) []projectioncontract.TaskDecompositionNode {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.TaskDecompositionNode, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.TaskDecompositionNode{
			ID: item.ID, Kind: item.Kind, Title: item.Title, State: item.State, DependsOn: append([]string(nil), item.DependsOn...),
			Branch: item.Branch, StepID: item.StepID, StepState: item.StepState, RetryOf: item.RetryOf, RetryAttempt: item.RetryAttempt,
			FailureClass: item.FailureClass, FailureSignature: item.FailureSignature, TargetScope: item.TargetScope,
			BlockingTicketID: item.BlockingTicketID, EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
			CoverageTargets: append([]string(nil), item.CoverageTargets...), Input: item.Input.Map(),
		})
	}
	return ret
}

func taskDecompositionDTOs(items []projectioncontract.TaskDecomposition) ([]workorchdevents.CLITaskDecompositionInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLITaskDecompositionInfo, 0, len(items))
	for _, item := range items {
		nodes, err := taskNodeDTOs(item.Nodes)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLITaskDecompositionInfo{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, RootStepID: item.RootStepID, State: item.State,
			Summary: item.Summary, GraphBranch: item.GraphBranch, NodeTotal: item.NodeTotal, NodeCompleted: item.NodeCompleted,
			NodeFailed: item.NodeFailed, NodeRunning: item.NodeRunning, Nodes: nodes, EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret, nil
}

func taskDecompositionProjections(items []workorchdevents.CLITaskDecompositionInfo) []projectioncontract.TaskDecomposition {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.TaskDecomposition, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.TaskDecomposition{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, RootStepID: item.RootStepID, State: item.State,
			Summary: item.Summary, GraphBranch: item.GraphBranch, NodeTotal: item.NodeTotal, NodeCompleted: item.NodeCompleted,
			NodeFailed: item.NodeFailed, NodeRunning: item.NodeRunning, Nodes: taskNodeProjections(item.Nodes), EvidenceRefs: append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func taskPlanDTOs(items []projectioncontract.TaskPlan) ([]workorchdevents.CLITaskPlanInfo, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLITaskPlanInfo, 0, len(items))
	for _, item := range items {
		nodes, err := taskNodeDTOs(item.Nodes)
		if err != nil {
			return nil, err
		}
		ret = append(ret, workorchdevents.CLITaskPlanInfo{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, AnchorStepID: item.AnchorStepID, RevisionStepID: item.RevisionStepID,
			Revision: item.Revision, State: item.State, Stage: item.Stage, SourceOutputState: item.SourceOutputState, Summary: item.Summary,
			GraphBranch: item.GraphBranch, NodeTotal: item.NodeTotal, NodeCompleted: item.NodeCompleted, NodeFailed: item.NodeFailed,
			NodeRunning: item.NodeRunning, Nodes: nodes, RetryOf: item.RetryOf, RetryAttempt: item.RetryAttempt,
			FailureClass: item.FailureClass, FailureSignature: item.FailureSignature, TargetScope: item.TargetScope,
			BlockingTicketID: item.BlockingTicketID, EvidenceRefs: append([]string(nil), item.EvidenceRefs...), OccurredAt: item.OccurredAt,
			SourceProjection: item.SourceProjection, SealedLLMTarget: runtimecontract.CloneProviderModelTarget(item.SealedLLMTarget),
			RuntimeActivation: cloneRuntimeActivation(item.RuntimeActivation),
		})
	}
	return ret, nil
}

func taskPlanProjections(items []workorchdevents.CLITaskPlanInfo) []projectioncontract.TaskPlan {
	if items == nil {
		return nil
	}
	ret := make([]projectioncontract.TaskPlan, 0, len(items))
	for _, item := range items {
		ret = append(ret, projectioncontract.TaskPlan{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, AnchorStepID: item.AnchorStepID, RevisionStepID: item.RevisionStepID,
			Revision: item.Revision, State: item.State, Stage: item.Stage, SourceOutputState: item.SourceOutputState, Summary: item.Summary,
			GraphBranch: item.GraphBranch, NodeTotal: item.NodeTotal, NodeCompleted: item.NodeCompleted, NodeFailed: item.NodeFailed,
			NodeRunning: item.NodeRunning, Nodes: taskNodeProjections(item.Nodes), RetryOf: item.RetryOf, RetryAttempt: item.RetryAttempt,
			FailureClass: item.FailureClass, FailureSignature: item.FailureSignature, TargetScope: item.TargetScope,
			BlockingTicketID: item.BlockingTicketID, EvidenceRefs: append([]string(nil), item.EvidenceRefs...), OccurredAt: item.OccurredAt,
			SourceProjection: item.SourceProjection, SealedLLMTarget: runtimecontract.CloneProviderModelTarget(item.SealedLLMTarget),
			RuntimeActivation: cloneRuntimeActivation(item.RuntimeActivation),
		})
	}
	return ret
}

func cloneRuntimeActivation(item *projectioncontract.RuntimeActivationStatus) *projectioncontract.RuntimeActivationStatus {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
