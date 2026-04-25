package appservice

import "github.com/muidea/workorch/internal/pkg/common"

// Guard partially assembled appservice objects from nil-interface panics.
func validateRequiredDependencies(values ...any) error {
	for _, value := range values {
		if value == nil {
			return common.ErrInvalidArgument
		}
	}
	return nil
}
