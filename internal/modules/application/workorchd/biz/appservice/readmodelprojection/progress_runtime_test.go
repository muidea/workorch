package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestApplyRuntimeProgressAlignsCountsAndMilestones(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		State:       string(common.RunStateFailed),
		Milestones: []projectioncontract.ProgressMilestone{
			{ID: "run_created", Title: "Run created", State: "completed"},
			{ID: "execution_steps", Title: "Execution steps", State: "pending"},
			{ID: "validation", Title: "Validation", State: "pending"},
			{ID: "formal_result", Title: "Result", State: "failed"},
		},
		RepairLoop: readmodelcore.RepairLoop{State: "inactive", AttemptCount: 0},
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateFailed,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 3, EvidenceSequence: 2,
			Facts: runtimecontract.RuntimeFacts{
				WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-2",
				Focus: runtimecontract.CoverageFocus{
					ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
				},
				Evidence: runtimecontract.EvidenceLedger{
					Accepted: []runtimecontract.EvidenceFact{{
						ID: "acc-1", Kind: runtimecontract.EvidenceKindStateChange,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
					}},
					Rejected: []runtimecontract.EvidenceFact{{
						ID: "rej-1", Kind: runtimecontract.EvidenceKindValidation,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
						FailureSignature:    "sig-1", FailureMessage: "validation failed",
					}},
				},
				Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Signature: "sig-1", Message: "validation failed"},
			},
			Decision: runtimecontract.RuntimeDecision{
				State:             runtimecontract.ConvergenceStateFailed,
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-1"},
			},
			CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "dispatch-1"}, {ID: "dispatch-2"}},
			CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{
				{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted},
				{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed},
				{Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStateFailed},
			},
		},
	}
	applyRuntimeProgress(&progress, run)
	if progress.Runtime == nil {
		t.Fatal("expected runtime summary")
	}
	if progress.Runtime.CompletedDispatchCount != 2 {
		t.Fatalf("completed dispatch count = %d", progress.Runtime.CompletedDispatchCount)
	}
	// Candidate contract failures are SM-3 retry bookkeeping, not SM-2 focus repair.
	if progress.RepairLoop.AttemptCount != 0 {
		t.Fatalf("repair attempt count = %d, want 0 from the focus repair ledger", progress.RepairLoop.AttemptCount)
	}
	if progress.RepairLoop.State != "terminated" {
		t.Fatalf("repair state = %s", progress.RepairLoop.State)
	}
	byID := map[string]string{}
	for _, item := range progress.Milestones {
		byID[item.ID] = item.State
	}
	// Terminal failed run must not leave execution_steps as running.
	if byID["execution_steps"] != "failed" {
		t.Fatalf("execution milestone = %s, want failed for terminal run", byID["execution_steps"])
	}
	if byID["validation"] != "failed" {
		t.Fatalf("validation milestone = %s", byID["validation"])
	}
	// Current step should use dispatch identity, not authority snapshot.
	if progress.CurrentStepID != "dispatch-2" {
		t.Fatalf("current step = %s, want last completed dispatch", progress.CurrentStepID)
	}
}

func TestApplyRuntimeProgressHealthyRunDoesNotMarkRepairActive(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1", State: string(common.RunStateActive),
		RepairLoop: readmodelcore.RepairLoop{State: "inactive"},
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{
				{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted},
			},
			CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "dispatch-1"}},
			Facts: runtimecontract.RuntimeFacts{
				Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
				WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def", EvidenceRevision: "ev",
				Focus: runtimecontract.CoverageFocus{ID: "focus-1", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
			},
			Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation},
		},
	}
	applyRuntimeProgress(&progress, run)
	if progress.RepairLoop.AttemptCount != 0 {
		t.Fatalf("healthy run repair attempts = %d, want 0", progress.RepairLoop.AttemptCount)
	}
	if progress.RepairLoop.State != "inactive" {
		t.Fatalf("healthy run repair state = %s, want inactive", progress.RepairLoop.State)
	}
}

func TestApplyRuntimeProgressDoesNotTreatStateChangeEvidenceAsValidation(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1", State: string(common.RunStateFailed),
		Milestones: []projectioncontract.ProgressMilestone{{ID: "validation", State: "pending"}},
	}
	run := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateFailed, RuntimeState: &runtimecontract.RuntimeStateRecord{
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def", EvidenceRevision: "ev",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
			Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{ID: "state-change", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}},
		},
	}}
	applyRuntimeProgress(&progress, run)
	if progress.Runtime == nil || progress.Runtime.AcceptedEvidenceCount != 1 || progress.Runtime.AcceptedValidationEvidenceCount != 0 {
		t.Fatalf("evidence-kind projection = %+v", progress.Runtime)
	}
	if got := progress.Milestones[0]; got.State != "failed" || len(got.EvidenceRefs) != 0 {
		t.Fatalf("state-change evidence fabricated validation completion: %+v", got)
	}
}

func TestApplyRuntimeProgressCompletesValidationOnlyWithValidationEvidence(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1", State: string(common.RunStateActive),
		Milestones: []projectioncontract.ProgressMilestone{{ID: "validation", State: "pending"}},
	}
	run := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive, RuntimeState: &runtimecontract.RuntimeStateRecord{
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def", EvidenceRevision: "ev",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
			Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{ID: "validation", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}},
		},
	}}
	applyRuntimeProgress(&progress, run)
	if progress.Runtime == nil || progress.Runtime.AcceptedValidationEvidenceCount != 1 {
		t.Fatalf("validation evidence count = %+v", progress.Runtime)
	}
	if got := progress.Milestones[0]; got.State != "completed" || len(got.EvidenceRefs) != 1 || got.EvidenceRefs[0] != "runtime/validation-evidence" {
		t.Fatalf("validation evidence did not complete validation milestone: %+v", got)
	}
}

func TestMapRuntimeProgressIgnoresUnboundEvidenceBeforeLastFocus(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus:            runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		RemainingFocuses: []runtimecontract.CoverageFocus{{ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}}},
		Evidence:         runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{ID: "unbound", Kind: runtimecontract.EvidenceKindValidation}}},
	}.Normalized()}
	summary := mapRuntimeProgress(state)
	if summary.AcceptedEvidenceCount != 0 {
		t.Fatalf("accepted evidence count = %d, want 0 before the last focus", summary.AcceptedEvidenceCount)
	}
	if len(summary.MissingEvidenceKinds) != 1 || summary.MissingEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("missing evidence = %#v, want validation", summary.MissingEvidenceKinds)
	}
}

func TestMapRuntimeProgressProjectsPrerequisiteRefreshWithoutCandidateRetry(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
		Prerequisites: runtimecontract.PrerequisiteState{Pending: []runtimecontract.RuntimePrerequisite{{
			ID: "operation_admissibility:file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			OwnerModuleID: runtimecontract.RuntimeModuleFile,
		}}},
		RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
		Recovery:       runtimecontract.RuntimeRecoveryLedger{PrerequisiteRefreshes: 1},
	}.Normalized()
	change := runtimecontract.RuntimeResourceChange{
		Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "main.go", BeforeRevision: "main-v1", AfterRevision: "main-v2",
	}
	summary := mapRuntimeProgress(runtimecontract.RuntimeStateRecord{
		Facts: facts,
		PrerequisiteInvalidations: []runtimecontract.RuntimePrerequisiteInvalidationRecord{{
			Signature: "resource-change-1", PrerequisiteIDs: []string{"operation_admissibility:file"}, ObservationIDs: []string{"observation-1"},
			DispatchID: "dispatch-1", OperationID: "write-1", ReceiptHash: "receipt-1", ResourceChanges: []runtimecontract.RuntimeResourceChange{change},
		}},
	})
	if summary.PrerequisiteRefreshState != "prerequisite_refreshing" || summary.PrerequisiteRefreshUsed != 1 ||
		summary.PrerequisiteRefreshLimit != runtimecontract.DefaultMaxPrerequisiteRefreshes {
		t.Fatalf("refresh status = %+v", summary)
	}
	if summary.CandidateAttemptCount != 0 {
		t.Fatalf("freshness projection was represented as candidate retry: %+v", summary)
	}
	if len(summary.PrerequisiteInvalidations) != 1 {
		t.Fatalf("invalidation projection = %#v", summary.PrerequisiteInvalidations)
	}
	invalidation := summary.PrerequisiteInvalidations[0]
	if invalidation.Source != "owner_receipt_resource_change" || invalidation.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		invalidation.ResourceDomain != string(runtimecontract.RuntimeResourceWorkspaceTarget) || invalidation.SubjectRef != "main.go" ||
		len(invalidation.ResourceChanges) != 1 || invalidation.ResourceChanges[0].AfterRevision != "main-v2" {
		t.Fatalf("resource-change source projection = %#v", invalidation)
	}
}

func TestMapRuntimeProgressProjectsPrerequisiteRefreshExhausted(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"}, RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
		Recovery: runtimecontract.RuntimeRecoveryLedger{PrerequisiteRefreshes: runtimecontract.DefaultMaxPrerequisiteRefreshes},
		Failure:  &runtimecontract.FailureFact{Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted", Signature: "refresh-1", Message: "refresh exhausted"},
	}.Normalized()
	summary := mapRuntimeProgress(runtimecontract.RuntimeStateRecord{Facts: facts})
	if summary.PrerequisiteRefreshState != "prerequisite_refresh_exhausted" ||
		summary.PrerequisiteRefreshUsed != runtimecontract.DefaultMaxPrerequisiteRefreshes {
		t.Fatalf("exhausted refresh status = %+v", summary)
	}
}

func TestRuntimeAwareStepCountUsesDispatchUnits(t *testing.T) {
	snapshot := readmodelcore.RunSnapshot{
		TaskProgress: readmodelcore.TaskProgress{
			Runtime: &projectioncontract.RuntimeProgress{
				CompletedDispatchCount: 3,
				ActiveDispatchID:       "active-1",
				AcceptedEvidenceCount:  10,
				RejectedEvidenceCount:  2,
			},
		},
	}
	progress := MapRuntimeProgress(snapshot)
	if progress.StepTotal != 4 {
		t.Fatalf("step total = %d, want 4 dispatch units", progress.StepTotal)
	}
	if progress.StepCompleted+progress.StepFailed != 3 {
		t.Fatalf("completed+failed = %d, want 3 completed dispatches", progress.StepCompleted+progress.StepFailed)
	}
	if progress.StepCompleted > progress.StepTotal {
		t.Fatalf("stepCompleted %d > stepTotal %d", progress.StepCompleted, progress.StepTotal)
	}
}

func TestStepProgressStateTerminalFailedWins(t *testing.T) {
	snapshot := readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{State: string(common.RunStateFailed)},
		TaskProgress: readmodelcore.TaskProgress{
			Runtime: &projectioncontract.RuntimeProgress{
				CompletedDispatchCount: 2,
				AcceptedEvidenceCount:  1,
				RejectedEvidenceCount:  1,
			},
		},
	}
	if got := StepProgressState(snapshot); got != "failed" {
		t.Fatalf("StepProgressState = %s, want failed", got)
	}
}

func TestApplyRuntimeProgressCompletedRunRepairResolved(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1", State: string(common.RunStateCompleted),
		RepairLoop: readmodelcore.RepairLoop{State: "active", AttemptCount: 1},
	}
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateCompleted,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{
				{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed},
				{Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStateAccepted},
			},
			CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "dispatch-1"}},
			Facts: runtimecontract.RuntimeFacts{
				Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
				WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def", EvidenceRevision: "ev",
				Focus:  runtimecontract.CoverageFocus{ID: "focus-1"},
				Repair: runtimecontract.RuntimeRepairLedger{FocusID: "focus-1", RepairAttempts: 1},
			},
			Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateComplete},
		},
	}
	applyRuntimeProgress(&progress, run)
	if progress.RepairLoop.State != "resolved" {
		t.Fatalf("completed run repair state = %s, want resolved", progress.RepairLoop.State)
	}
}

func TestApplyRuntimeProgressClearsHistoricalCandidateFailureAfterFocusAdvance(t *testing.T) {
	progress := readmodelcore.TaskProgress{
		WorkspaceID: "ws-1", RunID: "run-1", State: string(common.RunStateActive),
		RepairLoop: readmodelcore.RepairLoop{State: "active", AttemptCount: 2, RepeatedFailureReason: "old candidate failure", Escalated: true},
	}
	run := &runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive, RuntimeState: &runtimecontract.RuntimeStateRecord{
		CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{
			{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed},
			{Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStateAccepted},
		},
		Facts: runtimecontract.RuntimeFacts{
			Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def", EvidenceRevision: "ev-2",
			Focus:    runtimecontract.CoverageFocus{ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, RequireAssessment: true},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		},
		Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedAssessment},
	}}
	applyRuntimeProgress(&progress, run)
	if progress.RepairLoop.State != "inactive" || progress.RepairLoop.AttemptCount != 0 || progress.RepairLoop.RepeatedFailureReason != "" || progress.RepairLoop.Escalated {
		t.Fatalf("repair loop retained historical candidate failure: %+v", progress.RepairLoop)
	}
}
