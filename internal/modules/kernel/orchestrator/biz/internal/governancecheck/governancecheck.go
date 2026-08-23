// Package governancecheck contains owner-neutral governance identity and
// outcome predicates. It does not create tickets, mutate bindings, resume a
// dispatch, or change Runtime state.
package governancecheck

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type DispatchFacts struct {
	DecisionState       runtimecontract.ConvergenceState
	DispatchID          string
	AuthoritySnapshotID string
	CandidateGovernance bool
	DependencyKind      string
	SubjectRef          string
}

// OperationSuspensionFinding is the detached, owner-neutral result of matching
// one owner-committed operation execution with its durable governance receipt.
// Runtime decides how to persist the binding and which convergence transition
// follows; this package only proves the immutable execution/dependency facts.
type OperationSuspensionFinding struct {
	Binding    runtimecontract.RuntimeOperationGovernanceBinding
	Dependency runtimecontract.ExternalDependency
}

func DispatchIdentityMatches(facts DispatchFacts, dispatchID, authoritySnapshotID string) bool {
	return strings.TrimSpace(facts.DispatchID) == strings.TrimSpace(dispatchID) &&
		strings.TrimSpace(facts.AuthoritySnapshotID) == strings.TrimSpace(authoritySnapshotID)
}

func RequestDispatchMatches(request runtimecontract.RuntimeGovernanceRequestRecord, facts DispatchFacts) bool {
	return facts.CandidateGovernance && DispatchIdentityMatches(facts, request.DispatchID, request.AuthoritySnapshotID)
}

func DependencyMatches(expectedKind, expectedSubject, actualKind, actualSubject string) bool {
	return strings.TrimSpace(expectedKind) == strings.TrimSpace(actualKind) &&
		strings.TrimSpace(expectedSubject) == strings.TrimSpace(actualSubject)
}

// ExternalDependencyMatches compares the complete sealed dependency,
// including its owner-authored reason, after canonical normalization.
func ExternalDependencyMatches(left, right runtimecontract.ExternalDependency) bool {
	return runtimecontract.SameExternalDependency(left, right)
}

// RequestForTicket returns the exact persisted request bound to a Decision
// owner ticket. It is used by the protocol adapter only to construct a typed
// resolution; it neither loads state nor consumes the request.
func RequestForTicket(values []runtimecontract.RuntimeGovernanceRequestRecord, ticketID common.TicketID) (runtimecontract.RuntimeGovernanceRequestRecord, bool) {
	for _, value := range values {
		if value.TicketID != ticketID {
			continue
		}
		value.AuthoritySnapshotID = strings.TrimSpace(value.AuthoritySnapshotID)
		value.DispatchID = strings.TrimSpace(value.DispatchID)
		value.DependencyKind = strings.TrimSpace(value.DependencyKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.RequestedAction = strings.TrimSpace(value.RequestedAction)
		value.RiskLevel = strings.TrimSpace(value.RiskLevel)
		value.AuthorizationHash = strings.TrimSpace(value.AuthorizationHash)
		return value, true
	}
	return runtimecontract.RuntimeGovernanceRequestRecord{}, false
}

// RequestForApproval returns the exact persisted request bound to an Approval
// owner record. It is a lookup only; Approval lifecycle remains owner-local.
func RequestForApproval(values []runtimecontract.RuntimeGovernanceRequestRecord, approvalID common.ApprovalID) (runtimecontract.RuntimeGovernanceRequestRecord, bool) {
	for _, value := range values {
		if value.ApprovalID != approvalID {
			continue
		}
		value.AuthoritySnapshotID = strings.TrimSpace(value.AuthoritySnapshotID)
		value.DispatchID = strings.TrimSpace(value.DispatchID)
		value.DependencyKind = strings.TrimSpace(value.DependencyKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.RequestedAction = strings.TrimSpace(value.RequestedAction)
		value.RiskLevel = strings.TrimSpace(value.RiskLevel)
		value.AuthorizationHash = strings.TrimSpace(value.AuthorizationHash)
		return value, true
	}
	return runtimecontract.RuntimeGovernanceRequestRecord{}, false
}

// RequestForDispatch returns the ticket already bound to one exact governance
// dispatch/snapshot pair. It is a replay lookup, not ticket creation policy.
func RequestForDispatch(values []runtimecontract.RuntimeGovernanceRequestRecord, dispatchID, authoritySnapshotID string) (runtimecontract.RuntimeGovernanceRequestRecord, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	authoritySnapshotID = strings.TrimSpace(authoritySnapshotID)
	for _, value := range values {
		if strings.TrimSpace(value.DispatchID) != dispatchID || strings.TrimSpace(value.AuthoritySnapshotID) != authoritySnapshotID {
			continue
		}
		value.AuthoritySnapshotID = authoritySnapshotID
		value.DispatchID = dispatchID
		value.DependencyKind = strings.TrimSpace(value.DependencyKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.RequestedAction = strings.TrimSpace(value.RequestedAction)
		value.RiskLevel = strings.TrimSpace(value.RiskLevel)
		value.AuthorizationHash = strings.TrimSpace(value.AuthorizationHash)
		return value, true
	}
	return runtimecontract.RuntimeGovernanceRequestRecord{}, false
}

// SameRequestRecord compares the complete normalized ticket binding. It is a
// replay predicate only; it does not persist or reopen a governance request.
func SameRequestRecord(left, right runtimecontract.RuntimeGovernanceRequestRecord) bool {
	return left.TicketID == right.TicketID && left.ApprovalID == right.ApprovalID &&
		strings.TrimSpace(left.AuthoritySnapshotID) == strings.TrimSpace(right.AuthoritySnapshotID) &&
		strings.TrimSpace(left.DispatchID) == strings.TrimSpace(right.DispatchID) &&
		strings.TrimSpace(left.DependencyKind) == strings.TrimSpace(right.DependencyKind) &&
		strings.TrimSpace(left.SubjectRef) == strings.TrimSpace(right.SubjectRef) &&
		strings.TrimSpace(left.RequestedAction) == strings.TrimSpace(right.RequestedAction) &&
		strings.TrimSpace(left.RiskLevel) == strings.TrimSpace(right.RiskLevel) &&
		strings.TrimSpace(left.AuthorizationHash) == strings.TrimSpace(right.AuthorizationHash)
}

// RequestRecordConflicts reports whether two different request records claim
// the same ticket or the same authority/dispatch identity.
func RequestRecordConflicts(left, right runtimecontract.RuntimeGovernanceRequestRecord) bool {
	if SameRequestRecord(left, right) {
		return false
	}
	return (left.TicketID != "" && left.TicketID == right.TicketID) ||
		(left.ApprovalID != "" && left.ApprovalID == right.ApprovalID) ||
		(strings.TrimSpace(left.AuthoritySnapshotID) == strings.TrimSpace(right.AuthoritySnapshotID) &&
			strings.TrimSpace(left.DispatchID) == strings.TrimSpace(right.DispatchID))
}

// RequestBindsResolution compares a persisted request with a typed resolution
// before Runtime mutates any governance binding or evidence ledger.
func RequestBindsResolution(request runtimecontract.RuntimeGovernanceRequestRecord, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	return request.TicketID == resolution.TicketID && request.ApprovalID == resolution.ApprovalID &&
		strings.TrimSpace(request.AuthoritySnapshotID) == strings.TrimSpace(resolution.AuthoritySnapshotID) &&
		strings.TrimSpace(request.DispatchID) == strings.TrimSpace(resolution.DispatchID) &&
		DependencyMatches(request.DependencyKind, request.SubjectRef, resolution.DependencyKind, resolution.SubjectRef)
}

// RequestsBindResolution keeps protocol adapters from reimplementing the
// persisted owner-binding scan when they verify an idempotent resolution.
func RequestsBindResolution(values []runtimecontract.RuntimeGovernanceRequestRecord, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	for _, request := range values {
		if RequestBindsResolution(request, resolution) {
			return true
		}
	}
	return false
}

// SuspendedBindingMatchesRequest identifies the only operation binding that a
// newly observed ticket may promote. The promotion itself remains in Runtime.
func SuspendedBindingMatchesRequest(binding runtimecontract.RuntimeOperationGovernanceBinding, request runtimecontract.RuntimeGovernanceRequestRecord) bool {
	binding = binding.Normalized()
	return binding.State == runtimecontract.RuntimeOperationGovernanceBindingSuspended &&
		DependencyMatches(binding.DependencyKind, binding.SubjectRef, request.DependencyKind, request.SubjectRef) &&
		binding.RequestedAction == strings.TrimSpace(request.RequestedAction) && binding.RiskLevel == strings.TrimSpace(request.RiskLevel) &&
		binding.AuthorizationHash == strings.TrimSpace(request.AuthorizationHash)
}

// BindingMatchesResolution compares only the immutable governance identity;
// outcome/state validation is kept separate so callers cannot accidentally
// turn a comparison into a transition.
func BindingMatchesResolution(binding runtimecontract.RuntimeOperationGovernanceBinding, resolution runtimecontract.RuntimeGovernanceResolution) bool {
	binding = binding.Normalized()
	return binding.TicketID == resolution.TicketID && binding.ApprovalID == resolution.ApprovalID &&
		strings.TrimSpace(binding.GovernanceDispatchID) == strings.TrimSpace(resolution.DispatchID) &&
		strings.TrimSpace(binding.GovernanceSnapshotID) == strings.TrimSpace(resolution.AuthoritySnapshotID)
}

func BindingMatchesExecution(binding runtimecontract.RuntimeOperationGovernanceBinding, execution runtimecontract.RuntimeOperationExecutionRecord) bool {
	binding = binding.Normalized()
	execution = execution.Normalized()
	return execution.State == runtimecontract.OperationExecutionOwnerCommitted &&
		execution.OperationID == binding.OperationID && execution.ExecutionKey == binding.ExecutionKey && execution.Fence == binding.Fence &&
		execution.OwnerReceiptHash == binding.GovernanceReceiptHash
}

func BindingMatchesReceipt(binding runtimecontract.RuntimeOperationGovernanceBinding, receipt runtimecontract.RuntimeOperationReceipt) bool {
	binding = binding.Normalized()
	receipt = receipt.Normalized()
	return receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired &&
		receipt.ExecutionKey == binding.ExecutionKey && receipt.Fence == binding.Fence && receipt.OperationID == binding.OperationID &&
		runtimecontract.StableRuntimeOperationReceiptHash(receipt) == binding.GovernanceReceiptHash
}

func BindingMatchesRequest(binding runtimecontract.RuntimeOperationGovernanceBinding, request runtimecontract.RuntimeGovernanceRequestRecord) bool {
	binding = binding.Normalized()
	return binding.TicketID == request.TicketID && binding.ApprovalID == request.ApprovalID &&
		strings.TrimSpace(binding.GovernanceDispatchID) == strings.TrimSpace(request.DispatchID) &&
		strings.TrimSpace(binding.GovernanceSnapshotID) == strings.TrimSpace(request.AuthoritySnapshotID) &&
		DependencyMatches(binding.DependencyKind, binding.SubjectRef, request.DependencyKind, request.SubjectRef) &&
		binding.RequestedAction == strings.TrimSpace(request.RequestedAction) && binding.RiskLevel == strings.TrimSpace(request.RiskLevel) &&
		binding.AuthorizationHash == strings.TrimSpace(request.AuthorizationHash)
}

func ResolutionRecordMatchesRequest(resolution runtimecontract.RuntimeGovernanceResolutionRecord, request runtimecontract.RuntimeGovernanceRequestRecord) bool {
	return resolution.TicketID == request.TicketID && resolution.ApprovalID == request.ApprovalID &&
		strings.TrimSpace(resolution.DispatchID) == strings.TrimSpace(request.DispatchID) &&
		strings.TrimSpace(resolution.AuthoritySnapshotID) == strings.TrimSpace(request.AuthoritySnapshotID)
}

func ResolutionRecordMatchesDispatch(resolution runtimecontract.RuntimeGovernanceResolutionRecord, facts DispatchFacts) bool {
	return facts.CandidateGovernance && DispatchIdentityMatches(facts, resolution.DispatchID, resolution.AuthoritySnapshotID)
}

// GrantForExecution finds the only persisted operation-scoped grant that can
// authorize the requested resume fence. The returned value is detached from
// the aggregate so protocol adapters cannot mutate owner state through it.
func GrantForExecution(bindings []runtimecontract.RuntimeOperationGovernanceBinding, execution runtimecontract.RuntimeOperationExecutionRecord) *runtimecontract.RuntimeOperationGovernanceGrant {
	execution = execution.Normalized()
	for _, raw := range bindings {
		binding := raw.Normalized()
		if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil || binding.ExecutionKey != execution.ExecutionKey {
			continue
		}
		grant := binding.Grant.Normalized()
		if grant.ResumeFence != execution.Fence {
			continue
		}
		grant.EvidenceRefs = append([]string(nil), grant.EvidenceRefs...)
		return &grant
	}
	return nil
}

func SameGrant(left, right runtimecontract.RuntimeOperationGovernanceGrant) bool {
	return runtimecontract.SameRuntimeOperationGovernanceGrant(left, right)
}

// EvaluateOperationSuspension proves that the owner response, durable receipt
// and persisted execution all describe the same pre-effect governance stop.
// It returns only detached binding/dependency facts; it does not close a
// dispatch, write failure authority, run a reducer, or select a next action.
func EvaluateOperationSuspension(
	dispatchID string,
	execution runtimecontract.RuntimeOperationExecutionRecord,
	persistedReceipt runtimecontract.RuntimeOperationReceipt,
	observedReceipt runtimecontract.RuntimeOperationReceipt,
	requirement runtimecontract.RuntimeGovernanceRequirement,
) (OperationSuspensionFinding, error) {
	dispatchID = strings.TrimSpace(dispatchID)
	execution = execution.Normalized()
	persistedReceipt = persistedReceipt.Normalized()
	observedReceipt = observedReceipt.Normalized()
	requirement = requirement.Normalized()
	if dispatchID == "" {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance suspension requires dispatch identity")
	}
	if err := execution.Validate(); err != nil {
		return OperationSuspensionFinding{}, err
	}
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance suspension requires owner_committed execution")
	}
	if err := persistedReceipt.Validate(); err != nil {
		return OperationSuspensionFinding{}, err
	}
	if err := observedReceipt.Validate(); err != nil {
		return OperationSuspensionFinding{}, err
	}
	if err := requirement.Validate(); err != nil {
		return OperationSuspensionFinding{}, err
	}
	if persistedReceipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired ||
		!runtimecontract.SameRuntimeOperationReceipt(persistedReceipt, observedReceipt) {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance suspension requires one exact durable governance receipt")
	}
	if persistedReceipt.ExecutionKey != execution.ExecutionKey || persistedReceipt.Fence != execution.Fence || persistedReceipt.OperationID != execution.OperationID {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance receipt does not match owner-committed execution")
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(persistedReceipt)
	if execution.OwnerReceiptHash != receiptHash {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance receipt hash does not match owner-committed execution")
	}
	if persistedReceipt.Governance == nil || !runtimecontract.SameRuntimeGovernanceRequirement(*persistedReceipt.Governance, requirement) {
		return OperationSuspensionFinding{}, fmt.Errorf("operation governance requirement does not match durable receipt")
	}
	finding := OperationSuspensionFinding{
		Binding: runtimecontract.RuntimeOperationGovernanceBinding{
			OperationDispatchID:   dispatchID,
			OperationID:           execution.OperationID,
			ExecutionKey:          execution.ExecutionKey,
			Fence:                 execution.Fence,
			ExpectedEffectScope:   execution.ExpectedEffectScope,
			DependencyKind:        string(requirement.Kind),
			SubjectRef:            requirement.SubjectRef,
			RequestedAction:       requirement.RequestedAction,
			RiskLevel:             requirement.RiskLevel,
			AuthorizationHash:     runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
			GovernanceReceiptHash: receiptHash,
			State:                 runtimecontract.RuntimeOperationGovernanceBindingSuspended,
		},
		Dependency: runtimecontract.ExternalDependency{
			Kind: string(requirement.Kind), SubjectRef: requirement.SubjectRef, Reason: requirement.Reason,
			RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
			AuthorizationContext: requirement.AuthorizationContext.Clone(),
		},
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceBindings([]runtimecontract.RuntimeOperationGovernanceBinding{finding.Binding}); err != nil {
		return OperationSuspensionFinding{}, err
	}
	if err := finding.Dependency.Validate(); err != nil {
		return OperationSuspensionFinding{}, err
	}
	return finding, nil
}

// DependencyForFacts derives the single typed dependency that a governance
// dispatch may seal. Operation-scoped suspension facts take precedence over a
// generic failure dependency; provider authentication/configuration is the
// only allowed fallback. No generic "decision" dependency is invented.
func DependencyForFacts(facts runtimecontract.RuntimeFacts, bindings []runtimecontract.RuntimeOperationGovernanceBinding) (runtimecontract.ExternalDependency, bool) {
	facts = facts.Normalized()
	for idx := len(bindings) - 1; idx >= 0; idx-- {
		binding := bindings[idx].Normalized()
		if binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		dependency := runtimecontract.ExternalDependency{
			Kind: binding.DependencyKind, SubjectRef: binding.SubjectRef,
			RequestedAction: binding.RequestedAction, RiskLevel: binding.RiskLevel,
		}
		if facts.Failure != nil && facts.Failure.GovernanceDependency != nil {
			candidate := facts.Failure.GovernanceDependency.Normalized()
			if !DependencyMatches(dependency.Kind, dependency.SubjectRef, candidate.Kind, candidate.SubjectRef) {
				return runtimecontract.ExternalDependency{}, false
			}
			dependency.Reason = candidate.Reason
			dependency.RequestedAction = candidate.RequestedAction
			dependency.RiskLevel = candidate.RiskLevel
			dependency.AuthorizationContext = candidate.AuthorizationContext.Clone()
		}
		if dependency.Reason == "" && facts.Failure != nil {
			dependency.Reason = strings.TrimSpace(facts.Failure.Message)
		}
		dependency = dependency.Normalized()
		if dependency.Validate() == nil {
			return dependency, true
		}
		return runtimecontract.ExternalDependency{}, false
	}
	if facts.Failure != nil && facts.Failure.GovernanceDependency != nil {
		dependency := facts.Failure.GovernanceDependency.Normalized()
		if dependency.Validate() == nil {
			return dependency, true
		}
		return runtimecontract.ExternalDependency{}, false
	}
	if facts.Failure == nil ||
		(facts.Failure.Class != runtimecontract.FailureClassProviderAuthentication && facts.Failure.Class != runtimecontract.FailureClassProviderConfiguration) ||
		facts.Resources.ProviderCeiling == nil {
		return runtimecontract.ExternalDependency{}, false
	}
	target := facts.Resources.ProviderCeiling.Selection.Target.Normalized()
	if target.ProviderID == "" || target.ModelID == "" {
		return runtimecontract.ExternalDependency{}, false
	}
	reason := "provider configuration requires attention"
	if facts.Failure.Class == runtimecontract.FailureClassProviderAuthentication {
		reason = "provider authentication requires configuration"
	}
	return runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceConfiguration), SubjectRef: target.ProviderID + "/" + target.ModelID, Reason: reason,
	}, true
}

func GovernedDispatchMatches(facts DispatchFacts, dispatchID, authoritySnapshotID string) bool {
	return facts.DecisionState == runtimecontract.ConvergenceStateNeedGovernance && facts.CandidateGovernance &&
		DispatchIdentityMatches(facts, dispatchID, authoritySnapshotID)
}

func RequestMatches(request runtimecontract.RuntimeGovernanceRequestRecord, facts DispatchFacts) bool {
	return GovernedDispatchMatches(facts, request.DispatchID, request.AuthoritySnapshotID) &&
		DependencyMatches(facts.DependencyKind, facts.SubjectRef, request.DependencyKind, request.SubjectRef)
}

func ResolutionOutcomeSupported(outcome runtimecontract.RuntimeGovernanceResolutionOutcome) bool {
	return outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeContinue || outcome == runtimecontract.RuntimeGovernanceResolutionOutcomeFail
}

func BindingStateAllowsOutcome(state runtimecontract.RuntimeOperationGovernanceBindingState, outcome runtimecontract.RuntimeGovernanceResolutionOutcome) bool {
	if state != runtimecontract.RuntimeOperationGovernanceBindingTicketBound {
		return false
	}
	return ResolutionOutcomeSupported(outcome)
}
