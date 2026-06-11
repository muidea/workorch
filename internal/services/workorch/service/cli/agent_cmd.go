package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/services/workorch/biz/agent"
)

var agentCmd = &cobra.Command{
	Use:   "agent [goal]",
	Short: "Run an AI agent to accomplish a goal",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runAgent,
}

func init() {
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	goalText := strings.Join(args, " ")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsID := defaultWorkspaceID
	if wsID == "" {
		return fmt.Errorf("%w: workspace is required; create or select a workspace before running agent", common.ErrInvalidArgument)
	}
	fmt.Fprintf(os.Stderr, "Workspace: %s\n", wsID)

	a := agent.NewWithBackend(mustBackend())
	defer a.Close()

	runErr := make(chan error, 1)
	go func() {
		runErr <- a.RunWithBackend(ctx, wsID, goalText)
	}()
	finish := func(preferred error) error {
		cancel()
		err := <-runErr
		if preferred != nil {
			return preferred
		}
		return err
	}

	for evt := range a.Events() {
		switch evt.Type {
		case agent.EventRunCreated:
			info := evt.Payload.(agent.RunCreatedInfo)
			fmt.Printf("→ Run started: %s\n", info.RunID)

		case agent.EventDecision:
			info := evt.Payload.(agent.DecisionInfo)
			fmt.Printf("\n⚠ Decision required [%s]\n  %s\n\n  Your answer: ", info.TicketID, info.Question)
			var answer string
			fmt.Scanln(&answer)
			if answer == "" {
				answer = "continue"
			}
			if err := a.ResolveDecision(ctx, info.WorkspaceID, info.TicketID, "provide_missing_info", answer); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				a.ClearNotified()
				continue
			}
			if err := a.ResumeRun(ctx, info.WorkspaceID, info.RunID); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				a.ClearNotified()
				continue
			}

		case agent.EventApproval:
			info := evt.Payload.(agent.ApprovalInfo)
			fmt.Printf("\n⚡ Approval required [%s]\n  %s\n  Approve? [y/N]: ", info.ApprovalID, info.Reason)
			var answer string
			fmt.Scanln(&answer)
			if err := a.ResolveApproval(ctx, info.WorkspaceID, info.ApprovalID, answer == "y" || answer == "Y"); err != nil {
				return finish(fmt.Errorf("resolve approval: %w", err))
			}
			if err := a.ResumeRun(ctx, info.WorkspaceID, info.RunID); err != nil {
				return finish(fmt.Errorf("resume run: %w", err))
			}

		case agent.EventRunCompleted:
			info, _ := evt.Payload.(agent.RunCompletedInfo)
			fmt.Println("\n✓ Run completed successfully.")
			if info.Summary != "" {
				fmt.Println("── Result ──")
				fmt.Println(info.Summary)
			}
			if len(info.Sources) > 0 {
				fmt.Println("── Sources ──")
				for _, source := range info.Sources {
					fmt.Printf("  - %s\n", source)
				}
			}
			for _, o := range info.Outputs {
				fmt.Printf("  %s: %s\n", o.Kind, o.Summary)
			}
			return finish(nil)

		case agent.EventRunFailed:
			if evt.Error != nil {
				fmt.Printf("\n✗ Run failed: %v\n", evt.Error)
			}
			return finish(evt.Error)

		case agent.EventCancelled:
			fmt.Println("\nInterrupted.")
			return finish(nil)
		}
	}
	return <-runErr
}
