package skillhubpreparation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const RegistrationVersion = "skillhub-project-registration/v1"
const RegistrationAttemptLimit = 4

var registrationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func RegistrationRequestID(workspaceID, requestID string) string {
	return fmt.Sprintf("workorch-%x", sha256.Sum256([]byte(workspaceID+"\x00"+requestID)))
}

// Home is part of the original external identity: receipts are local to one
// SkillHub home. These values and all output stay in owner-private persistence.
type RegistrationPlan struct{ Version, RequestID, Directory, Home string }

func (p RegistrationPlan) Validate() error {
	if p.Version != RegistrationVersion || !registrationIDPattern.MatchString(p.RequestID) || !cleanAbsolute(p.Directory) || !cleanAbsolute(p.Home) {
		return common.ErrConflict
	}
	return nil
}
func cleanAbsolute(v string) bool { return v != "" && filepath.IsAbs(v) && filepath.Clean(v) == v }

type RegistrationReceipt struct {
	RequestID    string               `json:"request_id"`
	ProjectPath  string               `json:"project_path,omitempty"`
	Status       string               `json:"status"`
	Executed     *bool                `json:"executed"`
	Finished     *bool                `json:"finished"`
	StartedAt    string               `json:"started_at,omitempty"`
	CompletedAt  string               `json:"completed_at,omitempty"`
	Result       *RegistrationEffects `json:"result,omitempty"`
	CommandError *RegistrationError   `json:"command_error,omitempty"`
}
type RegistrationEffects struct {
	Registered         bool     `json:"registered"`
	ProjectCreated     bool     `json:"project_created"`
	CreatedDirectories []string `json:"created_directories"`
	SkillCount         int      `json:"skill_count"`
}
type RegistrationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ParseRegistrationReceipt(raw []byte, p RegistrationPlan) (RegistrationReceipt, error) {
	var r RegistrationReceipt
	if p.Validate() != nil || !ValidStatusDocument(raw) {
		return r, common.ErrConflict
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(json.RawMessage)) != io.EOF || r.RequestID != p.RequestID || r.ProjectPath != p.Directory {
		return RegistrationReceipt{}, common.ErrConflict
	}
	start, err := time.Parse(time.RFC3339Nano, r.StartedAt)
	if err != nil || start.IsZero() {
		return RegistrationReceipt{}, common.ErrConflict
	}
	validError := r.CommandError != nil && r.CommandError.Code != "" && r.CommandError.Message != ""
	switch r.Status {
	case "succeeded", "failed":
		end, err := time.Parse(time.RFC3339Nano, r.CompletedAt)
		if err != nil || end.Before(start) || r.Executed == nil || !*r.Executed || r.Finished == nil || !*r.Finished || r.Result == nil || r.Result.SkillCount < 0 || len(r.Result.CreatedDirectories) > 2 || r.Result.CreatedDirectories == nil || r.Status == "succeeded" && (!r.Result.Registered || r.CommandError != nil) || r.Status == "failed" && !validError {
			return RegistrationReceipt{}, common.ErrConflict
		}
		seen := map[string]bool{}
		for _, v := range r.Result.CreatedDirectories {
			if (v != filepath.Join(p.Directory, ".agents") && v != filepath.Join(p.Directory, ".agents", "skills")) || seen[v] {
				return RegistrationReceipt{}, common.ErrConflict
			}
			seen[v] = true
		}
	case "running":
		if r.Finished == nil || *r.Finished || r.Executed != nil || r.CompletedAt != "" || r.Result != nil || r.CommandError != nil {
			return RegistrationReceipt{}, common.ErrConflict
		}
	case "unknown":
		if r.Executed != nil || r.Finished != nil || r.CompletedAt != "" || r.Result != nil || !validError || r.CommandError.Code != "REQUEST_OUTCOME_UNKNOWN" {
			return RegistrationReceipt{}, common.ErrConflict
		}
	default:
		return RegistrationReceipt{}, common.ErrConflict
	}
	return r, nil
}

// Raw streams are preserved as bounded exact prefixes, with explicit counts
// and completeness. Command success alone never grants a registration result.
type RegistrationObservation struct {
	StartedAt, CompletedAt             time.Time
	CommandStarted, ExitCodeKnown      bool
	ExitCode                           int
	Stdout, Stderr                     []byte
	TotalStdoutBytes, TotalStderrBytes uint64
	StdoutSHA256, StderrSHA256         string
	Complete                           bool
	RawError                           []byte
	TotalErrorBytes                    uint64
	ErrorSHA256                        string
	ErrorComplete                      bool
}

func (o RegistrationObservation) Clone() RegistrationObservation {
	o.RawError = bytes.Clone(o.RawError)
	o.Stdout = bytes.Clone(o.Stdout)
	o.Stderr = bytes.Clone(o.Stderr)
	return o
}
func (o RegistrationObservation) Validate() error {
	if len(o.RawError) > 1<<16 || uint64(len(o.RawError)) > o.TotalErrorBytes || o.ErrorSHA256 != fileobservation.Digest(o.RawError) || o.ErrorComplete != (uint64(len(o.RawError)) == o.TotalErrorBytes) || o.StartedAt.IsZero() || o.CompletedAt.Before(o.StartedAt) || len(o.Stdout) > 1<<20 || len(o.Stderr) > 1<<20 || uint64(len(o.Stdout)) > o.TotalStdoutBytes || uint64(len(o.Stderr)) > o.TotalStderrBytes || o.StdoutSHA256 != fileobservation.Digest(o.Stdout) || o.StderrSHA256 != fileobservation.Digest(o.Stderr) || o.Complete && (uint64(len(o.Stdout)) != o.TotalStdoutBytes || uint64(len(o.Stderr)) != o.TotalStderrBytes) || !o.CommandStarted && (o.ExitCodeKnown || o.ExitCode != 0 || o.TotalStdoutBytes != 0 || o.TotalStderrBytes != 0) || o.ExitCodeKnown && o.ExitCode < 0 {
		return common.ErrConflict
	}
	return nil
}
func (o RegistrationObservation) Receipt(p RegistrationPlan, query bool) (RegistrationReceipt, error) {
	if o.Validate() != nil || !o.ErrorComplete || o.ExitCode == 0 && o.TotalErrorBytes != 0 || !o.Complete || !o.CommandStarted || !o.ExitCodeKnown {
		return RegistrationReceipt{}, common.ErrConflict
	}
	r, err := ParseRegistrationReceipt(o.Stdout, p)
	if err != nil || o.ExitCode != 0 && (query || r.Status != "failed" || o.ExitCode != 1) {
		return RegistrationReceipt{}, common.ErrConflict
	}
	return r, nil
}

// Transport errors remain separate from exact stdout/stderr; retaining a prefix
// reports its boundary explicitly and never changes a known command failure.
func (o *RegistrationObservation) PreserveError(err error) {
	var raw []byte
	if err != nil {
		raw = []byte(err.Error())
	}
	o.TotalErrorBytes = uint64(len(raw))
	o.ErrorComplete = len(raw) <= 1<<16
	if len(raw) > 1<<16 {
		raw = raw[:1<<16]
	}
	o.RawError = bytes.Clone(raw)
	o.ErrorSHA256 = fileobservation.Digest(raw)
}
