package workorchmanagementcontract

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// StartupPreparationView reports persisted operations only. In this first
// increment it cannot express readiness, M3, no-Run or activation permission.
// ActiveRunID is an independent Workspace fact, not an inventory proof.
type StartupPreparationView struct {
	WorkspaceID       string                 `json:"workspaceId"`
	WorkspaceRevision uint64                 `json:"workspaceRevision"`
	ActiveRunID       string                 `json:"activeRunId,omitempty"`
	OperationState    string                 `json:"operationState"`
	CurrentValidity   string                 `json:"currentValidity"`
	Record            *PreparationRecordView `json:"record,omitempty"`
}

type PreparationRecordView struct {
	Version  string                   `json:"version"`
	Attempts []PreparationAttemptView `json:"attempts"`
}

type PreparationAttemptView struct {
	RequestID                  string                 `json:"requestId"`
	PayloadHash                string                 `json:"payloadHash"`
	ExpectedWorkspaceRevision  uint64                 `json:"expectedWorkspaceRevision"`
	CommittedWorkspaceRevision uint64                 `json:"committedWorkspaceRevision"`
	Binding                    PreparationBindingView `json:"binding"`
	StartedAt                  time.Time              `json:"startedAt"`
	CompletedAt                *time.Time             `json:"completedAt,omitempty"`
	ExpiresAt                  *time.Time             `json:"expiresAt,omitempty"`
	State                      string                 `json:"state"`
}

type PreparationBindingView struct {
	InputHash                  string `json:"inputHash"`
	DerivationPermitID         string `json:"derivationPermitId"`
	ProposalHash               string `json:"proposalHash"`
	SpecificationHash          string `json:"specificationHash"`
	CandidateID                string `json:"candidateId"`
	CandidateRevision          uint64 `json:"candidateRevision"`
	CandidateHash              string `json:"candidateHash"`
	DefinitionRevision         string `json:"definitionRevision"`
	StartupInputHash           string `json:"startupInputHash"`
	AuthoringConfigurationHash string `json:"authoringConfigurationHash"`
	SnapshotHash               string `json:"snapshotHash"`
	ManifestHash               string `json:"manifestHash"`
	CheckPolicyVersion         string `json:"checkPolicyVersion"`
	CompilerVersion            string `json:"compilerVersion"`
}

func (v StartupPreparationView) ValidateFor(workspaceID string) error {
	if workspaceID == "" || v.WorkspaceID != workspaceID || v.WorkspaceRevision == 0 {
		return common.ErrConflict
	}
	if v.Record == nil {
		if v.OperationState != "not_checked" || v.CurrentValidity != "not_checked" {
			return common.ErrConflict
		}
		return nil
	}
	if v.Record.Version != "workorch-preparation-record/v1" || len(v.Record.Attempts) == 0 || len(v.Record.Attempts) > 64 || (v.CurrentValidity != "not_evaluated" && v.CurrentValidity != "stale") {
		return common.ErrConflict
	}
	lastRevision := uint64(0)
	seen := make(map[string]bool, len(v.Record.Attempts))
	for _, a := range v.Record.Attempts {
		if !preparationViewIdentity(a.RequestID) || !preparationViewIdentity(a.PayloadHash) || a.Binding.validate() != nil || seen[a.RequestID] || a.ExpectedWorkspaceRevision == 0 || a.CommittedWorkspaceRevision <= a.ExpectedWorkspaceRevision || a.CommittedWorkspaceRevision <= lastRevision || a.CommittedWorkspaceRevision > v.WorkspaceRevision || a.StartedAt.IsZero() {
			return common.ErrConflict
		}
		seen[a.RequestID] = true
		lastRevision = a.CommittedWorkspaceRevision
		switch a.State {
		case "accepted", "unknown":
			if a.CompletedAt != nil || a.ExpiresAt != nil {
				return common.ErrConflict
			}
		case "blocked", "failed":
			if a.CompletedAt == nil || a.CompletedAt.IsZero() || a.CompletedAt.Before(a.StartedAt) || a.ExpiresAt != nil && !a.ExpiresAt.After(*a.CompletedAt) {
				return common.ErrConflict
			}
		default:
			return common.ErrConflict
		}
	}
	if v.OperationState != v.Record.Attempts[len(v.Record.Attempts)-1].State {
		return common.ErrConflict
	}
	return nil
}

func (b PreparationBindingView) validate() error {
	if b.CandidateRevision == 0 {
		return common.ErrConflict
	}
	for _, value := range []string{b.InputHash, b.DerivationPermitID, b.ProposalHash, b.SpecificationHash} {
		if !preparationViewIdentity(value) || len(value) > 256 {
			return common.ErrConflict
		}
	}
	for _, value := range []string{b.CandidateID, b.CandidateHash, b.DefinitionRevision, b.StartupInputHash, b.AuthoringConfigurationHash, b.SnapshotHash, b.ManifestHash, b.CheckPolicyVersion, b.CompilerVersion} {
		if !preparationViewIdentity(value) {
			return common.ErrConflict
		}
	}
	return nil
}

func preparationViewIdentity(value string) bool {
	return value != "" && len(value) <= 512 && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0) && utf8.ValidString(value)
}
