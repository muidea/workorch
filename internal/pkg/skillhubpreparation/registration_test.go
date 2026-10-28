package skillhubpreparation

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistrationReceiptRequiresExactOriginalIdentityAndCompleteOutcome(t *testing.T) {
	p := RegistrationPlan{Version: RegistrationVersion, RequestID: "original", Directory: filepath.Join(t.TempDir(), "project"), Home: t.TempDir()}
	base := `{"request_id":"original","project_path":` + quotedRegistration(p.Directory) + `,"status":"succeeded","executed":true,"finished":true,"started_at":"2026-01-01T00:00:00Z","completed_at":"2026-01-01T00:00:01Z","result":{"registered":true,"project_created":true,"created_directories":[],"skill_count":0}}`
	if _, err := ParseRegistrationReceipt([]byte(base), p); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"other_request":    strings.Replace(base, `"original"`, `"other"`, 1),
		"other_path":       strings.Replace(base, quotedRegistration(p.Directory), `"/other"`, 1),
		"duplicate":        strings.Replace(base, `"status":"succeeded"`, `"status":"failed","status":"succeeded"`, 1),
		"missing_finish":   strings.Replace(base, `"finished":true,`, "", 1),
		"not_registered":   strings.Replace(base, `"registered":true`, `"registered":false`, 1),
		"bad_time":         strings.Replace(base, "00:00:01Z", "00:00:00+01:00", 1),
		"foreign_creation": strings.Replace(base, `"created_directories":[]`, `"created_directories":["/other"]`, 1),
		"trailing":         base + `{}`, "prefix": "warning\n" + base,
		"unknown_field":  strings.Replace(base, `"skill_count":0`, `"skill_count":0,"allow":true`, 1),
		"negative_count": strings.Replace(base, `"skill_count":0`, `"skill_count":-1`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRegistrationReceipt([]byte(raw), p); err == nil {
				t.Fatal("accepted invalid original receipt")
			}
		})
	}
	for _, state := range []string{"running", "unknown", "failed"} {
		doc := RegistrationReceipt{RequestID: p.RequestID, ProjectPath: p.Directory, Status: state, StartedAt: "2026-01-01T00:00:00Z"}
		yes, no := true, false
		switch state {
		case "running":
			doc.Finished = &no
		case "unknown":
			doc.CommandError = &RegistrationError{Code: "REQUEST_OUTCOME_UNKNOWN", Message: "original outcome unknown"}
		case "failed":
			doc.Executed = &yes
			doc.Finished = &yes
			doc.CompletedAt = "2026-01-01T00:00:01Z"
			doc.Result = &RegistrationEffects{CreatedDirectories: []string{}}
			doc.CommandError = &RegistrationError{Code: "FILE_OPERATION", Message: "original failure"}
		}
		raw, _ := json.Marshal(doc)
		r, err := ParseRegistrationReceipt(raw, p)
		if err != nil || r.Status != state {
			t.Fatal("lost non-success original receipt", state, err)
		}
	}
}
func quotedRegistration(v string) string { b, _ := json.Marshal(v); return string(b) }

func TestRegistrationTransportErrorRetainsExactBoundedBytes(t *testing.T) {
	for _, raw := range []string{"原始错误\r\n\xff", strings.Repeat("x", 1<<16+1)} {
		var o RegistrationObservation
		o.PreserveError(errors.New(raw))
		want := []byte(raw)
		if len(want) > 1<<16 {
			want = want[:1<<16]
		}
		if !bytes.Equal(o.RawError, want) || o.TotalErrorBytes != uint64(len(raw)) || o.ErrorComplete != (len(raw) <= 1<<16) {
			t.Fatal("diagnostic was rewritten or prefix presented as complete")
		}
		clone := o.Clone()
		clone.RawError[0] ^= 1
		if bytes.Equal(clone.RawError, o.RawError) {
			t.Fatal("diagnostic clone aliases source")
		}
	}
}
