package runtimev2

import (
	"fmt"
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
				Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
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
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-036", State: runtimecontract.RuntimeCoverageItemStateOpen}},
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
				Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
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
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-085", State: runtimecontract.RuntimeCoverageItemStateOpen}},
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

func TestReduceRuntimeStateUsesSealedRepairBudgetForRepeatedStableFailure(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "stable-patch-failure", Message: "patch input remains invalid", TargetRefs: []string{"main.go"},
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "accepted-unrelated", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000001"}}
	for idx := 0; idx < 2; idx++ {
		facts.Evidence.Rejected = append(facts.Evidence.Rejected, runtimecontract.EvidenceFact{
			ID: common.EvidenceRecordID("rejected-" + string(rune('a'+idx))), Kind: runtimecontract.EvidenceKindStateChange,
			FailureClass: runtimecontract.FailureClassExecutionFailure, FailureSignature: "stable-patch-failure",
			Revision:            fmt.Sprintf("evidence-v2-%020d", idx+2),
			CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		})
	}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("two repeated failures action = %s, want bounded retry", decision.Action)
	}
	facts.Evidence.Rejected = append(facts.Evidence.Rejected, runtimecontract.EvidenceFact{
		ID: "rejected-c", Kind: runtimecontract.EvidenceKindStateChange,
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureSignature: "stable-patch-failure",
		Revision:            "evidence-v2-00000000000000000004",
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	})
	// Rejected evidence count is diagnostic only; the sealed repair ledger is
	// the single budget authority.
	decision, err = ReduceRuntimeState(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("rejected count bypassed repair ledger: %+v", decision)
	}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 3}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 3}
	decision, err = ReduceRuntimeState(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionFail || decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("sealed repair budget decision = %+v", decision)
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

func TestReduceRuntimeStateMissingConcreteCapabilityUsesFormalDecision(t *testing.T) {
	tests := []struct {
		name       string
		governance bool
		action     runtimecontract.RuntimeAction
		code       string
	}{
		{name: "bounded terminal without governance", action: runtimecontract.RuntimeActionFail, code: "capability_unavailable"},
		{name: "governance when capability exists", governance: true, action: runtimecontract.RuntimeActionRequestGovernance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			facts.Capabilities.StateChangeKinds = nil
			if tt.governance {
				facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
			}
			decision, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatalf("missing capability escaped reducer as error: %v", err)
			}
			if decision.Action != tt.action {
				t.Fatalf("decision = %+v, want action %s", decision, tt.action)
			}
			if tt.code != "" && (decision.Terminal == nil || decision.Terminal.Code != tt.code) {
				t.Fatalf("terminal = %+v, want code %s", decision.Terminal, tt.code)
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
	facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}
	facts.Focus.RejectedStepKinds = []string{"patch/apply"}
	facts.Capabilities.StateChangeKinds = []string{"patch/apply", "git/write", "file/edit"}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != "git/write" ||
		len(decision.RequiredOutput.AcceptedOperationVariants) != 1 || decision.RequiredOutput.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: "git/write", VariantID: "commit"}) ||
		decision.Focus.EvidenceRole != "supplemental_evidence" {
		t.Fatalf("focus-specific required output = %+v focus=%+v", decision.RequiredOutput, decision.Focus)
	}
}

func TestReduceRuntimeStateKeepsStateChangeAfterRejectedRepair(t *testing.T) {
	// Validation rejected, then state-change repair also rejected: stay on produce_state_change.
	// Re-opening produce_validation would anti-replay the same failed validation shape.
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-6",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRefs:            []string{"cmd/proxy/main.go"},
		},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{
				{
					ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
					FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "rej-patch", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000003",
					FailureClass: runtimecontract.FailureClassExecutionFailure, FailureCode: "conflict",
					FailureSignature: "sig-repair", FailureMessage: "conflict",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
			},
		},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "conflict",
			Signature: "sig-repair", Message: "conflict",
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("action = %s, want produce_state_change after rejected repair", got.Action)
	}
}

func TestReduceRuntimeStateHonorsTypedValidationFailureRoute(t *testing.T) {
	facts := validationFacts()
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		ValidationFailureImpliesStateChange: false, MaxValidationOnlyRetriesPerFocus: 2,
	}}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindFileRead}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "validation failed", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Message: "validation failed"}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("typed validation failure route decision = %+v", decision)
	}
}

func TestReduceRuntimeStateAllowsValidationAfterAcceptedRepair(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-4",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{
				{
					ID: "acc-old", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "acc-repair", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000003",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
			},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != runtimecontract.RuntimeActionProduceValidation {
		t.Fatalf("action = %s, want produce_validation after accepted repair", got.Action)
	}
}

func TestReduceRuntimeStateForcesStateChangeOnlyForUnresolvedValidationFailure(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-2",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("action = %s, want produce_state_change immediately after rejected validation", got.Action)
	}
}

func validationFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID:                    "cwi-1",
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{"patch/apply"},
			ValidationKinds:  []string{"validation/managed_session"},
		},
	}
}

func TestReduceRuntimeStateRequiresRunLevelStateChangeBeforeComplete(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = false
	facts.RemainingFocuses = nil
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{
		Required: true, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	})
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("decision = %+v, want produce_state_change for unsatisfied run ledger", decision)
	}

	// satisfy ledger then complete
	facts.StateChange = facts.StateChange.ApplyAcceptedStateChangeEvidence(runtimecontract.EvidenceFact{
		ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "evidence-v2-00000000000000000002",
	}, []common.CoverageWorkItemID{"cwi-1"})
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("decision = %+v, want complete after ledger satisfied", decision)
	}
}

func TestReduceRuntimeStateRepairableAssessmentDoesNotTerminal(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-repairable", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "implementation incomplete",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "tools not implemented",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("decision = %+v, want produce_state_change for repairable implementation gap", decision)
	}
	if decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("repairable assessment must not terminal: %+v", decision)
	}
}

func TestReduceRuntimeStateTerminalAssessmentWithoutPersistedGovernanceRejectionRepairs(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-terminal", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionTerminalRejection,
		Summary:     "irrecoverable acceptance failure",
		Gap:         &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Reason: "irrecoverable acceptance failure"},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange || decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("decision = %+v, want repair because assessment text cannot choose terminal", decision)
	}
}

func TestReduceRuntimeStateSameFocusRepairThenReassess(t *testing.T) {
	// After repairable assessment -> state-change -> validation, missing evidence is empty and
	// the prior repairable assessment has been invalidated, so reducer must request assessment again.
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Capabilities.StateChangeKinds = []string{"patch/apply"}
	facts.Capabilities.ValidationKinds = []string{"validation/managed_session"}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{
		{ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000001"},
		{ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000002"},
	}
	// No assessment present (invalidated after repair evidence) => produce_assessment.
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceAssessment {
		t.Fatalf("decision = %+v, want produce_assessment after repair evidence", decision)
	}

	// Satisfied assessment then completes.
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-1", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary:     "coverage satisfied", EvidenceRefs: []string{"val-1"},
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{Required: false})
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"val-1"}}}
	facts = facts.Normalized()
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionComplete {
		t.Fatalf("decision = %+v, want complete after satisfied assessment", decision)
	}
}

func TestInvalidateNonSatisfiedAssessmentsForFocus(t *testing.T) {
	values := []runtimecontract.RuntimeAssessmentFact{
		{ID: "assessment-gap", FocusID: "focus-1", Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "gap"},
		{ID: "assessment-ok", FocusID: "focus-1", Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "ok"},
		{ID: "assessment-other", FocusID: "focus-2", Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "other"},
	}
	got := invalidateNonSatisfiedAssessmentsForFocus(values, "focus-1")
	if len(got) != 2 {
		t.Fatalf("got = %+v, want satisfied focus-1 and focus-2 retained", got)
	}
	if got[0].Disposition != runtimecontract.AssessmentDispositionSatisfied || got[1].FocusID != "focus-2" {
		t.Fatalf("got = %+v", got)
	}
}

func TestReduceRuntimeStateRepairBudgetExhausted(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 2, MaxValidationOnlyRetriesPerFocus: 1}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 2, Exhausted: false}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-exhausted", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "still broken",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "still broken",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionFail || decision.State != runtimecontract.ConvergenceStateFailed {
		t.Fatalf("decision = %+v, want budget exhausted terminal", decision)
	}
	if decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("terminal = %+v, want no_progress_exhausted", decision.Terminal)
	}
}

func TestReduceRuntimeStateValidationOnlyCapEscalatesToStateChange(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Capabilities.StateChangeKinds = []string{"patch/apply"}
	facts.Capabilities.ValidationKinds = []string{"validation/managed_session"}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 10, MaxValidationOnlyRetriesPerFocus: 1}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1, ValidationOnlyRetries: 1}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-validation-cap", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "validation still failing",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			Reason:               "validation still failing",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("decision = %+v, want escalate to state-change after validation-only cap", decision)
	}
}

func TestReduceRuntimeStateGovernanceRequiredOpensGovernance(t *testing.T) {
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceRequired, Code: "approval_required",
		Signature: "governance-approval-web", Message: "web operation requires approval before side effects",
		TargetRefs: []string{"https://example.com"},
	}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionRequestGovernance {
		t.Fatalf("action = %s, want request_governance (not execution repair)", decision.Action)
	}
}

func TestReduceRuntimeStateProviderAuthenticationOpensConfigurationGovernance(t *testing.T) {
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassProviderAuthentication, Code: "provider_authentication",
		Message: "provider rejected the configured credential",
	}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != runtimecontract.RuntimeActionRequestGovernance || decision.RequiredOutput.Kind != runtimecontract.RequiredOutputKindGovernance {
		t.Fatalf("provider authentication decision = %+v", decision)
	}
}
