package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

func shellBaseOutput(request RuntimeCommandRequest, command string, args []string, workingDir string, commandNormalization string, execCommand string) RuntimeCommandResult {
	var copiedInputs []shellValidationCopiedInput
	if request.validationCopiedInputs != nil {
		copiedInputs = append(make([]shellValidationCopiedInput, 0, len(request.validationCopiedInputs)), request.validationCopiedInputs...)
	}
	output := RuntimeCommandResult{
		Command:                command,
		ValidationRecipeID:     strings.TrimSpace(request.ValidationRecipeID),
		Args:                   append([]string(nil), args...),
		WorkingDirectory:       workingDir,
		TargetRefs:             normalizeShellStrings(request.TargetRefs),
		ValidationEvidence:     cloneRuntimeValidationEvidence(request.ValidationEvidence),
		ManagedValidation:      request.ManagedValidation,
		validationCopiedInputs: copiedInputs,
	}
	if commandNormalization != "" {
		output.ActualCommand = filepath.Base(execCommand)
		output.CommandNormalization = commandNormalization
	}
	return output
}

type limitedOutputBuffer struct {
	limit int
	total int
	buf   bytes.Buffer
}

func newLimitedOutputBuffer(limit int) *limitedOutputBuffer {
	if limit <= 0 {
		limit = MaxCommandOutputBytes
	}
	return &limitedOutputBuffer{limit: limit}
}

func (b *limitedOutputBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	written := len(p)
	b.total += len(p)
	if b.buf.Len() >= b.limit {
		return written, nil
	}
	remaining := b.limit - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return written, nil
}

func (b *limitedOutputBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buf.String()
}

func (b *limitedOutputBuffer) Truncated() bool {
	return b != nil && b.total > b.limit
}

func (b *limitedOutputBuffer) TotalBytes() int {
	if b == nil {
		return 0
	}
	return b.total
}

func normalizeShellExecutionCommand(command string, args []string) (string, []string, string) {
	if shellCommandRequiresInlineShell(command, args) {
		return "sh", []string{"-c", shellCommandInlineScript(command, args)}, "compound_command_wrapped_with_sh_c"
	}
	bashReason := shellCommandBashNormalizationReason(command, args)
	if bashReason == "" {
		return command, args, ""
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		return command, args, ""
	}
	return bashPath, args, bashReason
}

func shellCommandRequiresInlineShell(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "" || base == "sh" || base == "bash" || base == "zsh" {
		return false
	}
	if shellCommandIsShellBuiltin(base) && len(args) > 0 {
		return true
	}
	for _, arg := range args {
		if shellCommandArgIsShellOperator(arg) {
			return true
		}
	}
	return false
}

func shellCommandIsShellBuiltin(command string) bool {
	switch strings.TrimSpace(command) {
	case "cd", "export", "alias", "source", ".":
		return true
	default:
		return false
	}
}

func shellCommandArgIsShellOperator(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "&&", "||", ";", "|", ">", ">>", "<", "2>", "2>>":
		return true
	default:
		return false
	}
}

func shellCommandInlineScript(command string, args []string) string {
	parts := []string{shellQuoteCommandToken(command)}
	for _, arg := range args {
		if shellCommandArgIsShellOperator(arg) {
			parts = append(parts, strings.TrimSpace(arg))
			continue
		}
		parts = append(parts, shellQuoteCommandToken(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func shellQuoteCommandToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '=' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellCommandBashNormalizationReason(command string, args []string) string {
	if filepath.Base(strings.TrimSpace(command)) != "sh" {
		return ""
	}
	script, ok := inlineShellScript(args)
	if !ok {
		return ""
	}
	normalized := strings.ToLower(script)
	if strings.Contains(normalized, "pipefail") {
		return "sh_script_requires_bash_pipefail"
	}
	if strings.Contains(script, "${PIPESTATUS") || strings.Contains(script, "${PIPESTATUS[") {
		return "sh_script_requires_bash_pipestatus"
	}
	return ""
}

func inlineShellScript(args []string) (string, bool) {
	for idx, arg := range args {
		if arg == "-c" && idx+1 < len(args) {
			return args[idx+1], true
		}
	}
	return "", false
}

func shellCommandTimeout(timeoutMillis int64, defaultTimeout time.Duration) (time.Duration, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultCommandTimeout
	}
	if timeoutMillis == 0 {
		return defaultTimeout, nil
	}
	if timeoutMillis < 1000 {
		return 0, fmt.Errorf("%w: timeoutMillis must be at least 1000", common.ErrInvalidArgument)
	}
	if timeoutMillis > int64(MaxCommandTimeout/time.Millisecond) {
		return 0, fmt.Errorf("%w: timeoutMillis must not exceed %d", common.ErrInvalidArgument, MaxCommandTimeout/time.Millisecond)
	}
	return time.Duration(timeoutMillis) * time.Millisecond, nil
}

func (a *RuntimeAdapter) filteredEnv() []string {
	if a == nil || len(a.envAllowlist) == 0 {
		return nil
	}
	out := make([]string, 0, len(a.envAllowlist))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, allowed := a.envAllowlist[key]; allowed {
			out = append(out, item)
		}
	}
	return out
}

// validateShellValidationPreflight performs domain-neutral executability checks for validation-like shell commands.
// It does not encode product-domain test frameworks beyond generic package/tool presence signals.

func sanitizeSuccessfulCommandStderr(command string, args []string, exitCode int, stdoutText string, stderrText string) string {
	if exitCode != 0 || strings.TrimSpace(stdoutText) == "" || strings.TrimSpace(stderrText) == "" {
		return stderrText
	}
	if !shellCommandMayEmitCurlProgress(command, args) {
		return stderrText
	}
	cleaned, changed := stripCurlProgressMeter(stderrText)
	if !changed {
		return stderrText
	}
	return cleaned
}

func shellCommandMayEmitCurlProgress(command string, args []string) bool {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "curl" {
		return true
	}
	switch base {
	case "sh", "bash", "zsh":
		for idx, arg := range args {
			if arg != "-c" || idx+1 >= len(args) {
				continue
			}
			if shellScriptContainsCurl(args[idx+1]) {
				return true
			}
		}
	}
	return false
}

func shellScriptContainsCurl(script string) bool {
	fields := strings.Fields(strings.TrimSpace(script))
	for _, field := range fields {
		field = strings.Trim(field, "'\"`();|&")
		if filepath.Base(field) == "curl" {
			return true
		}
	}
	return false
}

func stripCurlProgressMeter(stderrText string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(stderrText, "\r", "\n"), "\n")
	ret := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if isCurlProgressMeterLine(line) {
			changed = true
			continue
		}
		ret = append(ret, line)
	}
	if !changed {
		return stderrText, false
	}
	return strings.TrimSpace(strings.Join(ret, "\n")), true
}

func isCurlProgressMeterLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "% Total") && strings.Contains(trimmed, "% Received") {
		return true
	}
	if strings.Contains(trimmed, "Dload") && strings.Contains(trimmed, "Upload") && strings.Contains(trimmed, "Speed") {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 10 {
		return false
	}
	if !allDigits(fields[0]) {
		return false
	}
	return strings.Contains(trimmed, "--:--:--") || strings.Contains(trimmed, "k") || strings.Contains(trimmed, "M")
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
