package events

import (
	"errors"
	"reflect"
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEnvironmentReferencesAreStableAndReferenceOnly(t *testing.T) {
	entries := EnvironmentReferencesFromMap(map[string]string{
		"Z_TOKEN": "env://Z_TOKEN",
		"A_ROOT":  "{workspaceDeliverableDirectory}",
	})
	want := []EnvironmentReference{
		{Name: "A_ROOT", Reference: "{workspaceDeliverableDirectory}"},
		{Name: "Z_TOKEN", Reference: "env://Z_TOKEN"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
	if _, err := EnvironmentReferenceMap([]EnvironmentReference{{Name: "TOKEN", Reference: "secret"}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("plaintext err = %v, want ErrInvalidArgument", err)
	}
	if _, err := EnvironmentReferenceMap([]EnvironmentReference{{Name: "TOKEN", Reference: "env://TOKEN"}, {Name: "TOKEN", Reference: "env://OTHER"}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("duplicate err = %v, want ErrInvalidArgument", err)
	}
	if _, err := EnvironmentReferenceMap([]EnvironmentReference{{Name: " ", Reference: "env://TOKEN"}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty name err = %v, want ErrInvalidArgument", err)
	}
	for _, reference := range []string{"env://1TOKEN", "{workspaceManagedConfig:../secret}", `{workspaceManagedConfig:dir\secret}`} {
		if ValidEnvironmentReference(reference) {
			t.Fatalf("reference %q unexpectedly valid", reference)
		}
	}
}

func TestMCPServerEventRoundTripDoesNotShareMutableFields(t *testing.T) {
	model := &mcpmodel.Server{
		ID: "server-1", Args: []string{"serve"}, Env: map[string]string{"TOKEN": "env://TOKEN"},
		AllowedTools: []string{"repo.read"}, AllowedResources: []string{"workspace://root"},
	}
	dto, err := MCPServerFromModel(model)
	if err != nil {
		t.Fatalf("MCPServerFromModel: %v", err)
	}
	model.Args[0] = "changed"
	model.Env["TOKEN"] = "env://CHANGED"
	model.AllowedTools[0] = "changed"
	if dto.Args[0] != "serve" || dto.EnvironmentRefs[0].Reference != "env://TOKEN" || dto.AllowedTools[0] != "repo.read" {
		t.Fatalf("event DTO shares model state: %#v", dto)
	}

	roundTrip, err := dto.Model()
	if err != nil {
		t.Fatalf("Model: %v", err)
	}
	dto.Args[0] = "dto-changed"
	dto.EnvironmentRefs[0].Reference = "env://DTO_CHANGED"
	dto.AllowedResources[0] = "dto-changed"
	if roundTrip.Args[0] != "serve" || roundTrip.Env["TOKEN"] != "env://TOKEN" || roundTrip.AllowedResources[0] != "workspace://root" {
		t.Fatalf("model shares event DTO state: %#v", roundTrip)
	}
}

func TestMCPCapabilityResultCloneDoesNotShareMutableFields(t *testing.T) {
	model := &mcpmodel.Capability{ID: "cap-1", Tags: []string{"owner"}, StageHints: []string{"planning"}}
	dto := mcpmodel.CloneCapability(model)
	model.Tags[0], model.StageHints[0] = "mutated", "mutated"
	if dto.Tags[0] != "owner" || dto.StageHints[0] != "planning" {
		t.Fatal("capability result shares owner model fields")
	}
	dto.Tags[0] = "dto-mutated"
	if model.Tags[0] != "mutated" {
		t.Fatal("owner model shares capability result fields")
	}
}
