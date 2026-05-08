package orchestrator

import "testing"

func TestCompactMemoryContextsMarksLegacyRunSummaryMemory(t *testing.T) {
	items := []map[string]any{
		{
			"id":         "memory-legacy",
			"memoryType": "run_summary",
			"summary":    "legacy summary",
			"tags":       []string{"run_summary", "dod_assessment"},
		},
		{
			"id":         "memory-v2",
			"memoryType": "run_summary",
			"summary":    "v2 summary",
			"tags":       []string{"run_summary", runSummaryDoDGateVersionTag},
		},
	}

	compact := compactMemoryContexts(items)
	if len(compact) != 2 {
		t.Fatalf("compactMemoryContexts len = %d, want 2", len(compact))
	}
	if compact[0]["evidenceTrust"] != "legacy_pre_dod_gate" {
		t.Fatalf("legacy evidenceTrust = %#v, want legacy_pre_dod_gate", compact[0]["evidenceTrust"])
	}
	if _, ok := compact[1]["evidenceTrust"]; ok {
		t.Fatalf("v2 memory evidenceTrust = %#v, want unset", compact[1]["evidenceTrust"])
	}
}
