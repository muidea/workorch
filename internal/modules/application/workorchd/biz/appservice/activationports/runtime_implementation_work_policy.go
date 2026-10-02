package activationports

import (
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 非法但显式存在的配置必须保留为 invalid typed declaration，不能变成 nil。
func runtimeImplementationWorkPolicy(input map[string]any) *runtimecontract.RuntimeImplementationWorkPolicy {
	return runtimeprofiledecode.ImplementationWorkPolicy(input)
}
