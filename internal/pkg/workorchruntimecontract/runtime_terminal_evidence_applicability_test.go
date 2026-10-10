package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func terminalEvidenceApplicabilityFixture(t *testing.T) RuntimeStateRecord {
	t.Helper()
	source := terminalResultCompilerSource()
	binding := testCoverageEvidenceBinding()
	binding.Proofs = binding.Proofs[1:]
	binding.Proofs[0].MetricConstraints, binding.Proofs[0].ArtifactRefs = nil, nil
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	source.Facts.DefinitionRevision = binding.DefinitionRevision
	source.Facts.Focus = CoverageFocus{ID: binding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}}
	source.Facts.RemainingFocuses, source.Facts.CompletedFocuses = nil, nil
	source.Facts.Assessments = nil
	source.Facts.Coverage = []RuntimeCoverageItemFact{{ID: binding.CoverageWorkItemID, State: RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"evidence"}, CurrentBindingRef: &RuntimeCWIBindingRef{BindingID: binding.ID, BindingHash: binding.Hash, FocusID: binding.FocusID, SemanticManifestHash: "manifest"}}}
	proof := binding.Proofs[0]
	observation := RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID, ObservationProtocolRef: proof.ValidationCheckAttestations[0].ObservationProtocolRef}
	for _, attestation := range proof.ValidationCheckAttestations {
		observation.ExecutedChecks = append(observation.ExecutedChecks, attestation.NativeCheckRef)
		observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
	}
	source.Facts.Evidence.Accepted = []EvidenceFact{{ID: "evidence", Kind: EvidenceKindValidation, DispatchID: "source-dispatch", OperationID: "source-validation", CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, BindingRefs: []string{binding.ID}, Validation: &observation}}
	source.Facts.Evidence.Rejected = nil
	source.CompletedDispatches = []RuntimeDispatchRecord{{ID: "source-dispatch", WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID, CoverageBindings: []CoverageEvidenceBinding{binding.Clone()}}}
	return source
}

func terminalEvidenceLineageSource(t *testing.T, source RuntimeStateRecord, binding CoverageEvidenceBinding) RuntimeCoverageProofLineage {
	t.Helper()
	identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindValidation)
	if err != nil {
		t.Fatal(err)
	}
	row := RuntimeCoverageProofLineage{
		Kind: RuntimeCoverageProofLineageKindSource, WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID,
		CoverageWorkItemID: binding.CoverageWorkItemID, EvidenceKind: EvidenceKindValidation,
		SourceEvidenceID: source.Facts.Evidence.Accepted[0].ID, SourceDispatchID: source.Facts.Evidence.Accepted[0].DispatchID, SourceOperationID: source.Facts.Evidence.Accepted[0].OperationID,
		SourceExecutionKey: "execution", SourceFence: 1, SourceReceiptHash: "receipt",
		SourceBindingRef: binding.ID, SourceBindingHash: binding.Hash, SourceManifestHash: "manifest",
		SourceLogicalRequestID: "request", SourceProofIdentity: identity,
	}
	row.ID = StableRuntimeCoverageProofLineageSourceID(row.SourceReceiptHash, row.SourceEvidenceID, row.SourceBindingHash, row.SourceProofIdentity.Hash)
	if err := row.Validate(); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestTerminalEvidenceProjectionRequiresExactScopedEquivalentLineage(t *testing.T) {
	for _, name := range []string{"same ID new hash", "equivalent", "incompatible disposition", "wrong resolution hash", "cross Run", "cross Workspace", "proof identity drift", "duplicate source", "duplicate resolution", "conflicting resolution"} {
		t.Run(name, func(t *testing.T) {
			source := terminalEvidenceApplicabilityFixture(t)
			originalBinding := source.Facts.Focus.CoverageEvidenceBindings[0].Clone()
			original := terminalEvidenceLineageSource(t, source, originalBinding)
			current := originalBinding.Clone()
			current.PrerequisiteRefs = []string{"fresh-catalog"}
			current.Hash = StableCoverageEvidenceBindingHash(current)
			if originalBinding.ID != current.ID || originalBinding.Hash == current.Hash {
				t.Fatal("未构造同 ID 新 hash")
			}
			source.Facts.Focus.CoverageEvidenceBindings[0] = current
			source.Facts.Coverage[0].CurrentBindingRef.BindingHash = current.Hash
			identity, err := DeriveCoverageEvidenceProofIdentity(current, EvidenceKindValidation)
			if err != nil {
				t.Fatal(err)
			}
			resolution := RuntimeCoverageProofLineage{
				Kind: RuntimeCoverageProofLineageKindResolution, WorkspaceID: source.Facts.WorkspaceID, RunID: source.Facts.RunID,
				CoverageWorkItemID: current.CoverageWorkItemID, EvidenceKind: EvidenceKindValidation,
				SourceLineageID: original.ID, CurrentBindingRef: current.ID, CurrentBindingHash: current.Hash,
				CurrentProofIdentity: identity, Disposition: RuntimeCoverageProofLineageDispositionEquivalent,
			}
			resolution.ID = StableRuntimeCoverageProofLineageResolutionID(resolution.SourceLineageID, resolution.CurrentBindingRef, resolution.CurrentBindingHash)
			source.CoverageProofLineage = []RuntimeCoverageProofLineage{original, resolution}
			switch name {
			case "same ID new hash":
				source.CoverageProofLineage = source.CoverageProofLineage[:1]
			case "incompatible disposition":
				source.CoverageProofLineage[1].Disposition = RuntimeCoverageProofLineageDispositionIncompatible
			case "wrong resolution hash":
				source.CoverageProofLineage[1].CurrentBindingHash = "wrong"
			case "cross Run":
				for index := range source.CoverageProofLineage {
					source.CoverageProofLineage[index].RunID = "other-run"
				}
			case "cross Workspace":
				for index := range source.CoverageProofLineage {
					source.CoverageProofLineage[index].WorkspaceID = "other-workspace"
				}
			case "proof identity drift":
				source.CoverageProofLineage[1].CurrentProofIdentity.TargetRefs = []string{"other-target"}
				source.CoverageProofLineage[1].CurrentProofIdentity.Hash = StableCoverageEvidenceProofIdentityHash(source.CoverageProofLineage[1].CurrentProofIdentity)
			case "duplicate source":
				source.CoverageProofLineage = append(source.CoverageProofLineage, original.Clone())
			case "duplicate resolution", "conflicting resolution":
				duplicate := resolution.Clone()
				if name == "conflicting resolution" {
					duplicate.Disposition = RuntimeCoverageProofLineageDispositionIncompatible
				}
				source.CoverageProofLineage = append(source.CoverageProofLineage, duplicate)
			}
			if name == "equivalent" {
				if err := ValidateRuntimeCoverageProofLineage(source.CoverageProofLineage); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(source)
			outcomes := DeriveRuntimeTerminalRequirementOutcomesFromState(source, RuntimeOwnerRevisionSet{})
			accepted := name == "equivalent"
			if (len(outcomes[0].AcceptedEvidenceRefs) == 1) != accepted {
				t.Fatalf("current lineage eligibility 漂移: %+v", outcomes[0])
			}
			var reloaded RuntimeStateRecord
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(outcomes, DeriveRuntimeTerminalRequirementOutcomesFromState(reloaded, RuntimeOwnerRevisionSet{})) {
				t.Fatal("JSON 重载改变 current eligibility")
			}
			after, _ := json.Marshal(source)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("原始账本被改写")
			}
		})
	}
}

func TestTerminalEvidenceProjectionDoesNotRestoreHistoricalBindingCredit(t *testing.T) {
	for _, name := range []string{"old binding", "missing current binding", "current hash drift", "missing manifest", "duplicate current binding", "missing source dispatch", "same ID no lineage", "missing metric", "missing artifact", "weak evidence", "only A"} {
		t.Run(name, func(t *testing.T) {
			source := terminalEvidenceApplicabilityFixture(t)
			switch name {
			case "old binding":
				source.Facts.Evidence.Accepted[0].BindingRefs = []string{"historical-binding"}
			case "missing current binding":
				source.Facts.Coverage[0].CurrentBindingRef = nil
			case "current hash drift":
				source.Facts.Coverage[0].CurrentBindingRef.BindingHash = "changed"
			case "missing manifest":
				source.Facts.Coverage[0].CurrentBindingRef.SemanticManifestHash = ""
			case "duplicate current binding":
				source.Facts.CompletedFocuses = []CoverageFocus{source.Facts.Focus.Normalized()}
			case "missing source dispatch":
				source.CompletedDispatches = nil
			case "same ID no lineage":
				binding := source.Facts.Focus.CoverageEvidenceBindings[0].Clone()
				binding.PrerequisiteRefs = []string{"new-generation"}
				binding.Hash = StableCoverageEvidenceBindingHash(binding)
				source.Facts.Focus.CoverageEvidenceBindings[0] = binding
				source.Facts.Coverage[0].CurrentBindingRef.BindingHash = binding.Hash
			case "missing metric", "missing artifact":
				binding := source.Facts.Focus.CoverageEvidenceBindings[0].Clone()
				if name == "missing metric" {
					binding.Proofs[0].MetricConstraints = []string{"required-metric"}
				} else {
					binding.Proofs[0].ArtifactRefs = []string{"required-artifact"}
				}
				binding.Hash = StableCoverageEvidenceBindingHash(binding)
				source.Facts.Focus.CoverageEvidenceBindings[0] = binding
				source.Facts.Coverage[0].CurrentBindingRef.BindingHash = binding.Hash
				source.CompletedDispatches[0].CoverageBindings = []CoverageEvidenceBinding{binding.Clone()}
			case "weak evidence":
				source.Facts.Evidence.Accepted[0].ZeroWork = true
			case "only A":
				source.Facts.Evidence.Accepted[0].Validation.PassedChecks = nil
			}
			if name == "missing metric" || name == "missing artifact" {
				proof := source.Facts.Focus.CoverageEvidenceBindings[0].Proofs[0]
				if !RuntimeValidationObservationSatisfiesCoverageProof(proof, *source.Facts.Evidence.Accepted[0].Validation) {
					t.Fatal("metric/artifact 反例必须先满足独立 validation all-of")
				}
			}
			before, _ := json.Marshal(source)
			outcomes := DeriveRuntimeTerminalRequirementOutcomesFromState(source, RuntimeOwnerRevisionSet{})
			if len(outcomes) != 1 || len(outcomes[0].EvidenceItems) != 1 {
				t.Fatal("历史证据审计被丢弃")
			}
			item := outcomes[0].EvidenceItems[0]
			if item.State != RuntimeRequirementEvidenceStaleOrSuperseded || item.OwnerRevisionSetHash != "" || len(outcomes[0].AcceptedEvidenceRefs) != 0 {
				t.Fatalf("历史 accepted 被列为当前证明: %+v", outcomes[0])
			}
			after, _ := json.Marshal(source)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("投影改写原始账本")
			}
			if name == "missing metric" || name == "missing artifact" {
				proof := source.Facts.Focus.CoverageEvidenceBindings[0].Proofs[0]
				source.Facts.Evidence.Accepted[0].EvidenceTags = append([]string(nil), proof.MetricConstraints...)
				source.Facts.Evidence.Accepted[0].EvidenceRefs = append([]string(nil), proof.ArtifactRefs...)
				current := DeriveRuntimeTerminalRequirementOutcomesFromState(source, RuntimeOwnerRevisionSet{})
				if len(current[0].AcceptedEvidenceRefs) != 1 {
					t.Fatal("补齐 exact metric/artifact 后仍未恢复当前适用性")
				}
			}
		})
	}
}
