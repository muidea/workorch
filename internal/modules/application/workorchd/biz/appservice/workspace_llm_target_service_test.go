package appservice

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestUpdateWorkspaceLLMTargetRejectsUnavailableTargetBeforeOwnerWrite(t *testing.T) {
	hub := magicevent.NewHub(16)
	llmObserver := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	llmObserver.Subscribe(llmevents.TopicLLMCommandResolveProviderModelTarget, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "provider target unavailable", Cause: common.ErrNotFound})
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicLLMCommandResolveProviderModelTarget) })

	ownerWrites := 0
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget, func(_ magicevent.Event, result magicevent.Result) {
		ownerWrites++
		result.Set(workspaceevents.WorkspaceCommandResult{}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMTarget) })

	target := runtimecontract.ProviderModelTarget{ProviderID: "missing-provider", ModelID: "missing-model"}
	_, err := UpdateWorkspaceLLMTargetWithHub(context.Background(), hub, "ws-unavailable", &target)
	if err == nil || !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update error = %v, want not found readiness failure", err)
	}
	if ownerWrites != 0 {
		t.Fatalf("owner writes = %d, want 0", ownerWrites)
	}
}
