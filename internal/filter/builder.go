package filter

import "github.com/uptrace/bun"

// filterBuilder accumulates JOINs, WHERE, and HAVING clauses for dynamic queries.
type filterBuilder struct {
	joins   map[string]string // alias → "LEFT JOIN table AS alias ON ..."
	wheres  []func(*bun.SelectQuery) *bun.SelectQuery
	havings []func(*bun.SelectQuery) *bun.SelectQuery
}

// NewFilterBuilder creates an empty filterBuilder.
func NewFilterBuilder() *filterBuilder {
	return &filterBuilder{joins: make(map[string]string)}
}

// AddJoin registers a JOIN clause, deduplicated by alias.
func (f *filterBuilder) AddJoin(alias, clause string) {
	f.joins[alias] = clause
}

// AddWhere appends a WHERE clause function.
func (f *filterBuilder) AddWhere(fn func(*bun.SelectQuery) *bun.SelectQuery) {
	f.wheres = append(f.wheres, fn)
}

// AddHaving appends a HAVING clause function.
func (f *filterBuilder) AddHaving(fn func(*bun.SelectQuery) *bun.SelectQuery) {
	f.havings = append(f.havings, fn)
}

// Apply applies all accumulated JOINs, WHEREs, and HAVINGs to the query.
func (f *filterBuilder) Apply(q *bun.SelectQuery) *bun.SelectQuery {
	for _, clause := range f.joins {
		q = q.Join(clause)
	}
	for _, fn := range f.wheres {
		q = fn(q)
	}
	for _, fn := range f.havings {
		q = fn(q)
	}
	return q
}
