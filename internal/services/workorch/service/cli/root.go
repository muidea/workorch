package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/services/workorch/biz/config"
)

var (
	cfg        *config.Config
	cfgPath    string
	cliBackend backend.Backend

	remoteAddr         string
	jsonOutput         bool
	defaultWorkspaceID string

	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Padding(0, 1)
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleAlert   = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
)

var rootCmd = &cobra.Command{
	Use:   "workorch",
	Short: "WorkOrch CLI - AI workspace orchestration terminal",
	Long: `WorkOrch CLI provides terminal-based management for AI workspace orchestration.

Runs in local embedded mode by default. Use --remote <addr> to connect
to a running workorch --server service.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 {
			if err := runTUI(strings.Join(args, " ")); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		} else {
			if err := runTUI(""); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		}
	},
}

func ExecuteWithBackend(be backend.Backend, defaultWS projectioncontract.Workspace) error {
	cliBackend = be

	// Store default workspace for TUI/agent use.
	if defaultWS.ID != "" {
		defaultWorkspaceID = string(defaultWS.ID)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	err := rootCmd.ExecuteContext(ctx)

	if err != nil && isUnknownCommandError(err) {
		goal := strings.Join(os.Args[1:], " ")
		return runTUI(goal)
	}

	return err
}

func isUnknownCommandError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "unknown command") || strings.Contains(s, "unknown shorthand flag")
}

func init() {
	cfg = config.Default()
	cfgPath, _ = config.Path()

	rootCmd.PersistentFlags().StringVar(&remoteAddr, "remote", "", "Remote workorch --server address (e.g. http://192.168.1.100:8080)")
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "CLI config file path")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as raw JSON")
}

func mustBackend() backend.Backend {
	if cliBackend == nil {
		fmt.Fprintln(os.Stderr, "Error: workorch backend is not initialized.")
		os.Exit(1)
	}
	return cliBackend
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func stateIcon(state string) string {
	switch state {
	case "running":
		return styleSuccess.Render("✓") + " " + state
	case "completed":
		return styleSuccess.Render("✓") + " " + state
	case "blocked_on_decision":
		return styleWarning.Render("⚠") + " " + state
	case "blocked_on_approval":
		return styleAlert.Render("⚡") + " " + state
	case "failed":
		return styleError.Render("✗") + " " + state
	case "waiting_decision":
		return styleWarning.Render("⚠") + " " + state
	case "draft":
		return styleDim.Render("○") + " " + state
	default:
		return state
	}
}

func ctx() context.Context {
	return context.Background()
}
