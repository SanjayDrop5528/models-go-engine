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
func SafeQuery(query string, args []any) QueryWithArgs {
	return QueryWithArgs{
		Query: query,
		Args:  args,
	}
}

// UnsafeIdent constructs a QueryWithArgs that will be quoted as an identifier.
func UnsafeIdent(ident string) QueryWithArgs {
	return QueryWithArgs{
		Query: ident,
	}
}

func (q QueryWithArgs) IsZero() bool {
	return q.Query == "" && len(q.Args) == 0
}

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
func SafeQueryWithSep(query string, args []any, sep string) QueryWithSep {
	return QueryWithSep{
		Query: query,
		Args:  args,
		Sep:   sep,
	}
}

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

func NewTable(name string) *Table {
	return &Table{
		Name:     name,
		SQLName:  name,
		SQLAlias: name,
		FieldMap: make(map[string]*Field),
	}
}

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
