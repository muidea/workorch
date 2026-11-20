package workorchruntimecontract

// RuntimeStateTransitionDiff summarizes field-level changes between two
// RuntimeStateRecord values so a single CAS pass evaluates committed-history,
// source-session, owner-reload, and transition validators without re-comparing
// unchanged subtrees. It is strictly scoped to one transition; nothing caches
// or persists validation results across states.
type RuntimeStateTransitionDiff struct {
	PreviousAvailable bool

	Definition                      bool
	CoverageProofLineage            bool
	OperationReceipts               bool
	ProcessedEventIDs               bool
	CompletedDispatches             bool
	Reconciliation                  bool
	CandidateAttempts               bool
	CandidateRequestSessions        bool
	SemanticPlanning                bool
	ExecutionBudget                 bool
	RepairSurfaceConfirmations      bool
	Diagnostics                     bool
	PrerequisiteInvalidations       bool
	GovernanceRequests              bool
	GovernanceResolutions           bool
	OperationGovernanceBindings     bool
	OperationRecoveryAuthorizations bool
	RecoverableWaits                bool
	DeliveryAcknowledgements        bool
	ImplementationWork              bool
	DefinitionClarification         bool
	TerminalMaterialSeal            bool
	ActiveDispatch                  bool
	Decision                        bool
	DecisionTerminal                bool
	DriveLease                      bool
	Termination                     bool
	ResultFeedbackVersion           bool
	StateRevision                   bool
	EvidenceSequence                bool

	FactsScope                             bool
	FactsDefinitionRevision                bool
	FactsCWILedgerSchemaVersion            bool
	FactsDefinitionSeed                    bool
	FactsDefinitionFocuses                 bool
	FactsActivationProfileContracts        bool
	FactsActivationEvidencePolicies        bool
	FactsActivationOther                   bool
	FactsPolicies                          bool
	FactsSlices                            bool
	FactsFocusQueues                       bool
	FactsAssessments                       bool
	FactsEvidenceAccepted                  bool
	FactsEvidenceRejected                  bool
	FactsEvidenceApplicabilityObservations bool
	FactsCapabilities                      bool
	FactsReconciliationContract            bool
	FactsCoverage                          bool
	FactsCWISuspensions                    bool
	FactsResourcesOperationAdmissibilities bool
	FactsResourcesOther                    bool
	FactsPrerequisites                     bool
	FactsEvidenceRevision                  bool
	FactsFeasibility                       bool
	FactsFailure                           bool
	FactsRepair                            bool
	FactsRepairDirective                   bool
	FactsRecovery                          bool
	FactsStateChange                       bool
}

func (d RuntimeStateTransitionDiff) FactsActivation() bool {
	return d.FactsActivationProfileContracts || d.FactsActivationEvidencePolicies || d.FactsActivationOther
}

func (d RuntimeStateTransitionDiff) FactsEvidence() bool {
	return d.FactsEvidenceAccepted || d.FactsEvidenceRejected
}

func (d RuntimeStateTransitionDiff) FactsResources() bool {
	return d.FactsResourcesOperationAdmissibilities || d.FactsResourcesOther
}

func (d RuntimeStateTransitionDiff) FactsCore() bool {
	return !d.PreviousAvailable ||
		d.FactsScope || d.FactsDefinitionRevision || d.FactsCWILedgerSchemaVersion ||
		d.FactsDefinitionSeed || d.FactsDefinitionFocuses || d.FactsCapabilities || d.FactsActivation() || d.FactsPolicies ||
		d.FactsFocusQueues || d.FactsAssessments || d.FactsEvidence() || d.FactsCoverage ||
		d.FactsReconciliationContract || d.FactsFeasibility || d.FactsFailure || d.FactsRepair ||
		d.FactsRepairDirective || d.FactsRecovery || d.FactsStateChange || d.FactsPrerequisites ||
		d.FactsResources() || d.FactsEvidenceRevision
}

func (d RuntimeStateTransitionDiff) FactsAny() bool {
	return d.FactsCore() || d.FactsSlices || d.FactsEvidenceApplicabilityObservations || d.FactsCWISuspensions
}

// FullRuntimeStateTransitionDiff returns a diff with every change flag enabled
// and PreviousAvailable=false, matching an initial state without a preimage.
func FullRuntimeStateTransitionDiff() RuntimeStateTransitionDiff {
	return RuntimeStateTransitionDiff{
		PreviousAvailable: false,

		Definition:                      true,
		CoverageProofLineage:            true,
		OperationReceipts:               true,
		ProcessedEventIDs:               true,
		CompletedDispatches:             true,
		Reconciliation:                  true,
		CandidateAttempts:               true,
		CandidateRequestSessions:        true,
		SemanticPlanning:                true,
		ExecutionBudget:                 true,
		RepairSurfaceConfirmations:      true,
		Diagnostics:                     true,
		PrerequisiteInvalidations:       true,
		GovernanceRequests:              true,
		GovernanceResolutions:           true,
		OperationGovernanceBindings:     true,
		OperationRecoveryAuthorizations: true,
		RecoverableWaits:                true,
		DeliveryAcknowledgements:        true,
		ImplementationWork:              true,
		DefinitionClarification:         true,
		TerminalMaterialSeal:            true,
		ActiveDispatch:                  true,
		Decision:                        true,
		DecisionTerminal:                true,
		DriveLease:                      true,
		Termination:                     true,
		ResultFeedbackVersion:           true,
		StateRevision:                   true,
		EvidenceSequence:                true,

		FactsScope:                             true,
		FactsDefinitionRevision:                true,
		FactsCWILedgerSchemaVersion:            true,
		FactsDefinitionSeed:                    true,
		FactsDefinitionFocuses:                 true,
		FactsActivationProfileContracts:        true,
		FactsActivationEvidencePolicies:        true,
		FactsActivationOther:                   true,
		FactsPolicies:                          true,
		FactsSlices:                            true,
		FactsFocusQueues:                       true,
		FactsAssessments:                       true,
		FactsEvidenceAccepted:                  true,
		FactsEvidenceRejected:                  true,
		FactsEvidenceApplicabilityObservations: true,
		FactsCapabilities:                      true,
		FactsReconciliationContract:            true,
		FactsCoverage:                          true,
		FactsCWISuspensions:                    true,
		FactsResourcesOperationAdmissibilities: true,
		FactsResourcesOther:                    true,
		FactsPrerequisites:                     true,
		FactsEvidenceRevision:                  true,
		FactsFeasibility:                       true,
		FactsFailure:                           true,
		FactsRepair:                            true,
		FactsRepairDirective:                   true,
		FactsRecovery:                          true,
		FactsStateChange:                       true,
	}
}

// ComputeRuntimeStateTransitionDiff computes field-level changes between
// previous and next in a single pass.
func ComputeRuntimeStateTransitionDiff(previous, next RuntimeStateRecord) RuntimeStateTransitionDiff {
	diff := RuntimeStateTransitionDiff{
		PreviousAvailable:               true,
		Definition:                      !runtimeRawEqual(previous.Definition, next.Definition),
		CoverageProofLineage:            !runtimeRawEqual(previous.CoverageProofLineage, next.CoverageProofLineage),
		OperationReceipts:               !runtimeRawEqual(previous.OperationReceipts, next.OperationReceipts),
		ProcessedEventIDs:               !runtimeRawEqual(previous.ProcessedEventIDs, next.ProcessedEventIDs),
		CompletedDispatches:             !runtimeRawEqual(previous.CompletedDispatches, next.CompletedDispatches),
		Reconciliation:                  !runtimeRawEqual(previous.Reconciliation, next.Reconciliation),
		CandidateAttempts:               !runtimeRawEqual(previous.CandidateAttempts, next.CandidateAttempts),
		CandidateRequestSessions:        !runtimeRawEqual(previous.CandidateRequestSessions, next.CandidateRequestSessions),
		SemanticPlanning:                !runtimeRawEqual(previous.SemanticPlanning, next.SemanticPlanning),
		ExecutionBudget:                 !runtimeRawEqual(previous.ExecutionBudget, next.ExecutionBudget),
		RepairSurfaceConfirmations:      !runtimeRawEqual(previous.RepairSurfaceConfirmations, next.RepairSurfaceConfirmations),
		Diagnostics:                     !runtimeRawEqual(previous.Diagnostics, next.Diagnostics),
		PrerequisiteInvalidations:       !runtimeRawEqual(previous.PrerequisiteInvalidations, next.PrerequisiteInvalidations),
		GovernanceRequests:              !runtimeRawEqual(previous.GovernanceRequests, next.GovernanceRequests),
		GovernanceResolutions:           !runtimeRawEqual(previous.GovernanceResolutions, next.GovernanceResolutions),
		OperationGovernanceBindings:     !runtimeRawEqual(previous.OperationGovernanceBindings, next.OperationGovernanceBindings),
		OperationRecoveryAuthorizations: !runtimeRawEqual(previous.OperationRecoveryAuthorizations, next.OperationRecoveryAuthorizations),
		RecoverableWaits:                !runtimeRawEqual(previous.RecoverableWaits, next.RecoverableWaits),
		DeliveryAcknowledgements:        !runtimeRawEqual(previous.DeliveryAcknowledgements, next.DeliveryAcknowledgements),
		ImplementationWork:              !runtimeRawEqual(previous.ImplementationWork, next.ImplementationWork),
		DefinitionClarification:         !runtimeRawEqual(previous.DefinitionClarification, next.DefinitionClarification),
		TerminalMaterialSeal:            !runtimeRawEqual(previous.TerminalMaterialSeal, next.TerminalMaterialSeal),
		ActiveDispatch:                  !runtimeRawEqual(previous.ActiveDispatch, next.ActiveDispatch),
		DecisionTerminal:                !runtimeRawEqual(previous.Decision.Terminal, next.Decision.Terminal),
		DriveLease:                      !runtimeRawEqual(previous.DriveLease, next.DriveLease),
		Termination:                     !runtimeRawEqual(previous.Termination, next.Termination),
		ResultFeedbackVersion:           previous.ResultFeedbackVersion != next.ResultFeedbackVersion,
		StateRevision:                   previous.StateRevision != next.StateRevision,
		EvidenceSequence:                previous.EvidenceSequence != next.EvidenceSequence,

		FactsScope:                  previous.Facts.WorkspaceID != next.Facts.WorkspaceID || previous.Facts.RunID != next.Facts.RunID,
		FactsDefinitionRevision:     previous.Facts.DefinitionRevision != next.Facts.DefinitionRevision,
		FactsCWILedgerSchemaVersion: previous.Facts.CWILedgerSchemaVersion != next.Facts.CWILedgerSchemaVersion,
		FactsEvidenceRevision:       previous.Facts.EvidenceRevision != next.Facts.EvidenceRevision,
		FactsDefinitionSeed:         !runtimeRawEqual(previous.Facts.DefinitionSeed, next.Facts.DefinitionSeed),
		FactsDefinitionFocuses:      !runtimeRawEqual(previous.Facts.DefinitionFocuses, next.Facts.DefinitionFocuses),
		FactsCapabilities:           !runtimeRawEqual(previous.Facts.Capabilities, next.Facts.Capabilities),
		FactsActivationProfileContracts: !runtimeRawEqual(
			previous.Facts.Activation.ExecutionContract.ProfileContracts,
			next.Facts.Activation.ExecutionContract.ProfileContracts,
		),
		FactsActivationEvidencePolicies: !runtimeRawEqual(
			previous.Facts.Activation.EvidencePolicies,
			next.Facts.Activation.EvidencePolicies,
		),
		FactsPolicies: !runtimeRawEqual(previous.Facts.ConvergencePolicy, next.Facts.ConvergencePolicy) ||
			!runtimeRawEqual(previous.Facts.RecoveryPolicy, next.Facts.RecoveryPolicy) ||
			!runtimeRawEqual(previous.Facts.ExecutionBudgetPolicy, next.Facts.ExecutionBudgetPolicy),
		FactsSlices: !runtimeRawEqual(previous.Facts.Slices, next.Facts.Slices),
		FactsFocusQueues: !runtimeRawEqual(previous.Facts.Focus, next.Facts.Focus) ||
			!runtimeRawEqual(previous.Facts.RemainingFocuses, next.Facts.RemainingFocuses) ||
			!runtimeRawEqual(previous.Facts.CompletedFocuses, next.Facts.CompletedFocuses),
		FactsAssessments:                       !runtimeRawEqual(previous.Facts.Assessments, next.Facts.Assessments),
		FactsEvidenceAccepted:                  !runtimeRawEqual(previous.Facts.Evidence.Accepted, next.Facts.Evidence.Accepted),
		FactsEvidenceRejected:                  !runtimeRawEqual(previous.Facts.Evidence.Rejected, next.Facts.Evidence.Rejected),
		FactsEvidenceApplicabilityObservations: !runtimeRawEqual(previous.Facts.EvidenceApplicabilityObservations, next.Facts.EvidenceApplicabilityObservations),
		FactsReconciliationContract:            !runtimeRawEqual(previous.Facts.ReconciliationContract, next.Facts.ReconciliationContract),
		FactsCoverage:                          !runtimeRawEqual(previous.Facts.Coverage, next.Facts.Coverage),
		FactsCWISuspensions:                    !runtimeRawEqual(previous.Facts.CWISuspensions, next.Facts.CWISuspensions),
		FactsResourcesOperationAdmissibilities: !runtimeRawEqual(previous.Facts.Resources.OperationAdmissibilities, next.Facts.Resources.OperationAdmissibilities),
		FactsPrerequisites:                     !runtimeRawEqual(previous.Facts.Prerequisites, next.Facts.Prerequisites),
		FactsFeasibility:                       !runtimeRawEqual(previous.Facts.Feasibility, next.Facts.Feasibility),
		FactsFailure:                           !runtimeRawEqual(previous.Facts.Failure, next.Facts.Failure),
		FactsRepair:                            !runtimeRawEqual(previous.Facts.Repair, next.Facts.Repair),
		FactsRepairDirective:                   !runtimeRawEqual(previous.Facts.RepairDirective, next.Facts.RepairDirective),
		FactsRecovery:                          !runtimeRawEqual(previous.Facts.Recovery, next.Facts.Recovery),
		FactsStateChange:                       !runtimeRawEqual(previous.Facts.StateChange, next.Facts.StateChange),
	}
	if diff.DecisionTerminal {
		diff.Decision = true
	} else {
		prevDecision := previous.Decision
		prevDecision.Terminal = next.Decision.Terminal
		diff.Decision = !runtimeRawEqual(prevDecision, next.Decision)
	}
	prevActivation := previous.Facts.Activation
	prevActivation.ExecutionContract.ProfileContracts = next.Facts.Activation.ExecutionContract.ProfileContracts
	prevActivation.EvidencePolicies = next.Facts.Activation.EvidencePolicies
	diff.FactsActivationOther = !runtimeRawEqual(prevActivation, next.Facts.Activation)

	prevResources := previous.Facts.Resources
	prevResources.OperationAdmissibilities = next.Facts.Resources.OperationAdmissibilities
	diff.FactsResourcesOther = !runtimeRawEqual(prevResources, next.Facts.Resources)
	return diff
}
