package workspaceviewcore

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// BenchmarkStoreUpsertSnapshotAndRead freezes the allocation-sensitive
// transaction/read path around the single Store lock and revision authority.
func BenchmarkStoreUpsertSnapshotAndRead(b *testing.B) {
	store := NewStoreWithRevisionFloor(1)
	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-benchmark", Title: "benchmark"},
		TaskProgress: projectioncontract.TaskProgress{
			Runtime: &projectioncontract.RuntimeProgress{ConvergenceState: "need_state_change"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		snapshot.TaskProgress.Runtime.StateRevision = uint64(index + 1)
		store.UpsertSnapshot(snapshot)
		if _, _, err := store.GetSnapshot("ws-benchmark"); err != nil {
			b.Fatal(err)
		}
	}
}
