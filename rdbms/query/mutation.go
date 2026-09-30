package query

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Tx is an engine query handle bound to a database transaction.
type Tx struct {
	db *DB
	tx *sql.Tx
}

func (t *Tx) NewSelect() *SelectQuery { return NewSelectQuery(t.db).Conn(t.tx) }
func (t *Tx) NewInsert() *InsertQuery { return newInsertQuery(t.db, t.tx) }
func (t *Tx) NewUpdate() *UpdateQuery { return newUpdateQuery(t.db, t.tx) }
func (t *Tx) NewRaw(statement string, args ...any) *RawQuery {
	return &RawQuery{conn: t.tx, statement: statement, args: args}
}
func (t *Tx) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, q, args...)
}
func (t *Tx) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, q, args...)
}
func (t *Tx) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, q, args...)
}
func (t *Tx) Commit() error   { return t.tx.Commit() }
func (t *Tx) Rollback() error { return t.tx.Rollback() }

// NewInsert starts an engine-owned INSERT query.
func (db *DB) NewInsert() *InsertQuery { return newInsertQuery(db, db.db) }

// NewUpdate starts an engine-owned UPDATE query.
func (db *DB) NewUpdate() *UpdateQuery { return newUpdateQuery(db, db.db) }

type mutationBase struct {
	db        *DB
	conn      IConn
	model     any
	table     string
	columns   []string
	wheres    []string
	whereArgs []any
	sets      []string
	setArgs   []any
}

// RawQuery executes parameterized SQL through an engine-owned connection.
type RawQuery struct {
	conn      IConn
	statement string
	args      []any
}

// TableInfo contains engine-resolved relational model metadata.
type TableInfo struct{ SQLName string }

// CreateTableQuery creates a table from model metadata for legacy bootstrap
// paths. Runtime applications should prefer schema plans.
type CreateTableQuery struct {
	conn        IConn
	modelType   reflect.Type
	ifNotExists bool
}

func (q *CreateTableQuery) Model(model any) *CreateTableQuery {
	q.modelType = reflect.TypeOf(model)
	return q
}
func (q *CreateTableQuery) IfNotExists() *CreateTableQuery { q.ifNotExists = true; return q }
func (q *CreateTableQuery) Exec(ctx context.Context) (sql.Result, error) {
	modelType := q.modelType
	for modelType != nil && (modelType.Kind() == reflect.Pointer || modelType.Kind() == reflect.Slice) {
		modelType = modelType.Elem()
	}
	if modelType == nil || modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("rdbms: create table model must be a struct")
	}
	columns := make([]string, 0, modelType.NumField())
	collectDDLFields(modelType, &columns)
	prefix := "CREATE TABLE "
	if q.ifNotExists {
		prefix += "IF NOT EXISTS "
	}
	return q.conn.ExecContext(ctx, prefix+inferTable(q.modelType, "")+" ("+strings.Join(columns, ", ")+")")
}

func collectDDLFields(modelType reflect.Type, columns *[]string) {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if field.Anonymous {
			nested := field.Type
			for nested.Kind() == reflect.Pointer {
				nested = nested.Elem()
			}
			if nested.Kind() == reflect.Struct {
				collectDDLFields(nested, columns)
			}
			continue
		}
		name := taggedColumn(field)
		if name == "" || name == "-" {
			continue
		}
		definition := name + " " + sqlType(field.Type)
		if name == "id" {
			definition += " PRIMARY KEY"
		}
		*columns = append(*columns, definition)
	}
}

func sqlType(fieldType reflect.Type) string {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	if fieldType.PkgPath() == "github.com/google/uuid" && fieldType.Name() == "UUID" {
		return "UUID"
	}
	if fieldType == reflect.TypeOf(time.Time{}) {
		return "TIMESTAMPTZ"
	}
	switch fieldType.Kind() {
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE PRECISION"
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		return "JSONB"
	default:
		return "TEXT"
	}
}

func (q *RawQuery) Exec(ctx context.Context) (sql.Result, error) {
	statement, args := compileRaw(q.statement, q.args)
	return q.conn.ExecContext(ctx, statement, args...)
}

func (q *RawQuery) Scan(ctx context.Context, destinations ...any) error {
	statement, args := compileRaw(q.statement, q.args)
	rows, err := q.conn.QueryContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	return scanRows(rows, destinations...)
}

func compileRaw(statement string, values []any) (string, []any) {
	var args []any
	var out strings.Builder
	valueIndex := 0
	for _, char := range statement {
		if char != '?' || valueIndex >= len(values) {
			out.WriteRune(char)
			continue
		}
		value := values[valueIndex]
		valueIndex++
		if list, ok := value.(InValues); ok {
			items := reflect.ValueOf(list.Values)
			if !items.IsValid() || (items.Kind() != reflect.Slice && items.Kind() != reflect.Array) || items.Len() == 0 {
				out.WriteString("NULL")
				continue
			}
			for i := 0; i < items.Len(); i++ {
				if i > 0 {
					out.WriteString(", ")
				}
				args = append(args, items.Index(i).Interface())
				out.WriteString(fmt.Sprintf("$%d", len(args)))
			}
			continue
		}
		args = append(args, value)
		out.WriteString(fmt.Sprintf("$%d", len(args)))
	}
	return out.String(), args
}

// InsertQuery builds parameterized INSERT statements from models or explicit values.
type InsertQuery struct {
	mutationBase
	onClause string
}

func newInsertQuery(db *DB, conn IConn) *InsertQuery {
	return &InsertQuery{mutationBase: mutationBase{db: db, conn: conn}}
}
func (q *InsertQuery) Model(value any) *InsertQuery                 { q.model = value; return q }
func (q *InsertQuery) Table(name string) *InsertQuery               { q.table = name; return q }
func (q *InsertQuery) TableExpr(name string, _ ...any) *InsertQuery { q.table = name; return q }
func (q *InsertQuery) Column(names ...string) *InsertQuery {
	q.columns = append(q.columns, names...)
	return q
}
func (q *InsertQuery) On(clause string, _ ...any) *InsertQuery { q.onClause = clause; return q }
func (q *InsertQuery) Set(expr string, args ...any) *InsertQuery {
	q.sets = append(q.sets, expr)
	q.setArgs = append(q.setArgs, args...)
	return q
}

func (q *InsertQuery) Exec(ctx context.Context) (sql.Result, error) {
	if q.conn == nil {
		return nil, sql.ErrConnDone
	}
	rows, table, err := mutationRows(q.model, q.table, q.columns)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("rdbms: insert model is empty")
	}
	columns := sortedKeys(rows[0])
	var args []any
	valueGroups := make([]string, 0, len(rows))
	for _, row := range rows {
		placeholders := make([]string, len(columns))
		for i, column := range columns {
			args = append(args, row[column])
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ", ")+")")
	}
	statement := "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") VALUES " + strings.Join(valueGroups, ", ")
	if q.onClause != "" {
		statement += " " + q.onClause
		if len(q.sets) > 0 {
			statement += " SET " + strings.Join(q.sets, ", ")
		}
	}
	return q.conn.ExecContext(ctx, statement, args...)
}

// UpdateQuery builds parameterized UPDATE statements.
type UpdateQuery struct{ mutationBase }

func newUpdateQuery(db *DB, conn IConn) *UpdateQuery {
	return &UpdateQuery{mutationBase: mutationBase{db: db, conn: conn}}
}
func (q *UpdateQuery) Model(value any) *UpdateQuery                 { q.model = value; return q }
func (q *UpdateQuery) Table(name string) *UpdateQuery               { q.table = name; return q }
func (q *UpdateQuery) TableExpr(name string, _ ...any) *UpdateQuery { q.table = name; return q }
func (q *UpdateQuery) Column(names ...string) *UpdateQuery {
	q.columns = append(q.columns, names...)
	return q
}
func (q *UpdateQuery) Set(expr string, args ...any) *UpdateQuery {
	q.sets = append(q.sets, expr)
	q.setArgs = append(q.setArgs, args...)
	return q
}
func (q *UpdateQuery) Where(expr string, args ...any) *UpdateQuery {
	q.wheres = append(q.wheres, expr)
	q.whereArgs = append(q.whereArgs, args...)
	return q
}
func (q *UpdateQuery) WherePK(columns ...string) *UpdateQuery {
	row, _, err := mutationRow(q.model, q.table, nil)
	if err != nil {
		return q
	}
	if len(columns) == 0 {
		columns = []string{"id"}
	}
	for _, column := range columns {
		q.Where(column+" = ?", row[column])
	}
	return q
}

func (q *UpdateQuery) Exec(ctx context.Context) (sql.Result, error) {
	if q.conn == nil {
		return nil, sql.ErrConnDone
	}
	_, table, err := mutationRow(q.model, q.table, nil)
	if err != nil && table == "" {
		return nil, err
	}
	sets := append([]string(nil), q.sets...)
	args := append([]any(nil), q.setArgs...)
	if len(sets) == 0 {
		row, _, rowErr := mutationRow(q.model, table, q.columns)
		if rowErr != nil {
			return nil, rowErr
		}
		columns := sortedKeys(row)
		for _, column := range columns {
			if column == "id" {
				continue
			}
			sets = append(sets, column+" = ?")
			args = append(args, row[column])
		}
	}
	if len(sets) == 0 {
		return nil, fmt.Errorf("rdbms: update has no values")
	}
	statement := "UPDATE " + table + " SET " + strings.Join(sets, ", ")
	if len(q.wheres) > 0 {
		statement += " WHERE " + strings.Join(q.wheres, " AND ")
		args = append(args, q.whereArgs...)
	}
	statement = postgresPlaceholders(statement)
	return q.conn.ExecContext(ctx, statement, args...)
}

var questionMark = regexp.MustCompile(`\?`)

func postgresPlaceholders(statement string) string {
	index := 0
	return questionMark.ReplaceAllStringFunc(statement, func(string) string {
		index++
		return fmt.Sprintf("$%d", index)
	})
}

func mutationRows(model any, explicitTable string, selected []string) ([]map[string]any, string, error) {
	value := reflect.ValueOf(model)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, inferTable(reflect.TypeOf(model), explicitTable), fmt.Errorf("rdbms: nil insert model")
		}
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Slice {
		result := make([]map[string]any, 0, value.Len())
		table := explicitTable
		for i := 0; i < value.Len(); i++ {
			row, inferred, err := mutationRow(value.Index(i).Interface(), table, selected)
			if err != nil {
				return nil, "", err
			}
			table = inferred
			result = append(result, row)
		}
		return result, table, nil
	}
	row, table, err := mutationRow(model, explicitTable, selected)
	return []map[string]any{row}, table, err
}

func mutationRow(model any, explicitTable string, selected []string) (map[string]any, string, error) {
	table := inferTable(reflect.TypeOf(model), explicitTable)
	if table == "" {
		return nil, "", fmt.Errorf("rdbms: table is required")
	}
	value := reflect.ValueOf(model)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return map[string]any{}, table, nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil, table, fmt.Errorf("rdbms: mutation model must be a struct")
	}
	allowed := make(map[string]bool, len(selected))
	for _, column := range selected {
		allowed[column] = true
	}
	row := make(map[string]any)
	collectMutationFields(value, row, allowed)
	return row, table, nil
}

func collectMutationFields(value reflect.Value, row map[string]any, allowed map[string]bool) {
	typeOf := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldInfo := typeOf.Field(i)
		field := value.Field(i)
		if fieldInfo.Anonymous {
			for field.Kind() == reflect.Pointer && !field.IsNil() {
				field = field.Elem()
			}
			if field.IsValid() && field.Kind() == reflect.Struct {
				collectMutationFields(field, row, allowed)
			}
			continue
		}
		column := taggedColumn(fieldInfo)
		if column == "" || column == "-" || (len(allowed) > 0 && !allowed[column]) || !field.CanInterface() {
			continue
		}
		if field.Type() == reflect.TypeOf(time.Time{}) && field.Interface().(time.Time).IsZero() {
			continue
		}
		row[column] = field.Interface()
	}
}

func inferTable(modelType reflect.Type, explicit string) string {
	if explicit != "" {
		return strings.Fields(explicit)[0]
	}
	if modelType == nil {
		return ""
	}
	for modelType.Kind() == reflect.Pointer || modelType.Kind() == reflect.Slice {
		modelType = modelType.Elem()
	}
	zero := reflect.New(modelType)
	if named, ok := zero.Interface().(interface{ TableName() string }); ok {
		return named.TableName()
	}
	name := snakeCase(modelType.Name())
	if !strings.HasSuffix(name, "s") {
		name += "s"
	}
	pkg := modelType.PkgPath()
	if slash := strings.LastIndex(pkg, "/"); slash >= 0 {
		pkg = pkg[slash+1:]
	}
	if pkg != "" && pkg != "main" {
		return pkg + "." + name
	}
	return name
}

func snakeCase(value string) string {
	var out []rune
	for i, r := range value {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out = append(out, '_')
		}
		out = append(out, []rune(strings.ToLower(string(r)))...)
	}
	return string(out)
}

func sortedKeys(row map[string]any) []string {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
