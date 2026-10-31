package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestPreparationObservationPersistenceCloneAndBounds(t *testing.T) {
	at := time.Now().UTC()
	raw := []byte("原始\xff\x00\r\n错误")
	hash := sha256.Sum256(raw)
	d := PreparationObservationDiagnostic{Phase: "final", Owner: "mcpregistry", Stage: "registry_after", Code: "owner_rejected", Attempted: true, StartedAt: at, CompletedAt: at, ErrorBytes: raw, ErrorByteCount: len(raw), ErrorSHA256: hex.EncodeToString(hash[:])}
	if d.Validate() != nil {
		t.Fatal("valid diagnostic rejected")
	}
	r := WorkspacePreparationRecord{Attempts: []WorkspacePreparationAttempt{{Observations: []PreparationObservationDiagnostic{d}, Finalization: &PreparationFinalization{Code: "owner_observation_incomplete", Owner: "mcpregistry", Stage: "final_owner_revalidation"}}}}
	copy := r.Clone()
	copy.Attempts[0].Observations[0].ErrorBytes[0] = 'X'
	copy.Attempts[0].Finalization.Code = "changed"
	if reflect.DeepEqual(copy, r) || !reflect.DeepEqual(r.Attempts[0].Observations[0], d) || r.Attempts[0].Finalization.Code != "owner_observation_incomplete" {
		t.Fatal("alias into owner diagnostic")
	}
	encoded, _ := json.Marshal(d)
	var restored PreparationObservationDiagnostic
	if json.Unmarshal(encoded, &restored) != nil || !reflect.DeepEqual(d, restored) {
		t.Fatal("raw bytes changed in persistence")
	}
	for name, mutate := range map[string]func(*PreparationObservationDiagnostic){
		"raw":             func(d *PreparationObservationDiagnostic) { d.ErrorBytes = []byte("changed") },
		"hash":            func(d *PreparationObservationDiagnostic) { d.ErrorSHA256 = "wrong" },
		"time":            func(d *PreparationObservationDiagnostic) { d.CompletedAt = d.StartedAt.Add(-time.Second) },
		"fake_success":    func(d *PreparationObservationDiagnostic) { d.Code = "observed" },
		"fake_not_called": func(d *PreparationObservationDiagnostic) { d.Code = "budget_exhausted_before_call" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := ClonePreparationObservations([]PreparationObservationDiagnostic{d})[0]
			mutate(&bad)
			if bad.Validate() == nil {
				t.Fatal("invalid diagnostic accepted")
			}
		})
	}
	if ValidatePreparationObservations([]PreparationObservationDiagnostic{d, d}, at, at) == nil || ValidatePreparationObservations([]PreparationObservationDiagnostic{d}, at.Add(time.Second), at.Add(time.Second)) == nil {
		t.Fatal("duplicate/out of scope diagnostic accepted")
	}
}
