package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCandidateFrozenWorkSourceUsesOriginalOwnerAcrossReload(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	file, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Teardown(ctx) })
	directory := t.TempDir()
	body := "original exact working bytes\r\n"
	if err = os.WriteFile(filepath.Join(directory, "target.txt"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	owner := &implementationWorkRealFileFixture{hub: hub, directory: directory}
	a := implementationWorkPrivatePipeline(t, contract.RuntimeImplementationWorkPurposeEffect, []string{"target.txt"}, owner, false, true)
	m, err := BuildRuntimeCandidateContractManifestForAggregate(a)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := CandidateWorkContent(a, m)
	if err != nil || frozen.Snapshots[0].Content != body {
		t.Fatalf("source=%+v err=%v", frozen, err)
	}
	encoded, _ := json.Marshal(a)
	var restored RuntimeAggregate
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	replay, err := CandidateWorkContent(restored, m)
	if err != nil || !reflect.DeepEqual(frozen, replay) {
		t.Fatal("restart changed original scope", err)
	}
	frozen.Snapshots[0].Content = "mutated transport"
	if !reflect.DeepEqual(replay, a.ImplementationWork.Rounds[0].ScopeObservation.Content) {
		t.Fatal("transport aliased owner aggregate")
	}
	if err = os.WriteFile(filepath.Join(directory, "target.txt"), []byte("mutable file changed later"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := CandidateWorkContent(restored, m)
	if err != nil || !reflect.DeepEqual(again, replay) {
		t.Fatal("recovery substituted a mutable reread", err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"scope": func(a *RuntimeAggregate) {
			a.ImplementationWork.Rounds[0].ScopeObservation.Content.Snapshots[0].Content = "drift"
		},
		"working binding": func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[0].WorkingBinding.Hash = "foreign" },
		"purpose source":  func(a *RuntimeAggregate) { a.ImplementationWork.Rounds[0].CoverageWorkItemID = "foreign-cwi" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := restored.CloneRaw()
			mutate(&bad)
			if _, err := CandidateWorkContent(bad, m); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	if a.ActiveDispatch != nil || len(a.OperationReceipts) != 0 || len(a.Facts.Evidence.Accepted) != 0 {
		t.Fatal("reading source generated effects or credit")
	}
}
