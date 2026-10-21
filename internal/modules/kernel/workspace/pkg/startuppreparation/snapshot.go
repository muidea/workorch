// Package startuppreparation contains pure preparation contracts. It performs
// no resource observation, admission mutation or Runtime lifecycle operation.
package startuppreparation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const SchemaVersion = "workorch-startup-preparation/v1"

// ExactConfirmedStartupSnapshot is a detached read of the sealed input. Its
// hash binds content, not resource availability, no-Run, M3 or permission.
// The owner verifies the original complete authoring ledger before emitting it;
// the bounded snapshot does not export provider diagnostics or resource bytes.
type ExactConfirmedStartupSnapshot struct {
	SchemaVersion              string
	WorkspaceID                common.WorkspaceID
	WorkspaceRevision          uint64
	AuthoringRef               workspace.DefinitionAuthoringAdmissionRef
	AuthoringConfigurationHash string
	InternalDirectory          string
	Candidate                  workspace.DefinitionAdmissionCandidate
	StartupInput               workspace.WorkspaceStartupInputContract
	ActivationAdmission        workspace.WorkspaceActivationAdmission
	Goal                       workspace.Goal
	DoD                        workspace.DoD
	FormalDoDItemRefs          []string
	SnapshotHash               string
}

// FromOwnerPersistence must be called with the owner's actual formal rows,
// not the journal's expected after-images. It never materializes missing rows.
func FromOwnerPersistence(w *workspace.Workspace, goal *workspace.Goal, dod *workspace.DoD) (ExactConfirmedStartupSnapshot, error) {
	if w == nil || goal == nil || dod == nil || w.Revision == 0 || w.DefinitionAuthoringConfirmation == nil || w.DefinitionChangeCandidate != nil || w.Deletion != nil {
		return ExactConfirmedStartupSnapshot{}, common.ErrConflict
	}
	candidate, present, err := w.ValidateActivationAdmissionPersistence()
	if err != nil || !present || candidate.AuthoringRef == nil || w.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
		return ExactConfirmedStartupSnapshot{}, common.ErrConflict
	}
	journal := w.DefinitionAuthoringConfirmation
	if journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized ||
		!reflect.DeepEqual(*goal, journal.Goal) || !reflect.DeepEqual(*dod, journal.DoD) ||
		workspace.WorkspaceConfigurationHash(w) != journal.ConfigurationHash ||
		w.DeliverableDirectory != w.StartupInput.ResolvedProjectWorkspaceDirectory ||
		w.TemplateID != candidate.WorkspaceTemplateSnapshot.TemplateRef || w.InternalDirectory == "" {
		return ExactConfirmedStartupSnapshot{}, common.ErrConflict
	}
	snapshot := ExactConfirmedStartupSnapshot{
		SchemaVersion: SchemaVersion, WorkspaceID: w.ID, WorkspaceRevision: w.Revision,
		AuthoringRef: *candidate.AuthoringRef, AuthoringConfigurationHash: journal.ConfigurationHash,
		InternalDirectory: w.InternalDirectory, Candidate: candidate.Clone(), StartupInput: w.StartupInput.Clone(),
		ActivationAdmission: *w.ActivationAdmission, Goal: *goal, DoD: *workspace.CloneDoD(dod),
	}
	_, items, _, _, err := workspace.ConfirmedAuthoringAdmissionSource(w.DefinitionAuthoring, *journal, w.ID)
	if err != nil {
		return ExactConfirmedStartupSnapshot{}, common.ErrConflict
	}
	for _, item := range items {
		snapshot.FormalDoDItemRefs = append(snapshot.FormalDoDItemRefs, item.CandidateItemRef)
	}
	snapshot.SnapshotHash = snapshot.contentHash()
	if err := snapshot.Validate(); err != nil {
		return ExactConfirmedStartupSnapshot{}, err
	}
	return snapshot, nil
}

func (s ExactConfirmedStartupSnapshot) Clone() ExactConfirmedStartupSnapshot {
	s.Candidate = s.Candidate.Clone()
	s.StartupInput = s.StartupInput.Clone()
	s.DoD = *workspace.CloneDoD(&s.DoD)
	s.FormalDoDItemRefs = slices.Clone(s.FormalDoDItemRefs)
	return s
}

func (s ExactConfirmedStartupSnapshot) Validate() error {
	if s.SchemaVersion != SchemaVersion || s.WorkspaceID == "" || s.WorkspaceRevision == 0 || s.AuthoringRef.Validate() != nil || s.AuthoringConfigurationHash == "" || s.InternalDirectory == "" {
		return fmt.Errorf("%w: exact startup snapshot identity is incomplete", common.ErrConflict)
	}
	// encoding/json replaces invalid UTF-8 instead of preserving its bytes.
	// Reject unrepresentable owner paths before hashing/transport, rather than
	// silently aliasing distinct directory inputs or repairing their content.
	if !utf8.ValidString(s.InternalDirectory) || strings.ContainsRune(s.InternalDirectory, 0) {
		return fmt.Errorf("%w: exact startup snapshot directory is not representable", common.ErrConflict)
	}
	if s.Candidate.WorkspaceID != s.WorkspaceID || s.StartupInput.WorkspaceID != s.WorkspaceID ||
		s.ActivationAdmission.WorkspaceID != s.WorkspaceID || s.Candidate.AuthoringRef == nil || *s.Candidate.AuthoringRef != s.AuthoringRef ||
		s.ActivationAdmission.State != workspace.WorkspaceActivationEligible || s.ActivationAdmission.ValidateCandidate(s.StartupInput, s.Candidate) != nil ||
		s.StartupInput.SpecificationHash != s.AuthoringRef.SpecificationHash {
		return fmt.Errorf("%w: exact startup snapshot seal mismatch", common.ErrConflict)
	}
	if s.Goal.WorkspaceID != s.WorkspaceID || s.DoD.WorkspaceID != s.WorkspaceID || s.Goal.ID == "" || s.DoD.ID == "" ||
		s.Goal.LineageState != "active" || s.DoD.LineageState != "active" || s.DoD.State != common.DoDStateReady ||
		contracts.ExecutionDefinitionRevision(string(s.Goal.ID), s.Goal.DefinitionRevision, string(s.DoD.ID), s.DoD.DefinitionRevision) != s.StartupInput.DefinitionRevision ||
		s.Goal.Text != s.Candidate.GoalInput.Text || workspace.StableDefinitionTextHash(s.Goal.Text) != s.StartupInput.GoalTextHash ||
		workspace.StableDefinitionStringSetHash(s.DoD.Items) != s.StartupInput.DoDItemSetHash {
		return fmt.Errorf("%w: exact startup snapshot formal rows mismatch", common.ErrConflict)
	}
	// Candidate refs have canonical order, while formal DoD preserves the
	// original reviewed order. Recover that order by exact source identity.
	if s.Candidate.Specification == nil || len(s.DoD.Items) != len(s.Candidate.DoDInputs) || len(s.FormalDoDItemRefs) != len(s.DoD.Items) {
		return fmt.Errorf("%w: exact startup snapshot lacks reviewed DoD", common.ErrConflict)
	}
	byRef := make(map[string]string, len(s.Candidate.DoDInputs))
	for _, item := range s.Candidate.DoDInputs {
		if item.SourceIdentity != "dod:"+string(s.DoD.ID)+"@"+s.DoD.DefinitionRevision {
			return fmt.Errorf("%w: exact startup snapshot DoD source mismatch", common.ErrConflict)
		}
		byRef[item.CandidateItemRef] = item.Text
	}
	for index, ref := range s.FormalDoDItemRefs {
		text, present := byRef[ref]
		if !present || text != s.DoD.Items[index] {
			return fmt.Errorf("%w: exact startup snapshot changed reviewed DoD order", common.ErrConflict)
		}
		delete(byRef, ref)
	}
	if s.Candidate.GoalInput.SourceIdentity != "goal:"+string(s.Goal.ID)+"@"+s.Goal.DefinitionRevision {
		return fmt.Errorf("%w: exact startup snapshot Goal source mismatch", common.ErrConflict)
	}
	// Compile only the already sealed declaration. This pure compiler cannot
	// create an admission, a ticket, an owner fact or a Run.
	result, err := definitionadmission.Compile(s.Candidate)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		return fmt.Errorf("%w: exact startup snapshot is not complete all_of", common.ErrConflict)
	}
	if s.SnapshotHash == "" || s.SnapshotHash != s.contentHash() {
		return fmt.Errorf("%w: exact startup snapshot content hash mismatch", common.ErrConflict)
	}
	return nil
}

func (s ExactConfirmedStartupSnapshot) contentHash() string {
	s.SnapshotHash = ""
	encoded, err := json.Marshal(s)
	if err != nil || len(encoded) > 2*1024*1024 {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return "startup-snapshot-sha256:" + hex.EncodeToString(digest[:])
}
