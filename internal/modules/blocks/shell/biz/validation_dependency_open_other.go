//go:build !unix

package shell

import (
	"context"
	"fmt"
	"os"
)

// The versioned currentness query has no safe descriptor-based implementation
// on this platform yet. Do not silently weaken it to a check-then-follow open.
// Original execution receipts remain readable; no current observation is made.
func openShellDependencySelector(ctx context.Context, _ *os.Root, _ string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("Shell current dependency query requires supported no-follow descriptor reads")
}
