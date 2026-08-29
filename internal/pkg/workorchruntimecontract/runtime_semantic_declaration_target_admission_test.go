package workorchruntimecontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticDefinitionSelectorCandidatesConstrainEveryTargetPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   RuntimeOperationTargetPolicy
		wantPath string
		mutate   func(*RuntimeSemanticProofProposal)
	}{
		{
			name: "exact target outside declaration candidates", policy: RuntimeOperationTargetPolicyExactTarget,
			wantPath: "$.proposals[0].target.exactTargetRefs",
			mutate: func(proposal *RuntimeSemanticProofProposal) {
				proposal.Target.ExactTargetRefs = []string{"outside.go"}
			},
		},
		{
			name: "existing scope outside declaration candidates", policy: RuntimeOperationTargetPolicyExistingOwnerScope,
			wantPath: "$.proposals[0].target.ownerScopeTargetRefs",
			mutate: func(proposal *RuntimeSemanticProofProposal) {
				proposal.Target.OwnerScopeTargetRefs = []string{"outside.go"}
			},
		},
		{
			name: "selector-only existing scope cannot bypass finite candidates", policy: RuntimeOperationTargetPolicyExistingOwnerScope,
			wantPath: "$.proposals[0].target.ownerScopeTargetRefs",
			mutate: func(proposal *RuntimeSemanticProofProposal) {
				proposal.Target.OwnerScopeTargetRefs = nil
				proposal.Target.OwnerSelectorRefs = []string{"file/workspace-relative-create"}
			},
		},
		{
			name: "prospective target outside declaration candidates", policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			wantPath: "$.proposals[0].target.prospectiveTargetRefs",
			mutate: func(proposal *RuntimeSemanticProofProposal) {
				proposal.Target.ProspectiveTargetRefs = []string{"outside-new.go"}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest, document, fact := semanticDeclarationTargetSurfaceFixture(t, tc.policy)
			if err := document.ValidateAgainstManifest(manifest); err != nil {
				t.Fatalf("valid declaration target surface: %v", err)
			}
			if _, _, err := CompileRuntimeSemanticBindings(manifest, "semantic-declaration-target-request", document, []RuntimeSemanticProofFeasibility{fact}); err != nil {
				t.Fatalf("valid declaration target binding: %v", err)
			}

			outside := document.Clone()
			tc.mutate(&outside.Proposals[0])
			var violation *RuntimeSemanticProposalViolationError
			if err := outside.ValidateAgainstManifest(manifest); !errors.As(err, &violation) || violation.Violation().Path != tc.wantPath {
				t.Fatalf("document admission violation=%+v err=%v, want %s", violation, err, tc.wantPath)
			}
			violation = nil
			if _, _, err := CompileRuntimeSemanticBindings(manifest, "semantic-declaration-target-request", outside, nil); !errors.As(err, &violation) || violation.Violation().Path != tc.wantPath {
				t.Fatalf("binding admission violation=%+v err=%v, want %s", violation, err, tc.wantPath)
			}
		})
	}
}

func semanticDeclarationTargetSurfaceFixture(t *testing.T, policy RuntimeOperationTargetPolicy) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, RuntimeSemanticProofFeasibility) {
	t.Helper()
	manifest := semanticContractTestManifest(t)
	manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
		RuntimeOperationTargetPolicyProspectiveOwnerTarget,
	}
	catalog := &manifest.OwnerCatalogs[0]
	proposal := semanticContractTestProposal("cwi-001")
	allowedTarget := "allowed.go"
	switch policy {
	case RuntimeOperationTargetPolicyExactTarget:
		catalog.TargetRefs = []string{allowedTarget, "outside.go"}
		proposal.Target = RuntimeOperationTargetAuthorization{Policy: policy, ExactTargetRefs: []string{allowedTarget}}
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		catalog.TargetRefs = []string{allowedTarget, "outside.go"}
		proposal.Target = RuntimeOperationTargetAuthorization{Policy: policy, OwnerScopeTargetRefs: []string{allowedTarget}}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		allowedTarget = "allowed-new.go"
		catalog.TargetRefs = []string{"existing.go"}
		proposal.Target = RuntimeOperationTargetAuthorization{
			Policy: policy, OwnerSelectorRefs: []string{"file/workspace-relative-create"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRefs: []string{allowedTarget},
		}
	default:
		t.Fatalf("unsupported fixture target policy %q", policy)
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	source := CoverageFocus{
		ID: "definition-target-surface", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, TargetRole: "deliverable",
		TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef:      "profile/declaration-selector", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
		TargetSelectorCandidateRefs: []string{allowedTarget}, RequireTargetGrounding: true,
	}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-declaration-target", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	fact := semanticFeasibilityForTest(t, manifest, proposal, RuntimeSemanticProposalTargetRefs(proposal), "", nil)
	return manifest, document, fact
}
