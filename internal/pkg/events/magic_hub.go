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
	defaultMagicHubSubscribeTimeout   = 100 * time.Millisecond
	defaultMagicHubUnsubscribeTimeout = 100 * time.Millisecond
	wildcardTopic                     = "*"
	magicWildcardTopic                = "/#"
	magicWorkorchObserverDestination  = "/_workorch_observer/#"
	defaultMagicHubOwner              = "workorch/events"
)

type MagicHub struct {
	mu            sync.RWMutex
	hub           magicevent.Hub
	ownsHub       bool
	owner         string
	nextID        int
	revisionState *workspaceRevisionState
	subscriptions map[int]magicSubscription
	observers     map[string]*magicDispatchObserver
	published     []Envelope
}

type workspaceRevisionState struct {
	mu        sync.Mutex
	revisions map[string]int64
}

var sharedMagicRevisionStates sync.Map

type magicSubscription struct {
	topic     string
	eventID   string
	localOnly bool
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
		revisionState: sharedMagicRevisionState(hub),
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
	envelope.Revision = h.nextWorkspaceRevisionLocked(envelope.WorkspaceID)
	h.recordPublishedEnvelopeLocked(envelope)
	h.mu.Unlock()
	if err := h.dispatchLocalWildcard(ctx, envelope); err != nil {
		return err
	}

	ev := magicevent.NewEventWithContext(
		envelope.Topic,
		envelope.SourceModule,
		magicWorkorchObserverDestination,
		magicevent.NewHeader(),
		ctx,
		cloneEnvelope(envelope),
	)
	ev.BindLaneKey(EventHubLaneKey(envelope))
	result := h.hub.Send(ev)
	if result == nil {
		return nil
	}
	if result.Error() != nil && strings.Contains(result.Error().Message, "missing observer") {
		return nil
	}
	if err := cd.ToStdError(result.Error()); err != nil {
		return fmt.Errorf("handle event %s: %w", envelope.Topic, err)
	}
	_, resultErr := result.Get()
	if err := cd.ToStdError(resultErr); err != nil {
		return fmt.Errorf("handle event %s: %w", envelope.Topic, err)
	}
	return nil
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
	envelope.Revision = h.nextWorkspaceRevisionLocked(envelope.WorkspaceID)
	h.recordPublishedEnvelopeLocked(envelope)
	h.mu.Unlock()
	if err := h.dispatchLocalWildcard(ctx, envelope); err != nil {
		return err
	}

	ev := magicevent.NewEventWithContext(
		envelope.Topic,
		envelope.SourceModule,
		magicWorkorchObserverDestination,
		magicevent.NewHeader(),
		ctx,
		cloneEnvelope(envelope),
	)
	ev.BindLaneKey(EventHubLaneKey(envelope))
	h.hub.Post(ev)
	return nil
}

func (h *MagicHub) nextWorkspaceRevisionLocked(workspaceID string) int64 {
	if workspaceID == "" || h.revisionState == nil {
		return 0
	}
	return h.revisionState.next(workspaceID)
}

func (h *MagicHub) recordPublishedEnvelopeLocked(envelope Envelope) {
	if h == nil {
		return
	}
	h.published = append(h.published, cloneEnvelope(envelope))
}

func (s *workspaceRevisionState) next(workspaceID string) int64 {
	if s == nil || workspaceID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revisions == nil {
		s.revisions = map[string]int64{}
	}
	s.revisions[workspaceID]++
	return s.revisions[workspaceID]
}

func sharedMagicRevisionState(hub magicevent.Hub) *workspaceRevisionState {
	if key := magicHubIdentityKey(hub); key != "" {
		if state, _ := sharedMagicRevisionStates.LoadOrStore(key, &workspaceRevisionState{revisions: map[string]int64{}}); state != nil {
			if typed, ok := state.(*workspaceRevisionState); ok {
				return typed
			}
		}
	}
	return &workspaceRevisionState{revisions: map[string]int64{}}
}

func magicHubIdentityKey(hub magicevent.Hub) string {
	if hub == nil {
		return ""
	}
	return fmt.Sprintf("%T/%p", hub, hub)
}

func (h *MagicHub) Subscribe(topic string, handler Handler) func() {
	if h == nil || h.hub == nil || handler == nil {
		return func() {}
	}
	eventID := magicEventID(topic)
	localOnly := eventID == magicWildcardTopic

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	observer := h.observers[eventID]
	needsSubscribe := false
	if observer == nil {
		observer = &magicDispatchObserver{id: magicObserverID(h.owner, eventID), handlers: map[int]Handler{}}
		h.observers[eventID] = observer
		if !localOnly {
			needsSubscribe = true
		}
	}
	observer.add(id, handler)
	h.subscriptions[id] = magicSubscription{
		topic:     topic,
		eventID:   eventID,
		localOnly: localOnly,
	}
	h.mu.Unlock()
	if needsSubscribe {
		subscribeMagicHubWithTimeout(h.hub, eventID, observer, defaultMagicHubSubscribeTimeout)
	}

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
		if ok && observer != nil && !subscription.localOnly {
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

func subscribeMagicHubWithTimeout(hub magicevent.Hub, eventID string, observer magicevent.Observer, timeout time.Duration) {
	if hub == nil || observer == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		hub.Subscribe(eventID, observer)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (h *MagicHub) dispatchLocalWildcard(ctx context.Context, envelope Envelope) error {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	observer := h.observers[magicWildcardTopic]
	var handlers []Handler
	if observer != nil {
		handlers = observer.snapshotHandlers()
	}
	h.mu.RUnlock()
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, cloneEnvelope(envelope)); err != nil {
			return fmt.Errorf("handle event %s: %w", envelope.Topic, err)
		}
	}
	return nil
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

func (h *MagicHub) Terminate(ctx context.Context) {
	if h == nil || h.hub == nil || !h.ownsHub {
		return
	}
	h.hub.Terminate(ctx)
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
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
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
