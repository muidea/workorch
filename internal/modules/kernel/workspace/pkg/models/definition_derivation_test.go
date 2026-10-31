package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestDerivationReserveRequiresSealedResourcesBeforeBudget(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := WorkspaceTemplate{ID: "t"}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	if _, _, fresh, err := record.ReserveDerivation("derive-1", "provider-config", nil); err == nil || fresh || len(record.DerivationRequests) != 0 {
		t.Fatal("unsealed resource obtained derivation permit")
	}
	resources := []DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: "验收要求", ContentHash: "sha256:5fe34f5e"}}
	if _, _, _, err := record.ReserveDerivation("derive-1", "provider-config", resources); err == nil {
		t.Fatal("forged resource hash obtained derivation permit")
	}
	sum := sha256.Sum256([]byte(resources[0].Content))
	resources[0].ContentHash = "sha256:" + hex.EncodeToString(sum[:])
	stored, reserved, fresh, err := record.ReserveDerivation("derive-1", "provider-config", resources)
	if err != nil || !fresh || reserved.ResourceSnapshotHash != DefinitionAuthoringResourcesHash(resources) || stored.Validate() != nil {
		t.Fatalf("bounded snapshot did not persist with permit: %+v %v", reserved, err)
	}
	resources[0].Content = "调用方改写"
	if stored.DerivationRequests[0].Resources[0].Content != "验收要求" {
		t.Fatal("persisted request aliases caller's resource content")
	}
	replayed, request, fresh, err := stored.ReserveDerivation("derive-1", "provider-config", stored.DerivationRequests[0].Resources)
	if err != nil || fresh || len(replayed.DerivationRequests) != 1 || request.PermitID != reserved.PermitID {
		t.Fatalf("exact resource replay reissued permit: %+v %v", request, err)
	}
	changed := sha256.Sum256([]byte("调用方改写"))
	if _, _, _, err := stored.ReserveDerivation("derive-1", "provider-config", []DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: "调用方改写", ContentHash: "sha256:" + hex.EncodeToString(changed[:])}}); err == nil {
		t.Fatal("same request ID accepted a changed resource")
	}
	mutated := stored.Clone()
	mutated.DerivationRequests[0].Resources[0].Content = "篡改"
	if mutated.Validate() == nil {
		t.Fatal("persisted resource content changed without updating its hash")
	}
}

func TestDerivationBudgetCannotBeResetByChangingRequestID(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := WorkspaceTemplate{ID: "t"}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	for n := 0; n < DefinitionDerivationAttemptLimit; n++ {
		var err error
		record, _, _, err = record.ReserveDerivation(fmt.Sprintf("derive-attempt-%d", n), "provider-config", nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, err := record.ReserveDerivation("derive-extra", "provider-config", nil); err == nil {
		t.Fatal("changing request ID reset the input attempt budget")
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	forged := record.Clone()
	extra := forged.DerivationRequests[0].Clone()
	extra.RequestID = "forged-extra"
	extra.PermitID = extra.ExpectedPermitID()
	forged.DerivationRequests = append(forged.DerivationRequests, extra)
	if forged.Validate() == nil {
		t.Fatal("persisted input attempt budget can be forged")
	}
}

func TestDerivationReserveReplayCompletionAndBudget(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := WorkspaceTemplate{ID: "t"}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	next, request, fresh, err := record.ReserveDerivation("derive-1", "provider-config", nil)
	if err != nil || !fresh || len(record.DerivationRequests) != 0 {
		t.Fatalf("reserve: %v", err)
	}
	_, replay, fresh, err := next.ReserveDerivation("derive-1", "provider-config", nil)
	if err != nil || fresh || replay.PermitID != request.PermitID {
		t.Fatalf("replay granted physical call: %v", err)
	}
	if _, _, _, err := next.ReserveDerivation("derive-1", "changed-provider", nil); err == nil {
		t.Fatal("request provider changed")
	}
	proposal := &DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"验收"}}
	completed, err := completeTestDerivation(next, request.PermitID, proposal, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := completeTestDerivation(completed, request.PermitID, proposal, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := completeTestDerivation(completed, request.PermitID, nil, "provider_failed"); err == nil {
		t.Fatal("completion overwritten")
	}
	proposal.AdditionalItems[0] = "篡改"
	if completed.DerivationRequests[0].Proposal.AdditionalItems[0] != "验收" {
		t.Fatal("completion aliases caller")
	}
	completed.Input.Revision++
	completed.Input.GoalText = "修改目标"
	completed.InputHash = completed.Input.InputHash()
	for n := 1; n < DefinitionDerivationRequestLimit; n++ {
		completed.Input.Revision++
		completed.Input.GoalText = fmt.Sprintf("目标修订 %d", n)
		completed.InputHash = completed.Input.InputHash()
		completed, _, fresh, err = completed.ReserveDerivation(fmt.Sprint("derive-", n+1), "provider-config", nil)
		if err != nil || !fresh {
			t.Fatalf("request %d: %v", n, err)
		}
	}
	if _, _, _, err := completed.ReserveDerivation("over-budget", "provider-config", nil); err == nil {
		t.Fatal("revision reset budget")
	}
	corrupt := completed.Clone()
	corrupt.DerivationRequests[0].PermitID = "forged"
	if corrupt.Validate() == nil {
		t.Fatal("corrupt history accepted")
	}
}
