package biz

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
)

// runtimeComponent is the sole Workorchd owner of the process runtime session.
// It exposes only typed application ports and short-lived CLI/gateway helpers;
// the session itself never crosses the service/server transport boundary.
type runtimeComponent struct {
	session        *runtimecore.Session
	ports          server.ApplicationPorts
	authoringTasks *authoringTaskLifecycle
}

func newRuntimeComponent(session *runtimecore.Session) *runtimeComponent {
	if session == nil {
		return nil
	}
	component := &runtimeComponent{session: session, authoringTasks: newAuthoringTaskLifecycle()}
	component.ports = newApplicationPorts(component.withSession, component.authoringTasks)
	return component
}

func newRuntimeComponentFromCommandHub(hub magicevent.Hub, background task.BackgroundRoutine) (*runtimeComponent, error) {
	session, err := appservice.NewRuntimeSessionFromCommandHub(hub, background)
	if err != nil {
		return nil, err
	}
	return newRuntimeComponent(session), nil
}

func (c *runtimeComponent) ready() bool {
	return c != nil && c.session != nil
}

func (c *runtimeComponent) applicationPorts() server.ApplicationPorts {
	if c == nil {
		return server.ApplicationPorts{}
	}
	return c.ports
}

func (c *runtimeComponent) newCLICommandDispatcher(hub magicevent.Hub, reader appservice.WorkspaceViewReader) *appservice.CLICommandDispatcher {
	if c == nil || c.session == nil {
		return nil
	}
	return appservice.NewCLICommandDispatcher(c.session, hub, reader)
}

func (c *runtimeComponent) borrow() *runtimecore.Session {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.Borrow()
}

// withSession is the only place where a borrowed runtime session is opened
// for application-port callbacks.  runtimeApplicationPorts stores this
// operation, not the long-lived session pointer, so runtimeComponent remains
// the sole session owner.
func (c *runtimeComponent) withSession(ctx context.Context, fn func(*runtimecore.Session) error) error {
	if c == nil || c.session == nil || fn == nil {
		return errRuntimeComponentUnavailable
	}
	borrowed := c.session.Borrow()
	if borrowed == nil {
		return errors.New("workorch runtime session borrow failed")
	}
	err := fn(borrowed)
	return errors.Join(err, borrowed.Close(ctx))
}

func (c *runtimeComponent) close(ctx context.Context) error {
	if c == nil || c.session == nil {
		return nil
	}
	c.authoringTasks.beginStop(ctx)
	if err := c.authoringTasks.waitContext(ctx); err != nil {
		return err
	}
	if err := c.session.Close(ctx); err != nil {
		return err
	}
	c.session = nil
	c.ports = server.ApplicationPorts{}
	return nil
}

var errRuntimeComponentUnavailable = errors.New("workorch runtime component is not configured")
