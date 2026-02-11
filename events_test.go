package memcache

import (
	"testing"
	"time"
)

func TestSubscribeReceivesSetEvents(t *testing.T) {
	c := New()
	sub := c.Subscribe(EventSet)
	defer sub.Close()

	c.Set("key1", []byte("val1"))

	select {
	case e := <-sub.C:
		if e.Type != EventSet {
			t.Fatalf("expected EventSet, got %d", e.Type)
		}
		if e.Key != "key1" {
			t.Fatalf("expected key1, got %s", e.Key)
		}
		if e.Size != len("key1")+len("val1") {
			t.Fatalf("unexpected size %d", e.Size)
		}
		if e.Timestamp.IsZero() {
			t.Fatal("timestamp should be set")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesDeleteEvents(t *testing.T) {
	c := New()
	c.Set("key1", []byte("val1"))

	sub := c.Subscribe(EventDelete)
	defer sub.Close()

	c.Delete("key1")

	select {
	case e := <-sub.C:
		if e.Type != EventDelete {
			t.Fatalf("expected EventDelete, got %d", e.Type)
		}
		if e.Key != "key1" {
			t.Fatalf("expected key1, got %s", e.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesEvictEvents(t *testing.T) {
	c := NewWithMaxSize(2)
	sub := c.Subscribe(EventEvict)
	defer sub.Close()

	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	c.Set("c", []byte("3")) // evicts "a"

	select {
	case e := <-sub.C:
		if e.Type != EventEvict {
			t.Fatalf("expected EventEvict, got %d", e.Type)
		}
		if e.Key != "a" {
			t.Fatalf("expected evicted key 'a', got %s", e.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesExpireEventsOnGet(t *testing.T) {
	c := New()
	c.SetWithTTL("key1", []byte("val1"), 10*time.Millisecond)

	sub := c.Subscribe(EventExpire)
	defer sub.Close()

	time.Sleep(20 * time.Millisecond)
	c.Get("key1") // triggers lazy expiry

	select {
	case e := <-sub.C:
		if e.Type != EventExpire {
			t.Fatalf("expected EventExpire, got %d", e.Type)
		}
		if e.Key != "key1" {
			t.Fatalf("expected key1, got %s", e.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesExpireEventsOnHas(t *testing.T) {
	c := New()
	c.SetWithTTL("key1", []byte("val1"), 10*time.Millisecond)

	sub := c.Subscribe(EventExpire)
	defer sub.Close()

	time.Sleep(20 * time.Millisecond)
	c.Has("key1") // triggers lazy expiry

	select {
	case e := <-sub.C:
		if e.Type != EventExpire {
			t.Fatalf("expected EventExpire, got %d", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeReceivesClearEvents(t *testing.T) {
	c := New()
	c.Set("key1", []byte("val1"))

	sub := c.Subscribe(EventClear)
	defer sub.Close()

	c.Clear()

	select {
	case e := <-sub.C:
		if e.Type != EventClear {
			t.Fatalf("expected EventClear, got %d", e.Type)
		}
		if e.Key != "" {
			t.Fatalf("expected empty key for clear, got %s", e.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribeAllEvents(t *testing.T) {
	c := NewWithMaxSize(1)
	sub := c.Subscribe() // all events
	defer sub.Close()

	c.Set("a", []byte("1"))           // EventSet
	c.Set("b", []byte("2"))           // EventEvict + EventSet
	c.Delete("b")                     // EventDelete

	events := drainEvents(sub.C, 4, time.Second)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}
	if events[0].Type != EventSet {
		t.Fatalf("event 0: expected EventSet, got %d", events[0].Type)
	}
	if events[1].Type != EventEvict {
		t.Fatalf("event 1: expected EventEvict, got %d", events[1].Type)
	}
	if events[2].Type != EventSet {
		t.Fatalf("event 2: expected EventSet, got %d", events[2].Type)
	}
	if events[3].Type != EventDelete {
		t.Fatalf("event 3: expected EventDelete, got %d", events[3].Type)
	}
}

func TestSubscribeFiltersByType(t *testing.T) {
	c := New()
	sub := c.Subscribe(EventDelete)
	defer sub.Close()

	c.Set("key1", []byte("val1")) // should NOT be received
	c.Delete("key1")              // should be received

	select {
	case e := <-sub.C:
		if e.Type != EventDelete {
			t.Fatalf("expected EventDelete, got %d", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscriptionClose(t *testing.T) {
	c := New()
	sub := c.Subscribe()
	sub.Close()

	// Channel should be closed
	_, ok := <-sub.C
	if ok {
		t.Fatal("expected channel to be closed after Close()")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	c := New()
	sub1 := c.Subscribe(EventSet)
	sub2 := c.Subscribe(EventSet)
	defer sub1.Close()
	defer sub2.Close()

	c.Set("key1", []byte("val1"))

	for _, sub := range []*Subscription{sub1, sub2} {
		select {
		case e := <-sub.C:
			if e.Type != EventSet || e.Key != "key1" {
				t.Fatalf("unexpected event: %+v", e)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestNoSubscribersZeroCost(t *testing.T) {
	c := New()
	// Just verify no panic when emitting with no subscribers
	c.Set("key1", []byte("val1"))
	c.Delete("key1")
	c.Clear()
}

func TestSubscribeDeleteExpired(t *testing.T) {
	c := New()
	c.SetWithTTL("a", []byte("1"), 10*time.Millisecond)
	c.SetWithTTL("b", []byte("2"), 10*time.Millisecond)

	sub := c.Subscribe(EventExpire)
	defer sub.Close()

	time.Sleep(20 * time.Millisecond)
	n := c.DeleteExpired()
	if n != 2 {
		t.Fatalf("expected 2 deleted, got %d", n)
	}

	events := drainEvents(sub.C, 2, time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 expire events, got %d", len(events))
	}
	for _, e := range events {
		if e.Type != EventExpire {
			t.Fatalf("expected EventExpire, got %d", e.Type)
		}
	}
}

func TestSetUpdateEmitsSet(t *testing.T) {
	c := New()
	sub := c.Subscribe(EventSet)
	defer sub.Close()

	c.Set("key1", []byte("val1"))
	c.Set("key1", []byte("val2")) // update

	events := drainEvents(sub.C, 2, time.Second)
	if len(events) != 2 {
		t.Fatalf("expected 2 set events, got %d", len(events))
	}
	for _, e := range events {
		if e.Type != EventSet {
			t.Fatalf("expected EventSet, got %d", e.Type)
		}
	}
}

func drainEvents(ch <-chan Event, n int, timeout time.Duration) []Event {
	var events []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for range n {
		select {
		case e := <-ch:
			events = append(events, e)
		case <-timer.C:
			return events
		}
	}
	return events
}
