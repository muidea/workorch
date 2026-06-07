package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func SelectedRuntimeCapabilities(executionContext map[string]any, catalog map[string]readmodelcore.RuntimeCapability) []readmodelcore.RuntimeCapability {
	if len(executionContext) == 0 {
		return nil
	}
	ret := make([]readmodelcore.RuntimeCapability, 0)
	seen := map[string]struct{}{}
	ret = appendSelectedRuntimeCapabilitiesFromContext(ret, seen, executionContext)
	ret = appendSelectedRuntimeCapabilitiesFromIDs(ret, seen, executionContext, catalog)
	if len(ret) == 0 {
		return nil
	}
	return ret
}
