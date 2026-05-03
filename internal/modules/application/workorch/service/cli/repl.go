package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/service/tui"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Enter interactive REPL mode",
	Long:  "Start an interactive terminal session for managing workorch workspaces, runs, decisions, and approvals.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()

		m := tui.NewAppModel(cl, true)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("repl error: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
}
