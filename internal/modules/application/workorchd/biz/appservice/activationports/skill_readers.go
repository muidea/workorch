package activationports

import (
	"context"
	"errors"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func SelectedSkills(ctx context.Context, hub events.Hub, ids []string, workspaceID string, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) ([]contextregistrycore.Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	type skillResult struct {
		item contextregistrycore.Skill
		err  error
	}
	results := make([]skillResult, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, sid string) {
			defer wg.Done()
			item, err := ownerreadports.ReadSkillByOwnerCommand(ctx, hub, common.SkillID(sid))
			if err == nil {
				results[idx].item = contextregistrycore.MapRepositorySkill(item)
				return
			}
			if !errors.Is(err, common.ErrNotFound) {
				results[idx].err = err
				return
			}
			results[idx].item, results[idx].err = contextregistrycore.ReadSkillHubSkill(ctx, skillHubCatalog, sid, contextregistrycore.SkillQuery{WorkspaceID: workspaceID})
		}(i, id)
	}
	wg.Wait()
	ret := make([]contextregistrycore.Skill, 0, len(ids))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		ret = append(ret, r.item)
	}
	return ret, nil
}

func SkillsForSelection(ctx context.Context, hub events.Hub, query contextregistrycore.SkillQuery, skillHubCatalog contextregistrycore.SkillHubSkillCatalog) ([]contextregistrycore.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := ownerreadports.ListSkillsByOwnerCommand(ctx, hub)
	if err != nil {
		return nil, err
	}
	ret := make([]contextregistrycore.Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, contextregistrycore.MapRepositorySkill(item))
	}
	ret = append(ret, contextregistrycore.ListSkillHubSkillsForQuery(ctx, skillHubCatalog, query)...)
	return ret, nil
}
