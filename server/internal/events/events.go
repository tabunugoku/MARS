// Package events fans out live changes (task progress, scans, analysis,
// read progress) to the open /api/v1/events streams. It only carries
// messages; who may see which event is decided by the stream handler in
// httpapi, which knows the user.
package events

import "sync"

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

// Hub is safe for concurrent use. The zero value is not; use New. A nil
// *Hub accepts Publish and does nothing, so producers need no checks.
type Hub struct {
	mu     sync.Mutex
	subs   map[*Sub]struct{}
	closed bool
}

func New() *Hub { return &Hub{subs: map[*Sub]struct{}{}} }

// Sub is one subscriber. C is closed when the hub drops it (it fell behind)
// or the hub closes.
type Sub struct {
	C    chan Event
	once sync.Once
}

func (s *Sub) close() { s.once.Do(func() { close(s.C) }) }

// Subscribe adds a subscriber. Call Unsubscribe when done. On a closed hub
// the returned Sub's channel is already closed.
func (h *Hub) Subscribe() *Sub {
	s := &Sub{C: make(chan Event, Buffer)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		s.close()
		return s
	}
	h.subs[s] = struct{}{}
	return s
}

func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	delete(h.subs, s)
	h.mu.Unlock()
	s.close()
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
			delete(h.subs, s)
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
		delete(h.subs, s)
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
