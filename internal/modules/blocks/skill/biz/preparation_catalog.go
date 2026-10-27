package skill

import (
	"context"
	"sort"
	"time"

	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) observeRegistryCatalog(ctx context.Context, command skillevents.ObserveRegistryCatalogCommand) (skillevents.ObserveRegistryCatalogResult, error) {
	if ctx == nil || command.Validate() != nil {
		return skillevents.ObserveRegistryCatalogResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	if _, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID); err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	// Existing memory/file List implementations take one repository read lock
	// and detach the full map. Never stitch pages from separate observations.
	items, err := m.service.skills.List(ctx)
	if err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	if len(items) > skillevents.RegistryCatalogMaxItems {
		return skillevents.ObserveRegistryCatalogResult{}, common.ErrConflict
	}
	// Validate before normalization/clone and reject all malformed/duplicate
	// rows, including those that the eventual selection would not use.
	materialBytes := 0
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return skillevents.ObserveRegistryCatalogResult{}, err
		}
		if skillmodel.ValidateRegistrySnapshotSkill(item) != nil {
			return skillevents.ObserveRegistryCatalogResult{}, common.ErrConflict
		}
		materialBytes += item.Content().ByteSize()
		if materialBytes > skillevents.RegistryCatalogMaxBytes {
			return skillevents.ObserveRegistryCatalogResult{}, common.ErrConflict
		}
	}
	items = skillmodel.CloneSkills(items)
	if items == nil {
		items = []*skillmodel.Skill{}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	hash, err := skillevents.RegistryCatalogHash(items)
	if err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	value := skillevents.ObserveRegistryCatalogResult{Version: skillevents.RegistryCatalogVersion, WorkspaceID: command.WorkspaceID, OwnerEpoch: m.preparationEpoch, ObservedAt: time.Now().UTC(), Skills: items, CatalogHash: hash}
	if err := value.ValidateFor(command); err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return skillevents.ObserveRegistryCatalogResult{}, err
	}
	return value, nil
}
