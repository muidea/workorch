package file

import (
	"reflect"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
)

// 磁盘序列化不能掩盖仓库 helper 在写入前仍共享 owner binding 的问题。
func TestDecisionTicketCloneDetachesClarificationAndActions(t *testing.T) {
	original := &decisionmodel.Ticket{
		Reason: "原因\r\n原文\x00", AllowedRecoveryActions: []string{"revise_definition_and_restart"},
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{
			OpenBindingHash: "original-binding", OffendingRefs: []string{"原始引用"},
		},
	}
	before := decisionmodel.CloneTicket(original)
	cloned := cloneDecisionTicket(original)
	if !reflect.DeepEqual(cloned, before) {
		t.Fatal("仓库 helper 改变 Ticket 原文或来源")
	}
	cloned.AllowedRecoveryActions[0] = "caller-action"
	cloned.DefinitionClarification.OpenBindingHash = "caller-binding"
	cloned.DefinitionClarification.OffendingRefs[0] = "caller-ref"
	if !reflect.DeepEqual(original, before) {
		t.Fatal("仓库 helper 共享可变的 Ticket authority 来源")
	}
}
