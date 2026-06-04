package humble

import (
	"net/http"

	"golang.org/x/time/rate"
)

// Test-only setters, mirroring internal/services/psn/export_test.go.
func (c *Client) SetHTTPClient(h *http.Client) { c.httpClient = h }
func (c *Client) SetBaseURL(u string)          { c.baseURL = u }
func (c *Client) SetLimiter(l *rate.Limiter)   { c.limiter = l }
