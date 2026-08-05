package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// ── Soul commands ──

var soulCmd = &cobra.Command{
	Use:   "soul",
	Short: "Manage souls",
}

var soulListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all souls",
	RunE: func(cmd *cobra.Command, args []string) error {
		souls, err := mustBackend().Soul().List(ctx())
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(souls)
			return nil
		}
		if len(souls) == 0 {
			fmt.Println(styleDim.Render("No souls found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tName\tBusiness Domains"))
		for _, s := range souls {
			fmt.Fprintf(w, "%s\t%s\t%v\n", s.ID, s.Name, s.BusinessDomains)
		}
		w.Flush()
		return nil
	},
}

// ── Skill commands ──

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skills",
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		skills, err := mustBackend().Skill().List(ctx())
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(skills)
			return nil
		}
		if len(skills) == 0 {
			fmt.Println(styleDim.Render("No skills found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tName\tVersion"))
		for _, s := range skills {
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.ID, s.Name, s.Version)
		}
		w.Flush()
		return nil
	},
}

var skillCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a skill",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		version, _ := cmd.Flags().GetString("version")
		instruction, _ := cmd.Flags().GetString("instruction")
		s := &workorchmanagementcontract.SkillMutation{Name: name, Version: version, Instruction: instruction}
		result, err := mustBackend().Skill().Create(ctx(), s)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(result)
		} else {
			fmt.Printf("Skill created: %s\n", styleTitle.Render(string(result.ID)))
		}
		return nil
	},
}

// ── MCP commands ──

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers and capabilities",
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		servers, err := mustBackend().MCP().ListServers(ctx())
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(servers)
			return nil
		}
		if len(servers) == 0 {
			fmt.Println(styleDim.Render("No MCP servers found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("ID\tName\tTransport\tEnabled"))
		for _, s := range servers {
			enabled := styleSuccess.Render("✓")
			if !s.Enabled {
				enabled = styleDim.Render("✗")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Name, s.Transport, enabled)
		}
		w.Flush()
		return nil
	},
}

var mcpToolsCmd = &cobra.Command{
	Use:   "tools <server-id>",
	Short: "List tools on an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tools, err := mustBackend().MCP().ListCapabilities(ctx())
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(tools)
			return nil
		}
		for _, t := range tools {
			if t.ServerID == args[0] {
				fmt.Printf("  %s\n", t.Name)
			}
		}
		return nil
	},
}

// ── Memory commands ──

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Inspect workspace memory",
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memory items",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		query, _ := cmd.Flags().GetString("query")
		tags, _ := cmd.Flags().GetStringArray("tag")
		types, _ := cmd.Flags().GetStringArray("type")
		limit, _ := cmd.Flags().GetInt("limit")
		items, err := mustBackend().Memory().Search(ctx(), workorchquerycontract.MemorySearchQuery{
			WorkspaceID: common.WorkspaceID(wsID),
			Query:       query,
			Tags:        tags,
			Types:       types,
			Limit:       limit,
		})
		if err != nil {
			return err
		}
		return printMemoryItems(items)
	},
}

var memorySearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search workspace memory",
	RunE: func(cmd *cobra.Command, args []string) error {
		return memoryListCmd.RunE(cmd, args)
	},
}

var memoryGetCmd = &cobra.Command{
	Use:   "get <memory-id>",
	Short: "Read a memory item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		item, err := mustBackend().Memory().Get(ctx(), common.WorkspaceID(wsID), common.MemoryID(args[0]))
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(item)
			return nil
		}
		printMemoryItem(*item)
		return nil
	},
}

func printMemoryItems(items []workorchmanagementcontract.MemoryItem) error {
	if jsonOutput {
		printJSON(items)
		return nil
	}
	if len(items) == 0 {
		fmt.Println(styleDim.Render("No memory items found."))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, styleHeader.Render("ID\tType\tSummary\tWorkspace"))
	for _, m := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.ID, m.MemoryType, m.Summary, m.WorkspaceID)
	}
	return w.Flush()
}

func printMemoryItem(item workorchmanagementcontract.MemoryItem) {
	fmt.Printf("ID: %s\n", styleTitle.Render(item.ID))
	fmt.Printf("Workspace: %s\n", item.WorkspaceID)
	if item.RunID != "" {
		fmt.Printf("Run: %s\n", item.RunID)
	}
	fmt.Printf("Type: %s\n", item.MemoryType)
	fmt.Printf("Source: %s %s\n", item.SourceType, item.SourceRef)
	fmt.Printf("Summary: %s\n", item.Summary)
	if item.Content != "" {
		fmt.Printf("Content: %s\n", item.Content)
	}
	if len(item.Tags) > 0 {
		fmt.Printf("Tags: %v\n", item.Tags)
	}
	if item.VectorReady {
		fmt.Printf("Vector: ready dimension=%d score=%.4f\n", item.VectorDimension, item.VectorScore)
	}
}

func registerMemoryQueryFlags(cmd *cobra.Command) {
	cmd.Flags().String("query", "", "Keyword query")
	cmd.Flags().StringArray("tag", nil, "Filter by tag (repeatable)")
	cmd.Flags().StringArray("type", nil, "Filter by memory type (repeatable)")
	cmd.Flags().Int("limit", 0, "Maximum number of memory items")
}

// ── Events command ──

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "View event stream",
}

var eventsStreamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Show recent events",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("event history listing is not exposed by the shared CLI backend; use the TUI for live event monitoring")
	},
}

// ── Config command ──

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI and service configuration",
}

// ── Template command ──

var templateCmd = &cobra.Command{
	Use:     "workspace-template",
	Short:   "Manage workspace templates",
	Aliases: []string{"template", "tmpl"},
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspace templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpls, err := mustBackend().Template().List(ctx())
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(tmpls)
			return nil
		}
		if len(tmpls) == 0 {
			fmt.Println(styleDim.Render("No workspace templates found."))
			return nil
		}
		for _, t := range tmpls {
			fmt.Printf("  %s %s\n", styleTitle.Render(t.ID), t.Name)
		}
		return nil
	},
}

func init() {
	// Soul
	rootCmd.AddCommand(soulCmd)
	soulCmd.AddCommand(soulListCmd)

	// Skill
	rootCmd.AddCommand(skillCmd)
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillCreateCmd)
	skillCreateCmd.Flags().String("name", "", "Skill name (required)")
	skillCreateCmd.Flags().String("version", "1.0.0", "Skill version")
	skillCreateCmd.Flags().String("instruction", "", "Skill instruction (required)")
	skillCreateCmd.MarkFlagRequired("name")
	skillCreateCmd.MarkFlagRequired("instruction")
	skillListCmd.Flags().String("workspace", "", "Filter by workspace ID")

	// MCP
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpToolsCmd)

	// Memory
	rootCmd.AddCommand(memoryCmd)
	memoryCmd.AddCommand(memoryListCmd)
	memoryCmd.AddCommand(memorySearchCmd)
	memoryCmd.AddCommand(memoryGetCmd)
	memoryListCmd.Flags().String("workspace", "", "Workspace ID (required)")
	registerMemoryQueryFlags(memoryListCmd)
	memorySearchCmd.Flags().String("workspace", "", "Workspace ID (required)")
	registerMemoryQueryFlags(memorySearchCmd)
	memoryGetCmd.Flags().String("workspace", "", "Workspace ID (required)")
	memoryGetCmd.MarkFlagRequired("workspace")

	// Events
	rootCmd.AddCommand(eventsCmd)
	eventsCmd.AddCommand(eventsStreamCmd)
	eventsStreamCmd.Flags().String("workspace", "", "Filter by workspace ID")
	eventsStreamCmd.Flags().String("run", "", "Filter by run ID")

	// Config — subcommands registered in config_cmd.go
	rootCmd.AddCommand(configCmd)

	// Template
	rootCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(templateListCmd)
}
