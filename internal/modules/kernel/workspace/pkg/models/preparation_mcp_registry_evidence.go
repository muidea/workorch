package workspace

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	connectionevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationMCPRegistryEvidenceVersion = "workorch-preparation-mcp-registry-evidence/v1"
const PreparationMCPRegistryRefPrefix = "mcp-registry-observation:"

// Complete original double-read registry, not selected-only data, connection
// evidence, continuous freshness or a readiness verdict.
type PreparationMCPObservationFacts struct {
	Version                              string
	WorkspaceID                          common.WorkspaceID
	WorkspaceRevision                    uint64
	SnapshotHash, ReobservedSnapshotHash string
	Before, After                        mcpevents.RegistrySnapshotResult
	StartedAt, CompletedAt               time.Time
}

func (f PreparationMCPObservationFacts) Clone() PreparationMCPObservationFacts {
	f.Before, f.After = f.Before.Clone(), f.After.Clone()
	return f
}
func (f PreparationMCPObservationFacts) Validate() error {
	if f.Version != PreparationMCPRegistryEvidenceVersion || !boundedPreparationIdentity(string(f.WorkspaceID)) || f.WorkspaceRevision == 0 || !boundedPreparationIdentity(f.SnapshotHash) || f.SnapshotHash != f.ReobservedSnapshotHash || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || f.Before.ValidatePreparation() != nil || f.After.ValidatePreparation() != nil || !reflect.DeepEqual(f.Before, f.After) {
		return common.ErrConflict
	}
	return nil
}

type WorkspacePreparationMCPRegistryEvidence struct {
	Connection                 *connectionevents.ObservePreparationResult `json:",omitempty"`
	Version, Ref               string
	RequestID, PayloadHash     string
	Binding                    WorkspacePreparationBinding
	Facts                      PreparationMCPObservationFacts
	ContentHash                string
	Selection                  *PreparationMCPSelectionCompilation `json:"Selection,omitempty"`
	SelectionEvaluationVersion string                              `json:"SelectionEvaluationVersion,omitempty"`
}

func ValidPreparationMCPRegistryRef(ref string) bool {
	return strings.HasPrefix(ref, PreparationMCPRegistryRefPrefix) && preparationFenceHex(strings.TrimPrefix(ref, PreparationMCPRegistryRefPrefix), 16)
}
func (e WorkspacePreparationMCPRegistryEvidence) Clone() WorkspacePreparationMCPRegistryEvidence {
	e.Facts = e.Facts.Clone()
	if e.Connection != nil {
		c := e.Connection.Clone()
		e.Connection = &c
	}
	if e.Selection != nil {
		c := e.Selection.Clone()
		e.Selection = &c
	}
	return e
}
func (e WorkspacePreparationMCPRegistryEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}
func (e WorkspacePreparationMCPRegistryEvidence) Validate() error {
	if e.Connection != nil && !e.validConnectionBinding() {
		return common.ErrConflict
	}
	if e.SelectionEvaluationVersion != "" && (e.SelectionEvaluationVersion != PreparationMCPSelectionEvaluationVersion || e.Selection == nil || e.BlockedReason() == "") {
		return common.ErrConflict
	}
	if e.Selection != nil && (e.Selection.Version != PreparationMCPSelectionVersion || len(e.Selection.SelectedServerIDs) > 256 || len(e.Selection.SelectedCapabilityIDs) > 4096 || len(e.Selection.Selected.Servers) > 256 || len(e.Selection.Selected.Capabilities) > 4096) {
		return common.ErrConflict
	}
	if e.Version != PreparationMCPRegistryEvidenceVersion || !ValidPreparationMCPRegistryRef(e.Ref) || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.Binding.Validate() != nil || e.Facts.Validate() != nil || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	return nil
}
func ClonePreparationMCPRegistryEvidence(items []WorkspacePreparationMCPRegistryEvidence) []WorkspacePreparationMCPRegistryEvidence {
	if items == nil {
		return nil
	}
	out := make([]WorkspacePreparationMCPRegistryEvidence, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}
func validatePreparationMCPRegistryEvidencePersistence(w *Workspace) error {
	if w == nil || len(w.PreparationMCPRegistryEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			for _, check := range a.Checks {
				if check.ID == "mcp_selection" && PreparationMCPReasonRequiresEvidence(check.Reason) && a.MCPRegistryEvidenceRef == "" {
					return common.ErrConflict
				}
			}
			if a.MCPRegistryEvidenceRef != "" {
				if _, exists := refs[a.MCPRegistryEvidenceRef]; exists {
					return common.ErrConflict
				}
				refs[a.MCPRegistryEvidenceRef] = a
			}
		}
	}
	if len(refs) != len(w.PreparationMCPRegistryEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationMCPRegistryEvidence {
		a, ok := refs[e.Ref]
		f := e.Facts
		if !ok || e.Validate() != nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || f.WorkspaceID != w.ID || f.WorkspaceRevision != a.ExpectedWorkspaceRevision+1 || f.SnapshotHash != a.ObservationSnapshotHash || !reflect.DeepEqual(e.Binding, a.Binding) || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || f.StartedAt.Before(a.StartedAt) || f.CompletedAt.After(a.CompletedAt) {
			return common.ErrConflict
		}
		found := false
		for _, c := range a.Checks {
			if c.ID == "mcp_selection" {
				if c.State == "passed" && (c.Reason == PreparationEmptyMCPReason || c.Reason == PreparationMCPConnectionReason && e.Connection != nil && e.Connection.Satisfied()) && ValidPreparationEvaluatedCheck(a, c) {
					found = true
					continue
				}
				if c.State != "blocked" || c.Owner != "mcpregistry" || !c.Required || c.Reason != e.BlockedReason() && !(c.Reason == "binding_changed" && e.SelectionEvaluationVersion == "") {
					return common.ErrConflict
				}
				found = true
			}
		}
		if e.Connection != nil && (e.Connection.StartedAt.Before(a.StartedAt) || e.Connection.CompletedAt.After(a.CompletedAt)) {
			return common.ErrConflict
		}
		if !found {
			return common.ErrConflict
		}
		if !reflect.DeepEqual(a.Checks, EvaluatePreparationMCPConnection(w, a)) {
			return common.ErrConflict
		}
		delete(refs, e.Ref)
	}
	return nil
}
func validatePreparationMCPRegistryEvidenceTransition(current, next *Workspace, cas bool) error {
	if validatePreparationMCPRegistryEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationMCPRegistryEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if validatePreparationMCPRegistryEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationMCPRegistryEvidence, next.PreparationMCPRegistryEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) > 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a, b := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1], next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted || b.State != WorkspacePreparationBlocked || a.RequestID != b.RequestID || a.MCPRegistryEvidenceRef != "" || b.MCPRegistryEvidenceRef != after[len(after)-1].Ref {
		return common.ErrConflict
	}
	if compiled := after[len(after)-1].Selection; compiled != nil {
		if after[len(after)-1].SelectionEvaluationVersion != PreparationMCPSelectionEvaluationVersion {
			return common.ErrConflict
		}
		// Scoped metadata excludes separately persisted Soul bindings. Derive
		// declarations from the real preimage, but take full configuration
		// identity from the immutable, already accepted operation binding.
		expected := PreparationMCPDefinition(current)
		expected.ConfigurationHash = a.Binding.AuthoringConfigurationHash
		if !reflect.DeepEqual(compiled.Definition, expected) {
			return common.ErrConflict
		}
	}
	return nil
}

const PreparationMCPConnectionReason = "mcp_connection_observed_remaining_checks"

func (e WorkspacePreparationMCPRegistryEvidence) validConnectionBinding() bool {
	r := e.Connection
	if r == nil || r.ValidateFor(r.Command) != nil || e.Selection == nil || e.Selection.FailureCode != "" {
		return false
	}
	c := r.Command
	var caps []connectionevents.PreparationCapability
	for _, cap := range e.Selection.Selected.Capabilities {
		caps = append(caps, connectionevents.PreparationCapability{ID: string(cap.ID), ServerID: string(cap.ServerID), Kind: cap.Kind, Name: cap.Name, URI: cap.URI})
	}
	return c.WorkspaceID == e.Facts.WorkspaceID && c.WorkspaceRevision == e.Facts.WorkspaceRevision && c.RequestID == e.RequestID && c.PayloadHash == e.PayloadHash && c.SnapshotHash == e.Facts.SnapshotHash && c.RegistryRevision == e.Selection.Selected.RegistryRevision && reflect.DeepEqual(c.ServerIDs, e.Selection.SelectedServerIDs) && reflect.DeepEqual(c.Capabilities, caps)
}
func EvaluatePreparationMCPConnection(w *Workspace, a WorkspacePreparationAttempt) []WorkspacePreparationCheckResult {
	checks := append([]WorkspacePreparationCheckResult(nil), a.Checks...)
	for _, e := range w.PreparationMCPRegistryEvidence {
		if e.Ref != a.MCPRegistryEvidenceRef || e.Connection == nil {
			continue
		}
		exactDirectory := false
		for _, d := range w.PreparationDirectoryEvidence {
			if d.ContentHash == a.DirectoryEvidenceHash && e.Connection.Command.InternalDirectory == d.Internal.Path && e.Connection.Command.DeliverableDirectory == d.Deliverable.Path {
				exactDirectory = true
			}
		}
		if !exactDirectory || e.Validate() != nil || !e.Connection.Satisfied() {
			continue
		}
		for i, c := range checks {
			if c.ID == "mcp_selection" && c.Reason != "binding_changed" && c.Reason != "original_check_interrupted" {
				checks[i].State = "passed"
				checks[i].Reason = PreparationMCPConnectionReason
			}
		}
	}
	return checks
}
