// Package schema defines the table, field, and relation metadata constructs used
// by the RDBMS query engine to build SQL queries with joins and projection aliases.
//
// File: schema.go
// Usage:
//   Defines Table, Field, Relation, TableModel, QueryWithArgs, and QueryWithSep types
//   representing relational database schemas and parameter-bound query fragments.
package schema

import (
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
)

// QueryGen formats queries according to dialect rules.
type QueryGen interface {
	Dialect() dialect.Dialect
	IsNop() bool
	AppendQuery(b []byte, query string, args ...any) ([]byte, error)
}

// QueryWithArgs represents a raw SQL snippet with positional arguments.
type QueryWithArgs struct {
	Query string
	Args  []any
}

// SafeQuery constructs a QueryWithArgs.
//
// Purpose:
//   Encapsulates a SQL query fragment alongside its bound execution arguments.
//
// Where it is used:
//   - In query builders constructing WHERE conditions, SELECT expressions, and ORDER clauses.
//
// When can it be used:
//   - When passing parameterized SQL snippets safely without SQL injection risk.
func SafeQuery(query string, args []any) QueryWithArgs {
	return QueryWithArgs{
		Query: query,
		Args:  args,
	}
}

// UnsafeIdent constructs a QueryWithArgs that will be quoted as an identifier.
//
// Purpose:
//   Encapsulates an identifier string to be quoted according to dialect identifier rules.
//
// Where it is used:
//   - In query column projection and table reference wrappers.
//
// When can it be used:
//   - When emitting column or table names that need dialect quoting.
func UnsafeIdent(ident string) QueryWithArgs {
	return QueryWithArgs{
		Query: ident,
	}
}

// IsZero reports whether the QueryWithArgs is empty.
//
// Purpose:
//   Checks if the query string is blank and no arguments are present.
//
// Where it is used:
//   - In query compilation filters and conditional checks.
//
// When can it be used:
//   - When verifying whether a query snippet holds content before rendering.
func (q QueryWithArgs) IsZero() bool {
	return q.Query == "" && len(q.Args) == 0
}

// AppendQuery appends the formatted query and replaces parameters via dialect rules.
//
// Purpose:
//   Serializes the query fragment into the buffer, handling identifier quoting and parameter binding.
//
// Where it is used:
//   - In SelectQuery when assembling the final SQL query buffer.
//
// When can it be used:
//   - Whenever rendering a parameterized fragment into an active query buffer.
func (q QueryWithArgs) AppendQuery(gen QueryGen, b []byte) ([]byte, error) {
	if q.IsZero() {
		return b, nil
	}
	if gen == nil {
		return append(b, q.Query...), nil
	}
	// If no args and looks like a bare identifier (no spaces or SQL keywords), quote it
	if len(q.Args) == 0 && !strings.ContainsAny(q.Query, " ()'\"=,><+-*/") {
		return gen.Dialect().AppendIdent(b, q.Query), nil
	}
	return gen.AppendQuery(b, q.Query, q.Args...)
}

// QueryWithSep represents a query condition combined with a separator (e.g., AND/OR).
type QueryWithSep struct {
	Query string
	Args  []any
	Sep   string
}

// SafeQueryWithSep constructs a QueryWithSep.
//
// Purpose:
//   Builds a query fragment accompanied by a logical separator (e.g., " AND ", " OR ").
//
// Where it is used:
//   - In SelectQuery WHERE and HAVING clause builders.
//
// When can it be used:
//   - When chaining chained boolean conditions in SQL statements.
func SafeQueryWithSep(query string, args []any, sep string) QueryWithSep {
	return QueryWithSep{
		Query: query,
		Args:  args,
		Sep:   sep,
	}
}

// AppendQuery appends the separated query fragment to the buffer.
//
// Purpose:
//   Appends the conditional query snippet into the SQL buffer using dialect formatting.
//
// Where it is used:
//   - In SelectQuery clause generation loops.
//
// When can it be used:
//   - When serializing boolean filter criteria.
func (q QueryWithSep) AppendQuery(gen QueryGen, b []byte) ([]byte, error) {
	if q.Query == "" && len(q.Args) == 0 {
		return b, nil
	}
	if gen == nil {
		return append(b, q.Query...), nil
	}
	return gen.AppendQuery(b, q.Query, q.Args...)
}

// RelationType defines multiplicity in relational models.
type RelationType int

const (
	HasOneRelation RelationType = iota + 1
	BelongsToRelation
	HasManyRelation
	ManyToManyRelation
)

// Field represents an RDBMS table column.
type Field struct {
	Name     string
	SQLName  string
	DataType string
	IsPK     bool
}

// Table represents an RDBMS table metadata.
type Table struct {
	Name       string
	SQLName    string
	SQLAlias   string
	Fields     []*Field
	FieldMap   map[string]*Field
	PrimaryKey []*Field
	ZeroIface  any
}

// NewTable constructs a Table with default SQL name and alias.
//
// Purpose:
//   Initializes a new Table metadata container for column and key registrations.
//
// Where it is used:
//   - In TableModel construction and dialect introspection.
//
// When can it be used:
//   - When declaring or discovering a relational table schema.
func NewTable(name string) *Table {
	return &Table{
		Name:     name,
		SQLName:  name,
		SQLAlias: name,
		FieldMap: make(map[string]*Field),
	}
}

// AddField registers a Field on the Table.
//
// Purpose:
//   Adds a column definition to the fields slice, field lookup map, and primary key list if marked as PK.
//
// Where it is used:
//   - In NewTableModelFromModel and table schema definitions.
//
// When can it be used:
//   - When adding attributes to a table schema model.
func (t *Table) AddField(f *Field) {
	t.Fields = append(t.Fields, f)
	t.FieldMap[f.Name] = f
	t.FieldMap[f.SQLName] = f
	if f.IsPK {
		t.PrimaryKey = append(t.PrimaryKey, f)
	}
}

// Relation defines a relationship from one Table to another.
type Relation struct {
	Name        string
	Type        RelationType
	Field       *Field
	JoinTable   *Table
	Condition   []string
	ForeignKey  string
	TargetKey   string
	M2MTable    *Table
}

// TableModel abstracts models mapped to an RDBMS table with relations.
type TableModel interface {
	Table() *Table
	Clone() TableModel
	GetJoins() []*RelationJoin
	Join(name string) *RelationJoin
}
