package query

// FilterOp defines the comparison operator in dynamic filters.
type FilterOp string

const (
	OpEq        FilterOp = "EQ"
	OpNeq       FilterOp = "NEQ"
	OpGt        FilterOp = "GT"
	OpGte       FilterOp = "GTE"
	OpLt        FilterOp = "LT"
	OpLte       FilterOp = "LTE"
	OpIn        FilterOp = "IN"
	OpNin       FilterOp = "NIN"
	OpLike      FilterOp = "LIKE"
	OpILike     FilterOp = "ILIKE"
	OpIsNull    FilterOp = "IS_NULL"
	OpIsNotNull FilterOp = "IS_NOT_NULL"
	OpBetween   FilterOp = "BETWEEN"
)

// Filter represents a single field comparison criterion.
type Filter struct {
	Field    string   `json:"field"`
	Op       FilterOp `json:"op"`
	Value    any      `json:"value"`
	ValueTo  any      `json:"value_to,omitempty"` // For BETWEEN
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

// Query represents a rich, database-independent query specification.
type Query struct {
	Fields     []string   `json:"fields,omitempty"`
	Filters    []Filter   `json:"filters,omitempty"`
	LogicalOp  LogicalOp  `json:"logical_op,omitempty"`
	Sorts      []Sort     `json:"sorts,omitempty"`
	Pagination Pagination `json:"pagination"`
	CountTotal bool       `json:"count_total,omitempty"`
}

// NewQuery returns a default Query instance.
func NewQuery() Query {
	return Query{
		LogicalOp: OpAnd,
		Pagination: Pagination{
			Limit:  50,
			Offset: 0,
		},
	}
}

// Where adds a filter to the query.
func (q Query) Where(field string, op FilterOp, val any) Query {
	q.Filters = append(q.Filters, Filter{
		Field: field,
		Op:    op,
		Value: val,
	})
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

// LimitOffset sets the limit and offset.
func (q Query) LimitOffset(limit, offset int) Query {
	q.Pagination.Limit = limit
	q.Pagination.Offset = offset
	return q
}
