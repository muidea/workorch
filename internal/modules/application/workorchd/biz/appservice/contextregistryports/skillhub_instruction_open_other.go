//go:build !linux

package contextregistryports

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"os"
)

// No unreviewed platform fallback may drop nonblocking regular-file fencing.
func openPreparationInstructionFile(_ *os.Root, _ string) (*os.File, error) {
	return nil, common.ErrConflict
}
