package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestRuntimeTerminalReadDependenciesRejectInvalidOriginal(t *testing.T) {
	for _, name := range []string{"hash", "raw", "duplicate", "definition", "missing resource"} {
		t.Run(name, func(t *testing.T) {
			source := terminalSourceObligationFixture(t)
			set := source.Facts.Evidence.Accepted[0].OwnerRevisionSet
			switch name {
			case "hash":
				set.SetHash = "forged"
			case "raw":
				set.Entries[0].Revision = " " + set.Entries[0].Revision
			case "duplicate":
				set.Entries = append(set.Entries, set.Entries[0])
				*set = set.WithHash()
			case "definition":
				set.DefinitionRevision = "other"
				*set = set.WithHash()
			case "missing resource":
				set.Entries[0].ResourceID = ""
				*set = set.WithHash()
			}
			before := source.Clone()
			if _, err := DeriveRuntimeTerminalResultMaterialFromState(source); err == nil {
				t.Fatal("compiler normalized or admitted invalid original read dependency")
			}
			if !reflect.DeepEqual(source, before) {
				t.Fatal("failed compiler mutated original source")
			}
		})
	}
}

func TestRuntimeTerminalReadDependenciesDoNotInventMissingObservation(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	source.Facts.Evidence.Accepted[0].OwnerRevisionSet = nil
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	if material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSet != nil {
		t.Fatal("missing original snapshot became an observed empty dependency set")
	}
}

func TestRuntimeTerminalSealRejectsRehashedEvidenceReadSnapshot(t *testing.T) {
	source := terminalSourceObligationFixture(t)
	snapshot := &source.Decision.AuthoritySnapshot
	snapshot.WorkspaceID, snapshot.RunID = source.Facts.WorkspaceID, source.Facts.RunID
	snapshot.DefinitionRevision, snapshot.EvidenceRevision = source.Facts.DefinitionRevision, source.Facts.EvidenceRevision
	material, err := DeriveRuntimeTerminalResultMaterialFromState(source)
	if err != nil {
		t.Fatal(err)
	}
	outcome := material.RuntimeCloseout.TerminalOutcome
	seal := RuntimeTerminalMaterialSeal{
		Identity: RuntimeTerminalDeliveryIdentity{
			TerminalDeliveryID: StableTerminalDeliveryID(source.Facts.WorkspaceID, source.Facts.RunID, snapshot.ID, outcome),
			WorkspaceID:        source.Facts.WorkspaceID, RunID: source.Facts.RunID, AuthoritySnapshotID: snapshot.ID,
			DefinitionRevision: source.Facts.DefinitionRevision, EvidenceRevision: source.Facts.EvidenceRevision, DispatchID: snapshot.ID, Outcome: outcome,
		},
		GenerationStateRevision: source.StateRevision, RecoveryPolicy: source.Facts.RecoveryPolicy, RecoveryLedger: source.Facts.Recovery,
		Material: material,
	}
	seal.MaterialHash, err = seal.Material.CanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeTerminalMaterialSeal(&seal, source); err != nil {
		t.Fatalf("original seal: %v", err)
	}
	for _, name := range []string{"removed", "changed"} {
		t.Run(name, func(t *testing.T) {
			changed := seal.CloneRaw()
			item := &changed.Material.RuntimeCloseout.RequirementOutcomes[0].EvidenceItems[0]
			if name == "removed" {
				item.OwnerRevisionSet = nil
			} else {
				item.OwnerRevisionSet.Entries[0].Revision = "self-reported-replacement"
				*item.OwnerRevisionSet = item.OwnerRevisionSet.WithHash()
			}
			*changed.Material.RuntimeCloseout = changed.Material.RuntimeCloseout.WithCloseoutHash()
			changed.MaterialHash, err = changed.Material.CanonicalHash()
			if err != nil {
				t.Fatal(err)
			}
			if err := changed.ValidateRaw(); err != nil {
				t.Fatalf("test replacement must be self-consistent to exercise independent source guard: %v", err)
			}
			if ValidateRuntimeTerminalMaterialSeal(&changed, source) == nil {
				t.Fatal("fresh source accepted rehashed removal/replacement of original read dependency")
			}
		})
	}
}
