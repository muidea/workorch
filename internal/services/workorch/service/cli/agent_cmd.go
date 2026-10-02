package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
)

var agentCmd = &cobra.Command{
	Use:    "agent [goal]",
	Short:  "已退休：请通过 Web 创作规格后显式提交执行",
	Hidden: true,
	RunE:   runAgent,
}

func init() {
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("%w: Goal-first agent 执行已退休，请通过 Web 完成规格创作、审查与确认，再显式提交工作空间执行意图", common.ErrConflict)
}
