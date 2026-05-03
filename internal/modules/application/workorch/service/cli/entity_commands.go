package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
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
		cl := mustClient()
		souls, err := cl.ListSouls(ctx())
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
		cl := mustClient()
		wsID, _ := cmd.Flags().GetString("workspace")
		skills, err := cl.ListSkills(ctx(), wsID)
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
		fmt.Fprintln(w, styleHeader.Render("ID\tName\tVersion\tScope"))
		for _, s := range skills {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Name, s.Version, s.Scope)
		}
		w.Flush()
		return nil
	},
}

var skillCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a skill",
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		name, _ := cmd.Flags().GetString("name")
		version, _ := cmd.Flags().GetString("version")
		instruction, _ := cmd.Flags().GetString("instruction")
		s := &client.Skill{Name: name, Version: version, Instruction: instruction}
		result, err := cl.CreateSkill(ctx(), s)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(result)
		} else {
			fmt.Printf("Skill created: %s\n", styleTitle.Render(result.ID))
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
		cl := mustClient()
		servers, err := cl.ListMCPServers(ctx())
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
		cl := mustClient()
		tools, err := cl.ListMCPServerTools(ctx(), args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(tools)
			return nil
		}
		for _, t := range tools {
			fmt.Printf("  %s\n", t.Name)
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
		cl := mustClient()
		wsID, _ := cmd.Flags().GetString("workspace")
		if wsID == "" {
			return fmt.Errorf("--workspace is required")
		}
		items, err := cl.ListMemory(ctx(), wsID)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(items)
			return nil
		}
		if len(items) == 0 {
			fmt.Println(styleDim.Render("No memory items found."))
			return nil
		}
		for _, m := range items {
			fmt.Printf("%s %s %s [%s]\n",
				styleDim.Render(m.ID), m.Summary, m.MemoryType, styleDim.Render(m.SourceType))
		}
		return nil
	},
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
		cl := mustClient()
		wsID, _ := cmd.Flags().GetString("workspace")
		runID, _ := cmd.Flags().GetString("run")
		events, err := cl.ListEvents(ctx(), wsID, runID)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(events)
			return nil
		}
		for _, e := range events {
			fmt.Printf("%s %s %s %s\n",
				styleDim.Render(e.OccurredAt),
				e.Topic,
				e.SubjectType,
				styleDim.Render(e.SubjectID),
			)
		}
		return nil
	},
}

// ── Config command ──

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI and service configuration",
}

// ── Template command ──

var templateCmd = &cobra.Command{
	Use:   "workspace-template",
	Short: "Manage workspace templates",
	Aliases: []string{"template", "tmpl"},
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspace templates",
	RunE: func(cmd *cobra.Command, args []string) error {
		cl := mustClient()
		tmpls, err := cl.ListWorkspaceTemplates(ctx())
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
	memoryListCmd.Flags().String("workspace", "", "Workspace ID (required)")

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
