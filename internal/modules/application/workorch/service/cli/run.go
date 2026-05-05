package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Manage execution runs",
}

var runListCmd = &cobra.Command{
	Use:   "list",
	Short: "List runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		runs, err := mustBackend().Run().List(ctx(), common.WorkspaceID(wsID))
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
				blocked = "decision:" + string(r.BlockingTicketID)
			} else if r.BlockingApprovalID != "" {
				blocked = "approval:" + string(r.BlockingApprovalID)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				r.ID, r.WorkspaceID, stateIcon(string(r.State)), styleDim.Render(blocked))
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
		r, err := mustBackend().Run().Get(ctx(), common.RunID(args[0]))
		if err != nil {
			return fmt.Errorf("get run: %w", err)
		}
		if jsonOutput {
			printJSON(r)
			return nil
		}
		fmt.Println(styleTitle.Render("Run: " + string(r.ID)))
		fmt.Printf("  Workspace: %s\n", r.WorkspaceID)
		fmt.Printf("  State: %s\n", stateIcon(string(r.State)))
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
		req := backend.ActivateRunRequest{}
		goalID, _ := cmd.Flags().GetString("goal")
		dodID, _ := cmd.Flags().GetString("dod")
		soulID, _ := cmd.Flags().GetString("soul")
		req.GoalID = common.GoalID(goalID)
		req.DoDID = common.DoDID(dodID)
		req.SoulID = common.SoulID(soulID)
		req.LLMProviderID, _ = cmd.Flags().GetString("llm-provider")
		req.DialogLanguage, _ = cmd.Flags().GetString("lang")
		skillIDs, _ := cmd.Flags().GetStringArray("skill")
		mcpIDs, _ := cmd.Flags().GetStringArray("mcp-server")
		for _, id := range skillIDs {
			req.SkillIDs = append(req.SkillIDs, common.SkillID(id))
		}
		for _, id := range mcpIDs {
			req.MCPServerIDs = append(req.MCPServerIDs, common.MCPServerID(id))
		}
		req.Priority, _ = cmd.Flags().GetInt("priority")

		run, err := mustBackend().Run().Activate(ctx(), common.WorkspaceID(args[0]), req)
		if err != nil {
			return fmt.Errorf("activate execution: %w", err)
		}
		if jsonOutput {
			printJSON(run)
			return nil
		}
		fmt.Println(styleSuccess.Render("Execution activated."))
		fmt.Printf("  Run: %s\n", run.ID)
		fmt.Printf("  State: %s\n", stateIcon(string(run.State)))
		return nil
	},
}

var runStatusCmd = &cobra.Command{
	Use:   "status <id>",
	Short: "Show run status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := mustBackend().Run().Status(ctx(), common.RunID(args[0]))
		if err != nil {
			return fmt.Errorf("get run status: %w", err)
		}
		if jsonOutput {
			printJSON(s)
			return nil
		}
		fmt.Println(styleTitle.Render("Run Status: " + string(s.ID)))
		fmt.Printf("  State: %s\n", stateIcon(string(s.State)))
		fmt.Printf("  Steps: %d\n", s.StepCount)
		fmt.Printf("  Decisions: %d\n", s.DecisionCount)
		fmt.Printf("  Approvals: %d\n", s.ApprovalCount)
		if s.CurrentStepKind != "" {
			fmt.Printf("  Current Step: %s\n", s.CurrentStepKind)
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
		r, err := mustBackend().Run().Resume(ctx(), common.RunID(args[0]))
		if err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
		fmt.Println(styleSuccess.Render("Run resumed: " + string(r.ID)))
		fmt.Printf("  State: %s\n", stateIcon(string(r.State)))
		return nil
	},
}

var runObservabilityCmd = &cobra.Command{
	Use:     "observability <id>",
	Short:   "Show run observability with timeline",
	Aliases: []string{"obs"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		run, err := mustBackend().Run().Get(ctx(), common.RunID(args[0]))
		if err != nil {
			return fmt.Errorf("get observability: %w", err)
		}
		if jsonOutput {
			printJSON(run)
			return nil
		}
		fmt.Println(styleTitle.Render("Run Observability: " + args[0]))
		fmt.Printf("  State: %s\n", stateIcon(string(run.State)))
		fmt.Printf("  Workspace: %s\n", run.WorkspaceID)
		fmt.Printf("  Goal: %s\n", run.GoalID)
		fmt.Printf("  DoD: %s\n", run.DoDID)
		return nil
	},
}

var runPatchesCmd = &cobra.Command{
	Use:   "patches <id>",
	Short: "Show patch evidence for a run",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		run, err := mustBackend().Run().Get(ctx(), common.RunID(args[0]))
		if err != nil {
			return fmt.Errorf("get run: %w", err)
		}
		snapshotBackend, ok := mustBackend().Workspace().(interface {
			Snapshot(context.Context, common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error)
		})
		if !ok {
			return fmt.Errorf("workspace snapshot is unavailable")
		}
		snapshot, err := snapshotBackend.Snapshot(ctx(), run.WorkspaceID)
		if err != nil {
			return fmt.Errorf("get workspace snapshot: %w", err)
		}
		patches := patchesForRun(snapshot, run.ID)
		if jsonOutput {
			printJSON(patches)
			return nil
		}
		if len(patches) == 0 {
			fmt.Println(styleDim.Render("No patches found."))
			return nil
		}
		fmt.Println(styleTitle.Render("Run Patches: " + args[0]))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tState\tType\tTarget\tLines\tApproval\tSummary"))
		for _, item := range patches {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t+%d/-%d\t%s\t%s\n",
				item.ID,
				stateIcon(string(item.State)),
				item.PatchType,
				emptyDash(item.TargetRef),
				item.AddedLines,
				item.DeletedLines,
				emptyDash(nonEmptyApprovalID(item.ApprovalID, item.RevertApprovalID)),
				emptyDash(item.Summary),
			)
		}
		w.Flush()
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
	runCmd.AddCommand(runPatchesCmd)

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

func patchesForRun(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.PatchInfo {
	if snapshot == nil || len(snapshot.Patches) == 0 {
		return nil
	}
	out := make([]backend.PatchInfo, 0, len(snapshot.Patches))
	for _, item := range snapshot.Patches {
		if item.RunID != runID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func nonEmptyApprovalID(values ...common.ApprovalID) string {
	for _, value := range values {
		if value != "" {
			return string(value)
		}
	}
	return ""
}
