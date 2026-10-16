package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func terminalSourceObligationFixture(t *testing.T) RuntimeStateRecord {
	t.Helper()
	source := terminalResultCompilerSource()
	inventory := sourceObligationInventoryFixture()
	source.Definition.Revision, source.Definition.StartupInputHash = inventory.DefinitionRevision, inventory.StartupInputHash
	source.Definition.SourceObligations = &inventory
	source.Facts.DefinitionRevision = inventory.DefinitionRevision
	item := RuntimeCoverageItem{ID: "cwi-shared", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	item.SourceObligationChecks = checks
	source.Facts.DefinitionSeed = RuntimeDefinitionSeed{DefinitionRevision: inventory.DefinitionRevision, CoverageItems: []RuntimeCoverageItem{item}, SourceObligations: &inventory}
	requirement := runtimeValidationProofRequirementForTest(t, inventory.DefinitionRevision, item, "", "recipe", nil)
	attestations := runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)
	proof := CoverageEvidenceProof{EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession}, AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe",
		ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: attestations}
	binding := CoverageEvidenceBinding{ID: "binding", CoverageWorkItemID: item.ID, FocusID: "focus", DefinitionRevision: inventory.DefinitionRevision, DeclarationRef: requirement.DeclarationRef, TargetRefs: []string{"target"}, Proofs: []CoverageEvidenceProof{proof}}
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	source.Facts.Focus = CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}}
	source.Facts.RemainingFocuses, source.Facts.CompletedFocuses = nil, nil
	source.Facts.Assessments = nil
	source.Facts.Coverage = []RuntimeCoverageItemFact{{ID: item.ID, SliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID, TargetText: item.TargetText, State: RuntimeCoverageItemStateSatisfied,
		CWIConvergenceState: RuntimeCWIConvergenceStateSettled, EvidenceRefs: []string{"evidence"}, CurrentBindingRef: &RuntimeCWIBindingRef{BindingID: binding.ID, BindingHash: binding.Hash, FocusID: binding.FocusID, SemanticManifestHash: "manifest"}}}
	observation := RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: attestations[0].ObservationProtocolRef}
	for _, attestation := range attestations {
		observation.ExecutedChecks = append(observation.ExecutedChecks, attestation.NativeCheckRef)
		observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
	}
	ownerDependencies := (RuntimeOwnerRevisionSet{DefinitionRevision: inventory.DefinitionRevision, Entries: []RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: "blocks/file/ws-1", ResourceKind: "workspace_target", ResourceID: "target", Revision: "bytes:1", DependencyHash: "selector:target",
	}}}).WithHash()
	source.Facts.Evidence.Accepted = []EvidenceFact{{ID: "evidence", Kind: EvidenceKindValidation, DispatchID: "source-dispatch", OperationID: "source-validation", CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, BindingRefs: []string{binding.ID}, AuthoritySnapshotID: "authority", Revision: "evidence:1", ContentHash: "content", OwnerRevisionSet: &ownerDependencies, Validation: &observation}}
	source.CompletedDispatches = []RuntimeDispatchRecord{{ID: "source-dispatch", WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID, CoverageBindings: []CoverageEvidenceBinding{binding.Clone()}}}
	source.Facts.Evidence.Rejected = nil
	source.Decision.State = ConvergenceStateComplete
	return source
}

func TestRuntimeTerminalSourceObligationsPreserveAllOfAndRestart(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	before := source.Clone()
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := material.RuntimeCloseout.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	outcome := material.RuntimeCloseout.RequirementOutcomes[0]
	if !reflect.DeepEqual(outcome.EvidenceItems[0].OwnerRevisionSet, source.Facts.Evidence.Accepted[0].OwnerRevisionSet) {
		t.Fatalf("terminal evidence dropped original owner dependencies: %+v", outcome.EvidenceItems[0])
	}
	if len(outcome.SourceObligations) != 2 || len(material.DoDAssessment.Items) != 3 {
		t.Fatal("terminal material lost shared CWI source obligations")
	}
	for _, obligation := range outcome.SourceObligations {
		if obligation.CoverageState != RuntimeRequirementCovered || obligation.AssessmentState != RuntimeRequirementSatisfied || !reflect.DeepEqual(obligation.AcceptedEvidenceRefs, []string{"evidence"}) || obligation.CurrentBindingRef == nil {
			t.Fatalf("complete all_of source proof missing: %+v", obligation)
		}
	}
	if material.DoDAssessment.Items[1].Item != source.Facts.DefinitionSeed.CoverageItems[0].SourceObligationChecks[0].Criterion {
		t.Fatal("source criterion bytes changed in public feedback")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	repeated, err := DeriveRuntimeTerminalResultMaterialFromState(reloaded)
	if err != nil || !reflect.DeepEqual(material, repeated) || !reflect.DeepEqual(source, before) {
		t.Fatalf("terminal source recovery changed exact material: %v", err)
	}
	clone := material.CloneRaw()
	clone.RuntimeCloseout.RequirementOutcomes[0].SourceObligations[0].AcceptedEvidenceRefs[0] = "changed"
	clone.RuntimeCloseout.RequirementOutcomes[0].SourceObligations[0].CurrentBindingRef.BindingHash = "changed"
	clone.RuntimeCloseout.SourceObligations.Obligations[0].DoDItemRefs[0] = "changed"
	clone.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSet.Entries[0].Revision = "changed"
	if !reflect.DeepEqual(material, repeated) {
		t.Fatal("raw terminal clone aliases source outcomes or inventory")
	}
}

func TestRuntimeTerminalEvidencePreservesDistinctReadSnapshots(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	source.Decision.State = ConvergenceStateFailed
	second := source.Facts.Evidence.Accepted[0]
	second.ID = "evidence-conflict"
	second.OperationID = "source-validation-conflict"
	second.OwnerRevisionSet = CloneRuntimeOwnerRevisionSet(second.OwnerRevisionSet)
	second.OwnerRevisionSet.Entries[0].Revision = "bytes:2"
	*second.OwnerRevisionSet = second.OwnerRevisionSet.WithHash()
	second.BindingRefs = []string{"historical-binding"}
	source.Facts.Evidence.Accepted = append(source.Facts.Evidence.Accepted, second)
	before := source.Clone()
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil || material.RuntimeCloseout.ValidateRaw() != nil {
		t.Fatalf("original read snapshots cannot be compiled: %v", err)
	}
	items := material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems
	if len(items) != 2 || items[1].State != RuntimeRequirementEvidenceStaleOrSuperseded ||
		!reflect.DeepEqual(items[0].OwnerRevisionSet, source.Facts.Evidence.Accepted[0].OwnerRevisionSet) ||
		!reflect.DeepEqual(items[1].OwnerRevisionSet, second.OwnerRevisionSet) {
		t.Fatal("terminal projection selected one read version or restored historical credit")
	}
	if !reflect.DeepEqual(source, before) {
		t.Fatal("terminal compiler rewrote read history")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeStateRecord
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	repeated, err := DeriveRuntimeTerminalResultMaterialFromState(reloaded)
	if err != nil || !reflect.DeepEqual(material, repeated) {
		t.Fatalf("restart changed original terminal evidence reads: %v", err)
	}
	changed := material.CloneRaw()
	changed.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSet.Entries[0].Revision = "tamper"
	if changed.RuntimeCloseout.ValidateRaw() == nil {
		t.Fatal("dependency tamper passed terminal hash/raw validation")
	}
}

func TestRuntimeTerminalSourceObligationsRequireUniqueEquivalentResolution(t *testing.T) {
	for _, name := range []string{"equivalent", "missing", "duplicate", "conflicting", "cross Run", "proof drift"} {
		t.Run(name, func(t *testing.T) {
			source := terminalSourceObligationFixture(t)
			binding := source.Facts.Focus.CoverageEvidenceBindings[0].Clone()
			original := terminalEvidenceLineageSource(t, source, binding)
			binding.PrerequisiteRefs = []string{"fresh-catalog"}
			binding.Hash = StableCoverageEvidenceBindingHash(binding)
			source.Facts.Focus.CoverageEvidenceBindings[0] = binding
			source.Facts.Coverage[0].CurrentBindingRef.BindingHash = binding.Hash
			identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindValidation)
			if err != nil {
				t.Fatal(err)
			}
			resolution := RuntimeCoverageProofLineage{Kind: RuntimeCoverageProofLineageKindResolution, WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID, CoverageWorkItemID: binding.CoverageWorkItemID, EvidenceKind: EvidenceKindValidation, SourceLineageID: original.ID, CurrentBindingRef: binding.ID, CurrentBindingHash: binding.Hash, CurrentProofIdentity: identity, Disposition: RuntimeCoverageProofLineageDispositionEquivalent}
			resolution.ID = StableRuntimeCoverageProofLineageResolutionID(original.ID, binding.ID, binding.Hash)
			source.CoverageProofLineage = []RuntimeCoverageProofLineage{original, resolution}
			switch name {
			case "missing":
				source.CoverageProofLineage = source.CoverageProofLineage[:1]
			case "duplicate", "conflicting":
				duplicate := resolution.Clone()
				if name == "conflicting" {
					duplicate.Disposition = RuntimeCoverageProofLineageDispositionIncompatible
				}
				source.CoverageProofLineage = append(source.CoverageProofLineage, duplicate)
			case "cross Run":
				source.CoverageProofLineage[1].RunID = "other-run"
			case "proof drift":
				source.CoverageProofLineage[1].CurrentProofIdentity.TargetRefs = []string{"other-target"}
				source.CoverageProofLineage[1].CurrentProofIdentity.Hash = StableCoverageEvidenceProofIdentityHash(source.CoverageProofLineage[1].CurrentProofIdentity)
			}
			before, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			outcomes := DeriveRuntimeTerminalRequirementOutcomesFromState(source, RuntimeOwnerRevisionSet{})
			if len(outcomes) != 1 || len(outcomes[0].SourceObligations) != 2 {
				t.Fatal("lineage 拒绝丢失完整逐源审计")
			}
			for _, obligation := range outcomes[0].SourceObligations {
				accepted := name == "equivalent"
				if (obligation.AssessmentState == RuntimeRequirementSatisfied) != accepted || (len(obligation.AcceptedEvidenceRefs) == 1) != accepted {
					t.Fatalf("逐源 credit 未匹配唯一 exact equivalent resolution: %+v", obligation)
				}
			}
			var reloaded RuntimeStateRecord
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(outcomes, DeriveRuntimeTerminalRequirementOutcomesFromState(reloaded, RuntimeOwnerRevisionSet{})) {
				t.Fatal("重载改变逐源 lineage 适用性")
			}
			after, err := json.Marshal(source)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("只读投影改写原始 source")
			}
		})
	}
}

func TestRuntimeTerminalSourceObligationsRejectMissingBAndStaleCredit(t *testing.T) {
	for _, name := range []string{"only A", "old binding", "reopened CWI"} {
		t.Run(name, func(t *testing.T) {
			source := terminalSourceObligationFixture(t)
			switch name {
			case "only A":
				b := source.Facts.DefinitionSeed.CoverageItems[0].SourceObligationChecks[1].CheckRef
				proof := source.Facts.Focus.CoverageEvidenceBindings[0].Proofs[0]
				for _, attestation := range proof.ValidationCheckAttestations {
					if attestation.RequiredCheckRef == b {
						var passed []string
						for _, native := range source.Facts.Evidence.Accepted[0].Validation.PassedChecks {
							if native != attestation.NativeCheckRef {
								passed = append(passed, native)
							}
						}
						source.Facts.Evidence.Accepted[0].Validation.PassedChecks = passed
					}
				}
			case "old binding":
				source.Facts.Evidence.Accepted[0].BindingRefs = []string{"old-binding"}
			case "reopened CWI":
				source.Facts.Coverage[0].State = RuntimeCoverageItemStateOpen
			}
			material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
			if err != nil {
				t.Fatal(err)
			}
			if material.RuntimeCloseout.Validate() == nil {
				t.Fatal("incomplete current source proof became valid completed closeout")
			}
			for _, obligation := range material.RuntimeCloseout.RequirementOutcomes[0].SourceObligations {
				if obligation.AssessmentState == RuntimeRequirementSatisfied || len(obligation.AcceptedEvidenceRefs) != 0 {
					t.Fatal("terminal projection restored stale or incomplete source credit")
				}
			}
		})
	}
	source := terminalSourceObligationFixture(t)
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"omit B", "alter B", "remove inventory", "wrong startup"} {
		t.Run(name, func(t *testing.T) {
			forged := material.RuntimeCloseout.CloneRaw()
			switch name {
			case "omit B":
				forged.RequirementOutcomes[0].SourceObligations = forged.RequirementOutcomes[0].SourceObligations[:1]
			case "alter B":
				forged.RequirementOutcomes[0].SourceObligations[1].Check.Criterion = "A"
			case "remove inventory":
				forged.SourceObligations = nil
			case "wrong startup":
				forged.StartupInputHash = "wrong"
			}
			forged = forged.WithCloseoutHash()
			if forged.ValidateRaw() == nil {
				t.Fatal("self-consistent closeout omitted exact source authority")
			}
		})
	}
}
