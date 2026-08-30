package llm

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

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
	topicCount := len(owner.topics)
	if !owner.started || topicCount == 0 {
		t.Fatalf("owner lifecycle = started:%t topics:%d", owner.started, topicCount)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if len(owner.topics) != topicCount {
		t.Fatalf("second Start changed subscriptions: %d -> %d", topicCount, len(owner.topics))
	}
	owner.Teardown(ctx)
	if owner.started || len(owner.topics) != 0 || owner.catalogService != nil || owner.candidateProvider != nil {
		t.Fatalf("owner teardown retained lifecycle resources: started=%t topics=%d", owner.started, len(owner.topics))
	}
}
