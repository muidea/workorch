package llm

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type llmPanicDataEvent struct{ magicevent.Event }

func (llmPanicDataEvent) Data() any {
	panic("LLM generic ingress inspected payload before its source gate")
}

func TestLLMOwnerStartIsIdempotentAndTeardownClearsLifecycle(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = "http://127.0.0.1:8090"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "test-model"}
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	topicCount := len(owner.subscriptions)
	if !owner.started || topicCount == 0 {
		t.Fatalf("owner lifecycle = started:%t topics:%d", owner.started, topicCount)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if len(owner.subscriptions) != topicCount {
		t.Fatalf("second Start changed subscriptions: %d -> %d", topicCount, len(owner.subscriptions))
	}
	owner.Teardown(ctx)
	if owner.started || len(owner.subscriptions) != 0 || owner.catalogService != nil || owner.candidateProvider != nil {
		t.Fatalf("owner teardown retained lifecycle resources: started=%t topics=%d", owner.started, len(owner.subscriptions))
	}
}

func TestLLMGenericCommandMatrixRequiresExactApplicationSourceBeforePayload(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = "http://127.0.0.1:8090"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "test-model"}
	owner, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	topics := []struct {
		topic         string
		allowedSource string
	}{
		{llmevents.TopicLLMCommandCompleteProvider, llmSetupDraftAppserviceSource},
		{llmevents.TopicLLMCommandGenerateEmbedding, llmApplicationOwnerSource},
		{llmevents.TopicLLMCommandGetActiveConfiguration, llmApplicationOwnerSource},
		{llmevents.TopicLLMCommandGetProviderModelCatalog, llmApplicationOwnerSource},
		{llmevents.TopicLLMCommandRefreshProviderModelCatalog, llmApplicationOwnerSource},
		{llmevents.TopicLLMCommandValidateInferenceSelection, llmApplicationOwnerSource},
		{llmevents.TopicObserveProviderCatalogCache, llmApplicationOwnerSource},
	}
	for _, item := range topics {
		t.Run(item.topic, func(t *testing.T) {
			for _, source := range []string{"blocks/file", " " + item.allowedSource + " ", ""} {
				result := hub.Send(llmPanicDataEvent{Event: magicevent.NewEventWithContext(
					item.topic, source, llmevents.CommandDestination, magicevent.NewHeader(), ctx, nil,
				)})
				if result == nil || result.Error() == nil || !strings.Contains(result.Error().Error(), "exact Application source") {
					t.Fatalf("source %q result = %#v, want exact Application source rejection", source, result)
				}
			}
		})
	}
}
