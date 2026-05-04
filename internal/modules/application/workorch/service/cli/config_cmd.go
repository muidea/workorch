package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetLLMCmd)
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := workorchconfig.Current()
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		if jsonOutput {
			printJSON(cfg)
			return nil
		}
		fmt.Printf("Endpoint: %s\n", cfg.EndpointName)
		fmt.Printf("Server:   %s\n", cfg.Server.Addr)
		fmt.Printf("Storage:  %s (%s)\n", cfg.Repository.Backend, cfg.Repository.DataDir)
		fmt.Printf("LLM default: %s\n", cfg.LLM.DefaultProvider)
		fmt.Println("LLM providers:")
		for id, p := range cfg.LLM.Providers {
			status := styleSuccess.Render("enabled")
			if p.Disabled != nil && *p.Disabled {
				status = styleDim.Render("disabled")
			}
			model := ""
			if p.Model != "" {
				model = " → " + p.Model
			}
			fmt.Printf("  %s  %s  %s%s\n",
				styleTitle.Render(id),
				styleDim.Render(p.Provider),
				status,
				model,
			)
		}
		return nil
	},
}

var configSetLLMCmd = &cobra.Command{
	Use:   "set-llm [provider-id]",
	Short: "Add or update an LLM provider",
	Long: `Add or update an LLM provider configuration.

Run without flags for interactive wizard mode.
Run with --model/--base-url/etc for non-interactive setup.

Examples:
  workorch config set-llm local
  workorch config set-llm local --model qwen2.5 --base-url http://localhost:11434/v1 --api-key ollama
  workorch config set-llm openai --model gpt-4o --api-key sk-xxx
  workorch config set-llm local --disable`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		providerID := args[0]
		if providerID == "help" || providerID == "-h" || providerID == "--help" {
			return cmd.Help()
		}

		if hasAnyFlag(cmd) {
			return runFlagMode(cmd, providerID)
		}
		return runWizardMode(providerID)
	},
}

func hasAnyFlag(cmd *cobra.Command) bool {
	for _, name := range []string{"model", "base-url", "api-key", "provider", "timeout", "disable", "default", "streaming"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func runFlagMode(cmd *cobra.Command, providerID string) error {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]workorchconfig.LLMProviderConfig{}
	}

	p, exists := cfg.LLM.Providers[providerID]
	if !exists {
		p = workorchconfig.LLMProviderConfig{}
	}

	if cmd.Flags().Changed("model") {
		v, _ := cmd.Flags().GetString("model")
		p.Model = v
	}
	if cmd.Flags().Changed("base-url") {
		v, _ := cmd.Flags().GetString("base-url")
		p.BaseURL = v
	}
	if cmd.Flags().Changed("api-key") {
		v, _ := cmd.Flags().GetString("api-key")
		p.APIKey = v
	}
	if cmd.Flags().Changed("provider") {
		v, _ := cmd.Flags().GetString("provider")
		p.Provider = v
		p.ChatCompletions = boolPtr(true)
	}
	if cmd.Flags().Changed("timeout") {
		v, _ := cmd.Flags().GetString("timeout")
		p.Timeout = v
	}
	if cmd.Flags().Changed("disable") {
		disabled, _ := cmd.Flags().GetBool("disable")
		p.Disabled = &disabled
	}
	if cmd.Flags().Changed("default") {
		isDefault, _ := cmd.Flags().GetBool("default")
		if isDefault {
			cfg.LLM.DefaultProvider = providerID
		}
	}
	if cmd.Flags().Changed("streaming") {
		streaming, _ := cmd.Flags().GetBool("streaming")
		p.Streaming = &streaming
	}

	if p.Provider == "" {
		p.Provider = "compatible"
		p.ChatCompletions = boolPtr(true)
	}

	cfg.LLM.Providers[providerID] = p
	if strings.TrimSpace(cfg.LLM.DefaultProvider) == "" {
		cfg.LLM.DefaultProvider = providerID
	}

	if err := workorchconfig.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	_ = workorchconfig.Reload()

	printProviderResult(providerID, p)
	return nil
}

func runWizardMode(providerID string) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(os.Stderr, styleTitle.Render(fmt.Sprintf("  LLM Provider Setup: %s\n", providerID)))

	// Check if existing provider to update
	cfg, _ := workorchconfig.Current()
	existing, updating := cfg.LLM.Providers[providerID]
	if updating {
		fmt.Fprintf(os.Stderr, "  Updating existing provider.\n")
		fmt.Fprintf(os.Stderr, "  Press Enter to keep current value.\n\n")
	}

	// 1. Provider type
	defType := "compatible"
	if updating && existing.Provider != "" {
		defType = existing.Provider
	}
	fmt.Fprint(os.Stderr, styleDim.Render(fmt.Sprintf("  Provider type [%s]: ", defType)))
	providerType := readLine(reader, defType)

	// 2. Model
	defModel := ""
	if updating && existing.Model != "" {
		defModel = existing.Model
	}
	fmt.Fprint(os.Stderr, styleDim.Render(fmt.Sprintf("  Model name%s: ", placeholder(defModel))))
	model := readLine(reader, defModel)

	// 3. Base URL
	defURL := "http://localhost:11434/v1"
	if updating && existing.BaseURL != "" {
		defURL = existing.BaseURL
	}
	fmt.Fprint(os.Stderr, styleDim.Render(fmt.Sprintf("  Base URL [%s]: ", defURL)))
	baseURL := readLine(reader, defURL)

	// 4. API Key
	defKey := ""
	if updating && existing.APIKey != "" {
		defKey = "***"
	}
	prompt := fmt.Sprintf("  API key%s: ", placeholder(defKey))
	fmt.Fprint(os.Stderr, styleDim.Render(prompt))
	apiKey := readLine(reader, "")
	if apiKey == "" && updating {
		apiKey = existing.APIKey // keep existing
	}

	// 5. Timeout
	defTimeout := "120s"
	if updating && existing.Timeout != "" {
		defTimeout = existing.Timeout
	}
	fmt.Fprint(os.Stderr, styleDim.Render(fmt.Sprintf("  Timeout [%s]: ", defTimeout)))
	timeout := readLine(reader, defTimeout)

	// 6. Streaming
	defStreaming := "n"
	if updating && existing.Streaming != nil && *existing.Streaming {
		defStreaming = "y"
	}
	fmt.Fprint(os.Stderr, styleDim.Render(fmt.Sprintf("  Enable streaming? [%s]: ", defStreaming)))
	streamStr := readLine(reader, defStreaming)
	streaming := strings.HasPrefix(strings.ToLower(streamStr), "y")

	// 7. Default provider
	isDefault := !updating || cfg.LLM.DefaultProvider == providerID
	if !isDefault {
		fmt.Fprint(os.Stderr, styleDim.Render("  Set as default provider? [Y/n]: "))
		ans := readLine(reader, "y")
		isDefault = strings.HasPrefix(strings.ToLower(ans), "y")
	}

	fmt.Fprintln(os.Stderr)

	// Build config
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]workorchconfig.LLMProviderConfig{}
	}

	p := workorchconfig.LLMProviderConfig{
		Provider:        providerType,
		Model:           model,
		BaseURL:         baseURL,
		APIKey:          apiKey,
		Timeout:         timeout,
		ChatCompletions: boolPtr(true),
		Streaming:       &streaming,
	}

	cfg.LLM.Providers[providerID] = p
	if isDefault {
		cfg.LLM.DefaultProvider = providerID
	}

	if err := workorchconfig.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	_ = workorchconfig.Reload()

	printProviderResult(providerID, p)
	return nil
}

func readLine(r *bufio.Reader, def string) string {
	line, err := r.ReadString('\n')
	if err != nil || strings.TrimSpace(line) == "" {
		return def
	}
	return strings.TrimSpace(line)
}

func placeholder(s string) string {
	if s == "" {
		return ""
	}
	return ""
}

func printProviderResult(id string, p workorchconfig.LLMProviderConfig) {
	model := p.Model
	if model == "" {
		model = "(not set)"
	}
	status := styleSuccess.Render("enabled")
	if p.Disabled != nil && *p.Disabled {
		status = styleDim.Render("disabled")
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = styleDim.Render("(not set)")
	}
	fmt.Printf("LLM provider %s configured:\n", styleTitle.Render(id))
	fmt.Printf("  Provider: %s\n", p.Provider)
	fmt.Printf("  Model:    %s\n", model)
	fmt.Printf("  BaseURL:  %s\n", baseURL)
	fmt.Printf("  Status:   %s\n\n", status)
	fmt.Println(styleDim.Render("Restart workorch for changes to take effect."))
}

func init() {
	configSetLLMCmd.Flags().String("model", "", "Model name (e.g. qwen2.5, gpt-4o)")
	configSetLLMCmd.Flags().String("base-url", "", "Provider base URL")
	configSetLLMCmd.Flags().String("api-key", "", "API key")
	configSetLLMCmd.Flags().String("provider", "", "Provider type (openai, anthropic, compatible)")
	configSetLLMCmd.Flags().String("timeout", "120s", "Request timeout")
	configSetLLMCmd.Flags().Bool("disable", false, "Disable this provider")
	configSetLLMCmd.Flags().Bool("default", false, "Set as default provider")
	configSetLLMCmd.Flags().Bool("streaming", false, "Enable streaming responses")
}

func boolPtr(v bool) *bool { return &v }
