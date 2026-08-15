package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellCommandSpec(command string, args []string) (string, []string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, common.ErrInvalidArgument
	}
	return command, append([]string(nil), args...), nil
}

func validateShellCommandPolicy(command string, args []string, deliverableDirectory string, allowManagedProcessControl bool) error {
	base := filepath.Base(strings.TrimSpace(command))
	if base == "" {
		return common.ErrInvalidArgument
	}
	switch base {
	case "sudo", "su", "shutdown", "reboot", "poweroff", "halt", "mkfs", "mount", "umount":
		return fmt.Errorf("%w: shell command %q is not allowed", common.ErrInvalidArgument, base)
	case "kill", "pkill", "killall":
		return fmt.Errorf("%w: process termination command %q is not allowed", common.ErrInvalidArgument, base)
	case "fuser":
		if fuserCommandIsDangerous(args) {
			return fmt.Errorf("%w: process termination command %q is not allowed", common.ErrInvalidArgument, base)
		}
	case "rm":
		if rmCommandIsDangerous(args, deliverableDirectory) {
			return fmt.Errorf("%w: destructive rm command is not allowed", common.ErrInvalidArgument)
		}
	case "chmod":
		if chmodCommandIsDangerous(args) {
			return fmt.Errorf("%w: chmod 777 is not allowed", common.ErrInvalidArgument)
		}
	case "git":
		if gitCommandIsDangerous(args) {
			return fmt.Errorf("%w: git force push is not allowed", common.ErrInvalidArgument)
		}
	case "sh", "bash", "zsh":
		if shellScriptIsDangerous(args, allowManagedProcessControl) {
			return fmt.Errorf("%w: destructive shell script is not allowed", common.ErrInvalidArgument)
		}
		if shellScriptStartsDetachedProcess(args) {
			return fmt.Errorf("%w: detached or background shell script is not allowed; use a bounded foreground command or wait for all background work", common.ErrInvalidArgument)
		}
	}
	return nil
}

func validateValidationShellEvidencePolicy(evidence runtimemodel.RuntimeValidationEvidence, command string, args []string) error {
	if runtimeValidationEvidenceEmpty(evidence) {
		return nil
	}
	base := filepath.Base(strings.TrimSpace(command))
	if base != "sh" && base != "bash" && base != "zsh" {
		return nil
	}
	script, ok := shellScriptArg(args)
	if !ok {
		return nil
	}
	if shellScriptMasksFailureWithFallback(script) {
		return fmt.Errorf("%w: validation shell command masks command failure with fallback; use fail-fast shell or exit with the failed status", common.ErrInvalidArgument)
	}
	if !shellScriptCapturesExitStatus(script) {
		return nil
	}
	if shellScriptReturnsCapturedExitStatus(script) {
		return nil
	}
	return fmt.Errorf("%w: validation shell command captures exit status without returning it; use fail-fast shell or exit with the captured status", common.ErrInvalidArgument)
}

func shellScriptArg(args []string) (string, bool) {
	for idx, arg := range args {
		if strings.TrimSpace(arg) == "-c" && idx+1 < len(args) {
			return args[idx+1], true
		}
	}
	return "", false
}

func shellScriptMasksFailureWithFallback(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	for {
		idx := strings.Index(lower, "||")
		if idx < 0 {
			return false
		}
		tail := strings.TrimSpace(lower[idx+2:])
		if shellFailureFallbackMasksStatus(tail) {
			return true
		}
		lower = tail
	}
}

func shellFailureFallbackMasksStatus(tail string) bool {
	segment := shellFallbackSegment(tail)
	if shellSegmentReturnsFailure(segment) {
		return false
	}
	switch shellFirstCommandWord(tail) {
	case ":", "echo", "printf", "true":
		return true
	default:
		return false
	}
}

func shellFallbackSegment(tail string) string {
	segment := strings.TrimSpace(tail)
	if idx := strings.Index(segment, "||"); idx >= 0 {
		segment = segment[:idx]
	}
	return segment
}

func shellSegmentReturnsFailure(segment string) bool {
	for _, marker := range []string{
		"exit ",
		"exit\t",
		"exit\n",
		"return ",
		"return\t",
		"return\n",
	} {
		if strings.Contains(segment, marker) {
			return true
		}
	}
	return false
}

func shellFirstCommandWord(tail string) string {
	trimmed := strings.TrimSpace(tail)
	for {
		next := strings.TrimLeft(trimmed, "{( ")
		if next == trimmed {
			break
		}
		trimmed = next
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], ";")
}

func shellScriptCapturesExitStatus(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	return strings.Contains(lower, "$?") ||
		strings.Contains(lower, "exit_code=") ||
		strings.Contains(lower, "exitcode=")
}

func shellScriptReturnsCapturedExitStatus(script string) bool {
	lower := strings.ToLower(strings.TrimSpace(script))
	for _, marker := range []string{
		"exit $?",
		"exit ${?}",
		"exit $status",
		"exit ${status}",
		"exit $exit_code",
		"exit ${exit_code}",
		"exit $exitcode",
		"exit ${exitcode}",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func rmCommandIsDangerous(args []string, deliverableDirectory string) bool {
	recursiveForce := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && strings.Contains(arg, "r") && strings.Contains(arg, "f") {
			recursiveForce = true
			break
		}
	}
	if !recursiveForce {
		return false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		target := strings.TrimSpace(arg)
		switch target {
		case "", "/", ".", "..", "*", ".git", "./.git":
			return true
		}
		if filepath.IsAbs(target) {
			cleanTarget := filepath.Clean(target)
			cleanWorking := filepath.Clean(deliverableDirectory)
			if cleanTarget == cleanWorking || !strings.HasPrefix(cleanTarget, cleanWorking+string(os.PathSeparator)) {
				return true
			}
		}
		if strings.Contains(target, "..") {
			return true
		}
	}
	return false
}

func fuserCommandIsDangerous(args []string) bool {
	for _, arg := range args {
		if strings.Contains(strings.TrimSpace(arg), "k") && strings.HasPrefix(strings.TrimSpace(arg), "-") {
			return true
		}
	}
	return false
}

func chmodCommandIsDangerous(args []string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "777" {
			return true
		}
	}
	return false
}

func gitCommandIsDangerous(args []string) bool {
	if len(args) < 2 || args[0] != "push" {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--force" || arg == "-f" || strings.HasPrefix(arg, "--force-") {
			return true
		}
	}
	return false
}

func shellScriptIsDangerous(args []string, allowManagedProcessControl bool) bool {
	for idx, arg := range args {
		if arg != "-c" || idx+1 >= len(args) {
			continue
		}
		script := strings.ToLower(args[idx+1])
		dangerousMarkers := []string{
			"rm -rf /",
			"rm -fr /",
			"rm -rf .git",
			"rm -fr .git",
			"sudo ",
			" shutdown",
			" reboot",
			" poweroff",
			" mkfs",
			"git push --force",
			"git push -f",
			"chmod 777",
		}
		padded := " " + script
		for _, marker := range dangerousMarkers {
			if strings.Contains(padded, marker) {
				return true
			}
		}
		if shellScriptHasForbiddenProcessTermination(padded, allowManagedProcessControl) {
			return true
		}
	}
	return false
}

func shellScriptHasForbiddenProcessTermination(paddedScript string, allowManagedProcessControl bool) bool {
	for _, marker := range []string{
		" fuser -k ",
		" fuser -k",
		" xargs kill",
		" pkill ",
		" killall ",
		" kill -",
	} {
		if strings.Contains(paddedScript, marker) {
			return true
		}
	}
	return !allowManagedProcessControl && strings.Contains(paddedScript, " kill ")
}

func shellScriptStartsDetachedProcess(args []string) bool {
	for idx, arg := range args {
		if arg != "-c" || idx+1 >= len(args) {
			continue
		}
		script := strings.ToLower(strings.TrimSpace(args[idx+1]))
		if script == "" {
			continue
		}
		padded := " " + script + " "
		if strings.Contains(padded, " nohup ") || strings.Contains(padded, " setsid ") {
			return true
		}
		if !shellCommandLineContainsStandaloneAmpersand(script) {
			continue
		}
		if shellScriptWaitsForBackgroundWork(script) {
			continue
		}
		return true
	}
	return false
}

func shellScriptWaitsForBackgroundWork(script string) bool {
	padded := " " + strings.ToLower(strings.TrimSpace(script)) + " "
	trimmed := strings.TrimSpace(padded)
	return strings.Contains(padded, " wait ") ||
		strings.Contains(padded, ";wait ") ||
		strings.Contains(padded, "&&wait ") ||
		strings.HasPrefix(trimmed, "wait ") ||
		strings.HasSuffix(trimmed, " wait") ||
		trimmed == "wait"
}

func shellCommandLineContainsStandaloneAmpersand(commandLine string) bool {
	for idx, value := range commandLine {
		if value != '&' {
			continue
		}
		prevBoundary := idx == 0 || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[idx-1]))
		nextIdx := idx + len(string(value))
		nextBoundary := nextIdx >= len(commandLine) || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[nextIdx]))
		if prevBoundary && nextBoundary {
			return true
		}
	}
	return false
}
