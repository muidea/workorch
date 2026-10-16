package runtime

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Detached projection fixture only. The shared compiler tests separately
// prove exact owner/dispatch/receipt members; this is not a live owner flow.
func historicalBehaviorProjectionFixture(t *testing.T) RuntimeAggregate {
	t.Helper()
	inventory := (runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent, CandidateHash: "confirmed-candidate",
		SpecificationHash: "confirmed-specification", DefinitionRevision: "definition", StartupInputHash: "startup",
		Sources: []runtimecontract.RuntimeDefinitionSource{
			{CandidateItemRef: "goal", ItemRef: "formal-goal", SourceScope: "goal", SourceID: "goal", Text: "原目标"},
			{CandidateItemRef: "history", ItemRef: "formal-history", SourceScope: "dodItem", SourceID: "history", Text: "原历史行为"},
			{CandidateItemRef: "acceptance", ItemRef: "formal-acceptance", SourceScope: "dodItem", SourceID: "acceptance", Text: "原当前验收"},
		},
		Requirements: []runtimecontract.RuntimeSourceRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"goal", "history", "acceptance"}, Text: "原要求"}},
		Obligations: []runtimecontract.RuntimeSourceObligation{
			{ID: "historical", RequirementID: "requirement", DoDItemRefs: []string{"history"}, Text: "原历史行为准则"},
			{ID: "current", RequirementID: "requirement", DoDItemRefs: []string{"acceptance"}, Text: "原当前验收准则"},
		},
		Verifications: []runtimecontract.RuntimeSourceVerification{
			{ID: "history-check", ObligationID: "historical", Declaration: "真实行为", ProfileID: "profile", EvidenceRequirementID: "history", Scope: "dodItem"},
			{ID: "acceptance-check", ObligationID: "current", Declaration: "真实普通验收", ProfileID: "profile", EvidenceRequirementID: "acceptance", Scope: "dodItem"},
		},
		EvidencePurposeMappings: []runtimecontract.RuntimeEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurposeV1, HistoricalItemRef: "history", AcceptanceItemRef: "acceptance"}},
	}).WithHash()
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(inventory, []runtimecontract.RuntimeCoverageItem{
		{ID: "history", SourceScope: "dodItem", SourceID: "history", TargetText: "原历史行为"},
		{ID: "acceptance", SourceScope: "dodItem", SourceID: "acceptance", TargetText: "原当前验收"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := range items {
		items[index].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	facts := runtimecontract.RuntimeFacts{CWILedgerSchemaVersion: runtimecontract.RuntimeCWILedgerSchemaCurrent, DefinitionRevision: "definition",
		DefinitionSeed:   runtimecontract.RuntimeDefinitionSeed{SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition", SourceObligations: &inventory, CoverageItems: items},
		Focus:            runtimecontract.CoverageFocus{ID: "history-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"history"}},
		RemainingFocuses: []runtimecontract.CoverageFocus{{ID: "acceptance-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"acceptance"}}},
		Coverage:         runtimecontract.InitialRuntimeCoverageFacts(items),
	}
	for index := range facts.Coverage {
		row := &facts.Coverage[index]
		row.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
		row.FocusID = "acceptance-focus"
		if row.ID != "history" {
			continue
		}
		var declaration *runtimecontract.RuntimeHistoricalAcceptanceSource
		for _, item := range items {
			if item.ID == row.ID {
				declaration = item.HistoricalAcceptance
			}
		}
		ref := runtimecontract.RuntimeCWIBindingRef{BindingID: "binding", BindingHash: "hash", FocusID: "history-focus", SemanticManifestHash: "original-generation"}
		row.CWIConvergenceState, row.CurrentBindingRef, row.FocusID = runtimecontract.RuntimeCWIConvergenceStateBound, &ref, ref.FocusID
		row.BindingLineage = []runtimecontract.RuntimeCWIBindingRef{ref}
		row.HistoricalBehavior = &runtimecontract.RuntimeHistoricalBehaviorCompletion{Source: *declaration, BindingRef: ref, DispatchID: "dispatch", EvidenceRefs: []string{"effect"}}
	}
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "file/workspace", ResourceKind: "workspace_target", ResourceID: "original", Revision: "before", DependencyHash: "original-read"}}}).WithHash()
	post := *runtimecontract.CloneRuntimeOwnerRevisionSet(&reads)
	post.Entries[0].Revision = "after"
	post = post.WithHash()
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "effect", Kind: runtimecontract.EvidenceKindStateChange, FocusID: "history-focus", DispatchID: "dispatch", BindingRefs: []string{"binding"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"history"}, Quality: common.EvidenceQualityValid, OwnerRevisionSet: &reads,
		ApplicabilityDependencies: &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV2, OwnerRevisionSet: post, PurposeRef: runtimecontract.RuntimeHistoricalEffectProofV1, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite, OwnerContractRef: runtimecontract.RuntimeFileHistoricalEffectProofV1}}}
	return RuntimeAggregate{Facts: facts}
}

func TestHistoricalBehaviorHandoffPreservesOpenRowAndOriginalHistoryAcrossReload(t *testing.T) {
	aggregate := historicalBehaviorProjectionFixture(t)
	before := aggregate.CloneRaw()
	changed, err := handoffHistoricalBehaviorFocus(&aggregate)
	if err != nil || !changed || aggregate.Facts.Focus.ID != "acceptance-focus" || len(aggregate.Facts.CompletedFocuses) != 1 || len(aggregate.Facts.RemainingFocuses) != 0 {
		t.Fatalf("exact handoff failed: changed=%v err=%v", changed, err)
	}
	row := runtimeCoverageByID(aggregate.Facts.Coverage)["history"]
	if row.State != runtimecontract.RuntimeCoverageItemStateOpen || row.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || len(row.EvidenceRefs) != 0 || row.LastAssessmentID != "" || !reflect.DeepEqual(before.Facts.Evidence, aggregate.Facts.Evidence) {
		t.Fatal("behavior handoff granted terminal credit or rewrote original evidence")
	}
	body, _ := json.Marshal(aggregate)
	var reload RuntimeAggregate
	if err := json.Unmarshal(body, &reload); err != nil {
		t.Fatal(err)
	}
	if changed, err := handoffHistoricalBehaviorFocus(&reload); err != nil || changed || !reflect.DeepEqual(reload, aggregate) {
		t.Fatal("restarted/duplicate handoff mutated queue or history")
	}
	byID, err := runtimeCWICoverageIndexes(aggregate.Facts.Coverage)
	if err != nil {
		t.Fatal(err)
	}
	if retained := appendRetainedRuntimeCWIFocuses(aggregate.Facts, nil, byID, nil); len(retained) != 0 {
		t.Fatal("consumer binding requeued completed create-only historical behavior")
	}
}

func TestHistoricalBehaviorCompletionTransitionIsImmutableAndReceiptCASOnly(t *testing.T) {
	next := historicalBehaviorProjectionFixture(t)
	previous := next.CloneRaw()
	for index := range previous.Facts.Coverage {
		previous.Facts.Coverage[index].HistoricalBehavior = nil
	}
	if validateHistoricalBehaviorTransition(previous, next) == nil {
		t.Fatal("detached completion escaped original complete receipt CAS")
	}
	previous.ActiveDispatch = &SealedDispatch{ID: "dispatch"}
	if err := validateHistoricalBehaviorTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	previous = next.CloneRaw()
	if err := validateHistoricalBehaviorTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	for index := range next.Facts.Coverage {
		if next.Facts.Coverage[index].HistoricalBehavior != nil {
			next.Facts.Coverage[index].HistoricalBehavior.EvidenceRefs[0] = "replacement"
		}
	}
	if validateHistoricalBehaviorTransition(previous, next) == nil {
		t.Fatal("rehashed/reloaded historical completion replaced immutable receipt source")
	}
}

func TestHistoricalBehaviorHandoffPersistsFirstOfSharedConsumerSources(t *testing.T) {
	aggregate := historicalBehaviorProjectionFixture(t)
	inventory := aggregate.Facts.DefinitionSeed.SourceObligations.Clone()
	inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: "second", ItemRef: "formal-second", SourceScope: "dodItem", SourceID: "second", Text: "第二项原历史行为"})
	inventory.Obligations = append(inventory.Obligations, runtimecontract.RuntimeSourceObligation{ID: "second-obligation", RequirementID: "requirement", DoDItemRefs: []string{"second"}, Text: "第二项原历史行为准则"})
	inventory.Verifications = append(inventory.Verifications, runtimecontract.RuntimeSourceVerification{ID: "second-check", ObligationID: "second-obligation", Declaration: "第二项真实行为", ProfileID: "profile", EvidenceRequirementID: "second", Scope: "dodItem"})
	inventory.EvidencePurposeMappings = append(inventory.EvidencePurposeMappings, runtimecontract.RuntimeEvidencePurposeMapping{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurposeV1, HistoricalItemRef: "second", AcceptanceItemRef: "acceptance"})
	inventory = inventory.WithHash()
	items := runtimecontract.NormalizeRuntimeCoverageItems(aggregate.Facts.DefinitionSeed.CoverageItems)
	items = append(items, runtimecontract.RuntimeCoverageItem{ID: "second", SourceScope: "dodItem", SourceID: "second", TargetText: "第二项原历史行为"})
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(inventory, items)
	if err != nil {
		t.Fatal(err)
	}
	for index := range items {
		items[index].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	aggregate.Facts.DefinitionSeed.CoverageItems = items
	aggregate.Facts.DefinitionSeed.SourceObligations = &inventory
	rows := runtimecontract.InitialRuntimeCoverageFacts(items)
	for index := range rows {
		switch rows[index].ID {
		case "history":
			original := runtimeCoverageByID(aggregate.Facts.Coverage)["history"]
			rows[index].FocusID, rows[index].CurrentBindingRef, rows[index].BindingLineage = original.FocusID, original.CurrentBindingRef, original.BindingLineage
			rows[index].CWIConvergenceState, rows[index].HistoricalBehavior = original.CWIConvergenceState, original.HistoricalBehavior
			for _, item := range items {
				if item.ID == "history" {
					rows[index].HistoricalBehavior.Source = *item.HistoricalAcceptance
				}
			}
		case "second":
			rows[index].FocusID, rows[index].CWIConvergenceState = "second-focus", runtimecontract.RuntimeCWIConvergenceStateUnbound
		case "acceptance":
			rows[index].FocusID, rows[index].CWIConvergenceState = "acceptance-focus", runtimecontract.RuntimeCWIConvergenceStateUnbound
		}
	}
	aggregate.Facts.Coverage = rows
	aggregate.Facts.RemainingFocuses = append(aggregate.Facts.RemainingFocuses, runtimecontract.CoverageFocus{ID: "second-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"second"}})
	before := aggregate.CloneRaw()
	t.Run("already-bound independent source", func(t *testing.T) {
		bound := before.CloneRaw()
		binding := runtimecontract.CoverageEvidenceBinding{ID: "second-binding", FocusID: "second-focus", CoverageWorkItemID: "second"}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, "original-bound-generation")
		for index := range bound.Facts.Coverage {
			if bound.Facts.Coverage[index].ID == "second" {
				bound.Facts.Coverage[index].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
				bound.Facts.Coverage[index].CurrentBindingRef = &ref
			}
		}
		bound.Facts.RemainingFocuses[1].CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
		bound.SemanticPlanning.Manifest = &runtimecontract.RuntimeSemanticPlanManifest{Hash: ref.SemanticManifestHash}
		if got := runtimecontract.RuntimeCWIsEligibleForPlanning(bound.Facts); len(got) != 0 {
			t.Fatalf("bound projection fixture unexpectedly has planning members: %v", got)
		}
		changed, err := handoffHistoricalBehaviorFocus(&bound)
		if err != nil || !changed || bound.Facts.Focus.ID != "second-focus" {
			t.Fatalf("complete receipt could not retain exact bound successor: %v", err)
		}
		if !reflect.DeepEqual(bound.Facts.Evidence, before.Facts.Evidence) || !reflect.DeepEqual(runtimeCoverageByID(bound.Facts.Coverage)["history"].HistoricalBehavior, runtimeCoverageByID(before.Facts.Coverage)["history"].HistoricalBehavior) {
			t.Fatal("bound handoff rewrote original effect")
		}
	})
	changed, err := handoffHistoricalBehaviorFocus(&aggregate)
	if err != nil || !changed || aggregate.Facts.Focus.ID != "second-focus" || len(aggregate.Facts.CompletedFocuses) != 1 || len(aggregate.Facts.RemainingFocuses) != 1 || aggregate.Facts.RemainingFocuses[0].ID != "acceptance-focus" {
		t.Fatalf("first real receipt could not hand off to independently eligible prerequisite: changed=%v err=%v", changed, err)
	}
	row := runtimeCoverageByID(aggregate.Facts.Coverage)["history"]
	if row.State != runtimecontract.RuntimeCoverageItemStateOpen || !reflect.DeepEqual(row.HistoricalBehavior, runtimeCoverageByID(before.Facts.Coverage)["history"].HistoricalBehavior) || !reflect.DeepEqual(aggregate.Facts.Evidence, before.Facts.Evidence) {
		t.Fatal("shared-consumer progress changed historical proof or granted credit")
	}
	if runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(aggregate.Facts, row, "second") {
		t.Fatal("historical progress granted another CWI prerequisite credit")
	}
}

func TestHistoricalAcceptanceTransitionIsOriginalAssessmentCASAndAppendOnly(t *testing.T) {
	previous := historicalBehaviorProjectionFixture(t)
	next := previous.CloneRaw()
	for index := range next.Facts.Coverage {
		if next.Facts.Coverage[index].ID == "history" {
			next.Facts.Coverage[index].HistoricalAcceptanceProofs = []runtimecontract.RuntimeHistoricalAcceptanceProof{{AssessmentDispatchID: "ordinary-assessment"}}
		}
	}
	if validateHistoricalBehaviorTransition(previous, next) == nil {
		t.Fatal("settlement appended without original assessment CAS")
	}
	previous.ActiveDispatch = &SealedDispatch{ID: "ordinary-assessment", Candidate: runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{}}}
	if err := validateHistoricalBehaviorTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	previous = next.CloneRaw()
	if err := validateHistoricalBehaviorTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	for index := range next.Facts.Coverage {
		if next.Facts.Coverage[index].ID == "history" {
			next.Facts.Coverage[index].HistoricalAcceptanceProofs = nil
		}
	}
	if validateHistoricalBehaviorTransition(previous, next) == nil {
		t.Fatal("revocation deleted immutable original settlement proof")
	}
}

func TestHistoricalAcceptanceRevocationLeavesBehaviorOutsideExecutionQueue(t *testing.T) {
	aggregate := historicalBehaviorProjectionFixture(t)
	if _, err := handoffHistoricalBehaviorFocus(&aggregate); err != nil {
		t.Fatal(err)
	}
	for index := range aggregate.Facts.Coverage {
		row := &aggregate.Facts.Coverage[index]
		if row.ID != "history" {
			row.State, row.CWIConvergenceState, row.CurrentBindingRef = runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateReopened, nil
			continue
		}
		row.HistoricalAcceptanceProofs = []runtimecontract.RuntimeHistoricalAcceptanceProof{{Source: row.HistoricalBehavior.Source, AssessmentDispatchID: "ordinary-assessment", AcceptanceEvidenceRefs: []string{"ordinary"}}}
		row.State, row.CWIConvergenceState, row.LastAssessmentID, row.EvidenceRefs = runtimecontract.RuntimeCoverageItemStateSatisfied, runtimecontract.RuntimeCWIConvergenceStateSettled, "ordinary-assessment", []string{"effect", "ordinary"}
	}
	before := aggregate.CloneRaw()
	revokeHistoricalAcceptanceCredit(&aggregate.Facts)
	row := runtimeCoverageByID(aggregate.Facts.Coverage)["history"]
	if row.State != runtimecontract.RuntimeCoverageItemStateOpen || row.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || row.LastAssessmentID != "" || len(row.EvidenceRefs) != 0 ||
		!reflect.DeepEqual(row.HistoricalBehavior, runtimeCoverageByID(before.Facts.Coverage)["history"].HistoricalBehavior) || !reflect.DeepEqual(row.HistoricalAcceptanceProofs, runtimeCoverageByID(before.Facts.Coverage)["history"].HistoricalAcceptanceProofs) ||
		!reflect.DeepEqual(aggregate.Facts.CompletedFocuses, before.Facts.CompletedFocuses) || !reflect.DeepEqual(aggregate.Facts.Focus, before.Facts.Focus) || !reflect.DeepEqual(aggregate.Facts.Evidence, before.Facts.Evidence) {
		t.Fatal("ordinary stale revocation lost history or requeued original effect")
	}
	if got := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts); !reflect.DeepEqual(got, []common.CoverageWorkItemID{"acceptance"}) {
		t.Fatalf("revoked history reentered planning instead of exact acceptance source: %v", got)
	}
}
