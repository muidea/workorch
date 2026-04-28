package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	plugininitiator "github.com/muidea/magicCommon/framework/plugin/initiator"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type RuntimeSession struct {
	Bootstrap *bootstrap.Runtime
	Runtime   *ManagedRuntime
	exports   bootstrap.Exports
	borrowed  bool
}

func NewRuntimeSession(bootstrapRuntime *bootstrap.Runtime) *RuntimeSession {
	if bootstrapRuntime == nil {
		return &RuntimeSession{}
	}
	return &RuntimeSession{
		Bootstrap: bootstrapRuntime,
		Runtime:   NewRuntimeFromBootstrap(bootstrapRuntime),
	}
}

func NewRuntimeSessionFromExports(exports bootstrap.Exports) (*RuntimeSession, error) {
	if !hasWorkorchRuntimeExports(exports) {
		return nil, common.ErrInvalidArgument
	}
	return &RuntimeSession{
		Runtime:  NewRuntimeFromExports(exports),
		exports:  exports,
		borrowed: true,
	}, nil
}

func NewRuntimeSessionFromInitiator() (*RuntimeSession, error) {
	var helper runtimecommon.Helper
	runtime, err := plugininitiator.GetEntity(runtimecommon.InitiatorID, helper)
	if err != nil {
		return nil, err
	}
	return NewRuntimeSessionFromExports(exportsFromRuntimeHelper(runtime))
}

func (s *RuntimeSession) Borrow() *RuntimeSession {
	if s == nil {
		return &RuntimeSession{borrowed: true}
	}
	if s.Bootstrap == nil {
		return &RuntimeSession{
			Runtime:  NewRuntimeFromExports(s.exports),
			exports:  s.exports,
			borrowed: true,
		}
	}
	return &RuntimeSession{
		Bootstrap: s.Bootstrap,
		Runtime:   NewRuntimeFromBootstrap(s.Bootstrap),
		borrowed:  true,
	}
}

func NewRuntimeSessionWithBackend(backend string, dataRoot string) (*RuntimeSession, error) {
	bootstrapRuntime, err := buildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrap.NewWithBackend(backend, dataRoot)
	})
	if err != nil {
		return nil, err
	}
	return NewRuntimeSession(bootstrapRuntime), nil
}

func NewRuntimeSessionWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) (*RuntimeSession, error) {
	bootstrapRuntime, err := buildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrap.NewWithBackendAndEventHub(backend, dataRoot, eventHub)
	})
	if err != nil {
		return nil, err
	}
	return NewRuntimeSession(bootstrapRuntime), nil
}

func NewRuntimeSessionFromEnv() (*RuntimeSession, error) {
	bootstrapRuntime, err := buildBootstrapRuntime(bootstrap.NewFromEnv)
	if err != nil {
		return nil, err
	}
	return NewRuntimeSession(bootstrapRuntime), nil
}

func (s *RuntimeSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.Runtime != nil {
		s.Runtime.Close()
		s.Runtime = nil
	}
	if s.Bootstrap == nil {
		if s.borrowed {
			s.exports = bootstrap.Exports{}
		}
		return nil
	}
	if s.borrowed {
		s.Bootstrap = nil
		return nil
	}
	err := s.Bootstrap.Shutdown(ctx)
	s.Bootstrap = nil
	return err
}

func (s *RuntimeSession) CloseOnReturn(ctx context.Context, errp *error) {
	if errp == nil {
		return
	}
	*errp = errors.Join(*errp, s.Close(ctx))
}

func (s *RuntimeSession) Exports() (bootstrap.Exports, error) {
	if s == nil {
		return bootstrap.Exports{}, common.ErrInvalidArgument
	}
	if s.Bootstrap == nil {
		if !hasWorkorchRuntimeExports(s.exports) {
			return bootstrap.Exports{}, common.ErrInvalidArgument
		}
		return s.exports, nil
	}
	return s.Bootstrap.Exports(), nil
}

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprint(recovered)
			if strings.HasPrefix(message, "unsupported repository backend: ") {
				err = fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
				runtime = nil
				return
			}
			panic(recovered)
		}
	}()
	return factory(), nil
}

type eventHubContextKey struct{}

func ContextWithEventHub(ctx context.Context, eventHub events.Hub) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if eventHub == nil {
		return ctx
	}
	return context.WithValue(ctx, eventHubContextKey{}, eventHub)
}

func EventHubFromContext(ctx context.Context) events.Hub {
	if ctx == nil {
		return nil
	}
	eventHub, _ := ctx.Value(eventHubContextKey{}).(events.Hub)
	return eventHub
}

func newRuntimeSessionWithBackend(ctx context.Context, backend string, dataRoot string) (*RuntimeSession, error) {
	return NewRuntimeSessionWithBackendAndEventHub(backend, dataRoot, EventHubFromContext(ctx))
}

func exportsFromRuntimeHelper(runtime runtimecommon.Helper) bootstrap.Exports {
	if runtime == nil {
		return bootstrap.Exports{}
	}
	repos := runtime.Repositories()
	return bootstrap.Exports{
		Repositories: bootstrap.RepositoryExports{
			Souls:              repos.Souls,
			MemoryItems:        repos.MemoryItems,
			Skills:             repos.Skills,
			MCPServers:         repos.MCPServers,
			Workspaces:         repos.Workspaces,
			Goals:              repos.Goals,
			DoDs:               repos.DoDs,
			Results:            repos.Results,
			Artifacts:          repos.Artifacts,
			EventRecords:       repos.EventRecords,
			Runs:               repos.Runs,
			Steps:              repos.Steps,
			ToolCalls:          repos.ToolCalls,
			Patches:            repos.Patches,
			ValidationResults:  repos.ValidationResults,
			RuntimeCheckpoints: repos.RuntimeCheckpoints,
			Tickets:            repos.Tickets,
			Resolutions:        repos.Resolutions,
			Approvals:          repos.Approvals,
		},
		Services: bootstrap.ServiceExports{
			WorkspaceService:   runtime.WorkspaceService(),
			SoulService:        runtime.SoulService(),
			MemoryService:      runtime.MemoryService(),
			SkillService:       runtime.SkillService(),
			MCPService:         runtime.MCPService(),
			EventStreamService: runtime.EventStreamService(),
			DecisionService:    runtime.DecisionService(),
			ApprovalService:    runtime.ApprovalService(),
			RunService:         runtime.RunService(),
		},
		Infrastructure: bootstrap.InfrastructureExports{
			EventHub: runtime.EventHub(),
		},
	}
}
