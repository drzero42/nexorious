package humble

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// libraryClient is the subset of *Client the adapter needs; declared as an
// interface so tests can supply a fake.
type libraryClient interface {
	ListGamekeys(ctx context.Context, cookie string) ([]string, error)
	GetOrder(ctx context.Context, cookie, gamekey string) (*Order, error)
}

// gamePlatforms is the whitelist of Humble download platforms that qualify a
// subproduct as a game, mapped to canonical platforms.name slugs. Every other
// platform (ebook, audio, video, asmjs, …) is excluded by absence — a whitelist
// handles unknown future non-game platforms safely.
var gamePlatforms = map[string]string{
	"windows": "pc-windows",
	"mac":     "mac",
	"linux":   "pc-linux",
	"android": "android",
}

// launcherBlocklist holds machine_names that pass the platform filter but are
// not games. A scan of a full 138-order test library found uplayclient to be
// the only such launcher. Extend only with confirmed entries.
var launcherBlocklist = map[string]bool{
	"uplayclient": true,
}

// Adapter wraps a Humble client with a session cookie and implements
// storefrontadapter.Adapter.
type Adapter struct {
	client libraryClient
	cookie string
}

// NewAdapter returns a storefrontadapter.Adapter for Humble Bundle.
func NewAdapter(client libraryClient, cookie string) storefrontadapter.Adapter {
	return &Adapter{client: client, cookie: cookie}
}

func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if batchSize <= 0 {
		batchSize = 10
	}

	gamekeys, err := a.client.ListGamekeys(ctx, a.cookie)
	if errors.Is(err, ErrCredentials) {
		return fmt.Errorf("%w: humble session cookie rejected", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return fmt.Errorf("humble: list gamekeys: %w", err)
	}

	batch := make([]storefrontadapter.ExternalGameEntry, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := onBatch(batch); err != nil {
			return err
		}
		// Fresh slice per batch (matching sibling adapters) so the worker can
		// safely retain the slice it was handed.
		batch = make([]storefrontadapter.ExternalGameEntry, 0, batchSize)
		return nil
	}

	for _, gk := range gamekeys {
		order, err := a.client.GetOrder(ctx, a.cookie, gk)
		if errors.Is(err, ErrCredentials) {
			return fmt.Errorf("%w: humble session cookie rejected", storefrontadapter.ErrCredentials)
		}
		if err != nil {
			// A single failing order is logged and skipped so one bad order
			// doesn't sink the whole sync.
			slog.Error("humble: skipping order", "gamekey", gk, "err", err)
			continue
		}
		if order == nil {
			continue
		}
		for i := range order.Subproducts {
			entry, ok := gameEntry(&order.Subproducts[i])
			if !ok {
				continue
			}
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}

// gameEntry applies the filtering rule to a subproduct and returns an
// ExternalGameEntry plus true if it is a DRM-free game, false otherwise. A
// subproduct qualifies iff it is not in the launcher blocklist and has at least
// one download whose platform is in gamePlatforms with a non-empty url.web.
func gameEntry(sp *Subproduct) (storefrontadapter.ExternalGameEntry, bool) {
	if launcherBlocklist[sp.MachineName] {
		return storefrontadapter.ExternalGameEntry{}, false
	}

	var slugs []string
	seen := make(map[string]bool)
	for _, d := range sp.Downloads {
		slug, ok := gamePlatforms[d.Platform]
		if !ok {
			continue
		}
		if len(d.DownloadStruct) == 0 || d.DownloadStruct[0].URL.Web == "" {
			continue
		}
		if !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
	}
	if len(slugs) == 0 {
		return storefrontadapter.ExternalGameEntry{}, false
	}

	return storefrontadapter.ExternalGameEntry{
		ExternalID:      sp.MachineName,
		Title:           sp.HumanName,
		PlaytimeHours:   0,
		Platforms:       slugs,
		OwnershipStatus: "owned",
		IsSubscription:  false,
	}, true
}
