// Package query provides fluent SQL query builders, connection handling, statement
// compilation, and lifecycle hook definitions for the RDBMS abstraction layer.
//
// File: helpers.go
// Usage:
//
//	Internal query wrapping types (joinQuery, countQuery, selectExistsQuery, whereExistsQuery,
//	selectQueryBuilder) that decorate SelectQuery for COUNT, EXISTS, subquery, and nested builder operations.
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

// AppendQuery renders the explicit JOIN and its ON clauses into the buffer.
//
// Purpose:
//
//	Formats custom JOIN expressions alongside multiple ON condition predicates.
//
// Where it is used:
//   - In SelectQuery.AppendQuery when serializing explicit table joins.
//
// When can it be used:
//   - When appending manually configured JOIN clauses to a SQL buffer.
//
// Parameters:
//   - gen (schema.QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//
// Returns:
//   - _ ([]byte): Input value used by the operation.
//   - err (error): Input value used by the operation.
//
// Business Rules:
//   - Follows the invariants documented by AppendQuery and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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

// AppendQuery formats the query as a COUNT(*) expression.
//
// Purpose:
//
//	Renders a SELECT count(*) query preserving WHERE and JOIN filters while omitting ORDER BY.
//
// Where it is used:
//   - In SelectQuery.Count execution.
//
// When can it be used:
//   - When executing total row count queries.
//
// Parameters:
//   - gen (schema.QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//
// Returns:
//   - _ ([]byte): Input value used by the operation.
//   - err (error): Input value used by the operation.
//
// Business Rules:
//   - Follows the invariants documented by AppendQuery and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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

// AppendQuery renders SELECT EXISTS (...) for dialects supporting the feature.
//
// Purpose:
//
//	Renders a boolean existence probe query.
//
// Where it is used:
//   - In SelectQuery.Exists when the dialect supports feature.SelectExists.
//
// When can it be used:
//   - When checking for record existence efficiently.
//
// Parameters:
//   - gen (schema.QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//
// Returns:
//   - _ ([]byte): Input value used by the operation.
//   - err (error): Input value used by the operation.
//
// Business Rules:
//   - Follows the invariants documented by AppendQuery and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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

// AppendQuery renders SELECT 1 WHERE EXISTS (...) for dialects without native SELECT EXISTS.
//
// Purpose:
//
//	Renders an existence fallback query for databases without direct SELECT EXISTS support.
//
// Where it is used:
//   - In SelectQuery.Exists as fallback syntax.
//
// When can it be used:
//   - When checking existence on older SQL dialects.
//
// Parameters:
//   - gen (schema.QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//
// Returns:
//   - _ ([]byte): Input value used by the operation.
//   - err (error): Input value used by the operation.
//
// Business Rules:
//   - Follows the invariants documented by AppendQuery and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
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

// WhereGroup groups nested conditions inside parenthesis joined by separator.
//
// Purpose:
//
//	Creates a nested condition block (e.g. AND (a = 1 OR b = 2)) in the query builder.
//
// Where it is used:
//   - In complex filter combinations in SelectQuery.
//
// When can it be used:
//   - When building compound boolean conditions.
//
// Parameters:
//   - sep (string): Input value used by the operation.
//   - fn (func(QueryBuilder) QueryBuilder): Input value used by the operation.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by WhereGroup and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) WhereGroup(
	sep string, fn func(QueryBuilder) QueryBuilder,
) QueryBuilder {
	q.SelectQuery = q.SelectQuery.WhereGroup(sep, func(qs *SelectQuery) *SelectQuery {
		return fn(q).(*selectQueryBuilder).SelectQuery
	})
	return q
}

// Where adds an AND condition to the query builder.
//
// Purpose:
//
//	Appends an AND condition predicate.
//
// Where it is used:
//   - In QueryBuilder implementations.
//
// When can it be used:
//   - When chaining WHERE filters.
//
// Parameters:
//   - query (string): Input value used by the operation.
//   - args (...any): Input value used by the operation.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Where and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) Where(query string, args ...any) QueryBuilder {
	q.SelectQuery.Where(query, args...)
	return q
}

// WhereOr adds an OR condition to the query builder.
//
// Purpose:
//
//	Appends an OR condition predicate.
//
// Where it is used:
//   - In QueryBuilder implementations.
//
// When can it be used:
//   - When chaining disjunctive WHERE filters.
//
// Parameters:
//   - query (string): Input value used by the operation.
//   - args (...any): Input value used by the operation.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by WhereOr and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) WhereOr(query string, args ...any) QueryBuilder {
	q.SelectQuery.WhereOr(query, args...)
	return q
}

// WhereDeleted filters for soft-deleted rows.
//
// Purpose:
//
//	Restricts query to records marked with non-null deleted_at.
//
// Where it is used:
//   - In soft delete handling.
//
// When can it be used:
//   - When querying deleted records for auditing or restoration.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by WhereDeleted and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) WhereDeleted() QueryBuilder {
	q.SelectQuery.WhereDeleted()
	return q
}

// WhereAllWithDeleted includes both active and soft-deleted rows.
//
// Purpose:
//
//	Bypasses soft-delete filtering so all records are selected.
//
// Where it is used:
//   - In administrative or historical queries.
//
// When can it be used:
//   - When retrieving full audit histories.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by WhereAllWithDeleted and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) WhereAllWithDeleted() QueryBuilder {
	q.SelectQuery.WhereAllWithDeleted()
	return q
}

// WherePK adds primary key equality constraints.
//
// Purpose:
//
//	Matches rows against table primary key columns.
//
// Where it is used:
//   - In find-by-primary-key operations.
//
// When can it be used:
//   - When targeting single records by identity.
//
// Parameters:
//   - cols (...string): Input value used by the operation.
//
// Returns:
//   - result1 (QueryBuilder): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by WherePK and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) WherePK(cols ...string) QueryBuilder {
	q.SelectQuery.WherePK(cols...)
	return q
}

// Unwrap returns the underlying SelectQuery.
//
// Purpose:
//
//	Unwraps the selectQueryBuilder adapter to expose the raw SelectQuery.
//
// Where it is used:
//   - In query compilation callers requiring access to the base SelectQuery.
//
// When can it be used:
//   - When passing the concrete query to database execution methods.
//
// Parameters:
//   - None.
//
// Returns:
//   - result1 (any): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by Unwrap and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (q *selectQueryBuilder) Unwrap() any {
	return q.SelectQuery
}
