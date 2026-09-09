package query

import (
	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
)

// RelationOpts configures how a relation is joined in a SelectQuery.
type RelationOpts struct {
	// Apply applies additional options to the relation.
	Apply func(*SelectQuery) *SelectQuery
	// AdditionalJoinOnConditions adds additional conditions to the JOIN ON clause.
	AdditionalJoinOnConditions []schema.QueryWithArgs
	// LoadWithChildren explicitly configures whether child relations are loaded or not.
	LoadWithChildren bool
}

// joinQuery represents an explicit JOIN clause.
type joinQuery struct {
	join schema.QueryWithArgs
	on   []schema.QueryWithSep
}

func (j *joinQuery) AppendQuery(gen schema.QueryGen, b []byte) (_ []byte, err error) {
	b = append(b, ' ')

	b, err = j.join.AppendQuery(gen, b)
	if err != nil {
		return nil, err
	}

	if len(j.on) > 0 {
		b = append(b, " ON "...)
		for i, on := range j.on {
			if i > 0 {
				b = append(b, on.Sep...)
			}

			b = append(b, '(')
			b, err = on.AppendQuery(gen, b)
			if err != nil {
				return nil, err
			}
			b = append(b, ')')
		}
	}

	return b, nil
}

// countQuery wraps SelectQuery to generate a COUNT query.
type countQuery struct {
	*SelectQuery
}

func (q countQuery) AppendQuery(gen schema.QueryGen, b []byte) (_ []byte, err error) {
	if q.err != nil {
		return nil, q.err
	}
	return q.appendQuery(gen, b, true)
}

// selectExistsQuery wraps SelectQuery to generate a SELECT EXISTS (...) query.
type selectExistsQuery struct {
	*SelectQuery
}

func (q selectExistsQuery) AppendQuery(gen schema.QueryGen, b []byte) (_ []byte, err error) {
	if q.err != nil {
		return nil, q.err
	}

	b = append(b, "SELECT EXISTS ("...)

	b, err = q.appendQuery(gen, b, false)
	if err != nil {
		return nil, err
	}

	b = append(b, ')')

	return b, nil
}

// whereExistsQuery wraps SelectQuery to generate a SELECT 1 WHERE EXISTS (...) query.
type whereExistsQuery struct {
	*SelectQuery
}

func (q whereExistsQuery) AppendQuery(gen schema.QueryGen, b []byte) (_ []byte, err error) {
	if q.err != nil {
		return nil, q.err
	}

	b = append(b, "SELECT 1 WHERE EXISTS ("...)

	b, err = q.appendQuery(gen, b, false)
	if err != nil {
		return nil, err
	}

	b = append(b, ')')

	return b, nil
}

// selectQueryBuilder provides a fluent QueryBuilder interface over SelectQuery.
type selectQueryBuilder struct {
	*SelectQuery
}

func (q *selectQueryBuilder) WhereGroup(
	sep string, fn func(QueryBuilder) QueryBuilder,
) QueryBuilder {
	q.SelectQuery = q.SelectQuery.WhereGroup(sep, func(qs *SelectQuery) *SelectQuery {
		return fn(q).(*selectQueryBuilder).SelectQuery
	})
	return q
}

func (q *selectQueryBuilder) Where(query string, args ...any) QueryBuilder {
	q.SelectQuery.Where(query, args...)
	return q
}

func (q *selectQueryBuilder) WhereOr(query string, args ...any) QueryBuilder {
	q.SelectQuery.WhereOr(query, args...)
	return q
}

func (q *selectQueryBuilder) WhereDeleted() QueryBuilder {
	q.SelectQuery.WhereDeleted()
	return q
}

func (q *selectQueryBuilder) WhereAllWithDeleted() QueryBuilder {
	q.SelectQuery.WhereAllWithDeleted()
	return q
}

func (q *selectQueryBuilder) WherePK(cols ...string) QueryBuilder {
	q.SelectQuery.WherePK(cols...)
	return q
}

func (q *selectQueryBuilder) Unwrap() any {
	return q.SelectQuery
}
