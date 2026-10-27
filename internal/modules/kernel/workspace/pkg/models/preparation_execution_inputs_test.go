package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationConfigurationPreimagePreservesHistoricalHashAndRawClone(t *testing.T) {
	for _, populated := range []bool{false, true} {
		w := &Workspace{DeliverableDirectory: "project", InternalDirectory: "internal", TemplateID: "template", ProfileID: "profile", SoulBindings: map[string]common.SoulID{}, EvidencePolicies: []payload.Payload{}}
		if populated {
			w.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "declared-only"}}
			w.DefaultSoulID, w.SoulBindings["execution"] = "soul", "execution-soul"
			w.ContextHints = payload.New(payload.StringField("stateChangePolicy", "must_produce_state_change"))
			w.EvidencePolicies = []payload.Payload{payload.New(payload.StringField("id", "original"))}
		}
		// Exact historical wire shape, independently retained as a regression.
		legacy := struct {
			Directory, InternalDirectory, TemplateID, ProfileID string
			DefaultLLMSelection                                 *runtimecontract.ProviderInferenceSelection
			DefaultSoulID                                       common.SoulID
			SoulBindings                                        map[string]common.SoulID
			ContextHints                                        payload.Payload
			EvidencePolicies                                    []payload.Payload
		}{w.DeliverableDirectory, w.InternalDirectory, w.TemplateID, w.ProfileID, w.DefaultLLMSelection, w.DefaultSoulID, w.SoulBindings, w.ContextHints, w.EvidencePolicies}
		if len(legacy.SoulBindings) == 0 {
			legacy.SoulBindings = nil
		}
		if len(legacy.EvidencePolicies) == 0 {
			legacy.EvidencePolicies = nil
		}
		raw, err := json.Marshal(legacy)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		expected := "workspace-configuration-sha256:" + hex.EncodeToString(digest[:])
		snapshot := CaptureWorkspaceConfiguration(w)
		if WorkspaceConfigurationHash(w) != expected || snapshot.Hash() != expected {
			t.Fatal("changed historical configuration identity")
		}
		snapshot.SoulBindings["execution"] = "caller"
		if populated {
			snapshot.DefaultLLMSelection.Target.ModelID = "caller"
			snapshot.ContextHints.Fields[0].Name = "caller"
			snapshot.EvidencePolicies[0].Fields[0].Name = "caller"
		}
		if WorkspaceConfigurationHash(w) != expected {
			t.Fatal("archive aliases source configuration")
		}
	}
}
