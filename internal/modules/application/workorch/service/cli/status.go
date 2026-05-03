package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/service/tui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show interactive status dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := tui.NewAppModel(cliBackend)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("tui error: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
