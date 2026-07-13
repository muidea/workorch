package cli

import (
	"context"
	"fmt"
	"os"

	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	"github.com/spf13/cobra"
)

var runtimeV1RetirementApply bool
var runtimeV1RetirementDryRun bool
var runtimeV1RetirementBackupDir string
var runtimeV1RetirementRestoreFrom string

var runtimeV1RetirementCmd = &cobra.Command{
	Use:   "runtime-v1-retirement",
	Short: "Dry-run or apply the one-time Runtime V1 retirement migration",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if runtimeV1RetirementRestoreFrom != "" {
			return fmt.Errorf("restore must run before framework startup")
		}
		if runtimeV1RetirementApply && runtimeV1RetirementDryRun {
			return fmt.Errorf("--apply and --dry-run are mutually exclusive")
		}
		result, err := executeRuntimeV1Retirement(cmd.Context(), mustBackend(), runtimeV1RetirementApply)
		if err != nil {
			if backup := os.Getenv("WORKORCH_RUNTIME_V1_RETIREMENT_BACKUP"); backup != "" {
				return fmt.Errorf("%w; restore with: workorch runtime-v1-retirement --restore-from %s", err, backup)
			}
			return err
		}
		result.BackupDirectory = os.Getenv("WORKORCH_RUNTIME_V1_RETIREMENT_BACKUP")
		if jsonOutput {
			printJSON(result)
			return nil
		}
		mode := "dry-run"
		if !result.DryRun {
			mode = "apply"
		}
		fmt.Printf("Runtime V1 retirement %s: scanned=%d supersededRuns=%d clearedActiveRunIDs=%d supersededTickets=%d supersededApprovals=%d resourceScopes=%d releasedResources=%d projectionRevision=%d failures=%d backup=%s\n",
			mode, result.Report.ScannedV1Runs, result.Report.SupersededRuns, result.Report.ClearedActiveRunIDs,
			result.Report.SupersededTickets, result.Report.SupersededApprovals, result.Report.ResourceScopesToRelease,
			result.Report.ReleasedResourceScopes, result.Report.ProjectionRebuildRevision, len(result.Report.Failures), result.BackupDirectory)
		return nil
	},
}

func executeRuntimeV1Retirement(ctx context.Context, backend workorchport.Backend, apply bool) (workorchport.RuntimeV1RetirementResult, error) {
	retirement, ok := backend.(workorchport.RuntimeV1RetirementBackend)
	if !ok {
		return workorchport.RuntimeV1RetirementResult{}, fmt.Errorf("runtime-v1-retirement is only available in local embedded maintenance mode")
	}
	return retirement.RetireRuntimeV1(ctx, workorchport.RuntimeV1RetirementOptions{DryRun: !apply})
}

func init() {
	runtimeV1RetirementCmd.Flags().BoolVar(&runtimeV1RetirementDryRun, "dry-run", false, "preview the migration without changing state; this is the default")
	runtimeV1RetirementCmd.Flags().BoolVar(&runtimeV1RetirementApply, "apply", false, "apply the migration; default is dry-run")
	runtimeV1RetirementCmd.Flags().StringVar(&runtimeV1RetirementBackupDir, "backup-dir", "", "backup destination used before --apply")
	runtimeV1RetirementCmd.Flags().StringVar(&runtimeV1RetirementRestoreFrom, "restore-from", "", "restore a previous retirement backup before framework startup")
	rootCmd.AddCommand(runtimeV1RetirementCmd)
}
