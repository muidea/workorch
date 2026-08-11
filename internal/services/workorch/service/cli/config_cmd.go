package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func init() {
	configCmd.AddCommand(configShowCmd)
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
		return nil
	},
}
