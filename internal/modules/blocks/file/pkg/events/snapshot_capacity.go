package events

import "fmt"

// DefaultTargetSnapshotBytes bounds complete snapshot acquisition when a caller
// does not supply a byte limit. It is independent of the ordinary file/read
// prefix limit and of the downstream provider's request budget.
const DefaultTargetSnapshotBytes = 256 * 1024

// TargetSnapshotCapacityError reports the actual full-stream size and the
// requested acquisition limit. A partial snapshot never becomes complete input.
type TargetSnapshotCapacityError struct {
	TargetRef  string
	SizeBytes  int64
	LimitBytes int
}

func (e *TargetSnapshotCapacityError) Error() string {
	return fmt.Sprintf("target snapshot %q exceeds acquisition capacity: size=%d limit=%d bytes", e.TargetRef, e.SizeBytes, e.LimitBytes)
}
