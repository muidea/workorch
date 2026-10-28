package contextregistryports

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func SkillHubRegistrationHome() (string, error) {
	home := os.Getenv("SKILL_HUB_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".skill-hub")
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return "", common.ErrConflict
	}
	return home, nil
}

// Registration is only called after the Workspace owner's reservation CAS.
// Recovery queries the original receipt, never repeats registration. The exact
// home is pinned in the child environment and checked against current scope.
func ObserveSkillHubRegistration(ctx context.Context, p skillhubpreparation.RegistrationPlan, query bool) skillhubpreparation.RegistrationObservation {
	o := skillhubpreparation.RegistrationObservation{StartedAt: time.Now().UTC()}
	var commandErr error
	deferFinish := func() skillhubpreparation.RegistrationObservation {
		o.PreserveError(commandErr)
		o.CompletedAt = time.Now().UTC()
		o.StdoutSHA256 = fileobservation.Digest(o.Stdout)
		o.StderrSHA256 = fileobservation.Digest(o.Stderr)
		return o
	}
	home, err := SkillHubRegistrationHome()
	if ctx == nil || p.Validate() != nil || err != nil || home != p.Home || ctx.Err() != nil {
		commandErr = common.ErrConflict
		if err != nil {
			commandErr = err
		}
		if ctx != nil && ctx.Err() != nil {
			commandErr = ctx.Err()
		}
		return deferFinish()
	}
	bound, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	args := []string{"project", "register"}
	if query {
		args = append(args, "--query")
	} else {
		args = append(args, p.Directory)
	}
	args = append(args, "--request-id", p.RequestID, "--json")
	cmd := exec.CommandContext(bound, "skill-hub", args...)
	cmd.Env = append(os.Environ(), "SKILL_HUB_HOME="+p.Home)
	cmd.WaitDelay = 100 * time.Millisecond
	stdout, stderr := &preparationStatusCapture{}, &preparationStatusCapture{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if commandErr = cmd.Start(); commandErr != nil {
		return deferFinish()
	}
	o.CommandStarted = true
	err = cmd.Wait()
	commandErr = err
	o.Stdout, o.TotalStdoutBytes = stdout.snapshot()
	o.Stderr, o.TotalStderrBytes = stderr.snapshot()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		o.ExitCodeKnown = true
		o.ExitCode = cmd.ProcessState.ExitCode()
	}
	o.Complete = bound.Err() == nil && !errors.Is(err, exec.ErrWaitDelay) && uint64(len(o.Stdout)) == o.TotalStdoutBytes && uint64(len(o.Stderr)) == o.TotalStderrBytes
	return deferFinish()
}
