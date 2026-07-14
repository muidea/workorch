package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

var approvalCmd = &cobra.Command{
	Use:     "approval",
	Short:   "Manage approval gates",
	Aliases: []string{"apr"},
}

var approvalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List approval gates",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		approvals, err := mustBackend().Approval().List(ctx(), common.WorkspaceID(wsID))
		if err != nil {
			return fmt.Errorf("list approvals: %w", err)
		}
		if jsonOutput {
			printJSON(approvals)
			return nil
		}
		if len(approvals) == 0 {
			fmt.Println(styleDim.Render("No approval gates found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tWorkspace\tRun\tState\tReason"))
		for _, a := range approvals {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				a.ID, a.WorkspaceID, a.RunID, stateIcon(a.State), a.Reason)
		}
		w.Flush()
		return nil
	},
}

var approvalShowCmd = &cobra.Command{
	Use:   "show <approval-id>",
	Short: "Show approval gate details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		a, err := mustBackend().Approval().Get(ctx(), common.WorkspaceID(wsID), common.ApprovalID(args[0]))
		if err != nil {
			return fmt.Errorf("get approval: %w", err)
		}
		if jsonOutput {
			printJSON(a)
			return nil
		}
		fmt.Println(styleTitle.Render("Approval: " + string(a.ID)))
		fmt.Printf("  Workspace: %s\n", a.WorkspaceID)
		fmt.Printf("  Run: %s\n", a.RunID)
		fmt.Printf("  State: %s\n", stateIcon(a.State))
		fmt.Printf("  Reason: %s\n", a.Reason)
		return nil
	},
}

var approvalApproveCmd = &cobra.Command{
	Use:   "approve <approval-id>",
	Short: "Approve an approval gate",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		a, err := mustBackend().Approval().Approve(ctx(), common.WorkspaceID(wsID), common.ApprovalID(args[0]), workorcheventpayload.Payload{})
		if err != nil {
			return fmt.Errorf("approve: %w", err)
		}
		fmt.Println(styleSuccess.Render("Approved: " + string(a.ID)))
		fmt.Printf("  New state: %s\n", stateIcon(a.State))
		return nil
	},
}

var approvalRejectCmd = &cobra.Command{
	Use:   "reject <approval-id>",
	Short: "Reject an approval gate",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		a, err := mustBackend().Approval().Reject(ctx(), common.WorkspaceID(wsID), common.ApprovalID(args[0]), workorcheventpayload.Payload{})
		if err != nil {
			return fmt.Errorf("reject: %w", err)
		}
		fmt.Println(styleError.Render("Rejected: " + string(a.ID)))
		fmt.Printf("  New state: %s\n", stateIcon(a.State))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(approvalCmd)
	approvalCmd.AddCommand(approvalListCmd)
	approvalCmd.AddCommand(approvalShowCmd)
	approvalCmd.AddCommand(approvalApproveCmd)
	approvalCmd.AddCommand(approvalRejectCmd)

	approvalListCmd.Flags().String("workspace", "", "Filter by workspace ID")
	approvalListCmd.Flags().String("run", "", "Filter by run ID")
	approvalShowCmd.Flags().String("workspace", "", "Workspace ID (required)")
	approvalApproveCmd.Flags().String("workspace", "", "Workspace ID (required)")
	approvalRejectCmd.Flags().String("workspace", "", "Workspace ID (required)")
	approvalShowCmd.MarkFlagRequired("workspace")
	approvalApproveCmd.MarkFlagRequired("workspace")
	approvalRejectCmd.MarkFlagRequired("workspace")
}
