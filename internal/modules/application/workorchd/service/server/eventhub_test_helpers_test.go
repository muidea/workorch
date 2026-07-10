package server

import (
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/events"
)

func newIsolatedTestHTTPServerWithBackground(config Config, eventHub events.Hub, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	return NewHTTPServerWithEventHubsAndBackground(config, eventHub, nil, backgroundRoutine)
}

func newIsolatedTestServerStateWithBackground(config Config, eventHub events.Hub, backgroundRoutine task.BackgroundRoutine) *serverState {
	return newServerStateWithHubsAndBackground(config, eventHub, nil, backgroundRoutine)
}
