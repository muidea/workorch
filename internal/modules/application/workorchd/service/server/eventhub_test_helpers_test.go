package server

import (
	"github.com/muidea/magicCommon/task"
)

func newIsolatedTestHTTPServerWithBackground(config Config, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	return NewHTTPServerWithCommandHubAndBackground(config, nil, backgroundRoutine)
}

func newIsolatedTestServerStateWithBackground(config Config, backgroundRoutine task.BackgroundRoutine) *serverState {
	return newServerStateWithCommandHubAndBackground(config, nil, backgroundRoutine)
}
