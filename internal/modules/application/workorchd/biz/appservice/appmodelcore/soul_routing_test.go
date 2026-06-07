package appmodelcore

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeSoulBindingsRejectsUnknownStageAndDropsEmptyValues(t *testing.T) {
	got, err := NormalizeSoulBindings(map[string]string{
		" execution ": " soul-execution ",
		"planning":    "",
	})
	if err != nil {
		t.Fatalf("NormalizeSoulBindings: %v", err)
	}
	want := map[string]common.SoulID{SoulBindingStageExecution: "soul-execution"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %+v, want %+v", got, want)
	}

	if _, err := NormalizeSoulBindings(map[string]string{"unknown": "soul"}); err != common.ErrInvalidArgument {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestMapSoulBindingIDsUsesStageOrder(t *testing.T) {
	got := MapSoulBindingIDs(map[string]common.SoulID{
		SoulBindingStageArtifact:   "soul-artifact",
		SoulBindingStageExecution:  "soul-execution",
		SoulBindingStageValidation: "soul-validation",
		"unknown":                  "soul-unknown",
	})
	want := map[string]string{
		SoulBindingStageExecution:  "soul-execution",
		SoulBindingStageValidation: "soul-validation",
		SoulBindingStageArtifact:   "soul-artifact",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bindings = %+v, want %+v", got, want)
	}
}

func TestStageSoulBindingPriorityAndUnboundRouting(t *testing.T) {
	soulID, source := StageSoulBinding(
		SoulBindingStageExecution,
		map[string]common.SoulID{SoulBindingStageExecution: "soul-run"},
		map[string]common.SoulID{SoulBindingStageExecution: "soul-workspace"},
		"soul-default",
		"workspace_default",
	)
	if soulID != "soul-run" || source != "run_stage_override" {
		t.Fatalf("stage binding = %s/%s, want run override", soulID, source)
	}

	routing := UnboundSoulRouting("missing")
	if routing.Status != "unbound" || routing.Reason != "missing" || len(routing.Bindings) != 0 {
		t.Fatalf("routing = %+v, want unbound empty binding map", routing)
	}
	if !reflect.DeepEqual(routing.PriorityPolicy, SoulRoutingPriorityPolicy()) {
		t.Fatalf("priority = %+v, want default policy", routing.PriorityPolicy)
	}
}

func TestAppendSoulIDOnce(t *testing.T) {
	got := AppendSoulIDOnce([]common.SoulID{"soul-a"}, "soul-a")
	got = AppendSoulIDOnce(got, "soul-b")
	if !reflect.DeepEqual(got, []common.SoulID{"soul-a", "soul-b"}) {
		t.Fatalf("ids = %+v, want unique append", got)
	}
}
