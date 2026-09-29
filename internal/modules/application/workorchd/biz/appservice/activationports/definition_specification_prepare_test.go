package activationports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
)

func TestPrepareDefinitionSpecificationRejectsIncompleteScope(t *testing.T) {
	hub := magicevent.NewHub(8)
	defer hub.Terminate(context.Background())
	if _, err := PrepareDefinitionSpecification(nil, hub, "ws", nil, nil); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := PrepareDefinitionSpecification(context.Background(), nil, "ws", nil, nil); err == nil {
		t.Fatal("nil EventHub accepted")
	}
	if _, err := PrepareDefinitionSpecification(context.Background(), hub, "", nil, nil); err == nil {
		t.Fatal("missing workspace scope accepted")
	}
}
