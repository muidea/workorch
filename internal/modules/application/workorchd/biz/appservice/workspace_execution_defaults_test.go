package appservice

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceExecutionDefaultsUseLLMOwnerActiveEmbeddingTarget(t *testing.T) {
	ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) {
		return workorchconfig.Config{LLM: workorchconfig.LLMConfig{
			DefaultChat:      workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "hot-chat"},
			DefaultEmbedding: &workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "hot-embedding"},
		}}, nil
	})
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	activeEmbedding := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "active-embedding"}
	observer.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{
			ProviderID: "ai-proxy", ConfigurationRevision: "active-revision",
			DefaultChat:      runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "active-chat"},
			DefaultEmbedding: &activeEmbedding,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration) })

	defaults, err := workspaceExecutionDefaultsFromContext(ctx, hub)
	if err != nil {
		t.Fatal(err)
	}
	target, err := defaults.defaultEmbeddingProviderLoader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !target.Same(activeEmbedding) {
		t.Fatalf("embedding target = %+v, want owner active target %+v", target, activeEmbedding)
	}
}
