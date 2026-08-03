package events

import (
	"errors"
	"reflect"
	"testing"

	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestProviderRequestTraceReferencesAreStableAndRoundTrip(t *testing.T) {
	input := llmmodel.ProviderRequest{TraceRefs: map[string]string{"templateId": "template-1", "requestId": "request-1"}}
	contract := ProviderRequestFromModel(input)
	want := []TraceReference{{Name: "requestId", Ref: "request-1"}, {Name: "templateId", Ref: "template-1"}}
	if !reflect.DeepEqual(contract.TraceRefs, want) {
		t.Fatalf("TraceRefs = %+v, want stable %+v", contract.TraceRefs, want)
	}
	input.TraceRefs["requestId"] = "mutated"
	model, err := contract.ToModel()
	if err != nil {
		t.Fatal(err)
	}
	if model.TraceRefs["requestId"] != "request-1" {
		t.Fatalf("round trip TraceRefs = %+v", model.TraceRefs)
	}
}

func TestProviderRequestRejectsAmbiguousTraceReferences(t *testing.T) {
	for _, refs := range [][]TraceReference{
		{{Name: "", Ref: "ref"}},
		{{Name: "requestId", Ref: ""}},
		{{Name: "requestId", Ref: "one"}, {Name: "requestId", Ref: "two"}},
	} {
		if _, err := (ProviderRequest{TraceRefs: refs}).ToModel(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("Model error = %v for %+v, want invalid argument", err, refs)
		}
	}
}
