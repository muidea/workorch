package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Short:   "Manage workspaces",
	Aliases: []string{"ws"},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaces, err := mustBackend().Workspace().List(ctx())
		if err != nil {
			return fmt.Errorf("list workspaces: %w", err)
		}
		if jsonOutput {
			printJSON(workspaces)
			return nil
		}
		if len(workspaces) == 0 {
			fmt.Println(styleDim.Render("No workspaces found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tTitle\tState\tActive Run"))
		for _, ws := range workspaces {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				ws.ID,
				ws.Title,
				stateIcon(string(ws.State)),
				styleDim.Render(string(ws.ActiveRunID)),
			)
		}
		w.Flush()
		return nil
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		dir, _ := cmd.Flags().GetString("working-dir")
		ws, err := mustBackend().Workspace().Create(ctx(), backend.CreateWorkspaceRequest{
			Title:            title,
			Description:      desc,
			WorkingDirectory: dir,
		})
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}
		if jsonOutput {
			printJSON(ws)
			return nil
		}
		fmt.Printf("Workspace created: %s\n", styleTitle.Render(string(ws.ID)))
		fmt.Printf("  Title: %s\n", ws.Title)
		if ws.Description != "" {
			fmt.Printf("  Description: %s\n", ws.Description)
		}
		return nil
	},
}

var workspaceShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show workspace details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := mustBackend().Workspace().Get(ctx(), common.WorkspaceID(args[0]))
		if err != nil {
			return fmt.Errorf("get workspace: %w", err)
		}
		if jsonOutput {
			printJSON(ws)
			return nil
		}
		printWorkspaceDetail(ws)
		return nil
	},
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		purge, _ := cmd.Flags().GetBool("purge")
		if err := mustBackend().Workspace().Delete(ctx(), common.WorkspaceID(args[0]), purge); err != nil {
			return fmt.Errorf("delete workspace: %w", err)
		}
		fmt.Println(styleSuccess.Render("Workspace deleted."))
		return nil
	},
}

var workspaceSnapshotCmd = &cobra.Command{
	Use:   "snapshot <id>",
	Short: "Show workspace snapshot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := mustBackend().Workspace().Get(ctx(), common.WorkspaceID(args[0]))
		if err != nil {
			return fmt.Errorf("get snapshot: %w", err)
		}
		if jsonOutput {
			printJSON(ws)
			return nil
		}
		fmt.Println(styleTitle.Render("Workspace Snapshot: " + args[0]))
		fmt.Printf("  State: %s\n", stateIcon(string(ws.State)))
		fmt.Printf("  Active Run: %s\n", styleDim.Render(string(ws.ActiveRunID)))
		return nil
	},
}

func printWorkspaceDetail(ws *backend.WorkspaceInfo) {
	fmt.Println(styleTitle.Render("Workspace: " + string(ws.ID)))
	fmt.Printf("  Title: %s\n", ws.Title)
	if ws.Description != "" {
		fmt.Printf("  Description: %s\n", ws.Description)
	}
	fmt.Printf("  State: %s\n", stateIcon(string(ws.State)))
	if ws.ActiveRunID != "" {
		fmt.Printf("  Active Run: %s\n", ws.ActiveRunID)
	}
	if ws.WorkingDirectory != "" {
		fmt.Printf("  Working Dir: %s\n", ws.WorkingDirectory)
	}
}

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	workspaceCmd.AddCommand(workspaceSnapshotCmd)

	workspaceCreateCmd.Flags().String("title", "", "Workspace title (required)")
	workspaceCreateCmd.Flags().String("description", "", "Workspace description")
	workspaceCreateCmd.Flags().String("working-dir", "", "Working directory path")
	workspaceCreateCmd.MarkFlagRequired("title")

	workspaceDeleteCmd.Flags().Bool("purge", false, "Purge all related data")
}
