package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type DoD struct {
	ID                     common.DoDID
	WorkspaceID            common.WorkspaceID
	Items                  []string
	State                  common.DoDState
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousDoDID          common.DoDID
	LineageState           string
}

func CloneDoD(item *DoD) *DoD {
	if item == nil {
		return nil
	}
	ret := *item
	ret.Items = append([]string(nil), item.Items...)
	return &ret
}

func CloneDoDs(items []*DoD) []*DoD {
	if items == nil {
		return nil
	}
	ret := make([]*DoD, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneDoD(item))
	}
	return ret
}
