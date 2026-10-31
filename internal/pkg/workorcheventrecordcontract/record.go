// Package workorcheventrecordcontract contains the immutable event record
// value shared by eventstream persistence and its typed EventHub contract. It
// defines no topic, command, repository or projection behavior.
package workorcheventrecordcontract

import (
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// CanonicalOccurredAt validates an event source timestamp and returns the
// single persisted representation used by the Eventstream owner.
func CanonicalOccurredAt(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: event record occurredAt is required", common.ErrInvalidArgument)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", fmt.Errorf("%w: event record occurredAt must be RFC3339: %v", common.ErrInvalidArgument, err)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

// Record is the complete immutable value appended by the Eventstream owner.
// Protocol projections select contract metadata without rewriting original
// business values and must not post it back as authority.
type Record struct {
	ID           common.EventRecordID
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	StepID       common.StepID
	// StreamPosition is the Eventstream-owner-issued, globally monotonic append
	// position used for durable cursor replay. Revision remains workspace-local
	// and is used by projection high-watermarks.
	StreamPosition int64
	Revision       int64
	Payload        workorcheventpayload.Payload
	PayloadSummary workorcheventpayload.Payload
	PayloadRef     string
}
