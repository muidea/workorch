package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSyncRepairDirectiveMaterializesStateChangeFromDecision(t *testing.T) {
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1",
		DefinitionRevision: "def-1", EvidenceRevision: InitialRuntimeEvidenceRevision,
		Focus:        CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}},
		Capabilities: RuntimeCapabilities{StateChangeKinds: []string{StepKindFileWrite}, ValidationKinds: []string{StepKindFileRead}},
	}
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange, Action: RuntimeActionProduceStateChange, Focus: facts.Focus,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		},
	}
	facts = SyncRepairDirective(facts, decision)
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != RuntimeActionProduceStateChange {
		t.Fatalf("directive = %+v", facts.RepairDirective)
	}
	if !RuntimeEffectIsDurable(facts.RepairDirective.RequiredEffectScopes[0]) {
		t.Fatalf("directive effect scopes = %v", facts.RepairDirective.RequiredEffectScopes)
	}
}

func TestSyncRepairDirectiveFlipsToValidationAfterAcceptedStateChange(t *testing.T) {
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-v2-00000000000000000002",
		Focus: CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}},
		RepairDirective: &RuntimeRepairDirective{
			ID: "repair-focus-1-produce_state_change-0", FocusID: "focus-1",
			Trigger: RuntimeRepairTriggerRejectedValidation, RequiredAction: RuntimeActionProduceStateChange,
			RequiredEffectScopes:       []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds:      []string{EvidenceKindStateChange},
			SupersedesEvidenceRevision: InitialRuntimeEvidenceRevision,
		},
		Evidence: EvidenceLedger{Accepted: []EvidenceFact{{
			ID: "acc-1", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Revision: "evidence-v2-00000000000000000002",
		}}},
	}
	// Decision may still be state-change until re-reduce; flip is based on accepted evidence.
	decision := RuntimeDecision{Action: RuntimeActionProduceStateChange, Focus: facts.Focus}
	facts = SyncRepairDirective(facts, decision)
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredAction != RuntimeActionProduceValidation {
		t.Fatalf("after accepted state-change directive = %+v", facts.RepairDirective)
	}
}

func TestApplyRepairDirectiveActionOverridesMissingEvidence(t *testing.T) {
	// Run-022 lineage: missing only validation, but directive requires state-change repair.
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1",
		DefinitionRevision: "def-1", EvidenceRevision: InitialRuntimeEvidenceRevision,
		Focus:        CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}},
		Capabilities: RuntimeCapabilities{StateChangeKinds: []string{StepKindFileWrite}, ValidationKinds: []string{StepKindFileRead}},
		// No accepted state-change: missing validation alone would normally prefer
		// produce_validation, but the repair directive forces state-change.
		Evidence: EvidenceLedger{Rejected: []EvidenceFact{{
			ID: "rej-val", Kind: EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			Revision: "evidence-v2-00000000000000000001", FailureClass: FailureClassExecutionRejected, FailureMessage: "test failed",
		}}},
		RepairDirective: &RuntimeRepairDirective{
			ID: "repair-1", FocusID: "focus-1", Trigger: RuntimeRepairTriggerRejectedValidation,
			RequiredAction:        RuntimeActionProduceStateChange,
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{StepKindFileWrite},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		},
	}
	base := RuntimeDecision{Focus: facts.Focus}
	decision, ok, err := ApplyRepairDirectiveAction(facts, base)
	if err != nil || !ok {
		t.Fatalf("ApplyRepairDirectiveAction err=%v ok=%v", err, ok)
	}
	if decision.Action != RuntimeActionProduceStateChange {
		t.Fatalf("action = %s, want produce_state_change", decision.Action)
	}
}

func TestDeriveRuntimeRepairHintsObeysDirective(t *testing.T) {
	facts := RuntimeFacts{
		Focus: CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindValidation}},
		RepairDirective: &RuntimeRepairDirective{
			ID: "repair-1", FocusID: "focus-1", Trigger: RuntimeRepairTriggerRejectedValidation,
			RequiredAction:        RuntimeActionProduceStateChange,
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		},
	}
	hints := DeriveRuntimeRepairHints(facts)
	if len(hints) == 0 || hints[0].Code != "obey_repair_directive" {
		t.Fatalf("hints = %+v", hints)
	}
	for _, hint := range hints {
		if hint.Code == "produce_missing_evidence" || strings.Contains(hint.Message, "produce only the missing kinds") {
			t.Fatalf("must not emit missing-kinds override when directive active: %+v", hint)
		}
	}
}

func TestRuntimeCandidateContextFromFactsProjectsDirective(t *testing.T) {
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1",
		DefinitionRevision: "def-1", EvidenceRevision: InitialRuntimeEvidenceRevision,
		Focus: CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
		RepairDirective: &RuntimeRepairDirective{
			ID: "repair-1", FocusID: "focus-1", Trigger: RuntimeRepairTriggerRejectedValidation,
			RequiredAction:        RuntimeActionProduceStateChange,
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			AllowedOperationKinds: []string{StepKindFileWrite},
		},
		Activation: RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "p", ModelID: "m"}},
	}
	// Activation may need valid provider - skip full Validate; just project context.
	ctx := RuntimeCandidateContextFromFacts(facts)
	if ctx.RequiredAction != RuntimeActionProduceStateChange {
		t.Fatalf("RequiredAction = %q", ctx.RequiredAction)
	}
	if ctx.RepairDirective == nil || len(ctx.RequiredEffectScopes) == 0 {
		t.Fatalf("context = %+v", ctx)
	}
	if len(ctx.AdmissibleOperationKinds) != 1 || ctx.AdmissibleOperationKinds[0] != StepKindFileWrite {
		t.Fatalf("admissible = %v", ctx.AdmissibleOperationKinds)
	}
}

func TestClearRepairDirectiveOnFocusAdvance(t *testing.T) {
	facts := RuntimeFacts{
		Focus:           CoverageFocus{ID: "focus-2"},
		RepairDirective: &RuntimeRepairDirective{ID: "r", FocusID: "focus-1", RequiredAction: RuntimeActionProduceStateChange, Trigger: RuntimeRepairTriggerMissingEvidence},
	}
	facts = ClearRepairDirectiveForFocusAdvance(facts)
	if facts.RepairDirective != nil {
		t.Fatalf("directive should clear on focus advance, got %+v", facts.RepairDirective)
	}
}
