package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var decisionCmd = &cobra.Command{
	Use:     "decision",
	Short:   "Manage decision tickets",
	Aliases: []string{"dec"},
}

var decisionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List decision tickets",
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		wsID, _ := cmd.Flags().GetString("workspace")
		runID, _ := cmd.Flags().GetString("run")
		decisions, err := cl.ListDecisions(ctx(), wsID, runID)
		if err != nil {
			return fmt.Errorf("list decisions: %w", err)
		}
		if jsonOutput {
			printJSON(decisions)
			return nil
		}
		if len(decisions) == 0 {
			fmt.Println(styleDim.Render("No decision tickets found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tWorkspace\tRun\tState\tQuestion"))
		for _, d := range decisions {
			question := d.Question
			if len(question) > 60 {
				question = question[:57] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				d.ID, d.WorkspaceID, d.RunID, stateIcon(d.State), question)
		}
		w.Flush()
		return nil
	},
}

var decisionShowCmd = &cobra.Command{
	Use:   "show <ticket-id>",
	Short: "Show decision ticket details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		d, err := cl.GetDecision(ctx(), args[0])
		if err != nil {
			return fmt.Errorf("get decision: %w", err)
		}
		if jsonOutput {
			printJSON(d)
			return nil
		}
		fmt.Println(styleTitle.Render("Decision: " + d.ID))
		fmt.Printf("  Workspace: %s\n", d.WorkspaceID)
		fmt.Printf("  Run: %s\n", d.RunID)
		fmt.Printf("  State: %s\n", stateIcon(d.State))
		fmt.Printf("  Kind: %s\n", d.DecisionKind)
		if d.Reason != "" {
			fmt.Printf("  Reason: %s\n", d.Reason)
		}
		if d.Question != "" {
			fmt.Println(styleWarning.Render("\n  " + d.Question))
		}
		return nil
	},
}

var decisionResolveCmd = &cobra.Command{
	Use:   "resolve <ticket-id>",
	Short: "Resolve a decision ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		resType, _ := cmd.Flags().GetString("type")
		payloadStr, _ := cmd.Flags().GetString("payload")
		var payload map[string]any
		if payloadStr != "" {
			payload = map[string]any{"value": payloadStr}
		}
		d, err := cl.ResolveDecision(ctx(), args[0], resType, payload)
		if err != nil {
			return fmt.Errorf("resolve decision: %w", err)
		}
		fmt.Println(styleSuccess.Render("Decision resolved: " + d.ID))
		fmt.Printf("  New state: %s\n", stateIcon(d.State))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(decisionCmd)
	decisionCmd.AddCommand(decisionListCmd)
	decisionCmd.AddCommand(decisionShowCmd)
	decisionCmd.AddCommand(decisionResolveCmd)

	decisionListCmd.Flags().String("workspace", "", "Filter by workspace ID")
	decisionListCmd.Flags().String("run", "", "Filter by run ID")

	decisionResolveCmd.Flags().String("type", "provide_missing_info", "Resolution type")
	decisionResolveCmd.Flags().String("payload", "", "Resolution payload (simple string)")
}
