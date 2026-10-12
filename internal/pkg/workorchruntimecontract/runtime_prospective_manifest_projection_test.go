package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 固定优化前的完整 manifest 规范化路径，不调用新 projection helper。
func referenceProspectiveObservationForProposal(value RuntimeOwnerFactProspectiveSatisfactionObservation, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition, delta RuntimeProspectiveWorkspaceManifestDelta) error {
	value = value.Normalized()
	manifest = manifest.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Satisfaction != nil {
		return referenceProspectiveSatisfactionForProposal(*value.Satisfaction, manifest, proposal, acquisition, delta)
	}
	catalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, acquisition.Requirement.OwnerModuleID)
	if !found || value.Stale.PrerequisiteID != catalog.PrerequisiteID || value.Stale.OwnerModuleID != catalog.OwnerModuleID || value.Stale.ExpectedRevision != catalog.ObservedRevision {
		return fmt.Errorf("runtime owner-fact prospective satisfaction stale result does not match sealed dependency catalog")
	}
	return nil
}

func referenceProspectiveSatisfactionForProposal(value RuntimeOwnerFactProspectiveSatisfaction, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition, delta RuntimeProspectiveWorkspaceManifestDelta) error {
	value = value.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	acquisition = acquisition.Normalized()
	delta = delta.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.WorkspaceID != manifest.WorkspaceID || value.RunID != manifest.RunID || value.ManifestHash != manifest.Hash || value.DefinitionSeedHash != manifest.DefinitionSeedHash || value.CoverageWorkItemID != proposal.CoverageWorkItemID || value.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || value.Requirement != acquisition.Requirement || value.ProspectiveSatisfactionContractRef != acquisition.ProspectiveSatisfactionContractRef || value.OwnerModuleID != acquisition.Requirement.OwnerModuleID || value.ProspectiveWorkspaceManifestDeltaHash != StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction does not match sealed proposal and File delta")
	}
	catalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, acquisition.Requirement.OwnerModuleID)
	if !found || value.CatalogSnapshotID != catalog.SnapshotID || value.ObservedRevision != catalog.ObservedRevision {
		return fmt.Errorf("runtime owner-fact prospective satisfaction does not match sealed dependency catalog")
	}
	return nil
}

func referenceProspectiveDeltaForProposal(delta RuntimeProspectiveWorkspaceManifestDelta, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition) error {
	delta = delta.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	acquisition = acquisition.Normalized()
	if delta.WorkspaceID != manifest.WorkspaceID || delta.RunID != manifest.RunID || delta.ManifestHash != manifest.Hash || delta.DefinitionSeedHash != manifest.DefinitionSeedHash || delta.CoverageWorkItemID != proposal.CoverageWorkItemID || delta.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || delta.OwnerModuleID != proposal.OwnerModuleID || delta.CatalogSnapshotID != acquisition.OwnerCatalogSnapshotID || delta.ProspectiveSatisfactionContractRef != acquisition.ProspectiveSatisfactionContractRef || delta.BaseWorkspaceManifestRevision != acquisition.OwnerCatalogObservedRevision || !sameStrings(delta.ProspectiveTargetRefs, RuntimeSemanticProposalTargetRefs(proposal)) {
		return fmt.Errorf("runtime prospective workspace manifest delta does not match sealed prospective proposal")
	}
	return nil
}

type prospectiveProjectionFixture struct {
	Manifest    RuntimeSemanticPlanManifest
	Proposal    RuntimeSemanticProofProposal
	Acquisition RuntimeSemanticOwnerFactAcquisition
	Delta       RuntimeProspectiveWorkspaceManifestDelta
	Observation RuntimeOwnerFactProspectiveSatisfactionObservation
}

func prospectiveProjectionFixtureForTest(t *testing.T) prospectiveProjectionFixture {
	t.Helper()
	manifest, _, proposal := semanticMissingOwnerFactAcquisitionFixture(t)
	fileCatalog, found := runtimeSemanticOwnerCatalogFor(manifest, RuntimeModuleFile)
	if !found {
		t.Fatal("缺少 File catalog")
	}
	delta, err := NewRuntimeProspectiveWorkspaceManifestDelta(manifest, proposal, runtimeProspectiveSatisfactionContractRefForTest, fileCatalog, WorkspaceManifest{Revision: fileCatalog.ObservedRevision, TargetRefs: append([]string(nil), fileCatalog.TargetRefs...)})
	if err != nil {
		t.Fatal(err)
	}
	catalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, RuntimeModuleShell)
	if !found {
		t.Fatal("缺少 Shell dependency catalog")
	}
	observed, recipes := runtimeProspectiveSatisfactionOwnerFactsForTest(t, catalog, delta.BaseWorkspaceManifestRevision)
	acquisition := *manifest.ProofObligations[0].OwnerFactAcquisition
	request := RuntimeOwnerFactProspectiveSatisfactionRequest{WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "projection-batch", Manifest: manifest, Proposal: proposal, Requirement: acquisition.Requirement, ProspectiveSatisfactionContractRef: runtimeProspectiveSatisfactionContractRefForTest, OwnerCatalog: catalog, CatalogObservation: &observed, ValidationRecipes: recipes, ProspectiveWorkspaceManifestDelta: delta}
	observation, err := NewRuntimeOwnerFactProspectiveSatisfactionObservation(request, RuntimeOwnerFactProspectiveWouldSatisfy, []string{recipes[0].ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	return prospectiveProjectionFixture{manifest, proposal, acquisition, delta, observation}
}

func assertProspectiveProjectionEquivalent(t testing.TB, fixture prospectiveProjectionFixture) {
	t.Helper()
	before, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	compare := func(got, want error) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("首个拒绝改变: got=%v want=%v", got, want)
		}
	}
	for range 2 {
		compare(validateRuntimeProspectiveWorkspaceManifestDeltaForProposal(fixture.Delta, fixture.Manifest, fixture.Proposal, fixture.Acquisition), referenceProspectiveDeltaForProposal(fixture.Delta, fixture.Manifest, fixture.Proposal, fixture.Acquisition))
		compare(validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta), referenceProspectiveObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta))
		if fixture.Observation.Satisfaction != nil {
			compare(validateRuntimeOwnerFactProspectiveSatisfactionForProposal(*fixture.Observation.Satisfaction, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta), referenceProspectiveSatisfactionForProposal(*fixture.Observation.Satisfaction, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta))
		}
	}
	after, err := json.Marshal(fixture)
	if err != nil || string(before) != string(after) {
		t.Fatal("纯校验改写原输入")
	}
	var reloaded prospectiveProjectionFixture
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	compare(validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(reloaded.Observation, reloaded.Manifest, reloaded.Proposal, reloaded.Acquisition, reloaded.Delta), referenceProspectiveObservationForProposal(reloaded.Observation, reloaded.Manifest, reloaded.Proposal, reloaded.Acquisition, reloaded.Delta))
}

func mutateProspectiveProjection(fixture *prospectiveProjectionFixture, value string, mode uint8) {
	switch mode % 13 {
	case 0:
		fixture.Manifest.WorkspaceID = common.WorkspaceID(value)
	case 1:
		fixture.Manifest.RunID = common.RunID(value)
	case 2:
		fixture.Manifest.Hash = value
	case 3:
		fixture.Manifest.DefinitionSeedHash = value
	case 4:
		fixture.Manifest.OwnerFactDependencyCatalogs[0].ObservedRevision = value
	case 5:
		fixture.Manifest.OwnerFactDependencyCatalogs[0].OwnerModuleID = value
	case 6:
		fixture.Manifest.OwnerFactDependencyCatalogs = append(fixture.Manifest.OwnerFactDependencyCatalogs, fixture.Manifest.OwnerFactDependencyCatalogs[0])
	case 7:
		fixture.Manifest.OwnerFactDependencyCatalogs = nil
	case 8:
		fixture.Proposal.Target.ProspectiveTargetRefs = []string{value}
	case 9:
		fixture.Acquisition.ProspectiveSatisfactionContractRef = value
	case 10:
		fixture.Delta.BaseWorkspaceManifestRevision = value
	case 11:
		fixture.Observation.Satisfaction.CatalogSnapshotID = value
	case 12:
		fixture.Observation.Satisfaction.ManifestHash = value
	}
}

func TestProspectiveManifestProjectionPreservesFullNormalization(t *testing.T) {
	fixture := prospectiveProjectionFixtureForTest(t)
	if err := referenceProspectiveObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta); err != nil {
		t.Fatal(err)
	}
	assertProspectiveProjectionEquivalent(t, fixture)
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for mode := uint8(0); mode < 13; mode++ {
		for _, value := range []string{"", " ", "different", " " + fixture.Manifest.Hash + " ", "中文", "\xff"} {
			var changed prospectiveProjectionFixture
			if err := json.Unmarshal(encoded, &changed); err != nil {
				t.Fatal(err)
			}
			mutateProspectiveProjection(&changed, value, mode)
			assertProspectiveProjectionEquivalent(t, changed)
		}
	}
	stale := fixture
	catalog := fixture.Manifest.OwnerFactDependencyCatalogs[0]
	stale.Observation = RuntimeOwnerFactProspectiveSatisfactionObservation{FeasibilityBatchID: fixture.Observation.FeasibilityBatchID, RequestHash: fixture.Observation.RequestHash, Stale: &RuntimePrerequisiteStale{
		PrerequisiteID: catalog.PrerequisiteID, ObservationID: "original-observation", Domain: RuntimeResourceOwnerCatalog,
		OwnerModuleID: catalog.OwnerModuleID, SubjectRef: "managed-validation", ExpectedRevision: catalog.ObservedRevision,
		ObservedRevision: "changed-revision", Reason: "owner policy changed",
	}}
	if err := referenceProspectiveObservationForProposal(stale.Observation, stale.Manifest, stale.Proposal, stale.Acquisition, stale.Delta); err != nil {
		t.Fatal(err)
	}
	assertProspectiveProjectionEquivalent(t, stale)
	stale.Observation.Stale.ExpectedRevision = "wrong-revision"
	assertProspectiveProjectionEquivalent(t, stale)
	stale.Observation.Stale = &RuntimePrerequisiteStale{}
	assertProspectiveProjectionEquivalent(t, stale)
	raw := fixture
	raw.Manifest.WorkspaceID = common.WorkspaceID(" " + string(raw.Manifest.WorkspaceID) + " ")
	raw.Manifest.RunID = common.RunID(" " + string(raw.Manifest.RunID) + " ")
	raw.Manifest.Hash = " " + raw.Manifest.Hash + " "
	raw.Manifest.DefinitionSeedHash = " " + raw.Manifest.DefinitionSeedHash + " "
	assertProspectiveProjectionEquivalent(t, raw)
	irrelevant := fixture
	irrelevant.Manifest.OwnerCatalogs = nil
	irrelevant.Manifest.ProofObligations = nil
	irrelevant.Manifest.CoverageItems = nil
	assertProspectiveProjectionEquivalent(t, irrelevant)
	projection := normalizeRuntimeProspectiveManifestProjection(fixture.Manifest)
	projection.OwnerFactDependencyCatalogs[0].TargetRefs = []string{"changed"}
	if reflect.DeepEqual(projection.OwnerFactDependencyCatalogs[0].TargetRefs, fixture.Manifest.OwnerFactDependencyCatalogs[0].TargetRefs) {
		t.Fatal("projection 共享 catalog")
	}
	later := fixture
	later.Manifest.Hash = "later-change"
	if validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(later.Observation, later.Manifest, later.Proposal, later.Acquisition, later.Delta) == nil {
		t.Fatal("后续输入变化复用旧成功")
	}
}

func TestProspectiveManifestProjectionReducesAllocations(t *testing.T) {
	fixture := prospectiveProjectionFixtureForTest(t)
	optimized := testing.AllocsPerRun(20, func() {
		if err := validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta); err != nil {
			panic(err)
		}
	})
	original := testing.AllocsPerRun(20, func() {
		if err := referenceProspectiveObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta); err != nil {
			panic(err)
		}
	})
	if optimized >= original {
		t.Fatalf("未减少分配: optimized=%v original=%v", optimized, original)
	}
	t.Logf("allocations optimized=%v original=%v", optimized, original)
	for _, implementation := range []struct {
		name string
		run  func() error
	}{
		{"projection", func() error {
			return validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta)
		}},
		{"reference", func() error {
			return referenceProspectiveObservationForProposal(fixture.Observation, fixture.Manifest, fixture.Proposal, fixture.Acquisition, fixture.Delta)
		}},
	} {
		result := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := implementation.run(); err != nil {
					b.Fatal(err)
				}
			}
		})
		t.Logf("%s: %s %s", implementation.name, result.String(), result.MemString())
	}
}

func FuzzProspectiveManifestProjectionMatchesOriginal(f *testing.F) {
	// 每轮构造完整原值；不跨轮保存成功校验结果。
	for _, seed := range []string{"", "different", " ", "中文", "\xff"} {
		f.Add(seed, uint8(0))
	}
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		if len(value) > 256 {
			return
		}
		fixture := prospectiveProjectionFixtureForTest(t)
		mutateProspectiveProjection(&fixture, strings.Clone(value), mode)
		assertProspectiveProjectionEquivalent(t, fixture)
	})
}
