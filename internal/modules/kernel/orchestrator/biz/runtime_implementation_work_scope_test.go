package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestImplementationWorkScopeRealOwnerReadOnlyAndRevisionFence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	startOwner := func() *filemodule.Module {
		owner := filemodule.New()
		if err := owner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	owner := startOwner()
	defer func() { _ = owner.Stop(ctx) }()
	directory := t.TempDir()
	manifest := func() runtimecontract.RuntimeWorkspaceManifestBinding {
		facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1"},
			[]runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile}})
		id := facts.Prerequisites.Pending[0].ID
		committed, err := AcquireAndCommitWorkspaceManifestPrerequisite(ctx, hub, facts, fileevents.AcquireWorkspaceManifestCommand{
			WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", PrerequisiteID: id, WorkspaceDeliverableDirectory: directory,
		})
		if err != nil {
			t.Fatal(err)
		}
		observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(committed.Resources.PrerequisiteObservations, id)
		if !ok {
			t.Fatal("missing owner manifest observation")
		}
		if committed.Resources.Manifest == nil {
			t.Fatal("missing owner manifest")
		}
		return runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: *committed.Resources.Manifest, Observation: observation}
	}
	seed := runtimecontract.RuntimeDefinitionSeed{SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition-1", PolicyHash: "policy-1",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "original criterion"}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"},
			CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
	}.Normalized()
	seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{
			{Kind: runtimecontract.StepKindFileWrite},
			{Kind: runtimecontract.StepKindFileEdit},
			{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace"},
		},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		TargetSelectorRef:                    "file/workspace_deliverable", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "work",
		MaxRepeatedWorkStates: 2, MaxTargetsPerRound: 1, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
	}
	sources := map[string]runtimecontract.RuntimeImplementationWorkScopeSources{}
	compile := func(selector string, binding runtimecontract.RuntimeWorkspaceManifestBinding) runtimecontract.RuntimeImplementationWorkScopeRequest {
		p := policy.Clone()
		p.TargetSelectorRef = selector
		focuses := []runtimecontract.RuntimeDefinitionFocus{{ID: "focus", SliceID: "slice", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "original criterion",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, RequireAssessment: true,
			EvidenceDeclarationRef: "definition/dodItem/dod-1", AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			RequiredCheckRefs: []string{"check-1"}, ImplementationWorkPolicy: &p}}
		profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "dod-1", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ImplementationWorkPolicy: &p,
		}}}}
		source := runtimecontract.RuntimeImplementationWorkScopeSources{WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority",
			Seed: seed, Focuses: focuses, Profiles: profiles, CoverageWorkItemID: "cwi", ManifestBinding: binding}
		request, err := source.Compile()
		if err != nil {
			t.Fatal(err)
		}
		sources[request.RequestID] = source
		return request
	}
	// 空目录只取得完整只读 scope；prospective 工作目标仍须由后续 semantic/feasibility 封存。
	emptyRequest := compile(policy.TargetSelectorRef, manifest())
	emptyObserved, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, emptyRequest, sources[emptyRequest.RequestID])
	if err != nil {
		t.Fatal(err)
	}
	if emptyObserved.Selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || emptyObserved.Selection.CandidateCount != 0 || len(emptyObserved.Selection.TargetRefs) != 0 || len(emptyObserved.Selection.CandidateTargetRefs) != 0 || emptyObserved.Selection.ProspectiveTargetRef != "" {
		t.Fatalf("empty owner scope invented a selected working target: %#v", emptyObserved.Selection)
	}
	if entries, err := os.ReadDir(directory); err != nil || len(entries) != 0 {
		t.Fatalf("empty scope observation wrote deliverable content: entries=%v err=%v", entries, err)
	}
	const implementationContent = "implementation.txt\r\n  literal [REDACTED] [SECRET] [MASKED] 用户原文\r\n"
	if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte(implementationContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, emptyRequest, sources[emptyRequest.RequestID]); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkScopeStale) {
		t.Fatalf("empty scope stayed current after real creation: %v", err)
	}
	singleRequest := compile(policy.TargetSelectorRef, manifest())
	singleObserved, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, singleRequest, sources[singleRequest.RequestID])
	if err != nil || singleObserved.Selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || singleObserved.Selection.CandidateCount != 1 || !reflect.DeepEqual(singleObserved.Selection.TargetRefs, []string{"implementation.txt"}) {
		t.Fatalf("single existing target owner scope incomplete: selection=%#v err=%v", singleObserved.Selection, err)
	}

	if singleObserved.Content == nil || len(singleObserved.Content.Snapshots) != 1 || singleObserved.Content.Snapshots[0].Content != implementationContent || singleObserved.Content.Validate(singleObserved) != nil {
		t.Fatal("scope lost exact real source content")
	}
	frozen := singleObserved.Clone()
	frozen.Content.Snapshots[0].Content = "altered"
	if frozen.ValidateForRequest(singleRequest) == nil || singleObserved.Content.Snapshots[0].Content != implementationContent {
		t.Fatal("scope clone mutated original or trusted content hash only")
	}
	if singleRequest.RequestID == emptyRequest.RequestID || singleObserved.Selection.ObservedRevision == emptyObserved.Selection.ObservedRevision {
		t.Fatal("real creation reused the empty scope identity or owner revision")
	}
	if err := os.WriteFile(filepath.Join(directory, "acceptance.txt"), []byte("acceptance.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, singleRequest, sources[singleRequest.RequestID]); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkScopeStale) {
		t.Fatalf("single target scope stayed current after another creation: %v", err)
	}
	request := compile(policy.TargetSelectorRef, manifest())
	before, _ := json.Marshal(request)
	// A valid, independently compiled request for another authority must fail
	// before touching the Hub. A nil Hub makes accidental owner I/O fail loudly.
	other := sources[request.RequestID]
	other.AuthoritySnapshotID = "other-authority"
	foreign, err := other.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireRuntimeImplementationWorkScope(ctx, nil, directory, foreign, sources[request.RequestID]); err == nil {
		t.Fatal("cross-authority request reached owner adapter")
	}
	observed, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, request, sources[request.RequestID])
	if err != nil {
		t.Fatal(err)
	}
	// A one-target work limit must not truncate the complete two-target fact.
	// Neither selected target is a write permission; input protection remains
	// the later manifest/feasibility contract's responsibility.
	if observed.Selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || observed.Selection.CandidateCount != 2 || len(observed.Selection.TargetRefs) != 2 {
		t.Fatalf("observation lost complete inventory: %#v", observed.Selection)
	}
	for _, name := range []string{"implementation.txt", "acceptance.txt"} {
		data, err := os.ReadFile(filepath.Join(directory, name))
		want := name
		if name == "implementation.txt" {
			want = implementationContent
		}
		if err != nil || string(data) != want {
			t.Fatalf("query changed %s: %v", name, err)
		}
	}
	if err := owner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	owner = startOwner()
	var replayRequest runtimecontract.RuntimeImplementationWorkScopeRequest
	if err := json.Unmarshal(before, &replayRequest); err != nil {
		t.Fatal(err)
	}
	replayed, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, replayRequest, sources[request.RequestID])
	if err != nil || !reflect.DeepEqual(observed.Selection, replayed.Selection) || !reflect.DeepEqual(observed.Content, replayed.Content) || observed.Observation.ObservationID != replayed.Observation.ObservationID {
		t.Fatalf("fresh owner did not reproduce scope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte("new revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, request, sources[request.RequestID]); !errors.Is(err, runtimecontract.ErrRuntimeImplementationWorkScopeStale) {
		t.Fatalf("stale tree did not return typed stale: %v", err)
	}
	fresh := compile(policy.TargetSelectorRef, manifest())
	if fresh.RequestID == request.RequestID {
		t.Fatal("fresh tree reused request fence")
	}
	if _, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, fresh, sources[fresh.RequestID]); err != nil {
		t.Fatal(err)
	}
	unsupported := compile("not-supported", fresh.ManifestBinding)
	negative, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, unsupported, sources[unsupported.RequestID])
	if err != nil || negative.Selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || len(negative.Selection.TargetRefs) != 0 {
		t.Fatalf("unsupported selector lost negative owner fact: %#v, %v", negative.Selection, err)
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("read-only adapter changed request")
	}
	// Independent owner reads retain complete bytes when the workspace total
	// crosses the retired 128 KiB scope ceiling. Per-target owner bounds remain.
	for n := 0; n < 3; n++ {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("large-%d.txt", n)), []byte(strings.Repeat("x", 60*1024)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	grown := compile(policy.TargetSelectorRef, manifest())
	grownScope, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, grown, sources[grown.RequestID])
	if err != nil {
		t.Fatal("cumulative workspace bytes incorrectly limited", err)
	}
	total := 0
	for _, snapshot := range grownScope.Content.Snapshots {
		total += len(snapshot.Content)
	}
	if total <= 128*1024 || grownScope.Content.Validate(grownScope) != nil {
		t.Fatal("grown scope lost complete bytes")
	}
	// A complete single target may cross the ordinary read and retired scope
	// ceilings without losing any member of the selected inventory.
	for _, size := range []int{72001, 128*1024 + 1, fileevents.DefaultTargetSnapshotBytes} {
		if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte(strings.Repeat("x", size)), 0600); err != nil {
			t.Fatal(err)
		}
		request := compile(policy.TargetSelectorRef, manifest())
		scope, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, request, sources[request.RequestID])
		if err != nil || scope.Content == nil || scope.Content.Validate(scope) != nil {
			t.Fatal("legal full scope rejected", size, err)
		}
	}
	// Full original bytes are required; an oversized source cannot silently
	// become a complete input, even when its prefix and inventory are available.
	if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte(strings.Repeat("x", fileevents.DefaultTargetSnapshotBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	tooLarge := compile(policy.TargetSelectorRef, manifest())
	partial, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, tooLarge, sources[tooLarge.RequestID])
	var capacity *fileevents.TargetSnapshotCapacityError
	if !errors.As(err, &capacity) || capacity.SizeBytes != fileevents.DefaultTargetSnapshotBytes+1 || capacity.LimitBytes != fileevents.DefaultTargetSnapshotBytes || (partial.Content != nil && partial.Content.Validate(partial) == nil) {
		t.Fatal("oversized current content became complete semantic input")
	}
}

func TestImplementationWorkScopeWithExclusionsSkipsProtectedInputsUnlessRevised(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner := filemodule.New()
	if err := owner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Stop(ctx) }()

	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "checks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "implementation.go"), []byte("package impl\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "checks/input_test.go"), []byte("package checks\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1"},
		[]runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile}})
	prereqID := facts.Prerequisites.Pending[0].ID
	committed, err := AcquireAndCommitWorkspaceManifestPrerequisite(ctx, hub, facts, fileevents.AcquireWorkspaceManifestCommand{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority", PrerequisiteID: prereqID, WorkspaceDeliverableDirectory: directory,
	})
	if err != nil {
		t.Fatal(err)
	}
	obs, ok := runtimecontract.RuntimePrerequisiteObservationForID(committed.Resources.PrerequisiteObservations, prereqID)
	if !ok || committed.Resources.Manifest == nil {
		t.Fatal("missing owner manifest observation")
	}
	binding := runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: *committed.Resources.Manifest, Observation: obs}

	seed := runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition-1", PolicyHash: "policy-1",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi", SliceID: "slice", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "criterion"}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"},
			CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
	}.Normalized()
	seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
	policy := runtimecontract.RuntimeImplementationWorkPolicy{
		ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
		},
		TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "work",
		MaxRepeatedWorkStates: 2, MaxTargetsPerRound: 2, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
		InputRevisionProtocolRef: runtimecontract.RuntimeValidationInputRevisionProtocolV1,
	}
	focuses := []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus", SliceID: "slice", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "criterion",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		RequireAssessment: true, EvidenceDeclarationRef: "definition/dodItem/dod-1",
		AcceptedStepKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationRecipeID: "recipe-1",
		RequiredCheckRefs:  []string{"check-1"}, ImplementationWorkPolicy: &policy,
	}}
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "dod-1", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ImplementationWorkPolicy: &policy,
	}}}}
	catalogs := []runtimecontract.RuntimeOperationAdmissibility{{
		PrerequisiteID: "shell-inputs",
		OwnerModuleID:  runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "recipe-1", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind:            runtimecontract.RuntimeBlockKindManagedValidationSession,
			InputTargetOwnerModuleID: runtimecontract.RuntimeModuleFile,
			InputTargetRefs:          []string{"checks/input_test.go"},
		}},
	}}

	ordinarySource := runtimecontract.RuntimeImplementationWorkScopeSources{
		WorkspaceID: "ws", RunID: "run", AuthoritySnapshotID: "authority",
		Seed: seed, Focuses: focuses, Profiles: profiles, CoverageWorkItemID: "cwi", ManifestBinding: binding,
	}
	ordinaryReq, err := ordinarySource.Compile()
	if err != nil {
		t.Fatal(err)
	}
	excluded := runtimecontract.RuntimeImplementationWorkContentExcludedTargets(runtimecontract.RuntimeModuleFile, catalogs, nil)
	ordinaryObs, err := acquireRuntimeImplementationWorkScopeWithExclusions(ctx, hub, directory, ordinaryReq, ordinarySource, excluded)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinaryObs.Selection.TargetRefs) != 2 || ordinaryObs.Content == nil ||
		!reflect.DeepEqual(ordinaryObs.Content.ExcludedTargetRefs, []string{"checks/input_test.go"}) ||
		len(ordinaryObs.Content.Snapshots) != 1 || ordinaryObs.Content.Snapshots[0].TargetRef != "implementation.go" ||
		ordinaryObs.Content.Validate(ordinaryObs) != nil {
		t.Fatalf("ordinary excluded scope observation unexpected: sel=%v content=%+v", ordinaryObs.Selection.TargetRefs, ordinaryObs.Content)
	}

	reqs, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition-1", seed.CoverageItems, focuses)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("CompileRuntimeValidationProofRequirements = %+v err=%v", reqs, err)
	}
	attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(reqs[0].RequiredCheckRefs))
	for _, checkRef := range reqs[0].RequiredCheckRefs {
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef:       checkRef,
			ObservationProtocolRef: "test-validation-protocol/v1",
			NativeCheckRef:         "native/" + checkRef,
		})
	}
	revBinding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(reqs[0], "recipe-1", attestations...)
	if err != nil {
		t.Fatalf("RuntimeValidationProofRequirementBindingForRecipe: %v", err)
	}
	inputContent := "package checks\n"
	subject := runtimecontract.RuntimeValidationInputReviewSubject{
		ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol,
		Files: []runtimecontract.RuntimeValidationInputReviewFile{{
			TargetRef:   "checks/input_test.go",
			Content:     inputContent,
			ContentHash: runtimecontract.RuntimeValidationInputContentHash(inputContent),
		}},
	}
	subject.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(subject, revBinding)
	revBinding.InputReview = &subject
	checks := make([]runtimecontract.RuntimeValidationInputCheckReview, 0, len(revBinding.RequiredCheckRefs))
	for _, checkRef := range revBinding.RequiredCheckRefs {
		checks = append(checks, runtimecontract.RuntimeValidationInputCheckReview{
			RequiredCheckRef: checkRef,
			Conclusion:       runtimecontract.RuntimeValidationInputConflict,
			Reason:           "input requires revision",
			InputTargetRefs:  []string{"checks/input_test.go"},
			Obligations: []runtimecontract.RuntimeValidationInputObligationReview{{
				CriterionText:   "criterion",
				Finding:         runtimecontract.RuntimeValidationInputConflicting,
				Reason:          "input conflicts with criterion",
				InputTargetRefs: []string{"checks/input_test.go"},
			}},
		})
	}
	revision := &runtimecontract.RuntimeValidationInputRevision{
		WorkspaceID:            "ws",
		RunID:                  "run",
		DefinitionSeedHash:     seed.Hash,
		SourceManifestHash:     "manifest-rev-1",
		SourceLogicalRequestID: "logical-rev-1",
		SourceDocumentHash:     "doc-rev-1",
		OwnerModuleID:          runtimecontract.RuntimeModuleFile,
		InputTargetRefs:        []string{"checks/input_test.go"},
		Binding:                revBinding,
		Report: runtimecontract.RuntimeValidationInputReview{
			SubjectHash: subject.Hash,
			Checks:      checks,
		},
	}
	if err := revision.Validate(); err != nil {
		t.Fatalf("invalid test input revision: %v", err)
	}
	revisedSource := ordinarySource
	revisedSource.InputRevision = revision
	revisedReq, err := revisedSource.Compile()
	if err != nil {
		t.Fatal(err)
	}
	revisedExcluded := runtimecontract.RuntimeImplementationWorkContentExcludedTargets(runtimecontract.RuntimeModuleFile, catalogs, revisedReq.InputRevision)
	revisedObs, err := acquireRuntimeImplementationWorkScopeWithExclusions(ctx, hub, directory, revisedReq, revisedSource, revisedExcluded)
	if err != nil {
		t.Fatal(err)
	}
	if revisedObs.Content == nil || revisedObs.Content.ExcludedTargetRefs != nil ||
		len(revisedObs.Content.Snapshots) != 2 || revisedObs.Content.Validate(revisedObs) != nil {
		t.Fatalf("revision-unlocked scope observation unexpected: content=%+v", revisedObs.Content)
	}

	// Simulate a completed Round 1 where checks/input_test.go was modified under InputRevision,
	// then acquire Round 2 scope (ordinary Request with InputRevision==nil):
	// checks/input_test.go must stay in ExcludedTargetRefs and Snapshots-excluded, while being
	// acquired read-only into Content.ReviewSnapshots at the same ManifestRevision.
	rounds := []runtimecontract.RuntimeImplementationWorkRound{{
		CoverageWorkItemID:      ordinaryReq.CoverageWorkItemID,
		Ordinal:                 1,
		ScopeRequest:            revisedReq,
		CompletedReceiptSetHash: "receipt-set-round-1",
		WorkingBinding: &runtimecontract.RuntimeImplementationWorkingBinding{
			Proposals: []runtimecontract.RuntimeSemanticProofProposal{{
				CoverageWorkItemID: ordinaryReq.CoverageWorkItemID,
				Target: runtimecontract.RuntimeOperationTargetAuthorization{
					Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
					OwnerScopeTargetRefs: []string{"checks/input_test.go"},
				},
			}},
		},
	}}
	nextObs, err := acquireRuntimeImplementationWorkScopeWithTargets(ctx, hub, directory, ordinaryReq, ordinarySource, excluded, rounds)
	if err != nil {
		t.Fatalf("acquireRuntimeImplementationWorkScopeWithTargets: %v", err)
	}
	if nextObs.Content == nil ||
		!reflect.DeepEqual(nextObs.Content.ExcludedTargetRefs, []string{"checks/input_test.go"}) ||
		len(nextObs.Content.Snapshots) != 1 || nextObs.Content.Snapshots[0].TargetRef != "implementation.go" ||
		len(nextObs.Content.ReviewSnapshots) != 1 || nextObs.Content.ReviewSnapshots[0].TargetRef != "checks/input_test.go" ||
		nextObs.Content.ManifestRevision != nextObs.Selection.ObservedRevision ||
		nextObs.Content.Validate(nextObs) != nil {
		t.Fatalf("round 2 excluded ReviewSnapshots unexpected: content=%+v", nextObs.Content)
	}
}
