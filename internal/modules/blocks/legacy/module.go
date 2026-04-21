package legacy

import (
	legacybiz "github.com/muidea/workorch/internal/modules/blocks/legacy/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	legacyservice "github.com/muidea/workorch/internal/modules/blocks/legacy/service"
)

const ModuleID = modulecommon.ModuleID

type MinimalRuntimeAdapter = legacybiz.MinimalRuntimeAdapter
type MinimalToolAdapter = legacyservice.MinimalToolAdapter

func NewMinimalRuntimeAdapter() *legacybiz.MinimalRuntimeAdapter {
	return legacybiz.NewMinimalRuntimeAdapter()
}

func NewMinimalToolAdapter() *legacyservice.MinimalToolAdapter {
	return legacyservice.NewMinimalToolAdapter()
}
