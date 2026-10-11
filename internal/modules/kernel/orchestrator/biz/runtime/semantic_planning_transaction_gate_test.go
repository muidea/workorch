package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSemanticPlanningMutationKeepsCompleteCoordinatorGate(t *testing.T) {
	for _, scenario := range []string{"valid", "after_image", "ledger", "preimage", "stale", "save_failure", "unchanged"} {
		t.Run(scenario, func(t *testing.T) {
			previous := stateChangeAggregate(t).CloneRaw()
			repository := &transitionCoordinatorValidatedRepository{transitionCoordinatorRepository: transitionCoordinatorRepository{previous: &previous}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			transaction, err := controller.planning.beginSemanticPlanningMutation(context.Background(), previous.Facts.WorkspaceID, previous.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			// 让测试替身也按持久化边界隔离，不能用共享前像制造假拒绝。
			transaction.aggregate = transaction.aggregate.CloneRaw()
			before := previous.CloneRaw()
			expectedSaveFailure := errors.New("保存故障")
			changed := true
			switch scenario {
			case "after_image":
				transaction.aggregate.Decision.AuthoritySnapshot.RunID = "wrong-run"
			case "ledger":
				transaction.ledger.SchemaVersion = "wrong-schema"
			case "preimage":
				previous.Decision.AuthoritySnapshot.RunID = "wrong-run"
			case "stale":
				previous.StateRevision++
			case "save_failure":
				repository.saveErr = expectedSaveFailure
			case "unchanged":
				changed = false
			}
			result, err := controller.planning.commitSemanticPlanningMutation(context.Background(), transaction, changed)
			switch scenario {
			case "valid":
				if err != nil || repository.validatedSaves != 1 || repository.saves != 0 || result.StateRevision != before.StateRevision+1 || result.Validate() != nil {
					t.Fatalf("完整语义 after-image 未经唯一 coordinator 提交：%v", err)
				}
			case "unchanged":
				if err != nil || repository.validatedSaves != 0 || !reflect.DeepEqual(result, transaction.aggregate) {
					t.Fatal("未变化事务被重复提交或改写")
				}
			case "save_failure":
				if !errors.Is(err, expectedSaveFailure) || repository.validatedSaves != 1 || !reflect.DeepEqual(previous, before) {
					t.Fatal("真实保存错误被吞掉或污染前像")
				}
			default:
				if err == nil || repository.validatedSaves != 0 || repository.saves != 0 {
					t.Fatalf("非法完整事务越过 coordinator 门禁：%v", err)
				}
				if scenario == "stale" && !errors.Is(err, common.ErrConflict) {
					t.Fatalf("stale 前像没有保持 CAS conflict：%v", err)
				}
			}
		})
	}
}
