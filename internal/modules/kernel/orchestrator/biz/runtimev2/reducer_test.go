package runtimev2

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReduceRuntimeStateLiveFailureFixtures(t *testing.T) {
	tests := []struct {
		name  string
		facts runtimecontract.RuntimeFacts
	}{
		{
			name: "lsp validation failure requires another state change",
			facts: runtimecontract.RuntimeFacts{
				WorkspaceID:        "ws-5cfc9874cebda0b8",
				RunID:              "run-006",
				DefinitionRevision: "definition-lsp",
				EvidenceRevision:   "evidence-lsp-build-failed",
				Focus: runtimecontract.CoverageFocus{
					ID:                    "cwi-036",
					CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-036"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
					TargetRefs:            []string{"server.go"},
				},
				Evidence: runtimecontract.EvidenceLedger{
					Accepted: []runtimecontract.EvidenceFact{{ID: "e-state", Kind: runtimecontract.EvidenceKindStateChange}},
					Rejected: []runtimecontract.EvidenceFact{{ID: "e-vet", Kind: runtimecontract.EvidenceKindValidation, FailureClass: runtimecontract.FailureClassBuildFailure, FailureMessage: "server.go:29:6: main redeclared in this block"}},
				},
				Capabilities: runtimecontract.RuntimeCapabilities{
					StateChangeKinds: []string{"patch/apply"},
					ValidationKinds:  []string{"shell/command", "validation/managed_session"},
				},
			},
		},
		{
			name: "gateway incomplete coverage requires state change",
			facts: runtimecontract.RuntimeFacts{
				WorkspaceID:        "ws-609ce9149734ebb7",
				RunID:              "run-007-replan-001",
				DefinitionRevision: "definition-gateway",
				EvidenceRevision:   "evidence-coverage-assessment-incomplete",
				Focus: runtimecontract.CoverageFocus{
					ID:                    "cwi-085",
					CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-085"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
					TargetRefs:            []string{"gateway"},
				},
				Evidence: runtimecontract.EvidenceLedger{
					Accepted: []runtimecontract.EvidenceFact{{ID: "e-validation", Kind: runtimecontract.EvidenceKindValidation}},
				},
				Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCoverageAssessmentIncomplete, Signature: "coverage_assessment_incomplete:cwi-085"},
				Capabilities: runtimecontract.RuntimeCapabilities{
					StateChangeKinds: []string{"patch/apply"},
					ValidationKinds:  []string{"shell/command"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReduceRuntimeState(tt.facts, nil)
			if err != nil {
				t.Fatalf("ReduceRuntimeState() error = %v", err)
			}
			if got.State != runtimecontract.ConvergenceStateNeedStateChange || got.Action != runtimecontract.RuntimeActionProduceStateChange {
				t.Fatalf("decision = %+v, want ProduceStateChange", got)
			}
			if got.RequiredOutput.Kind != runtimecontract.RequiredOutputKindStateChange || got.AuthoritySnapshot.RequiredOutputHash == "" {
				t.Fatalf("required output is not atomically sealed: %+v", got)
			}
		})
	}
}

func TestReduceRuntimeStateIsDeterministic(t *testing.T) {
	facts := validationFacts()
	left, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if left.AuthoritySnapshot != right.AuthoritySnapshot {
		t.Fatalf("snapshot changed for identical facts: left=%+v right=%+v", left.AuthoritySnapshot, right.AuthoritySnapshot)
	}
}

func TestReduceRuntimeStateProgression(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFacts)
		state  runtimecontract.ConvergenceState
		action runtimecontract.RuntimeAction
	}{
		{name: "missing prerequisite", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}
		}, state: runtimecontract.ConvergenceStateNeedPrerequisite, action: runtimecontract.RuntimeActionAcquirePrerequisite},
		{name: "validation", state: runtimecontract.ConvergenceStateNeedValidation, action: runtimecontract.RuntimeActionProduceValidation},
		{name: "complete", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "e-validation", Kind: runtimecontract.EvidenceKindValidation}}
		}, state: runtimecontract.ConvergenceStateComplete, action: runtimecontract.RuntimeActionComplete},
		{name: "governance", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency}
			f.Capabilities.GovernanceKinds = []string{"decision/resolution"}
		}, state: runtimecontract.ConvergenceStateNeedGovernance, action: runtimecontract.RuntimeActionRequestGovernance},
		{name: "runtime defect fails", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment}
		}, state: runtimecontract.ConvergenceStateFailed, action: runtimecontract.RuntimeActionFail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			if tt.mutate != nil {
				tt.mutate(&facts)
			}
			got, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatalf("ReduceRuntimeState() error = %v", err)
			}
			if got.State != tt.state || got.Action != tt.action {
				t.Fatalf("decision = %s/%s, want %s/%s", got.State, got.Action, tt.state, tt.action)
			}
		})
	}
}

func TestReduceRuntimeStateAttemptOnlyIncrementsForSameFactsRevision(t *testing.T) {
	facts := validationFacts()
	first, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ReduceRuntimeState(facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same facts attempt = %d, want 1", retry.AuthoritySnapshot.Attempt)
	}
	facts.EvidenceRevision = "evidence-2"
	next, err := ReduceRuntimeState(facts, &retry.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if next.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("advanced evidence attempt = %d, want reset to 0", next.AuthoritySnapshot.Attempt)
	}
}

func TestReduceRuntimeStatePrerequisiteProgressResetsAttempt(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.StateChangeKinds = []string{"file/write"}
	facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{ID: "manifest", Kind: runtimecontract.PrerequisiteKindManifest}}
	first, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ReduceRuntimeState(facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same prerequisite attempt = %d", retry.AuthoritySnapshot.Attempt)
	}
	facts.Prerequisites.Pending = nil
	next, err := ReduceRuntimeState(facts, &retry.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if next.Action != runtimecontract.RuntimeActionProduceStateChange || next.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("progressed decision = %+v", next)
	}
}

func TestReduceRuntimeStateFiltersCapabilitiesByFocusContract(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.EvidenceRole = "supplemental_evidence"
	facts.Focus.AcceptedStepKinds = []string{"git/write"}
	facts.Focus.AcceptedOperations = []string{"commit"}
	facts.Focus.RejectedStepKinds = []string{"patch/apply"}
	facts.Capabilities.StateChangeKinds = []string{"patch/apply", "git/write", "file/edit"}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != "git/write" ||
		len(decision.RequiredOutput.AcceptedOperations) != 1 || decision.RequiredOutput.AcceptedOperations[0] != "commit" ||
		decision.Focus.EvidenceRole != "supplemental_evidence" {
		t.Fatalf("focus-specific required output = %+v focus=%+v", decision.RequiredOutput, decision.Focus)
	}
}

func validationFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID:                    "cwi-1",
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{"patch/apply"},
			ValidationKinds:  []string{"validation/managed_session"},
		},
	}
}
