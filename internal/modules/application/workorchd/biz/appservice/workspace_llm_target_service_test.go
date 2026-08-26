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

func TestUpdateWorkspaceLLMSelectionRejectsUnavailableTargetBeforeOwnerWrite(t *testing.T) {
	hub := magicevent.NewHub(16)
	llmObserver := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	llmObserver.Subscribe(llmevents.TopicLLMCommandValidateInferenceSelection, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "provider target unavailable", Cause: common.ErrNotFound})
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicLLMCommandValidateInferenceSelection) })

	ownerWrites := 0
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection, func(_ magicevent.Event, result magicevent.Result) {
		ownerWrites++
		result.Set(workspaceevents.WorkspaceLLMSelectionResult{}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandUpdateLLMSelection) })

	target := runtimecontract.ProviderModelTarget{ProviderID: "missing-provider", ModelID: "missing-model"}
	selection := runtimecontract.ProviderInferenceSelection{Target: target, ReasoningEffort: "high"}
	_, err := UpdateWorkspaceLLMSelectionWithHub(context.Background(), hub, "ws-unavailable", &selection, 1)
	if err == nil || !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update error = %v, want not found readiness failure", err)
	}
	if ownerWrites != 0 {
		t.Fatalf("owner writes = %d, want 0", ownerWrites)
	}
}
