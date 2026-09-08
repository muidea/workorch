package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var shellGoTestTemporaryPath = regexp.MustCompile(`^tmp/(Test[^/]*?)[0-9]{6,}/([0-9]{3,})/`)

const shellAssertionFeedbackRequirements = " Optional failure diagnostics: when an actual testing assertion fails, report a separate exact line with testing.T.Error/Fatal using WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array. Use four tokens in that order, no extra text or actual values. Supported codes/pairs: shape_mismatch uses distinct null|boolean|number|string|array|object; presence_mismatch uses present|absent; emptiness_mismatch uses empty|nonempty; predicate_mismatch uses true|false; value_mismatch uses expected=equal observed=different. Report only the applicable actual mismatch; missing reports remain diagnostic-incomplete. This optional grammar never proves execution or pass, changes acceptance criteria, or authorizes editing existing inputs."

// Preserve the exact diagnostic digest separately. Only this owner's current
// scratch prefix and Go testing's generated directory suffix are comparison
// noise; arbitrary /tmp paths, filenames, expected values and output remain.
// No normalized text leaves this owner, and no archived digest is rewritten.
func shellAssertionComparisonHash(assertion, scratch string) string {
	assertion = strings.TrimSpace(assertion)
	if filepath.IsAbs(scratch) && filepath.Clean(scratch) == scratch && scratch != string(filepath.Separator) && scratch != filepath.Clean(os.TempDir()) {
		prefix := filepath.ToSlash(scratch) + "/"
		var normalized strings.Builder
		for offset := 0; offset < len(assertion); {
			index := strings.Index(assertion[offset:], prefix)
			if index < 0 {
				normalized.WriteString(assertion[offset:])
				break
			}
			index += offset
			preceding, _ := utf8.DecodeLastRuneInString(assertion[:index])
			if index != 0 && !unicode.IsSpace(preceding) && !strings.ContainsRune(`="'([{:`, preceding) {
				normalized.WriteString(assertion[offset : index+len(prefix)])
				offset = index + len(prefix)
				continue
			}
			normalized.WriteString(assertion[offset:index])
			normalized.WriteString("<validation-scratch>/")
			offset = index + len(prefix)
			if match := shellGoTestTemporaryPath.FindStringSubmatch(assertion[offset:]); len(match) == 3 {
				normalized.WriteString("tmp/" + match[1] + "<generated>/" + match[2] + "/")
				offset += len(match[0])
			}
		}
		assertion = normalized.String()
	}
	digest := sha256.Sum256([]byte("shell/assertion-comparison/v1\x00" + assertion))
	return hex.EncodeToString(digest[:])
}

// This optional diagnostic grammar is independent of check attestation. It is
// admitted only on a location-bearing output event for an actually failed
// native check. Values are a closed shape vocabulary, never arbitrary logs,
// credentials, expected/actual values, target authorization or pass markers.
func shellReportedAssertion(assertion string) runtimecontract.RuntimeValidationReportedAssertion {
	fields := strings.Fields(assertion)
	if len(fields) != 4 || fields[0] != "WORKORCH_ASSERTION_V1" ||
		!strings.HasPrefix(fields[1], "code=") || !strings.HasPrefix(fields[2], "expected=") || !strings.HasPrefix(fields[3], "observed=") {
		return runtimecontract.RuntimeValidationReportedAssertion{}
	}
	value := runtimecontract.RuntimeValidationReportedAssertion{
		Code: strings.TrimPrefix(fields[1], "code="), ExpectedShape: strings.TrimPrefix(fields[2], "expected="),
		ObservedShape: strings.TrimPrefix(fields[3], "observed="),
	}
	if value.Validate() != nil {
		return runtimecontract.RuntimeValidationReportedAssertion{}
	}
	return value
}
