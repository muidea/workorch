package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/services/workorch/biz/config"
	"github.com/muidea/workorch/internal/services/workorch/service/tui"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Enter interactive TUI agent mode",
	Long:  "Start the primary WorkOrch terminal agent session.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI("")
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
}

func runTUI(initialGoal string) error {
	recordActiveWorkspace(defaultWorkspaceID)
	if info, err := os.Stdin.Stat(); err == nil && (info.Mode()&os.ModeCharDevice == 0 || os.Getenv("TERM") == "dumb") {
		return fmt.Errorf("TUI requires an interactive terminal; use --server to run the backend service without TUI")
	}
	m := tui.NewAppModelWithOptions(mustBackend(), defaultWorkspaceID, initialGoal, tui.Options{
		OnActiveWorkspaceChanged: recordActiveWorkspace,
	})
	defer m.CloseProjectionSubscription(tui.ProjectionSubscriptionCleanupTimeout())
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(os.Stderr))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui error: %w", err)
	}
	return nil
}

func recordActiveWorkspace(workspaceID string) {
	if workspaceID == "" {
		return
	}
	next, err := config.Load(cfgPath)
	if err != nil {
		return
	}
	next.Defaults.LastActiveWorkspaceID = workspaceID
	_ = config.Save(cfgPath, next)
}
