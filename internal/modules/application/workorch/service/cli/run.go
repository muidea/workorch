package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Manage execution runs",
}

var runListCmd = &cobra.Command{
	Use:   "list",
	Short: "List runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		wsID, _ := cmd.Flags().GetString("workspace")
		runs, err := cl.ListRuns(ctx(), wsID)
		if err != nil {
			return fmt.Errorf("list runs: %w", err)
		}
		if jsonOutput {
			printJSON(runs)
			return nil
		}
		if len(runs) == 0 {
			fmt.Println(styleDim.Render("No runs found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tWorkspace\tState\tBlocked By"))
		for _, r := range runs {
			blocked := "-"
			if r.BlockingTicketID != "" {
				blocked = "decision:" + r.BlockingTicketID
			} else if r.BlockingApprovalID != "" {
				blocked = "approval:" + r.BlockingApprovalID
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				r.ID, r.WorkspaceID, stateIcon(r.State), styleDim.Render(blocked))
		}
		w.Flush()
		return nil
	},
}

var runShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show run details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		r, err := cl.GetRun(ctx(), args[0])
		if err != nil {
			return fmt.Errorf("get run: %w", err)
		}
		if jsonOutput {
			printJSON(r)
			return nil
		}
		fmt.Println(styleTitle.Render("Run: " + r.ID))
		fmt.Printf("  Workspace: %s\n", r.WorkspaceID)
		fmt.Printf("  State: %s\n", stateIcon(r.State))
		if r.BlockingTicketID != "" {
			fmt.Printf("  Blocked by decision: %s\n", r.BlockingTicketID)
		}
		if r.BlockingApprovalID != "" {
			fmt.Printf("  Blocked by approval: %s\n", r.BlockingApprovalID)
		}
		if r.FailureReason != "" {
			fmt.Printf("  Failure: %s\n", styleError.Render(r.FailureReason))
		}
		return nil
	},
}

var runActivateCmd = &cobra.Command{
	Use:   "activate <workspace-id>",
	Short: "Activate workspace execution (create + start run)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		req := &client.ActivationRequest{}
		req.GoalID, _ = cmd.Flags().GetString("goal")
		req.DoDID, _ = cmd.Flags().GetString("dod")
		req.SoulID, _ = cmd.Flags().GetString("soul")
		req.LLMProviderID, _ = cmd.Flags().GetString("llm-provider")
		skillIDs, _ := cmd.Flags().GetStringArray("skill")
		mcpIDs, _ := cmd.Flags().GetStringArray("mcp-server")
		req.SkillIDs = skillIDs
		req.MCPServerIDs = mcpIDs
		req.DialogLanguage, _ = cmd.Flags().GetString("lang")
		req.Priority, _ = cmd.Flags().GetInt("priority")

		act, err := cl.ActivateWorkspaceExecution(ctx(), args[0], req)
		if err != nil {
			return fmt.Errorf("activate execution: %w", err)
		}
		if jsonOutput {
			printJSON(act)
			return nil
		}
		fmt.Println(styleSuccess.Render("Execution activated."))
		fmt.Printf("  Run: %s\n", act.Run.ID)
		fmt.Printf("  State: %s\n", stateIcon(act.Run.State))
		return nil
	},
}

var runStatusCmd = &cobra.Command{
	Use:   "status <id>",
	Short: "Show run status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		waitState, _ := cmd.Flags().GetString("wait")
		waitTimeout, _ := cmd.Flags().GetInt("wait-timeout")
		s, err := cl.GetRunStatus(ctx(), args[0], waitState, waitTimeout)
		if err != nil {
			return fmt.Errorf("get run status: %w", err)
		}
		if jsonOutput {
			printJSON(s)
			return nil
		}
		fmt.Println(styleTitle.Render("Run Status: " + s.ID))
		fmt.Printf("  State: %s\n", stateIcon(s.State))
		fmt.Printf("  Steps: %d\n", s.StepCount)
		fmt.Printf("  Decisions: %d\n", s.DecisionCount)
		fmt.Printf("  Approvals: %d\n", s.ApprovalCount)
		if s.CurrentStepKind != "" {
			fmt.Printf("  Current Step: %s [%s]\n", s.CurrentStepKind, s.CurrentStepState)
		}
		if s.BlockingTicketID != "" {
			fmt.Printf("  Blocking Decision: %s\n", s.BlockingTicketID)
		}
		if s.BlockingApprovalID != "" {
			fmt.Printf("  Blocking Approval: %s\n", s.BlockingApprovalID)
		}
		return nil
	},
}

var runResumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume a blocked run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		r, err := cl.ResumeRun(ctx(), args[0])
		if err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
		fmt.Println(styleSuccess.Render("Run resumed: " + r.ID))
		fmt.Printf("  State: %s\n", stateIcon(r.State))
		return nil
	},
}

var runObservabilityCmd = &cobra.Command{
	Use:   "observability <id>",
	Short: "Show run observability with timeline",
	Aliases: []string{"obs"},
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		obs, err := cl.GetRunObservability(ctx(), args[0])
		if err != nil {
			return fmt.Errorf("get observability: %w", err)
		}
		if jsonOutput {
			printJSON(obs)
			return nil
		}
		fmt.Println(styleTitle.Render("Run Observability: " + args[0]))
		fmt.Printf("  State: %s\n", stateIcon(obs.Run.State))
		fmt.Println(styleDim.Render("  ── Timeline ──"))
		for _, e := range obs.Timeline {
			icon := "•"
			if e.Type == "step" {
				icon = "→"
			} else if e.Type == "decision" {
				icon = "⚠"
			} else if e.Type == "approval" {
				icon = "⚡"
			}
			fmt.Printf("  %s %s %s %s\n",
				icon,
				styleDim.Render(e.OccurredAt),
				e.Summary,
				styleDim.Render(e.StepID),
			)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runListCmd)
	runCmd.AddCommand(runShowCmd)
	runCmd.AddCommand(runActivateCmd)
	runCmd.AddCommand(runStatusCmd)
	runCmd.AddCommand(runResumeCmd)
	runCmd.AddCommand(runObservabilityCmd)

	runListCmd.Flags().String("workspace", "", "Filter by workspace ID")

	runActivateCmd.Flags().String("goal", "", "Goal ID")
	runActivateCmd.Flags().String("dod", "", "DoD ID")
	runActivateCmd.Flags().String("soul", "", "Soul ID")
	runActivateCmd.Flags().String("llm-provider", "", "LLM provider ID")
	runActivateCmd.Flags().StringArray("skill", nil, "Skill IDs (can repeat)")
	runActivateCmd.Flags().StringArray("mcp-server", nil, "MCP server IDs (can repeat)")
	runActivateCmd.Flags().String("lang", "", "Dialog language")
	runActivateCmd.Flags().Int("priority", 0, "Run priority")

	runStatusCmd.Flags().String("wait", "", "Wait for run to reach this state")
	runStatusCmd.Flags().Int("wait-timeout", 30000, "Wait timeout in ms")
}
