package workspace

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func preparationRecordFixture() WorkspacePreparationRecord {
	return WorkspacePreparationRecord{Version: WorkspacePreparationRecordVersion, WorkspaceID: "ws", Attempts: []WorkspacePreparationAttempt{{
		RequestID: "request", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 2,
		Binding: WorkspacePreparationBinding{
			AuthoringRef: DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"},
			CandidateID:  "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup",
			AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "checks", CompilerVersion: "compiler",
		},
		StartedAt: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), State: WorkspacePreparationAccepted,
	}}}
}

func TestPreparationRecordBoundedIdentityAndStates(t *testing.T) {
	for _, state := range []WorkspacePreparationAttemptState{WorkspacePreparationAccepted, WorkspacePreparationUnknown, WorkspacePreparationBlocked, WorkspacePreparationFailed} {
		record := preparationRecordFixture()
		record.Attempts[0].State = state
		if state == WorkspacePreparationBlocked || state == WorkspacePreparationFailed {
			record.Attempts[0].CompletedAt = record.Attempts[0].StartedAt.Add(time.Second)
			record.Attempts[0].ExpiresAt = record.Attempts[0].CompletedAt.Add(time.Minute)
		}
		if err := record.Validate("ws"); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
	}
	for name, mutate := range map[string]func(*WorkspacePreparationRecord){
		"version":   func(r *WorkspacePreparationRecord) { r.Version = "other" },
		"scope":     func(r *WorkspacePreparationRecord) { r.WorkspaceID = "other" },
		"empty":     func(r *WorkspacePreparationRecord) { r.Attempts = nil },
		"duplicate": func(r *WorkspacePreparationRecord) { r.Attempts = append(r.Attempts, r.Attempts[0]) },
		"unordered_revision": func(r *WorkspacePreparationRecord) {
			copy := r.Attempts[0]
			copy.RequestID = "second"
			r.Attempts = append(r.Attempts, copy)
		},
		"no_cas":          func(r *WorkspacePreparationRecord) { r.Attempts[0].CommittedWorkspaceRevision = 1 },
		"zero_expected":   func(r *WorkspacePreparationRecord) { r.Attempts[0].ExpectedWorkspaceRevision = 0 },
		"unbounded":       func(r *WorkspacePreparationRecord) { r.Attempts[0].RequestID = strings.Repeat("r", 513) },
		"whitespace":      func(r *WorkspacePreparationRecord) { r.Attempts[0].Binding.AuthoringRef.InputHash = " input" },
		"missing_binding": func(r *WorkspacePreparationRecord) { r.Attempts[0].Binding.ManifestHash = "" },
		"zero_time":       func(r *WorkspacePreparationRecord) { r.Attempts[0].StartedAt = time.Time{} },
		"unknown_settled": func(r *WorkspacePreparationRecord) {
			r.Attempts[0].State = WorkspacePreparationUnknown
			r.Attempts[0].CompletedAt = r.Attempts[0].StartedAt
		},
		"accepted_expiry": func(r *WorkspacePreparationRecord) {
			r.Attempts[0].ExpiresAt = r.Attempts[0].StartedAt.Add(time.Minute)
		},
		"blocked_no_completion": func(r *WorkspacePreparationRecord) { r.Attempts[0].State = WorkspacePreparationBlocked },
		"false_ready":           func(r *WorkspacePreparationRecord) { r.Attempts[0].State = "ready" },
		"false_m3":              func(r *WorkspacePreparationRecord) { r.Attempts[0].State = "completed" },
	} {
		t.Run(name, func(t *testing.T) {
			record := preparationRecordFixture()
			mutate(&record)
			if record.Validate("ws") == nil {
				t.Fatal("invalid operation ledger accepted")
			}
		})
	}
	record := preparationRecordFixture()
	for n := 1; n < WorkspacePreparationAttemptLimit; n++ {
		attempt := record.Attempts[0]
		attempt.RequestID = fmt.Sprintf("request-%d", n)
		attempt.ExpectedWorkspaceRevision += uint64(n)
		attempt.CommittedWorkspaceRevision += uint64(n)
		record.Attempts = append(record.Attempts, attempt)
	}
	if record.Validate("ws") != nil {
		t.Fatal("bounded record rejected")
	}
	clone := record.Clone()
	clone.Attempts[0].Binding.CandidateHash = "caller mutation"
	if record.Attempts[0].Binding.CandidateHash == "caller mutation" {
		t.Fatal("record clone aliases binding")
	}
	attempt := record.Attempts[len(record.Attempts)-1]
	attempt.RequestID = "overflow"
	attempt.ExpectedWorkspaceRevision++
	attempt.CommittedWorkspaceRevision++
	record.Attempts = append(record.Attempts, attempt)
	if record.Validate("ws") == nil || len(record.Attempts) != WorkspacePreparationAttemptLimit+1 {
		t.Fatal("overflow accepted or silently truncated")
	}
}
