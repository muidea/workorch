package events

import (
	"errors"
	"reflect"
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEnvironmentVariablesAreStableAndRejectDuplicates(t *testing.T) {
	entries := EnvironmentVariablesFromMap(map[string]string{"Z_TOKEN": "z", "A_TOKEN": "a"})
	want := []EnvironmentVariable{{Name: "A_TOKEN", Value: "a"}, {Name: "Z_TOKEN", Value: "z"}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
	if _, err := EnvironmentVariableMap([]EnvironmentVariable{{Name: "TOKEN"}, {Name: "TOKEN"}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("duplicate err = %v, want ErrInvalidArgument", err)
	}
	if _, err := EnvironmentVariableMap([]EnvironmentVariable{{Name: " "}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty name err = %v, want ErrInvalidArgument", err)
	}
}

func TestMCPServerEventRoundTripDoesNotShareMutableFields(t *testing.T) {
	model := &mcpmodel.Server{
		ID: "server-1", Args: []string{"serve"}, Env: map[string]string{"TOKEN": "secret"},
		AllowedTools: []string{"repo.read"}, AllowedResources: []string{"workspace://root"},
	}
	dto := MCPServerFromModel(model)
	model.Args[0] = "changed"
	model.Env["TOKEN"] = "changed"
	model.AllowedTools[0] = "changed"
	if dto.Args[0] != "serve" || dto.Env[0].Value != "secret" || dto.AllowedTools[0] != "repo.read" {
		t.Fatalf("event DTO shares model state: %#v", dto)
	}

	roundTrip, err := dto.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	dto.Args[0] = "dto-changed"
	dto.Env[0].Value = "dto-changed"
	dto.AllowedResources[0] = "dto-changed"
	if roundTrip.Args[0] != "serve" || roundTrip.Env["TOKEN"] != "secret" || roundTrip.AllowedResources[0] != "workspace://root" {
		t.Fatalf("model shares event DTO state: %#v", roundTrip)
	}
}
