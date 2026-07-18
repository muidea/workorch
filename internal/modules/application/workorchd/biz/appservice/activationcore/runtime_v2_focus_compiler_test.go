package activationcore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompileRuntimeV2CoverageFocusesAppliesDodItemWildcard(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem",
					RequireDirectEvidence: true, RequireClaimEvidence: true, RequireExecutableBehaviorEvidence: true,
					AcceptedStepKinds: []string{"shell/command", "patch/apply", "mcp/tool_call"},
					RejectedStepKinds: []string{"file/read", "file/list", "mcp/resource_read", "mcp/prompt_get"},
				}},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	for _, focus := range focuses {
		if focus.AssessmentScope != "dodItem" || focus.SliceID == "" || !focus.RequireDirectEvidence || !focus.RequireClaimEvidence || !focus.RequireExecutableBehaviorEvidence {
			t.Fatalf("dod focus policy not applied: %+v", focus)
		}
		if len(focus.AcceptedStepKinds) != 3 || focus.AcceptedStepKinds[0] != "mcp/tool_call" || focus.AcceptedStepKinds[1] != "patch/apply" || focus.AcceptedStepKinds[2] != "shell/command" {
			t.Fatalf("accepted kinds = %v", focus.AcceptedStepKinds)
		}
		if len(focus.RejectedStepKinds) != 4 || focus.RejectedStepKinds[0] != "file/list" || focus.RejectedStepKinds[1] != "file/read" {
			t.Fatalf("rejected kinds = %v", focus.RejectedStepKinds)
		}
		// Run-level must_produce_state_change no longer forces every DoD focus to require state_change.
		if len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
			t.Fatalf("required evidence kinds = %v, want validation only", focus.RequiredEvidenceKinds)
		}
	}
}

func TestCompileRuntimeV2CoverageFocusesExactBeatsWildcard(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				AcceptanceDimensions: []runtimecontract.RuntimeAcceptanceDimension{{
					ID: "runtime_entrypoint", Required: true, Description: "entrypoint works",
				}},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{
						ID: "*", Scope: "acceptanceDimension",
						AcceptedStepKinds: []string{"shell/command"},
						RejectedStepKinds: []string{"file/read"},
					},
					{
						ID: "runtime_entrypoint", Scope: "acceptanceDimension",
						RequireExecutableBehaviorEvidence: true,
						AcceptedStepKinds:                 []string{"validation/managed_session", "shell/command"},
						RejectedStepKinds:                 []string{"file/read", "file/list"},
						Description:                       "exact entrypoint policy",
					},
				},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	entrypoint := focuses[1]
	if entrypoint.AssessmentID != "runtime_entrypoint" {
		t.Fatalf("entrypoint focus = %+v", entrypoint)
	}
	// exact accepted kinds win (first non-empty highest priority), rejected kinds are union
	// Normalized() stably sorts accepted kinds; exact source still excludes wildcard-only values.
	if len(entrypoint.AcceptedStepKinds) != 2 || entrypoint.AcceptedStepKinds[0] != "shell/command" || entrypoint.AcceptedStepKinds[1] != "validation/managed_session" {
		t.Fatalf("exact accepted kinds = %v", entrypoint.AcceptedStepKinds)
	}
	if !entrypoint.RequireExecutableBehaviorEvidence {
		t.Fatalf("exact executable constraint missing: %+v", entrypoint)
	}
	if len(entrypoint.RejectedStepKinds) != 2 {
		t.Fatalf("rejected union = %v", entrypoint.RejectedStepKinds)
	}
}

func TestCompileRuntimeV2CoverageFocusesRejectsUnknownAcceptedKind(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "broken",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", AcceptedStepKinds: []string{"totally/unknown"},
				}},
			}},
		},
	}
	_, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "no runtime operation descriptor") {
		t.Fatalf("expected unknown kind failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageFocusesUsesEvidencePolicyCoverageRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "policy-1", AppliesTo: "shell/command",
			CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
				ID: "*", Scope: "dod_item",
				RequiredEvidenceKinds:             []string{"validation"},
				RequireDirectEvidence:             true,
				RequireExecutableBehaviorEvidence: true,
				AcceptedStepKinds:                 []string{"shell/command", "mcp/tool_call"},
				RejectedStepKinds:                 []string{"file/read"},
			}},
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	focus := focuses[0]
	if !focus.RequireDirectEvidence || !focus.RequireExecutableBehaviorEvidence {
		t.Fatalf("evidence policy not applied: %+v", focus)
	}
	if len(focus.AcceptedStepKinds) != 2 || focus.AcceptedStepKinds[0] != "mcp/tool_call" || focus.AcceptedStepKinds[1] != "shell/command" {
		t.Fatalf("accepted = %v", focus.AcceptedStepKinds)
	}
	if len(focus.RejectedStepKinds) != 1 || focus.RejectedStepKinds[0] != "file/read" {
		t.Fatalf("rejected = %v", focus.RejectedStepKinds)
	}
}

func TestRuntimeV2BootstrapCompilesDodWildcardIntoFocuses(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
			StateChangePolicy:     runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem",
					RequireDirectEvidence: true, RequireExecutableBehaviorEvidence: true,
					AcceptedStepKinds: []string{"shell/command", "mcp/tool_call"},
					RejectedStepKinds: []string{"file/read"},
				}},
			}},
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"实现行为一"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 1 {
		t.Fatalf("focuses = %+v", bootstrap.Focuses)
	}
	focus := bootstrap.Focuses[0]
	if !focus.RequireDirectEvidence || !focus.RequireExecutableBehaviorEvidence || len(focus.RejectedStepKinds) == 0 {
		t.Fatalf("bootstrap did not compile dod wildcard policy: %+v", focus)
	}
}

func TestRuntimeV2BootstrapSealsRunLevelStateChangeRequirement(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
			StateChangePolicy:     runtimecontract.RuntimeStateChangeMustProduce,
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrap.StateChangeRequirement.Required {
		t.Fatalf("state-change requirement = %+v", bootstrap.StateChangeRequirement)
	}
	if len(bootstrap.StateChangeRequirement.CoverageWorkItemIDs) != 2 {
		t.Fatalf("coverage ids = %v", bootstrap.StateChangeRequirement.CoverageWorkItemIDs)
	}
	// DoD focuses themselves must not be mechanically forced to state_change.
	for _, focus := range bootstrap.Focuses {
		for _, kind := range focus.RequiredEvidenceKinds {
			if kind == runtimecontract.EvidenceKindStateChange {
				t.Fatalf("DoD focus unexpectedly requires state_change: %+v", focus)
			}
		}
	}
	facts, err := bootstrap.InitialFacts("ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !facts.StateChange.Required || facts.StateChange.Satisfied {
		t.Fatalf("initial ledger = %+v", facts.StateChange)
	}
}

func TestCompileRuntimeV2CoverageFocusesKeepsDeliverableStateChange(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
					{ID: "source_change", Required: true, EvidenceRole: "primary_state_change", Description: "workspace changed"},
				},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change",
					AcceptedStepKinds: []string{"patch/apply"},
				}},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	dod, deliverable := focuses[0], focuses[1]
	for _, kind := range dod.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindStateChange {
			t.Fatalf("DoD focus must not auto-require state_change: %+v", dod)
		}
	}
	found := false
	for _, kind := range deliverable.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindStateChange {
			found = true
		}
	}
	if !found || deliverable.EvidenceRole != "primary_state_change" {
		t.Fatalf("deliverable focus must keep explicit state_change: %+v", deliverable)
	}
}

func TestCompileRuntimeV2CoverageSlicesBoundLargeDoD(t *testing.T) {
	items := make([]string, 0, 10)
	for i := 1; i <= 10; i++ {
		items = append(items, "行为"+string(rune('A'+i-1)))
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
				MaxWorkItems:  4, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
					ID: "slice-remaining-acceptance", Title: "Remaining", Scope: "remaining_acceptance", Priority: 90,
					MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
				}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	// 10 items / max 4 => 3 focuses/slices, not 10 assessment rounds.
	if len(compiled.Focuses) != 3 || len(compiled.Slices) != 3 {
		t.Fatalf("focuses=%d slices=%d, want 3/3", len(compiled.Focuses), len(compiled.Slices))
	}
	if len(compiled.CoverageItems) != 10 {
		t.Fatalf("coverage items = %d, want 10 stable identities", len(compiled.CoverageItems))
	}
	if compiled.Focuses[0].SliceID != "slice-remaining-acceptance-001" || len(compiled.Focuses[0].CoverageWorkItemIDs) != 4 {
		t.Fatalf("first slice focus = %+v", compiled.Focuses[0])
	}
	if compiled.Focuses[2].SliceID != "slice-remaining-acceptance-003" || len(compiled.Focuses[2].CoverageWorkItemIDs) != 2 {
		t.Fatalf("last slice focus = %+v", compiled.Focuses[2])
	}
	// Item IDs are independent of slice packing.
	if compiled.CoverageItems[0].ID != "cwi-001" || compiled.CoverageItems[9].ID != "cwi-010" {
		t.Fatalf("coverage item ids = %+v", compiled.CoverageItems)
	}
}

func TestCompileRuntimeV2CoverageRejectsInvalidSliceMax(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
				MaxWorkItems:  100, MaxFocuses: 4, MaxCandidateContextItems: 4,
			},
		},
	}
	_, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "maxWorkItems") {
		t.Fatalf("expected maxWorkItems validation failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageRejectsUnboundedFocusCount(t *testing.T) {
	items := make([]string, runtimecontract.RuntimeAcceptanceFocusMaxCount+1)
	for idx := range items {
		items[idx] = fmt.Sprintf("requirement-%03d", idx+1)
	}
	_, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeding sealed policy limit") {
		t.Fatalf("expected focus-count guard failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageRehydratesUnresolvedContinuation(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		},
		Continuation: runtimecontract.RuntimeContinuationContext{
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				DefinitionRevision: "definition-1",
				Disposition:        runtimecontract.AssessmentDispositionRepairableGap,
				SatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{
					CoverageWorkItemID: "cwi-001", State: "satisfied",
				}},
				UnsatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{
					CoverageWorkItemID: "cwi-002", State: "unsatisfied",
				}, {
					CoverageWorkItemID: "cwi-003", State: "unsatisfied",
				}},
				CoverageGaps: []runtimecontract.RuntimeContinuationCoverageGap{{
					Scope: "coverage", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002", "cwi-003"},
					MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二", "行为三"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.CoverageItems) != 2 {
		t.Fatalf("coverage items = %+v, want only unresolved cwi-002/cwi-003", compiled.CoverageItems)
	}
	// IDs must remain original definition IDs, not renumbered.
	ids := []string{string(compiled.CoverageItems[0].ID), string(compiled.CoverageItems[1].ID)}
	if ids[0] != "cwi-002" || ids[1] != "cwi-003" {
		t.Fatalf("coverage ids = %v, want [cwi-002 cwi-003]", ids)
	}
	if len(compiled.Focuses) == 0 {
		t.Fatal("expected focuses for unresolved items")
	}
}

func TestCompileRuntimeV2CoverageGapAnalysisWhenContinuationLacksStructuredGaps(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		Continuation: runtimecontract.RuntimeContinuationContext{
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				Summary: "legacy summary only",
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Focuses) != 1 || compiled.Focuses[0].AssessmentScope != "continuation_gap_analysis" {
		t.Fatalf("focuses = %+v, want single continuation_gap_analysis focus", compiled.Focuses)
	}
	if !compiled.Focuses[0].RequireAssessment {
		t.Fatalf("gap-analysis focus must require assessment: %+v", compiled.Focuses[0])
	}
}

func TestCompileRuntimeV2CoverageGapAnalysisOnDefinitionChangeSupersede(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		Continuation: runtimecontract.RuntimeContinuationContext{
			DefinitionChangeSupersedesPrior: true,
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				UnsatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{CoverageWorkItemID: "cwi-002", State: "unsatisfied"}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Focuses) != 1 || compiled.Focuses[0].AssessmentScope != "continuation_gap_analysis" {
		t.Fatalf("focuses = %+v, want gap-analysis on definition change", compiled.Focuses)
	}
}
