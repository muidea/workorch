package contracts

import "github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"

type UpdateRuntimePolicyCommand struct {
	Policy workorchruntimepolicy.RuntimePolicy `json:"policy"`
}
