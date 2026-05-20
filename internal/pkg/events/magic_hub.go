package events

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	defaultMagicHubCapacity           = 1024
	defaultMagicHubUnsubscribeTimeout = 100 * time.Millisecond
	wildcardTopic                     = "*"
	magicWildcardTopic                = "/#"
	defaultMagicHubOwner              = "workorch/events"
)

type MagicHub struct {
	mu            sync.RWMutex
	hub           magicevent.Hub
	ownsHub       bool
	owner         string
	nextID        int
	subscriptions map[int]magicSubscription
	observers     map[string]*magicDispatchObserver
	published     []Envelope
}

type magicSubscription struct {
	topic   string
	eventID string
}

type magicDispatchObserver struct {
	id       string
	mu       sync.RWMutex
	handlers map[int]Handler
}

func NewMagicHub() *MagicHub {
	return NewMagicHubWithOwner(defaultMagicHubOwner)
}

func NewMagicHubWithOwner(owner string) *MagicHub {
	return newMagicHub(magicevent.NewHub(defaultMagicHubCapacity), true, owner)
}

func NewMagicHubWithHub(hub magicevent.Hub) *MagicHub {
	return NewMagicHubWithHubOwner(hub, defaultMagicHubOwner)
}

func NewMagicHubWithHubOwner(hub magicevent.Hub, owner string) *MagicHub {
	if hub == nil {
		return NewMagicHubWithOwner(owner)
	}
	return newMagicHub(hub, false, owner)
}

func newMagicHub(hub magicevent.Hub, ownsHub bool, owner string) *MagicHub {
	return &MagicHub{
		hub:           hub,
		ownsHub:       ownsHub,
		owner:         normalizeMagicHubOwner(owner),
		subscriptions: map[int]magicSubscription{},
		observers:     map[string]*magicDispatchObserver{},
	}
}

func (h *MagicHub) Publish(ctx context.Context, envelope Envelope) error {
	if h == nil || h.hub == nil {
		return common.ErrInvalidArgument
	}
	envelope = prepareEnvelope(envelope)
	if envelope.Topic == "" {
		return common.ErrInvalidArgument
	}

	h.mu.Lock()
	h.published = append(h.published, cloneEnvelope(envelope))
	h.mu.Unlock()

	ev := magicevent.NewEventWithContext(
		envelope.Topic,
		envelope.SourceModule,
		magicWildcardTopic,
		magicevent.NewHeader(),
		ctx,
		cloneEnvelope(envelope),
	)
	ev.BindLaneKey(envelope.Topic)
	result := h.hub.Send(ev)
	if result == nil {
		return nil
	}
	if result.Error() != nil && strings.Contains(result.Error().Message, "missing observer") {
		return nil
	}
	if data, resultErr := result.Get(); resultErr == nil {
		if envelope, ok := data.(Envelope); ok {
			return Response(envelope)
		}
		return nil
	}
	return fmt.Errorf("handle event %s: %s", envelope.Topic, result.Error().Error())
}

func (h *MagicHub) PublishAsync(ctx context.Context, envelope Envelope) error {
	if h == nil || h.hub == nil {
		return common.ErrInvalidArgument
	}
	envelope = prepareEnvelope(envelope)
	if envelope.Topic == "" {
		return common.ErrInvalidArgument
	}

	h.mu.Lock()
	h.published = append(h.published, cloneEnvelope(envelope))
	h.mu.Unlock()

	ev := magicevent.NewEventWithContext(
		envelope.Topic,
		envelope.SourceModule,
		magicWildcardTopic,
		magicevent.NewHeader(),
		ctx,
		cloneEnvelope(envelope),
	)
	ev.BindLaneKey(envelope.Topic)
	h.hub.Post(ev)
	return nil
}

func (h *MagicHub) Subscribe(topic string, handler Handler) func() {
	if h == nil || h.hub == nil || handler == nil {
		return func() {}
	}
	eventID := magicEventID(topic)

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	observer := h.observers[eventID]
	if observer == nil {
		observer = &magicDispatchObserver{id: magicObserverID(h.owner, eventID), handlers: map[int]Handler{}}
		h.observers[eventID] = observer
		h.hub.Subscribe(eventID, observer)
	}
	observer.add(id, handler)
	h.subscriptions[id] = magicSubscription{
		topic:   topic,
		eventID: eventID,
	}
	h.mu.Unlock()

	return func() {
		h.mu.Lock()
		subscription, ok := h.subscriptions[id]
		if ok {
			delete(h.subscriptions, id)
		}
		observer := h.observers[subscription.eventID]
		if observer != nil {
			observer.remove(id)
			if observer.empty() {
				delete(h.observers, subscription.eventID)
			} else {
				observer = nil
			}
		}
		h.mu.Unlock()
		if ok && observer != nil {
			done := make(chan struct{})
			go func() {
				h.hub.Unsubscribe(subscription.eventID, observer)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(defaultMagicHubUnsubscribeTimeout):
			}
		}
	}
}

func (h *MagicHub) Published() []Envelope {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Envelope, 0, len(h.published))
	for _, item := range h.published {
		out = append(out, cloneEnvelope(item))
	}
	return out
}

func (h *MagicHub) Terminate() {
	if h == nil || h.hub == nil || !h.ownsHub {
		return
	}
	h.hub.Terminate()
}

func (o *magicDispatchObserver) ID() string {
	if o == nil {
		return ""
	}
	return o.id
}

func (o *magicDispatchObserver) Notify(ev magicevent.Event, result magicevent.Result) {
	if o == nil {
		if result != nil {
			result.Set(nil, nil)
		}
		return
	}
	envelope, ok := ev.Data().(Envelope)
	if !ok {
		if result != nil {
			result.Set(nil, cd.NewError(cd.Unexpected, "workorch event envelope type mismatch"))
		}
		return
	}

	handlers := o.snapshotHandlers()
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ev.Context(), cloneEnvelope(envelope)); err != nil {
			if result != nil {
				if responseEnvelope, ok := ResponseEnvelope(err); ok {
					result.Set(responseEnvelope, nil)
					return
				}
				result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
			}
			return
		}
	}
	if result != nil {
		result.Set(nil, nil)
	}
}

func (o *magicDispatchObserver) add(id int, handler Handler) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.handlers[id] = handler
}

func (o *magicDispatchObserver) remove(id int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.handlers, id)
}

func (o *magicDispatchObserver) empty() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.handlers) == 0
}

func (o *magicDispatchObserver) snapshotHandlers() []Handler {
	o.mu.RLock()
	defer o.mu.RUnlock()
	handlers := make([]Handler, 0, len(o.handlers))
	for _, handler := range o.handlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

func magicEventID(topic string) string {
	if topic == wildcardTopic {
		return magicWildcardTopic
	}
	return topic
}

func magicObserverID(owner string, eventID string) string {
	owner = normalizeMagicHubOwner(owner)
	if eventID == magicWildcardTopic {
		return "/_workorch_observer/" + owner + "/all"
	}
	if !strings.HasPrefix(eventID, "/") {
		eventID = "/" + eventID
	}
	return "/_workorch_observer/" + owner + "/" + strings.Trim(strings.TrimSpace(eventID), "/")
}

func normalizeMagicHubOwner(owner string) string {
	owner = strings.Trim(strings.TrimSpace(owner), "/")
	if owner == "" {
		return defaultMagicHubOwner
	}
	parts := strings.FieldsFunc(owner, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= 'A' && r <= 'Z':
			return false
		case r >= '0' && r <= '9':
			return false
		case r == '/', r == '-', r == '_', r == '.':
			return false
		default:
			return true
		}
	})
	owner = strings.Join(parts, "-")
	owner = strings.Trim(owner, "/-_.")
	if owner == "" {
		return defaultMagicHubOwner
	}
	return owner
}

func prepareEnvelope(envelope Envelope) Envelope {
	envelope = cloneEnvelope(envelope)
	if envelope.ID == "" {
		envelope.ID = fmt.Sprintf("evt-%d", time.Now().UTC().UnixNano())
	}
	if envelope.Version == "" {
		envelope.Version = "v1"
	}
	if envelope.OccurredAt == "" {
		envelope.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	return envelope
}
