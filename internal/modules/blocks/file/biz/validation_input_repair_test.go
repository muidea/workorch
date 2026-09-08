package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileOwnerInputRepairRejectsAnyUnattestedByteDeltaWithoutEffect(t *testing.T) {
	before := "const expectedValue = 7\nassert(value == expectedvalue)\n"
	after := strings.ReplaceAll(before, "== expectedvalue", "== expectedValue")
	for _, scenario := range []string{"exact", "assertion_change", "stale_input", "old_diagnostic"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner, err := New(hub, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { owner.Teardown(ctx) })
			workspace, target := t.TempDir(), "input.test"
			current := before
			if scenario == "stale_input" {
				current += "// user change\n"
			}
			if err := os.WriteFile(filepath.Join(workspace, target), []byte(current), 0600); err != nil {
				t.Fatal(err)
			}
			catalog, err := owner.runtimeAdapter.AcquireRuntimeOperationAdmissibility(workspace, "file-catalog", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{target}, 4)
			if err != nil {
				t.Fatal(err)
			}
			repair := &runtimecontract.RuntimeValidationInputRepair{ContractRef: "owner/input-case/v1", RequiredCheckRef: "check", ObservationProtocolRef: "owner/v4", OriginalNativeCheckRef: "original", SupplementalNativeCheckRef: "supplement", TargetRef: target, BeforeHash: sha256Hex([]byte(before)), AfterHash: sha256Hex([]byte(after)), Corrections: []runtimecontract.RuntimeValidationInputSymbolCorrection{{Line: 2, Column: 17, Before: "expectedvalue", After: "expectedValue"}}}
			if scenario == "old_diagnostic" {
				repair.BeforeHash = strings.Repeat("c", 64)
			}
			authorization := fileCandidateConformanceAuthorization(t, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, target)
			authorization.InputRepair = repair
			authorization.AdmissibilitySnapshotRefs = []string{catalog.SnapshotID}
			authorization.RequireTargetRefs, authorization.TargetCardinality = true, runtimecontract.RuntimeOperationTargetCardinalityOne
			authorization.TargetParameterBindings = catalog.TargetParameterBindings
			oldText, newText, variant := "== expectedvalue", "== expectedValue", patchTypeTextReplace
			if scenario == "assertion_change" {
				newText = "!= expectedValue"
			}
			operation := runtimecontract.RuntimeOperation{ID: "input-repair", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &variant}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}}
			preimages, err := runtimecontract.RuntimeCandidateTargetParameters(authorization, operation.TargetRefs)
			if err != nil {
				t.Fatal(err)
			}
			operation.Parameters = append(operation.Parameters, preimages...)
			command := fileevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: "ws", RunID: "run", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority"}, WorkspaceDeliverableDirectory: workspace, Operation: operation, OperationAuthorization: authorization, Admissibility: catalog, CandidateManifestHash: "manifest"}
			result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
			data, resultErr := result.Get()
			if resultErr != nil {
				t.Fatal(resultErr)
			}
			observation, ok := data.(fileevents.ProbeRuntimeOperationFeasibilityResult)
			if !ok {
				t.Fatalf("unexpected owner result: %T", data)
			}
			content, err := os.ReadFile(filepath.Join(workspace, target))
			if err != nil || string(content) != current {
				t.Fatal("feasibility wrote durable input")
			}
			if scenario != "exact" {
				if observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || observation.ReplayProof != nil || observation.Feedback == nil || observation.Feedback.OriginalDiagnostic == nil || observation.Feedback.OriginalDiagnostic.OriginalCode != "validation_input_postimage_mismatch" {
					t.Fatalf("unattested input delta escaped: %+v", observation)
				}
				return
			}
			if observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || !repair.MatchesReplayProof(observation.ReplayProof) {
				t.Fatalf("exact delta refused: %+v", observation)
			}
			request, ok := patchApplyRequestFromRuntimeOperation(workspace, operation)
			if !ok {
				t.Fatal("patch request unavailable")
			}
			request.ReplayProof = observation.ReplayProof
			for attempt := 0; attempt < 2; attempt++ {
				changed, err := NewRuntimeAdapter().ApplyPatch(request)
				if err != nil || changed.BeforeHash != repair.BeforeHash || changed.AfterHash != repair.AfterHash {
					t.Fatalf("exact repair/restart failed: %+v %v", changed, err)
				}
			}
			if err := os.WriteFile(filepath.Join(workspace, target), []byte(before+"// newer input\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewRuntimeAdapter().ApplyPatch(request); err == nil {
				t.Fatal("old sealed repair overwrote newer input")
			}
			content, err = os.ReadFile(filepath.Join(workspace, target))
			if err != nil || string(content) != before+"// newer input\n" {
				t.Fatal("stale replay changed input")
			}
		})
	}
}
