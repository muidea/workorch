package ssh

import (
	"context"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), runtimeAdapter: NewRuntimeAdapter()}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeFunc(sshevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(sshevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(sshevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(sshevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(sshevents.TopicExecuteOperation, m.handleSSHOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(sshevents.TopicExecuteOperation)
	m.UnsubscribeFunc(sshevents.TopicAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(sshevents.TopicAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(sshevents.TopicProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(sshevents.TopicProbeRuntimeSemanticProofFeasibility)
	m.runtimeAdapter = nil
}

func (m *Biz) handleSSHOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(sshevents.ExecuteOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(sshevents.ExecuteOperationResult{Receipt: receipt}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		Host:                          command.Data.Host, Port: command.Data.Port, Username: command.Data.Username,
		Password: command.Data.Password, PrivateKey: command.Data.PrivateKey, PrivateKeyPath: command.Data.PrivateKeyPath,
		Command: command.Data.Command, TimeoutMillis: command.Data.TimeoutMillis, EvidencePolicies: command.EvidencePolicies,
	})
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindSSHCommand, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		observation.ObservedEffectScope = command.Execution.ExpectedEffectScope
		observation.Changed = runtimecontract.RuntimeEffectIsDurable(command.Execution.ExpectedEffectScope)
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(sshevents.ExecuteOperationResult{Receipt: committed}, nil)
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(sshevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := sshevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(sshevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := sshevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, nil, []string{"remote-host-parameter", "owner-execution-policy"}, m.runtimeAdapter.admissibilityRevision())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := sshAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(sshevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

func setRuntimeOperationFeasibilityResult(result magicevent.Result, command sshevents.ProbeRuntimeOperationFeasibilityCommand, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale) {
	category := ownerFeasibilityCategory(failure)
	observation, err := sshevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(sshevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := sshevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	if command.Admissibility.ObservedRevision != m.runtimeAdapter.admissibilityRevision() {
		if stale := sshAdmissibilityStale(command.Observation, command.Admissibility, m.runtimeAdapter.admissibilityRevision()); stale != nil {
			setRuntimeOperationFeasibilityResult(result, command, nil, stale)
			return
		}
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime ssh operation admissibility policy is stale")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	data, err := sshevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "ssh_operation_invalid", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		Host:                          data.Host, Port: data.Port, Username: data.Username, Password: data.Password,
		PrivateKey: data.PrivateKey, PrivateKeyPath: data.PrivateKeyPath, Command: data.Command, TimeoutMillis: data.TimeoutMillis,
	})
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "ssh_prerequisite_unavailable", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(sshevents.ProbeRuntimeSemanticProofFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := sshevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	revision := m.runtimeAdapter.admissibilityRevision()
	if revision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime SSH semantic catalog is stale", command.OwnerCatalog.ObservedRevision, revision, nil))
		return
	}
	if strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0 {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "ssh_semantic_proof_unavailable", "SSH policy and command admissibility are not an independent semantic validation proof", "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal), nil)
}

func setRuntimeSemanticProofFeasibilityResult(result magicevent.Result, command sshevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := sshevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func sshAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID, SubjectRef: "ssh_runtime_policy", Revision: inventory.ObservedRevision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func sshAdmissibilityStale(observation *runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	if observation == nil {
		return nil
	}
	value := observation.Normalized()
	for _, dependency := range value.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "ssh_runtime_policy" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime ssh owner policy changed"}
		}
	}
	return nil
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
