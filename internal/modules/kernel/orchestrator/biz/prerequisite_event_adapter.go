package orchestrator

// Prerequisite EventHub adapter is internal to the owner Biz.

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func AcquireTargetSnapshotPrerequisite(ctx context.Context, hub magicevent.Hub, command orchestratorevents.AcquireTargetSnapshotCommand) (orchestratorevents.AcquireTargetSnapshotResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.TargetRef == "" {
		return orchestratorevents.AcquireTargetSnapshotResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireTargetSnapshot,
		ModuleID,
		orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return orchestratorevents.AcquireTargetSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return orchestratorevents.AcquireTargetSnapshotResult{}, err.Cause
		}
		return orchestratorevents.AcquireTargetSnapshotResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		if err.Cause != nil {
			return orchestratorevents.AcquireTargetSnapshotResult{}, err.Cause
		}
		return orchestratorevents.AcquireTargetSnapshotResult{}, err
	}
	response, ok := data.(orchestratorevents.AcquireTargetSnapshotResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID || !response.Snapshot.Complete() {
		return orchestratorevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	return response, nil
}

func AcquireAndCommitTargetSnapshotPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command orchestratorevents.AcquireTargetSnapshotCommand) (runtimecontract.RuntimeFacts, error) {
	response, err := AcquireTargetSnapshotPrerequisite(ctx, hub, command)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: response.PrerequisiteID,
		TargetSnapshot: &response.Snapshot,
		Observation:    response.Observation,
	})
}

func AcquireAndCommitWorkspaceManifestPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command orchestratorevents.AcquireWorkspaceManifestCommand) (runtimecontract.RuntimeFacts, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireWorkspaceManifest, ModuleID, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeFacts{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.AcquireWorkspaceManifestResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID || response.Manifest.Revision == "" {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("workspace manifest prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: response.PrerequisiteID,
		Manifest:       &response.Manifest,
		Observation:    response.Observation,
	})
}

func AcquireAndCommitProviderCeilingPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command orchestratorevents.AcquireProviderCeilingCommand) (runtimecontract.RuntimeFacts, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireProviderCeiling, ModuleID, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeFacts{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.AcquireProviderCeilingResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID || response.Ceiling.MaxBytes <= 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("provider ceiling prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID:  response.PrerequisiteID,
		ProviderCeiling: &response.Ceiling,
		Observation:     response.Observation,
	})
}

func AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command orchestratorevents.AcquireRuntimeCapabilitiesCommand) (runtimecontract.RuntimeFacts, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.ModuleID == "" || len(command.RequiredKinds) == 0 {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, ModuleID, command.ModuleID, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeFacts{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeCapabilitiesResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime capability prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: response.PrerequisiteID,
		Capabilities:   &response.Capabilities,
		Observation:    response.Observation,
	})
}

func AcquireGitCapabilityStatePrerequisite(ctx context.Context, hub magicevent.Hub, moduleID string, command orchestratorevents.AcquireGitCapabilityStateCommand) (orchestratorevents.AcquireGitCapabilityStateResult, error) {
	if hub == nil || moduleID == "" || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind == "" || command.SubjectRef == "" {
		return orchestratorevents.AcquireGitCapabilityStateResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireGitCapabilityState, ModuleID, moduleID, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return orchestratorevents.AcquireGitCapabilityStateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.AcquireGitCapabilityStateResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return orchestratorevents.AcquireGitCapabilityStateResult{}, prerequisiteMagicError(err)
	}
	response, ok := data.(orchestratorevents.AcquireGitCapabilityStateResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID {
		return orchestratorevents.AcquireGitCapabilityStateResult{}, fmt.Errorf("git capability-state prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	return response, nil
}

func prerequisiteResultError(result magicevent.Result) error {
	if err := result.Error(); err != nil {
		return prerequisiteMagicError(err)
	}
	return nil
}

func prerequisiteMagicError(err *cd.Error) error {
	if err == nil {
		return nil
	}
	if err.Cause != nil {
		return err.Cause
	}
	return err
}
