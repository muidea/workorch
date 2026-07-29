package orchestrator

// Prerequisite EventHub adapter is internal to the owner Biz.

import (
	"context"
	"fmt"
	"strings"

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
	if response.Snapshot.TargetRef != command.TargetRef {
		return orchestratorevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q returned a mismatched target", command.PrerequisiteID)
	}
	if err := validateFileTargetSnapshotObservation(command.PrerequisiteID, response.Snapshot, response.Observation); err != nil {
		return orchestratorevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q: %w", command.PrerequisiteID, err)
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

func AcquireRuntimeTargetSelectionPrerequisite(ctx context.Context, hub magicevent.Hub, command orchestratorevents.AcquireRuntimeTargetSelectionCommand) (orchestratorevents.AcquireRuntimeTargetSelectionResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.TargetRole == "" || command.TargetSelectorRef == "" || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorAcquireRuntimeTargetSelection, ModuleID, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeTargetSelectionResult)
	if !ok || response.PrerequisiteID != command.PrerequisiteID || response.Selection.PrerequisiteID != command.PrerequisiteID || response.Selection.OwnerModuleID != runtimecontract.RuntimeModuleFile || response.Selection.TargetRole != command.TargetRole || response.Selection.SelectorRef != command.TargetSelectorRef || response.Selection.SelectorOwnerRef != command.TargetSelectorOwnerRef || response.Selection.SelectorSubjectRef != command.TargetSelectorSubjectRef || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(response.Selection.SelectorCandidateRefs, command.TargetSelectorCandidateRefs) || response.Selection.ProspectiveTargetRef != command.ProspectiveTargetRef {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if err := response.Selection.Validate(); err != nil {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q: %w", command.PrerequisiteID, err)
	}
	// A target selection is a mutable File-owner observation, not a durable
	// target authority by itself. Require the owner-authored workspace-tree
	// dependency here so the controller can later invalidate this exact
	// selection after a workspace change. The adapter verifies the concrete
	// EventHub result; it does not select a target or inspect workspace paths.
	if err := validateRuntimeTargetSelectionObservation(command.PrerequisiteID, response.Selection, response.Observation); err != nil {
		return orchestratorevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q: %w", command.PrerequisiteID, err)
	}
	return response, nil
}

func ConfirmRuntimeRepairSurface(ctx context.Context, hub magicevent.Hub, command orchestratorevents.ConfirmRuntimeRepairSurfaceCommand) (orchestratorevents.ConfirmRuntimeRepairSurfaceResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || command.Observation.Validate() != nil || command.Observation.WorkspaceID != command.WorkspaceID || command.Observation.RunID != command.RunID {
		return orchestratorevents.ConfirmRuntimeRepairSurfaceResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorConfirmRuntimeRepairSurface, ModuleID, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return orchestratorevents.ConfirmRuntimeRepairSurfaceResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return orchestratorevents.ConfirmRuntimeRepairSurfaceResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return orchestratorevents.ConfirmRuntimeRepairSurfaceResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(orchestratorevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || response.Confirmation.Validate() != nil || response.Confirmation.OwnerModuleID != runtimecontract.RuntimeModuleFile || !runtimecontract.SameRuntimeValidationFailureObservation(response.Confirmation.Observation, command.Observation) {
		return orchestratorevents.ConfirmRuntimeRepairSurfaceResult{}, fmt.Errorf("repair surface confirmation returned an invalid result")
	}
	return response, nil
}

func validateRuntimeTargetSelectionObservation(prerequisiteID string, selection runtimecontract.RuntimeTargetSelection, observation *runtimecontract.RuntimePrerequisiteObservation) error {
	selection = selection.Normalized()
	if observation == nil {
		return fmt.Errorf("target selection returned no freshness observation")
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil {
		return fmt.Errorf("target selection returned invalid freshness observation: %w", err)
	}
	if value.PrerequisiteID != prerequisiteID || value.Kind != runtimecontract.PrerequisiteKindTargetSelection || value.OwnerModuleID != runtimecontract.RuntimeModuleFile || value.ContentHash != selection.SnapshotID {
		return fmt.Errorf("target selection freshness observation does not match selection")
	}
	if len(value.Dependencies) != 1 {
		return fmt.Errorf("target selection freshness observation requires exactly one workspace-tree dependency")
	}
	dependency := value.Dependencies[0].Normalized()
	if dependency.Domain != runtimecontract.RuntimeResourceWorkspaceTree || dependency.OwnerModuleID != runtimecontract.RuntimeModuleFile || dependency.SubjectRef != "workspace_deliverable" || dependency.Revision != selection.ObservedRevision {
		return fmt.Errorf("target selection freshness observation does not match workspace-tree revision")
	}
	return nil
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
	if err := validateFileWorkspaceManifestObservation(command.PrerequisiteID, response.Manifest, response.Observation); err != nil {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("workspace manifest prerequisite %q: %w", command.PrerequisiteID, err)
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: response.PrerequisiteID,
		Manifest:       &response.Manifest,
		Observation:    response.Observation,
	})
}

func validateFileTargetSnapshotObservation(prerequisiteID string, snapshot runtimecontract.TargetSnapshot, observation *runtimecontract.RuntimePrerequisiteObservation) error {
	if observation == nil {
		return fmt.Errorf("target snapshot returned no freshness observation")
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil {
		return fmt.Errorf("target snapshot returned invalid freshness observation: %w", err)
	}
	if value.PrerequisiteID != prerequisiteID || value.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || value.OwnerModuleID != runtimecontract.RuntimeModuleFile || value.ContentHash != snapshot.ContentHash {
		return fmt.Errorf("target snapshot freshness observation does not match snapshot")
	}
	if len(value.Dependencies) != 1 {
		return fmt.Errorf("target snapshot freshness observation requires exactly one workspace-target dependency")
	}
	dependency := value.Dependencies[0].Normalized()
	if dependency.Domain != runtimecontract.RuntimeResourceWorkspaceTarget || dependency.OwnerModuleID != runtimecontract.RuntimeModuleFile || dependency.SubjectRef != snapshot.TargetRef || dependency.Revision != snapshot.ContentHash {
		return fmt.Errorf("target snapshot freshness observation does not match workspace-target revision")
	}
	return nil
}

func validateFileWorkspaceManifestObservation(prerequisiteID string, manifest runtimecontract.WorkspaceManifest, observation *runtimecontract.RuntimePrerequisiteObservation) error {
	if observation == nil {
		return fmt.Errorf("workspace manifest returned no freshness observation")
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil {
		return fmt.Errorf("workspace manifest returned invalid freshness observation: %w", err)
	}
	if value.PrerequisiteID != prerequisiteID || value.Kind != runtimecontract.PrerequisiteKindManifest || value.OwnerModuleID != runtimecontract.RuntimeModuleFile || value.ContentHash != manifest.Revision {
		return fmt.Errorf("workspace manifest freshness observation does not match manifest")
	}
	if len(value.Dependencies) != 1 {
		return fmt.Errorf("workspace manifest freshness observation requires exactly one workspace-tree dependency")
	}
	dependency := value.Dependencies[0].Normalized()
	if dependency.Domain != runtimecontract.RuntimeResourceWorkspaceTree || dependency.OwnerModuleID != runtimecontract.RuntimeModuleFile || dependency.SubjectRef != "workspace_deliverable" || dependency.Revision != manifest.Revision {
		return fmt.Errorf("workspace manifest freshness observation does not match workspace-tree revision")
	}
	return nil
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
		return runtimecontract.RuntimeFacts{}, &orchestratorevents.RuntimeCapabilityUnsupportedError{
			ModuleID: command.ModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: append([]string(nil), command.RequiredKinds...), Reason: "owner_unavailable",
		}
	}
	// magicCommon represents a missing destination observer as a non-nil Result
	// carrying an Unexpected framework error. At this typed owner boundary it
	// means the sealed capability owner is absent, not a transient provider or
	// generic governance dependency.
	if magicErr := result.Error(); magicErr != nil && strings.HasPrefix(strings.TrimSpace(magicErr.Message), "missing observer,") {
		return runtimecontract.RuntimeFacts{}, &orchestratorevents.RuntimeCapabilityUnsupportedError{
			ModuleID: command.ModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: append([]string(nil), command.RequiredKinds...), Reason: "owner_unavailable",
		}
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
