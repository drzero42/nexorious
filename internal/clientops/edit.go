package clientops

import (
	"context"
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// EditOpts is one game edit. Each *Set flag distinguishes "set to the zero
// value" from "leave unchanged"; Loved/NoLoved set or clear is_loved.
type EditOpts struct {
	Status                  string
	StatusSet               bool
	Rating                  int
	RatingSet               bool
	Loved, NoLoved          bool
	Notes                   string
	NotesSet                bool
	AddPlatform, RmPlatform string
	Hours                   float64
	HoursSet                bool
	HoursPlatform           string
	AddTags, RmTags         []string
}

// EditOne applies o to u as an ordered series of REST calls: platform add,
// platform remove, hours, status, scalar fields, then tags.
func EditOne(ctx context.Context, c *cliclient.Client, key string, u *cliclient.UserGame, o EditOpts) error {
	if o.AddPlatform != "" {
		pl, sf := SplitPlatform(o.AddPlatform)
		if err := ValidatePlatform(ctx, c, key, pl); err != nil {
			return err
		}
		if err := c.AddPlatform(ctx, key, u.ID, cliclient.PlatformInput{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}); err != nil {
			return fmt.Errorf("add platform: %w", err)
		}
	}
	if o.RmPlatform != "" {
		pid, err := platformIDBySlug(ctx, c, key, u, o.RmPlatform)
		if err != nil {
			return err
		}
		if err := c.DeletePlatform(ctx, key, u.ID, pid); err != nil {
			return fmt.Errorf("remove platform: %w", err)
		}
	}
	if o.HoursSet {
		pid, err := targetPlatformID(ctx, c, key, u, o.HoursPlatform)
		if err != nil {
			return err
		}
		if err := c.UpdatePlatform(ctx, key, u.ID, pid, map[string]any{"hours_played": o.Hours}); err != nil {
			return fmt.Errorf("set hours: %w", err)
		}
	}
	if o.StatusSet {
		if _, err := c.UpdateProgress(ctx, key, u.ID, o.Status); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
	}
	fields := map[string]any{}
	if o.RatingSet {
		fields["personal_rating"] = o.Rating
	}
	if o.Loved {
		fields["is_loved"] = true
	}
	if o.NoLoved {
		fields["is_loved"] = false
	}
	if o.NotesSet {
		fields["personal_notes"] = o.Notes
	}
	if len(fields) > 0 {
		if _, err := c.UpdateUserGame(ctx, key, u.ID, fields); err != nil {
			return fmt.Errorf("update fields: %w", err)
		}
	}
	if len(o.AddTags) > 0 || len(o.RmTags) > 0 {
		if err := applyTagEdits(ctx, c, key, u, o.AddTags, o.RmTags); err != nil {
			return err
		}
	}
	return nil
}

// platformIDBySlug finds the platform row id whose platform slug matches.
func platformIDBySlug(ctx context.Context, c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(ctx, key, u.ID)
	if err != nil {
		return "", err
	}
	for i := range cur.Platforms {
		if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
			return cur.Platforms[i].ID, nil
		}
	}
	return "", fmt.Errorf("no platform %q on this game", slug)
}

// targetPlatformID picks the platform row for --hours: the named slug, else the
// sole platform, else an error asking which.
func targetPlatformID(ctx context.Context, c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(ctx, key, u.ID)
	if err != nil {
		return "", err
	}
	if slug != "" {
		for i := range cur.Platforms {
			if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
				return cur.Platforms[i].ID, nil
			}
		}
		return "", fmt.Errorf("no platform %q on this game", slug)
	}
	if len(cur.Platforms) == 1 {
		return cur.Platforms[0].ID, nil
	}
	return "", fmt.Errorf("game has %d platforms; specify which with --platform", len(cur.Platforms))
}

// applyTagEdits computes current ∪ add \ remove and replaces the tag set.
func applyTagEdits(ctx context.Context, c *cliclient.Client, key string, u *cliclient.UserGame, add, remove []string) error {
	cur, err := c.GetUserGame(ctx, key, u.ID)
	if err != nil {
		return err
	}
	set := map[string]string{} // lower -> display
	for _, t := range cur.Tags {
		set[strings.ToLower(t.Name)] = t.Name
	}
	for _, t := range add {
		set[strings.ToLower(t)] = t
	}
	for _, t := range remove {
		delete(set, strings.ToLower(t))
	}
	names := make([]string, 0, len(set))
	for _, disp := range set {
		names = append(names, disp)
	}
	if _, err := c.ReplaceTags(ctx, key, u.ID, names); err != nil {
		return fmt.Errorf("update tags: %w", err)
	}
	return nil
}
