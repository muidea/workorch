package events

import (
	"encoding/json"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
	"strings"
	"testing"
)

func TestOwnerContractAndOperationMapper(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 5 || len(MessageContracts()) != 5 {
		t.Fatalf("contracts=%+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "web" || !slices.Equal(contract.ReceiverOwners, []string{"web"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	query := "typed event contracts"
	maxResults := int64(5)
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{ID: "web-1", Kind: runtimecontract.StepKindWebSearch, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}}})
	if err != nil {
		t.Fatal(err)
	}
	if data.Query != query || data.MaxResults != maxResults || !data.SafeSearch {
		t.Fatalf("data=%+v", data)
	}
	operation := runtimecontract.RuntimeOperation{ID: "web-1", Kind: runtimecontract.StepKindWebSearch, TargetRefs: []string{"web-target"}, Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}}}
	catalog := testWebOwnerCatalog(t, operation.Kind)
	observation := testWebCatalogObservation(catalog)
	if _, err := SealRuntimeOperation(operation, runtimecontract.RuntimeOperationAdmissibility{}, observation); err == nil {
		t.Fatal("Web seal accepted missing owner catalog")
	}
	if _, err := SealRuntimeOperation(operation, catalog, runtimecontract.RuntimePrerequisiteObservation{}); err == nil {
		t.Fatal("Web seal accepted missing catalog observation")
	}
	wrongCatalog := catalog
	wrongCatalog.OwnerModuleID = runtimecontract.RuntimeModuleSSH
	wrongCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(wrongCatalog)
	if _, err := SealRuntimeOperation(operation, wrongCatalog, observation); err == nil {
		t.Fatal("Web seal accepted another owner's catalog")
	}
	sealed, err := SealRuntimeOperation(operation, catalog, observation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExpectedEffectScope:     runtimecontract.RuntimeEffectNone,
			AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		},
		OperationID: operation.ID, TargetRefs: operation.TargetRefs,
	}
	if _, _, err := sealed.ValidateForExecution(context); err != nil {
		t.Fatal(err)
	}
	drifted := sealed
	for index := range drifted.Operation.Parameters {
		if drifted.Operation.Parameters[index].Name == "query" {
			drifted.Operation.Parameters[index].String = ptrWebString("changed")
		}
	}
	if _, _, err := drifted.ValidateForExecution(context); err == nil || !strings.Contains(err.Error(), "payload hash") {
		t.Fatalf("drifted sealed operation err=%v", err)
	}
	wire, err := json.Marshal(ExecuteOperationCommand{RuntimeOperationExecutionContext: context, SealedOperation: sealed})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "approvalId") {
		t.Fatalf("Web operation wire exposes legacy approvalId: %s", wire)
	}
}

func testWebOwnerCatalog(t *testing.T, kind string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("web-admissibility", runtimecontract.RuntimeModuleWeb, []string{kind}, nil, []string{"provider-a"}, []string{"owner-execution-policy"}, "web-provider-v1")
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "web-policy", OwnerModuleID: runtimecontract.RuntimeModuleWeb, OperationKind: kind,
		SubjectRef: "web-provider", EffectScope: runtimecontract.RuntimeEffectNone,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func testWebCatalogObservation(catalog runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleWeb, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleWeb, SubjectRef: "configured_web_providers", Revision: catalog.ObservedRevision}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func ptrWebString(value string) *string { return &value }
