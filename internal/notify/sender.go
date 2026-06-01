package notify

import (
	"context"
	"fmt"
	"sync"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/types"
)

// Sender delivers a single rendered notification to one channel URL.
// Implementations must not panic on malformed URLs; return an error instead.
type Sender interface {
	Send(ctx context.Context, url, title, body string) error
}

// ShoutrrrSender is the production Sender backed by github.com/containrrr/shoutrrr.
type ShoutrrrSender struct{}

// NewShoutrrrSender constructs a ShoutrrrSender.
func NewShoutrrrSender() *ShoutrrrSender { return &ShoutrrrSender{} }

// Send delivers body (with title param) to the given Shoutrrr URL.
// ctx is accepted for interface compatibility; shoutrrr v0.8.0 has no context support.
func (s *ShoutrrrSender) Send(_ context.Context, url, title, body string) error {
	sender, err := shoutrrr.CreateSender(url)
	if err != nil {
		return fmt.Errorf("notify: create sender: %w", err)
	}
	params := types.Params{}
	params.SetTitle(title)
	for _, e := range sender.Send(body, &params) {
		if e != nil {
			return fmt.Errorf("notify: send: %w", e)
		}
	}
	return nil
}

// SentMessage is one recorded delivery (test impl).
type SentMessage struct {
	URL   string
	Title string
	Body  string
}

// RecorderSender records sends instead of delivering them (test impl).
type RecorderSender struct {
	mu   sync.Mutex
	sent []SentMessage
	// Err, if set, is returned by every Send (to exercise failure paths).
	Err error
}

// NewRecorderSender constructs a RecorderSender.
func NewRecorderSender() *RecorderSender { return &RecorderSender{} }

// Send records the message and returns r.Err.
func (r *RecorderSender) Send(_ context.Context, url, title, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, SentMessage{URL: url, Title: title, Body: body})
	return r.Err
}

// Sent returns a copy of recorded messages.
func (r *RecorderSender) Sent() []SentMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SentMessage, len(r.sent))
	copy(out, r.sent)
	return out
}
