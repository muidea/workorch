package eventmapping

import (
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestWorkspaceResultMappingIsStableDetachedAndRoundTrips(t *testing.T) {
	hints, err := workorcheventpayload.FromMap(map[string]any{"mode": "strict"})
	if err != nil {
		t.Fatal(err)
	}
	model := &workspacemodel.Workspace{
		ID: "ws-mapping", SoulBindings: map[string]common.SoulID{"validation": "soul-validation", "execution": "soul-execution"},
		ContextHints: hints, EvidencePolicies: []workorcheventpayload.Payload{hints},
	}
	dto := WorkspaceDTO(model)
	if dto == nil || len(dto.SoulBindings) != 2 || dto.SoulBindings[0].Stage != "execution" || dto.SoulBindings[1].Stage != "validation" {
		t.Fatalf("stable bindings = %+v", dto)
	}
	dto.SoulBindings[0].SoulID = "mutated"
	dto.EvidencePolicies[0] = workorcheventpayload.Payload{}
	if model.SoulBindings["execution"] != "soul-execution" || model.ContextHints.Map()["mode"] != "strict" {
		t.Fatalf("DTO mutation leaked to model: %+v %+v", model.SoulBindings, model.ContextHints)
	}
	dto = WorkspaceDTO(model)
	roundTrip := Workspace(dto)
	if !reflect.DeepEqual(roundTrip.SoulBindings, model.SoulBindings) || !reflect.DeepEqual(roundTrip.ContextHints.Map(), model.ContextHints.Map()) {
		t.Fatalf("round trip = %+v, want %+v", roundTrip, model)
	}
}
