package gog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// ExternalLibraryEntry is a normalised game entry from GOG.
// PlaytimeHours is always 0 — the GOG library API has no playtime field.
type ExternalLibraryEntry struct {
	ExternalID      string
	Title           string
	RawPlatform     string // "pc-windows", "pc-mac", or "pc-linux"
	PlaytimeHours   int
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
// account/getFilteredProducts. For each game available on both Windows and
// Linux, two entries are emitted with the same ExternalID but different
// RawPlatform values. onBatch is called once per page.
func (c *Client) GetLibrary(ctx context.Context, accessToken string, _ int, onBatch func([]ExternalLibraryEntry) error) error {
	for page := 1; ; page++ {
		entries, numPages, err := c.fetchPage(ctx, accessToken, page)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			if err := onBatch(entries); err != nil {
				return err
			}
		}
		if page >= numPages {
			break
		}
	}
	return nil
}

func (c *Client) fetchPage(ctx context.Context, accessToken string, page int) ([]ExternalLibraryEntry, int, error) {
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

	numPages := max(body.NumPages, 1)

	entries := make([]ExternalLibraryEntry, 0, len(body.Products)*2)
	for _, p := range body.Products {
		id := strconv.FormatInt(p.ID, 10)
		if p.WorksOn.Windows {
			entries = append(entries, ExternalLibraryEntry{
				ExternalID:      id,
				Title:           p.Title,
				RawPlatform:     "pc-windows",
				PlaytimeHours:   0,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
		}
		if p.WorksOn.Mac {
			entries = append(entries, ExternalLibraryEntry{
				ExternalID:      id,
				Title:           p.Title,
				RawPlatform:     "pc-mac",
				PlaytimeHours:   0,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
		}
		if p.WorksOn.Linux {
			entries = append(entries, ExternalLibraryEntry{
				ExternalID:      id,
				Title:           p.Title,
				RawPlatform:     "pc-linux",
				PlaytimeHours:   0,
				OwnershipStatus: "owned",
				IsSubscription:  false,
			})
		}
	}
	return entries, numPages, nil
}
