package activationports

import (
	"context"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResolveActivationProviderTargetPrecedence(t *testing.T) {
	explicit := runtimecontract.ProviderModelTarget{ProviderID: "explicit-provider", ModelID: "explicit-model"}
	workspaceDefault := runtimecontract.ProviderModelTarget{ProviderID: "workspace-provider", ModelID: "workspace-model"}

	got, err := resolveActivationProviderTarget(context.Background(), nil, &explicit, &workspaceDefault)
	if err != nil {
		t.Fatalf("resolve explicit target: %v", err)
	}
	if !got.Same(explicit) {
		t.Fatalf("explicit target = %+v, want %+v", got, explicit)
	}

	got, err = resolveActivationProviderTarget(context.Background(), nil, nil, &workspaceDefault)
	if err != nil {
		t.Fatalf("resolve workspace target: %v", err)
	}
	if !got.Same(workspaceDefault) {
		t.Fatalf("workspace target = %+v, want %+v", got, workspaceDefault)
	}
}
