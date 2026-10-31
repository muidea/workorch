package orchestrator

// Prerequisite EventHub adapter is internal to the owner Biz.

import (
	"context"
	"fmt"
	"strings"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func AcquireTargetSnapshotPrerequisite(ctx context.Context, hub magicevent.Hub, command fileevents.AcquireTargetSnapshotCommand) (fileevents.AcquireTargetSnapshotResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.TargetRef == "" {
		return fileevents.AcquireTargetSnapshotResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireTargetSnapshot,
		ModuleID,
		fileevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return fileevents.AcquireTargetSnapshotResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return fileevents.AcquireTargetSnapshotResult{}, err.Cause
		}
		return fileevents.AcquireTargetSnapshotResult{}, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		if resultErr.Cause != nil {
			return fileevents.AcquireTargetSnapshotResult{}, resultErr.Cause
		}
		return fileevents.AcquireTargetSnapshotResult{}, resultErr
	}
	response, ok := data.(fileevents.AcquireTargetSnapshotResult)
	if !ok || !samePrerequisiteResultScope(response.WorkspaceID, response.RunID, response.AuthoritySnapshotID, command.WorkspaceID, command.RunID, command.AuthoritySnapshotID) || !response.Snapshot.Complete() {
		return fileevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if response.Snapshot.TargetRef != command.TargetRef {
		return fileevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q returned a mismatched target", command.PrerequisiteID)
	}
	if err := validateFileTargetSnapshotObservation(command.PrerequisiteID, response.Snapshot, &response.Observation); err != nil {
		return fileevents.AcquireTargetSnapshotResult{}, fmt.Errorf("target snapshot prerequisite %q: %w", command.PrerequisiteID, err)
	}
	return response, nil
}

func AcquireAndCommitTargetSnapshotPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command fileevents.AcquireTargetSnapshotCommand) (runtimecontract.RuntimeFacts, error) {
	response, err := AcquireTargetSnapshotPrerequisite(ctx, hub, command)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{
		PrerequisiteID: response.Observation.PrerequisiteID,
		TargetSnapshot: &response.Snapshot,
		Observation:    &response.Observation,
	})
}

func AcquireRuntimeTargetSelectionPrerequisite(ctx context.Context, hub magicevent.Hub, command fileevents.AcquireRuntimeTargetSelectionCommand) (fileevents.AcquireRuntimeTargetSelectionResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.TargetRole == "" || command.TargetSelectorRef == "" || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireRuntimeTargetSelection, ModuleID, fileevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(fileevents.AcquireRuntimeTargetSelectionResult)
	if !ok || !samePrerequisiteResultScope(response.WorkspaceID, response.RunID, response.AuthoritySnapshotID, command.WorkspaceID, command.RunID, command.AuthoritySnapshotID) || response.Selection.PrerequisiteID != command.PrerequisiteID || response.Selection.OwnerModuleID != runtimecontract.RuntimeModuleFile || response.Selection.TargetRole != command.TargetRole || response.Selection.SelectorRef != command.TargetSelectorRef || response.Selection.SelectorOwnerRef != command.TargetSelectorOwnerRef || response.Selection.SelectorSubjectRef != command.TargetSelectorSubjectRef || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(response.Selection.SelectorCandidateRefs, command.TargetSelectorCandidateRefs) || !runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(response.Selection.ProspectiveTargetAllOfConstraintRefs, command.ProspectiveTargetAllOfConstraintRefs) || response.Selection.ProspectiveTargetRef != command.ProspectiveTargetRef {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if err := response.Selection.Validate(); err != nil {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q: %w", command.PrerequisiteID, err)
	}
	// A target selection is a mutable File-owner observation, not a durable
	// target authority by itself. Require the owner-authored workspace-tree
	// dependency here so the controller can later invalidate this exact
	// selection after a workspace change. The adapter verifies the concrete
	// EventHub result; it does not select a target or inspect workspace paths.
	if err := validateRuntimeTargetSelectionObservation(command.PrerequisiteID, response.Selection, &response.Observation); err != nil {
		return fileevents.AcquireRuntimeTargetSelectionResult{}, fmt.Errorf("target selection prerequisite %q: %w", command.PrerequisiteID, err)
	}
	return response, nil
}

func ConfirmFileRuntimeRepairSurface(ctx context.Context, hub magicevent.Hub, command fileevents.ConfirmRuntimeRepairSurfaceCommand) (fileevents.ConfirmRuntimeRepairSurfaceResult, error) {
	if hub == nil || strings.TrimSpace(command.RequestID) == "" || strings.TrimSpace(command.RequestID) != command.RequestID || command.IssuedAtStateRevision == 0 || strings.TrimSpace(command.ExpectedSubjectRevision) == "" || strings.TrimSpace(command.ExpectedSubjectRevision) != command.ExpectedSubjectRevision || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || command.Observation.Validate() != nil || command.Observation.WorkspaceID != command.WorkspaceID || command.Observation.RunID != command.RunID {
		return fileevents.ConfirmRuntimeRepairSurfaceResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, ModuleID, fileevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return fileevents.ConfirmRuntimeRepairSurfaceResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return fileevents.ConfirmRuntimeRepairSurfaceResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return fileevents.ConfirmRuntimeRepairSurfaceResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || response.Confirmation.Validate() != nil || response.Confirmation.RequestID != command.RequestID || response.Confirmation.IssuedAtStateRevision != command.IssuedAtStateRevision || response.Confirmation.ExpectedSubjectRevision != command.ExpectedSubjectRevision || response.Confirmation.OwnerModuleID != runtimecontract.RuntimeModuleFile || response.Confirmation.SubjectRef != runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef || !runtimecontract.SameRuntimeValidationFailureObservation(response.Confirmation.Observation, command.Observation) {
		return fileevents.ConfirmRuntimeRepairSurfaceResult{}, fmt.Errorf("repair surface confirmation returned an invalid result")
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

func AcquireAndCommitWorkspaceManifestPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command fileevents.AcquireWorkspaceManifestCommand) (runtimecontract.RuntimeFacts, error) {
	response, err := AcquireWorkspaceManifestPrerequisite(ctx, hub, command)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{PrerequisiteID: response.Observation.PrerequisiteID, Manifest: &response.Manifest, Observation: &response.Observation})
}
func AcquireWorkspaceManifestPrerequisite(ctx context.Context, hub magicevent.Hub, command fileevents.AcquireWorkspaceManifestCommand) (fileevents.AcquireWorkspaceManifestResult, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" {
		return fileevents.AcquireWorkspaceManifestResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireWorkspaceManifest, ModuleID, fileevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return fileevents.AcquireWorkspaceManifestResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return fileevents.AcquireWorkspaceManifestResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return fileevents.AcquireWorkspaceManifestResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(fileevents.AcquireWorkspaceManifestResult)
	if !ok || !samePrerequisiteResultScope(response.WorkspaceID, response.RunID, response.AuthoritySnapshotID, command.WorkspaceID, command.RunID, command.AuthoritySnapshotID) || response.Manifest.Revision == "" {
		return fileevents.AcquireWorkspaceManifestResult{}, fmt.Errorf("workspace manifest prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if err := validateFileWorkspaceManifestObservation(command.PrerequisiteID, response.Manifest, &response.Observation); err != nil {
		return fileevents.AcquireWorkspaceManifestResult{}, fmt.Errorf("workspace manifest prerequisite %q: %w", command.PrerequisiteID, err)
	}
	return response, nil
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

func AcquireAndCommitProviderCeilingPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command llmevents.AcquireProviderCeilingCommand) (runtimecontract.RuntimeFacts, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(llmevents.TopicAcquireProviderCeiling, ModuleID, orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, command))
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
	response, ok := data.(llmevents.AcquireProviderCeilingResult)
	if !ok || !samePrerequisiteResultScope(response.WorkspaceID, response.RunID, response.AuthoritySnapshotID, command.WorkspaceID, command.RunID, command.AuthoritySnapshotID) || response.Ceiling.Validate() != nil {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("provider ceiling prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	providerObservation := response.Observation.Normalized()
	if err := providerObservation.Validate(); err != nil || providerObservation.PrerequisiteID != command.PrerequisiteID || providerObservation.Kind != runtimecontract.PrerequisiteKindProviderCeiling || providerObservation.OwnerModuleID != runtimecontract.RuntimeModuleLLM || len(providerObservation.Dependencies) != 1 || providerObservation.Dependencies[0].Normalized().Domain != runtimecontract.RuntimeResourceProviderConfig || providerObservation.Dependencies[0].Normalized().Revision != response.Ceiling.ConfigurationRevision {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("provider ceiling prerequisite %q returned an invalid freshness observation", command.PrerequisiteID)
	}
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{
		PrerequisiteID:  providerObservation.PrerequisiteID,
		ProviderCeiling: &response.Ceiling,
		Observation:     &providerObservation,
	})
}

func AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx context.Context, hub magicevent.Hub, facts runtimecontract.RuntimeFacts, command runtimecontract.RuntimeCapabilityAcquisitionRequest) (runtimecontract.RuntimeFacts, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.OwnerModuleID == "" || len(command.RequiredKinds) == 0 {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, resultType := sendRuntimeCapabilityOwnerCommand(ctx, hub, command)
	if resultType == "" {
		return runtimecontract.RuntimeFacts{}, &runtimecontract.RuntimeCapabilityUnsupportedError{
			OwnerModuleID: command.OwnerModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: append([]string(nil), command.RequiredKinds...), Reason: "owner_unsupported",
		}
	}
	if result == nil {
		return runtimecontract.RuntimeFacts{}, &runtimecontract.RuntimeCapabilityUnsupportedError{
			OwnerModuleID: command.OwnerModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: append([]string(nil), command.RequiredKinds...), Reason: "owner_unavailable",
		}
	}
	// magicCommon represents a missing destination observer as a non-nil Result
	// carrying an Unexpected framework error. At this typed owner boundary it
	// means the sealed capability owner is absent, not a transient provider or
	// generic governance dependency.
	if magicErr := result.Error(); magicErr != nil && strings.HasPrefix(strings.TrimSpace(magicErr.Message), "missing observer,") {
		return runtimecontract.RuntimeFacts{}, &runtimecontract.RuntimeCapabilityUnsupportedError{
			OwnerModuleID: command.OwnerModuleID, CapabilityClass: command.CapabilityClass, RequiredKinds: append([]string(nil), command.RequiredKinds...), Reason: "owner_unavailable",
		}
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeFacts{}, prerequisiteMagicError(magicErr)
	}
	response, ok := normalizeRuntimeCapabilityOwnerResult(resultType, data)
	if !ok {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime capability prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if err := response.ValidateForRequest(command); err != nil {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime capability prerequisite %q returned an invalid result: %w", command.PrerequisiteID, err)
	}
	observation := response.Observation.Normalized()
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{
		PrerequisiteID: observation.PrerequisiteID,
		Capabilities:   &response.Capabilities,
		Observation:    &observation,
	})
}

func sendRuntimeCapabilityOwnerCommand(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeCapabilityAcquisitionRequest) (magicevent.Result, string) {
	switch command.OwnerModuleID {
	case runtimecontract.RuntimeModuleApproval:
		return hub.Send(magicevent.NewEventWithContext(approvalevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, approvalevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleApproval
	case runtimecontract.RuntimeModuleDecision:
		return hub.Send(magicevent.NewEventWithContext(decisionevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, decisionevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleDecision
	case runtimecontract.RuntimeModuleFile:
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, fileevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleFile
	case runtimecontract.RuntimeModuleGit:
		return hub.Send(magicevent.NewEventWithContext(gitevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, gitevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleGit
	case runtimecontract.RuntimeModuleLLM:
		return hub.Send(magicevent.NewEventWithContext(llmevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, llmevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleLLM
	case runtimecontract.RuntimeModuleMCP:
		return hub.Send(magicevent.NewEventWithContext(mcpevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleMCP
	case runtimecontract.RuntimeModuleShell:
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleShell
	case runtimecontract.RuntimeModuleSSH:
		return hub.Send(magicevent.NewEventWithContext(sshevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, sshevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleSSH
	case runtimecontract.RuntimeModuleWeb:
		return hub.Send(magicevent.NewEventWithContext(webevents.TopicAcquireRuntimeCapabilities, ModuleID, command.OwnerModuleID, magicevent.NewHeader(), ctx, webevents.AcquireRuntimeCapabilitiesCommand(command))), runtimecontract.RuntimeModuleWeb
	default:
		return nil, ""
	}
}

func normalizeRuntimeCapabilityOwnerResult(ownerModuleID string, data any) (runtimecontract.RuntimeCapabilityAcquisition, bool) {
	switch ownerModuleID {
	case runtimecontract.RuntimeModuleApproval:
		value, ok := data.(approvalevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleDecision:
		value, ok := data.(decisionevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleFile:
		value, ok := data.(fileevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleGit:
		value, ok := data.(gitevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleLLM:
		value, ok := data.(llmevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleMCP:
		value, ok := data.(mcpevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleShell:
		value, ok := data.(shellevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleSSH:
		value, ok := data.(sshevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	case runtimecontract.RuntimeModuleWeb:
		value, ok := data.(webevents.AcquireRuntimeCapabilitiesResult)
		return value.Acquisition, ok
	default:
		return runtimecontract.RuntimeCapabilityAcquisition{}, false
	}
}

func AcquireGitCapabilityStatePrerequisite(ctx context.Context, hub magicevent.Hub, moduleID string, command gitevents.AcquireCapabilityStateCommand) (gitevents.AcquireCapabilityStateResult, error) {
	if hub == nil || moduleID == "" || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind == "" || command.SubjectRef == "" {
		return gitevents.AcquireCapabilityStateResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(gitevents.TopicAcquireCapabilityState, ModuleID, moduleID, magicevent.NewHeader(), ctx, command))
	if result == nil {
		return gitevents.AcquireCapabilityStateResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return gitevents.AcquireCapabilityStateResult{}, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return gitevents.AcquireCapabilityStateResult{}, prerequisiteMagicError(resultErr)
	}
	response, ok := data.(gitevents.AcquireCapabilityStateResult)
	if !ok || !samePrerequisiteResultScope(response.WorkspaceID, response.RunID, response.AuthoritySnapshotID, command.WorkspaceID, command.RunID, command.AuthoritySnapshotID) || response.Snapshot.RequirementID != command.PrerequisiteID || response.Snapshot.OwnerModuleID != moduleID || response.Snapshot.CapabilityStateKind != command.CapabilityStateKind || response.Snapshot.SubjectRef != command.SubjectRef || response.Snapshot.AcquisitionKey != command.AcquisitionKey || response.Snapshot.SnapshotID == "" {
		return gitevents.AcquireCapabilityStateResult{}, fmt.Errorf("git capability-state prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	gitObservation := response.Observation.Normalized()
	if err := gitObservation.Validate(); err != nil || gitObservation.PrerequisiteID != command.PrerequisiteID || gitObservation.Kind != runtimecontract.PrerequisiteKindCapabilityState || gitObservation.OwnerModuleID != moduleID || gitObservation.ContentHash != response.Snapshot.ContentHash {
		return gitevents.AcquireCapabilityStateResult{}, fmt.Errorf("git capability-state prerequisite %q returned an invalid freshness observation", command.PrerequisiteID)
	}
	response.Observation = gitObservation
	return response, nil
}

func samePrerequisiteResultScope(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID string, expectedWorkspaceID common.WorkspaceID, expectedRunID common.RunID, expectedAuthoritySnapshotID string) bool {
	return workspaceID == expectedWorkspaceID && runID == expectedRunID && strings.TrimSpace(authoritySnapshotID) == strings.TrimSpace(expectedAuthoritySnapshotID)
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
