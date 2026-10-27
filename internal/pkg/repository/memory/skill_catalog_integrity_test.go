package memory

import (
	"context"
	"errors"
	"testing"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSkillListRejectsNilAndKeyIdentityMismatchWithoutPartialRows(t *testing.T) {
	for _, item := range []*skillmodel.Skill{nil, {ID: "different-record-id"}} {
		repo := NewSkillRepository()
		repo.items["a-valid"] = &skillmodel.Skill{ID: "a-valid"}
		repo.items["z-storage-key"] = item
		items, err := repo.List(context.Background())
		if !errors.Is(err, common.ErrConflict) || items != nil {
			t.Fatal("corrupt map became complete or partial catalog")
		}
		if repo.items["z-storage-key"] != item || len(repo.items) != 2 {
			t.Fatal("read rewrote owner state")
		}
	}
}
