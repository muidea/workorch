package appservice

import (
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
)

func unsupportedCLICommand(cmd clicore.Command) (map[string]any, []map[string]any, error) {
	return nil, nil, fmt.Errorf("unsupported CLI command %q", cmd.Command)
}
