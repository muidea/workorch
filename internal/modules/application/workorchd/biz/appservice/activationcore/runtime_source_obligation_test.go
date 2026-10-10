package activationcore

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func sourceObligationBootstrapFixture(t *testing.T) (*runtimecontract.RuntimeBootstrap, runtimecontract.RuntimeSourceObligationInventory) {
	t.Helper()
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: "workspace", Text: "目标"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"共同判据"}}
	inventory := (runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent,
		CandidateHash: "candidate", SpecificationHash: "specification", DefinitionRevision: "definition", StartupInputHash: "startup",
		Sources: []runtimecontract.RuntimeDefinitionSource{{CandidateItemRef: "goal", ItemRef: "goal-formal", SourceScope: "goal", SourceID: "goal", Text: goal.Text},
			{CandidateItemRef: "dod", ItemRef: "dod-formal", SourceScope: "dodItem", SourceID: "dod-001", Text: dod.Items[0]}},
		Requirements: []runtimecontract.RuntimeSourceRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"goal", "dod"}, Text: "源要求"}},
		Obligations: []runtimecontract.RuntimeSourceObligation{{ID: "A", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: " A 独立判据\n "},
			{ID: "B", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "B 独立判据"}},
		Verifications: []runtimecontract.RuntimeSourceVerification{{ID: "verify-A", ObligationID: "A", Declaration: "验证 A", ProfileID: "test-deterministic-proof-dodItem", EvidenceRequirementID: "dod-001", Scope: "dodItem"},
			{ID: "verify-B", ObligationID: "B", Declaration: "验证 B", ProfileID: "test-deterministic-proof-dodItem", EvidenceRequirementID: "dod-001", Scope: "dodItem"}},
	}).WithHash()
	activation := withTestDeterministicProof(goal, dod, runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
	})
	bootstrap, err := RuntimeBootstrapWithSourceObligations(goal, dod, inventory.DefinitionRevision, inventory.StartupInputHash, nil, activation, &inventory)
	if err != nil {
		t.Fatal(err)
	}
	return bootstrap, inventory
}

func TestRuntimeBootstrapSourceObligationsKeepOneCWIAndExactRequirement(t *testing.T) {
	bootstrap, inventory := sourceObligationBootstrapFixture(t)
	if len(bootstrap.CoverageItems) != 1 || len(bootstrap.DefinitionFocuses) != 1 || len(bootstrap.CoverageItems[0].SourceObligationChecks) != 2 ||
		!reflect.DeepEqual(bootstrap.DefinitionSeed.SourceObligations, &inventory) {
		t.Fatalf("shared CWI lost source inventory: %+v", bootstrap)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(bootstrap.Definition.Revision, bootstrap.CoverageItems, bootstrap.DefinitionFocuses)
	if err != nil || len(requirements) != 1 || len(requirements[0].SourceObligationChecks) != 2 {
		t.Fatalf("source proof mapping: %+v %v", requirements, err)
	}
	for _, check := range requirements[0].SourceObligationChecks {
		found := false
		for _, ref := range requirements[0].RequiredCheckRefs {
			if ref == check.CheckRef {
				found = true
			}
		}
		if !found {
			t.Fatalf("source check omitted: %+v", check)
		}
	}
	facts, err := bootstrap.InitialFacts(common.WorkspaceID("workspace"), common.RunID("run"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(facts.DefinitionSeed.SourceObligations, &inventory) {
		t.Fatal("initial facts lost immutable source inventory")
	}
	clone := bootstrap.Clone()
	clone.DefinitionSeed.SourceObligations.Obligations[0].DoDItemRefs[0] = "changed"
	clone.CoverageItems[0].SourceObligationChecks[0].Criterion = "changed"
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("clone changed original bootstrap: %v", err)
	}
}

func TestRuntimeSourceApplicabilityIsRecheckedByBootstrapAndFreshFacts(t *testing.T) {
	bootstrap, _ := sourceObligationBootstrapFixture(t)
	for _, test := range []struct {
		name   string
		change func(*runtimecontract.RuntimeBootstrap)
	}{
		{"wrong Profile", func(b *runtimecontract.RuntimeBootstrap) {
			b.Definition.SourceObligations.Verifications[1].ProfileID = "other"
		}},
		{"wrong evidence requirement", func(b *runtimecontract.RuntimeBootstrap) {
			b.Definition.SourceObligations.Verifications[1].EvidenceRequirementID = "dod-002"
		}},
		{"wrong scope", func(b *runtimecontract.RuntimeBootstrap) {
			b.Definition.SourceObligations.Verifications[1].Scope = "goalCoverage"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts, err := bootstrap.InitialFacts("workspace", "run")
			if err != nil {
				t.Fatal(err)
			}
			changed := bootstrap.Clone()
			test.change(&changed)
			inventory := changed.Definition.SourceObligations.WithHash()
			changed.Definition.SourceObligations = &inventory
			changed.DefinitionSeed.SourceObligations = &inventory
			checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(inventory, changed.CoverageItems[0])
			if err != nil {
				t.Fatal(err)
			}
			changed.CoverageItems[0].SourceObligationChecks = checks
			changed.DefinitionSeed.CoverageItems = changed.CoverageItems
			changed.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(changed.DefinitionSeed)
			if err := changed.DefinitionSeed.Validate(); err != nil {
				t.Fatalf("反例 source graph 必须自洽：%v", err)
			}
			if err := changed.Validate(); err == nil || !strings.Contains(err.Error(), "source applicability") {
				t.Fatalf("bootstrap 未在来源适用性 gate 拒绝：%v", err)
			}
			facts.DefinitionSeed = changed.DefinitionSeed
			body, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded runtimecontract.RuntimeFacts
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "source applicability") {
				t.Fatalf("fresh facts 未在来源适用性 gate 拒绝：%v", err)
			}
			if err := bootstrap.Validate(); err != nil {
				t.Fatalf("校验改写原 bootstrap：%v", err)
			}
		})
	}
}

func TestRuntimeBootstrapRejectsSourceCheckOmissionAndWrongStartup(t *testing.T) {
	bootstrap, _ := sourceObligationBootstrapFixture(t)
	changed := bootstrap.Clone()
	changed.CoverageItems[0].SourceObligationChecks = changed.CoverageItems[0].SourceObligationChecks[:1]
	changed.DefinitionSeed.CoverageItems = changed.CoverageItems
	changed.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(changed.DefinitionSeed)
	if changed.Validate() == nil {
		t.Fatal("coordinated seed/CWI omission of B accepted")
	}
	changed = bootstrap.Clone()
	changed.Definition.StartupInputHash = "different-startup"
	if changed.Validate() == nil {
		t.Fatal("source inventory startup drift accepted")
	}
}
