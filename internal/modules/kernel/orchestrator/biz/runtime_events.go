package orchestrator

import (
	"context"
	"fmt"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (o *RuntimeOrchestrator) publishRunStateChanged(ctx context.Context, run *runModel.Run) error {
	return o.publishRunStateChangedAtStep(ctx, run, "")
}

func (o *RuntimeOrchestrator) publishRunStateChangedAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if err := o.recordRunCheckpointAtStep(ctx, run, currentStepID); err != nil {
		return err
	}
	if o.eventHub == nil || run == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "run",
		SubjectID:    string(run.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		Payload: events.RunPayload(
			run.GoalID,
			run.DoDID,
			run.State,
			run.BlockingTicketID,
			run.BlockingApprovalID,
			run.FailureReason,
		),
	})
}

func (o *RuntimeOrchestrator) publishResultAccepted(ctx context.Context, item *resultmodel.Result) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/accepted", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicResultAccepted,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "result",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ResultPayload(
			item.GoalID,
			item.DoDID,
			item.State,
			item.Title,
			item.Summary,
			item.SourceType,
			item.SourceRef,
			item.EvidenceRefs,
			artifactRefs,
		),
	})
}

func (o *RuntimeOrchestrator) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		ID:           fmt.Sprintf("%s/event/created", item.ID),
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicArtifactCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "artifact",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload: events.ArtifactPayload(
			item.ResultID,
			item.StepID,
			item.ToolCallID,
			item.PatchID,
			item.ValidationResultID,
			item.ArtifactType,
			item.Name,
			item.Summary,
			item.URI,
			item.ContentType,
			item.State,
			item.EvidenceRefs,
		),
	})
}

func (o *RuntimeOrchestrator) publishMemoryChanged(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	if item == nil || o.eventHub == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "memory",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload:      events.MemoryPayload(item.MemoryType, item.Summary, item.Content, item.SourceType, item.SourceRef, item.Tags, item.Importance, item.Confidence, item.Visibility),
	})
}

func (o *RuntimeOrchestrator) publishWorkspaceStateChanged(ctx context.Context, workspace *workspaceModel.Workspace, runID common.RunID) error {
	if o.eventHub == nil || workspace == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicWorkspaceStateChanged,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "workspace",
		SubjectID:    string(workspace.ID),
		WorkspaceID:  string(workspace.ID),
		RunID:        string(runID),
		Payload:      events.WorkspacePayload(workspace.Title, workspace.Description, workspace.WorkingDirectory, workspace.State, workspace.ActiveRunID),
	})
}

func (o *RuntimeOrchestrator) publishStepState(ctx context.Context, run *runModel.Run, step *runModel.Step, topic string) error {
	if o.eventHub == nil || run == nil || step == nil {
		return nil
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "step",
		SubjectID:    string(step.ID),
		WorkspaceID:  string(run.WorkspaceID),
		RunID:        string(run.ID),
		StepID:       string(step.ID),
		Payload:      events.StepPayload(step.Kind, step.State, step.Input, step.Output),
	})
}
