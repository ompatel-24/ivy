package session

import (
	"bytes"
	"sync"
)

// Subscription contains a point-in-time history snapshot followed by ordered
// live PTY output. Output byte slices are immutable and must not be modified.
type Subscription struct {
	session *Session
	id      uint64
	initial []byte
	output  chan []byte

	mu     sync.RWMutex
	err    error
	closed bool
}

// Initial returns a copy of the terminal history captured when the subscriber
// joined. Reading Output after Initial produces a gap-free stream.
func (s *Subscription) Initial() []byte {
	return bytes.Clone(s.initial)
}

// Output returns ordered live PTY chunks. The channel closes when the session
// ends, the subscription is closed, or the subscriber is evicted.
func (s *Subscription) Output() <-chan []byte {
	return s.output
}

// Err reports why the subscription closed. Normal session completion and an
// explicit Close return nil.
func (s *Subscription) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

// Close detaches this subscriber without affecting the child process.
func (s *Subscription) Close() {
	if s.session != nil {
		s.session.unsubscribe(s.id, nil)
	}
}

func (s *Subscription) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.err = err
	s.closed = true
	close(s.output)
}

// Subscribe atomically captures current history and joins the live output set.
func (s *Session) Subscribe() *Subscription {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	s.nextID++
	subscription := &Subscription{
		session: s,
		id:      s.nextID,
		initial: s.history.Bytes(),
		output:  make(chan []byte, s.options.subscriberBuffer),
	}
	if s.closed {
		subscription.finish(nil)
		return subscription
	}
	s.subscribers[subscription.id] = subscription
	return subscription
}

func (s *Session) unsubscribe(id uint64, err error) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	subscription, ok := s.subscribers[id]
	if !ok {
		return
	}
	delete(s.subscribers, id)
	subscription.finish(err)
}

func (s *Session) publish(data []byte) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	s.history.Write(data)
	for id, subscription := range s.subscribers {
		select {
		case subscription.output <- data:
		default:
			delete(s.subscribers, id)
			subscription.finish(ErrSlowSubscriber)
		}
	}
}

func (s *Session) closeSubscribers() {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	s.closed = true
	for id, subscription := range s.subscribers {
		delete(s.subscribers, id)
		subscription.finish(nil)
	}
}
