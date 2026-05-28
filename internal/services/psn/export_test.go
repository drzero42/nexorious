package psn

import (
	"context"
	"net/http"

	"golang.org/x/time/rate"
)

func (c *Client) SetHTTPClient(h *http.Client)                                         { c.httpClient = h }
func (c *Client) SetGamelistURL(url string)                                            { c.gamelistURL = url }
func (c *Client) SetGraphQLURL(url string)                                             { c.graphqlURL = url }
func (c *Client) SetGraphQLPageSize(n int)                                             { c.graphqlPageSize = n }
func (c *Client) SetAuthFn(fn func(ctx context.Context, token string) (string, error)) { c.authFn = fn }
func (c *Client) SetLimiter(l *rate.Limiter)                                           { c.limiter = l }
