package memcache

import (
	"time"
)

// EventType represents the type of cache mutation.
type EventType int

const (
	EventSet    EventType = 1 << iota // Item was added or updated
	EventDelete                       // Item was explicitly deleted
	EventEvict                        // Item was evicted due to capacity
	EventExpire                       // Item was removed due to TTL expiry
	EventClear                        // Cache was cleared
)

// Event represents a cache mutation event.
type Event struct {
	Type      EventType
	Key       string
	Size      int
	Timestamp time.Time
}

// Subscription represents an active event subscription.
// Read events from C. Call Close when done.
type Subscription struct {
	C      <-chan Event
	cancel func()
}

// Close unsubscribes and releases resources.
func (s *Subscription) Close() { s.cancel() }

type subscriber struct {
	ch    chan Event
	types EventType // bitmask of subscribed event types
}

// Subscribe creates a subscription for the given event types.
// If no types are specified, all event types are subscribed.
// The returned Subscription has a buffered channel (256) that receives events.
// Events are dropped if the channel is full (slow consumer).
func (c *Cache) Subscribe(eventTypes ...EventType) *Subscription {
	var mask EventType
	for _, t := range eventTypes {
		mask |= t
	}

	if mask == 0 {
		mask = EventSet | EventDelete | EventEvict | EventExpire | EventClear
	}

	ch := make(chan Event, 256)
	sub := &subscriber{ch: ch, types: mask}

	c.subMu.Lock()
	c.subscribers = append(c.subscribers, sub)
	c.subMu.Unlock()

	return &Subscription{
		C: ch,
		cancel: func() {
			c.removeSub(sub)
		},
	}
}

func (c *Cache) removeSub(sub *subscriber) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, s := range c.subscribers {
		if s == sub {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			close(sub.ch)
			return
		}
	}
}

// emit sends an event to all matching subscribers. Non-blocking: drops events on full channels.
// Must NOT hold subMu when calling.
func (c *Cache) emit(e Event) {
	c.subMu.RLock()
	if len(c.subscribers) == 0 {
		c.subMu.RUnlock()
		return
	}
	e.Timestamp = time.Now()
	subs := make([]*subscriber, len(c.subscribers))
	copy(subs, c.subscribers)
	c.subMu.RUnlock()

	for _, sub := range subs {
		if sub.types&e.Type != 0 {
			select {
			case sub.ch <- e:
			default:
			}
		}
	}
}

// emitUnlocked is a convenience to build and emit an event.
// Call after the mutation is complete and the cache lock can be released.
func (c *Cache) emitEvent(typ EventType, key string, size int) {
	c.emit(Event{Type: typ, Key: key, Size: size})
}
