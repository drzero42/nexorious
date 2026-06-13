package filter

import (
	"bytes"
	"encoding/json"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/dbutil"
)

// PoolFilter is a pool's saved filter: an ordered list of faceted cards
// evaluated as OR — a game matches the pool if it matches ANY card (#955).
type PoolFilter struct {
	Filters []FilterCard `json:"filters"`
}

// FilterCard mirrors the library list params. Each facet ANDs with the others
// within a card; multiple values within a single facet OR together.
type FilterCard struct {
	PlayStatus        PlayStatusFilter `json:"play_status,omitempty"`
	Genre             []string         `json:"genre,omitempty"`
	Theme             []string         `json:"theme,omitempty"`
	Tag               []string         `json:"tag,omitempty"`
	Platform          []string         `json:"platform,omitempty"`
	Storefront        []string         `json:"storefront,omitempty"`
	RatingMin         *float64         `json:"rating_min,omitempty"`
	RatingMax         *float64         `json:"rating_max,omitempty"`
	IsLoved           *bool            `json:"is_loved,omitempty"`
	GameMode          []string         `json:"game_mode,omitempty"`
	PlayerPerspective []string         `json:"player_perspective,omitempty"`
	Q                 *string          `json:"q,omitempty"`
	TimeToBeatMin     *float64         `json:"time_to_beat_min,omitempty"`
	TimeToBeatMax     *float64         `json:"time_to_beat_max,omitempty"`
}

// PlayStatusFilter holds the play_status facet's selected values. Play status
// is multi-value (OR-within-facet) like the other enum facets, but filters
// saved before #976 persisted a single JSON string. UnmarshalJSON accepts both
// shapes — a bare string normalises to a one-element slice — so legacy stored
// filters parse without a migration. It always marshals as a JSON array.
type PlayStatusFilter []string

func (p *PlayStatusFilter) UnmarshalJSON(data []byte) error {
	// Current shape: a JSON array of strings.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*p = arr
		return nil
	}
	// Legacy shape: a single JSON string.
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*p = []string{s}
	return nil
}

// ParsePoolFilter unmarshals a saved filter with unknown keys rejected
// ("typed in Go, JSONB at rest"). DisallowUnknownFields applies recursively to
// the nested cards too.
func ParsePoolFilter(raw []byte) (PoolFilter, error) {
	var pf PoolFilter
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pf); err != nil {
		return PoolFilter{}, err
	}
	return pf, nil
}

// HasFacets reports whether the card constrains at least one facet. An empty
// card (no facets) is rejected at pool create/update time.
func (c FilterCard) HasFacets() bool {
	return len(c.PlayStatus) > 0 ||
		len(c.Genre) > 0 ||
		len(c.Theme) > 0 ||
		len(c.Tag) > 0 ||
		len(c.Platform) > 0 ||
		len(c.Storefront) > 0 ||
		c.RatingMin != nil ||
		c.RatingMax != nil ||
		c.IsLoved != nil ||
		len(c.GameMode) > 0 ||
		len(c.PlayerPerspective) > 0 ||
		(c.Q != nil && *c.Q != "") ||
		c.TimeToBeatMin != nil ||
		c.TimeToBeatMax != nil
}

func (c FilterCard) needsGamesJoin() bool {
	return len(c.Genre) > 0 || len(c.Theme) > 0 || len(c.GameMode) > 0 ||
		len(c.PlayerPerspective) > 0 || (c.Q != nil && *c.Q != "") ||
		c.TimeToBeatMin != nil || c.TimeToBeatMax != nil
}

func (c FilterCard) needsPlatformsJoin() bool {
	return len(c.Platform) > 0 || len(c.Storefront) > 0
}

// ApplyPoolFilter applies the OR-of-cards predicate: (card1) OR (card2) OR …,
// where each card is the AND of its facet predicates. Required JOINs are
// registered once. The global finished-status exclusion is applied by the
// caller, OUTSIDE this function — it is never stored in a card.
func ApplyPoolFilter(fb *FilterBuilder, pf PoolFilter) {
	if len(pf.Filters) == 0 {
		return
	}
	for _, c := range pf.Filters {
		if c.needsGamesJoin() {
			fb.AddJoin("g", joinGames)
		}
		if c.needsPlatformsJoin() {
			fb.AddJoin("ugp", joinUserGamePlatforms)
		}
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, c := range pf.Filters {
				q = q.WhereGroup(" OR ", func(q *bun.SelectQuery) *bun.SelectQuery {
					return applyCardPredicates(q, c)
				})
			}
			return q
		})
	})
}

// applyCardPredicates ANDs one card's facet predicates onto q. Multi-value
// facets OR their values inside an AND-attached group, mirroring the standalone
// Apply* helpers in criteria.go.
func applyCardPredicates(q *bun.SelectQuery, c FilterCard) *bun.SelectQuery {
	q = orIn(q, "ug.play_status", c.PlayStatus)
	if c.IsLoved != nil {
		q = q.Where("ug.is_loved = ?", *c.IsLoved)
	}
	if c.RatingMin != nil {
		q = q.Where("ug.personal_rating >= ?", *c.RatingMin)
	}
	if c.RatingMax != nil {
		q = q.Where("ug.personal_rating <= ?", *c.RatingMax)
	}
	if c.TimeToBeatMin != nil {
		q = q.Where("g.howlongtobeat_main >= ?", *c.TimeToBeatMin)
	}
	if c.TimeToBeatMax != nil {
		q = q.Where("g.howlongtobeat_main <= ?", *c.TimeToBeatMax)
	}
	q = orILike(q, "g.genre", c.Genre)
	q = orILike(q, "g.themes", c.Theme)
	q = orILike(q, "g.game_modes", c.GameMode)
	q = orILike(q, "g.player_perspectives", c.PlayerPerspective)
	if c.Q != nil && *c.Q != "" {
		pattern := dbutil.LikeContains(*c.Q)
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereOr("g.title ILIKE ?", pattern)
			q = q.WhereOr("ug.personal_notes IS NOT NULL AND ug.personal_notes ILIKE ?", pattern)
			return q
		})
	}
	q = orIn(q, "ugp.platform", c.Platform)
	q = orIn(q, "ugp.storefront", c.Storefront)
	if len(c.Tag) > 0 {
		q = q.Where("ug.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (?))", bun.List(c.Tag))
	}
	return q
}

// orILike ANDs an (col ILIKE v1 OR col ILIKE v2 …) group onto q.
func orILike(q *bun.SelectQuery, col string, values []string) *bun.SelectQuery {
	if len(values) == 0 {
		return q
	}
	return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		for _, v := range values {
			q = q.WhereOr(col+" ILIKE ?", dbutil.LikeContains(v))
		}
		return q
	})
}

// orIn ANDs a (col IN (values)) group onto q.
func orIn(q *bun.SelectQuery, col string, values []string) *bun.SelectQuery {
	if len(values) == 0 {
		return q
	}
	return q.Where(col+" IN (?)", bun.List(values))
}
