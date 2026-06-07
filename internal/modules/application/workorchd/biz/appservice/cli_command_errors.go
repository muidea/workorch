package appservice

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/events"
)

func unsupportedCLICommand(cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	return nil, nil, fmt.Errorf("unsupported CLI command %q", cmd.Command)
}
