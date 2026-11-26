package clientops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// ResolveStorefront validates arg against the configured storefronts by fetching
// the sync config list. Matching is case-insensitive. On success the canonical
// Storefront string is returned; on failure a descriptive error listing valid
// options is returned.
func ResolveStorefront(ctx context.Context, c *cliclient.Client, key, arg string) (string, error) {
	configs, err := c.ListSyncConfigs(ctx, key)
	if err != nil {
		return "", fmt.Errorf("list sync configs failed: %w", err)
	}
	if len(configs) == 0 {
		return "", fmt.Errorf("no storefronts available on this server")
	}
	for _, cfg := range configs {
		if strings.EqualFold(cfg.Storefront, arg) {
			return cfg.Storefront, nil
		}
	}
	slugs := make([]string, len(configs))
	for i, cfg := range configs {
		slugs[i] = cfg.Storefront
	}
	return "", fmt.Errorf("unknown storefront %q; valid: %s", arg, strings.Join(slugs, ", "))
}

// ExternalGameMatch is one entry of a storefront's External game list that a
// ref matched. Edition is the edition a UUID ref named (the entry's own id names
// its representative edition); a title ref names no edition, so it is nil.
type ExternalGameMatch struct {
	Game    cliclient.ExternalGame
	Edition *cliclient.ExternalGameEdition
}

// FindExternalGamesByRef returns every entry matching ref from the given
// storefront: a UUID is matched exactly against the entries' ids and the ids of
// their editions (one element or error); a title is matched case-insensitively
// (zero or more). No cmd, no picker — callers own disambiguation
// (ExternalDecisionTarget).
func FindExternalGamesByRef(ctx context.Context, c *cliclient.Client, key, sf, ref string) ([]ExternalGameMatch, error) {
	games, err := c.ListExternalGames(ctx, key, sf)
	if err != nil {
		return nil, err
	}
	if LooksLikeUUID(ref) {
		for i := range games {
			for j := range games[i].Editions {
				if games[i].Editions[j].ID == ref {
					return []ExternalGameMatch{{Game: games[i], Edition: &games[i].Editions[j]}}, nil
				}
			}
			if games[i].ID == ref { // a server that lists no editions
				return []ExternalGameMatch{{Game: games[i]}}, nil
			}
		}
		return nil, fmt.Errorf("no external game with id %s", ref)
	}
	var matches []ExternalGameMatch
	for i := range games {
		if strings.EqualFold(games[i].Title, ref) {
			matches = append(matches, ExternalGameMatch{Game: games[i]})
		}
	}
	return matches, nil
}

// ErrNoEditionsListed: the server lists no editions, so it predates decisions
// on one edition alone and would ignore the option, acting on the whole set.
var ErrNoEditionsListed = errors.New("the server does not support deciding one edition alone; upgrade it, or decide the whole set without the option")

// ExternalDecisionTarget picks the External game id a Rematch or Skip of the
// matches is sent to, or reports that the user must choose. Without
// onlyThisEdition one entry is the target and the id goes as the user gave it
// (any edition's id addresses the set); several entries are ambiguous, the
// choice among entries. With onlyThisEdition the id is always an edition's: the
// one a UUID named, or the only edition of the only entry a title matched;
// otherwise the choice is among all editions of all matching entries.
// No matches is neither: callers report it.
func ExternalDecisionTarget(matches []ExternalGameMatch, onlyThisEdition bool) (id string, ambiguous bool, err error) {
	if len(matches) == 0 {
		return "", false, nil
	}
	if onlyThisEdition {
		for i := range matches {
			if len(matches[i].Game.Editions) == 0 {
				return "", false, ErrNoEditionsListed
			}
		}
		if len(matches) == 1 {
			m := matches[0]
			if m.Edition != nil {
				return m.Edition.ID, false, nil
			}
			if len(m.Game.Editions) == 1 {
				return m.Game.Editions[0].ID, false, nil
			}
		}
		return "", true, nil
	}
	if len(matches) > 1 {
		return "", true, nil
	}
	if m := matches[0]; m.Edition != nil {
		return m.Edition.ID, false, nil
	}
	return matches[0].Game.ID, false, nil
}

// ErrNoExternalGame: a title ref matched no entry of the storefront's list.
var ErrNoExternalGame = errors.New("no external game")

// ExternalTarget is what a Rematch or Skip is sent to: the External game id,
// with the entry's title for prompts and messages.
type ExternalTarget struct {
	ID    string
	Title string
}

// ExternalCandidate is one choice of an ambiguous ref: an entry, or with
// onlyThisEdition one edition of an entry. ID, ExternalID, Platforms and
// StoreURL are the edition's when it is one; Game is its entry.
type ExternalCandidate struct {
	ID         string
	ExternalID string
	Platforms  []string
	StoreURL   *string
	Game       cliclient.ExternalGame
}

// ResolveExternalDecision resolves ref within a storefront to the target of a
// Rematch or Skip (ExternalDecisionTarget). When the ref is ambiguous it
// returns the candidates instead, for the caller to let the user choose: with
// onlyThisEdition every edition of every matching entry, else the entries. A
// title matching nothing is ErrNoExternalGame.
func ResolveExternalDecision(ctx context.Context, c *cliclient.Client, key, sf, ref string, onlyThisEdition bool) (ExternalTarget, []ExternalCandidate, error) {
	matches, err := FindExternalGamesByRef(ctx, c, key, sf, ref)
	if err != nil {
		return ExternalTarget{}, nil, err
	}
	if len(matches) == 0 {
		return ExternalTarget{}, nil, fmt.Errorf("%w matching %q", ErrNoExternalGame, ref)
	}
	id, ambiguous, err := ExternalDecisionTarget(matches, onlyThisEdition)
	if err != nil {
		return ExternalTarget{}, nil, err
	}
	if !ambiguous {
		return ExternalTarget{ID: id, Title: matches[0].Game.Title}, nil, nil
	}
	var cands []ExternalCandidate
	for _, m := range matches {
		if !onlyThisEdition {
			cands = append(cands, ExternalCandidate{ID: m.Game.ID, ExternalID: m.Game.ExternalID, Platforms: m.Game.Platforms, StoreURL: m.Game.StoreURL, Game: m.Game})
			continue
		}
		for _, e := range m.Game.Editions {
			cands = append(cands, ExternalCandidate{ID: e.ID, ExternalID: e.ExternalID, Platforms: e.Platforms, StoreURL: e.StoreURL, Game: m.Game})
		}
	}
	return ExternalTarget{}, cands, nil
}
