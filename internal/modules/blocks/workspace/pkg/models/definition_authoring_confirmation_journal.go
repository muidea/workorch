package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type DefinitionAuthoringConfirmationPhase string

const (
	DefinitionAuthoringConfirmationPrepared     DefinitionAuthoringConfirmationPhase = "prepared"
	DefinitionAuthoringConfirmationMaterialized DefinitionAuthoringConfirmationPhase = "materialized"
)

// DefinitionAuthoringConfirmation is an owner-private durable journal. Goal and
// DoD are deterministic after-images, not an admission or a Run authorization.
// Replaying a prepared journal compares existing rows before completing it.
type DefinitionAuthoringConfirmation struct {
	Phase                DefinitionAuthoringConfirmationPhase
	InputHash            string
	DerivationPermitID   string
	ProposalHash         string
	SpecificationHash    string
	TemplateSnapshotHash string
	ResourceSnapshotHash string
	ConfigurationHash    string
	Goal                 Goal
	DoD                  DoD
	PreviousInputHash    string
	FormalSequence       uint64
}

func (c DefinitionAuthoringConfirmation) Clone() DefinitionAuthoringConfirmation {
	c.DoD.Items = slices.Clone(c.DoD.Items)
	return c
}

func StableDefinitionAuthoringConfirmationID(workspaceID common.WorkspaceID, inputHash, derivationPermitID, specificationHash string) string {
	h := sha256.New()
	writeDefinitionHash(h, "workspace-authoring-confirmation-v1", string(workspaceID), inputHash, derivationPermitID, specificationHash)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func authoringFormalRevision(prefix, text string) string {
	return authoringFormalRevisionAt(prefix, 1, text)
}

func authoringFormalRevisionAt(prefix string, sequence uint64, text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%s:%020d:%s", prefix, sequence, hex.EncodeToString(sum[:]))
}

func (c DefinitionAuthoringConfirmation) Validate(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID) error {
	_, _, err := c.validatedSpecification(record, workspaceID)
	return err
}

func (c DefinitionAuthoringConfirmation) validateMode(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID, view bool) error {
	_, _, err := c.validatedSpecificationMode(record, workspaceID, view)
	return err
}
func (c DefinitionAuthoringConfirmation) ValidateView(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID) error {
	_, _, err := c.validatedSpecificationMode(record, workspaceID, true)
	return err
}
func (c DefinitionAuthoringConfirmation) validatedSpecification(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID) (GoalSpecification, []DefinitionCandidateItem, error) {
	return c.validatedSpecificationMode(record, workspaceID, false)
}
func (c DefinitionAuthoringConfirmation) validatedSpecificationMode(record *DefinitionAuthoringRecord, workspaceID common.WorkspaceID, view bool) (GoalSpecification, []DefinitionCandidateItem, error) {
	if record == nil || record.Input.WorkspaceID != workspaceID || record.validate(view) != nil || workspaceID == "" || c.InputHash != record.InputHash || c.TemplateSnapshotHash != record.TemplateSnapshotHash || c.ConfigurationHash == "" ||
		(c.Phase != DefinitionAuthoringConfirmationPrepared && c.Phase != DefinitionAuthoringConfirmationMaterialized) {
		return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal has invalid input or phase")
	}
	derivation, specification, err := record.confirmableSpecification(c.DerivationPermitID, c.ProposalHash)
	if err != nil || specification.AdmissionHash() != c.SpecificationHash || c.ResourceSnapshotHash != derivation.ResourceSnapshotHash {
		return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal has no exact reviewed specification or resource snapshot")
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil || len(items) != len(c.DoD.Items) || len(items) == 0 {
		return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal lost derived DoD")
	}
	for n, item := range items {
		if c.DoD.Items[n] != item.Text {
			return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal changed derived DoD item")
		}
	}
	sequence := c.FormalSequence
	if sequence == 0 && c.PreviousInputHash == "" {
		sequence = 1
	}
	if sequence == 0 || sequence == 1 && c.PreviousInputHash != "" || sequence > 1 && c.PreviousInputHash == "" || sequence > 1 && c.PreviousInputHash == c.InputHash {
		return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal has invalid formal lineage")
	}
	previousGoal, previousDoD := common.GoalID(""), common.DoDID("")
	if sequence > 1 {
		previousGoal = c.Goal.PreviousGoalID
		previousDoD = c.DoD.PreviousDoDID
		if previousGoal == "" || previousDoD == "" || c.Goal.PreviousDefinitionHash == "" || c.DoD.PreviousDefinitionHash == "" {
			return GoalSpecification{}, nil, fmt.Errorf("authoring successor journal lacks predecessor definition identity")
		}
	} else if c.Goal.PreviousGoalID != "" || c.DoD.PreviousDoDID != "" || c.Goal.PreviousDefinitionHash != "" || c.DoD.PreviousDefinitionHash != "" {
		return GoalSpecification{}, nil, fmt.Errorf("initial authoring journal claims a predecessor")
	}
	if c.Goal.WorkspaceID != workspaceID || c.DoD.WorkspaceID != workspaceID || c.Goal.ID != common.GoalID("authoring-goal-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.DoD.ID != common.DoDID("authoring-dod-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.Goal.Text != strings.TrimSpace(record.Input.GoalText) || c.Goal.DefinitionRevision != authoringFormalRevisionAt("goal", sequence, c.Goal.Text) || c.DoD.DefinitionRevision != authoringFormalRevisionAt("dod", sequence, strings.Join(c.DoD.Items, "\n")) || c.Goal.LineageState != "active" || c.DoD.LineageState != "active" || c.DoD.State != common.DoDStateReady {
		return GoalSpecification{}, nil, fmt.Errorf("authoring confirmation journal contains invalid formal after-images")
	}
	return specification, items, nil
}

func (c DefinitionAuthoringConfirmation) ValidateSuccessor(predecessor DefinitionAuthoringConfirmation) error {
	if c.FormalSequence < 2 || predecessor.Phase != DefinitionAuthoringConfirmationMaterialized || c.PreviousInputHash != predecessor.InputHash || c.Goal.PreviousGoalID != predecessor.Goal.ID || c.DoD.PreviousDoDID != predecessor.DoD.ID || c.Goal.PreviousDefinitionHash != authoringFormalHash(predecessor.Goal.DefinitionRevision) || c.DoD.PreviousDefinitionHash != authoringFormalHash(predecessor.DoD.DefinitionRevision) {
		return fmt.Errorf("authoring confirmation predecessor does not match archived definition")
	}
	previousSequence := predecessor.FormalSequence
	if previousSequence == 0 {
		previousSequence = 1
	}
	if c.FormalSequence != previousSequence+1 {
		return fmt.Errorf("authoring confirmation formal sequence is not contiguous")
	}
	return nil
}

func authoringFormalHash(revision string) string {
	parts := strings.SplitN(revision, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	if sequence, err := strconv.ParseUint(parts[1], 10, 64); err != nil || sequence == 0 {
		return ""
	}
	return parts[2]
}
