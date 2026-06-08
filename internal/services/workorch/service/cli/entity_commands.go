package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
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
		s := &backend.SkillInfo{Name: name, Version: version, Instruction: instruction}
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
			if t.ServerID == common.MCPServerID(args[0]) {
				fmt.Printf("  %s\n", t.Name)
			}
		}
		return nil
	},
}

// ── Memory commands ──

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage workspace memory",
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
		items, err := mustBackend().Memory().Search(ctx(), common.WorkspaceID(wsID), backend.MemorySearchRequest{
			Query: query,
			Tags:  tags,
			Types: types,
			Limit: limit,
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
		item, err := mustBackend().Memory().Get(ctx(), common.MemoryID(args[0]))
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

var memoryCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a workspace memory item",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		item := memoryItemFromFlags(cmd)
		created, err := mustBackend().Memory().Create(ctx(), common.WorkspaceID(wsID), item)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(created)
			return nil
		}
		fmt.Printf("Memory created: %s\n", styleTitle.Render(string(created.ID)))
		return nil
	},
}

var memoryUpdateCmd = &cobra.Command{
	Use:   "update <memory-id>",
	Short: "Update a memory item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		item := memoryItemFromFlags(cmd)
		updated, err := mustBackend().Memory().Update(ctx(), common.MemoryID(args[0]), item)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(updated)
			return nil
		}
		fmt.Printf("Memory updated: %s\n", styleTitle.Render(string(updated.ID)))
		return nil
	},
}

var memoryDeleteCmd = &cobra.Command{
	Use:   "delete <memory-id>",
	Short: "Delete a memory item",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := mustBackend().Memory().Delete(ctx(), common.MemoryID(args[0])); err != nil {
			return err
		}
		if jsonOutput {
			printJSON(map[string]any{"deleted": true, "id": args[0]})
			return nil
		}
		fmt.Printf("Memory deleted: %s\n", styleTitle.Render(args[0]))
		return nil
	},
}

func memoryItemFromFlags(cmd *cobra.Command) *backend.MemoryItemInfo {
	id, _ := cmd.Flags().GetString("id")
	runID, _ := cmd.Flags().GetString("run")
	sourceType, _ := cmd.Flags().GetString("source-type")
	sourceRef, _ := cmd.Flags().GetString("source-ref")
	memoryType, _ := cmd.Flags().GetString("type")
	content, _ := cmd.Flags().GetString("content")
	summary, _ := cmd.Flags().GetString("summary")
	tags, _ := cmd.Flags().GetStringArray("tag")
	importance, _ := cmd.Flags().GetInt("importance")
	confidence, _ := cmd.Flags().GetInt("confidence")
	visibility, _ := cmd.Flags().GetString("visibility")
	return &backend.MemoryItemInfo{
		ID:         common.MemoryID(id),
		RunID:      common.RunID(runID),
		SourceType: sourceType,
		SourceRef:  sourceRef,
		MemoryType: memoryType,
		Content:    content,
		Summary:    summary,
		Tags:       tags,
		Importance: importance,
		Confidence: confidence,
		Visibility: visibility,
	}
}

func printMemoryItems(items []backend.MemoryItemInfo) error {
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

func printMemoryItem(item backend.MemoryItemInfo) {
	fmt.Printf("ID: %s\n", styleTitle.Render(string(item.ID)))
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

func registerMemoryEditFlags(cmd *cobra.Command, withDefaults bool) {
	sourceType := ""
	memoryType := ""
	visibility := ""
	importance := 0
	confidence := 0
	if withDefaults {
		sourceType = "user"
		memoryType = "project_context"
		visibility = "workspace"
		importance = 50
		confidence = 50
	}
	cmd.Flags().String("run", "", "Source run ID")
	cmd.Flags().String("source-type", sourceType, "Memory source type")
	cmd.Flags().String("source-ref", "", "Memory source reference")
	cmd.Flags().String("type", memoryType, "Memory type")
	cmd.Flags().String("summary", "", "Memory summary")
	cmd.Flags().String("content", "", "Memory content")
	cmd.Flags().StringArray("tag", nil, "Memory tag (repeatable)")
	cmd.Flags().Int("importance", importance, "Importance score")
	cmd.Flags().Int("confidence", confidence, "Confidence score")
	cmd.Flags().String("visibility", visibility, "Memory visibility")
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
	memoryCmd.AddCommand(memoryCreateCmd)
	memoryCmd.AddCommand(memoryUpdateCmd)
	memoryCmd.AddCommand(memoryDeleteCmd)
	memoryListCmd.Flags().String("workspace", "", "Workspace ID (required)")
	registerMemoryQueryFlags(memoryListCmd)
	memorySearchCmd.Flags().String("workspace", "", "Workspace ID (required)")
	registerMemoryQueryFlags(memorySearchCmd)
	memoryCreateCmd.Flags().String("workspace", "", "Workspace ID (required)")
	memoryCreateCmd.Flags().String("id", "", "Memory ID (optional)")
	registerMemoryEditFlags(memoryCreateCmd, true)
	registerMemoryEditFlags(memoryUpdateCmd, false)
	memoryCreateCmd.MarkFlagRequired("workspace")
	memoryCreateCmd.MarkFlagRequired("summary")

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
