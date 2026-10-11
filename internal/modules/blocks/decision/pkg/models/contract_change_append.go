package decision

import (
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 仓库只接受 owner reducer 已生成的单版本追加；旧历史与来源不可覆盖。
func ValidateContractChangeAppend(current, next *Ticket) error {
	if current == nil || next == nil || !current.IsContractChange() || !next.IsContractChange() ||
		current.ValidateContractChange() != nil || next.ValidateContractChange() != nil ||
		len(next.ContractChangeRevisions) != len(current.ContractChangeRevisions)+1 ||
		!reflect.DeepEqual(current.ContractChangeRevisions, next.ContractChangeRevisions[:len(current.ContractChangeRevisions)]) {
		return common.ErrConflict
	}
	before, after := CloneTicket(current), CloneTicket(next)
	before.ContractChangeRevisions, after.ContractChangeRevisions = nil, nil
	before.UpdatedAt, after.UpdatedAt = "", ""
	if !reflect.DeepEqual(before, after) {
		return common.ErrConflict
	}
	return nil
}
