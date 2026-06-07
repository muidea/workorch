package setupdraftcore

import soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"

func SoulFromRepository(item *soulmodel.Soul) Soul {
	if item == nil {
		return Soul{}
	}
	return Soul{
		ID:          string(item.ID),
		Name:        item.Name,
		Description: item.Description,
		SoulMd:      item.SoulMD,
	}
}
