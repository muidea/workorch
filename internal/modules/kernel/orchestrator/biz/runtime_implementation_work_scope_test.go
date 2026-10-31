package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
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
		MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true,
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
	if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte("implementation.txt"), 0o600); err != nil {
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

	if singleObserved.Content == nil || len(singleObserved.Content.Snapshots) != 1 || singleObserved.Content.Snapshots[0].Content != "implementation.txt" || singleObserved.Content.Validate(singleObserved) != nil {
		t.Fatal("scope lost exact real source content")
	}
	frozen := singleObserved.Clone()
	frozen.Content.Snapshots[0].Content = "altered"
	if frozen.ValidateForRequest(singleRequest) == nil || singleObserved.Content.Snapshots[0].Content != "implementation.txt" {
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
		if err != nil || string(data) != name {
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
	if err != nil || !reflect.DeepEqual(observed.Selection, replayed.Selection) || observed.Observation.ObservationID != replayed.Observation.ObservationID {
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
	// Full original bytes are required; an oversized source cannot silently
	// become a complete input, even when its prefix and inventory are available.
	if err := os.WriteFile(filepath.Join(directory, "implementation.txt"), []byte(strings.Repeat("x", runtimecontract.RuntimeImplementationWorkContentMaxBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	tooLarge := compile(policy.TargetSelectorRef, manifest())
	if partial, err := acquireRuntimeImplementationWorkScope(ctx, hub, directory, tooLarge, sources[tooLarge.RequestID]); err == nil || (partial.Content != nil && partial.Content.Validate(partial) == nil) {
		t.Fatal("oversized current content became complete semantic input")
	}
}
