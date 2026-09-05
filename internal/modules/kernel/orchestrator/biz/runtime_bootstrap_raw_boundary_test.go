package orchestrator

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func orchestratorInvalidRawBootstrap() runtimecontract.RuntimeBootstrap {
	policy := runtimecontract.RuntimeEvidencePolicy{
		ID: "policy-1", AppliesTo: runtimecontract.StepKindFileWrite,
		CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
			ID: "dod-1", Scope: "dodItem", TargetRefs: []string{" target.go "},
		}},
	}
	return runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{policy}}}
}

func TestInitializeRuntimeFromRunRejectsRawBootstrapBeforeClone(t *testing.T) {
	bootstrap := orchestratorInvalidRawBootstrap()
	owner := &Biz{runtimeController: &runtime.RuntimeController{}}
	_, err := owner.initializeRuntimeFromRun(context.Background(), &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", RuntimeBootstrap: &bootstrap,
	})
	if err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("Orchestrator initialization accepted noncanonical bootstrap: %v", err)
	}
	if got := bootstrap.Activation.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0]; got != " target.go " {
		t.Fatalf("Orchestrator initialization normalized rejected bootstrap in place: %q", got)
	}
}

func TestRunSnapshotReaderRejectsRawBootstrapFromOwnerResponse(t *testing.T) {
	bootstrap := orchestratorInvalidRawBootstrap()
	rawHub := magicevent.NewHub(8)
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	observer.Subscribe(runevents.TopicRunCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(runevents.GetRunResult{Run: &runmodel.Run{
			ID: "run-1", WorkspaceID: "ws-1", RuntimeBootstrap: &bootstrap,
		}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandGet) })

	_, err := (runSnapshotReader{hub: rawHub}).Read(context.Background(), "ws-1", "run-1")
	if err == nil || !strings.Contains(err.Error(), "evidence polic") {
		t.Fatalf("Run snapshot reader accepted noncanonical bootstrap: %v", err)
	}
}
