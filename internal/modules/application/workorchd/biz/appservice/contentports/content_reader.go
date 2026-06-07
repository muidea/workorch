package contentports

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/events"
)

type Reader struct {
	eventHub          events.Hub
	repositoryBackend string
	repositoryDataDir string
}

func NewReader(eventHub events.Hub, repositoryBackend string, repositoryDataDir string) *Reader {
	return &Reader{
		eventHub:          eventHub,
		repositoryBackend: strings.TrimSpace(repositoryBackend),
		repositoryDataDir: strings.TrimSpace(repositoryDataDir),
	}
}
