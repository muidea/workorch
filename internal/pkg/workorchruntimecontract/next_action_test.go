package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNextActionNormalizedDerivesStableScopedIdentity(t *testing.T) {
	t.Parallel()

	action := NextAction{
		WorkspaceID:           "ws-next-action",
		RunID:                 "run-next-action",
		Type:                  NextActionTypeStateChange,
		SubjectType:           " coverage ",
		SubjectID:             " cwi-001 ",
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{" cwi-001 ", "cwi-001", "cwi-002"},
		RequiredEvidenceKinds: []string{" state_change ", "validation", "state_change"},
		AllowedActions:        []string{"produce_output", "produce_output", "validate"},
		ForbiddenActions:      []string{"complete", "complete"},
		Stage:                 " state_change ",
		StageMode:             " primary ",
		Reason:                " missing required evidence ",
	}.Normalized()

	if action.ID == "" || action.IdempotencyKey == "" {
		t.Fatalf("normalized action = %+v, want stable identity", action)
	}
	if !action.HasScope() {
		t.Fatalf("normalized action = %+v, want workspace/run scope", action)
	}
	if len(action.FocusCoverageItemIDs) != 2 || action.FocusCoverageItemIDs[0] != "cwi-001" || action.FocusCoverageItemIDs[1] != "cwi-002" {
		t.Fatalf("focus coverage ids = %+v, want compact ordered ids", action.FocusCoverageItemIDs)
	}
	if len(action.RequiredEvidenceKinds) != 2 || action.RequiredEvidenceKinds[0] != "state_change" || action.RequiredEvidenceKinds[1] != "validation" {
		t.Fatalf("required evidence kinds = %+v, want compact ordered kinds", action.RequiredEvidenceKinds)
	}
	if action.SubjectType != "coverage" || action.SubjectID != "cwi-001" || action.Stage != "state_change" || action.StageMode != "primary" || action.Reason != "missing required evidence" {
		t.Fatalf("normalized action = %+v, want trimmed stage and reason", action)
	}
}

func TestNextActionIdentitySeparatesWorkspaceAndRunScope(t *testing.T) {
	t.Parallel()

	base := NextAction{
		WorkspaceID:           "ws-a",
		RunID:                 "run-a",
		Type:                  NextActionTypeValidate,
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{"validation"},
	}.Normalized()
	otherWorkspace := NextAction{
		WorkspaceID:           "ws-b",
		RunID:                 "run-a",
		Type:                  NextActionTypeValidate,
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{"validation"},
	}.Normalized()
	otherRun := NextAction{
		WorkspaceID:           "ws-a",
		RunID:                 "run-b",
		Type:                  NextActionTypeValidate,
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{"validation"},
	}.Normalized()

	if base.IdempotencyKey == otherWorkspace.IdempotencyKey || base.ID == otherWorkspace.ID {
		t.Fatalf("workspace scoped actions share identity: base=%+v other=%+v", base, otherWorkspace)
	}
	if base.IdempotencyKey == otherRun.IdempotencyKey || base.ID == otherRun.ID {
		t.Fatalf("run scoped actions share identity: base=%+v other=%+v", base, otherRun)
	}
}

func TestNextActionIdentityCanBeRebuiltFromLedgerFields(t *testing.T) {
	t.Parallel()

	left := NextAction{
		WorkspaceID:           "ws-ledger",
		RunID:                 "run-ledger",
		Type:                  NextActionTypeAssessment,
		SubjectType:           "coverage",
		SubjectID:             "cwi-001",
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{"cwi-001", "cwi-002"},
		RequiredEvidenceKinds: []string{"assessment"},
		Stage:                 "assessment",
	}.Normalized()
	right := NextAction{
		WorkspaceID:           "ws-ledger",
		RunID:                 "run-ledger",
		Type:                  NextActionTypeAssessment,
		SubjectType:           "coverage",
		SubjectID:             "cwi-001",
		FocusCoverageItemIDs:  []common.CoverageWorkItemID{"cwi-001", "cwi-002"},
		RequiredEvidenceKinds: []string{"assessment"},
		Stage:                 "assessment",
		Reason:                "new runtime explanation should not change idempotency",
	}.Normalized()

	if left.IdempotencyKey != right.IdempotencyKey || left.ID != right.ID {
		t.Fatalf("rebuilt actions differ: left=%+v right=%+v", left, right)
	}
}

func TestNextActionIdentityIncludesGovernanceSubject(t *testing.T) {
	t.Parallel()

	left := NextAction{
		WorkspaceID: "ws-governance",
		RunID:       "run-governance",
		Type:        NextActionTypeRequestApproval,
		SubjectType: "approval",
		SubjectID:   "approval-001",
	}.Normalized()
	right := NextAction{
		WorkspaceID: "ws-governance",
		RunID:       "run-governance",
		Type:        NextActionTypeRequestApproval,
		SubjectType: "approval",
		SubjectID:   "approval-002",
	}.Normalized()

	if left.IdempotencyKey == right.IdempotencyKey || left.ID == right.ID {
		t.Fatalf("governance actions share identity: left=%+v right=%+v", left, right)
	}
}
