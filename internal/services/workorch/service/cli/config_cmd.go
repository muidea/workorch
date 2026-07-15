package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetAIProxyCmd)
	configCmd.AddCommand(configSetDefaultChatCmd)
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
		fmt.Printf("AI Proxy: %s\n", cfg.LLM.AIProxy.BaseURL)
		fmt.Printf("Default chat: %s/%s\n", cfg.LLM.DefaultChat.ProviderID, cfg.LLM.DefaultChat.ModelID)
		if cfg.LLM.DefaultEmbedding != nil {
			fmt.Printf("Default embedding: %s/%s\n", cfg.LLM.DefaultEmbedding.ProviderID, cfg.LLM.DefaultEmbedding.ModelID)
		}
		if len(cfg.LLM.CustomProviders) > 0 {
			fmt.Println("Custom providers:")
			for id, provider := range cfg.LLM.CustomProviders {
				fmt.Printf("  %s  %s  models=%d\n", styleTitle.Render(id), styleDim.Render(provider.Provider), len(provider.Models))
			}
		}
		return nil
	},
}

var configSetAIProxyCmd = &cobra.Command{
	Use:   "set-ai-proxy",
	Short: "Update built-in ai-proxy endpoint settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := workorchconfig.Current()
		if err != nil {
			return err
		}
		if baseURL, _ := cmd.Flags().GetString("base-url"); strings.TrimSpace(baseURL) != "" {
			cfg.LLM.AIProxy.BaseURL = strings.TrimSpace(baseURL)
		}
		if apiKey, _ := cmd.Flags().GetString("api-key"); cmd.Flags().Changed("api-key") {
			cfg.LLM.AIProxy.APIKey = strings.TrimSpace(apiKey)
		}
		if timeout, _ := cmd.Flags().GetString("timeout"); strings.TrimSpace(timeout) != "" {
			cfg.LLM.AIProxy.Timeout = strings.TrimSpace(timeout)
		}
		if err := workorchconfig.Save(cfg); err != nil {
			return err
		}
		if err := workorchconfig.Reload(); err != nil {
			return err
		}
		fmt.Printf("ai-proxy updated: baseURL=%s apiKeyConfigured=%t\n", cfg.LLM.AIProxy.BaseURL, strings.TrimSpace(cfg.LLM.AIProxy.APIKey) != "")
		return nil
	},
}

var configSetDefaultChatCmd = &cobra.Command{
	Use:   "set-default-chat",
	Short: "Set default chat provider/model target",
	RunE: func(cmd *cobra.Command, args []string) error {
		providerID, _ := cmd.Flags().GetString("provider-id")
		modelID, _ := cmd.Flags().GetString("model-id")
		providerID = strings.TrimSpace(providerID)
		modelID = strings.TrimSpace(modelID)
		if providerID == "" || modelID == "" {
			return fmt.Errorf("provider-id and model-id are required")
		}
		cfg, err := workorchconfig.Current()
		if err != nil {
			return err
		}
		cfg.LLM.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: providerID, ModelID: modelID}
		if err := workorchconfig.Save(cfg); err != nil {
			return err
		}
		if err := workorchconfig.Reload(); err != nil {
			return err
		}
		fmt.Printf("default chat target updated: %s/%s\n", providerID, modelID)
		return nil
	},
}

func init() {
	configSetAIProxyCmd.Flags().String("base-url", "", "ai-proxy base URL")
	configSetAIProxyCmd.Flags().String("api-key", "", "ai-proxy API key")
	configSetAIProxyCmd.Flags().String("timeout", "", "ai-proxy request timeout")
	configSetDefaultChatCmd.Flags().String("provider-id", workorchconfig.BuiltinAIProxyProviderID, "provider id")
	configSetDefaultChatCmd.Flags().String("model-id", "", "model id")
}
