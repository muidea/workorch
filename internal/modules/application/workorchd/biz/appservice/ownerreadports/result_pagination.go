package ownerreadports

import "github.com/muidea/workorch/internal/pkg/common"

const maxResultOwnerListPages = 1000

func collectResultOwnerPages[Item any](fetch func(string) ([]Item, string, error)) ([]Item, error) {
	ret := make([]Item, 0)
	cursor := ""
	seen := map[string]struct{}{}
	for page := 0; page < maxResultOwnerListPages; page++ {
		items, next, err := fetch(cursor)
		if err != nil {
			return nil, err
		}
		ret = append(ret, items...)
		if next == "" {
			return ret, nil
		}
		if _, ok := seen[next]; ok {
			return nil, common.ErrConflict
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, common.ErrConflict
}
