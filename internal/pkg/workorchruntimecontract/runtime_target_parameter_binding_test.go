package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func targetParameterTestInputs(t *testing.T, kind, variant string, targets []string) (RuntimeDecision, RuntimeCandidateManifestOwnerFacts) {
	t.Helper()
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds: []string{kind}, TargetRefs: targets, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		},
	})
	if variant != "" {
		decision.RequiredOutput.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: kind, VariantID: variant}}
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "file-catalog", OwnerModuleID: RuntimeModuleFile, OperationKinds: []string{kind},
		TargetRefs: []string{"session.txt", "session_manager.txt"}, ObservedRevision: "tree-1",
		TargetParameterBindings: []RuntimeTargetParameterBinding{{Kind: kind, Name: "baseHash", Values: []RuntimeTargetParameterValue{
			{TargetRef: "session.txt", Value: "session-preimage"}, {TargetRef: "session_manager.txt", Value: "manager-preimage"},
		}}},
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}}
}

func TestTargetParameterBindingCompilesExactSelectedPreimageAndRejectsProviderEcho(t *testing.T) {
	for _, kind := range []string{StepKindFileEdit, RuntimeBlockKindPatchApply} {
		for _, singleton := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/singleton=%t", kind, singleton), func(t *testing.T) {
				targets := []string{"session.txt", "session_manager.txt"}
				if singleton {
					targets = targets[:1]
				}
				variant := ""
				if kind == RuntimeBlockKindPatchApply {
					variant = "text_replace"
				}
				decision, owner := targetParameterTestInputs(t, kind, variant, targets)
				manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
				if err != nil {
					t.Fatal(err)
				}
				wire, err := BuildRuntimeCandidateWireContract(manifest)
				if err != nil {
					t.Fatal(err)
				}
				rule, _ := RuntimeCandidateWireRuleForAuxiliaryKind(wire, kind)
				if containsString(rule.ProviderParameterNames, "baseHash") || !containsString(rule.DerivedParameterNames(), "baseHash") {
					t.Fatalf("owner precondition remained provider-selectable: %+v", rule)
				}
				targetField := `,"targetRefs":["session.txt"]`
				if singleton {
					targetField = ""
				}
				payload := fmt.Sprintf(`{"operations":[{"kind":%q%s,"parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`, kind, targetField)
				candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(payload))
				if err != nil {
					t.Fatal(err)
				}
				operation := candidate.StateChange.Operations[0].Operation
				var hash string
				for _, parameter := range operation.Parameters {
					if parameter.Name == "baseHash" {
						hash = *parameter.String
					}
				}
				if hash != "session-preimage" {
					t.Fatalf("cross-target preimage = %q", hash)
				}
				encoded, err := EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
				if err != nil || bytes.Contains(encoded, []byte("baseHash")) {
					t.Fatalf("fixture leaked local parameter: %s err=%v", encoded, err)
				}
				repeated, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, encoded)
				if err != nil || !reflect.DeepEqual(candidate, repeated) {
					t.Fatalf("round trip drift: %v", err)
				}
				// Restart from the exact persisted contract, not mutable owner facts.
				persisted, _ := json.Marshal(manifest)
				var reloaded RuntimeCandidateContractManifest
				if err := json.Unmarshal(persisted, &reloaded); err != nil {
					t.Fatal(err)
				}
				replayed, err := DecodeRuntimeCandidatePayloadWithManifest(reloaded, encoded)
				if err != nil || !reflect.DeepEqual(replayed, candidate) {
					t.Fatalf("restart changed local derivation: %v", err)
				}
				for _, literal := range []string{"session-preimage", "manager-preimage", ""} {
					echo := strings.Replace(payload, `"parameters":[`, fmt.Sprintf(`"parameters":[{"name":"baseHash","string":%q},`, literal), 1)
					_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(echo))
					var diagnostic *RuntimeCandidatePayloadDiagnostic
					if !errors.As(err, &diagnostic) || diagnostic.Actual != "locally_derived_parameter_baseHash" || diagnostic.WireRule == nil || diagnostic.WireRule.WireContractHash != wire.Hash {
						t.Fatalf("echo must reject with exact correction lineage, not repair: %v", err)
					}
				}
				for i := range candidate.StateChange.Operations[0].Operation.Parameters {
					p := &candidate.StateChange.Operations[0].Operation.Parameters[i]
					if p.Name == "baseHash" {
						wrong := "manager-preimage"
						p.String = &wrong
					}
				}
				if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err == nil {
					t.Fatal("full DTO bypassed exact target precondition")
				}
			})
		}
	}
}

func TestTargetParameterBindingCatalogAndManifestFailClosed(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, StepKindFileEdit, "", []string{"session.txt", "session_manager.txt"})
	baseline, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeOperationAdmissibility)
	}{
		{"duplicate declaration", func(c *RuntimeOperationAdmissibility) {
			c.TargetParameterBindings = append(c.TargetParameterBindings, c.TargetParameterBindings[0])
		}},
		{"duplicate target", func(c *RuntimeOperationAdmissibility) {
			c.TargetParameterBindings[0].Values = append(c.TargetParameterBindings[0].Values, c.TargetParameterBindings[0].Values[0])
		}},
		{"foreign target", func(c *RuntimeOperationAdmissibility) {
			c.TargetParameterBindings[0].Values[0].TargetRef = "foreign.txt"
		}},
		{"foreign owner", func(c *RuntimeOperationAdmissibility) { c.OwnerModuleID = RuntimeModuleGit }},
		{"unknown parameter", func(c *RuntimeOperationAdmissibility) { c.TargetParameterBindings[0].Name = "unknown" }},
		{"required parameter", func(c *RuntimeOperationAdmissibility) { c.TargetParameterBindings[0].Name = "oldText" }},
		{"oversize literal", func(c *RuntimeOperationAdmissibility) {
			c.TargetParameterBindings[0].Values[0].Value = strings.Repeat("x", 1025)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := owner.OperationAdmissibilities[0].Clone()
			tc.mutate(&catalog)
			catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
			if err := catalog.Validate(); err == nil {
				t.Fatal("invalid owner binding passed validation")
			}
		})
	}
	missing := owner.OperationAdmissibilities[0].Clone()
	missing.TargetParameterBindings[0].Values = missing.TargetParameterBindings[0].Values[:1]
	missing.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(missing)
	if _, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{missing}}); err == nil {
		t.Fatal("partial target preimages reached provider manifest")
	}
	changed := owner.OperationAdmissibilities[0].Clone()
	changed.TargetParameterBindings[0].Values[0].Value = "changed"
	if err := changed.Validate(); err == nil {
		t.Fatal("target literal drift retained catalog identity")
	}
	changed.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changed)
	updated, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{changed}})
	if err != nil || updated.Hash == baseline.Hash {
		t.Fatalf("target literal drift retained manifest identity: %v", err)
	}
	oldWire, _ := BuildRuntimeCandidateWireContract(baseline)
	newWire, _ := BuildRuntimeCandidateWireContract(updated)
	if oldWire.Hash == newWire.Hash {
		t.Fatal("target literal drift retained wire identity")
	}
	// Empty extensions have no hash contribution and no serialized field, so
	// legacy sealed snapshots need neither a backfill nor an identity rewrite.
	var sink bytes.Buffer
	writeRuntimeTargetParameterBindingsHash(&sink, nil)
	writeRuntimeTargetParameterBindingsHash(&sink, []RuntimeTargetParameterBinding{})
	if sink.Len() != 0 {
		t.Fatal("empty extension changed historical hash preimage")
	}
	legacy := owner.OperationAdmissibilities[0].Clone()
	legacy.TargetParameterBindings = nil
	legacy.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(legacy)
	encoded, _ := json.Marshal(legacy)
	if bytes.Contains(encoded, []byte("TargetParameterBindings")) {
		t.Fatal("empty extension changed historical JSON shape")
	}
	legacyManifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{legacy}})
	if err != nil {
		t.Fatal(err)
	}
	legacyWire, err := BuildRuntimeCandidateWireContract(legacyManifest)
	if err != nil || !containsString(legacyWire.OperationRules[0].ProviderParameterNames, "baseHash") {
		t.Fatalf("legacy sealed contract was reinterpreted: %v", err)
	}
}

func TestTargetParameterBindingPreservesCanonicalTargetRejections(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, RuntimeBlockKindPatchApply, "text_replace", []string{"session.txt", "session_manager.txt"})
	manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatal(err)
	}
	legacy := manifest.Clone()
	legacy.Operations[0].TargetParameterBindings = nil
	legacy.Hash = StableRuntimeCandidateContractManifestHash(legacy)
	for _, targetJSON := range []string{`null`, `[]`, `"session.txt"`, `[9]`, `["unknown.txt"]`, `["session.txt","session_manager.txt"]`} {
		t.Run(targetJSON, func(t *testing.T) {
			payload := []byte(fmt.Sprintf(`{"operations":[{"kind":"patch/apply","targetRefs":%s,"parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`, targetJSON))
			_, oldErr := DecodeRuntimeCandidatePayloadWithManifest(legacy, payload)
			candidate, newErr := DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
			if oldErr == nil || newErr == nil {
				t.Fatalf("invalid target reached admission: old=%v new=%v", oldErr, newErr)
			}
			var oldPayload, newPayload *RuntimeCandidatePayloadDiagnostic
			var oldContract, newContract *RuntimeCandidateContractDiagnostic
			switch {
			case errors.As(oldErr, &oldPayload):
				if !errors.As(newErr, &newPayload) || oldPayload.Path != newPayload.Path || oldPayload.Actual != newPayload.Actual || !reflect.DeepEqual(oldPayload.Allowed, newPayload.Allowed) || !reflect.DeepEqual(oldPayload.Required, newPayload.Required) {
					t.Fatalf("preimage compilation replaced canonical target diagnostic: old=%v new=%v", oldErr, newErr)
				}
			case errors.As(oldErr, &oldContract):
				if !errors.As(newErr, &newContract) || oldContract.Path != newContract.Path || oldContract.Actual != newContract.Actual {
					t.Fatalf("preimage compilation replaced target contract diagnostic: old=%v new=%v", oldErr, newErr)
				}
			default:
				t.Fatalf("unexpected target diagnostic: %v", oldErr)
			}
			if targetJSON == `["unknown.txt"]` && candidate.StateChange == nil {
				t.Fatal("target rejection lost the typed correction frontier")
			}
		})
	}
}

func TestTargetParameterBindingRequiredSequenceUsesEachSlotPreimage(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, RuntimeBlockKindPatchApply, "text_replace", []string{"session.txt", "session_manager.txt"})
	decision.RequiredOutput.RequiredOperationSequence = []RuntimeRequiredOperationVariant{
		{RequirementID: "edit-session", Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace", TargetRef: "session.txt"},
		{RequirementID: "edit-manager", Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace", TargetRef: "session_manager.txt", DependsOnRequirementIDs: []string{"edit-session"}},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[
		{"requiredOperationRef":"edit-session","parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]},
		{"requiredOperationRef":"edit-manager","parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range candidate.StateChange.Operations {
		want := "session-preimage"
		if item.Operation.TargetRefs[0] == "session_manager.txt" {
			want = "manager-preimage"
		}
		got := ""
		for _, parameter := range item.Operation.Parameters {
			if parameter.Name == "baseHash" {
				got = *parameter.String
			}
		}
		if got != want {
			t.Fatalf("required slot %q preimage = %q, want %q", item.Operation.RequiredOperationRef, got, want)
		}
	}
}

func TestTargetParameterBindingProspectiveTargetOmitsPreimageWithoutProviderChoice(t *testing.T) {
	decision, owner := targetParameterTestInputs(t, RuntimeBlockKindPatchApply, "file_write", []string{"created.txt"})
	owner.TargetSelection = &RuntimeTargetSelection{
		PrerequisiteID: "create-selection", OwnerModuleID: RuntimeModuleFile, TargetRole: "deliverable", SelectorRef: "create-selector",
		Disposition: RuntimeTargetSelectionCreate, ProspectiveTargetRef: "created.txt", ObservedRevision: "tree-1",
	}
	owner.TargetSelection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(*owner.TargetSelection)
	catalog := &owner.OperationAdmissibilities[0]
	catalog.SelectorRefs, catalog.ConstraintRefs = []string{"create-selector"}, []string{"create-constraint"}
	catalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{Kind: RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: catalog.SelectorRefs, ConstraintRefs: catalog.ConstraintRefs}}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	manifest, err := BuildRuntimeCandidateContractManifest(decision, owner)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Operations[0].Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget || len(manifest.Operations[0].TargetParameterBindings) != 1 || len(manifest.Operations[0].TargetParameterBindings[0].Values) != 0 {
		t.Fatal("prospective target inherited another resource preimage")
	}
	const payload = `{"operations":[{"kind":"patch/apply","parameters":[{"name":"content","string":"created"}]}]}`
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	for _, parameter := range candidate.StateChange.Operations[0].Operation.Parameters {
		if parameter.Name == "baseHash" {
			t.Fatal("prospective operation invented a preimage")
		}
	}
	echo := strings.Replace(payload, `"parameters":[`, `"parameters":[{"name":"baseHash","string":"manager-preimage"},`, 1)
	_, err = DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(echo))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Actual != "locally_derived_parameter_baseHash" {
		t.Fatalf("prospective target opened provider precondition choice: %v", err)
	}
}
