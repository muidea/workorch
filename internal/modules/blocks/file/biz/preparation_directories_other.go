//go:build !linux

package file

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func observePreparationDirectory(path string) fileobservation.Directory {
	return failedPreparationDirectory(path, fmt.Errorf("exact directory permission observation is unsupported on this platform"))
}
