// Package storelink resolves per-storefront product-page identifiers
// (store_link values) used by the enrichment worker. Resolution is best-effort:
// a Resolver returns ("", nil) when no link can be determined.
package storelink

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/drzero42/nexorious/internal/observability"
)

// Resolver resolves a single external game's store_link.
type Resolver interface {
	Resolve(ctx context.Context, externalID string, sourceMeta map[string]string) (string, error)
}

// ── Steam ────────────────────────────────────────────────────────────────────

type steamResolver struct{}

func NewSteamResolver() Resolver { return steamResolver{} }

func (steamResolver) Resolve(_ context.Context, externalID string, _ map[string]string) (string, error) {
	return externalID, nil // store_link == appid == external_id
}

// ── GOG ──────────────────────────────────────────────────────────────────────

type gogResolver struct {
	httpClient *http.Client
	apiBase    string
}

const defaultGOGAPIBase = "https://api.gog.com"

func NewGOGResolver(httpClient *http.Client, apiBase string) Resolver {
	if httpClient == nil {
		httpClient = &http.Client{Transport: observability.HTTPTransport()}
	}
	if apiBase == "" {
		apiBase = defaultGOGAPIBase
	}
	return &gogResolver{httpClient: httpClient, apiBase: apiBase}
}

func (g *gogResolver) Resolve(ctx context.Context, externalID string, _ map[string]string) (string, error) {
	u := fmt.Sprintf("%s/products/%s", g.apiBase, externalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("gog: build product request: %w", err)
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gog: product fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gog: product HTTP %d", resp.StatusCode)
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("gog: product decode: %w", err)
	}
	return body.Slug, nil
}

// ── Epic ─────────────────────────────────────────────────────────────────────

type epicResolver struct {
	httpClient *http.Client
	mappingURL string
	once       sync.Once
	mapping    map[string]string
	mapErr     error
}

const defaultEpicMappingURL = "https://store-content-ipv4.ak.epicgames.com/api/content/productmapping"

func NewEpicResolver(httpClient *http.Client, mappingURL string) Resolver {
	if httpClient == nil {
		httpClient = &http.Client{Transport: observability.HTTPTransport()}
	}
	if mappingURL == "" {
		mappingURL = defaultEpicMappingURL
	}
	return &epicResolver{httpClient: httpClient, mappingURL: mappingURL}
}

func (e *epicResolver) Resolve(ctx context.Context, _ string, sourceMeta map[string]string) (string, error) {
	ns := sourceMeta["namespace"]
	if ns == "" {
		return "", nil
	}
	e.once.Do(func() { e.mapping, e.mapErr = e.fetchMapping(ctx) })
	if e.mapErr != nil {
		return "", e.mapErr
	}
	return e.mapping[ns], nil
}

func (e *epicResolver) fetchMapping(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.mappingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("epic: build productmapping request: %w", err)
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epic: productmapping fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epic: productmapping HTTP %d", resp.StatusCode)
	}
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("epic: productmapping decode: %w", err)
	}
	return m, nil
}

// ── PSN ──────────────────────────────────────────────────────────────────────

type psnConceptResolver struct {
	client  PSNConceptClient
	npsso   string
	once    sync.Once
	token   string
	authErr error
}

// PSNConceptClient is satisfied by *psn.Client.
type PSNConceptClient interface {
	Authenticate(ctx context.Context, npsso string) (string, error)
	ResolveConceptID(ctx context.Context, accessToken, titleID string) (string, error)
}

func NewPSNResolver(client PSNConceptClient, npsso string) Resolver {
	return &psnConceptResolver{client: client, npsso: npsso}
}

func (p *psnConceptResolver) Resolve(ctx context.Context, externalID string, _ map[string]string) (string, error) {
	p.once.Do(func() { p.token, p.authErr = p.client.Authenticate(ctx, p.npsso) })
	if p.authErr != nil {
		return "", p.authErr
	}
	return p.client.ResolveConceptID(ctx, p.token, externalID)
}
