package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResultEventPurgeRepositoriesUseReadDeleteContractsAndDetachResults(t *testing.T) {
	ctx, err := common.BindRuntimeWorkspaceContext(context.Background(), "ws-result-adapter")
	if err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	resultItem := &resultmodel.Result{ID: "result-adapter", WorkspaceID: "ws-result-adapter", RunID: "run-result-adapter", EvidenceRefs: []string{"evidence-1"}}
	artifactItem := &resultmodel.Artifact{ID: "artifact-adapter", WorkspaceID: "ws-result-adapter", RunID: "run-result-adapter", EvidenceRefs: []string{"artifact-evidence-1"}}

	subscribeResultCommand(t, hub, resultevents.TopicResultCommandListByRun, func(cmd resultevents.ListResultsByRunCommand, response magicevent.Result) {
		if cmd.WorkspaceID != resultItem.WorkspaceID || cmd.RunID != resultItem.RunID || cmd.Limit != resultevents.MaxListLimit {
			t.Fatalf("run id = %s", cmd.RunID)
		}
		response.Set(resultevents.ListResultsByRunResult{Results: []*resultmodel.Result{resultItem}}, nil)
	})
	subscribeResultCommand(t, hub, resultevents.TopicArtifactCommandListByRun, func(cmd resultevents.ListArtifactsByRunCommand, response magicevent.Result) {
		if cmd.WorkspaceID != artifactItem.WorkspaceID || cmd.RunID != artifactItem.RunID || cmd.Limit != resultevents.MaxListLimit {
			t.Fatalf("run id = %s", cmd.RunID)
		}
		response.Set(resultevents.ListArtifactsByRunResult{Artifacts: []*resultmodel.Artifact{artifactItem}}, nil)
	})

	results, err := (resultEventResultRepository{hub: hub}).ListByRun(ctx, resultItem.RunID)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (resultEventArtifactRepository{hub: hub}).ListByRun(ctx, artifactItem.RunID)
	if err != nil {
		t.Fatal(err)
	}
	results[0].EvidenceRefs[0] = "mutated"
	artifacts[0].EvidenceRefs[0] = "mutated"
	if resultItem.EvidenceRefs[0] != "evidence-1" || artifactItem.EvidenceRefs[0] != "artifact-evidence-1" {
		t.Fatal("adapter returned owner response aliases")
	}
}

func subscribeResultCommand[Command any](t *testing.T, hub magicevent.Hub, topic string, handle func(Command, magicevent.Result)) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(Command)
		if !ok {
			t.Fatalf("event %s command type = %T", topic, ev.Data())
		}
		handle(cmd, result)
	})
	t.Cleanup(func() { observer.Unsubscribe(topic) })
}
