package orchestrator

// Owner Biz prerequisite tests.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	activationcore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLocalPrerequisiteAdaptersCommitOwnerFacts(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:         runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus:              runtimecontract.CoverageFocus{ID: "cwi-1"},
		Failure: &runtimecontract.FailureFact{
			Signature:  "build-failure",
			TargetRefs: []string{"main.go"},
		},
	}
	internalDirectory := t.TempDir()
	requirements := []runtimecontract.RuntimeRequirement{
		{Kind: runtimecontract.PrerequisiteKindFailureTarget},
		{Kind: runtimecontract.PrerequisiteKindValidationScratch},
	}
	facts = runtime.PreflightRuntimeFacts(facts, requirements)
	for _, item := range append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...) {
		var err error
		switch item.Kind {
		case runtimecontract.PrerequisiteKindFailureTarget:
			facts, err = ResolveFailureTargetPrerequisite(facts, item.ID)
		case runtimecontract.PrerequisiteKindValidationScratch:
			facts, err = AcquireValidationScratchPrerequisite(facts, internalDirectory, item.ID)
		}
		if err != nil {
			t.Fatalf("resolve %s: %v", item.Kind, err)
		}
	}
	if facts.Resources.FailureTarget == nil || len(facts.Resources.FailureTarget.TargetRefs) != 1 {
		t.Fatalf("failure target = %+v", facts.Resources.FailureTarget)
	}
	if facts.Resources.ValidationScratch == nil || !strings.Contains(facts.Resources.ValidationScratch.Reference, "validation-scratch") {
		t.Fatalf("validation scratch = %+v", facts.Resources.ValidationScratch)
	}
	if !strings.HasPrefix(facts.Resources.ValidationScratch.Reference, filepath.Join(internalDirectory, "validation-scratch")) {
		t.Fatalf("validation scratch escaped workspace internal directory: %+v", facts.Resources.ValidationScratch)
	}
	if info, err := os.Stat(facts.Resources.ValidationScratch.Reference); err != nil || !info.IsDir() {
		t.Fatalf("validation scratch directory = %v, info=%+v", err, info)
	}
	var scratchPrerequisiteID string
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindValidationScratch {
			scratchPrerequisiteID = prerequisite.ID
			break
		}
	}
	observation, observed := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, scratchPrerequisiteID)
	if !observed || observation.Kind != runtimecontract.PrerequisiteKindValidationScratch || observation.OwnerModuleID != runtimecontract.RuntimeModuleOrchestrator ||
		len(observation.Dependencies) != 1 || observation.Dependencies[0].Domain != runtimecontract.RuntimeResourceLocalResource ||
		observation.Dependencies[0].SubjectRef != "validation-scratch/ws-1/run-1/global" || observation.Dependencies[0].Revision != facts.Resources.ValidationScratch.Revision {
		t.Fatalf("validation scratch observation = %+v", observation)
	}
	facts = runtime.PreflightRuntimeFacts(facts, requirements)
	if len(facts.Prerequisites.Pending) != 0 {
		t.Fatalf("resolved prerequisites scheduled again: %+v", facts.Prerequisites.Pending)
	}
}

func TestValidationScratchPrerequisiteScopeControlsReferenceAndFreshnessIdentity(t *testing.T) {
	base := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}
	for _, tc := range []struct {
		name, wantIdentity string
		requirement        runtimecontract.RuntimeRequirement
	}{
		{name: "global", wantIdentity: "validation-scratch/ws-1/run-1/global", requirement: runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindValidationScratch}},
		{name: "focus", wantIdentity: "validation-scratch/ws-1/run-1/focus/focus-1", requirement: runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindValidationScratch, Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := runtime.PreflightRuntimeFacts(base, []runtimecontract.RuntimeRequirement{tc.requirement})
			var err error
			facts, err = AcquireValidationScratchPrerequisite(facts, t.TempDir(), facts.Prerequisites.Pending[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			prerequisite := facts.Prerequisites.Satisfied[0]
			observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
			if !ok || len(observation.Dependencies) != 1 || observation.Dependencies[0].SubjectRef != tc.wantIdentity {
				t.Fatalf("scratch observation=%+v, want identity %q", observation, tc.wantIdentity)
			}
			if !strings.HasSuffix(filepath.ToSlash(facts.Resources.ValidationScratch.Reference), tc.wantIdentity) {
				t.Fatalf("scratch reference=%q, want suffix %q", facts.Resources.ValidationScratch.Reference, tc.wantIdentity)
			}
			if stale, inspectErr := InspectValidationScratchPrerequisite(facts); inspectErr != nil || stale != nil {
				t.Fatalf("fresh scratch inspection stale=%+v err=%v", stale, inspectErr)
			}
		})
	}
}

func TestAcquireValidationScratchPrerequisiteRejectsMissingWorkspaceInternalDirectory(t *testing.T) {
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	_, err := AcquireValidationScratchPrerequisite(facts, "", facts.Prerequisites.Pending[0].ID)
	if err == nil || !strings.Contains(err.Error(), "workspace internal directory") {
		t.Fatalf("missing workspace internal directory error = %v", err)
	}
}

func TestResolveFailureTargetPrerequisiteRejectsMissingTypedIdentity(t *testing.T) {
	for _, failure := range []*runtimecontract.FailureFact{
		nil,
		{TargetRefs: []string{"main.go"}},
		{Signature: "failure-without-target"},
	} {
		facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
			WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
			Failure: failure,
		}, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindFailureTarget}})
		if _, err := ResolveFailureTargetPrerequisite(facts, facts.Prerequisites.Pending[0].ID); err == nil {
			t.Fatalf("failure target mapping accepted incomplete failure: %+v", failure)
		}
	}
}

func TestInspectValidationScratchPrerequisiteReportsDeletedScopedResource(t *testing.T) {
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	var err error
	facts, err = AcquireValidationScratchPrerequisite(facts, t.TempDir(), facts.Prerequisites.Pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(facts.Resources.ValidationScratch.Reference); err != nil {
		t.Fatal(err)
	}
	stale, err := InspectValidationScratchPrerequisite(facts)
	if err != nil || stale == nil || stale.Domain != runtimecontract.RuntimeResourceLocalResource || stale.OwnerModuleID != runtimecontract.RuntimeModuleOrchestrator || stale.ExpectedRevision != facts.Resources.ValidationScratch.Revision || stale.ObservedRevision == stale.ExpectedRevision {
		t.Fatalf("validation scratch stale = %+v, err=%v", stale, err)
	}
}

func TestInspectValidationScratchPrerequisiteReportsRevisionMismatch(t *testing.T) {
	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1"},
	}, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	var err error
	facts, err = AcquireValidationScratchPrerequisite(facts, t.TempDir(), facts.Prerequisites.Pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(facts.Resources.ValidationScratch.Reference, validationScratchRevisionFile), []byte("scratch-replaced\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, err := InspectValidationScratchPrerequisite(facts)
	if err != nil || stale == nil || !strings.Contains(stale.Reason, "revision-mismatch") || stale.ExpectedRevision != facts.Resources.ValidationScratch.Revision || stale.ObservedRevision == stale.ExpectedRevision {
		t.Fatalf("validation scratch revision stale = %+v, err=%v", stale, err)
	}
}

func TestRuntimePrerequisiteResolverClassificationCoversContractKinds(t *testing.T) {
	external := []runtimecontract.PrerequisiteKind{
		runtimecontract.PrerequisiteKindManifest, runtimecontract.PrerequisiteKindTargetSnapshot,
		runtimecontract.PrerequisiteKindTargetSelection, runtimecontract.PrerequisiteKindProviderCeiling,
		runtimecontract.PrerequisiteKindCapability, runtimecontract.PrerequisiteKindOperationAdmissibility,
		runtimecontract.PrerequisiteKindCapabilityState,
	}
	for _, kind := range external {
		if got := runtimePrerequisiteResolverClassification(kind); got != runtimePrerequisiteResolverExternal {
			t.Fatalf("%s resolver class=%s, want external owner", kind, got)
		}
	}
	for _, kind := range []runtimecontract.PrerequisiteKind{runtimecontract.PrerequisiteKindFailureTarget, runtimecontract.PrerequisiteKindValidationScratch} {
		if got := runtimePrerequisiteResolverClassification(kind); got != runtimePrerequisiteResolverLocal {
			t.Fatalf("%s resolver class=%s, want local", kind, got)
		}
	}
	if got := runtimePrerequisiteResolverClassification("unknown"); got != runtimePrerequisiteResolverUnknown {
		t.Fatalf("unknown resolver class=%s, want unreachable", got)
	}
	if _, err := (&Biz{}).resolveRuntimePrerequisite(nil, runtime.RuntimeAggregate{}, "", "", runtimecontract.RuntimePrerequisite{ID: "unknown", Kind: "unknown"}); err == nil || !strings.Contains(err.Error(), "unsupported runtime prerequisite kind") {
		t.Fatalf("unknown resolver did not fail before any owner interaction: %v", err)
	}
}

// The resolver inventory must be derived from the same activation bootstrap
// and SM-2 reducer paths used in production. A handwritten kind list here
// would let a new profile/grounding prerequisite bypass the resolver audit.
func TestRuntimePrerequisiteResolverInventoryUsesProductionRequirementSources(t *testing.T) {
	policy := &runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
		CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll,
		ResultPolicy:     runtimecontract.RuntimeAcceptanceSliceResultAllowed,
		Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-default", Title: "Default", Scope: "default", Priority: 1, MaxWorkItems: 1,
			Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
		}},
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "test-provider", ModelID: "test-model"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "git-state-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: policy, StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "inventory-profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					// This fixture exercises production prerequisite derivation. Its
					// proof must be an exact CWI declaration; the wildcard below is
					// intentionally only the shared quality/grounding rule.
					ID: "dod-001", Scope: "dodItem", EvidenceRole: "primary_state_change",
					TargetRefs:            []string{"workspace_deliverable"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "git-state-validation", RequiredCheckRefs: []string{"check/git-state"},
					CapabilityStateRequirements: []runtimecontract.RuntimeProfileCapabilityStateRequirement{{
						OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
					}},
				}, {
					ID: "*", Scope: "dodItem", EvidenceRole: "primary_state_change",
					TargetRefs:            []string{"workspace_deliverable"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "git-state-validation", RequiredCheckRefs: []string{"check/git-state"},
					CapabilityStateRequirements: []runtimecontract.RuntimeProfileCapabilityStateRequirement{{
						OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
					}},
				}},
			}},
		},
	}
	bootstrap, err := activationcore.RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver bounded inventory fixture"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"verified deliverable"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := bootstrap.InitialFacts(common.WorkspaceID("ws-1"), common.RunID("run-1"))
	if err != nil {
		t.Fatal(err)
	}
	initial := runtime.PreflightRuntimeFacts(facts, bootstrap.Requirements)
	seen := make(map[runtimecontract.PrerequisiteKind]struct{})
	for _, prerequisite := range initial.Prerequisites.Pending {
		seen[prerequisite.Kind] = struct{}{}
		if prerequisite.Kind == runtimecontract.PrerequisiteKindValidationScratch {
			if prerequisite.OwnerModuleID != runtimecontract.RuntimeModuleOrchestrator || runtimePrerequisiteResolverClassification(prerequisite.Kind) != runtimePrerequisiteResolverLocal {
				t.Fatalf("activation scratch prerequisite has non-local resolver binding: %+v", prerequisite)
			}
			continue
		}
		if runtimePrerequisiteResolverClassification(prerequisite.Kind) != runtimePrerequisiteResolverExternal || strings.TrimSpace(prerequisite.OwnerModuleID) == "" {
			t.Fatalf("activation prerequisite has no canonical external owner: %+v", prerequisite)
		}
	}

	// Target selection and snapshot are reducer-generated only after a focus
	// requires grounding; they are deliberately absent from the static bootstrap.
	groundingFacts := facts
	groundingFacts.Focus = runtimecontract.CoverageFocus{
		ID: "focus-ground", SliceID: "slice-default-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequireTargetGrounding: true,
		TargetRole: "implementation", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorOwnerRef: "profile-inventory",
		TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"implementation.go"},
	}
	groundingFacts.Coverage[0].FocusID = groundingFacts.Focus.ID
	groundingFacts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
	grounded, decision, err := runtime.ReduceRuntimeStateWithDirective(groundingFacts, nil)
	if err != nil || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("reducer did not generate target selection prerequisite: facts=%+v decision=%+v err=%v", grounded, decision, err)
	}
	var selectionPrerequisite runtimecontract.RuntimePrerequisite
	for _, prerequisite := range grounded.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindManifest {
			grounded, err = runtime.CommitPrerequisiteResult(grounded, runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &runtimecontract.WorkspaceManifest{Revision: "tree-v1"}})
			if err != nil {
				t.Fatal(err)
			}
		}
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			selectionPrerequisite = prerequisite
		}
	}
	if selectionPrerequisite.ID == "" {
		t.Fatalf("reducer did not expose target selection prerequisite: %+v", grounded.Prerequisites.Pending)
	}
	if selectionPrerequisite.Kind != runtimecontract.PrerequisiteKindTargetSelection || selectionPrerequisite.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		runtimePrerequisiteResolverClassification(selectionPrerequisite.Kind) != runtimePrerequisiteResolverExternal {
		t.Fatalf("reducer target selection binding=%+v", selectionPrerequisite)
	}
	seen[selectionPrerequisite.Kind] = struct{}{}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: selectionPrerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: selectionPrerequisite.SubjectRef,
		SelectorRef: selectionPrerequisite.TargetSelectorRef, SelectorOwnerRef: selectionPrerequisite.TargetSelectorOwnerRef,
		SelectorSubjectRef: selectionPrerequisite.TargetSelectorSubjectRef, SelectorCandidateRefs: selectionPrerequisite.TargetSelectorCandidateRefs,
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1, ObservedRevision: "tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	grounded, err = runtime.CommitPrerequisiteResult(grounded, runtime.PrerequisiteResult{PrerequisiteID: selectionPrerequisite.ID, TargetSelection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	grounded, decision, err = runtime.ReduceRuntimeStateWithDirective(grounded, nil)
	if err != nil || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(grounded.Prerequisites.Pending) != 1 || grounded.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
		t.Fatalf("reducer did not generate target snapshot prerequisite: facts=%+v decision=%+v err=%v", grounded, decision, err)
	}
	if grounded.Prerequisites.Pending[0].OwnerModuleID != runtimecontract.RuntimeModuleFile || runtimePrerequisiteResolverClassification(grounded.Prerequisites.Pending[0].Kind) != runtimePrerequisiteResolverExternal {
		t.Fatalf("reducer target snapshot binding=%+v", grounded.Prerequisites.Pending[0])
	}
	seen[runtimecontract.PrerequisiteKindTargetSnapshot] = struct{}{}

	failureFacts := facts
	failureFacts.Failure = &runtimecontract.FailureFact{Signature: "failure-v1", TargetRefs: []string{"implementation.go"}}
	failureFacts = runtime.PreflightRuntimeFacts(failureFacts, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindFailureTarget, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}})
	if len(failureFacts.Prerequisites.Pending) != 1 || failureFacts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindFailureTarget ||
		runtimePrerequisiteResolverClassification(failureFacts.Prerequisites.Pending[0].Kind) != runtimePrerequisiteResolverLocal {
		t.Fatalf("failure target local prerequisite=%+v", failureFacts.Prerequisites.Pending)
	}
	seen[runtimecontract.PrerequisiteKindFailureTarget] = struct{}{}

	want := []runtimecontract.PrerequisiteKind{
		runtimecontract.PrerequisiteKindManifest, runtimecontract.PrerequisiteKindTargetSelection, runtimecontract.PrerequisiteKindTargetSnapshot,
		runtimecontract.PrerequisiteKindFailureTarget, runtimecontract.PrerequisiteKindCapability, runtimecontract.PrerequisiteKindCapabilityState,
		runtimecontract.PrerequisiteKindValidationScratch, runtimecontract.PrerequisiteKindProviderCeiling, runtimecontract.PrerequisiteKindOperationAdmissibility,
	}
	for _, kind := range want {
		if _, found := seen[kind]; !found {
			t.Fatalf("production requirement sources did not reach prerequisite kind %q; seen=%v", kind, seen)
		}
	}
}
