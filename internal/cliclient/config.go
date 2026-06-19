package cliclient

import (
	"net/http"
	"net/url"
)

// Settings holds the user's application settings as returned by
// GET /api/settings and PATCH /api/settings.
type Settings struct {
	DealRegion string `json:"deal_region"`
}

// GetSettings returns the current user settings via GET /api/settings.
func (c *Client) GetSettings(key string) (*Settings, error) {
	var out Settings
	if err := c.doBearer(http.MethodGet, "/api/settings", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSettings sets the deal_region field via PATCH /api/settings and
// returns the updated settings.
func (c *Client) UpdateSettings(key, dealRegion string) (*Settings, error) {
	body := map[string]any{"deal_region": dealRegion}
	var out Settings
	if err := c.doBearer(http.MethodPatch, "/api/settings", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NotifyChannel is one notification channel as returned by the
// /api/notifications/channels endpoints. The secret URL is never included in
// responses.
type NotifyChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// EventType is one notification event type as returned by
// GET /api/notifications/event-types.
type EventType struct {
	Type      string `json:"type"`
	Scope     string `json:"scope"`
	Category  string `json:"category"`
	Label     string `json:"label"`
	DefaultOn bool   `json:"default_on"`
}

// subscriptionsEnvelope is the wire envelope for GET/PUT /api/notifications/subscriptions
// and POST /api/notifications/subscriptions/reset.
type subscriptionsEnvelope struct {
	EventTypes []string `json:"event_types"`
}

// ListChannels returns the caller's notification channels via
// GET /api/notifications/channels.
func (c *Client) ListChannels(key string) ([]NotifyChannel, error) {
	var out []NotifyChannel
	if err := c.doBearer(http.MethodGet, "/api/notifications/channels", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateChannel creates a notification channel with the given name and
// webhook URL via POST /api/notifications/channels.
func (c *Client) CreateChannel(key, name, channelURL string) (*NotifyChannel, error) {
	body := map[string]any{"name": name, "url": channelURL}
	var out NotifyChannel
	if err := c.doBearer(http.MethodPost, "/api/notifications/channels", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateChannel applies a partial update to a notification channel via
// PATCH /api/notifications/channels/:id. Only keys present in fields are sent;
// valid keys are name and url.
func (c *Client) UpdateChannel(key, id string, fields map[string]any) (*NotifyChannel, error) {
	var out NotifyChannel
	if err := c.doBearer(http.MethodPatch, "/api/notifications/channels/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteChannel removes a notification channel via
// DELETE /api/notifications/channels/:id (expects 204).
func (c *Client) DeleteChannel(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/notifications/channels/"+url.PathEscape(id), key, nil, nil)
}

// TestChannel sends a test notification through the channel with the given id
// via POST /api/notifications/channels/:id/test (expects 204).
func (c *Client) TestChannel(key, id string) error {
	return c.doBearer(http.MethodPost, "/api/notifications/channels/"+url.PathEscape(id)+"/test", key, nil, nil)
}

// TestURL sends a test notification to an unsaved webhook URL via
// POST /api/notifications/test (expects 204).
func (c *Client) TestURL(key, rawURL string) error {
	body := map[string]any{"url": rawURL}
	return c.doBearer(http.MethodPost, "/api/notifications/test", key, body, nil)
}

// ListEventTypes returns all available notification event types via
// GET /api/notifications/event-types.
func (c *Client) ListEventTypes(key string) ([]EventType, error) {
	var out []EventType
	if err := c.doBearer(http.MethodGet, "/api/notifications/event-types", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSubscriptions returns the caller's active notification subscriptions
// (a slice of event-type strings) via GET /api/notifications/subscriptions.
// The {"event_types":[...]} envelope is unwrapped before returning.
func (c *Client) ListSubscriptions(key string) ([]string, error) {
	var env subscriptionsEnvelope
	if err := c.doBearer(http.MethodGet, "/api/notifications/subscriptions", key, nil, &env); err != nil {
		return nil, err
	}
	return env.EventTypes, nil
}

// PutSubscriptions replaces the caller's notification subscriptions with the
// given event-type strings via PUT /api/notifications/subscriptions. The
// {"event_types":[...]} envelope is unwrapped before returning.
func (c *Client) PutSubscriptions(key string, eventTypes []string) ([]string, error) {
	body := subscriptionsEnvelope{EventTypes: eventTypes}
	var env subscriptionsEnvelope
	if err := c.doBearer(http.MethodPut, "/api/notifications/subscriptions", key, body, &env); err != nil {
		return nil, err
	}
	return env.EventTypes, nil
}

// ResetSubscriptions resets the caller's notification subscriptions to their
// defaults via POST /api/notifications/subscriptions/reset and returns the
// resulting set of event types. The {"event_types":[...]} envelope is unwrapped
// before returning.
func (c *Client) ResetSubscriptions(key string) ([]string, error) {
	var env subscriptionsEnvelope
	if err := c.doBearer(http.MethodPost, "/api/notifications/subscriptions/reset", key, nil, &env); err != nil {
		return nil, err
	}
	return env.EventTypes, nil
}
