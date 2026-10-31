package workspace

import (
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
)

func TestPreparationProviderBudgetBindsOriginalSelectionWithoutReadiness(t *testing.T) {
	for _, scenario := range []string{"exact", "foreign", "unavailable", "malformed", "no_preview"} {
		t.Run(scenario, func(t *testing.T) {
			d, e := preparationProviderSelectionFixture()
			target := e.Facts.Observation.Configuration.DefaultChat
			e.Facts.Observation.Budget = &llmevents.PreparationProviderBudget{Version: llmevents.PreparationProviderBudgetVersion, Target: target, ContextWindowTokens: 32000, ProviderMaxOutputTokens: 4000, EffectiveMaxOutputTokens: 4000, MaxInputTokens: 20000, MaxRequestBytes: 80000, EstimatedTokensPerByte: 0.25, CapabilityRevision: "capability-exact"}
			reason := PreparationProviderConfigurationReason
			switch scenario {
			case "foreign":
				e.Facts.Observation.Budget.Target.ModelID = "foreign"
				reason = "provider_selection_budget_source_conflict"
			case "unavailable":
				e.Facts.Observation.Budget = &llmevents.PreparationProviderBudget{Version: llmevents.PreparationProviderBudgetVersion, Target: target, FailureCode: "capacity_unavailable"}
				reason = PreparationProviderConfigurationReason
			case "malformed":
				e.Facts.Observation.Budget.MaxInputTokens = 0
				reason = "provider_selection_source_binding_conflict"
			case "no_preview":
				e.Facts.Observation.Budget = nil
				reason = PreparationProviderConfigurationReason
			}
			c := CompilePreparationProviderSelection(d, e)
			if c.BlockedReason() != reason || scenario == "exact" && c.Budget == nil || scenario != "exact" && c.Budget != nil {
				t.Fatal("budget source lost or granted readiness", scenario, c.BlockedReason())
			}
			if scenario == "exact" {
				e.Selection = &c
				e.ContentHash = e.Hash()
				if e.Validate() != nil {
					t.Fatal("original capacity rejected")
				}
				copy := e.Clone()
				copy.Selection.Budget.MaxInputTokens++
				copy.ContentHash = copy.Hash()
				if copy.Validate() == nil || e.Selection.Budget.MaxInputTokens != 20000 {
					t.Fatal("rehashed compilation drift accepted or clone aliases original")
				}
			}
		})
	}
}
