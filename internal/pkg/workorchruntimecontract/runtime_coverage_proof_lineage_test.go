package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCoverageEvidenceProofIdentityIgnoresBindingAndPrerequisiteAuditIdentity(t *testing.T) {
	oldBinding := testCoverageEvidenceBinding()
	currentBinding := oldBinding.Clone()
	currentBinding.ID = "binding/cwi-main/resealed"
	currentBinding.PrerequisiteRefs = []string{"catalog/file/revision-2"}
	currentBinding.Hash = StableCoverageEvidenceBindingHash(currentBinding)

	oldIdentity, err := DeriveCoverageEvidenceProofIdentity(oldBinding, EvidenceKindStateChange)
	if err != nil {
		t.Fatalf("derive source proof identity: %v", err)
	}
	currentIdentity, err := DeriveCoverageEvidenceProofIdentity(currentBinding, EvidenceKindStateChange)
	if err != nil {
		t.Fatalf("derive re-sealed proof identity: %v", err)
	}
	if oldBinding.ID == currentBinding.ID || oldBinding.Hash == currentBinding.Hash {
		t.Fatalf("fixture did not change binding audit identity: old=%+v current=%+v", oldBinding, currentBinding)
	}
	if !SameCoverageEvidenceProofIdentity(oldIdentity, currentIdentity) || oldIdentity.Hash != currentIdentity.Hash {
		t.Fatalf("binding/prerequisite rollover changed proof identity: old=%+v current=%+v", oldIdentity, currentIdentity)
	}
}

func TestCoverageEvidenceProofIdentityDetectsEverySemanticProofChange(t *testing.T) {
	stateBinding := testCoverageEvidenceBinding()
	validationBinding := testCoverageEvidenceBinding()
	tests := []struct {
		name     string
		evidence string
		mutate   func(*CoverageEvidenceBinding)
		expected string
	}{
		{
			name: "definition revision", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) { binding.DefinitionRevision = "definition-next" }, expected: "definition_revision",
		},
		{
			name: "coverage work item", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) { binding.CoverageWorkItemID = "cwi-next" }, expected: "coverage_work_item",
		},
		{
			name: "owner", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) {
				binding.Proofs[0].AllowedOperationKinds = []string{StepKindGitWrite}
			}, expected: "owner_module",
		},
		{
			name: "target", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) { binding.TargetRefs = []string{"other.go"} }, expected: "target",
		},
		{
			name: "effect", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) {
				binding.Proofs[0].RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectRemoteDurable}
			}, expected: "effect",
		},
		{
			name: "declaration", evidence: EvidenceKindStateChange,
			mutate: func(binding *CoverageEvidenceBinding) { binding.DeclarationRef = "test/profile/other" }, expected: "declaration",
		},
		{
			name: "validation recipe", evidence: EvidenceKindValidation,
			mutate: func(binding *CoverageEvidenceBinding) { binding.Proofs[1].ValidationRecipeID = "recipe/other" }, expected: "validation_recipe",
		},
		{
			name: "required checks", evidence: EvidenceKindValidation,
			mutate: func(binding *CoverageEvidenceBinding) {
				binding.Proofs[1].RequiredCheckRefs = []string{"unit/other"}
				binding.Proofs[1].ValidationCheckAttestations = runtimeValidationCheckAttestationsForTest(binding.Proofs[1].RequiredCheckRefs)
			}, expected: "required_checks",
		},
		{
			name: "metrics", evidence: EvidenceKindValidation,
			mutate: func(binding *CoverageEvidenceBinding) { binding.Proofs[1].MetricConstraints = []string{"metric/other"} }, expected: "metrics",
		},
		{
			name: "artifacts", evidence: EvidenceKindValidation,
			mutate: func(binding *CoverageEvidenceBinding) { binding.Proofs[1].ArtifactRefs = []string{"artifact/other"} }, expected: "artifacts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := stateBinding
			if tt.evidence == EvidenceKindValidation {
				base = validationBinding
			}
			baseIdentity, err := DeriveCoverageEvidenceProofIdentity(base, tt.evidence)
			if err != nil {
				t.Fatalf("derive base identity: %v", err)
			}
			changed := base.Clone()
			tt.mutate(&changed)
			changed = changed.Normalized()
			changed.Hash = StableCoverageEvidenceBindingHash(changed)
			changedIdentity, err := DeriveCoverageEvidenceProofIdentity(changed, tt.evidence)
			if err != nil {
				t.Fatalf("derive changed identity: %v", err)
			}
			if SameCoverageEvidenceProofIdentity(baseIdentity, changedIdentity) {
				t.Fatalf("semantic change was treated as equivalent: base=%+v changed=%+v", baseIdentity, changedIdentity)
			}
			if got := FirstCoverageEvidenceProofIdentityDifference(baseIdentity, changedIdentity); got != tt.expected {
				t.Fatalf("first difference = %q, want %q; base=%+v changed=%+v", got, tt.expected, baseIdentity, changedIdentity)
			}
		})
	}
}

func TestRuntimeCoverageProofLineageValidatesSourceResolutionAndBounds(t *testing.T) {
	source := testRuntimeCoverageProofLineageSource(t)
	currentBinding := testCoverageEvidenceBinding().Clone()
	currentBinding.ID = "binding/cwi-main/current"
	currentBinding.Hash = StableCoverageEvidenceBindingHash(currentBinding)
	currentIdentity, err := DeriveCoverageEvidenceProofIdentity(currentBinding, EvidenceKindStateChange)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := RuntimeCoverageProofLineage{
		Kind:                 RuntimeCoverageProofLineageKindResolution,
		WorkspaceID:          source.WorkspaceID,
		RunID:                source.RunID,
		CoverageWorkItemID:   source.CoverageWorkItemID,
		EvidenceKind:         source.EvidenceKind,
		SourceLineageID:      source.ID,
		CurrentBindingRef:    currentBinding.ID,
		CurrentBindingHash:   currentBinding.Hash,
		CurrentProofIdentity: currentIdentity,
		Disposition:          RuntimeCoverageProofLineageDispositionEquivalent,
	}
	equivalent.ID = StableRuntimeCoverageProofLineageResolutionID(equivalent.SourceLineageID, equivalent.CurrentBindingRef, equivalent.CurrentBindingHash)
	if err := ValidateRuntimeCoverageProofLineage([]RuntimeCoverageProofLineage{source, equivalent}); err != nil {
		t.Fatalf("valid source/equivalent resolution rejected: %v", err)
	}

	clone := source.Clone()
	clone.SourceProofIdentity.TargetRefs[0] = "mutated.go"
	if source.SourceProofIdentity.TargetRefs[0] == "mutated.go" {
		t.Fatal("lineage clone retained nested identity storage")
	}
	if err := ValidateRuntimeCoverageProofLineage([]RuntimeCoverageProofLineage{source, source.Clone()}); err == nil {
		t.Fatal("duplicate source row accepted")
	}
	missingSource := equivalent.Clone()
	missingSource.SourceLineageID = "missing-source"
	missingSource.ID = StableRuntimeCoverageProofLineageResolutionID(missingSource.SourceLineageID, missingSource.CurrentBindingRef, missingSource.CurrentBindingHash)
	if err := ValidateRuntimeCoverageProofLineage([]RuntimeCoverageProofLineage{source, missingSource}); err == nil {
		t.Fatal("resolution without source row accepted")
	}
	if err := ValidateRuntimeCoverageProofLineage(make([]RuntimeCoverageProofLineage, RuntimeCoverageProofLineageLimit+1)); err == nil {
		t.Fatal("lineage overflow accepted")
	}
}

func TestRuntimeCoverageProofLineageOpaqueReloadRejectsAliasedProofChecks(t *testing.T) {
	binding := testCoverageEvidenceBinding()
	identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindValidation)
	if err != nil {
		t.Fatal(err)
	}
	row := RuntimeCoverageProofLineage{
		Kind: RuntimeCoverageProofLineageKindSource, WorkspaceID: "workspace-lineage", RunID: "run-lineage",
		CoverageWorkItemID: binding.CoverageWorkItemID, EvidenceKind: EvidenceKindValidation,
		SourceEvidenceID: "evidence-validation-lineage", SourceDispatchID: "dispatch-lineage",
		SourceOperationID: "operation-lineage", SourceExecutionKey: "execution-lineage", SourceFence: 1,
		SourceReceiptHash: "receipt-lineage", SourceBindingRef: binding.ID, SourceBindingHash: binding.Hash,
		SourceManifestHash: "manifest-lineage", SourceLogicalRequestID: "request-lineage", SourceProofIdentity: identity,
	}
	row.ID = StableRuntimeCoverageProofLineageSourceID(row.SourceReceiptHash, row.SourceEvidenceID, row.SourceBindingHash, row.SourceProofIdentity.Hash)
	if err := row.Validate(); err != nil {
		t.Fatalf("valid validation lineage fixture: %v", err)
	}
	checkRef := row.SourceProofIdentity.RequiredCheckRefs[0]
	row.SourceProofIdentity.RequiredCheckRefs = append(row.SourceProofIdentity.RequiredCheckRefs, " "+checkRef+" ")
	payload, err := json.Marshal([]RuntimeCoverageProofLineage{row})
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []RuntimeCoverageProofLineage
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeCoverageProofLineage(reloaded); err == nil || !strings.Contains(err.Error(), "duplicates check") {
		t.Fatalf("opaque lineage reload accepted aliased duplicate proof check: %v", err)
	}
}

func testRuntimeCoverageProofLineageSource(t *testing.T) RuntimeCoverageProofLineage {
	t.Helper()
	binding := testCoverageEvidenceBinding()
	identity, err := DeriveCoverageEvidenceProofIdentity(binding, EvidenceKindStateChange)
	if err != nil {
		t.Fatal(err)
	}
	row := RuntimeCoverageProofLineage{
		Kind:                   RuntimeCoverageProofLineageKindSource,
		WorkspaceID:            "workspace-lineage",
		RunID:                  "run-lineage",
		CoverageWorkItemID:     "cwi-main",
		EvidenceKind:           EvidenceKindStateChange,
		SourceEvidenceID:       common.EvidenceRecordID("evidence-lineage"),
		SourceDispatchID:       "dispatch-lineage",
		SourceOperationID:      "operation-lineage",
		SourceExecutionKey:     "execution-lineage",
		SourceFence:            1,
		SourceReceiptHash:      "receipt-lineage",
		SourceBindingRef:       binding.ID,
		SourceBindingHash:      binding.Hash,
		SourceManifestHash:     "manifest-lineage",
		SourceLogicalRequestID: "request-lineage",
		SourceProofIdentity:    identity,
	}
	row.ID = StableRuntimeCoverageProofLineageSourceID(row.SourceReceiptHash, row.SourceEvidenceID, row.SourceBindingHash, row.SourceProofIdentity.Hash)
	if err := row.Validate(); err != nil {
		t.Fatalf("build source lineage: %v", err)
	}
	return row
}
