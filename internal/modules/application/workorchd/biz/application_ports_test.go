package biz

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
)

func TestApplicationPortsFromSessionIsComplete(t *testing.T) {
	component := newRuntimeComponent(&runtimecore.Session{})
	ports := component.applicationPorts()
	if err := ports.Validate(); err != nil {
		t.Fatalf("application ports validation failed: %v", err)
	}
}
