package orchestrator

import "time"

func runtimeEvidenceTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
