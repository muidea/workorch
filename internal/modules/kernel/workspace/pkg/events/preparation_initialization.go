package events

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

const TopicWorkspacePreparationInitializeResources = "/workspace/activation-preparation/resources/initialize"
const TopicWorkspacePreparationRegistrationSettle = "/workspace/activation-preparation/resources/registration-settle"
const TopicWorkspacePreparationRecoverResources = "/workspace/activation-preparation/resources/recover"

type InitializeWorkspacePreparationCommand struct {
	RegistrationHome          string // Application-private scope; never accepted from HTTP.
	WorkspaceID               common.WorkspaceID
	RequestID                 string
	ExpectedWorkspaceRevision uint64
}

func (c InitializeWorkspacePreparationCommand) Validate() error {
	if len(c.RequestID) > 256 {
		return common.ErrInvalidArgument
	}
	return (CheckWorkspacePreparationCommand{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision}).Validate()
}

type InitializeWorkspacePreparationResult struct {
	RequestID                 string
	ExpectedWorkspaceRevision uint64
	State                     QueryWorkspacePreparationStateResult
	Registration              *PreparationRegistrationDispatch
}

func (r InitializeWorkspacePreparationResult) ValidateFor(c InitializeWorkspacePreparationCommand) error {
	if r.Registration != nil && (r.Registration.Plan.Validate() != nil || r.Registration.Plan.Home != c.RegistrationHome || r.Registration.Plan.RequestID != skillhubpreparation.RegistrationRequestID(string(c.WorkspaceID), c.RequestID) || r.Registration.Attempt < 1 || r.Registration.Attempt > skillhubpreparation.RegistrationAttemptLimit || r.Registration.Query != (r.Registration.Attempt > 1) || r.Registration.CommittedWorkspaceRevision != r.State.WorkspaceRevision) {
		return common.ErrConflict
	}
	if c.Validate() != nil || r.RequestID != c.RequestID || r.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || r.State.ValidateFor(QueryWorkspacePreparationStateCommand{WorkspaceID: c.WorkspaceID}) != nil {
		return common.ErrConflict
	}
	for _, v := range r.State.ResourceInitializations {
		if v.RequestID == c.RequestID && v.ExpectedWorkspaceRevision == c.ExpectedWorkspaceRevision {
			return nil
		}
	}
	return common.ErrConflict
}

// Only bounded metadata leaves the private initialization journal.
type PreparationInitializationView struct {
	RequestID                  string `json:"requestId"`
	ExpectedWorkspaceRevision  uint64 `json:"expectedWorkspaceRevision"`
	CommittedWorkspaceRevision uint64 `json:"committedWorkspaceRevision"`
	State                      string `json:"state"`
	Attempts                   int    `json:"attempts"`
	RegistrationState          string `json:"registrationState,omitempty"`
	RegistrationAttempts       int    `json:"registrationAttempts,omitempty"`
}

func (v PreparationInitializationView) Validate(revision uint64) error {
	if !ValidPreparationRegistrationView(v.RegistrationState, v.RegistrationAttempts) || v.State == "completed" && v.RegistrationState != "" && v.RegistrationState != "succeeded" || v.RegistrationState == "succeeded" && v.State != "completed" || (v.RegistrationState == "pending" || v.RegistrationState == "not_started") && v.State != "pending" || (v.RegistrationState == "failed" || v.RegistrationState == "unresolved" || v.RegistrationState == "unknown" || v.RegistrationState == "running") && v.State != "blocked" {
		return common.ErrConflict
	}
	if (CheckWorkspacePreparationCommand{WorkspaceID: "scope", RequestID: v.RequestID, ExpectedWorkspaceRevision: v.ExpectedWorkspaceRevision}).Validate() != nil || v.CommittedWorkspaceRevision <= v.ExpectedWorkspaceRevision || v.CommittedWorkspaceRevision > revision || v.Attempts < 1 || v.Attempts > workspace.PreparationInitializationResultLimit || v.State != "pending" && v.State != "blocked" && v.State != "completed" {
		return common.ErrConflict
	}
	return nil
}

// Private continuation, emitted only after the owner reservation CAS returns.
type PreparationRegistrationDispatch struct {
	Plan                       skillhubpreparation.RegistrationPlan
	Attempt                    int
	Query                      bool
	CommittedWorkspaceRevision uint64
}
type SettlePreparationRegistrationCommand struct {
	Original    InitializeWorkspacePreparationCommand
	Dispatch    PreparationRegistrationDispatch
	Observation skillhubpreparation.RegistrationObservation
}

func ValidPreparationRegistrationView(state string, n int) bool {
	if state == "" {
		return n == 0
	}
	if n < 0 || n > skillhubpreparation.RegistrationAttemptLimit {
		return false
	}
	switch state {
	case "not_started", "waiting_for_files":
		return n == 0
	case "pending", "unresolved", "running", "unknown", "failed", "succeeded":
		return n > 0
	}
	return false
}
