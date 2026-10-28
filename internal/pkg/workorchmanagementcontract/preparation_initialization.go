package workorchmanagementcontract

import "github.com/muidea/workorch/internal/pkg/common"

type PreparationResourceInitializationView struct {
	RequestID                  string `json:"requestId"`
	ExpectedWorkspaceRevision  uint64 `json:"expectedWorkspaceRevision"`
	CommittedWorkspaceRevision uint64 `json:"committedWorkspaceRevision"`
	State                      string `json:"state"`
	Attempts                   int    `json:"attempts"`
	RegistrationState          string `json:"registrationState,omitempty"`
	RegistrationAttempts       int    `json:"registrationAttempts,omitempty"`
}

func (v PreparationResourceInitializationView) Validate(revision uint64) error {
	valid := false
	switch v.RegistrationState {
	case "":
		valid = v.RegistrationAttempts == 0
	case "waiting_for_files", "not_started":
		valid = v.RegistrationAttempts == 0
	case "pending", "unresolved", "running", "unknown", "failed", "succeeded":
		valid = v.RegistrationAttempts > 0 && v.RegistrationAttempts <= 4
	}
	if !valid || v.State == "completed" && v.RegistrationState != "" && v.RegistrationState != "succeeded" || v.RegistrationState == "succeeded" && v.State != "completed" || (v.RegistrationState == "pending" || v.RegistrationState == "not_started") && v.State != "pending" || (v.RegistrationState == "failed" || v.RegistrationState == "unresolved" || v.RegistrationState == "unknown" || v.RegistrationState == "running") && v.State != "blocked" {
		return common.ErrConflict
	}

	if (PreparationCheckRequest{RequestID: v.RequestID, ExpectedWorkspaceRevision: v.ExpectedWorkspaceRevision}).Validate() != nil || v.CommittedWorkspaceRevision <= v.ExpectedWorkspaceRevision || v.CommittedWorkspaceRevision > revision || v.Attempts < 1 || v.Attempts > 4 || v.State != "pending" && v.State != "blocked" && v.State != "completed" {
		return common.ErrConflict
	}
	return nil
}

type PreparationResourceInitializationOperation struct {
	RequestID   string                 `json:"requestId"`
	Preparation StartupPreparationView `json:"preparation"`
}

func (v PreparationResourceInitializationOperation) ValidateFor(id string, c PreparationCheckRequest) error {
	if c.Validate() != nil || v.RequestID != c.RequestID || v.Preparation.ValidateFor(id) != nil {
		return common.ErrConflict
	}
	for _, r := range v.Preparation.ResourceInitializations {
		if r.RequestID == c.RequestID && r.ExpectedWorkspaceRevision == c.ExpectedWorkspaceRevision {
			return nil
		}
	}
	return common.ErrConflict
}
