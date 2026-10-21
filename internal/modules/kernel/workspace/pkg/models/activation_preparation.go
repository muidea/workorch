package workspace

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const WorkspacePreparationRecordVersion = "workorch-preparation-record/v1"
const WorkspacePreparationAttemptLimit = 64

// These are operation states, not Workspace lifecycle or activation permission.
// The first increment describes persisted trace snapshots only: it has no
// command/reducer that can create or transition these records, and cannot
// represent positive readiness or M3. The full observation/fence protocol and
// an explicit owner-local transition family are required before adding writes.
type WorkspacePreparationAttemptState string

const (
	WorkspacePreparationAccepted WorkspacePreparationAttemptState = "accepted"
	WorkspacePreparationBlocked  WorkspacePreparationAttemptState = "blocked"
	WorkspacePreparationFailed   WorkspacePreparationAttemptState = "failed"
	WorkspacePreparationUnknown  WorkspacePreparationAttemptState = "unknown"
)

type WorkspacePreparationBinding struct {
	AuthoringRef               DefinitionAuthoringAdmissionRef
	CandidateID                string
	CandidateRevision          uint64
	CandidateHash              string
	DefinitionRevision         string
	StartupInputHash           string
	AuthoringConfigurationHash string
	SnapshotHash               string
	ManifestHash               string
	CheckPolicyVersion         string
	CompilerVersion            string
}

type WorkspacePreparationAttempt struct {
	RequestID                  string
	PayloadHash                string
	ExpectedWorkspaceRevision  uint64
	CommittedWorkspaceRevision uint64
	Binding                    WorkspacePreparationBinding
	StartedAt                  time.Time
	CompletedAt                time.Time
	ExpiresAt                  time.Time
	State                      WorkspacePreparationAttemptState
}

// WorkspacePreparationRecord stores bounded operation identities only. Raw
// resources, credentials, provider clients and diagnostic text cannot enter it.
// Reads never create this record or resume an accepted/unknown attempt.
type WorkspacePreparationRecord struct {
	Version     string
	WorkspaceID common.WorkspaceID
	Attempts    []WorkspacePreparationAttempt
}

func (r WorkspacePreparationRecord) Clone() WorkspacePreparationRecord {
	r.Attempts = slices.Clone(r.Attempts)
	return r
}

func (r WorkspacePreparationRecord) Validate(workspaceID common.WorkspaceID) error {
	if r.Version != WorkspacePreparationRecordVersion || r.WorkspaceID != workspaceID || workspaceID == "" || len(r.Attempts) == 0 || len(r.Attempts) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	seen := make(map[string]struct{}, len(r.Attempts))
	lastRevision := uint64(0)
	for _, attempt := range r.Attempts {
		if _, present := seen[attempt.RequestID]; present || attempt.Validate() != nil || attempt.CommittedWorkspaceRevision <= lastRevision {
			return common.ErrConflict
		}
		seen[attempt.RequestID] = struct{}{}
		lastRevision = attempt.CommittedWorkspaceRevision
	}
	return nil
}

func (a WorkspacePreparationAttempt) Validate() error {
	if !boundedPreparationIdentity(a.RequestID) || !boundedPreparationIdentity(a.PayloadHash) || a.ExpectedWorkspaceRevision == 0 || a.CommittedWorkspaceRevision <= a.ExpectedWorkspaceRevision || a.StartedAt.IsZero() || a.Binding.Validate() != nil {
		return common.ErrConflict
	}
	switch a.State {
	case WorkspacePreparationAccepted, WorkspacePreparationUnknown:
		if !a.CompletedAt.IsZero() || !a.ExpiresAt.IsZero() {
			return common.ErrConflict
		}
	case WorkspacePreparationBlocked, WorkspacePreparationFailed:
		if a.CompletedAt.Before(a.StartedAt) || a.CompletedAt.IsZero() || !a.ExpiresAt.IsZero() && !a.ExpiresAt.After(a.CompletedAt) {
			return common.ErrConflict
		}
	default:
		return common.ErrConflict
	}
	return nil
}

func (b WorkspacePreparationBinding) Validate() error {
	if b.AuthoringRef.Validate() != nil || b.CandidateRevision == 0 {
		return common.ErrConflict
	}
	for _, value := range []string{b.AuthoringRef.InputHash, b.AuthoringRef.DerivationPermitID, b.AuthoringRef.ProposalHash, b.AuthoringRef.SpecificationHash, b.CandidateID, b.CandidateHash, b.DefinitionRevision, b.StartupInputHash, b.AuthoringConfigurationHash, b.SnapshotHash, b.ManifestHash, b.CheckPolicyVersion, b.CompilerVersion} {
		if !boundedPreparationIdentity(value) {
			return common.ErrConflict
		}
	}
	return nil
}

func boundedPreparationIdentity(value string) bool {
	return value != "" && len(value) <= 512 && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0) && utf8.ValidString(value)
}
