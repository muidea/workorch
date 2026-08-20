package git

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/muidea/workorch/internal/pkg/common"
)

const maxGitRevisionBytes = 512

// normalizeGitRevision validates a revision before it is placed in argv. Git
// accepts many command options in the same position as a revision, including
// options with filesystem effects. Rejecting option-shaped/control input and
// still using --end-of-options keeps read operations effect-free.
func normalizeGitRevision(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxGitRevisionBytes {
		return "", fmt.Errorf("%w: git revision exceeds %d bytes", common.ErrInvalidArgument, maxGitRevisionBytes)
	}
	if strings.HasPrefix(value, "-") {
		return "", fmt.Errorf("%w: git option is not a revision", common.ErrInvalidArgument)
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", fmt.Errorf("%w: git revision contains control characters", common.ErrInvalidArgument)
		}
	}
	return value, nil
}
