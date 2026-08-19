package common

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = "blocks/approval"

func RuntimeCapabilityKinds() []string {
	return []string{runtimecontract.RuntimeBlockKindApprovalRequest}
}
