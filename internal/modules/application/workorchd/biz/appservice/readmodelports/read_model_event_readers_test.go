package readmodelports

import (
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestNewOwnerReadersRequiresHub(t *testing.T) {
	if _, err := NewOwnerReaders(nil); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("NewOwnerReaders(nil) error = %v, want ErrInvalidArgument", err)
	}
}

func TestNewOwnerReadersBindsEveryReaderToHub(t *testing.T) {
	hub := magicevent.NewHub(8)

	readers, err := NewOwnerReaders(hub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}

	if readers.Workspaces.Hub != hub ||
		readers.Goals.Hub != hub ||
		readers.DoDs.Hub != hub ||
		readers.Runs.Hub != hub ||
		readers.Steps.Hub != hub ||
		readers.Results.Hub != hub ||
		readers.Artifacts.Hub != hub ||
		readers.ArtifactContinuations.Hub != hub ||
		readers.CoverageWorkItems.Hub != hub ||
		readers.EventRecords.Hub != hub ||
		readers.MemoryRecalls.Hub != hub ||
		readers.ToolCalls.Hub != hub ||
		readers.MCPCapabilities.Hub != hub ||
		readers.Patches.Hub != hub ||
		readers.Validations.Hub != hub ||
		readers.Checkpoints.Hub != hub ||
		readers.AutoGovernanceTraces.Hub != hub ||
		readers.Tickets.Hub != hub ||
		readers.Resolutions.Hub != hub ||
		readers.Approvals.Hub != hub {
		t.Fatalf("NewOwnerReaders did not bind every reader to the provided hub")
	}
}

func TestOwnerReadersBuildsSnapshotAndStatusReaderBundles(t *testing.T) {
	hub := magicevent.NewHub(8)
	readers, err := NewOwnerReaders(hub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}
	publishedEvent, ok := events.DecodeWorkorchEvent(events.Envelope{
		Version: events.WorkorchEventVersion,
		Topic:   runevents.TopicRunCreated,
		RunID:   "run-001",
		Payload: map[string]any{
			events.PayloadKeyGoalID:             "goal-001",
			events.PayloadKeyDoDID:              "dod-001",
			events.PayloadKeyState:              string(common.RunStateActive),
			events.PayloadKeyBlockingTicketID:   "",
			events.PayloadKeyBlockingApprovalID: "",
		},
	})
	if !ok {
		t.Fatalf("DecodeWorkorchEvent returned !ok")
	}
	published := []events.WorkorchEvent{publishedEvent}

	runReaders := readers.RunSnapshotReaders(published)
	if runReaders.Workspaces != readers.Workspaces ||
		runReaders.Runs != readers.Runs ||
		runReaders.Steps != readers.Steps ||
		runReaders.ToolCalls != readers.ToolCalls ||
		runReaders.Patches != readers.Patches ||
		runReaders.Validations != readers.Validations ||
		runReaders.Checkpoints != readers.Checkpoints ||
		runReaders.Results != readers.Results ||
		runReaders.Artifacts != readers.Artifacts ||
		runReaders.ArtifactContinuations != readers.ArtifactContinuations ||
		runReaders.CoverageWorkItems != readers.CoverageWorkItems ||
		runReaders.EventRecords != readers.EventRecords ||
		runReaders.MemoryRecalls != readers.MemoryRecalls ||
		runReaders.Tickets != readers.Tickets ||
		runReaders.Resolutions != readers.Resolutions ||
		runReaders.Approvals != readers.Approvals ||
		runReaders.AutoGovernanceTraces != readers.AutoGovernanceTraces ||
		runReaders.MCPCapabilities != readers.MCPCapabilities ||
		len(runReaders.Published) != 1 ||
		runReaders.Published[0].Topic() != runevents.TopicRunCreated {
		t.Fatalf("RunSnapshotReaders did not preserve owner readers and published events")
	}

	workspaceReaders := readers.WorkspaceSnapshotReaders(published)
	if workspaceReaders.Workspaces != readers.Workspaces ||
		workspaceReaders.Goals != readers.Goals ||
		workspaceReaders.DoDs != readers.DoDs ||
		workspaceReaders.Runs != readers.Runs ||
		workspaceReaders.Results != readers.Results ||
		workspaceReaders.Artifacts != readers.Artifacts ||
		workspaceReaders.ArtifactContinuations != readers.ArtifactContinuations ||
		workspaceReaders.CoverageWorkItems != readers.CoverageWorkItems ||
		workspaceReaders.EventRecords != readers.EventRecords ||
		workspaceReaders.Patches != readers.Patches ||
		workspaceReaders.Tickets != readers.Tickets ||
		workspaceReaders.Resolutions != readers.Resolutions ||
		workspaceReaders.Approvals != readers.Approvals ||
		workspaceReaders.AutoGovernanceTraces != readers.AutoGovernanceTraces ||
		workspaceReaders.RunSnapshotReaders.Runs != readers.Runs ||
		len(workspaceReaders.RunSnapshotReaders.Published) != 1 {
		t.Fatalf("WorkspaceSnapshotReaders did not preserve owner readers and nested run readers")
	}

	statusReaders := readers.RunStatusReaders()
	if statusReaders.Runs != readers.Runs ||
		statusReaders.Steps != readers.Steps ||
		statusReaders.Checkpoints != readers.Checkpoints ||
		statusReaders.Tickets != readers.Tickets ||
		statusReaders.Approvals != readers.Approvals {
		t.Fatalf("RunStatusReaders did not preserve owner readers")
	}
}

func TestOwnerReaderCandidatesReuseOwnerBundlesWhenAllReadersMatch(t *testing.T) {
	hub := magicevent.NewHub(8)
	readers, err := NewOwnerReaders(hub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}
	candidates := OwnerReaderCandidates{
		Workspaces:            readers.Workspaces,
		Goals:                 readers.Goals,
		DoDs:                  readers.DoDs,
		Runs:                  readers.Runs,
		Steps:                 readers.Steps,
		Results:               readers.Results,
		Artifacts:             readers.Artifacts,
		ArtifactContinuations: readers.ArtifactContinuations,
		CoverageWorkItems:     readers.CoverageWorkItems,
		EventRecords:          readers.EventRecords,
		MemoryRecalls:         readers.MemoryRecalls,
		ToolCalls:             readers.ToolCalls,
		MCPCapabilities:       readers.MCPCapabilities,
		Patches:               readers.Patches,
		Validations:           readers.Validations,
		Checkpoints:           readers.Checkpoints,
		AutoGovernanceTraces:  readers.AutoGovernanceTraces,
		Tickets:               readers.Tickets,
		Resolutions:           readers.Resolutions,
		Approvals:             readers.Approvals,
	}

	if !candidates.Matches(readers) {
		t.Fatalf("candidates should match owner readers")
	}
	if candidates.RunSnapshotReaders(readers, nil).Runs != readers.Runs ||
		candidates.WorkspaceSnapshotReaders(readers, nil).Runs != readers.Runs ||
		candidates.RunStatusReaders(readers).Runs != readers.Runs {
		t.Fatalf("candidates did not reuse owner reader bundles")
	}
}

func TestOwnerReaderCandidatesUseFallbackBundlesWhenReadersDiffer(t *testing.T) {
	hub := magicevent.NewHub(8)
	readers, err := NewOwnerReaders(hub)
	if err != nil {
		t.Fatalf("NewOwnerReaders: %v", err)
	}
	otherHub := magicevent.NewHub(8)
	otherRunReader := RunReader{Hub: otherHub}
	candidates := OwnerReaderCandidates{
		Workspaces:            readers.Workspaces,
		Goals:                 readers.Goals,
		DoDs:                  readers.DoDs,
		Runs:                  otherRunReader,
		Steps:                 readers.Steps,
		Results:               readers.Results,
		Artifacts:             readers.Artifacts,
		ArtifactContinuations: readers.ArtifactContinuations,
		CoverageWorkItems:     readers.CoverageWorkItems,
		EventRecords:          readers.EventRecords,
		MemoryRecalls:         readers.MemoryRecalls,
		ToolCalls:             readers.ToolCalls,
		MCPCapabilities:       readers.MCPCapabilities,
		Patches:               readers.Patches,
		Validations:           readers.Validations,
		Checkpoints:           readers.Checkpoints,
		AutoGovernanceTraces:  readers.AutoGovernanceTraces,
		Tickets:               readers.Tickets,
		Resolutions:           readers.Resolutions,
		Approvals:             readers.Approvals,
	}

	if candidates.Matches(readers) {
		t.Fatalf("candidates should not match owner readers when one reader differs")
	}
	if candidates.RunSnapshotReaders(readers, nil).Runs != otherRunReader ||
		candidates.WorkspaceSnapshotReaders(readers, nil).Runs != otherRunReader ||
		candidates.RunStatusReaders(readers).Runs != otherRunReader {
		t.Fatalf("candidates did not use fallback reader bundles")
	}
}
