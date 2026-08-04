package backend

import port "github.com/muidea/workorch/internal/pkg/workorchport"

func canonicalRuntimeStepKind(kind string) string {
	return port.CanonicalRuntimeBlockKind(kind)
}
