package cli

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show interactive TUI agent status",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI("")
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
