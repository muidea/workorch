package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	projectcommon "github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	fileProspectiveDeltaCoverageID  = "cwi-owner-fact-acquisition"
	fileProspectiveDeltaSelector    = "workspace-relative-new-file-target"
	fileProspectiveDeltaConstraint  = "workspace-relative-path"
	fileProspectiveDeltaContractRef = "shell/managed-validation/prospective-workspace-manifest.v1"
)

func TestFileOwnerSemanticFeasibilityPublishesDeterministicProspectiveManifestDelta(t *testing.T) {
	ctx, hub := startFileProspectiveDeltaModule(t)
	workspace := t.TempDir()
	writeFileProspectiveDeltaFixtureFile(t, workspace, "z.txt", "z\n")
	writeFileProspectiveDeltaFixtureFile(t, workspace, "a.txt", "a\n")

	adapter := filebiz.NewRuntimeAdapter()
	before, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	const targetRef = "nested/new.txt"
	command := fileProspectiveDeltaCommand(t, workspace, targetRef, before.TargetRefs)

	first := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
	if first.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible || first.Observation.ProspectiveWorkspaceManifestDelta == nil {
		t.Fatalf("File owner feasibility = %+v", first)
	}
	if first.Observation.OwnerFactProspectiveSatisfactionObservation != nil {
		t.Fatalf("File owner synthesized required-owner satisfaction: %+v", first.Observation.OwnerFactProspectiveSatisfactionObservation)
	}
	delta := first.Observation.ProspectiveWorkspaceManifestDelta.Normalized()
	want := runtimecontract.RuntimeProspectiveWorkspaceManifestDelta{
		SchemaVersion: runtimecontract.RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent,
		WorkspaceID:   command.WorkspaceID, RunID: command.RunID,
		ManifestHash: command.Manifest.Hash, DefinitionSeedHash: command.Manifest.DefinitionSeedHash,
		CoverageWorkItemID: command.Proposal.CoverageWorkItemID,
		ProposalHash:       runtimecontract.StableRuntimeSemanticProofProposalHash(command.Proposal),
		OwnerModuleID:      runtimecontract.RuntimeModuleFile, CatalogSnapshotID: command.OwnerCatalog.SnapshotID,
		ProspectiveSatisfactionContractRef: fileProspectiveDeltaContractRef,
		BaseWorkspaceManifestRevision:      before.Revision, ProspectiveTargetRefs: []string{targetRef},
		ResultingTargetRefs: []string{"a.txt", targetRef, "z.txt"},
	}.Normalized()
	if !reflect.DeepEqual(delta, want) {
		t.Fatalf("prospective workspace manifest delta = %+v, want %+v", delta, want)
	}
	wantHash := runtimecontract.StableRuntimeProspectiveWorkspaceManifestDeltaHash(want)
	if got := runtimecontract.StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta); got == "" || got != wantHash {
		t.Fatalf("prospective workspace manifest delta hash = %q, want %q", got, wantHash)
	}
	if err := first.Observation.ValidateOwnerForProposal(command.Manifest, command.Proposal); err != nil {
		t.Fatalf("owner observation validation: %v", err)
	}

	afterFirst, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterFirst, before) {
		t.Fatalf("read-only feasibility changed File manifest: before=%+v after=%+v", before, afterFirst)
	}
	if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(targetRef))); !os.IsNotExist(err) {
		t.Fatalf("read-only feasibility created prospective target: err=%v", err)
	}

	replayed := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
	if !reflect.DeepEqual(replayed, first) {
		t.Fatalf("replayed File owner observation drifted: first=%+v replayed=%+v", first, replayed)
	}
	afterReplay, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterReplay, before) {
		t.Fatalf("replayed feasibility changed File manifest: before=%+v after=%+v", before, afterReplay)
	}
}

func TestFileOwnerSemanticFeasibilityPublishesBoundedBatchDeltaWithoutMutation(t *testing.T) {
	ctx, hub := startFileProspectiveDeltaModule(t)
	workspace := t.TempDir()
	targetRefs := []string{"go.mod", "internal/example/runtime_test.go"}
	command := fileProspectiveDeltaCommandForTargets(t, workspace, targetRefs, nil)

	observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
	if observation.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible || observation.Observation.ProspectiveWorkspaceManifestDelta == nil {
		t.Fatalf("batch File owner feasibility = %+v", observation)
	}
	delta := observation.Observation.ProspectiveWorkspaceManifestDelta.Normalized()
	if !reflect.DeepEqual(delta.ProspectiveTargetRefs, targetRefs) || !reflect.DeepEqual(delta.ResultingTargetRefs, targetRefs) {
		t.Fatalf("batch prospective delta targets = %+v", delta)
	}
	for _, targetRef := range targetRefs {
		if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(targetRef))); !os.IsNotExist(err) {
			t.Fatalf("batch feasibility created %q: %v", targetRef, err)
		}
	}
	replayed := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
	if !reflect.DeepEqual(replayed, observation) {
		t.Fatalf("batch feasibility replay drifted: first=%+v replay=%+v", observation, replayed)
	}
}

func TestFileOwnerSemanticFeasibilityDoesNotPublishProspectiveDeltaForNonFeasibleInput(t *testing.T) {
	t.Run("ancestor descendant batch", func(t *testing.T) {
		ctx, hub := startFileProspectiveDeltaModule(t)
		workspace := t.TempDir()
		command := fileProspectiveDeltaCommandForTargets(t, workspace, []string{"nested", "nested/runtime_test.go"}, nil)
		observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
		assertFileProspectiveDeltaAbsent(t, observation, runtimecontract.RuntimeSemanticFeasibilityInfeasible)
		if entries, err := os.ReadDir(workspace); err != nil || len(entries) != 0 {
			t.Fatalf("ancestor-conflicting batch changed workspace: entries=%v err=%v", entries, err)
		}
	})

	t.Run("aggregate batch capacity", func(t *testing.T) {
		ctx, hub := startFileProspectiveDeltaModule(t)
		workspace := t.TempDir()
		for idx := 0; idx < 498; idx++ {
			writeFileProspectiveDeltaFixtureFile(t, workspace, fmt.Sprintf("entry-%03d.txt", idx), "x\n")
		}
		command := fileProspectiveDeltaCommandForTargets(t, workspace, []string{"nested/one.go", "nested/two_test.go"}, nil)
		observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
		assertFileProspectiveDeltaAbsent(t, observation, runtimecontract.RuntimeSemanticFeasibilityInfeasible)
		for _, targetRef := range command.Proposal.Target.ProspectiveTargetRefs {
			if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(targetRef))); !os.IsNotExist(err) {
				t.Fatalf("capacity-rejected batch created %q: %v", targetRef, err)
			}
		}
	})

	t.Run("existing target", func(t *testing.T) {
		ctx, hub := startFileProspectiveDeltaModule(t)
		workspace := t.TempDir()
		const targetRef = "already-exists.txt"
		writeFileProspectiveDeltaFixtureFile(t, workspace, targetRef, "existing\n")
		manifest, err := filebiz.NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
		if err != nil {
			t.Fatal(err)
		}
		// Keep the sealed prospective catalog internally valid while allowing
		// the File owner to report the live existence contradiction.
		command := fileProspectiveDeltaCommand(t, workspace, targetRef, nil)
		if command.OwnerCatalog.ObservedRevision != manifest.Revision {
			t.Fatalf("catalog revision = %q, want %q", command.OwnerCatalog.ObservedRevision, manifest.Revision)
		}
		observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
		assertFileProspectiveDeltaAbsent(t, observation, runtimecontract.RuntimeSemanticFeasibilityInfeasible)
	})

	t.Run("stale manifest", func(t *testing.T) {
		ctx, hub := startFileProspectiveDeltaModule(t)
		workspace := t.TempDir()
		command := fileProspectiveDeltaCommand(t, workspace, "new.txt", nil)
		writeFileProspectiveDeltaFixtureFile(t, workspace, "concurrent.txt", "changed\n")
		observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
		assertFileProspectiveDeltaAbsent(t, observation, runtimecontract.RuntimeSemanticFeasibilityStale)
		if _, err := os.Lstat(filepath.Join(workspace, "new.txt")); !os.IsNotExist(err) {
			t.Fatalf("stale feasibility created prospective target: err=%v", err)
		}
	})

	t.Run("invalid target", func(t *testing.T) {
		workspace := t.TempDir()
		command := fileProspectiveDeltaCommand(t, workspace, "safe.txt", nil)
		command.Proposal.Target.ProspectiveTargetRefs = []string{"../escape.txt"}
		if err := fileevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, runtimecontract.RuntimeModuleFile); err == nil {
			t.Fatal("unsafe prospective target reached the File owner probe boundary")
		}
		if _, err := os.Lstat(filepath.Join(workspace, "safe.txt")); !os.IsNotExist(err) {
			t.Fatalf("pre-probe rejection touched workspace: %v", err)
		}
	})
}

func TestFileOwnerSemanticFeasibilityReusesCanonicalProspectiveAdmissionMatrixWithoutMutation(t *testing.T) {
	testCases := []struct {
		name      string
		targetRef string
		prepare   func(*testing.T, string)
		preProbe  bool
	}{
		{name: "excluded node modules", targetRef: "node_modules/new.go"},
		{name: "noncanonical clean alias", targetRef: "a/../new.go", preProbe: true},
		{name: "backslash", targetRef: `nested\new.go`, preProbe: true},
		{name: "internal symlink alias", targetRef: "alias/new.go", prepare: func(t *testing.T, workspace string) {
			if err := os.Mkdir(filepath.Join(workspace, "real"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("real", filepath.Join(workspace, "alias")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
		{name: "regular file parent", targetRef: "parent/new.go", prepare: func(t *testing.T, workspace string) {
			writeFileProspectiveDeltaFixtureFile(t, workspace, "parent", "not a directory\n")
		}},
		{name: "manifest capacity overflow", targetRef: "overflow.go", prepare: func(t *testing.T, workspace string) {
			for idx := 0; idx < 500; idx++ {
				writeFileProspectiveDeltaFixtureFile(t, workspace, fmt.Sprintf("entry-%03d.txt", idx), "x\n")
			}
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, hub := startFileProspectiveDeltaModule(t)
			workspace := t.TempDir()
			if testCase.prepare != nil {
				testCase.prepare(t, workspace)
			}
			adapter := filebiz.NewRuntimeAdapter()
			before, err := adapter.AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			commandTarget := testCase.targetRef
			if testCase.preProbe {
				commandTarget = "safe.go"
			}
			command := fileProspectiveDeltaCommand(t, workspace, commandTarget, nil)
			if testCase.preProbe {
				command.Proposal.Target.ProspectiveTargetRefs = []string{testCase.targetRef}
				if err := fileevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, runtimecontract.RuntimeModuleFile); err == nil {
					t.Fatalf("unsafe target %q reached the File owner probe boundary", testCase.targetRef)
				}
				after, err := adapter.AcquireWorkspaceManifest(workspace)
				if err != nil || !reflect.DeepEqual(after, before) {
					t.Fatalf("pre-probe rejection changed File manifest: before=%+v after=%+v err=%v", before, after, err)
				}
				return
			}
			observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
			assertFileProspectiveDeltaAbsent(t, observation, runtimecontract.RuntimeSemanticFeasibilityInfeasible)
			after, err := adapter.AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("prospective admission mutated manifest: before=%+v after=%+v", before, after)
			}
			if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(testCase.targetRef))); err == nil {
				t.Fatalf("prospective admission created target %q", testCase.targetRef)
			}
		})
	}
}

func TestFileWorkspaceManifestRevisionTracksDirectoryAndSymlinkTopology(t *testing.T) {
	workspace := t.TempDir()
	adapter := filebiz.NewRuntimeAdapter()
	baseline, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "directory-only"), 0o755); err != nil {
		t.Fatal(err)
	}
	directoryRevision, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if directoryRevision.Revision == baseline.Revision || len(directoryRevision.TargetRefs) != 0 {
		t.Fatalf("directory-only topology did not invalidate manifest: baseline=%+v current=%+v", baseline, directoryRevision)
	}
	if err := os.Mkdir(filepath.Join(workspace, "target-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	beforeSymlink, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-directory", filepath.Join(workspace, "internal-alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	afterSymlink, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if afterSymlink.Revision == beforeSymlink.Revision || len(afterSymlink.TargetRefs) != 0 {
		t.Fatalf("symlink topology did not invalidate manifest: before=%+v after=%+v", beforeSymlink, afterSymlink)
	}
	if err := os.Rename(filepath.Join(workspace, "internal-alias"), filepath.Join(workspace, "renamed-alias")); err != nil {
		t.Fatal(err)
	}
	renamedSymlink, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if renamedSymlink.Revision == afterSymlink.Revision {
		t.Fatalf("symlink alias rename retained manifest revision: before=%+v after=%+v", afterSymlink, renamedSymlink)
	}
}

func TestFileOwnerSemanticFeasibilityRejectsNonOrchestratorAndMismatchedWorkspaceContext(t *testing.T) {
	ctx, hub := startFileProspectiveDeltaModule(t)
	workspace := t.TempDir()
	command := fileProspectiveDeltaCommand(t, workspace, "new.go", nil)
	for _, testCase := range []struct {
		name   string
		source string
		ctx    context.Context
	}{
		{name: "non orchestrator source", source: runtimecontract.RuntimeModuleShell, ctx: ctx},
		{name: "workspace context mismatch", source: runtimecontract.RuntimeModuleOrchestrator, ctx: projectcommon.ContextWithRuntimeWorkspaceID(ctx, "ws-other")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := hub.Send(magicevent.NewEventWithContext(
				fileevents.TopicProbeRuntimeSemanticProofFeasibility, testCase.source, ModuleID, magicevent.NewHeader(), testCase.ctx, command,
			))
			if _, err := result.Get(); err == nil {
				t.Fatal("invalid semantic probe ingress was accepted")
			}
			if _, err := os.Lstat(filepath.Join(workspace, "new.go")); !os.IsNotExist(err) {
				t.Fatalf("invalid semantic probe ingress mutated workspace: %v", err)
			}
		})
	}
}

func TestFileOwnerSemanticFeasibilityLeavesOrdinaryProspectiveProposalUnchanged(t *testing.T) {
	ctx, hub := startFileProspectiveDeltaModule(t)
	workspace := t.TempDir()
	command := fileProspectiveDeltaCommand(t, workspace, "ordinary.txt", nil)
	manifest := command.Manifest.Clone()
	manifest.ProofObligations = []runtimecontract.RuntimeSemanticProofObligation{{
		CoverageWorkItemID:    fileProspectiveDeltaCoverageID,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	}}
	manifest.OwnerFactDependencyCatalogs = nil
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	command.Manifest = manifest
	if err := fileevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("ordinary File prospective semantic command: %v", err)
	}

	observation := sendFileProspectiveDeltaProbe(t, ctx, hub, command)
	if observation.Observation.Status != runtimecontract.RuntimeSemanticFeasibilityFeasible ||
		observation.Observation.ProspectiveWorkspaceManifestDelta != nil || observation.Observation.OwnerFactProspectiveSatisfactionObservation != nil {
		t.Fatalf("ordinary prospective proposal behavior changed: %+v", observation)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "ordinary.txt")); !os.IsNotExist(err) {
		t.Fatalf("ordinary feasibility created prospective target: err=%v", err)
	}
}

func startFileProspectiveDeltaModule(t *testing.T) (context.Context, magicevent.Hub) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})
	return ctx, hub
}

func fileProspectiveDeltaCommand(t *testing.T, workspace, targetRef string, catalogTargetRefs []string) fileevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	return fileProspectiveDeltaCommandForTargets(t, workspace, []string{targetRef}, catalogTargetRefs)
}

func fileProspectiveDeltaCommandForTargets(t *testing.T, workspace string, targetRefs, catalogTargetRefs []string) fileevents.ProbeRuntimeSemanticProofFeasibilityCommand {
	t.Helper()
	if len(targetRefs) == 0 {
		t.Fatal("prospective delta fixture requires targets")
	}
	current, err := filebiz.NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"file-owner-fact-catalog", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite},
		catalogTargetRefs, []string{fileProspectiveDeltaSelector}, []string{fileProspectiveDeltaConstraint}, current.Revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{fileProspectiveDeltaSelector}, ConstraintRefs: []string{fileProspectiveDeltaConstraint},
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)

	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	convergence := runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: requirement, ProspectiveSatisfactionContractRef: fileProspectiveDeltaContractRef,
		MissingTrigger:               runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID:     runtimecontract.RuntimeModuleFile,
		AcquisitionOperationKind:     runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:      runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef: fileProspectiveDeltaSelector,
		TargetSelectorRef:            "file/workspace-deliverable", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "validation-input",
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell,
		ReobserveFactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	targetClauses := make([]runtimecontract.RuntimeProspectiveTargetClause, 0, len(targetRefs))
	for index, targetRef := range targetRefs {
		targetClauses = append(targetClauses, runtimecontract.RuntimeProspectiveTargetClause{
			ID: fmt.Sprintf("required-target-%d", index),
			AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{
				Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathExact, Value: targetRef,
			}},
		})
	}
	prospectiveConstraint := runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: fileProspectiveDeltaContractRef,
		ValidationSurfaceID: "recipe/prospective", ValidationRecipeID: "recipe/prospective",
		InputSchemaHash: "schema/prospective", CatalogObservedRevision: "shell-policy-prospective",
		AllOf: targetClauses,
	}
	acquisition := runtimecontract.RuntimeSemanticOwnerFactAcquisition{
		Requirement: requirement, ProspectiveSatisfactionContractRef: fileProspectiveDeltaContractRef,
		ProspectiveTargetSatisfactionConstraint: &prospectiveConstraint,
		DeclarationTargetSelectorRef:            convergence.TargetSelectorRef,
		DeclarationTargetSelectorOwnerRef:       convergence.TargetSelectorOwnerRef,
		DeclarationTargetSelectorSubjectRef:     convergence.TargetSelectorSubjectRef,
		AcquisitionOwnerModuleID:                runtimecontract.RuntimeModuleFile,
		AcquisitionOperationKind:                runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:                 runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef:            fileProspectiveDeltaSelector,
		AcquisitionConstraintRefs:               []string{fileProspectiveDeltaConstraint},
		OwnerCatalogSnapshotID:                  catalog.SnapshotID, OwnerCatalogObservedRevision: catalog.ObservedRevision,
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell,
		ReobserveFactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	dependencyCatalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"shell-owner-fact-dependency", runtimecontract.RuntimeModuleShell,
		[]string{runtimecontract.RuntimeBlockKindManagedValidationSession}, nil, nil, nil, "shell-policy-prospective",
	)
	if err != nil {
		t.Fatal(err)
	}
	dependencyCatalog.ValidationSurfaces = []runtimecontract.RuntimeValidationSurface{{
		ID: "recipe/prospective", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness:     runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "applicable workspace input required",
		InputSchemaHash: "schema/prospective", PrerequisiteRef: "workspace/validation-input",
		ProspectiveSatisfactionContractRef:      fileProspectiveDeltaContractRef,
		ProspectiveTargetSatisfactionConstraint: &prospectiveConstraint,
		CheckRefs:                               []string{"check/prospective"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
		EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	dependencyCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(dependencyCatalog)
	if err := dependencyCatalog.Validate(); err != nil {
		t.Fatalf("dependency catalog: %v", err)
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-file-prospective", RunID: "run-file-prospective",
		DefinitionRevision: "definition-file-prospective", DefinitionSeedHash: "seed-file-prospective",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: fileProspectiveDeltaCoverageID, SliceID: "slice-owner-fact-acquisition",
			SourceScope: "dodItem", SourceID: "dod-owner-fact-acquisition", TargetText: "prepare validation input", Order: 1,
		}},
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID:                     fileProspectiveDeltaCoverageID,
			RequiredEvidenceKinds:                  []string{runtimecontract.EvidenceKindValidation},
			DeclarationTargetSelectorRef:           convergence.TargetSelectorRef,
			DeclarationTargetSelectorOwnerRef:      convergence.TargetSelectorOwnerRef,
			DeclarationTargetSelectorSubjectRef:    convergence.TargetSelectorSubjectRef,
			DeclarationTargetSelectorCandidateRefs: append([]string(nil), targetRefs...),
			RequireTargetGrounding:                 true,
			RequiredOwnerFacts:                     []runtimecontract.RuntimeOwnerFactRequirement{requirement},
			OwnerFactConvergences:                  []runtimecontract.RuntimeOwnerFactConvergence{convergence},
			OwnerFactAcquisition:                   &acquisition,
		}},
		OwnerCatalogs:               []runtimecontract.RuntimeOperationAdmissibility{catalog},
		OwnerFactDependencyCatalogs: []runtimecontract.RuntimeOperationAdmissibility{dependencyCatalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: fileProspectiveDeltaCoverageID,
		OwnerModuleID:      runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:                    runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{fileProspectiveDeltaSelector},
			ProspectiveConstraintRefs: []string{fileProspectiveDeltaConstraint}, ProspectiveTargetRefs: append([]string(nil), targetRefs...),
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "profile/test/validation-input",
	}
	catalogObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
		}},
	}
	catalogObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(catalogObservation)
	command := fileevents.ProbeRuntimeSemanticProofFeasibilityCommand(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "batch-file-prospective",
		WorkspaceDeliverableDirectory: workspace, Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog, CatalogObservation: &catalogObservation,
	})
	if err := fileevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("File prospective semantic command fixture: %v", err)
	}
	return command
}

func sendFileProspectiveDeltaProbe(t *testing.T, ctx context.Context, hub magicevent.Hub, command fileevents.ProbeRuntimeSemanticProofFeasibilityCommand) runtimecontract.RuntimeSemanticProofFeasibilityObservation {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicProbeRuntimeSemanticProofFeasibility, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := data.(fileevents.ProbeRuntimeSemanticProofFeasibilityResult)
	if !ok {
		t.Fatalf("File prospective semantic result type = %T", data)
	}
	return runtimecontract.RuntimeSemanticProofFeasibilityObservation(value)
}

func assertFileProspectiveDeltaAbsent(t *testing.T, observation runtimecontract.RuntimeSemanticProofFeasibilityObservation, wantStatus runtimecontract.RuntimeSemanticFeasibilityStatus) {
	t.Helper()
	if observation.Observation.Status != wantStatus || observation.Observation.ProspectiveWorkspaceManifestDelta != nil || observation.Observation.OwnerFactProspectiveSatisfactionObservation != nil {
		t.Fatalf("non-feasible File owner observation = %+v, want status %q without prospective facts", observation, wantStatus)
	}
}

func writeFileProspectiveDeltaFixtureFile(t *testing.T, workspace, ref, content string) {
	t.Helper()
	path := filepath.Join(workspace, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
