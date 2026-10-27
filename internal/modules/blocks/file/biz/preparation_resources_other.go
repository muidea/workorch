//go:build !linux

package file

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"os"
)

func preparationResourceIdentity(_ os.FileInfo, _ *fileobservation.Resource) error {
	return common.ErrInvalidArgument
}
