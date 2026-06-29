package backend

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

func assertTypedSnapshotInfo(v any) (*WorkspaceSnapshotInfo, error) {
	switch typed := v.(type) {
	case nil:
		return nil, nil
	case WorkspaceSnapshotInfo:
		return &typed, nil
	case *WorkspaceSnapshotInfo:
		return typed, nil
	default:
		return nil, fmt.Errorf("%w: workspace snapshot requires typed DTO, got %T", common.ErrInvalidArgument, v)
	}
}
