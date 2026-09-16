package query

import (
	"fmt"
	"strings"
)

// FilterOp defines the comparison operator in dynamic filters.
type FilterOp string

const (
	OpEq         FilterOp = "EQ"
	OpNeq        FilterOp = "NEQ"
	OpGt         FilterOp = "GT"
	OpGte        FilterOp = "GTE"
	OpLt         FilterOp = "LT"
	OpLte        FilterOp = "LTE"
	OpIn         FilterOp = "IN"
	OpNin        FilterOp = "NIN"
	OpLike       FilterOp = "LIKE"
	OpILike      FilterOp = "ILIKE"
	OpNotLike    FilterOp = "NOT_LIKE"
	OpStartsWith FilterOp = "STARTS_WITH"
	OpEndsWith   FilterOp = "ENDS_WITH"
	OpIsNull     FilterOp = "IS_NULL"
	OpIsNotNull  FilterOp = "IS_NOT_NULL"
	OpBetween    FilterOp = "BETWEEN"
)

// Filter represents a single field comparison criterion.
type Filter struct {
	Field   string   `json:"field"`
	Op      FilterOp `json:"op"`
	Value   any      `json:"value"`
	ValueTo any      `json:"value_to,omitempty"` // For BETWEEN
}

// SortOrder specifies ascending or descending order.
type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

// Sort represents a sorting criterion.
type Sort struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}

// Pagination defines offset/limit paging.
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// LogicalOp specifies whether filters are combined with AND or OR.
type LogicalOp string

const (
	OpAnd LogicalOp = "AND"
	OpOr  LogicalOp = "OR"
)

// JoinSpec defines a join clause across databases.
type JoinSpec struct {
	Type  string   `json:"type"` // "LEFT", "INNER", "RIGHT", "JOIN"
	Table string   `json:"table"`
	On    string   `json:"on"`
	Args  []any    `json:"args,omitempty"`
}

// RelationOpts configures eager relation loading behavior.
type RelationOpts struct {
	Apply            func(Query) Query `json:"-"`
	LoadWithChildren bool              `json:"load_with_children"`
	Conditions       []string          `json:"conditions,omitempty"`
	Fields           []string          `json:"fields,omitempty"`
	On               []string          `json:"on,omitempty"`
	Order            []Sort            `json:"order,omitempty"`
}

// RelationSpec stores relation configurations on the query.
type RelationSpec struct {
	Name             string            `json:"name"`
	Apply            func(Query) Query `json:"-"`
	LoadWithChildren bool              `json:"load_with_children"`
	Conditions       []string          `json:"conditions,omitempty"`
	Fields           []string          `json:"fields,omitempty"`
	On               []string          `json:"on,omitempty"`
	Order            []Sort            `json:"order,omitempty"`
	SubRelations     []RelationSpec    `json:"sub_relations,omitempty"`
}

// UnionSpec stores set operations on the query.
type UnionSpec struct {
	Expr  string `json:"expr"` // "UNION", "UNION ALL", "INTERSECT", "EXCEPT"
	Query *Query `json:"query"`
}

// RawExpr represents an expression with arguments.
type RawExpr struct {
	Query string `json:"query"`
	Args  []any  `json:"args,omitempty"`
}

// IndexHintSpec specifies database-specific index hints.
type IndexHintSpec struct {
	Use            []string `json:"use,omitempty"`
	UseForJoin     []string `json:"use_for_join,omitempty"`
	UseForOrderBy  []string `json:"use_for_order_by,omitempty"`
	UseForGroupBy  []string `json:"use_for_group_by,omitempty"`
	Ignore         []string `json:"ignore,omitempty"`
	Force          []string `json:"force,omitempty"`
}

// WhereGroupSpec represents a parenthesized condition group.
type WhereGroupSpec struct {
	Sep   string `json:"sep"`
	Query Query  `json:"query"`
}

// Query represents a unified, rich, multi-database query specification.
// It serves as a single universal data structure supporting PostgreSQL, MySQL,
// MongoDB, SQLite, and In-Memory stores.
type Query struct {
	// Projections & Targets
	Tables           []string         `json:"tables,omitempty"`
	Fields           []string         `json:"fields,omitempty"`
	ColumnExprs      []RawExpr        `json:"column_exprs,omitempty"`
	ExcludedColumns  []string         `json:"excluded_columns,omitempty"`
	ModelTarget      any              `json:"-"`
	DistinctFields   []string         `json:"distinct_fields,omitempty"`
	IsDistinct       bool             `json:"is_distinct,omitempty"`

	// Filters & Groups
	Filters          []Filter         `json:"filters,omitempty"`
	RawWheres        []RawExpr        `json:"raw_wheres,omitempty"`
	WhereGroups      []WhereGroupSpec `json:"where_groups,omitempty"`
	LogicalOp        LogicalOp        `json:"logical_op,omitempty"`
	Groups           []string         `json:"groups,omitempty"`
	Havings          []RawExpr        `json:"havings,omitempty"`

	// Joins & Relations
	Joins            []JoinSpec       `json:"joins,omitempty"`
	Relations        []string         `json:"relations,omitempty"`
	RelationSpecs    []RelationSpec   `json:"relation_specs,omitempty"`
	LoadWithChildren bool             `json:"load_with_children,omitempty"`

	// Set Operations
	Unions           []UnionSpec      `json:"unions,omitempty"`

	// Sorting & Pagination
	Sorts            []Sort           `json:"sorts,omitempty"`
	Pagination       Pagination       `json:"pagination"`
	CountTotal       bool             `json:"count_total,omitempty"`

	// Performance & Optimization
	IndexHints       IndexHintSpec    `json:"index_hints,omitempty"`
	CommentText      string           `json:"comment,omitempty"`
}

// New returns a fresh, fluent Query instance.
func New() Query {
	return NewQuery()
}

// NewQuery returns a default Query instance.
func NewQuery() Query {
	return Query{
		LogicalOp:        OpAnd,
		LoadWithChildren: true,
		Pagination: Pagination{
			Limit:  50,
			Offset: 0,
		},
	}
}

// Table specifies target table(s) or collections to query.
func (q Query) Table(tables ...string) Query {
	q.Tables = append(q.Tables, tables...)
	return q
}

// Model sets the destination model object or struct for reflection.
func (q Query) Model(m any) Query {
	q.ModelTarget = m
	return q
}

// Column sets the projected columns or fields.
func (q Query) Column(columns ...string) Query {
	q.Fields = append(q.Fields, columns...)
	return q
}

// ColumnExpr adds a computed column expression with optional arguments.
func (q Query) ColumnExpr(expr string, args ...any) Query {
	q.ColumnExprs = append(q.ColumnExprs, RawExpr{Query: expr, Args: args})
	return q
}

// ExcludeColumn excludes specific columns from being retrieved.
func (q Query) ExcludeColumn(columns ...string) Query {
	q.ExcludedColumns = append(q.ExcludedColumns, columns...)
	return q
}

// Distinct adds a DISTINCT constraint to eliminate duplicates.
func (q Query) Distinct() Query {
	q.IsDistinct = true
	return q
}

// DistinctOn adds a DISTINCT ON clause.
func (q Query) DistinctOn(fields ...string) Query {
	q.DistinctFields = append(q.DistinctFields, fields...)
	return q
}

// Where adds a filter condition. Supports both legacy: Where(field, op, val) and raw: Where("status = ?", "active").
func (q Query) Where(exprOrField string, args ...any) Query {
	if len(args) == 2 {
		if op, ok := args[0].(FilterOp); ok {
			return q.WhereFilter(exprOrField, op, args[1])
		}
	}
	if len(args) > 0 || strings.ContainsAny(exprOrField, " =<>!~") {
		q.RawWheres = append(q.RawWheres, RawExpr{Query: exprOrField, Args: args})
		return q
	}
	// Fallback single field comparison
	return q.WhereFilter(exprOrField, OpEq, nil)
}

// WhereFilter adds a structured Filter criterion.
func (q Query) WhereFilter(field string, op FilterOp, val any) Query {
	q.Filters = append(q.Filters, Filter{
		Field: field,
		Op:    op,
		Value: val,
	})
	return q
}

// WhereOr adds an OR condition to the query.
func (q Query) WhereOr(expr string, args ...any) Query {
	q.LogicalOp = OpOr
	q.RawWheres = append(q.RawWheres, RawExpr{Query: expr, Args: args})
	return q
}

// WhereGroup nests a group of filters joined by the given separator (" AND " or " OR ").
func (q Query) WhereGroup(sep string, fn func(Query) Query) Query {
	subInit := NewQuery()
	if strings.Contains(strings.ToUpper(sep), "OR") {
		subInit.LogicalOp = OpOr
	}
	sub := fn(subInit)
	q.WhereGroups = append(q.WhereGroups, WhereGroupSpec{
		Sep:   sep,
		Query: sub,
	})
	return q
}

// WherePK adds primary key filter conditions.
func (q Query) WherePK(cols ...string) Query {
	for _, col := range cols {
		q.RawWheres = append(q.RawWheres, RawExpr{Query: col + " = ?"})
	}
	return q
}

// WhereDeleted adds a soft-delete filter criterion.
func (q Query) WhereDeleted() Query {
	q.RawWheres = append(q.RawWheres, RawExpr{Query: "deleted_at IS NOT NULL"})
	return q
}

// WhereAllWithDeleted includes both active and soft-deleted rows.
func (q Query) WhereAllWithDeleted() Query {
	return q
}

// Group adds columns to the GROUP BY clause.
func (q Query) Group(columns ...string) Query {
	q.Groups = append(q.Groups, columns...)
	return q
}

// Having adds a HAVING clause condition.
func (q Query) Having(expr string, args ...any) Query {
	q.Havings = append(q.Havings, RawExpr{Query: expr, Args: args})
	return q
}

// Order adds raw order expressions.
func (q Query) Order(orders ...string) Query {
	for _, o := range orders {
		parts := strings.Fields(o)
		if len(parts) >= 2 && strings.EqualFold(parts[1], "DESC") {
			q.Sorts = append(q.Sorts, Sort{Field: parts[0], Order: SortDesc})
		} else if len(parts) >= 1 {
			q.Sorts = append(q.Sorts, Sort{Field: parts[0], Order: SortAsc})
		}
	}
	return q
}

// OrderBy adds a sort spec.
func (q Query) OrderBy(field string, order SortOrder) Query {
	q.Sorts = append(q.Sorts, Sort{
		Field: field,
		Order: order,
	})
	return q
}

// Limit sets the maximum number of records to return.
func (q Query) Limit(n int) Query {
	q.Pagination.Limit = n
	return q
}

// Offset sets the number of records to skip.
func (q Query) Offset(n int) Query {
	q.Pagination.Offset = n
	return q
}

// LimitOffset sets both the limit and offset.
func (q Query) LimitOffset(limit, offset int) Query {
	q.Pagination.Limit = limit
	q.Pagination.Offset = offset
	return q
}

// Join adds a JOIN clause across tables or collections.
func (q Query) Join(joinTypeAndTable string, onConditions ...string) Query {
	on := strings.Join(onConditions, " AND ")
	q.Joins = append(q.Joins, JoinSpec{
		Type:  "JOIN",
		Table: joinTypeAndTable,
		On:    on,
	})
	return q
}

// JoinOn adds an ON condition to the latest join.
func (q Query) JoinOn(cond string, args ...any) Query {
	if len(q.Joins) > 0 {
		idx := len(q.Joins) - 1
		if q.Joins[idx].On == "" {
			q.Joins[idx].On = cond
		} else {
			q.Joins[idx].On += " AND " + cond
		}
		q.Joins[idx].Args = append(q.Joins[idx].Args, args...)
	}
	return q
}

// JoinOnOr adds an OR ON condition to the latest join.
func (q Query) JoinOnOr(cond string, args ...any) Query {
	if len(q.Joins) > 0 {
		idx := len(q.Joins) - 1
		if q.Joins[idx].On == "" {
			q.Joins[idx].On = cond
		} else {
			q.Joins[idx].On += " OR " + cond
		}
		q.Joins[idx].Args = append(q.Joins[idx].Args, args...)
	}
	return q
}

// Relation registers a relation to join or populate eagerly.
func (q Query) Relation(name string, apply ...func(Query) Query) Query {
	spec := RelationSpec{
		Name:             name,
		LoadWithChildren: q.LoadWithChildren,
	}
	if len(apply) > 0 && apply[0] != nil {
		spec.Apply = apply[0]
		sub := apply[0](New())
		spec.LoadWithChildren = sub.LoadWithChildren
		spec.Fields = sub.Fields
		spec.Order = sub.Sorts
		spec.SubRelations = sub.RelationSpecs
		for _, f := range sub.Filters {
			opStr := "="
			switch f.Op {
			case OpEq:
				opStr = "="
			case OpNeq:
				opStr = "<>"
			case OpGt:
				opStr = ">"
			case OpGte:
				opStr = ">="
			case OpLt:
				opStr = "<"
			case OpLte:
				opStr = "<="
			}
			spec.Conditions = append(spec.Conditions, fmt.Sprintf("%s %s '%v'", f.Field, opStr, f.Value))
		}
	}
	q.Relations = append(q.Relations, name)
	q.RelationSpecs = append(q.RelationSpecs, spec)
	return q
}

// RelationWithOpts configures a relation with explicit options.
func (q Query) RelationWithOpts(name string, opts RelationOpts) Query {
	spec := RelationSpec{
		Name:             name,
		Apply:            opts.Apply,
		LoadWithChildren: opts.LoadWithChildren,
		Conditions:       opts.Conditions,
		Fields:           opts.Fields,
		On:               opts.On,
		Order:            opts.Order,
	}
	if opts.Apply != nil {
		sub := opts.Apply(New())
		if len(sub.Fields) > 0 && len(spec.Fields) == 0 {
			spec.Fields = sub.Fields
		}
		if len(sub.Sorts) > 0 && len(spec.Order) == 0 {
			spec.Order = sub.Sorts
		}
		if len(sub.RelationSpecs) > 0 {
			spec.SubRelations = sub.RelationSpecs
		}
	}
	q.Relations = append(q.Relations, name)
	q.RelationSpecs = append(q.RelationSpecs, spec)
	return q
}

// WithChildren sets whether related children models should be loaded.
func (q Query) WithChildren(load bool) Query {
	q.LoadWithChildren = load
	return q
}

// WithRelations adds relations to load.
func (q Query) WithRelations(relations ...string) Query {
	q.Relations = append(q.Relations, relations...)
	return q
}

// Union combines this query with another using UNION.
func (q Query) Union(other Query) Query {
	cp := other
	q.Unions = append(q.Unions, UnionSpec{
		Expr:  "UNION",
		Query: &cp,
	})
	return q
}

// UnionAll combines this query with another using UNION ALL.
func (q Query) UnionAll(other Query) Query {
	cp := other
	q.Unions = append(q.Unions, UnionSpec{
		Expr:  "UNION ALL",
		Query: &cp,
	})
	return q
}

// Intersect combines this query with another using INTERSECT.
func (q Query) Intersect(other Query) Query {
	cp := other
	q.Unions = append(q.Unions, UnionSpec{
		Expr:  "INTERSECT",
		Query: &cp,
	})
	return q
}

// Except combines this query with another using EXCEPT.
func (q Query) Except(other Query) Query {
	cp := other
	q.Unions = append(q.Unions, UnionSpec{
		Expr:  "EXCEPT",
		Query: &cp,
	})
	return q
}

// UseIndex adds a USE INDEX hint.
func (q Query) UseIndex(indexes ...string) Query {
	q.IndexHints.Use = append(q.IndexHints.Use, indexes...)
	return q
}

// UseIndexForJoin adds a USE INDEX FOR JOIN hint.
func (q Query) UseIndexForJoin(indexes ...string) Query {
	q.IndexHints.UseForJoin = append(q.IndexHints.UseForJoin, indexes...)
	return q
}

// UseIndexForOrderBy adds a USE INDEX FOR ORDER BY hint.
func (q Query) UseIndexForOrderBy(indexes ...string) Query {
	q.IndexHints.UseForOrderBy = append(q.IndexHints.UseForOrderBy, indexes...)
	return q
}

// UseIndexForGroupBy adds a USE INDEX FOR GROUP BY hint.
func (q Query) UseIndexForGroupBy(indexes ...string) Query {
	q.IndexHints.UseForGroupBy = append(q.IndexHints.UseForGroupBy, indexes...)
	return q
}

// IgnoreIndex adds an IGNORE INDEX hint.
func (q Query) IgnoreIndex(indexes ...string) Query {
	q.IndexHints.Ignore = append(q.IndexHints.Ignore, indexes...)
	return q
}

// ForceIndex adds a FORCE INDEX hint.
func (q Query) ForceIndex(indexes ...string) Query {
	q.IndexHints.Force = append(q.IndexHints.Force, indexes...)
	return q
}

// Comment sets a comment tag on the query.
func (q Query) Comment(comment string) Query {
	q.CommentText = comment
	return q
}
