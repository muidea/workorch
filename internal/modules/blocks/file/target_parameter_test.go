package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The actual File owner -> sealed manifest -> provider decoder -> same owner
// preflight chain must bind the selected resource, including after restart.
// No provider, feasibility response or projection selects a Runtime transition.
func TestFileTargetPreimageBindingThroughOwnerEvents(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	start := func() {
		t.Helper()
		if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
			t.Fatal(err)
		}
		if err := module.Start(); err != nil {
			t.Fatal(err)
		}
	}
	start()
	t.Cleanup(func() { _ = module.Stop(ctx) })
	workspace := t.TempDir()
	for target, content := range map[string]string{"session.txt": "before\n", "session_manager.txt": "manager\n"} {
		if err := os.WriteFile(filepath.Join(workspace, target), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	acquire := func() runtimecontract.RuntimeOperationAdmissibilityAcquisition {
		t.Helper()
		command := fileevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "file-catalog", OwnerModuleID: ModuleID,
			WorkspaceDeliverableDirectory: workspace, RequiredKinds: []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindPatchPreview},
			RequiredTargetRefs: []string{"session.txt", "session_manager.txt"}, MaxItems: 8,
		}
		response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
		value, err := response.Get()
		if err != nil {
			t.Fatal(err)
		}
		result := value.(fileevents.AcquireRuntimeOperationAdmissibilityResult).Acquisition
		if err := result.ValidateForRequest(runtimecontract.RuntimeOperationAdmissibilityRequest(command)); err != nil {
			t.Fatal(err)
		}
		return result
	}
	acquisition := acquire()
	if len(acquisition.Admissibility.TargetParameterBindings) != 3 {
		t.Fatal("File catalog did not bind edit/apply/preview preconditions")
	}
	for _, binding := range acquisition.Admissibility.TargetParameterBindings {
		if binding.Name != "baseHash" || len(binding.Values) != 2 {
			t.Fatalf("File catalog lost bounded preimages: %+v", binding)
		}
	}
	targets := []string{"session.txt", "session_manager.txt"}
	bindings := runtimecontract.NormalizeCoverageEvidenceBindings([]runtimecontract.CoverageEvidenceBinding{{
		ID: "binding-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1", TargetRefs: targets,
		DeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("test", "cwi-1"),
		Proofs: []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindStateChange,
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}}},
	}})
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, AuthoritySnapshot: runtimecontract.AuthoritySnapshot{DefinitionRevision: "definition-1"},
		Focus: runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: targets, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CoverageEvidenceBindings: bindings},
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace"}},
			TargetRefs: targets, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		},
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{acquisition.Admissibility}})
	if err != nil {
		t.Fatal(err)
	}
	const payload = `{"operations":[{"kind":"patch/apply","targetRefs":["session.txt"],"parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`
	candidate, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	operation := candidate.StateChange.Operations[0].Operation
	sum := sha256.Sum256([]byte("before\n"))
	wantHash := hex.EncodeToString(sum[:])
	foundHash := false
	for _, parameter := range operation.Parameters {
		if parameter.Name == "baseHash" {
			foundHash = true
			if parameter.String == nil || *parameter.String != wantHash {
				t.Fatal("File published the other resource preimage")
			}
		}
	}
	if !foundHash {
		t.Fatal("compiled operation omitted the selected preimage")
	}
	request := runtimecontract.RuntimeOperationFeasibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
		WorkspaceDeliverableDirectory: workspace, Admissibility: acquisition.Admissibility, CandidateManifestHash: manifest.Hash,
		OperationAuthorization: manifest.Operations[0], Operation: operation, Observation: &acquisition.Observation,
	}
	probe := func(request runtimecontract.RuntimeOperationFeasibilityRequest) (runtimecontract.RuntimeOperationFeasibilityObservation, error) {
		response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, fileevents.ProbeRuntimeOperationFeasibilityCommand(request)))
		value, err := response.Get()
		if err != nil {
			return runtimecontract.RuntimeOperationFeasibilityObservation{}, err
		}
		return runtimecontract.RuntimeOperationFeasibilityObservation(value.(fileevents.ProbeRuntimeOperationFeasibilityResult)), nil
	}
	first, err := probe(request)
	if err != nil || first.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || first.ReplayProof == nil || first.ReplayProof.BeforeHash != wantHash {
		t.Fatalf("exact preimage did not reach feasible replay proof: %+v err=%v", first, err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	start()
	persisted, _ := json.Marshal(request)
	var reloaded runtimecontract.RuntimeOperationFeasibilityRequest
	if err := json.Unmarshal(persisted, &reloaded); err != nil {
		t.Fatal(err)
	}
	replayed, err := probe(reloaded)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("duplicate/restart changed owner fact: %+v err=%v", replayed, err)
	}
	tampered := reloaded
	tampered.Operation = reloaded.Operation.Normalized()
	for i := range tampered.Operation.Parameters {
		if tampered.Operation.Parameters[i].Name == "baseHash" {
			wrong := "other-resource"
			tampered.Operation.Parameters[i].String = &wrong
		}
	}
	if _, err := probe(tampered); err == nil {
		t.Fatal("direct owner ingress bypassed sealed target precondition")
	}
	if current := acquire(); current.Admissibility.SnapshotID != acquisition.Admissibility.SnapshotID {
		t.Fatal("read-only probes changed owner resources")
	}
	if err := os.WriteFile(filepath.Join(workspace, "session.txt"), []byte("external change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err := probe(reloaded)
	if err != nil || stale.Outcome != runtimecontract.RuntimeOwnerFeasibilityStale || stale.Stale == nil || stale.ReplayProof != nil || stale.CandidateConstraintViolation != nil {
		t.Fatalf("real resource change did not remain typed stale: %+v err=%v", stale, err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "session.txt"))
	if err != nil || string(content) != "external change\n" {
		t.Fatal("stale probe caused an effect")
	}
}
