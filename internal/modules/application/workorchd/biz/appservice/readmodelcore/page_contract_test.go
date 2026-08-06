package readmodelcore_test

import (
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestPageContractsEmbedCursorMetadataWithoutChangingJSONShape(t *testing.T) {
	cursor := projectioncontract.CursorPage{
		Limit:             25,
		CursorAfterID:     "item-1",
		NextCursorAfterID: "item-2",
		HasMore:           true,
	}
	pages := []any{
		readmodelcore.StepPage{CursorPage: cursor},
		readmodelcore.ToolCallPage{CursorPage: cursor},
		readmodelcore.RuntimeCheckpointPage{CursorPage: cursor},
		readmodelcore.ResultPage{CursorPage: cursor},
		readmodelcore.PatchPage{CursorPage: cursor},
		readmodelcore.ValidationResultPage{CursorPage: cursor},
		readmodelcore.ArtifactPage{CursorPage: cursor},
		appmodelcore.DecisionPage{CursorPage: cursor},
		appmodelcore.ApprovalPage{CursorPage: cursor},
	}

	for _, page := range pages {
		raw, err := json.Marshal(page)
		if err != nil {
			t.Fatalf("marshal %T: %v", page, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("unmarshal %T: %v", page, err)
		}
		if fields["limit"] != float64(25) || fields["cursorAfterId"] != "item-1" || fields["nextCursorAfterId"] != "item-2" || fields["hasMore"] != true {
			t.Fatalf("%T cursor fields changed shape: %s", page, raw)
		}
		if _, nested := fields["CursorPage"]; nested {
			t.Fatalf("%T unexpectedly nested cursor metadata: %s", page, raw)
		}
	}
}
