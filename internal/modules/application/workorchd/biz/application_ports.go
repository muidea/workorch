package biz

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
)

// runtimeApplicationPorts is the Biz-owned adapter from the long-lived
// runtime session to the narrow HTTP application ports. The session never
// crosses into service/server; each call borrows a short-lived session and
// closes it before returning.
type runtimeApplicationPorts struct {
	withSessionFn  func(context.Context, func(*runtimecore.Session) error) error
	ports          server.ApplicationPorts
	authoringTasks *authoringTaskLifecycle
}

func newApplicationPorts(withSession func(context.Context, func(*runtimecore.Session) error) error, lifecycle ...*authoringTaskLifecycle) server.ApplicationPorts {
	authoring := newAuthoringTaskLifecycle()
	if len(lifecycle) == 1 && lifecycle[0] != nil {
		authoring = lifecycle[0]
	}
	p := &runtimeApplicationPorts{withSessionFn: withSession, authoringTasks: authoring}
	p.configureConfigurationPorts()
	p.configureWorkspacePorts()
	p.configureDefinitionPorts()
	p.configureAuthoringPorts()
	p.configureRunPorts()
	p.configureGovernancePorts()
	p.configureRegistryPorts()
	p.configureMemoryPorts()
	return p.ports
}

func (p *runtimeApplicationPorts) withSession(ctx context.Context, fn func(*runtimecore.Session) error) error {
	if p == nil || p.withSessionFn == nil || fn == nil {
		return errors.New("workorch runtime session is not configured")
	}
	return p.withSessionFn(ctx, fn)
}
