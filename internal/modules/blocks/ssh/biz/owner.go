package ssh

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
	started        bool
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	adapter, err := newConfiguredRuntimeAdapter()
	if err != nil {
		return nil, err
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), runtimeAdapter: adapter}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.SubscribeFunc(sshevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(sshevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(sshevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(sshevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(sshevents.TopicExecuteOperation, m.handleSSHOperation)
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	if m.started {
		m.UnsubscribeFunc(sshevents.TopicExecuteOperation)
		m.UnsubscribeFunc(sshevents.TopicAcquireRuntimeCapabilities)
		m.UnsubscribeFunc(sshevents.TopicAcquireRuntimeOperationAdmissibility)
		m.UnsubscribeFunc(sshevents.TopicProbeRuntimeOperationFeasibility)
		m.UnsubscribeFunc(sshevents.TopicProbeRuntimeSemanticProofFeasibility)
		m.started = false
	}
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
		m.runtimeAdapter = nil
	}
}

func (m *Biz) handleSSHOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := sshRuntimeIngressCommand[sshevents.ExecuteOperationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := command.Execution.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation, authorization, err := command.SealedOperation.ValidateForExecution(command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if command.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectRemoteDurable {
		setCapabilityError(result, fmt.Errorf("runtime SSH operation requires sealed remote_durable effect"))
		return
	}
	data, err := sshevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
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
	currentRevision := m.runtimeAdapter.admissibilityRevision()
	if currentRevision != authorization.OwnerRevision {
		message := "SSH owner policy changed after operation seal"
		failure := &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "operation_admissibility_stale",
			Signature: "ssh-owner-catalog-stale:" + authorization.CatalogObservation.ObservationID, Message: message,
			TargetRefs: append([]string(nil), command.TargetRefs...),
		}
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs,
			runtimecontract.RuntimeOperationEvidence{Summary: message}, failure, nil,
			orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: authorization.PrerequisiteID, ObservationID: authorization.CatalogObservation.ObservationID,
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID, SubjectRef: "ssh_runtime_policy",
			ExpectedRevision: authorization.OwnerRevision, ObservedRevision: currentRevision, Reason: message,
		}
		committed, commitErr := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
		if commitErr != nil {
			setCapabilityError(result, commitErr)
			return
		}
		result.Set(sshevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	requirement := sshApprovalRequirement(data)
	if command.Execution.GovernanceGrant == nil {
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs,
			runtimecontract.RuntimeOperationEvidence{Summary: "SSH remote command requires approval before execution"}, nil, requirement,
			orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly})
		committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		result.Set(sshevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	if !sshApprovalGranted(command.Execution, *requirement) {
		setCapabilityError(result, fmt.Errorf("runtime SSH approval grant does not match sealed owner authorization"))
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		TargetRef: data.TargetRef, SessionRef: data.SessionRef, CredentialRef: data.CredentialRef,
		Command: data.Command, TimeoutMillis: data.TimeoutMillis, EvidencePolicies: command.EvidencePolicies,
	})
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindSSHCommand, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{
		ObservedEffectScope: operationResult.ObservedScope, Changed: operationResult.Changed,
		NoOp:       status == runtimecontract.RuntimeOperationExecutionStatusAccepted && !operationResult.Changed,
		BeforeHash: operationResult.BeforeHash, AfterHash: operationResult.AfterHash,
	}
	if operationResult.Changed && operationResult.BeforeHash != "" && operationResult.AfterHash != "" {
		observation.ResourceChanges = []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceRemoteResource, OwnerModuleID: ModuleID,
			SubjectRef:     data.TargetRef + "/" + operationResult.ObservationRef,
			BeforeRevision: operationResult.BeforeHash, AfterRevision: operationResult.AfterHash,
		}}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, command.TargetRefs, evidence, failure, nil, observation)
	attachSSHOutputDiagnostic(&receipt, operationResult)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(sshevents.ExecuteOperationResult{Receipt: committed}, nil)
}

func sshApprovalRequirement(data sshevents.OperationData) *runtimecontract.RuntimeGovernanceRequirement {
	commandHash := hashSSHBytes([]byte(strings.TrimSpace(data.Command)))
	return &runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: strings.TrimSpace(data.TargetRef),
		Reason:          "SSH remote command requires explicit approval before any network side effect",
		RequestedAction: runtimecontract.StepKindSSHCommand, RiskLevel: "external_high",
		AuthorizationContext: workorcheventpayload.New(
			workorcheventpayload.StringField("targetRef", strings.TrimSpace(data.TargetRef)),
			workorcheventpayload.StringField("sessionRef", strings.TrimSpace(data.SessionRef)),
			workorcheventpayload.StringField("credentialRef", strings.TrimSpace(data.CredentialRef)),
			workorcheventpayload.StringField("operationKind", runtimecontract.StepKindSSHCommand),
			workorcheventpayload.StringField("commandHash", commandHash),
			workorcheventpayload.IntField("timeoutMillis", data.TimeoutMillis),
		),
	}
}

func sshApprovalGranted(execution runtimecontract.RuntimeOperationExecutionEnvelope, requirement runtimecontract.RuntimeGovernanceRequirement) bool {
	if execution.GovernanceGrant == nil {
		return false
	}
	grant := execution.GovernanceGrant.Normalized()
	requirement = requirement.Normalized()
	return grant.Validate() == nil && grant.TicketID == "" && grant.ApprovalID != "" &&
		grant.DependencyKind == string(requirement.Kind) && grant.SubjectRef == requirement.SubjectRef &&
		grant.RequestedAction == requirement.RequestedAction && grant.RiskLevel == requirement.RiskLevel &&
		grant.AuthorizationHash == runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext)
}

func attachSSHOutputDiagnostic(receipt *runtimecontract.RuntimeOperationReceipt, result RuntimeCommandResult) {
	if receipt == nil || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil || result.OutputHash == "" {
		return
	}
	diagnostic := receipt.Feedback.OriginalDiagnostic
	diagnostic.DetailsRef = "workorch://ssh/evidence/" + result.OutputHash
	diagnostic.DetailsHash = result.OutputHash
	diagnostic.DetailsBytes = result.StdoutBytes + result.StderrBytes
	diagnostic.ContentType = "application/vnd.workorch.ssh-output-metadata"
	diagnostic.OmittedReason = "raw_ssh_command_output"
	diagnostic.Truncated = result.OutputTruncated
	diagnostic.Redacted = true
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := sshRuntimeIngressCommand[sshevents.AcquireRuntimeCapabilitiesCommand](event, result)
	if !ok {
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
	command, ok := sshRuntimeIngressCommand[sshevents.AcquireRuntimeOperationAdmissibilityCommand](event, result)
	if !ok {
		return
	}
	if err := sshevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	if m.runtimeAdapter == nil || len(m.runtimeAdapter.targets) > command.MaxItems {
		setCapabilityError(result, fmt.Errorf("runtime SSH owner inventory exceeds bounded request"))
		return
	}
	targetRefs := m.runtimeAdapter.targetRefs(command.MaxItems)
	bindingRefs := m.runtimeAdapter.bindingConstraintRefs(command.MaxItems)
	if len(bindingRefs) != len(m.runtimeAdapter.bindingConstraintRefs(runtimecontract.RuntimeOwnerInventoryItemsMaxCount)) {
		setCapabilityError(result, fmt.Errorf("runtime SSH owner binding inventory exceeds bounded request"))
		return
	}
	constraintRefs := append([]string{"owner-resolved-refs", "strict-host-identity", "approval-before-side-effect", "remote-before-after-observation", "owner-execution-policy"}, bindingRefs...)
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, targetRefs, nil,
		constraintRefs, m.runtimeAdapter.admissibilityRevision())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind:                  runtimecontract.StepKindSSHCommand,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		ConstraintRefs:        append([]string(nil), constraintRefs...),
	}}
	inventory.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "operation-policy/ssh-command", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindSSHCommand,
		SubjectRef: "configured-ssh-target", EffectScope: runtimecontract.RuntimeEffectRemoteDurable,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
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
	command, ok := sshRuntimeIngressCommand[sshevents.ProbeRuntimeOperationFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
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
	if !containsSSHRef(command.Admissibility.TargetRefs, data.TargetRef) ||
		!containsSSHRef(command.Admissibility.ConstraintRefs, sshBindingConstraintRef(data.TargetRef, data.SessionRef, data.CredentialRef)) {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "ssh_owner_binding_unavailable", "runtime ssh operation refs are outside the sealed owner catalog")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID),
		TargetRef: data.TargetRef, SessionRef: data.SessionRef, CredentialRef: data.CredentialRef,
		Command: data.Command, TimeoutMillis: data.TimeoutMillis,
	})
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "ssh_prerequisite_unavailable", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := sshRuntimeIngressCommand[sshevents.ProbeRuntimeSemanticProofFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
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

// sshRuntimeIngressCommand is the owner-local strong-call gate for every
// Runtime topic subscribed by SSH. Post is rejected before DTO inspection;
// only the exact Orchestrator source may reach remote-session, policy, receipt,
// or command execution paths.
func sshRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) {
	var zero T
	if result == nil {
		return zero, false
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setSSHRuntimeIngressInvalid(result, "runtime SSH ingress requires exact Orchestrator source")
		return zero, false
	}
	command, ok := event.Data().(T)
	if !ok {
		setSSHRuntimeIngressInvalid(result, "runtime SSH ingress payload has an invalid concrete type")
		return zero, false
	}
	return command, true
}

func setSSHRuntimeIngressInvalid(result magicevent.Result, message string) {
	if result == nil {
		return
	}
	err := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
