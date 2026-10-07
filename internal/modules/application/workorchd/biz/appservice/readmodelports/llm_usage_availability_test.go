package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceUsageSupplementalUnprojectableRecordStaysPartial(t *testing.T) {
	for _, record := range []*eventstreammodel.EventRecord{nil, {}} {
		models := workspaceSnapshotModels{Workspace: &workspacemodel.Workspace{ID: "ws-usage-partial"}, LLMUsageLoaded: true}
		models.appendLLMUsageEventRecords(record)
		if !models.LLMUsageLimited {
			t.Fatal("追加查询中的不可投影记录被静默当成完整空窗口")
		}
		got := readmodelprojection.WorkspaceProjection().SnapshotFromModels(models.projectionInput(nil))
		if got.LLMUsageSummary.Availability != "partial" {
			t.Fatalf("summary=%+v", got.LLMUsageSummary)
		}
	}
}

func TestWorkspaceUsageLoadModeReportsUnloadedWithoutPayloadQueries(t *testing.T) {
	for _, test := range []struct {
		name    string
		options workspaceSnapshotLoadOptions
	}{
		{name: "detail", options: workspaceSnapshotDetailLoadOptions()},
		{name: "summary", options: workspaceSnapshotSummaryLoadOptions()},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := common.WorkspaceID("ws-usage-unloaded")
			reader := &recordingWorkspaceEventReader{}
			got, err := workspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
				Workspaces: fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{{ID: id, State: common.WorkspaceStateActive}}},
				Runs:       fakeRunStatusRunReader{}, Results: fakeWorkspaceSnapshotResultReader{},
				Artifacts: fakeRunSnapshotArtifactReader{}, EventRecords: reader,
				Tickets: fakeTicketReader{}, Resolutions: fakeResolutionReader{}, Approvals: fakeApprovalListReader{},
			}, id, nil, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if got.LLMUsageSummary.Availability != "not_loaded" || got.LLMUsageSummary.Scope != "workspace" {
				t.Fatalf("summary=%+v", got.LLMUsageSummary)
			}
			for _, query := range reader.queries {
				if query.Topic == "/llm/usage/recorded" || query.IncludePayload {
					t.Fatalf("普通详情额外加载 usage payload: %+v", query)
				}
			}
		})
	}
}
