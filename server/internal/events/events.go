// Package events fans out live changes (task progress, scans, analysis,
// read progress) to the open /api/v1/events streams. It only carries
// messages; who may see which event is decided by the stream handler in
// httpapi, which knows the user.
package events

import (
	"errors"
	"sync"
)

// Event is one message. Exactly one audience field is meant to apply:
// AdminOnly events go to administrators, UserID events to that user only,
// and LibraryID events to everyone who may see that library. An event with
// none of them goes to every signed-in user.
type Event struct {
	Type      string // task | library | book | progress
	LibraryID string
	UserID    string
	AdminOnly bool
	Data      any // marshalled as the event's JSON data
}

// Buffer is how many events a slow stream may fall behind before it is
// dropped. A dropped stream reconnects and the client refetches.
const Buffer = 256

// Stream limits: each holds a goroutine, a ticker and a Buffer of events,
// and every Publish visits every stream. A user rarely has more than a few
// tabs and devices open.
const (
	MaxPerKey = 16
	MaxTotal  = 512
)

// ErrTooMany means a new stream would pass MaxPerKey or MaxTotal.
var ErrTooMany = errors.New("events: too many streams")

// Hub is safe for concurrent use. The zero value is not; use New. A nil
// *Hub accepts Publish and does nothing, so producers need no checks.
type Hub struct {
	mu     sync.Mutex
	subs   map[*Sub]struct{}
	perKey map[string]int
	closed bool
}

func New() *Hub { return &Hub{subs: map[*Sub]struct{}{}, perKey: map[string]int{}} }

// Sub is one subscriber. C is closed when the hub drops it (it fell behind)
// or the hub closes.
type Sub struct {
	C    chan Event
	key  string
	once sync.Once
	gone bool // counted out of perKey; guarded by Hub.mu
}

func (s *Sub) close() { s.once.Do(func() { close(s.C) }) }

// Subscribe adds a subscriber counted under key (the user). Call
// Unsubscribe when done. It fails with ErrTooMany past the limits. On a
// closed hub the returned Sub's channel is already closed.
func (h *Hub) Subscribe(key string) (*Sub, error) {
	s := &Sub{C: make(chan Event, Buffer), key: key}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		s.gone = true
		s.close()
		return s, nil
	}
	if len(h.subs) >= MaxTotal || h.perKey[key] >= MaxPerKey {
		return nil, ErrTooMany
	}
	h.subs[s] = struct{}{}
	h.perKey[key]++
	return s, nil
}

func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	h.drop(s)
	h.mu.Unlock()
	s.close()
}

// drop removes s and frees its slot once; h.mu is held.
func (h *Hub) drop(s *Sub) {
	delete(h.subs, s)
	if s.gone {
		return
	}
	s.gone = true
	if h.perKey[s.key]--; h.perKey[s.key] <= 0 {
		delete(h.perKey, s.key)
	}
}

// Publish hands ev to every subscriber without blocking: one whose buffer
// is full is dropped.
func (h *Hub) Publish(ev Event) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.C <- ev:
		default:
			h.drop(s)
			s.close()
		}
	}
}

// Close ends every stream and refuses new ones (server shutdown).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.drop(s)
		s.close()
	}
}

// Len is the number of subscribers (for tests).
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Library is the Data of a "library" event: a scan or an analysis changed
// the library's series or books.
type Library struct {
	LibraryID     string `json:"libraryId"`
	SeriesAdded   int    `json:"seriesAdded"`
	SeriesRemoved int    `json:"seriesRemoved"`
	BooksAdded    int    `json:"booksAdded"`
	BooksChanged  int    `json:"booksChanged"`
	BooksRemoved  int    `json:"booksRemoved"`
	Restored      int    `json:"restored"`
}

// Book is the Data of a "book" event: a book was analysed.
type Book struct {
	BookID      string `json:"bookId"`
	SeriesID    string `json:"seriesId"`
	LibraryID   string `json:"libraryId"`
	MediaStatus string `json:"mediaStatus"`
}

// Progress is the Data of a "progress" event: the user's reading progress
// changed on a book, a series or a whole library.
type Progress struct {
	LibraryID string `json:"libraryId"`
	SeriesID  string `json:"seriesId,omitempty"`
	BookID    string `json:"bookId,omitempty"`
	Writer    string `json:"writer,omitempty"`
}
