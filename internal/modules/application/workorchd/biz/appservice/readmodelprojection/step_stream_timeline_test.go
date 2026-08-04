package readmodelprojection

import (
	"strings"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestStreamTimelineItemsBoundsEventCountAndDeltaPreview(t *testing.T) {
	events := make([]map[string]any, 0, 40)
	largeDelta := strings.Repeat("x", streamTimelineDeltaRunes+100)
	for idx := 0; idx < 40; idx++ {
		delta := "chunk"
		if idx == 0 {
			delta = largeDelta
		}
		events = append(events, map[string]any{
			"type":  "delta",
			"delta": delta,
		})
	}
	items := streamTimelineItems(projectioncontract.StepView{
		ID:    "step-stream",
		Kind:  "llm/workspace_execution",
		State: "completed",
		Output: map[string]any{
			"streamEvents": events,
		},
	}, "running", "2026-06-21T00:00:00Z")

	want := streamTimelineHeadItemKeep + streamTimelineTailItemKeep + 1
	if len(items) != want {
		t.Fatalf("timeline items = %d, want %d", len(items), want)
	}
	first := items[0]
	if first.Summary == largeDelta {
		t.Fatal("stream delta summary was not truncated")
	}
	if first.Data["deltaTruncated"] != true || first.Data["deltaRuneCount"] != len([]rune(largeDelta)) {
		t.Fatalf("first data = %+v, want truncated delta metadata", first.Data)
	}
	omitted := false
	for _, item := range items {
		if item.Type == "omitted" {
			omitted = true
			if item.Data["omitted"] != 16 || item.Data["total"] != 40 {
				t.Fatalf("omitted data = %+v, want omitted/total counts", item.Data)
			}
		}
	}
	if !omitted {
		t.Fatalf("items = %+v, want omitted marker", items)
	}
}

func TestStepRuntimeTimelineItemOmitsRawStreamEvents(t *testing.T) {
	item := stepRuntimeTimelineItem(projectioncontract.StepView{
		ID:    "step-stream-output",
		Kind:  "llm/workspace_execution",
		State: "completed",
		Output: map[string]any{
			"status": "completed",
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": strings.Repeat("x", 2048)},
			},
		},
	}, "running", "2026-06-21T00:00:00Z")

	if item.Output["streamEvents"] != nil {
		t.Fatalf("output = %+v, want raw streamEvents omitted", item.Output)
	}
	if item.Output["streamEventsOmitted"] != true || item.Output["streamEventCount"] != 1 {
		t.Fatalf("output = %+v, want stream omission summary", item.Output)
	}
	if item.Output["status"] != "completed" {
		t.Fatalf("output = %+v, want non-stream fields preserved", item.Output)
	}
}
