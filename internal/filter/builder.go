package filter

import "github.com/uptrace/bun"

// FilterBuilder accumulates JOINs and WHERE clauses for dynamic queries.
type FilterBuilder struct {
	joins  map[string]string // alias → "LEFT JOIN table AS alias ON ..."
	wheres []func(*bun.SelectQuery) *bun.SelectQuery
}

// NewFilterBuilder creates an empty FilterBuilder.
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{joins: make(map[string]string)}
}

// AddJoin registers a JOIN clause, deduplicated by alias.
func (f *FilterBuilder) AddJoin(alias, clause string) {
	f.joins[alias] = clause
}

// AddWhere appends a WHERE clause function.
func (f *FilterBuilder) AddWhere(fn func(*bun.SelectQuery) *bun.SelectQuery) {
	f.wheres = append(f.wheres, fn)
}

// Apply applies all accumulated JOINs and WHEREs to the query.
func (f *FilterBuilder) Apply(q *bun.SelectQuery) *bun.SelectQuery {
	for _, clause := range f.joins {
		q = q.Join(clause)
	}
	for _, fn := range f.wheres {
		q = fn(q)
	}
	return q
}
