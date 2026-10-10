package workorch

import (
	"context"
	"errors"
	"fmt"
	"os"

	frameworkservice "github.com/muidea/magicCommon/framework/service"
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
	return runLifecycle(ctx, svc)
}

func runLifecycle(ctx context.Context, svc frameworkservice.LifecycleService) (err error) {
	// Startup 已可能进入部分阶段；普通错误与 panic 都必须走同一检查式停止路径。
	defer func() {
		recovered := recover()
		if recovered != nil {
			if cause, ok := recovered.(error); ok {
				err = errors.Join(err, fmt.Errorf("service lifecycle panic: %w", cause))
			} else {
				err = errors.Join(err, fmt.Errorf("service lifecycle panic: %v", recovered))
			}
		}
		err = errors.Join(err, shutdownLifecycle(svc))
	}()
	if err = svc.Startup(ctx); err != nil {
		return err
	}
	return svc.Run(ctx)
}

func shutdownLifecycle(svc frameworkservice.LifecycleService) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if cause, ok := recovered.(error); ok {
				err = fmt.Errorf("service shutdown panic: %w", cause)
			} else {
				err = fmt.Errorf("service shutdown panic: %v", recovered)
			}
		}
	}()
	return svc.Shutdown(context.Background())
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
