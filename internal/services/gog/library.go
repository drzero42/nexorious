package gog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
)

// ExternalGameEntry is a normalised game entry from GOG.
// PlaytimeHours is always 0 — the GOG library API has no playtime field.
type ExternalGameEntry struct {
	ExternalID      string
	Title           string
	Platforms       []string // all platforms this product runs on
	PlaytimeHours   float64
	OwnershipStatus string
	IsSubscription  bool
}

type filteredProductsResponse struct {
	TotalProducts   int       `json:"totalProducts"`
	NumPages        int       `json:"numPages"`
	ProductsPerPage int       `json:"productsPerPage"`
	Page            int       `json:"page"`
	Products        []product `json:"products"`
}

type product struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	WorksOn struct {
		Windows bool `json:"Windows"`
		Mac     bool `json:"Mac"`
		Linux   bool `json:"Linux"`
	} `json:"worksOn"`
}

// GetLibrary fetches the user's complete GOG library by paging
// account/getFilteredProducts. Each product is emitted as a single entry whose
// Platforms slice holds all supported platforms. onBatch is called once per page.
func (c *Client) GetLibrary(ctx context.Context, accessToken string, _ int, onBatch func([]ExternalGameEntry) error) error {
	totalFetched := 0
	for page := 1; ; page++ {
		entries, numPages, err := c.fetchPage(ctx, accessToken, page)
		if err != nil {
			return err
		}
		slog.DebugContext(ctx, "gog: fetched page", "page", page, "numPages", numPages, "entriesOnPage", len(entries))
		totalFetched += len(entries)
		if len(entries) > 0 {
			if err := onBatch(entries); err != nil {
				return err
			}
		}
		if page >= numPages {
			slog.DebugContext(ctx, "gog: library fetch complete", "totalFetched", totalFetched, "lastPage", page, "numPages", numPages)
			break
		}
	}
	return nil
}

func (c *Client) fetchPage(ctx context.Context, accessToken string, page int) ([]ExternalGameEntry, int, error) {
	url := fmt.Sprintf("%s/account/getFilteredProducts?mediaType=1&page=%d", c.embedBase, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("gog: build library request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("gog: library request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, 0, ErrGOGAuthExpired
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("gog: library HTTP %d", resp.StatusCode)
	}

	var body filteredProductsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, fmt.Errorf("gog: decode library response: %w", err)
	}

	slog.DebugContext(ctx, "gog: page response metadata",
		"requestedPage", page,
		"responsePage", body.Page,
		"totalProducts", body.TotalProducts,
		"numPages", body.NumPages,
		"productsPerPage", body.ProductsPerPage,
		"productsInResponse", len(body.Products),
	)

	// GOG sometimes returns numPages: 0 even when totalProducts > productsPerPage.
	// Fall back to ceiling division so all pages are fetched.
	numPages := body.NumPages
	if numPages == 0 && body.ProductsPerPage > 0 {
		numPages = (body.TotalProducts + body.ProductsPerPage - 1) / body.ProductsPerPage
	}
	numPages = max(numPages, 1)

	entries := make([]ExternalGameEntry, 0, len(body.Products))
	for _, p := range body.Products {
		id := strconv.FormatInt(p.ID, 10)
		var platforms []string
		if p.WorksOn.Windows {
			platforms = append(platforms, "pc-windows")
		}
		if p.WorksOn.Mac {
			platforms = append(platforms, "mac")
		}
		if p.WorksOn.Linux {
			platforms = append(platforms, "pc-linux")
		}
		if len(platforms) == 0 {
			platforms = []string{"pc-windows"}
		}
		entries = append(entries, ExternalGameEntry{
			ExternalID:      id,
			Title:           p.Title,
			Platforms:       platforms,
			PlaytimeHours:   0,
			OwnershipStatus: "owned",
			IsSubscription:  false,
		})
	}
	return entries, numPages, nil
}
