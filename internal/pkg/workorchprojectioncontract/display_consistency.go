package workorchprojectioncontract

// GovernanceDisplay is a read-only correlation of canonical owner state with
// open Decision/Approval resources. It is display data only: clients must not
// use it as a Run/Workspace mutation intent or as proof that SM-1 transitioned.
type GovernanceDisplay struct {
	PromptMode string `json:"promptMode,omitempty"`
	RunID      string `json:"runId,omitempty"`
	DecisionID string `json:"decisionId,omitempty"`
	ApprovalID string `json:"approvalId,omitempty"`
}

// ConsistencyDiagnostic describes a read-side mismatch between independently
// owned facts. Code is diagnostic, never a lifecycle state or transition.
type ConsistencyDiagnostic struct {
	Code                    string `json:"code,omitempty"`
	Summary                 string `json:"summary,omitempty"`
	CanonicalWorkspaceState string `json:"canonicalWorkspaceState,omitempty"`
	CanonicalWorkspaceRunID string `json:"canonicalWorkspaceRunId,omitempty"`
	CanonicalRunState       string `json:"canonicalRunState,omitempty"`
	CanonicalRunDecisionID  string `json:"canonicalRunDecisionId,omitempty"`
	CanonicalRunApprovalID  string `json:"canonicalRunApprovalId,omitempty"`
	RuntimeConvergenceState string `json:"runtimeConvergenceState,omitempty"`
	ObservedDecisionID      string `json:"observedDecisionId,omitempty"`
	ObservedApprovalID      string `json:"observedApprovalId,omitempty"`
}

func CloneConsistencyDiagnostic(item *ConsistencyDiagnostic) *ConsistencyDiagnostic {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}
