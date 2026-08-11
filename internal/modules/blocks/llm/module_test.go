package llm

import (
	"context"
	"errors"
	"testing"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestLLMModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	activeConfig := workorchconfig.DefaultLLMConfig()
	activeConfig.AIProxy.BaseURL = "http://127.0.0.1:8090"
	activeConfig.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "test-model"}
	if err := module.Configure(llmbiz.Dependencies{
		EventHub: rawHub,
		LoadLLMConfig: func() (workorchconfig.LLMConfig, error) {
			return activeConfig, nil
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
}

func TestLLMModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(llmbiz.Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}
