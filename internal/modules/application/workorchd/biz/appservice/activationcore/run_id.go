package activationcore

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ParseNumericRunID(runID common.RunID) (int, bool) {
	raw := strings.TrimSpace(string(runID))
	if !strings.HasPrefix(raw, "run-") {
		return 0, false
	}
	suffix := strings.TrimPrefix(raw, "run-")
	if suffix == "" {
		return 0, false
	}
	seq, err := strconv.Atoi(suffix)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}

func FormatNumericRunID(seq int) string {
	if seq < 1 {
		seq = 1
	}
	return fmt.Sprintf("run-%03d", seq)
}
