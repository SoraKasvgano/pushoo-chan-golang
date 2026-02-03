package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pushoo-chan-gover/internal/push"
)

// EventHub provides a lightweight SSE broadcaster for push events.
type EventHub struct {
	mu    sync.Mutex
	subs  map[chan push.Event]struct{}
	alive bool
}

func NewEventHub() *EventHub {
	return &EventHub{
		subs:  map[chan push.Event]struct{}{},
		alive: true,
	}
}

func (h *EventHub) Subscribe() (ch chan push.Event, unsubscribe func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch = make(chan push.Event, 32)
	h.subs[ch] = struct{}{}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *EventHub) Broadcast(ev push.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// Drop rather than block.
		}
	}
}

func (h *EventHub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	// SSE basics.
	w.Header().Set("content-type", "text/event-stream; charset=utf-8")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, unsub := h.Subscribe()
	defer unsub()

	// Initial comment to establish stream.
	fmt.Fprintf(w, ": connected %s\n\n", time.Now().Format(time.RFC3339))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			var b []byte
			// Encode with Encoder to avoid any non-UTF8; Go strings are UTF-8.
			buf := &jsonBuffer{}
			if err := buf.Encode(ev); err != nil {
				return
			}
			b = buf.Bytes()
			fmt.Fprint(w, "event: push\n")
			fmt.Fprint(w, "data: ")
			w.Write(b)
			fmt.Fprint(w, "\n\n")
			flusher.Flush()
		}
	}
}

// jsonBuffer captures json.Encoder output without extra dependencies.
type jsonBuffer struct {
	buf []byte
}

func (b *jsonBuffer) Encode(v any) error {
	b.buf = b.buf[:0]
	w := &sliceWriter{b: &b.buf}
	e := json.NewEncoder(w)
	return e.Encode(v)
}

func (b *jsonBuffer) Bytes() []byte {
	// json.Encoder.Encode adds trailing '\n' - trim to make SSE a single-line data.
	out := b.buf
	for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
		out = out[:len(out)-1]
	}
	return out
}

type sliceWriter struct {
	b *[]byte
}

func (w *sliceWriter) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}
