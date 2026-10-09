package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDriveRepositoryRawPreimageIsolation(t *testing.T) {
	ctx := context.Background()
	original := runtimeAggregate(t, 1)
	// 非规范材料仅用于副本隔离，不作为合法 transition 或 proof 提交。
	original.ProcessedEventIDs = []string{}
	original.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{State: runtimecontract.RuntimeCandidateAttemptStatePending}}
	original.ActiveDispatch = &runtime.SealedDispatch{OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: "original", State: runtimecontract.OperationExecutionSealed}}}
	original.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{Material: runtimecontract.RuntimeTerminalResultMaterial{EvidenceRefs: []string{}, RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{OriginalMessage: "  原始诊断\n  ", TargetRefs: []string{"original"}}}}}
	repository := &runtimeDriveRepository{aggregate: original.CloneRaw()}
	loaded, err := repository.Load(ctx, original.Facts.WorkspaceID, original.Facts.RunID)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(loaded, original) || loaded.ProcessedEventIDs == nil || loaded.TerminalMaterialSeal.Material.EvidenceRefs == nil { t.Fatal("Load 改写原值或 nil/empty 身份") }
	loaded.CandidateAttempts[0].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	loaded.ActiveDispatch.OperationExecutions[0].State = runtimecontract.OperationExecutionOwnerCommitted
	loaded.TerminalMaterialSeal.Material.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "caller"
	loaded.Facts.Coverage[0].TargetText = "caller"
	if !reflect.DeepEqual(repository.aggregate, original) { t.Fatal("Load 副本污染独立持久前像") }
	if err := repository.Save(ctx, loaded, original.StateRevision+1); !errors.Is(err, common.ErrConflict) || !reflect.DeepEqual(repository.aggregate, original) { t.Fatalf("CAS conflict 改写持久前像: %v", err) }
	next := original.CloneRaw()
	next.StateRevision++
	if err := repository.Save(ctx, next, original.StateRevision); err != nil { t.Fatal(err) }
	persisted := repository.aggregate.CloneRaw()
	next.ActiveDispatch.OperationExecutions[0].OperationID = "caller"
	next.TerminalMaterialSeal.Material.RuntimeCloseout.OriginalDiagnostic.OriginalMessage = "摘要"
	if !reflect.DeepEqual(repository.aggregate, persisted) { t.Fatal("Save 调用方改写持久原值") }
}
