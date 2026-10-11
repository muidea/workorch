package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 修改前的完整路径保留为独立 oracle，不使用新 focus-only 投影。
func referenceCWIFocusBindings(facts RuntimeFacts) error {
	facts = facts.Normalized()
	for _, focus := range runtimeCWIFocusQueue(facts) {
		focus = focus.Normalized()
		if len(focus.CoverageEvidenceBindings) == 0 {
			continue
		}
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, facts.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime CWI focus %s: %w", focus.ID, err)
		}
	}
	return nil
}

func cwiFocusBindingsError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCWIFocusBindingsProjectionPreservesOriginalValidationAndInput(t *testing.T) {
	for _, scenario := range []string{"bound", "unbound", "raw-trim", "remaining", "completed", "nil-queues", "empty-queues", "duplicate-queue-id", "empty-queue-id", "remaining-bad-binding", "completed-bad-binding", "first-rejection", "bad-binding", "bad-hash", "definition", "irrelevant-resource"} {
		t.Run(scenario, func(t *testing.T) {
			facts := runtimeCWILedgerFacts(t)
			switch scenario {
			case "unbound":
				facts.Focus.CoverageEvidenceBindings = nil
			case "raw-trim":
				facts.DefinitionRevision = " " + facts.DefinitionRevision + " "
				facts.Focus.ID = " " + facts.Focus.ID + " "
			case "remaining":
				facts.RemainingFocuses = []CoverageFocus{facts.Focus.Normalized()}
			case "completed":
				facts.CompletedFocuses = []CoverageFocus{facts.Focus.Normalized()}
			case "nil-queues":
				facts.RemainingFocuses, facts.CompletedFocuses = nil, nil
			case "empty-queues":
				facts.RemainingFocuses, facts.CompletedFocuses = []CoverageFocus{}, []CoverageFocus{}
			case "duplicate-queue-id":
				first, second := facts.Focus.Normalized(), facts.Focus.Normalized()
				second.CoverageEvidenceBindings[0].Hash = "bad"
				facts.RemainingFocuses = []CoverageFocus{first, second}
			case "remaining-bad-binding", "completed-bad-binding":
				focus := facts.Focus.Normalized()
				focus.CoverageEvidenceBindings[0].Hash = "bad"
				if scenario == "remaining-bad-binding" {
					facts.RemainingFocuses = []CoverageFocus{focus}
				} else {
					facts.CompletedFocuses = []CoverageFocus{focus}
				}
			case "first-rejection":
				focus := facts.Focus.Normalized()
				focus.CoverageEvidenceBindings[0].Hash = "bad"
				facts.RemainingFocuses = []CoverageFocus{focus}
				facts.Focus.CoverageEvidenceBindings[0].CoverageWorkItemID = "other"
			case "empty-queue-id":
				focus := facts.Focus.Normalized()
				focus.ID = " "
				focus.CoverageEvidenceBindings[0].Hash = "bad"
				facts.RemainingFocuses = []CoverageFocus{focus}
			case "bad-binding":
				facts.Focus.CoverageEvidenceBindings[0].CoverageWorkItemID = "other"
			case "bad-hash":
				facts.Focus.CoverageEvidenceBindings[0].Hash = "bad"
			case "definition":
				facts.DefinitionRevision = "other"
			case "irrelevant-resource":
				facts.Resources.OperationAdmissibilities = []RuntimeOperationAdmissibility{{SnapshotID: "bad", ObservedRevision: strings.Repeat("原诊断", 4096)}}
			}
			before, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := cwiFocusBindingsError(ValidateRuntimeCWIFocusBindings(facts)), cwiFocusBindingsError(referenceCWIFocusBindings(facts)); got != want {
				t.Fatalf("validation differs: %q != %q", got, want)
			}
			after, err := json.Marshal(facts)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("read-only validation changed original facts")
			}
			var reloaded RuntimeFacts
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			if got, want := cwiFocusBindingsError(ValidateRuntimeCWIFocusBindings(reloaded)), cwiFocusBindingsError(referenceCWIFocusBindings(reloaded)); got != want {
				t.Fatal("reload validation differs")
			}
			facts.Focus.CoverageEvidenceBindings = []CoverageEvidenceBinding{{ID: "later-invalid", Hash: "bad"}}
			if ValidateRuntimeCWIFocusBindings(facts) == nil {
				t.Fatal("later call reused prior success")
			}
		})
	}
}

func TestCWIFocusBindingsProjectionAvoidsUnrelatedFactAllocations(t *testing.T) {
	facts := runtimeCWILedgerFacts(t)
	facts.Resources.OperationAdmissibilities = make([]RuntimeOperationAdmissibility, 48)
	for index := range facts.Resources.OperationAdmissibilities {
		facts.Resources.OperationAdmissibilities[index] = RuntimeOperationAdmissibility{SnapshotID: fmt.Sprintf("unrelated-%d", index), TargetRefs: []string{"file-a", "file-b"}, ObservedRevision: strings.Repeat("原文", 128)}
	}
	old := testing.AllocsPerRun(10, func() { _ = referenceCWIFocusBindings(facts) })
	current := testing.AllocsPerRun(10, func() { _ = ValidateRuntimeCWIFocusBindings(facts) })
	if current >= old {
		t.Fatalf("focus-only validation did not reduce allocations: %f >= %f", current, old)
	}
}

func FuzzCWIFocusBindingsProjectionMatchesOriginal(f *testing.F) {
	for _, value := range []string{"", " ", "focus", "\x00\xff原文"} {
		f.Add(value, value, uint8(0))
	}
	f.Fuzz(func(t *testing.T, id, revision string, mode uint8) {
		facts := runtimeCWILedgerFacts(t)
		facts.DefinitionRevision = revision
		focus := facts.Focus.Normalized()
		focus.ID = id
		switch mode % 4 {
		case 0:
			facts.Focus = focus
		case 1:
			facts.RemainingFocuses = []CoverageFocus{focus}
		case 2:
			facts.CompletedFocuses = []CoverageFocus{focus}
		case 3:
			facts.Focus.CoverageEvidenceBindings[0].Hash = id
		}
		if got, want := cwiFocusBindingsError(ValidateRuntimeCWIFocusBindings(facts)), cwiFocusBindingsError(referenceCWIFocusBindings(facts)); got != want {
			t.Fatalf("validation differs: %q != %q", got, want)
		}
	})
}
