package orchestrator

// Operation admissibility adapter is internal to the owner Biz.

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func AcquireRuntimeOperationAdmissibilityPrerequisite(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeOperationAdmissibilityRequest) (runtimecontract.RuntimeOperationAdmissibilityAcquisition, error) {
	if hub == nil || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(command.PrerequisiteID) == "" || strings.TrimSpace(command.OwnerModuleID) == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || len(command.RequiredKinds) == 0 || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeOperationAdmissibilityRequest(command, command.OwnerModuleID); err != nil {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, owner := sendRuntimeOperationAdmissibilityOwnerCommand(ctx, hub, command)
	if result == nil || owner == "" {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, prerequisiteMagicError(magicErr)
	}
	response, ok := normalizeRuntimeOperationAdmissibilityOwnerResult(owner, data)
	if !ok {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned an invalid result", command.PrerequisiteID)
	}
	if err := response.ValidateForRequest(command); err != nil {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned an invalid result: %w", command.PrerequisiteID, err)
	}
	inventory := response.Admissibility.Normalized()
	if binding := strings.TrimSpace(command.CapabilityStateBinding); binding != "" && inventory.AcquisitionBinding != binding {
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, fmt.Errorf("runtime operation admissibility prerequisite %q returned a mismatched capability-state binding", command.PrerequisiteID)
	}
	for _, required := range command.RequiredKinds {
		if !containsRuntimeOperationKind(inventory.OperationKinds, required) {
			return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, fmt.Errorf("runtime operation admissibility prerequisite %q omitted kind %q", command.PrerequisiteID, required)
		}
	}
	response.Admissibility = inventory
	response.Observation = response.Observation.Normalized()
	return response, nil
}

func sendRuntimeOperationAdmissibilityOwnerCommand(ctx context.Context, hub magicevent.Hub, c runtimecontract.RuntimeOperationAdmissibilityRequest) (magicevent.Result, string) {
	switch c.OwnerModuleID {
	case runtimecontract.RuntimeModuleFile:
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, fileRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	case runtimecontract.RuntimeModuleGit:
		return hub.Send(magicevent.NewEventWithContext(gitevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, gitRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	case runtimecontract.RuntimeModuleMCP:
		return hub.Send(magicevent.NewEventWithContext(mcpevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, mcpRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	case runtimecontract.RuntimeModuleShell:
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, shellRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	case runtimecontract.RuntimeModuleSSH:
		return hub.Send(magicevent.NewEventWithContext(sshevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, sshRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	case runtimecontract.RuntimeModuleWeb:
		return hub.Send(magicevent.NewEventWithContext(webevents.TopicAcquireRuntimeOperationAdmissibility, ModuleID, c.OwnerModuleID, magicevent.NewHeader(), ctx, webRuntimeOperationAdmissibilityCommand(c))), c.OwnerModuleID
	default:
		return nil, ""
	}
}

func fileRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) fileevents.AcquireRuntimeOperationAdmissibilityCommand {
	return fileevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}
func gitRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) gitevents.AcquireRuntimeOperationAdmissibilityCommand {
	return gitevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}
func mcpRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) mcpevents.AcquireRuntimeOperationAdmissibilityCommand {
	return mcpevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}
func shellRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) shellevents.AcquireRuntimeOperationAdmissibilityCommand {
	return shellevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}
func sshRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) sshevents.AcquireRuntimeOperationAdmissibilityCommand {
	return sshevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}
func webRuntimeOperationAdmissibilityCommand(c runtimecontract.RuntimeOperationAdmissibilityRequest) webevents.AcquireRuntimeOperationAdmissibilityCommand {
	return webevents.AcquireRuntimeOperationAdmissibilityCommand(c)
}

func normalizeRuntimeOperationAdmissibilityOwnerResult(owner string, data any) (runtimecontract.RuntimeOperationAdmissibilityAcquisition, bool) {
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		v, ok := data.(fileevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	case runtimecontract.RuntimeModuleGit:
		v, ok := data.(gitevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	case runtimecontract.RuntimeModuleMCP:
		v, ok := data.(mcpevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	case runtimecontract.RuntimeModuleShell:
		v, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	case runtimecontract.RuntimeModuleSSH:
		v, ok := data.(sshevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	case runtimecontract.RuntimeModuleWeb:
		v, ok := data.(webevents.AcquireRuntimeOperationAdmissibilityResult)
		return v.Acquisition, ok
	default:
		return runtimecontract.RuntimeOperationAdmissibilityAcquisition{}, false
	}
}

func sendRuntimeOperationFeasibilityOwnerCommand(ctx context.Context, hub magicevent.Hub, c runtimecontract.RuntimeOperationFeasibilityRequest) (magicevent.Result, string) {
	switch c.Admissibility.OwnerModuleID {
	case runtimecontract.RuntimeModuleFile:
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, fileRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	case runtimecontract.RuntimeModuleGit:
		return hub.Send(magicevent.NewEventWithContext(gitevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, gitRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	case runtimecontract.RuntimeModuleMCP:
		return hub.Send(magicevent.NewEventWithContext(mcpevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, mcpRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	case runtimecontract.RuntimeModuleShell:
		return hub.Send(magicevent.NewEventWithContext(shellevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, shellRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	case runtimecontract.RuntimeModuleSSH:
		return hub.Send(magicevent.NewEventWithContext(sshevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, sshRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	case runtimecontract.RuntimeModuleWeb:
		return hub.Send(magicevent.NewEventWithContext(webevents.TopicProbeRuntimeOperationFeasibility, ModuleID, c.Admissibility.OwnerModuleID, magicevent.NewHeader(), ctx, webRuntimeOperationFeasibilityCommand(c))), c.Admissibility.OwnerModuleID
	default:
		return nil, ""
	}
}

func fileRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) fileevents.ProbeRuntimeOperationFeasibilityCommand {
	return fileevents.ProbeRuntimeOperationFeasibilityCommand(c)
}
func gitRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) gitevents.ProbeRuntimeOperationFeasibilityCommand {
	return gitevents.ProbeRuntimeOperationFeasibilityCommand(c)
}
func mcpRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) mcpevents.ProbeRuntimeOperationFeasibilityCommand {
	return mcpevents.ProbeRuntimeOperationFeasibilityCommand(c)
}
func shellRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) shellevents.ProbeRuntimeOperationFeasibilityCommand {
	return shellevents.ProbeRuntimeOperationFeasibilityCommand(c)
}
func sshRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) sshevents.ProbeRuntimeOperationFeasibilityCommand {
	return sshevents.ProbeRuntimeOperationFeasibilityCommand(c)
}
func webRuntimeOperationFeasibilityCommand(c runtimecontract.RuntimeOperationFeasibilityRequest) webevents.ProbeRuntimeOperationFeasibilityCommand {
	return webevents.ProbeRuntimeOperationFeasibilityCommand(c)
}

func normalizeRuntimeOperationFeasibilityOwnerResult(owner string, data any) (runtimecontract.RuntimeOperationFeasibilityObservation, bool) {
	switch owner {
	case runtimecontract.RuntimeModuleFile:
		x, ok := data.(fileevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	case runtimecontract.RuntimeModuleGit:
		x, ok := data.(gitevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	case runtimecontract.RuntimeModuleMCP:
		x, ok := data.(mcpevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	case runtimecontract.RuntimeModuleShell:
		x, ok := data.(shellevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	case runtimecontract.RuntimeModuleSSH:
		x, ok := data.(sshevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	case runtimecontract.RuntimeModuleWeb:
		x, ok := data.(webevents.ProbeRuntimeOperationFeasibilityResult)
		return runtimecontract.RuntimeOperationFeasibilityObservation(x), ok
	default:
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, false
	}
}

func ProbeRuntimeOperationFeasibility(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeOperationFeasibilityRequest) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	if hub == nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, common.ErrInvalidArgument
	}
	command, descriptor, err := normalizeRuntimeOperationFeasibilityRequest(command)
	if err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	response, err := requestRuntimeOperationFeasibility(ctx, hub, command)
	if err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	response, err = normalizeRuntimeOperationFeasibilityResponse(command, response)
	if err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	if err := validateRuntimeOperationFeasibilityFeedback(command, descriptor, response); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	switch response.Outcome {
	case runtimecontract.RuntimeOwnerFeasibilityFeasible:
		return normalizeFeasibleRuntimeOperationObservation(command, descriptor, response)
	case runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated:
		return normalizeCandidateConstraintRuntimeOperationObservation(command, descriptor, response)
	default:
		return normalizeInfeasibleRuntimeOperationObservation(command, descriptor, response)
	}
}

func normalizeRuntimeOperationFeasibilityRequest(command runtimecontract.RuntimeOperationFeasibilityRequest) (runtimecontract.RuntimeOperationFeasibilityRequest, runtimecontract.RuntimeOperationDescriptor, error) {
	operation := command.Operation.Normalized()
	inventory := command.Admissibility.Normalized()
	if command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" ||
		strings.TrimSpace(command.Transaction.AuthoritySnapshotID) == "" || operation.ID == "" || operation.Kind == "" {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, common.ErrInvalidArgument
	}
	descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !ok || inventory.OwnerModuleID == "" || inventory.OwnerModuleID != descriptor.OwnerModuleID || !containsRuntimeOperationKind(inventory.OperationKinds, operation.Kind) {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, fmt.Errorf("runtime operation feasibility probe has no matching owner admissibility inventory")
	}
	if err := inventory.Validate(); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, err
	}
	// N5: sealed candidate identity is required on the wire before any owner
	// private probe. Owners re-validate the same fields and never rebuild them.
	if err := runtimecontract.ValidateRuntimeOperationFeasibilityRequest(runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, Transaction: command.Transaction,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, Operation: operation, Admissibility: inventory,
		CapabilityStateBinding: command.CapabilityStateBinding, CandidateManifestHash: command.CandidateManifestHash,
		OperationAuthorization: command.OperationAuthorization,
	}, descriptor.OwnerModuleID); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, err
	}
	command.Operation = operation
	command.Admissibility = inventory
	command.CandidateManifestHash = strings.TrimSpace(command.CandidateManifestHash)
	command.OperationAuthorization = command.OperationAuthorization.Normalized()
	if err := normalizeRuntimeFeasibilityFreshnessObservation(&command); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, err
	}
	if err := normalizeRuntimeFeasibilityTargetSnapshot(&command); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityRequest{}, runtimecontract.RuntimeOperationDescriptor{}, err
	}
	return command, descriptor, nil
}

func normalizeRuntimeFeasibilityFreshnessObservation(command *runtimecontract.RuntimeOperationFeasibilityRequest) error {
	if command.Observation == nil {
		return nil
	}
	observation := command.Observation.Normalized()
	if observation.PrerequisiteID != command.Admissibility.PrerequisiteID || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || observation.OwnerModuleID != command.Admissibility.OwnerModuleID {
		return fmt.Errorf("runtime operation feasibility probe has mismatched freshness observation")
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	command.Observation = &observation
	return nil
}

func normalizeRuntimeFeasibilityTargetSnapshot(command *runtimecontract.RuntimeOperationFeasibilityRequest) error {
	if command.CurrentTargetSnapshot == nil {
		return nil
	}
	snapshot := *command.CurrentTargetSnapshot
	snapshot.TargetRef = strings.TrimSpace(snapshot.TargetRef)
	snapshot.ContentHash = strings.TrimSpace(snapshot.ContentHash)
	if !snapshot.Complete() {
		return fmt.Errorf("runtime operation feasibility probe has an incomplete target snapshot")
	}
	command.CurrentTargetSnapshot = &snapshot
	return nil
}

func requestRuntimeOperationFeasibility(ctx context.Context, hub magicevent.Hub, command runtimecontract.RuntimeOperationFeasibilityRequest) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	result, owner := sendRuntimeOperationFeasibilityOwnerCommand(ctx, hub, command)
	if result == nil || owner == "" {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, prerequisiteMagicError(magicErr)
	}
	response, ok := normalizeRuntimeOperationFeasibilityOwnerResult(owner, data)
	if !ok {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility probe returned an invalid result")
	}
	return response, nil
}

func normalizeRuntimeOperationFeasibilityResponse(command runtimecontract.RuntimeOperationFeasibilityRequest, response runtimecontract.RuntimeOperationFeasibilityObservation) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	operation, inventory := command.Operation, command.Admissibility
	if response.OperationID != operation.ID || response.AdmissibilitySnapshotID != inventory.SnapshotID || response.CapabilityStateBinding != inventory.AcquisitionBinding {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility probe returned an invalid result")
	}
	if response.CandidateManifestHash != command.CandidateManifestHash {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility probe returned mismatched candidate manifest hash")
	}
	if normalized, err := runtimecontract.NormalizeRuntimeOperationFeasibilityObservation(response); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	} else {
		response = normalized
	}
	return response, nil
}

func validateRuntimeOperationFeasibilityFeedback(command runtimecontract.RuntimeOperationFeasibilityRequest, descriptor runtimecontract.RuntimeOperationDescriptor, response runtimecontract.RuntimeOperationFeasibilityObservation) error {
	if response.Feedback != nil {
		feedback := response.Feedback.Normalized()
		if feedback.WorkspaceID != command.WorkspaceID || feedback.RunID != command.RunID || feedback.AuthoritySnapshotID != command.Transaction.AuthoritySnapshotID || feedback.DispatchID != "" ||
			feedback.OriginalDiagnostic == nil || feedback.OriginalDiagnostic.OwnerModuleID != descriptor.OwnerModuleID || feedback.OriginalDiagnostic.OperationID != command.Operation.ID {
			return fmt.Errorf("runtime operation feasibility feedback has mismatched scope or owner identity")
		}
	}
	return nil
}

func normalizeFeasibleRuntimeOperationObservation(command runtimecontract.RuntimeOperationFeasibilityRequest, descriptor runtimecontract.RuntimeOperationDescriptor, response runtimecontract.RuntimeOperationFeasibilityObservation) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	if response.Stale != nil || response.PrerequisiteRequest != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("feasible runtime operation probe returned failure facts")
	}
	if response.ReplayProof == nil {
		return response, nil
	}
	proof := response.ReplayProof.Normalized()
	if err := proof.Validate(); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	if proof.OperationID != command.Operation.ID || proof.AdmissibilitySnapshotID != command.Admissibility.SnapshotID || proof.AcquisitionBinding != command.Admissibility.AcquisitionBinding || proof.OwnerModuleID != descriptor.OwnerModuleID {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility probe returned mismatched replay proof")
	}
	response.ReplayProof = &proof
	return response, nil
}

func normalizeCandidateConstraintRuntimeOperationObservation(command runtimecontract.RuntimeOperationFeasibilityRequest, descriptor runtimecontract.RuntimeOperationDescriptor, response runtimecontract.RuntimeOperationFeasibilityObservation) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	if response.CandidateConstraintViolation == nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("candidate-constraint runtime operation probe has no bounded violation")
	}
	policy := response.OwnerPolicy.Normalized()
	if err := policy.Validate(); err != nil || policy.OwnerModuleID != descriptor.OwnerModuleID || policy.CatalogSnapshotID != command.Admissibility.SnapshotID {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("candidate-constraint runtime operation probe returned mismatched owner policy")
	}
	expected, err := runtimecontract.NewRuntimeOperationCandidateConstraintViolationObservation(command, *response.CandidateConstraintViolation)
	if err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	if expected.CandidateConstraintViolation == nil || *expected.CandidateConstraintViolation != *response.CandidateConstraintViolation {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("candidate-constraint runtime operation probe returned mismatched violation")
	}
	return response, nil
}

func normalizeInfeasibleRuntimeOperationObservation(command runtimecontract.RuntimeOperationFeasibilityRequest, descriptor runtimecontract.RuntimeOperationDescriptor, response runtimecontract.RuntimeOperationFeasibilityObservation) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	if response.ReplayProof != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("infeasible runtime operation probe cannot return replay proof")
	}
	if response.Stale != nil {
		return normalizeStaleRuntimeOperationObservation(command, response)
	}
	if response.Feedback == nil || response.Feedback.OriginalDiagnostic == nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("infeasible runtime operation probe requires original diagnostic feedback")
	}
	if response.PrerequisiteRequest != nil {
		request := response.PrerequisiteRequest.Normalized()
		if err := request.Validate(); err != nil {
			return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
		}
		if request.OperationID != command.Operation.ID || request.OwnerModuleID != descriptor.OwnerModuleID {
			return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility probe returned mismatched prerequisite request")
		}
		response.PrerequisiteRequest = &request
	}
	return response, nil
}

func normalizeStaleRuntimeOperationObservation(command runtimecontract.RuntimeOperationFeasibilityRequest, response runtimecontract.RuntimeOperationFeasibilityObservation) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
	if response.PrerequisiteRequest != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("stale runtime operation probe returned ordinary failure facts")
	}
	stale := response.Stale.Normalized()
	if err := stale.Validate(); err != nil {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
	}
	if stale.OwnerModuleID != command.Admissibility.OwnerModuleID {
		return runtimecontract.RuntimeOperationFeasibilityObservation{}, fmt.Errorf("stale runtime operation probe returned mismatched owner")
	}
	response.Stale = &stale
	return response, nil
}

func validateAdmissibilityOwnerKinds(owner string, kinds []string) error {
	for _, kind := range kinds {
		descriptor, ok := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != owner {
			return fmt.Errorf("runtime operation kind %q does not belong to admissibility owner %q", kind, owner)
		}
	}
	return nil
}

func containsRuntimeOperationKind(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}
