package soulbinding

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestFromMapIsStableAndRoundTrips(t *testing.T) {
	input := map[string]common.SoulID{"validation": "soul-validation", "execution": "soul-execution"}
	bindings := FromSoulIDMap(input)
	want := []Binding{{Stage: "execution", SoulID: "soul-execution"}, {Stage: "validation", SoulID: "soul-validation"}}
	if !reflect.DeepEqual(bindings, want) {
		t.Fatalf("bindings = %+v, want stable %+v", bindings, want)
	}
	input["execution"] = "mutated"
	if got := SoulIDMap(bindings); !reflect.DeepEqual(got, map[string]common.SoulID{"execution": "soul-execution", "validation": "soul-validation"}) {
		t.Fatalf("round trip = %+v", got)
	}
}

func TestValidateRejectsAmbiguousEntries(t *testing.T) {
	tests := []struct {
		name     string
		bindings []Binding
	}{
		{name: "blank stage", bindings: []Binding{{Stage: " ", SoulID: "soul"}}},
		{name: "blank soul", bindings: []Binding{{Stage: "execution"}}},
		{name: "duplicate stage", bindings: []Binding{{Stage: "execution", SoulID: "soul-a"}, {Stage: "execution", SoulID: "soul-b"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.bindings); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Validate error = %v, want invalid argument", err)
			}
		})
	}
}
