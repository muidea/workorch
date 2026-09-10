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

func TestFileOwnerFeasibilityEnforcesSealedInputExclusionsWithoutEffect(t *testing.T) {
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
	root := t.TempDir()
	// File only knows resource identities; it does not classify test filenames.
	for _, ref := range []string{"input.fixture", "implementation"} {
		if err := os.WriteFile(filepath.Join(root, ref), []byte("before"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := owner.runtimeAdapter.AcquireRuntimeOperationAdmissibility(root, "catalog", []string{runtimecontract.StepKindFileEdit}, nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"input.fixture", "implementation"} {
		for range 2 {
			authorization := fileCandidateConformanceAuthorization(t, runtimecontract.StepKindFileEdit, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, "implementation")
			authorization.Target.OwnerScopeTargetRefs = append(authorization.Target.OwnerScopeTargetRefs, "input.fixture")
			authorization.Target.ExcludedTargetRefs = []string{"input.fixture"}
			authorization.AdmissibilitySnapshotRefs = []string{catalog.SnapshotID}
			authorization.RequireTargetRefs, authorization.TargetCardinality = true, runtimecontract.RuntimeOperationTargetCardinalityOne
			for _, source := range catalog.TargetParameterBindings {
				binding := runtimecontract.RuntimeTargetParameterBinding{Kind: source.Kind, Name: source.Name}
				for _, value := range source.Values {
					if value.TargetRef == "implementation" {
						binding.Values = append(binding.Values, value)
					}
				}
				authorization.TargetParameterBindings = append(authorization.TargetParameterBindings, binding)
			}
			before, after := "before", "after"
			operation := runtimecontract.RuntimeOperation{ID: "edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{target}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}}}
			preimages, err := runtimecontract.RuntimeCandidateTargetParameters(authorization, []string{"implementation"})
			if err != nil {
				t.Fatal(err)
			}
			operation.Parameters = append(operation.Parameters, preimages...)
			command := fileevents.ProbeRuntimeOperationFeasibilityCommand{WorkspaceID: "ws", RunID: "run", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority"}, WorkspaceDeliverableDirectory: root, Operation: operation, OperationAuthorization: authorization, Admissibility: catalog, CandidateManifestHash: "manifest"}
			result := hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
			data, resultErr := result.Get()
			if target == "input.fixture" {
				if resultErr == nil || !strings.Contains(resultErr.Error(), "target_outside_sealed_set") {
					t.Fatalf("input exclusion not enforced at target admission: data=%+v error=%v", data, resultErr)
				}
			} else if resultErr != nil {
				t.Fatal(resultErr)
			} else if observation, ok := data.(fileevents.ProbeRuntimeOperationFeasibilityResult); !ok || observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible {
				t.Fatalf("ordinary implementation blocked: %+v", data)
			}
			content, err := os.ReadFile(filepath.Join(root, target))
			if err != nil || string(content) != before {
				t.Fatal("feasibility changed a durable file")
			}
		}
	}
}
