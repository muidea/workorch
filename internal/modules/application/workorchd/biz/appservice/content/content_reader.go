package content

import magicevent "github.com/muidea/magicCommon/event"

type Reader struct {
	eventHub magicevent.Hub
}

func NewReader(eventHub magicevent.Hub) *Reader {
	return &Reader{eventHub: eventHub}
}
