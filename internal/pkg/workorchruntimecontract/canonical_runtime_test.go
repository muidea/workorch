package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeTargetSelectionCreateRequiresAndHashesProspectiveTarget(t *testing.T) {
	selection := RuntimeTargetSelection{
		PrerequisiteID: "selection-1", OwnerModuleID: RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: RuntimeTargetSelectionCreate, CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err == nil {
		t.Fatal("create selection without prospective target was accepted")
	}
	selection.ProspectiveTargetRef = "initial-deliverable.txt"
	selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		t.Fatalf("valid create selection rejected: %v", err)
	}
	changed := selection
	changed.ProspectiveTargetRef = "other-deliverable.txt"
	if selection.SnapshotID == StableRuntimeTargetSelectionSnapshotID(changed) {
		t.Fatal("selection snapshot did not change with prospective target")
	}
}

func TestRuntimeTargetSelectionEmptyWorkspaceScopeRequiresProfileBinding(t *testing.T) {
	selection := RuntimeTargetSelection{
		PrerequisiteID: "selection-1", OwnerModuleID: RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: RuntimeTargetSelectionScope, CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err == nil {
		t.Fatal("unbound empty workspace scope was accepted")
	}
	selection.SelectorOwnerRef = "software-dev"
	selection.SelectorSubjectRef = "workspace_deliverable_create_scope"
	selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		t.Fatalf("profile-bound empty workspace scope rejected: %v", err)
	}
}

func TestRuntimeOperationAdmissibilityKeepsVariantlessKindIndependent(t *testing.T) {
	inventory := RuntimeOperationAdmissibility{
		OwnerModuleID:  RuntimeModuleShell,
		OperationKinds: []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession},
		OperationVariants: []RuntimeOperationAdmissibilityVariant{{
			Kind: StepKindShellCommand,
			AllowedTargetPolicies: []RuntimeOperationTargetPolicy{
				RuntimeOperationTargetPolicyExactTarget,
				RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		TargetRefs:   []string{"runtime-validation-source.txt"},
		SelectorRefs: []string{"workspace-validation-target"},
	}

	allowed, selectors, _, factFound, err := RuntimeOperationAdmissibilityAllowsTargetPolicy(
		inventory,
		RuntimeBlockKindManagedValidationSession,
		"",
		RuntimeOperationTargetPolicyExactTarget,
	)
	if err != nil || !allowed || factFound || len(selectors) != 1 || selectors[0] != "workspace-validation-target" {
		t.Fatalf("managed-validation target policy = allowed:%t selectors:%v factFound:%t err:%v", allowed, selectors, factFound, err)
	}
}

func TestRuntimeOperationAdmissibilityStaticVariantStillRequiresOwnerFact(t *testing.T) {
	inventory := RuntimeOperationAdmissibility{
		OwnerModuleID:  RuntimeModuleFile,
		OperationKinds: []string{StepKindFileWrite, StepKindFileRead},
		OperationVariants: []RuntimeOperationAdmissibilityVariant{{
			Kind: StepKindFileRead,
			AllowedTargetPolicies: []RuntimeOperationTargetPolicy{
				RuntimeOperationTargetPolicyExactTarget,
			},
		}},
		TargetRefs: []string{"main.go"},
	}

	allowed, _, _, factFound, err := RuntimeOperationAdmissibilityAllowsTargetPolicy(
		inventory,
		StepKindFileWrite,
		"file_write",
		RuntimeOperationTargetPolicyExactTarget,
	)
	if err != nil || allowed || factFound {
		t.Fatalf("missing static variant fact = allowed:%t factFound:%t err:%v", allowed, factFound, err)
	}
}

func TestRuntimeDecisionStateMatrix(t *testing.T) {
	tests := []struct {
		name          string
		state         ConvergenceState
		output        RequiredOutputContract
		prerequisites []RuntimePrerequisite
		terminal      *RuntimeTerminalDecision
	}{
		{name: "prerequisite", state: ConvergenceStateNeedPrerequisite, output: RequiredOutputContract{PrerequisiteIDs: []string{"snapshot"}}, prerequisites: []RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}},
		{name: "state change", state: ConvergenceStateNeedStateChange, output: RequiredOutputContract{AcceptedKinds: []string{"patch/apply"}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
		{name: "validation", state: ConvergenceStateNeedValidation, output: RequiredOutputContract{AcceptedKinds: []string{"validation/managed_session"}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
		{name: "governance", state: ConvergenceStateNeedGovernance, output: RequiredOutputContract{AcceptedKinds: []string{"decision/resolution"}}},
		{name: "complete", state: ConvergenceStateComplete, terminal: &RuntimeTerminalDecision{Code: "satisfied"}},
		{name: "fail", state: ConvergenceStateFailed, terminal: &RuntimeTerminalDecision{Code: "unrecoverable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := SealRuntimeDecision(testRuntimeFacts(), RuntimeDecision{
				State:          tt.state,
				Focus:          testRuntimeFacts().Focus,
				RequiredOutput: tt.output,
				Prerequisites:  tt.prerequisites,
				Terminal:       tt.terminal,
			}, 0)
			if err != nil {
				t.Fatalf("SealRuntimeDecision() error = %v", err)
			}
			if err := decision.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			transaction := decision.AuthorityTransaction()
			if transaction.AuthoritySnapshotID == "" || transaction.RequiredOutputHash == "" {
				t.Fatalf("transaction is not sealed: %+v", transaction)
			}
		})
	}
}

func TestAuthorityIdentityTracksCanonicalDecisionAndExcludesDerivedFocusText(t *testing.T) {
	base := validDecisionForState(ConvergenceStateNeedStateChange)
	baseSnapshotID := base.AuthoritySnapshot.ID
	baseContractID := StableAuthorityContractID(base.AuthoritySnapshot.WorkspaceID, base.AuthoritySnapshot.RunID, base.AuthorityTransaction())
	if baseSnapshotID == "" || baseContractID == "" {
		t.Fatalf("base authority identity is incomplete: snapshot=%q contract=%q", baseSnapshotID, baseContractID)
	}

	// AssessmentText is request/projection wording. It is not a second authority
	// field, so changing it cannot alter the sealed state/output identity.
	presentationOnly := base
	presentationOnly.Focus.AssessmentText = "provider-facing wording changed"
	if err := presentationOnly.Validate(); err != nil {
		t.Fatalf("presentation-only decision must remain valid: %v", err)
	}
	if presentationOnly.AuthoritySnapshot.ID != baseSnapshotID ||
		StableAuthorityContractID(presentationOnly.AuthoritySnapshot.WorkspaceID, presentationOnly.AuthoritySnapshot.RunID, presentationOnly.AuthorityTransaction()) != baseContractID {
		t.Fatal("derived focus wording changed authority identity")
	}

	changedOutput := base.RequiredOutput
	changedOutput.TargetRefs = []string{"different-target"}
	outputChanged, err := SealRuntimeDecision(testRuntimeFacts(), RuntimeDecision{
		State: base.State, Focus: base.Focus, RequiredOutput: changedOutput,
	}, 0)
	if err != nil {
		t.Fatalf("seal changed output decision: %v", err)
	}
	if outputChanged.AuthoritySnapshot.ID == baseSnapshotID ||
		StableAuthorityContractID(outputChanged.AuthoritySnapshot.WorkspaceID, outputChanged.AuthoritySnapshot.RunID, outputChanged.AuthorityTransaction()) == baseContractID {
		t.Fatal("semantic required-output change did not change authority identity")
	}

	stateChanged := validDecisionForState(ConvergenceStateNeedValidation)
	if stateChanged.AuthoritySnapshot.ID == baseSnapshotID ||
		StableAuthorityContractID(stateChanged.AuthoritySnapshot.WorkspaceID, stateChanged.AuthoritySnapshot.RunID, stateChanged.AuthorityTransaction()) == baseContractID {
		t.Fatal("canonical convergence-state change did not change authority identity")
	}
}

func TestRuntimeDecisionRejectsOutputAuthorityMismatch(t *testing.T) {
	d := validDecisionForState(ConvergenceStateNeedValidation)
	d.RequiredOutput = RequiredOutputContract{
		AcceptedKinds:        []string{"patch/apply"},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	d.AuthoritySnapshot.RequiredOutputHash = StableRequiredOutputHash(d.RequiredOutput)
	d.AuthoritySnapshot.ID = stableAuthoritySnapshotID(d.AuthoritySnapshot)
	if err := d.Validate(); err == nil {
		t.Fatal("validation state accepted state-changing required output")
	}
}

func TestStableRequiredOutputHashNormalizesOrderAndDetectsSemanticChange(t *testing.T) {
	left := RequiredOutputContract{
		AcceptedKinds:             []string{"git/write", "patch/apply", "patch/apply"},
		AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}, {Kind: "git/write", VariantID: "add"}, {Kind: "git/write", VariantID: "commit"}},
		AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectRemoteDurable, RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEvidenceKinds:     []string{"validation", "state_change"},
		CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-2", "cwi-1"},
		TargetRefs:                []string{"b.go", "a.go"},
	}
	right := RequiredOutputContract{
		AcceptedKinds:             []string{"patch/apply", "git/write"},
		AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "add"}, {Kind: "git/write", VariantID: "commit"}},
		AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectRemoteDurable, RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds:     []string{"state_change", "validation"},
		CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
		TargetRefs:                []string{"a.go", "b.go"},
	}
	if StableRequiredOutputHash(left) != StableRequiredOutputHash(right) {
		t.Fatal("hash changed with set ordering")
	}
	right.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "push"}}
	if StableRequiredOutputHash(left) == StableRequiredOutputHash(right) {
		t.Fatal("hash did not change with accepted operation variant")
	}
	right.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "add"}, {Kind: "git/write", VariantID: "commit"}}
	right.TargetRefs = []string{"a.go", "c.go"}
	if StableRequiredOutputHash(left) == StableRequiredOutputHash(right) {
		t.Fatal("hash did not change with semantic target change")
	}
}

func TestSealedRuntimeDecisionRejectsSnapshotMutation(t *testing.T) {
	d := validDecisionForState(ConvergenceStateNeedStateChange)
	d.AuthoritySnapshot.EvidenceRevision = "evidence-2"
	if err := d.Validate(); err == nil {
		t.Fatal("decision accepted mutated authority snapshot")
	}
}

func validDecisionForState(state ConvergenceState) RuntimeDecision {
	terminal, known := convergenceStateContract(state)
	if !known {
		panic("unknown convergence state")
	}
	output := RequiredOutputContract{}
	var prerequisites []RuntimePrerequisite
	switch state {
	case ConvergenceStateNeedPrerequisite:
		output.PrerequisiteIDs = []string{"snapshot"}
		prerequisites = []RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}
	case ConvergenceStateNeedStateChange:
		output.AcceptedKinds = []string{"patch/apply"}
		output.AcceptedEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}
		output.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}
		output.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	case ConvergenceStateNeedValidation:
		output.AcceptedKinds = []string{"validation/managed_session"}
		output.AcceptedEffectScopes = []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession}
		output.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectNone}
		output.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	case ConvergenceStateNeedGovernance:
		output.AcceptedKinds = []string{"decision/resolution"}
	case ConvergenceStateNeedAssessment:
		output.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	}
	var outcome *RuntimeTerminalDecision
	if terminal {
		outcome = &RuntimeTerminalDecision{Code: "terminal"}
	}
	d, err := SealRuntimeDecision(testRuntimeFacts(), RuntimeDecision{
		State:          state,
		Focus:          testRuntimeFacts().Focus,
		RequiredOutput: output,
		Prerequisites:  prerequisites,
		Terminal:       outcome,
	}, 0)
	if err != nil {
		panic(err)
	}
	return d
}

func testRuntimeFacts() RuntimeFacts {
	return RuntimeFacts{
		Activation:         RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID:        common.WorkspaceID("ws-1"),
		RunID:              common.RunID("run-1"),
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: CoverageFocus{
			ID:                  "focus-1",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		},
		Coverage: []RuntimeCoverageItemFact{{ID: "cwi-1", State: RuntimeCoverageItemStateOpen}},
	}
}
