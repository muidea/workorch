package events

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSoulBindingsFromMapAreStableAndRoundTrip(t *testing.T) {
	input := map[string]common.SoulID{"validation": "soul-validation", "execution": "soul-execution"}
	bindings := SoulBindingsFromSoulIDMap(input)
	want := []SoulBinding{{Stage: "execution", SoulID: "soul-execution"}, {Stage: "validation", SoulID: "soul-validation"}}
	if !reflect.DeepEqual(bindings, want) {
		t.Fatalf("bindings = %+v, want stable %+v", bindings, want)
	}
	input["execution"] = "mutated"
	if got := SoulIDMapFromBindings(bindings); !reflect.DeepEqual(got, map[string]common.SoulID{"execution": "soul-execution", "validation": "soul-validation"}) {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestValidateSoulBindingsRejectsAmbiguousEntries(t *testing.T) {
	tests := []struct {
		name     string
		bindings []SoulBinding
	}{
		{name: "blank stage", bindings: []SoulBinding{{Stage: " ", SoulID: "soul"}}},
		{name: "blank soul", bindings: []SoulBinding{{Stage: "execution"}}},
		{name: "duplicate stage", bindings: []SoulBinding{{Stage: "execution", SoulID: "soul-a"}, {Stage: "execution", SoulID: "soul-b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSoulBindings(tt.bindings); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("ValidateSoulBindings error = %v, want invalid argument", err)
			}
		})
	}
}
