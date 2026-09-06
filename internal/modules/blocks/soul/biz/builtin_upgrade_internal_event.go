package soul

import (
	"context"
	"strings"

	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// One owner-local guarded replacement, not a multi-owner upgrade state machine.
type soulBuiltinUpgradeEvent struct {
	SoulID              common.SoulID
	ExpectedContentHash string
	TargetContentHash   string
}

func (s *service) submitBuiltinDefaultUpgrade(ctx context.Context, event soulBuiltinUpgradeEvent) (soulevents.UpgradeBuiltinSoulDefaultResult, error) {
	if s == nil || validateSoulID(event.SoulID) != nil || event.ExpectedContentHash == "" || event.TargetContentHash == "" ||
		event.ExpectedContentHash != strings.TrimSpace(event.ExpectedContentHash) || event.TargetContentHash != strings.TrimSpace(event.TargetContentHash) {
		return soulevents.UpgradeBuiltinSoulDefaultResult{}, common.ErrInvalidArgument
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.upgradeBuiltinDefault(ctx, event)
}
