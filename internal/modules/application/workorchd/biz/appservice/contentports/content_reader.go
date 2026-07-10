package contentports

import (
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
)

type Reader struct {
	eventHub          magicevent.Hub
	repositoryBackend string
	repositoryDataDir string
}

func NewReader(eventHub magicevent.Hub, repositoryBackend string, repositoryDataDir string) *Reader {
	return &Reader{
		eventHub:          eventHub,
		repositoryBackend: strings.TrimSpace(repositoryBackend),
		repositoryDataDir: strings.TrimSpace(repositoryDataDir),
	}
}
