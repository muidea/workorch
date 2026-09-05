package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// All public text-edit forms must report the same owner fact before any effect.
// A current snapshot with an incorrect oldText is not a stale resource.
func TestFileTextEditApplicabilityMatrixThroughOwnerEvents(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	for _, kind := range []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindPatchPreview} {
		for _, tc := range []struct {
			name, current, old, replacement, baseHash string
			occurrence                                int64
			want                                      runtimecontract.RuntimeOwnerFeasibilityOutcome
		}{
			{name: "exact", current: "one\ntwo\n", old: "two", replacement: "three", want: runtimecontract.RuntimeOwnerFeasibilityFeasible},
			{name: "byte_exact", current: "one\n two \n", old: " two \n", replacement: " three \n", want: runtimecontract.RuntimeOwnerFeasibilityFeasible},
			{name: "delete", current: "one\ntwo\n", old: "two\n", replacement: "", want: runtimecontract.RuntimeOwnerFeasibilityFeasible},
			{name: "selected_occurrence", current: "two two", old: "two", replacement: "three", occurrence: 2, want: runtimecontract.RuntimeOwnerFeasibilityFeasible},
			{name: "missing_old_text", current: "one\ntwo\n", old: "already fixed", replacement: "three", want: runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated},
			{name: "ambiguous", current: "two two", old: "two", replacement: "three", want: runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated},
			{name: "occurrence_out_of_range", current: "two", old: "two", replacement: "three", occurrence: 2, want: runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated},
			{name: "wrong_base_hash", current: "two", old: "two", replacement: "three", baseHash: "incorrect", want: runtimecontract.RuntimeOwnerFeasibilityInfeasible},
			{name: "binary_target", current: "two\x00", old: "two", replacement: "three", want: runtimecontract.RuntimeOwnerFeasibilityInfeasible},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				workspace, targetRef := t.TempDir(), "target.txt"
				target := filepath.Join(workspace, targetRef)
				if err := os.WriteFile(target, []byte(tc.current), 0o644); err != nil {
					t.Fatal(err)
				}
				adapter := filebiz.NewRuntimeAdapter()
				manifest, err := adapter.AcquireWorkspaceManifest(workspace)
				if err != nil {
					t.Fatal(err)
				}
				inventory, err := runtimecontract.NewRuntimeOperationAdmissibility("file-admissibility", runtimecontract.RuntimeModuleFile, []string{kind}, []string{targetRef}, nil, nil, manifest.Revision)
				if err != nil {
					t.Fatal(err)
				}
				contract, _ := runtimecontract.RuntimeOperationContractForKind(kind)
				authorization := runtimecontract.RuntimeCandidateOperationManifest{
					Kind: kind, OwnerModuleID: runtimecontract.RuntimeModuleFile, Parameters: contract.Parameters,
					Variants: contract.Variants, CandidateConstraints: contract.CandidateConstraints,
					OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
					Target:                    runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{targetRef}},
				}.Normalized()
				operation := runtimecontract.RuntimeOperation{ID: "edit", Kind: kind, TargetRefs: []string{targetRef},
					Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &tc.old}, {Name: "newText", String: &tc.replacement}}}
				if kind != runtimecontract.StepKindFileEdit {
					patchType := "text_replace"
					operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "patchType", String: &patchType})
				}
				if tc.occurrence != 0 {
					operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "occurrence", Integer: &tc.occurrence})
				}
				if tc.baseHash != "" {
					operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "baseHash", String: &tc.baseHash})
				}
				request := runtimecontract.RuntimeOperationFeasibilityRequest{
					WorkspaceID: "ws-1", RunID: "run-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
					WorkspaceDeliverableDirectory: workspace, Admissibility: inventory, CandidateManifestHash: "manifest-1",
					OperationAuthorization: authorization, Operation: operation,
					CurrentTargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: targetRef, Content: tc.current, ContentHash: "current", ContentState: runtimecontract.SnapshotContentStateComplete},
				}
				probe := func() runtimecontract.RuntimeOperationFeasibilityObservation {
					t.Helper()
					response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
						fileevents.ProbeRuntimeOperationFeasibilityCommand(request)))
					value, err := response.Get()
					if err != nil {
						t.Fatal(err)
					}
					return runtimecontract.RuntimeOperationFeasibilityObservation(value.(fileevents.ProbeRuntimeOperationFeasibilityResult))
				}
				got := probe()
				if got.Outcome != tc.want {
					t.Fatalf("outcome=%+v want=%s", got, tc.want)
				}
				if tc.want == runtimecontract.RuntimeOwnerFeasibilityCandidateConstraintViolated {
					v := got.CandidateConstraintViolation
					if v == nil || v.ParameterName != "oldText" || v.RequiredShape != runtimecontract.RuntimeOperationCandidateRequiredShapeTextReplaceApplicableToObservedTarget ||
						v.TargetRef != targetRef || v.ObservedRevision != manifest.Revision || got.Feedback != nil || got.Stale != nil || got.PrerequisiteRequest != nil || got.ReplayProof != nil {
						t.Fatalf("candidate-local mismatch was not an exact bounded owner fact: %+v", got)
					}
				} else if got.CandidateConstraintViolation != nil {
					t.Fatalf("resource/valid fact acquired correction authority: %+v", got)
				}
				if replay := probe(); !reflect.DeepEqual(got, replay) {
					t.Fatalf("read-only duplicate changed observation: %+v / %+v", got, replay)
				}
				after, err := adapter.AcquireWorkspaceManifest(workspace)
				if err != nil || after.Revision != manifest.Revision {
					t.Fatalf("probe changed tree: %+v err=%v", after, err)
				}
				content, err := os.ReadFile(target)
				if err != nil || string(content) != tc.current {
					t.Fatalf("probe changed content: %q err=%v", content, err)
				}
			})
		}
	}
}
