package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/SanjayDrop5528/models-go-engine/rdbms/dialect"
	"github.com/SanjayDrop5528/models-go-engine/rdbms/schema"
)

var errNilModel = errors.New("rdbms: model is nil")

// Order specifies sort order.
type Order string

const (
	OrderAsc     Order = "ASC"
	OrderDesc    Order = "DESC"
	OrderDefault Order = ""
)

// IConn defines the standard database connection interface.
type IConn interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Query defines an RDBMS executable or appendable query.
type Query interface {
	Operation() string
	AppendQuery(gen schema.QueryGen, b []byte) ([]byte, error)
}

// QueryBuilder wraps a query in a fluent conditional builder.
type QueryBuilder interface {
	Where(query string, args ...any) QueryBuilder
	WhereOr(query string, args ...any) QueryBuilder
	WhereGroup(sep string, fn func(QueryBuilder) QueryBuilder) QueryBuilder
	WhereDeleted() QueryBuilder
	WhereAllWithDeleted() QueryBuilder
	WherePK(cols ...string) QueryBuilder
	Unwrap() any
}

// DB represents the database handle used by queries.
type DB struct {
	gen     schema.QueryGen
	dialect dialect.Dialect
	db      *sql.DB
}

func NewDB(db *sql.DB, d dialect.Dialect) *DB {
	if d == nil {
		d = dialect.NewPostgreSQL()
	}
	return &DB{
		dialect: d,
		db:      db,
		gen:     &defaultQueryGen{dialect: d},
	}
}

func (db *DB) Dialect() dialect.Dialect {
	return db.dialect
}

func (db *DB) QueryGen() schema.QueryGen {
	return db.gen
}

func (db *DB) DB() *sql.DB {
	return db.db
}

func (db *DB) NewSelect() *SelectQuery {
	return NewSelectQuery(db)
}

func (db *DB) makeQueryBytes() []byte {
	return make([]byte, 0, 512)
}

func (db *DB) beforeQuery(ctx context.Context, q any, query string, args []any, formatted string, model any) (context.Context, any) {
	return ctx, nil
}

func (db *DB) afterQuery(ctx context.Context, event any, res sql.Result, err error) {
}

type defaultQueryGen struct {
	dialect dialect.Dialect
}

func (g *defaultQueryGen) Dialect() dialect.Dialect {
	return g.dialect
}

func (g *defaultQueryGen) IsNop() bool {
	return false
}

func (g *defaultQueryGen) AppendQuery(b []byte, query string, args ...any) ([]byte, error) {
	if len(args) == 0 {
		return append(b, query...), nil
	}

	argIdx := 1
	var out []byte
	queryLen := len(query)

	for i := 0; i < queryLen; i++ {
		if query[i] == '?' {
			if argIdx-1 < len(args) {
				val := args[argIdx-1]
				argIdx++
				out = g.appendValue(out, val)
			} else {
				out = append(out, '?')
			}
		} else {
			out = append(out, query[i])
		}
	}

	return append(b, out...), nil
}

func (g *defaultQueryGen) appendValue(b []byte, val any) []byte {
	switch v := val.(type) {
	case nil:
		return append(b, "NULL"...)
	case string:
		return g.dialect.AppendString(b, v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return append(b, fmt.Sprintf("%d", v)...)
	case float32, float64:
		return append(b, fmt.Sprintf("%v", v)...)
	case bool:
		if v {
			return append(b, "TRUE"...)
		}
		return append(b, "FALSE"...)
	default:
		return g.dialect.AppendString(b, fmt.Sprintf("%v", v))
	}
}

// WithQuery represents a Common Table Expression.
type WithQuery struct {
	name      string
	recursive bool
	query     Query
}

func NewWithQuery(name string, query Query) *WithQuery {
	return &WithQuery{
		name:  name,
		query: query,
	}
}

func (w *WithQuery) Recursive() *WithQuery {
	w.recursive = true
	return w
}

func (w *WithQuery) AppendQuery(gen schema.QueryGen, b []byte) ([]byte, error) {
	if gen != nil {
		b = gen.Dialect().AppendIdent(b, w.name)
	} else {
		b = append(b, w.name...)
	}
	b = append(b, " AS ("...)
	var err error
	if w.query != nil {
		b, err = w.query.AppendQuery(gen, b)
		if err != nil {
			return nil, err
		}
	}
	b = append(b, ')')
	return b, nil
}

// indexHints specifies index usage hints.
type indexHints struct {
	names      []schema.QueryWithArgs
	forJoin    []schema.QueryWithArgs
	forOrderBy []schema.QueryWithArgs
	forGroupBy []schema.QueryWithArgs
}

type idxHintsQuery struct {
	use    *indexHints
	ignore *indexHints
	force  *indexHints
}

func (q *idxHintsQuery) addUseIndex(indexes ...string) {
	if q.use == nil {
		q.use = &indexHints{}
	}
	for _, idx := range indexes {
		q.use.names = append(q.use.names, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addUseIndexForJoin(indexes ...string) {
	if q.use == nil {
		q.use = &indexHints{}
	}
	for _, idx := range indexes {
		q.use.forJoin = append(q.use.forJoin, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addUseIndexForOrderBy(indexes ...string) {
	if q.use == nil {
		q.use = &indexHints{}
	}
	for _, idx := range indexes {
		q.use.forOrderBy = append(q.use.forOrderBy, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addUseIndexForGroupBy(indexes ...string) {
	if q.use == nil {
		q.use = &indexHints{}
	}
	for _, idx := range indexes {
		q.use.forGroupBy = append(q.use.forGroupBy, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addIgnoreIndex(indexes ...string) {
	if q.ignore == nil {
		q.ignore = &indexHints{}
	}
	for _, idx := range indexes {
		q.ignore.names = append(q.ignore.names, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addIgnoreIndexForJoin(indexes ...string) {
	if q.ignore == nil {
		q.ignore = &indexHints{}
	}
	for _, idx := range indexes {
		q.ignore.forJoin = append(q.ignore.forJoin, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addIgnoreIndexForOrderBy(indexes ...string) {
	if q.ignore == nil {
		q.ignore = &indexHints{}
	}
	for _, idx := range indexes {
		q.ignore.forOrderBy = append(q.ignore.forOrderBy, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addIgnoreIndexForGroupBy(indexes ...string) {
	if q.ignore == nil {
		q.ignore = &indexHints{}
	}
	for _, idx := range indexes {
		q.ignore.forGroupBy = append(q.ignore.forGroupBy, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addForceIndex(indexes ...string) {
	if q.force == nil {
		q.force = &indexHints{}
	}
	for _, idx := range indexes {
		q.force.names = append(q.force.names, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addForceIndexForJoin(indexes ...string) {
	if q.force == nil {
		q.force = &indexHints{}
	}
	for _, idx := range indexes {
		q.force.forJoin = append(q.force.forJoin, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addForceIndexForOrderBy(indexes ...string) {
	if q.force == nil {
		q.force = &indexHints{}
	}
	for _, idx := range indexes {
		q.force.forOrderBy = append(q.force.forOrderBy, schema.SafeQuery(idx, nil))
	}
}

func (q *idxHintsQuery) addForceIndexForGroupBy(indexes ...string) {
	if q.force == nil {
		q.force = &indexHints{}
	}
	for _, idx := range indexes {
		q.force.forGroupBy = append(q.force.forGroupBy, schema.SafeQuery(idx, nil))
	}
}

type orderLimitOffsetQuery struct {
	order  []schema.QueryWithArgs
	limit  int
	offset int
}

func (q *orderLimitOffsetQuery) addOrder(orders ...string) {
	for _, o := range orders {
		q.order = append(q.order, schema.SafeQuery(o, nil))
	}
}

func (q *orderLimitOffsetQuery) addOrderBy(col string, dir Order) {
	expr := col
	if dir != OrderDefault {
		expr = fmt.Sprintf("%s %s", col, dir)
	}
	q.order = append(q.order, schema.SafeQuery(expr, nil))
}

func (q *orderLimitOffsetQuery) addOrderExpr(query string, args ...any) {
	q.order = append(q.order, schema.SafeQuery(query, args))
}

func (q *orderLimitOffsetQuery) setLimit(n int) {
	q.limit = n
}

func (q *orderLimitOffsetQuery) setOffset(n int) {
	q.offset = n
}

type baseQuery struct {
	db             *DB
	table          *schema.Table
	model          any
	tableModel     schema.TableModel
	with           []WithQuery
	tables         []schema.QueryWithArgs
	columns        []schema.QueryWithArgs
	modelTableName schema.QueryWithArgs
	err            error
	conn           IConn
}

func (q *baseQuery) setConn(c IConn) {
	q.conn = c
}

func (q *baseQuery) setModel(m any) {
	q.model = m
	if tm, ok := m.(schema.TableModel); ok {
		q.tableModel = tm
		q.table = tm.Table()
	} else if conv, ok := m.(interface{ ToTableModel() schema.TableModel }); ok {
		tm := conv.ToTableModel()
		if tm != nil {
			q.tableModel = tm
			q.table = tm.Table()
		}
	}
}

func (q *baseQuery) setErr(err error) {
	if q.err == nil {
		q.err = err
	}
}

func (q *baseQuery) addWith(w *WithQuery) {
	if w != nil {
		q.with = append(q.with, *w)
	}
}

func (q *baseQuery) addTable(t schema.QueryWithArgs) {
	q.tables = append(q.tables, t)
}

func (q *baseQuery) addColumn(c schema.QueryWithArgs) {
	q.columns = append(q.columns, c)
}

func (q *baseQuery) excludeColumn(cols []string) {
	if q.table == nil {
		return
	}
	excludeMap := make(map[string]bool)
	for _, c := range cols {
		excludeMap[c] = true
	}
	var filtered []schema.QueryWithArgs
	for _, f := range q.table.Fields {
		if !excludeMap[f.Name] && !excludeMap[f.SQLName] {
			filtered = append(filtered, schema.UnsafeIdent(f.SQLName))
		}
	}
	q.columns = filtered
}

func (q *baseQuery) hasTables() bool {
	return len(q.tables) > 0 || q.table != nil || !q.modelTableName.IsZero()
}

type whereBaseQuery struct {
	baseQuery
	where []schema.QueryWithSep
}

func (q *whereBaseQuery) addWhereCols(cols []string) {
	if q.table == nil || len(q.table.PrimaryKey) == 0 {
		return
	}
	for _, pk := range q.table.PrimaryKey {
		q.where = append(q.where, schema.SafeQueryWithSep(fmt.Sprintf("%s = ?", pk.SQLName), nil, " AND "))
	}
}

func (q *whereBaseQuery) addWhere(w schema.QueryWithSep) {
	q.where = append(q.where, w)
}

func (q *whereBaseQuery) addWhereGroup(sep string, sub []schema.QueryWithSep) {
	if len(sub) == 0 {
		return
	}
	var conds []string
	var args []any
	for _, s := range sub {
		conds = append(conds, s.Query)
		args = append(args, s.Args...)
	}
	joined := strings.Join(conds, sep)
	q.where = append(q.where, schema.SafeQueryWithSep("("+joined+")", args, " AND "))
}

func (q *whereBaseQuery) whereDeleted() {
	q.where = append(q.where, schema.SafeQueryWithSep("deleted_at IS NOT NULL", nil, " AND "))
}

func (q *whereBaseQuery) whereAllWithDeleted() {
	// No-op or removes soft-delete filtering
}
