package configcore

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLLMCatalogCommandsRequireTopicSpecificResult(t *testing.T) {
	tests := []struct {
		name     string
		topic    string
		response any
		call     func(context.Context, magicevent.Hub) error
	}{
		{
			name: "get rejects refresh result", topic: llmevents.TopicLLMCommandGetProviderModelCatalog,
			response: llmevents.RefreshProviderModelCatalogResult{},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := GetLLMModelCatalog(ctx, hub, GetLLMModelCatalogCommand{})
				return err
			},
		},
		{
			name: "refresh rejects get result", topic: llmevents.TopicLLMCommandRefreshProviderModelCatalog,
			response: llmevents.GetProviderModelCatalogResult{},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := RefreshLLMModelCatalog(ctx, hub, RefreshLLMModelCatalogCommand{})
				return err
			},
		},
		{
			name: "get rejects bare catalog", topic: llmevents.TopicLLMCommandGetProviderModelCatalog,
			response: llmmodels.ProviderModelCatalog{},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := GetLLMModelCatalog(ctx, hub, GetLLMModelCatalogCommand{})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			observer.Subscribe(tt.topic, func(_ magicevent.Event, result magicevent.Result) { result.Set(tt.response, nil) })
			t.Cleanup(func() { observer.Unsubscribe(tt.topic) })
			if err := tt.call(ctx, hub); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want exact result contract defect", err)
			}
		})
	}
}
