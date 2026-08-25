package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellOwnerProspectiveSatisfactionUsesOnlyResultingManifestTargets(t *testing.T) {
	ctx, hub, owner := newShellProspectiveSatisfactionOwner(t, NewRuntimeAdapter())
	for _, test := range []struct {
		name       string
		targetRef  string
		want       runtimecontract.RuntimeOwnerFactProspectiveSatisfactionOutcome
		wantRecipe bool
	}{
		{name: "ordinary script does not make go test selectable", targetRef: "scripts/validate.sh", want: runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy},
		{name: "go test source makes go test selectable", targetRef: "x_test.go", want: runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy, wantRecipe: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			command := shellProspectiveSatisfactionCommandFixture(t, owner.runtimeAdapter, workspace, test.targetRef)
			response, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command)
			if err != nil {
				t.Fatal(err)
			}
			satisfaction := response.Observation.Satisfaction
			if satisfaction.Outcome != test.want {
				t.Fatalf("prospective satisfaction = %+v, want %q", satisfaction, test.want)
			}
			if test.wantRecipe {
				if len(satisfaction.ProspectiveValidationRecipeRefs) != 1 || satisfaction.ProspectiveValidationRecipeRefs[0] != "go-test" || satisfaction.Reason != "" {
					t.Fatalf("would-satisfy result = %+v", satisfaction)
				}
			} else if len(satisfaction.ProspectiveValidationRecipeRefs) != 0 || satisfaction.Reason != shellProspectiveValidationNotSatisfiedReason {
				t.Fatalf("would-not-satisfy result = %+v", satisfaction)
			}
			if _, statErr := os.Stat(filepath.Join(workspace, filepath.FromSlash(test.targetRef))); !os.IsNotExist(statErr) {
				t.Fatalf("prospective query touched target %q: %v", test.targetRef, statErr)
			}
		})
	}
}

func TestShellOwnerProspectiveSatisfactionRejectsWrongSourceScopeAndStaleCatalog(t *testing.T) {
	ctx, hub, owner := newShellProspectiveSatisfactionOwner(t, NewRuntimeAdapter())
	workspace := t.TempDir()
	command := shellProspectiveSatisfactionCommandFixture(t, owner.runtimeAdapter, workspace, "x_test.go")
	if _, err := sendShellProspectiveSatisfaction(ctx, hub, "application/workorchd", command); err == nil {
		t.Fatal("prospective satisfaction accepted a non-Orchestrator source")
	}
	wrongScope := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-other")
	if _, err := sendShellProspectiveSatisfaction(wrongScope, hub, runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
		t.Fatal("prospective satisfaction accepted mismatched workspace context")
	}

	staleAdapter := NewRuntimeAdapterWithTimeout(2 * DefaultCommandTimeout)
	stale := shellProspectiveSatisfactionCommandFixture(t, staleAdapter, workspace, "x_test.go")
	if stale.OwnerCatalog.ObservedRevision == owner.runtimeAdapter.runtimeAdmissibilityPolicyRevision() {
		t.Fatal("stale fixture did not change Shell policy revision")
	}
	result, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, stale)
	if err != nil {
		t.Fatalf("typed stale prospective satisfaction: %v", err)
	}
	if result.Observation.Stale == nil || result.Observation.Satisfaction != nil ||
		result.Observation.Stale.ExpectedRevision != stale.OwnerCatalog.ObservedRevision ||
		result.Observation.Stale.ObservedRevision != owner.runtimeAdapter.runtimeAdmissibilityPolicyRevision() {
		t.Fatalf("prospective satisfaction stale result = %+v", result.Observation)
	}
}

func TestShellOwnerProspectiveSatisfactionDuplicateIsPureValue(t *testing.T) {
	ctx, hub, owner := newShellProspectiveSatisfactionOwner(t, NewRuntimeAdapter())
	workspace := t.TempDir()
	command := shellProspectiveSatisfactionCommandFixture(t, owner.runtimeAdapter, workspace, "x_test.go")
	first, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Observation.RequestHash == "" ||
		first.Observation.Satisfaction == nil || runtimecontract.StableRuntimeOwnerFactProspectiveSatisfactionHash(*first.Observation.Satisfaction) == "" {
		t.Fatalf("duplicate pure query drifted: first=%+v second=%+v", first, second)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("duplicate prospective query mutated workspace: entries=%v err=%v", entries, err)
	}
}

func TestShellOwnerProspectiveSatisfactionRejectsRecipeIdentityDriftWithoutEffects(t *testing.T) {
	ctx, hub, owner := newShellProspectiveSatisfactionOwner(t, NewRuntimeAdapter())
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeValidationRecipe)
	}{
		{name: "command", mutate: func(recipe *runtimecontract.RuntimeValidationRecipe) { recipe.Command = "sh" }},
		{name: "ordered args", mutate: func(recipe *runtimecontract.RuntimeValidationRecipe) {
			recipe.Args = []string{"test", "-run", "TestOther", "./..."}
		}},
		{name: "timeout", mutate: func(recipe *runtimecontract.RuntimeValidationRecipe) { recipe.TimeoutMillis = 1000 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			command := shellProspectiveSatisfactionCommandFixture(t, owner.runtimeAdapter, workspace, "x_test.go")
			test.mutate(&command.ValidationRecipes[0])
			if _, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("recipe identity drift error = %v, want typed conflict", err)
			}
			entries, err := os.ReadDir(workspace)
			if err != nil || len(entries) != 0 {
				t.Fatalf("recipe identity drift touched workspace: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestShellOwnerProspectiveSatisfactionRequiresExactRecipeForEveryContractSurface(t *testing.T) {
	ctx, hub, owner := newShellProspectiveSatisfactionOwner(t, NewRuntimeAdapter())
	recipes := []runtimecontract.RuntimeValidationRecipe{
		{ID: "go-test", Command: "go", Args: []string{"test", "./..."}},
		{ID: "shell-check", Command: "sh", Args: []string{"-c", "true"}, TimeoutMillis: 5000},
	}

	t.Run("all exact surface identities are evaluated", func(t *testing.T) {
		workspace := t.TempDir()
		command := shellProspectiveSatisfactionCommandWithRecipesFixture(t, owner.runtimeAdapter, workspace, "x_test.go", recipes)
		response, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"go-test", "shell-check"}
		if response.Observation.Satisfaction.Outcome != runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy ||
			!reflect.DeepEqual(response.Observation.Satisfaction.ProspectiveValidationRecipeRefs, want) {
			t.Fatalf("multi-surface satisfaction = %+v, want recipes %v", response.Observation.Satisfaction, want)
		}
		entries, readErr := os.ReadDir(workspace)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("multi-surface query touched workspace: entries=%v err=%v", entries, readErr)
		}
	})

	t.Run("one missing recipe fails the entire query closed", func(t *testing.T) {
		workspace := t.TempDir()
		command := shellProspectiveSatisfactionCommandWithRecipesFixture(t, owner.runtimeAdapter, workspace, "x_test.go", recipes)
		command.ValidationRecipes = command.ValidationRecipes[:1]
		if _, err := sendShellProspectiveSatisfaction(ctx, hub, runtimecontract.RuntimeModuleOrchestrator, command); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("missing surface recipe error = %v, want typed conflict", err)
		}
		entries, readErr := os.ReadDir(workspace)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("missing surface recipe query touched workspace: entries=%v err=%v", entries, readErr)
		}
	})
}

func newShellProspectiveSatisfactionOwner(t *testing.T, adapter *RuntimeAdapter) (context.Context, magicevent.Hub, *Biz) {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-prospective")
	hub := magicevent.NewHub(16)
	owner := &Biz{Base: basebiz.New(modulecommon.ModuleID, hub, nil), runtimeAdapter: adapter}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		owner.Teardown(ctx)
		hub.Terminate(ctx)
	})
	return ctx, hub, owner
}

func sendShellProspectiveSatisfaction(ctx context.Context, hub magicevent.Hub, source string, command shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand) (shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult, error) {
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction,
		source,
		modulecommon.ModuleID,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult{}, common.ErrConflict
	}
	data, err := result.Get()
	if err != nil {
		return shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult{}, err
	}
	response, ok := data.(shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult)
	if !ok {
		return shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionResult{}, common.ErrConflict
	}
	return response, nil
}

func shellProspectiveSatisfactionCommandFixture(t *testing.T, adapter *RuntimeAdapter, workspace, prospectiveTarget string) shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand {
	return shellProspectiveSatisfactionCommandWithRecipesFixture(t, adapter, workspace, prospectiveTarget, []runtimecontract.RuntimeValidationRecipe{{
		ID: "go-test", Command: "go", Args: []string{"test", "./..."},
	}})
}

func shellProspectiveSatisfactionCommandWithRecipesFixture(t *testing.T, adapter *RuntimeAdapter, workspace, prospectiveTarget string, recipes []runtimecontract.RuntimeValidationRecipe) shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand {
	t.Helper()
	const (
		workspaceID         = "ws-prospective"
		runID               = "run-prospective"
		coverageID          = "cwi-prospective"
		declarationSelector = "file/workspace-deliverable"
		operationSelector   = "workspace-relative-new-file-target"
		baseRevision        = "tree-prospective-base"
	)
	recipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	if len(recipes) == 0 {
		t.Fatal("prospective satisfaction fixture requires validation recipes")
	}
	shellCatalog, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace,
		"shell-prospective-catalog",
		[]string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	)
	if err != nil {
		t.Fatal(err)
	}
	shellCatalog.TargetRefs = []string{"main.go"}
	shellCatalog.ValidationSurfaces = make([]runtimecontract.RuntimeValidationSurface, 0, len(recipes))
	for _, recipe := range recipes {
		shellCatalog.ValidationSurfaces = append(shellCatalog.ValidationSurfaces, runtimecontract.RuntimeValidationSurface{
			ID: recipe.ID, OwnerModuleID: modulecommon.ModuleID, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:                          runtimecontract.RuntimeValidationSurfacePrerequisiteRequired,
			ReadinessReason:                    shellValidationApplicableInputRequiredReason,
			InputSchemaHash:                    shellValidationRecipeSchemaHash(recipe),
			PrerequisiteRef:                    "shell-validation-prerequisite/" + shellValidationRecipeSchemaHash(recipe) + "/applicable-workspace-input",
			ProspectiveSatisfactionContractRef: modulecommon.ProspectiveSatisfactionContractRef,
			CheckRefs:                          []string{shellValidationRecipeProofCheckRef(recipe.ID)},
			ProofApplicability:                 runtimecontract.RuntimeValidationProofDirect,
			EffectScope:                        runtimecontract.RuntimeEffectEphemeralSession,
			IdempotencyClass:                   runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		})
	}
	shellCatalog = shellCatalog.Normalized()
	shellCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	if err := shellCatalog.Validate(); err != nil {
		t.Fatalf("Shell prospective catalog: %v", err)
	}
	catalogObservation := adapter.runtimeAdmissibilityObservationWithManifest(
		workspace,
		shellCatalog.PrerequisiteID,
		shellCatalog,
		shellCatalog.TargetRefs,
		shellTestWorkspaceManifestBinding(baseRevision, []string{"main.go"}),
	)

	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "file-prospective-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
			SelectorRefs:          []string{operationSelector}, ConstraintRefs: []string{"workspace-relative-path"},
		}},
		TargetRefs: []string{"main.go"}, SelectorRefs: []string{operationSelector}, ConstraintRefs: []string{"workspace-relative-path"},
		ObservedRevision: baseRevision,
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	if err := fileCatalog.Validate(); err != nil {
		t.Fatalf("File prospective catalog: %v", err)
	}

	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: modulecommon.ModuleID,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	convergence := runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: requirement, ProspectiveSatisfactionContractRef: modulecommon.ProspectiveSatisfactionContractRef,
		MissingTrigger:               runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID:     runtimecontract.RuntimeModuleFile,
		AcquisitionOperationKind:     runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:      runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef: operationSelector,
		TargetSelectorRef:            declarationSelector, TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: modulecommon.ModuleID,
		ReobserveFactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	acquisition := runtimecontract.RuntimeSemanticOwnerFactAcquisition{
		Requirement: requirement, ProspectiveSatisfactionContractRef: modulecommon.ProspectiveSatisfactionContractRef,
		ProspectiveTargetRef:              prospectiveTarget,
		DeclarationTargetSelectorRef:      declarationSelector,
		DeclarationTargetSelectorOwnerRef: "software-dev", DeclarationTargetSelectorSubjectRef: "validation_preparation",
		AcquisitionOwnerModuleID:     runtimecontract.RuntimeModuleFile,
		AcquisitionOperationKind:     runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:      runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef: operationSelector,
		AcquisitionConstraintRefs:    []string{"workspace-relative-path"},
		OwnerCatalogSnapshotID:       fileCatalog.SnapshotID, OwnerCatalogObservedRevision: fileCatalog.ObservedRevision,
		ReobserveOwnerModuleID: modulecommon.ModuleID,
		ReobserveFactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   workspaceID, RunID: runID, DefinitionRevision: "definition-prospective", DefinitionSeedHash: "seed-prospective",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: coverageID, SliceID: "slice-prospective", SourceScope: "dodItem", SourceID: "dod-prospective", TargetText: "validation is selectable", Order: 1,
		}},
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile, modulecommon.ModuleID},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID:                coverageID,
			RequiredEvidenceKinds:             []string{runtimecontract.EvidenceKindValidation},
			DeclarationTargetSelectorRef:      declarationSelector,
			DeclarationTargetSelectorOwnerRef: "software-dev", DeclarationTargetSelectorSubjectRef: "validation_preparation",
			DeclarationProspectiveTargetRef: prospectiveTarget,
			RequireTargetGrounding:          true,
			RequiredOwnerFacts:              []runtimecontract.RuntimeOwnerFactRequirement{requirement},
			OwnerFactConvergences:           []runtimecontract.RuntimeOwnerFactConvergence{convergence},
			OwnerFactAcquisition:            &acquisition,
		}},
		OwnerCatalogs:               []runtimecontract.RuntimeOperationAdmissibility{fileCatalog, shellCatalog},
		OwnerFactDependencyCatalogs: []runtimecontract.RuntimeOperationAdmissibility{shellCatalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("prospective manifest: %v", err)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs: []string{operationSelector}, ProspectiveConstraintRefs: []string{"workspace-relative-path"},
			ProspectiveTargetRef: prospectiveTarget,
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "semantic/cwi-prospective",
	}
	delta, err := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
		manifest,
		proposal,
		modulecommon.ProspectiveSatisfactionContractRef,
		fileCatalog,
		runtimecontract.WorkspaceManifest{Revision: baseRevision, TargetRefs: []string{"main.go"}},
	)
	if err != nil {
		t.Fatalf("prospective delta: %v", err)
	}
	request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest{
		WorkspaceID: workspaceID, RunID: runID, FeasibilityBatchID: "batch-prospective",
		Manifest: manifest, Proposal: proposal, Requirement: requirement,
		ProspectiveSatisfactionContractRef: modulecommon.ProspectiveSatisfactionContractRef,
		OwnerCatalog:                       shellCatalog, CatalogObservation: &catalogObservation,
		ValidationRecipes:                 recipes,
		ProspectiveWorkspaceManifestDelta: delta,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("prospective satisfaction request: %v", err)
	}
	return shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand(request)
}
