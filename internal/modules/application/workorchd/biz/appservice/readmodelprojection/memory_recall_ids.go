package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/common"

func memoryIDsToReadModelStrings(input []common.MemoryID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}
