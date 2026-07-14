package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

var runCoverageCmd = &cobra.Command{
	Use:   "coverage <id>",
	Short: "Show run coverage progress",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := common.RunID(args[0])
		run, err := mustBackend().Run().Get(runCommandContext(cmd), runCommandWorkspaceID(cmd), runID)
		if err != nil {
			return fmt.Errorf("get run: %w", err)
		}
		snapshot, err := workspaceSnapshotForRun(run)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(snapshot.CoverageSummary)
			return nil
		}
		fmt.Println(styleTitle.Render("Run Coverage: " + string(runID)))
		coverageSummary := snapshot.CoverageSummary.Map()
		if summary := stringFromMap(coverageSummary, "summary"); summary != "" {
			fmt.Printf("  Summary: %s\n", summary)
		}
		if fullLedgerRef := stringFromMap(coverageSummary, "fullLedgerRef"); fullLedgerRef != "" {
			fmt.Printf("  Ledger: %s\n", fullLedgerRef)
		}
		if len(snapshot.DoDs) == 0 {
			fmt.Println(styleDim.Render("No DoD items found."))
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("DoD\tState\tItem"))
		for _, dod := range snapshot.DoDs {
			for idx, item := range dod.Items {
				fmt.Fprintf(w, "%s[%d]\t%s\t%s\n", dod.ID, idx+1, stateIcon(string(dod.State)), item)
			}
		}
		w.Flush()
		return nil
	},
}

var runTimelineCmd = &cobra.Command{
	Use:   "timeline <id>",
	Short: "Show run event timeline",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := common.RunID(args[0])
		items, err := listRunTimelineEvents(runCommandContext(cmd), runCommandWorkspaceID(cmd), runID)
		if err != nil {
			return err
		}
		if jsonOutput {
			printJSON(items)
			return nil
		}
		if len(items) == 0 {
			fmt.Println(styleDim.Render("No timeline events found."))
			return nil
		}
		fmt.Println(styleTitle.Render("Run Timeline: " + string(runID)))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, styleHeader.Render("Time\tTopic\tSubject\tSummary"))
		for _, item := range items {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				emptyDash(item.OccurredAt),
				item.Topic,
				emptyDash(item.SubjectID),
				eventRecordCompactSummary(item),
			)
		}
		w.Flush()
		return nil
	},
}

func workspaceSnapshotForRun(run *backend.RunInfo) (*backend.WorkspaceSnapshotInfo, error) {
	if run == nil {
		return nil, common.ErrInvalidArgument
	}
	snapshotBackend, ok := mustBackend().Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*backend.WorkspaceSnapshotInfo, error)
	})
	if !ok {
		return nil, fmt.Errorf("workspace snapshot is unavailable")
	}
	snapshot, err := snapshotBackend.Snapshot(ctx(), run.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace snapshot: %w", err)
	}
	return snapshot, nil
}

func listRunTimelineEvents(runCtx context.Context, workspaceID common.WorkspaceID, runID common.RunID) ([]backend.EventRecordInfo, error) {
	run, err := mustBackend().Run().Get(runCtx, workspaceID, runID)
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	snapshot, err := workspaceSnapshotForRun(run)
	if err != nil {
		return nil, err
	}
	return eventRecordsForRun(snapshot, runID), nil
}

func eventRecordsForRun(snapshot *backend.WorkspaceSnapshotInfo, runID common.RunID) []backend.EventRecordInfo {
	if snapshot == nil || len(snapshot.EventRecords) == 0 {
		return nil
	}
	ret := make([]backend.EventRecordInfo, 0, len(snapshot.EventRecords))
	for _, item := range snapshot.EventRecords {
		if item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret
}

func stringFromMap(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func eventRecordCompactSummary(item backend.EventRecordInfo) string {
	if summary := item.Payload.CompactSummary(); summary != "" {
		return summary
	}
	if strings.TrimSpace(item.Summary) != "" {
		return strings.TrimSpace(item.Summary)
	}
	return "-"
}
