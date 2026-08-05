package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TaskDecompositionInfosFromProjection(items []projectioncontract.TaskDecomposition) []TaskDecompositionInfo {
	ret := make([]TaskDecompositionInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TaskDecompositionInfo{
			ID:            item.ID,
			WorkspaceID:   common.WorkspaceID(item.WorkspaceID),
			RunID:         common.RunID(item.RunID),
			RootStepID:    common.StepID(item.RootStepID),
			State:         item.State,
			Summary:       item.Summary,
			GraphBranch:   item.GraphBranch,
			NodeTotal:     item.NodeTotal,
			NodeCompleted: item.NodeCompleted,
			NodeFailed:    item.NodeFailed,
			NodeRunning:   item.NodeRunning,
			Nodes:         taskDecompositionNodeInfosFromProjection(item.Nodes),
			EvidenceRefs:  append([]string(nil), item.EvidenceRefs...),
		})
	}
	return ret
}

func TaskPlanInfosFromProjection(items []projectioncontract.TaskPlan) []TaskPlanInfo {
	ret := make([]TaskPlanInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TaskPlanInfo{
			ID:                item.ID,
			WorkspaceID:       common.WorkspaceID(item.WorkspaceID),
			RunID:             common.RunID(item.RunID),
			AnchorStepID:      common.StepID(item.AnchorStepID),
			RevisionStepID:    common.StepID(item.RevisionStepID),
			Revision:          item.Revision,
			State:             item.State,
			Stage:             item.Stage,
			SourceOutputState: item.SourceOutputState,
			Summary:           item.Summary,
			GraphBranch:       item.GraphBranch,
			NodeTotal:         item.NodeTotal,
			NodeCompleted:     item.NodeCompleted,
			NodeFailed:        item.NodeFailed,
			NodeRunning:       item.NodeRunning,
			Nodes:             taskDecompositionNodeInfosFromProjection(item.Nodes),
			RetryOf:           item.RetryOf,
			RetryAttempt:      item.RetryAttempt,
			FailureClass:      item.FailureClass,
			FailureSignature:  item.FailureSignature,
			TargetScope:       item.TargetScope,
			BlockingTicketID:  common.TicketID(item.BlockingTicketID),
			EvidenceRefs:      append([]string(nil), item.EvidenceRefs...),
			OccurredAt:        item.OccurredAt,
			SourceProjection:  item.SourceProjection,
			SealedLLMTarget:   runtimecontract.CloneProviderModelTarget(item.SealedLLMTarget),
			RuntimeActivation: CloneRuntimeActivationStatus(item.RuntimeActivation),
		})
	}
	return ret
}

func taskDecompositionNodeInfosFromProjection(items []projectioncontract.TaskDecompositionNode) []TaskDecompositionNodeInfo {
	ret := make([]TaskDecompositionNodeInfo, 0, len(items))
	for _, item := range items {
		payload, _ := workorcheventpayload.FromMap(item.Input)
		ret = append(ret, TaskDecompositionNodeInfo{
			ID:               item.ID,
			Kind:             item.Kind,
			Title:            item.Title,
			State:            item.State,
			DependsOn:        append([]string(nil), item.DependsOn...),
			Branch:           item.Branch,
			StepID:           common.StepID(item.StepID),
			StepState:        item.StepState,
			RetryOf:          item.RetryOf,
			RetryAttempt:     item.RetryAttempt,
			FailureClass:     item.FailureClass,
			FailureSignature: item.FailureSignature,
			TargetScope:      item.TargetScope,
			BlockingTicketID: common.TicketID(item.BlockingTicketID),
			EvidenceRefs:     append([]string(nil), item.EvidenceRefs...),
			CoverageTargets:  append([]string(nil), item.CoverageTargets...),
			Input:            payload,
		})
	}
	return ret
}
