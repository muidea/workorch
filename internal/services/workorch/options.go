package workorch

import (
	"context"
	"errors"
	"fmt"
	"os"
)

type Mode string

const (
	ModeInteractive Mode = "interactive"
	ModeServer      Mode = "server"
	ModeRemote      Mode = "remote"
)

type Options struct {
	ProgramName string
	Args        []string
	ForceServer bool
}

type StartupArgs struct {
	ConfigDir string
	CobraArgs []string
	Remote    string
	Mode      Mode
}

func Run(ctx context.Context, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := ParseStartupArgs(opts.Args, opts.ForceServer)
	if err != nil {
		return err
	}
	svc, err := NewService(opts.ProgramName, parsed)
	if err != nil {
		return err
	}
	if err := svc.Startup(ctx); err != nil {
		return err
	}
	runErr := svc.Run(ctx)
	return errors.Join(runErr, svc.Shutdown(context.Background()))
}

func ParseStartupArgs(args []string, forceServer bool) (StartupArgs, error) {
	ret := StartupArgs{Mode: ModeInteractive}
	if forceServer {
		ret.Mode = ModeServer
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "serve", "server":
			ret.Mode = ModeServer
			continue
		case "--server", "-server", "--daemon", "-daemon":
			ret.Mode = ModeServer
			continue
		case "--config-dir", "-config-dir":
			if i+1 >= len(args) {
				return ret, fmt.Errorf("%s requires a value", arg)
			}
			ret.ConfigDir = args[i+1]
			i++
			continue
		case "--remote":
			if i+1 >= len(args) {
				return ret, fmt.Errorf("%s requires a value", arg)
			}
			ret.Remote = args[i+1]
			ret.Mode = ModeRemote
			ret.CobraArgs = append(ret.CobraArgs, arg, args[i+1])
			i++
			continue
		}
		ret.CobraArgs = append(ret.CobraArgs, arg)
	}
	return ret, nil
}

func withArgs(programName string, args []string, fn func() error) error {
	if programName == "" {
		programName = os.Args[0]
	}
	previous := os.Args
	os.Args = append([]string{programName}, args...)
	defer func() {
		os.Args = previous
	}()
	return fn()
}
