package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeScaleBenchmarkCoverageCount = runtimecontract.RuntimeAcceptanceFocusMaxCount

var (
	runtimeScaleBenchmarkProjection runtimecontract.RuntimeCWIEligibilityProjection
	runtimeScaleBenchmarkSchema     string
	runtimeScaleBenchmarkDecision   runtimecontract.RuntimeDecision
	runtimeScaleBenchmarkSignature  string
)

func BenchmarkRuntimeAggregateInvariantValidation64CWI(b *testing.B) {
	aggregate := runtimeScaleBenchmarkAggregate(b, runtimeScaleBenchmarkCoverageCount)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := aggregate.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRuntimeLongFocusQueueProjection64CWI(b *testing.B) {
	aggregate := runtimeScaleBenchmarkAggregate(b, runtimeScaleBenchmarkCoverageCount)
	providerOrder := make([]common.CoverageWorkItemID, 0, len(aggregate.Facts.Coverage))
	for index := len(aggregate.Facts.Coverage) - 1; index >= 0; index-- {
		providerOrder = append(providerOrder, aggregate.Facts.Coverage[index].ID)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		projection, err := runtimecontract.ProjectRuntimeCWIEligibility(aggregate.Facts, providerOrder)
		if err != nil {
			b.Fatal(err)
		}
		runtimeScaleBenchmarkProjection = projection
	}
}

func BenchmarkRuntimeSemanticManifestRendering64CWI(b *testing.B) {
	aggregate := runtimeScaleBenchmarkAggregate(b, runtimeScaleBenchmarkCoverageCount)
	identity := semanticplan.ManifestIdentity{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		Execution:        aggregate.Facts.Activation.ExecutionContract,
		Admissibilities:  aggregate.Facts.Resources.OperationAdmissibilities,
		EligibleCoverage: runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		manifest, err := semanticplan.CompileManifest(identity)
		if err != nil {
			b.Fatal(err)
		}
		schema, err := runtimecontract.RuntimeSemanticPlanJSONSchemaForRequest(
			manifest,
			runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
			"",
			0,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}
		runtimeScaleBenchmarkSchema = schema
	}
}

func BenchmarkRuntimeDriveLoopReduction64CWI(b *testing.B) {
	aggregate := runtimeScaleBenchmarkAggregate(b, runtimeScaleBenchmarkCoverageCount)
	previous := aggregate.Decision.AuthoritySnapshot
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		facts, decision, err := ReduceRuntimeStateWithDirective(
			aggregate.Facts,
			&previous,
			aggregate.CoverageProofLineage,
		)
		if err != nil {
			b.Fatal(err)
		}
		candidate := aggregate
		candidate.Facts = facts
		candidate.Decision = decision
		runtimeScaleBenchmarkDecision = decision
		runtimeScaleBenchmarkSignature = RuntimeConvergenceProgressSignature(candidate)
	}
}

func runtimeScaleBenchmarkAggregate(b *testing.B, count int) RuntimeAggregate {
	b.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		b.Fatal(err)
	}
	aggregate, err := controller.initialize(
		context.Background(),
		runtimeScaleBenchmarkFacts(count),
		controllerDefinition(),
		nil,
	)
	if err != nil {
		b.Fatal(err)
	}
	if got := len(aggregate.Facts.Coverage); got != count {
		b.Fatalf("runtime scale fixture has %d CWI, want %d", got, count)
	}
	if got := 1 + len(aggregate.Facts.RemainingFocuses); got != count {
		b.Fatalf("runtime scale fixture has %d active/pending focuses, want %d", got, count)
	}
	return aggregate
}

func runtimeScaleBenchmarkFacts(count int) runtimecontract.RuntimeFacts {
	facts := controllerFacts()
	facts.CWILedgerSchemaVersion = ""
	facts.Focus = runtimecontract.CoverageFocus{}
	facts.RemainingFocuses = nil
	facts.CompletedFocuses = nil
	facts.DefinitionFocuses = nil
	facts.Coverage = nil
	facts.Slices = nil
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()

	seedItems := make([]runtimecontract.RuntimeCoverageItem, 0, count)
	seedSlices := make([]runtimecontract.RuntimeAcceptanceSlice, 0, count)
	for index := 0; index < count; index++ {
		coverageID := common.CoverageWorkItemID(fmt.Sprintf("cwi-scale-%02d", index))
		sliceID := fmt.Sprintf("slice-scale-%02d", index)
		focus := runtimecontract.CoverageFocus{
			ID: fmt.Sprintf("focus-scale-%02d", index), SliceID: sliceID,
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{coverageID},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AcceptedStepKinds:     []string{runtimecontract.StepKindFileWrite},
			TargetRefs:            []string{"target.txt"},
		}
		if index == 0 {
			facts.Focus = focus
		} else {
			facts.RemainingFocuses = append(facts.RemainingFocuses, focus)
		}
		item := runtimecontract.RuntimeCoverageItem{
			ID: coverageID, SliceID: sliceID, SourceScope: "dodItem",
			SourceID: fmt.Sprintf("dod-scale-%02d", index), TargetText: fmt.Sprintf("deliver scale item %02d", index),
			Order: count - index,
		}
		facts.Coverage = append(facts.Coverage, runtimecontract.RuntimeCoverageItemFact{
			ID: item.ID, SliceID: item.SliceID, SourceScope: item.SourceScope,
			SourceID: item.SourceID, TargetText: item.TargetText,
			State: runtimecontract.RuntimeCoverageItemStateOpen,
		})
		slice := runtimecontract.RuntimeAcceptanceSlice{
			ID: sliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
		}
		facts.Slices = append(facts.Slices, slice)
		seedItems = append(seedItems, item)
		seedSlices = append(seedSlices, slice)
	}
	facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion:      runtimecontract.RuntimeDefinitionSeedSchemaCurrent,
		DefinitionRevision: facts.DefinitionRevision,
		PolicyHash:         "policy-scale-benchmark",
		CoverageItems:      seedItems,
		Slices:             seedSlices,
	}
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	return facts
}
