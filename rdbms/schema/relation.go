// Package schema defines the table, field, and relation metadata constructs used
// by the RDBMS query engine to build SQL queries with joins and projection aliases.
//
// File: relation.go
// Usage:
//
//	Defines RelationJoin and its methods for formatting SQL JOIN clauses, appending
//	table aliases, prefixing column names, and evaluating relation join conditions.
package schema

import (
	"context"
	"fmt"
)

// RelationJoin represents a joined relation in a SelectQuery.
type RelationJoin struct {
	Relation                   *Relation
	JoinModel                  TableModel
	Columns                    []QueryWithArgs
	AdditionalJoinOnConditions []QueryWithArgs
	Apply                      any
	LoadWithChildren           bool // Controls whether child/nested relations and their columns are loaded
}

// NewRelationJoin creates a new RelationJoin wrapper for a relation and target table model.
//
// Purpose:
//
//	Initializes a RelationJoin state container linking a relation definition to its TableModel.
//
// Where it is used:
//   - In DynamicTableModel and SelectQuery when configuring joins.
//
// When can it be used:
//   - When preparing a relational JOIN between tables during query construction.
//
// Parameters:
//   - rel (*Relation): Input value used by the operation.
//   - joinModel (TableModel): Input value used by the operation.
//
// Returns:
//   - result1 (*RelationJoin): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by NewRelationJoin and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func NewRelationJoin(rel *Relation, joinModel TableModel) *RelationJoin {
	return &RelationJoin{
		Relation:         rel,
		JoinModel:        joinModel,
		LoadWithChildren: true, // Default to true, can be configured via RelationOpts
	}
}

// ApplyTo applies custom configurations or hooks to the query builder.
//
// Purpose:
//
//	Provides an extension hook for applying custom scopes or modifiers to a joined query.
//
// Where it is used:
//   - In SelectQuery when processing relation join modifiers.
//
// When can it be used:
//   - When custom filtering or scoping is required on a joined table.
//
// Parameters:
//   - q (any): Input value used by the operation.
//
// Returns:
//   - None.
//
// Business Rules:
//   - Follows the invariants documented by ApplyTo and its owning package.
//
// Side Effects:
//   - May update receiver state, persistent storage, or configured external dependencies.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (j *RelationJoin) ApplyTo(q any) {
	// Custom apply hook if provided
}

// AppendAlias appends the dialect-quoted table alias or table name to the SQL buffer.
//
// Purpose:
//
//	Quotes and appends the target relation's SQL alias to the query byte buffer.
//
// Where it is used:
//   - In query builders constructing qualified column references.
//
// When can it be used:
//   - When serializing table aliases in SQL statements.
//
// Parameters:
//   - gen (QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//
// Returns:
//   - result1 ([]byte): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by AppendAlias and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (j *RelationJoin) AppendAlias(gen QueryGen, b []byte) []byte {
	if j.Relation != nil && j.Relation.JoinTable != nil {
		alias := j.Relation.JoinTable.SQLAlias
		if alias == "" {
			alias = j.Relation.JoinTable.SQLName
		}
		if gen != nil {
			return gen.Dialect().AppendIdent(b, alias)
		}
		return append(b, alias...)
	}
	return b
}

// AppendAliasColumn appends a namespaced column alias (e.g., relation__col) to the SQL buffer.
//
// Purpose:
//
//	Generates uniquely prefixed column aliases for joined tables to avoid name collisions in SELECT.
//
// Where it is used:
//   - In SelectQuery when projecting columns from joined relations.
//
// When can it be used:
//   - When flattening multi-table query projections into a single result row.
//
// Parameters:
//   - gen (QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//   - col (string): Input value used by the operation.
//
// Returns:
//   - result1 ([]byte): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by AppendAliasColumn and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - This function does not return an error.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (j *RelationJoin) AppendAliasColumn(gen QueryGen, b []byte, col string) []byte {
	alias := ""
	if j.Relation != nil {
		alias = j.Relation.Name + "__" + col
	} else {
		alias = col
	}
	if gen != nil {
		return gen.Dialect().AppendIdent(b, alias)
	}
	return append(b, alias...)
}

// AppendHasOneJoin generates and appends the SQL "LEFT JOIN ... ON ..." clause to the buffer.
//
// Purpose:
//
//	Renders the dialect-formatted JOIN clause including table name, alias, ON conditions,
//	and any additional custom JOIN predicates.
//
// Where it is used:
//   - In SelectQuery.AppendQuery when assembling relational FROM and JOIN clauses.
//
// When can it be used:
//   - Whenever generating SQL for a belongs-to or has-one relational join.
//
// Parameters:
//   - gen (QueryGen): Input value used by the operation.
//   - b ([]byte): Input value used by the operation.
//   - q (any): Input value used by the operation.
//
// Returns:
//   - result1 ([]byte): Returned value.
//   - result2 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by AppendHasOneJoin and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - Callers should preserve the documented ownership and lifecycle of supplied values.
func (j *RelationJoin) AppendHasOneJoin(gen QueryGen, b []byte, q any) ([]byte, error) {
	if j.Relation == nil || j.Relation.JoinTable == nil {
		return b, nil
	}

	b = append(b, "LEFT JOIN "...)
	tbl := j.Relation.JoinTable
	if gen != nil {
		b = gen.Dialect().AppendIdent(b, tbl.SQLName)
		if tbl.SQLAlias != "" && tbl.SQLAlias != tbl.SQLName {
			b = append(b, " AS "...)
			b = gen.Dialect().AppendIdent(b, tbl.SQLAlias)
		}
	} else {
		b = append(b, tbl.SQLName...)
	}

	b = append(b, " ON "...)
	fk := j.Relation.ForeignKey
	if fk == "" {
		fk = "id"
	}
	pk := j.Relation.TargetKey
	if pk == "" {
		pk = "id"
	}

	joinAlias := tbl.SQLAlias
	if joinAlias == "" {
		joinAlias = tbl.SQLName
	}

	if gen != nil {
		b = gen.Dialect().AppendIdent(b, fmt.Sprintf("%s.%s", joinAlias, pk))
		b = append(b, " = "...)
		// Parent key reference
		b = append(b, fmt.Sprintf("%s", fk)...)
	} else {
		b = append(b, fmt.Sprintf("%s.%s = %s", joinAlias, pk, fk)...)
	}

	for _, cond := range j.AdditionalJoinOnConditions {
		b = append(b, " AND ("...)
		var err error
		b, err = cond.AppendQuery(gen, b)
		if err != nil {
			return nil, err
		}
		b = append(b, ')')
	}

	return b, nil
}

// SelectMany executes child query loading for HasMany relationships.
//
// Purpose:
//
//	Provides an execution hook for fetching child collection rows in 1:N relations.
//
// Where it is used:
//   - In eager loading stages of SelectQuery execution.
//
// When can it be used:
//   - When resolving HasMany relationship queries.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - q (any): Input value used by the operation.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by SelectMany and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (j *RelationJoin) SelectMany(ctx context.Context, q any) error {
	name := "<unnamed>"
	if j != nil && j.Relation != nil && j.Relation.Name != "" {
		name = j.Relation.Name
	}
	return fmt.Errorf("generic RDBMS has-many loading for relation %q is unsupported; use the CRUD engine relation hydrator or a native adapter implementation", name)
}

// SelectM2M executes junction table query loading for ManyToMany relationships.
//
// Purpose:
//
//	Provides an execution hook for fetching associative rows in M:N relations.
//
// Where it is used:
//   - In eager loading stages of SelectQuery execution.
//
// When can it be used:
//   - When resolving ManyToMany relationship queries.
//
// Parameters:
//   - ctx (context.Context): Input value used by the operation.
//   - q (any): Input value used by the operation.
//
// Returns:
//   - result1 (error): Returned value.
//
// Business Rules:
//   - Follows the invariants documented by SelectM2M and its owning package.
//
// Side Effects:
//   - Does not intentionally mutate caller-owned input; configured dependencies may still be consulted.
//
// Errors:
//   - Returns validation, configuration, context, or underlying dependency errors.
//
// Note:
//   - The context controls cancellation and request lifetime.
func (j *RelationJoin) SelectM2M(ctx context.Context, q any) error {
	name := "<unnamed>"
	if j != nil && j.Relation != nil && j.Relation.Name != "" {
		name = j.Relation.Name
	}
	return fmt.Errorf("generic RDBMS many-to-many loading for relation %q is unsupported; use the CRUD engine relation hydrator with junction metadata", name)
}
