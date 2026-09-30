//go:build !linux

package file

import (
	"fmt"
	"os"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Without an atomic confined, no-cross-mount opener, creation-time resource
// content cannot be sent to a provider on this platform.
func openAuthoringResource(_, _ string) (*os.File, error) {
	return nil, fmt.Errorf("%w: authoring resource reads are unsupported on this platform", common.ErrInvalidArgument)
}

func statAuthoringResource(_, _ string) (os.FileInfo, error) {
	return nil, fmt.Errorf("%w: authoring resource reads are unsupported on this platform", common.ErrInvalidArgument)
}

func sameAuthoringFileVersion(_, _ os.FileInfo) bool { return false }
